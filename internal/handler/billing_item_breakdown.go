package handler

import (
	"context"
	"strings"
	"sync"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// 第 40 轮：分时明细行的「基准电量拆解」过程量。
//
// 展开「市场化购电费 / 零售输配电费」的四段明细时，用户需要看到基准电量是怎么
// 由「电表电量 + 差额电量 + 线损 + 加减电量」一步步构成的，而不只是最终那个数。
//
// 恒等式（每个时段，取两位小数后严格成立）：
//
//	billing_qty = meter_qty + diff_qty + line_loss_qty + adjust_qty
//
// 2026-10 持睿汽车实测：
//
//	尖峰 7800 + 170.13 + 4.97 − 387.67 = 7587.43
//	峰   47200 + 1735.12 + 19.34 − 904.65 = 48049.81
//	平   56600 + 1970.43 + 28.46 − 791.91 = 57806.98
//	谷   54950 + 1974.31 + 36.20 − 516.98 = 56443.53
//	合计 166550 + 5850 + 88.97 − 2601.21 = 169887.75
//
// 为什么单独存一份：`billing_record_meters` 是「逐表 × 逐时段」的 12 行，
// 明细行是「费用项 × 时段」的 8 行，两者的对应关系要现场聚合。把四分量固化在
// 明细行上，列表/打印/审计三处读同一份数据，不会各算一套。

// timedItemBreakdown 一个时段的四分量与基准计费电量。
type timedItemBreakdown struct {
	MeterQty    float64 `json:"meter_qty"`
	DiffQty     float64 `json:"diff_qty"`
	LineLossQty float64 `json:"line_loss_qty"`
	AdjustQty   float64 `json:"adjust_qty"`
	BillingQty  float64 `json:"billing_qty"`
}

// finalizeBreakdown 四分量取整 + 基准电量恒等式（**唯一口径**）。
//
// timedBreakdownOf（内存 timedPlan，供单测/审计）与 breakdownsFromMeters
// （落库路径，从 billing_record_meters 聚合）都必须经过这里，避免两处
// 各自 round2 导致明细与库里差 1 分。
func finalizeBreakdown(meter, diff, loss, adjust float64) timedItemBreakdown {
	b := timedItemBreakdown{
		MeterQty: round2(meter), DiffQty: round2(diff),
		LineLossQty: round2(loss), AdjustQty: round2(adjust),
	}
	// 基准电量由**取整后的四分量**相加得出：展示层的
	// 「电表 + 差额 + 线损 + 加减 == 计费数量」必须逐分严格成立，
	// 用未取整值相加会在尾数上差 0.01，用户按列一加就对不上。
	b.BillingQty = round2(b.MeterQty + b.DiffQty + b.LineLossQty + b.AdjustQty)
	return b
}

// timedBreakdownOf 从分时分摊结果聚合某个时段的四分量。
//
// period 是英文键（deep/peak/flat/valley），与 timedMeterRow.Period 一致。
// DiffKwh 在 allocateTimedKwh 里按表逐段 round2，四段累加会有 ≤0.01 的
// 取整尾差（2026-10 为 5849.99 vs ΔQ 5850）；明细行保留真实分摊值，
// 合计行另按 ΔQ 名义值展示并在表头注明尾差 —— 绝不反过去改计费电量，
// 否则会动到已锁定的分时电费金额。
func timedBreakdownOf(plan *timedPlan, period string) timedItemBreakdown {
	var meter, diff, loss, adjust float64
	if plan == nil {
		return finalizeBreakdown(0, 0, 0, 0)
	}
	for _, rows := range plan.Alloc.Rows {
		for _, r := range rows {
			if r.Period != period {
				continue
			}
			meter += r.Usage
			diff += r.DiffKwh
			loss += r.Loss
			adjust += r.Adjust
		}
	}
	return finalizeBreakdown(meter, diff, loss, adjust)
}

// isSystemRunningFeeCategory 是否「系统运行费」大类。
//
// 第 40 轮真机缺陷：2026-10 账单里「辅助服务费用」rate=0 / fee=0，
// 旧实现先算 unitRate（得 0）再走「有单价才按度计费」分支，于是掉进
// 「账单原值 × 分摊比例」兜底，展示成 27.63% / 0；后端甚至因为 rawTotal=0
// 整行不生成，导致系统运行费只有 7 项。
//
// 系统运行费大类下的**所有**子项（含单价为 0 的）都必须走平段按度口径：
// 计费数量 = 工业总电量，单价 = 账单该行单价（可为 0），金额 = 数量 × 单价。
func isSystemRunningFeeCategory(cat string) bool {
	return strings.Contains(cat, "系统运行费")
}

// ---------------------------------------------------------------------------
// 明细行拆解列的持久化
// ---------------------------------------------------------------------------
//
// 铁律：不改 internal/types（BillingRecordItem 是上游类型定义文件）。
// 所以用「幂等 DDL 加列 + map 更新」把过程量写到 billing_record_items，
// 结构体本身保持只读。读侧（列表 / 打印）仍走前端同源纯函数，不依赖这些列，
// 列只用于审计与后续核对。

const breakdownDDL = `
ALTER TABLE billing_record_items
  ADD COLUMN IF NOT EXISTS meter_qty      NUMERIC(18,2) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS diff_qty       NUMERIC(18,2) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS line_loss_qty  NUMERIC(18,2) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS adjust_qty     NUMERIC(18,2) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS billing_qty    NUMERIC(18,2) NOT NULL DEFAULT 0
`

var breakdownDDLOnce sync.Once

func ensureBreakdownColumns(ctx context.Context, db *gorm.DB) error {
	var err error
	breakdownDDLOnce.Do(func() {
		if db == nil {
			return
		}
		err = db.WithContext(ctx).Exec(breakdownDDL).Error
		if err != nil {
			logger.Warnf(ctx, "ensure billing item breakdown columns failed: %v", err)
		}
	})
	return err
}

// breakdownsFromMeters 把「逐表 × 逐时段」的电量明细行按 period 聚合成四分量。
//
// 与 timedBreakdownOf（从内存 timedPlan 聚合，供单测/审计）共用
// finalizeBreakdown 口径；这里是**落库路径**：persistRecord 已经持有
// meterRows，不必把 plan 再传一遍，也不必改它的签名。
func breakdownsFromMeters(meterRows []types.BillingRecordMeter) map[string]timedItemBreakdown {
	type bucket struct {
		meter, diff, loss, adjust float64
	}
	byPeriod := make(map[string]*bucket, 4)
	for _, r := range meterRows {
		p := strings.TrimSpace(r.Period)
		if p == "" {
			continue
		}
		b := byPeriod[p]
		if b == nil {
			b = &bucket{}
			byPeriod[p] = b
		}
		b.meter += r.Usage
		b.diff += r.DiffKwh
		b.loss += r.LineLoss
		b.adjust += r.Adjust
	}
	out := make(map[string]timedItemBreakdown, len(byPeriod))
	for p, v := range byPeriod {
		out[p] = finalizeBreakdown(v.meter, v.diff, v.loss, v.adjust)
	}
	return out
}

// writeBreakdowns 在事务内把拆解写入刚插入的分时明细行。
func writeBreakdowns(
	tx *gorm.DB, items []types.BillingRecordItem, meterRows []types.BillingRecordMeter,
) error {
	if len(meterRows) == 0 {
		return nil
	}
	byPeriod := breakdownsFromMeters(meterRows)
	if len(byPeriod) == 0 {
		return nil
	}
	for _, it := range items {
		if it.Kind != "fee" || it.Period == "" {
			continue
		}
		b, ok := byPeriod[it.Period]
		if !ok {
			continue
		}
		upd := map[string]any{
			"meter_qty": b.MeterQty, "diff_qty": b.DiffQty,
			"line_loss_qty": b.LineLossQty, "adjust_qty": b.AdjustQty,
			"billing_qty": b.BillingQty,
		}
		if err := tx.Table("billing_record_items").Where("id = ?", it.ID).Updates(upd).Error; err != nil {
			return err
		}
	}
	return nil
}

// BackfillSystemRunningAuxRow 存量账单补「辅助服务费用」0 元明细行（第 40 轮，幂等）。
//
// 账单原文里这一项单价与金额都是 0，旧实现整行丢弃；但分摊子项配置里
// field_enabled=t（前端展开面板照常列出 8 项），于是前端只能按「账单原值 ×
// 分摊比例」补出 27.63% / 0 的假口径。
//
// 为什么只补行、不整条重生成：这一行金额恒为 0，重生成会覆盖用户已核对过的
// 其它明细（并要跨洋重新取数 8s/条）；插入一行 0 元记录既让库内历史记录与
// 展开面板对齐，又**不改变 2026-09 / 2026-10 已锁定的总金额**。
func (h *BillingHandler) BackfillSystemRunningAuxRow(ctx context.Context) (int, error) {
	if h.db == nil {
		return 0, nil
	}
	var recs []types.BillingRecord
	if err := h.db.WithContext(ctx).Where("deleted_at IS NULL").Find(&recs).Error; err != nil {
		return 0, err
	}
	patched := 0
	for i := range recs {
		rec := &recs[i]
		// 该记录是否已有辅助服务费用行
		var exists int64
		if err := h.db.WithContext(ctx).Model(&types.BillingRecordItem{}).
			Where("record_id = ? AND kind = ? AND name = ?", rec.ID, "fee", "辅助服务费用").
			Count(&exists).Error; err != nil {
			continue
		}
		if exists > 0 {
			continue
		}
		// 取同大类任一行的 category 与最大 sort，保证新行紧跟在系统运行费末尾
		var ref types.BillingRecordItem
		if err := h.db.WithContext(ctx).
			Where("record_id = ? AND kind = ? AND category LIKE ?", rec.ID, "fee", "%系统运行费%").
			Order("sort DESC").First(&ref).Error; err != nil {
			continue
		}
		row := types.BillingRecordItem{
			ID: uuid.NewString(), RecordID: rec.ID, Kind: "fee",
			Category: ref.Category, Name: "辅助服务费用", Period: "",
			Qty: round2(rec.TotalKwh), Rate: 0, Fee: 0,
			Sort: ref.Sort + 1, CreatedAt: timeNowUTC(),
		}
		if err := h.db.WithContext(ctx).Create(&row).Error; err != nil {
			logger.Warnf(ctx, "insert system running aux row %s failed: %v", rec.ID, err)
			continue
		}
		patched++
	}
	if patched > 0 {
		logger.Infof(ctx, "system running aux rows backfilled: %d", patched)
	}
	return patched, nil
}

// BackfillBillingItemBreakdowns 启动幂等回填：已有账单从 billing_record_meters
// 聚合四分量写回明细行（含历史上缺列的行）。
//
// 为什么从 meters 表而不是重跑 prepareBillingInputs：后者每条要跨洋查 4~8 张表
// （实测 8s/条），而 meters 表已固化了逐表逐时段的 usage/line_loss/adjust/diff_kwh，
// 按 period 聚合即可，4 条记录一次查询搞定。
func (h *BillingHandler) BackfillBillingItemBreakdowns(ctx context.Context) (int, error) {
	if h.db == nil {
		return 0, nil
	}
	if err := ensureBreakdownColumns(ctx, h.db); err != nil {
		return 0, err
	}
	var recs []types.BillingRecord
	if err := h.db.WithContext(ctx).Where("deleted_at IS NULL").Find(&recs).Error; err != nil {
		return 0, err
	}
	type agg struct {
		Period   string
		Meter    float64
		Loss     float64
		Adjust   float64
		Diff     float64
		Billing  float64
	}
	patched := 0
	for i := range recs {
		var rows []agg
		if err := h.db.WithContext(ctx).Model(&types.BillingRecordMeter{}).
			Select(`period,
				COALESCE(SUM(usage),0) AS meter,
				COALESCE(SUM(line_loss),0) AS loss,
				COALESCE(SUM(adjust),0) AS adjust,
				COALESCE(SUM(diff_kwh),0) AS diff,
				COALESCE(SUM(bill_kwh + diff_kwh),0) AS billing`).
			Where("record_id = ?", recs[i].ID).
			Group("period").Scan(&rows).Error; err != nil {
			logger.Warnf(ctx, "aggregate billing meters %s failed: %v", recs[i].ID, err)
			continue
		}
		for _, r := range rows {
			if strings.TrimSpace(r.Period) == "" {
				continue
			}
			b := timedItemBreakdown{
				MeterQty: round2(r.Meter), DiffQty: round2(r.Diff),
				LineLossQty: round2(r.Loss), AdjustQty: round2(r.Adjust),
			}
			b.BillingQty = round2(b.MeterQty + b.DiffQty + b.LineLossQty + b.AdjustQty)
			upd := map[string]any{
				"meter_qty": b.MeterQty, "diff_qty": b.DiffQty,
				"line_loss_qty": b.LineLossQty, "adjust_qty": b.AdjustQty,
				"billing_qty": b.BillingQty,
			}
			res := h.db.WithContext(ctx).Table("billing_record_items").
				Where("record_id = ? AND kind = ? AND period = ? AND billing_qty = 0",
					recs[i].ID, "fee", r.Period).
				Updates(upd)
			if res.Error != nil {
				logger.Warnf(ctx, "update billing item breakdown %s/%s failed: %v", recs[i].ID, r.Period, res.Error)
				continue
			}
			patched += int(res.RowsAffected)
		}
	}
	if patched > 0 {
		logger.Infof(ctx, "billing item breakdowns backfilled: %d rows", patched)
	}
	return patched, nil
}
