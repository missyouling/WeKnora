package handler

// 账单生成的四段拆分（第 22 轮 P0-3）。
//
// 拆分前 `GenerateBillingRecord` 是一个 430+ 行的巨函数：解析请求、查库、分摊电量、
// 算全部费用、落库全揉在一起。任何一处口径改动都要在 400 行里找位置，而且「费用
// 明细」这一段完全没法单测 —— 只能靠起服务 + 真机对账，成本极高。
//
// 现在按职责切成四段，每段都可独立验证：
//
//	prepareBillingInputs  只读汇集：租户/设置/读数/账单/比例/子项开关
//	allocateTimed         分时电量分摊（计费电量 → 差额 → 基准电量）+ 明细卡片行
//	buildFeeItems         费用明细（分时 / 平段按度 / 基本电费 / 力调 / 宿舍 / 水费）
//	persistRecord         同月覆盖 + 子行写入（单事务）
//
// **行为与拆分前完全等价**：输入、中间量、错误分支、落库顺序逐一对应；差异只在
// 函数边界。收口基准见 testdata/billing-golden-2026-09.json（前后端共享）。

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
)

// billingInputs 生成一张账单所需的全部只读输入（一次查库，不再回头查）。
type billingInputs struct {
	TenantID   string
	TenantName string
	Month      string
	Setting    types.BillingTenantSetting

	// 分时读数：星达总表 / 租户分表（按 period 键）
	StarPeriod periodMap
	SubPeriod  periodMap
	SubRows    []subMeterRow

	StarTotal float64 // 星达分表总电量（= 工业总电量）
	SubTotal  float64 // 各分表抄见合计
	LineLoss  float64 // ΔQ = 星达 − 分表合计（负值按 0）

	// 宿舍/水表引用读数
	Refs       []types.BillingTenantMeterRef
	DormKwh    float64
	DormFee    float64
	WaterRows  []waterRow
	WaterUsage float64
	WaterFee   float64

	// 市电账单（缺失时为**零值账单**，不阻断生成）
	Bill *types.UtilityBillExtractionItem
	// Ratio 分摊比例 = 工业总电量 ÷ 账单总电量（8 位）。只作用于基本电费与力调电费。
	Ratio float64
	// BillLoss/BillAdjust 账单电量明细的（变损+线损）/加减电量，已乘 Ratio。
	BillLoss   map[string]float64
	BillAdjust map[string]float64

	// Enabled 子项开关（首次自动初始化，账单新增大类自动补充）
	Enabled map[string]bool
}

// timedPlan 分时分摊结果 + 电量明细卡片行。
type timedPlan struct {
	Alloc timedAllocation
	// BillPeriod 各时段基准电量（**未取整**，参与分时电费计算）
	BillPeriod map[string]float64
	// MeterRows 电量明细卡片行（表 × 时段）
	MeterRows []types.BillingRecordMeter
}

// feeBuildResult 费用明细计算结果。
type feeBuildResult struct {
	Items []types.BillingRecordItem
	// IndustrialFee 工业电费合计（**未取整累加**，落库前统一 round2）
	IndustrialFee float64
}

// prepareBillingInputs 汇集生成账单所需的全部输入。
//
// 口径（第 17 轮起）：**不再因缺读数/缺账单阻断生成**。缺分时读数按 0 参与计算，
// 缺市电账单用零值账单 —— 否则「2026-05 只缺一项分时读数」会导致整批月份都生成
// 不了；用户看到「电费待补」比看到 400 更有用。
func (h *BillingHandler) prepareBillingInputs(
	ctx context.Context, id, month string,
) (*billingInputs, error) {
	var tenant types.BillingTenant
	if err := h.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).First(&tenant).Error; err != nil {
		return nil, errors.NewNotFoundError("租户不存在")
	}
	var st types.BillingTenantSetting
	if err := h.db.WithContext(ctx).Where("billing_tenant_id = ?", id).First(&st).Error; err != nil {
		st = types.BillingTenantSetting{DormPrice: 1, WaterPrice: 5.22}
	}
	if st.DormPrice <= 0 {
		st.DormPrice = 1
	}
	if st.WaterPrice <= 0 {
		st.WaterPrice = 5.22
	}

	in := &billingInputs{
		TenantID:   id,
		TenantName: tenant.Name,
		Month:      month,
		Setting:    st,
	}

	// 1) 分时读数：总表（星达铜业）合计、租户分表合计
	starPeriod, subPeriod, err := h.loadPeriodKwh(ctx, id, tenant.Name, month)
	if err != nil {
		return nil, err
	}
	in.StarPeriod = starPeriod
	in.SubPeriod = subPeriod
	// 分时电表逐表读数（用于账单电量明细卡片）
	in.SubRows = h.loadSubMeterRows(ctx, tenant.Name, month)
	in.StarTotal = periodSum(starPeriod)
	in.SubTotal = periodSum(subPeriod)
	in.LineLoss = round2(in.StarTotal - in.SubTotal)
	if in.LineLoss < 0 {
		// 读数填反/分表大于总表：差额分摊按 0 处理（负的 ΔQ 会把分时电费算成负数）
		in.LineLoss = 0
	}

	// 2) 宿舍/水表引用读数（按使用单位=租户名自动匹配优先，显式引用兜底）
	refs := h.loadRefsByUseUnit(ctx, tenant.Name)
	if len(refs) == 0 {
		refs = h.loadMeterRefs(ctx, id)
	}
	in.Refs = refs
	dormKwh, dormFee, err := h.sumRefFee(ctx, refs, "electricity", month)
	if err != nil {
		return nil, err
	}
	in.DormKwh, in.DormFee = dormKwh, dormFee
	waterRows, waterUsage, waterFee, err := h.loadWaterRows(ctx, refs, month)
	if err != nil {
		return nil, err
	}
	in.WaterRows, in.WaterUsage, in.WaterFee = waterRows, waterUsage, waterFee

	// 3) 市电账单（缺失/电量为空不阻断：零值账单，所有市电分项金额 = 0）
	billItem, err := h.loadUtilityBillByMonth(ctx, st.BillKBID, month)
	if err != nil || billItem == nil {
		billItem = &types.UtilityBillExtractionItem{}
	}
	in.Bill = billItem
	if billItem.TotalKwh > 0 && in.StarTotal > 0 {
		// **8 位小数**：比例要参与两次乘法（基本电费 44,000、力调 -2,818.07），
		// 6 位会在 44,000 × 0.212356 上丢 1 分（9,343.66 vs 标准 9,343.67）。
		in.Ratio = round8(in.StarTotal / billItem.TotalKwh)
	}

	// 3.1 账单电量明细的损耗/加减电量（乘分摊比例）
	in.BillLoss, in.BillAdjust = billLossAdjust(billItem, in.Ratio)

	// 4) 子项开关：首次自动初始化，账单新增大类时自动补充
	enabled := h.loadEnabledItems(ctx, id)
	if len(enabled) == 0 {
		enabled = h.ensureDefaultItems(ctx, id, billItem.FeeItems)
	} else {
		enabled = h.syncMissingItems(ctx, id, billItem.FeeItems, enabled)
	}
	in.Enabled = enabled
	return in, nil
}

// billLossAdjust 把账单「电量明细」的（变损+线损）/加减电量折算到本租户。
//
// **必须乘分摊比例**：账单电量明细是**全厂**口径（尖峰线损 13 度、加减 -1341 度），
// 持睿只承担其中 ratio 那一份：13 × 0.21235625 = 2.76、-1341 × 0.21235625 = -284.77，
// 尖峰计费电量 = 5200 + 2.76 - 284.77 = 4,917.99。
//
// 历史坑：旧版本用的系数是「分表合计 ÷ 全厂电量」= 122700/595860 = 0.20592，
// 比正确的 0.21236 小 3%，尖峰算成 4,926.54（差 8.55 度），四段电费整体偏低。
//
// 变损与线损是两条独立列，电量明细里都体现在计费电量上，这里合并相加。
func billLossAdjust(
	bill *types.UtilityBillExtractionItem, ratio float64,
) (loss, adjust map[string]float64) {
	loss = make(map[string]float64, 4)
	adjust = make(map[string]float64, 4)
	for _, mr := range bill.MeterReadings {
		p := meterPeriodByType(mr.MeterType)
		if p == "" {
			continue
		}
		loss[p] = round2((mr.TransLoss + mr.LineLoss) * ratio)
		adjust[p] = round2(mr.Adjust * ratio)
	}
	return loss, adjust
}

// allocateTimed 分摊分时电量并生成「电量明细」卡片行。
//
// 卡片与费用必须同源（都取 allocateTimedKwh 的结果），否则「明细合计 ≠ 分时电费」
// —— 旧实现这里按 billBase×用量占比 另算了一套计费电量，卡片上 5,325.87、
// 标准表 4,927，两套账谁也对不上。
func allocateTimed(in *billingInputs, recordID string, now time.Time) *timedPlan {
	usages := make([]timedMeterUsage, 0, len(in.SubRows))
	for _, mr := range in.SubRows {
		usage := make(map[string]float64, 4)
		for _, p := range timedPeriodOrder {
			pr := mr.Periods[p]
			usage[p] = round2((pr.Curr - pr.Prev) * mr.Rate)
		}
		usages = append(usages, timedMeterUsage{
			Key: mr.MeterID, Name: mr.Name, Rate: mr.Rate, Usage: usage,
		})
	}
	alloc := allocateTimedKwh(usages, in.BillLoss, in.BillAdjust, in.LineLoss)

	// 各时段基准电量：用**未取整**值，取整后乘单价会在四段累加时产生 1 分误差
	period := make(map[string]float64, 4)
	for _, p := range timedPeriodOrder {
		period[p] = alloc.PeriodBaseExact[p]
	}

	rows := make([]types.BillingRecordMeter, 0, len(in.SubRows)*4)
	sortNo := 0
	for _, mr := range in.SubRows {
		for _, r := range alloc.Rows[mr.MeterID] {
			pr := mr.Periods[r.Period]
			rows = append(rows, types.BillingRecordMeter{
				ID: uuid.NewString(), RecordID: recordID, MeterName: mr.Name,
				Period: periodLabel(r.Period), Prev: pr.Prev, Curr: pr.Curr, Rate: mr.Rate,
				Usage: r.Usage, LineLoss: r.Loss, Adjust: r.Adjust,
				BillKwh: r.BillKwh, DiffKwh: r.DiffKwh, Sort: sortNo, CreatedAt: now,
			})
			sortNo++
		}
	}
	return &timedPlan{Alloc: alloc, BillPeriod: period, MeterRows: rows}
}

// buildFeeItems 生成全部费用明细（固化到 billing_record_items）。
//
// 三套口径并存，顺序不能换：
//   - 分时子项      = 逐表逐段 round2(基准电量 × 时段单价) 相加；
//   - 平段按度子项  = 工业总电量(126,600) × 账单该项单价；
//   - 基本电费/力调 = 账单原金额 × 工业电量 ÷ 账单电量（**不是** × ratio，见下）。
//
// 纯函数：不碰 DB、不读时钟以外的时间，便于用黄金基准直接断言。
func buildFeeItems(
	in *billingInputs, plan *timedPlan, recordID string, now time.Time,
) feeBuildResult {
	items := make([]types.BillingRecordItem, 0, 48)
	sortNo := 0
	industrialFee := 0.0
	starTotal := in.StarTotal
	ratio := in.Ratio
	enabled := in.Enabled

	// 5.1 账单 FeeItems 子项（按时段单价 / 计费基数×单价）
	for _, cat := range groupFeeItems(in.Bill.FeeItems) {
		// 居民/目录电费(定比单列)子项不参与按比例分摊
		if strings.Contains(cat.Name, "居民") || strings.Contains(cat.Name, "目录电费") {
			continue
		}
		// 力调电费由 5.2 统一计算，跳过账单原「功率因数/功率因素调整电费」大类，避免重复计费
		if strings.Contains(cat.Name, "功率因数调整电费") ||
			strings.Contains(cat.Name, "功率因素调整电费") {
			continue
		}
		// 子项开关控制（name 级）：同一大类下的同名子项可有多行（分时时段）
		for _, sg := range groupFeeItemsByName(cat.Rows) {
			// 开关:新格式「大类|子项名」优先,兼容旧格式(子项名)
			if !billingItemEnabled(enabled, cat.Name, sg.Name) {
				continue
			}
			// 子项大类严格跟随市电账单结构（零售损益分摊电费归属市场化购电费）
			catName := cat.Name
			rateByPeriod := make(map[string]float64, 4)
			seenPeriod := make(map[string]bool, 4)
			rawTotal := 0.0
			for _, r := range sg.Rows {
				rawTotal += r.Fee
				p := normalizePeriod(r.Period)
				if p == "" {
					continue
				}
				seenPeriod[p] = true
				rate := r.Rate
				if rate <= 0 && r.Qty > 0 {
					rate = r.Fee / r.Qty
				}
				if rate != 0 {
					rateByPeriod[p] = rate
				}
			}
			rawTotal = round2(rawTotal)

			// 真正分时子项:含尖峰/峰/谷任一时段(如零售交易、零售输配、偏差电费)
			if seenPeriod["deep"] || seenPeriod["peak"] || seenPeriod["valley"] {
				fee := 0.0
				for _, p := range timedPeriodOrder {
					rate, ok := rateByPeriod[p]
					if !ok || rate == 0 {
						continue
					}
					// **逐表逐段** round2 后相加：人工 Excel 是「每块表的每个时段各乘
					// 单价各占一格，合计再把格子加起来」，先汇总电量再乘单价会差 1 分
					// （2026-09 持睿：峰 6478.51+6859.73+2203.12 = 15541.36）。
					f := 0.0
					for _, mr := range in.SubRows {
						for _, r := range plan.Alloc.Rows[mr.MeterID] {
							if r.Period == p {
								f = round2(f + round2(r.BaseExact*rate))
								break
							}
						}
					}
					if f == 0 {
						// 没有分表明细（历史上只有汇总电量）时退回「时段合计电量 × 单价」
						f = round2(plan.BillPeriod[p] * rate)
					}
					items = append(items, types.BillingRecordItem{
						ID: uuid.NewString(), RecordID: recordID, Kind: "fee",
						Category: catName, Name: sg.Name, Period: periodLabel(p),
						Qty: round2(plan.BillPeriod[p]), Rate: rate, Fee: f,
						Sort: sortNo, CreatedAt: now,
					})
					sortNo++
					fee += f
				}
				industrialFee += fee
				continue
			}

			// 非分时子项（平段按度计费）：
			//   **计费数量 = 工业总电量（126,600），金额 = 电量 × 账单该项单价**
			//
			// 不能写「账单金额 × 分摊比例」：账单的按度项计费电量是 587,230 度
			// （= 全厂 596,168 − 居民 8,938），除以全厂电量得到的比例乘回去会
			// 系统性少算 —— 2026-09 零售损益分摊电费按比例算 4,434.02，标准表是
			// 4,501.52，差 67.5 元；上网环节线损差 32.5 元，系统运行 7 项 + 政府性
			// 基金 5 项逐项差一点，工业电费累计差 264.98。
			// 单价取账单该行的 rate；缺失时用 fee/qty 反算。
			unitRate := 0.0
			for _, r := range sg.Rows {
				if r.Rate != 0 {
					unitRate = r.Rate
					break
				}
				if r.Qty > 0 && r.Fee != 0 {
					unitRate = r.Fee / r.Qty
					break
				}
			}
			if unitRate != 0 && starTotal > 0 {
				qty := round2(starTotal)
				items = append(items, types.BillingRecordItem{
					ID: uuid.NewString(), RecordID: recordID, Kind: "fee",
					Category: catName, Name: sg.Name, Period: "",
					Qty: qty, Rate: unitRate, Fee: round2(qty * unitRate),
					Sort: sortNo, CreatedAt: now,
				})
				sortNo++
				// 累加**未取整**值：Excel 的合计是「各格原值相加后再四舍五入」，
				// 逐项 round 再相加会让政府性基金多 1 分（6,038.07 vs 6,038.06）。
				industrialFee += qty * unitRate
				continue
			}

			// 纯金额返还类（账单里 rate=0 且 qty=0）：**不计入分摊**。
			//
			// 「中长期偏差回收返还（用户）」「中长期超额申报费用返还」「零售交易发电侧
			// 收益返还电费」这三条在 2026-09 账单里是 -31,520.93 / -392.08 / -4,103.60，
			// 属于全厂冲抵项，没有计量口径可以折算到持睿头上。旧实现按「原金额 ×
			// 分摊比例」硬摊，凭空算出 -7,648.34，工业电费从 101,414.04 掉到 93,765.70。
			allZeroQty := true
			for _, r := range sg.Rows {
				if r.Qty != 0 {
					allZeroQty = false
					break
				}
			}
			if allZeroQty {
				continue
			}
			// 兜底：有计量口径但缺单价（历史脏数据）→ 按账单原金额 × 分摊比例
			if rawTotal != 0 {
				f := round2(rawTotal * ratio)
				items = append(items, types.BillingRecordItem{
					ID: uuid.NewString(), RecordID: recordID, Kind: "fee",
					Category: catName, Name: sg.Name, Period: "",
					Qty: ratio, Rate: rawTotal, Fee: f, Sort: sortNo, CreatedAt: now,
				})
				sortNo++
				industrialFee += f
			}
		}
	}

	// 5.2 基本电费 / 功率因素调整电费（按比例）
	if enabled["基本电费"] && in.Bill.CapacityFee != 0 {
		// 用「原金额 × 工业电量 ÷ 账单电量」而**不是** × ratio：
		// ratio 落库是 8 位近似（0.21235625），44,000 × 它 = 9,343.675 → 9,343.68；
		// 直接除得到 9,343.6749 → 9,343.67（与 Excel 一致）。
		exact := in.Bill.CapacityFee * starTotal / in.Bill.TotalKwh
		items = append(items, types.BillingRecordItem{
			ID: uuid.NewString(), RecordID: recordID, Kind: "base",
			Category: "基本电费", Name: "基本电费", Period: "",
			Qty: ratio, Rate: in.Bill.CapacityFee, Fee: round2(exact),
			Sort: sortNo, CreatedAt: now,
		})
		sortNo++
		industrialFee += exact
	}
	// 力调电费统一命名「功率因素调整电费」，但**必须同时认「因数」写法**。
	//
	// 库里 billing_tenant_items.item_key 是「功率因数调整电费|功率因数调整电费」
	// （账单原文用「因数」），而开关历史上写的是「因素」。硬按字面查三个键全不命中
	// → pfOn=false → 力调 -598.43 被整条丢掉，工业电费 101,414.04 变 102,012.47。
	if pfEnabled(enabled) && in.Bill.PfAdjustAmount != 0 {
		exact := in.Bill.PfAdjustAmount * starTotal / in.Bill.TotalKwh
		items = append(items, types.BillingRecordItem{
			ID: uuid.NewString(), RecordID: recordID, Kind: "pf",
			Category: "功率因素调整电费", Name: "功率因素调整电费", Period: "",
			Qty: ratio, Rate: in.Bill.PfAdjustAmount, Fee: round2(exact),
			Sort: sortNo, CreatedAt: now,
		})
		sortNo++
		industrialFee += exact
	}

	// 5.3 宿舍电费 / 水费（逐表）
	if in.DormFee != 0 {
		items = append(items, types.BillingRecordItem{
			ID: uuid.NewString(), RecordID: recordID, Kind: "dorm",
			Category: "居民电费", Name: "宿舍电费", Period: "",
			Qty: in.DormKwh, Rate: in.Setting.DormPrice, Fee: in.DormFee,
			Sort: sortNo, CreatedAt: now,
		})
		sortNo++
	}
	for _, wr := range in.WaterRows {
		items = append(items, types.BillingRecordItem{
			ID: uuid.NewString(), RecordID: recordID, Kind: "water",
			Category: "水费", Name: wr.MeterName, Period: "",
			Qty: wr.Usage, Rate: wr.Price, Fee: wr.Fee, Sort: sortNo, CreatedAt: now,
		})
		sortNo++
	}

	return feeBuildResult{Items: items, IndustrialFee: industrialFee}
}

// pfEnabled 判断「功率因素/功率因数调整电费」开关是否打开（两种写法都认）。
func pfEnabled(enabled map[string]bool) bool {
	for k, v := range enabled {
		if !v {
			continue
		}
		if strings.Contains(strings.ReplaceAll(k, "功率因素", "功率因数"), "功率因数调整电费") {
			return true
		}
	}
	return false
}

// persistRecord 单事务落库：同月已存在则整张覆盖（旧账单 + 三类子行一起删）。
//
// 必须是事务：旧实现分四步删除、再逐条插入，中途失败会留下「账单头在、子行没了」
// 的半张账单，页面上表现为明细空白但合计还在。
func (h *BillingHandler) persistRecord(
	ctx context.Context,
	rec types.BillingRecord,
	items []types.BillingRecordItem,
	meterRows []types.BillingRecordMeter,
	waterRows []waterRow,
) error {
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old types.BillingRecord
		if err := tx.Where("billing_tenant_id = ? AND month = ?", rec.BillingTenantID, rec.Month).
			First(&old).Error; err == nil {
			if err := tx.Where("record_id = ?", old.ID).Delete(&types.BillingRecordItem{}).Error; err != nil {
				return err
			}
			if err := tx.Where("record_id = ?", old.ID).Delete(&types.BillingRecordMeter{}).Error; err != nil {
				return err
			}
			if err := tx.Where("record_id = ?", old.ID).Delete(&types.BillingRecordWater{}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&old).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&rec).Error; err != nil {
			return err
		}
		for i := range items {
			if err := tx.Create(&items[i]).Error; err != nil {
				return err
			}
		}
		for i := range meterRows {
			if err := tx.Create(&meterRows[i]).Error; err != nil {
				return err
			}
		}
		for _, wr := range waterRows {
			w := types.BillingRecordWater{
				ID: uuid.NewString(), RecordID: rec.ID, MeterName: wr.MeterName,
				MeterKind: wr.MeterKind, Prev: wr.Prev, Curr: wr.Curr, Rate: wr.Rate,
				Usage: wr.Usage, Price: wr.Price, Fee: wr.Fee, Sort: wr.Sort, CreatedAt: rec.CreatedAt,
			}
			if err := tx.Create(&w).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
