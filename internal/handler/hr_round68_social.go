// 第 68 轮：社保月度账单「不可变快照」+ 原始记录变更提醒 + 按月物理隔离。
//
// 三条铁律：
//  ① 账单一旦生成，汇总行与个人/单位/部门明细全部固化进 `hr_si_bills.detail_snapshot`
//     （JSONB）。查看列表、打开明细弹窗、导出 Excel、打印预览一律只读快照，
//     绝不再实时回查 hr_si_records —— 事后增删改该月记录不影响已生成账单；
//  ② 该月记录被导入/编辑/删除后，账单置 `source_dirty=true`，接口回传
//     `bill_dirty_warning`，前端提示用户手动重新生成；重新生成会写新快照并清标记；
//  ③ 所有读写/覆盖 SQL 必须带 bill_month（唯一索引兜底，见 hrEnsureSIRecordIndex68）。
package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
)

// hrSIBillSnapshotVersion68 快照结构版本（后续字段变更时递增，读取侧按版本兼容）。
const hrSIBillSnapshotVersion68 = 1

// hrSIBillSnapshot68 账单快照：summary + 个人明细 + 部门汇总一次固化。
type hrSIBillSnapshot68 struct {
	Version     int              `json:"version"`
	Month       string           `json:"month"`
	GeneratedAt string           `json:"generated_at"`
	Summary     map[string]any   `json:"summary"`
	Items       []map[string]any `json:"items"`
	DeptSummary []map[string]any `json:"dept_summary"`
}

// hrSIBuildSnapshot68 由「生成瞬间」的汇总 + 明细构造快照。
func hrSIBuildSnapshot68(month string, summary HRSIBill, items []HRSIBillItem) map[string]any {
	itemViews := make([]map[string]any, 0, len(items))
	for _, it := range items {
		itemViews = append(itemViews, map[string]any{
			"id": it.ID, "序号": it.Sequence, "姓名": it.Name, "部门": it.Department,
			"证件号码": it.IDNumber, "个人编号": it.PersonCode,
			"基数": it.Base, "缴费基数": it.Base,
			"养老个人": it.PPension, "医疗个人": it.PMedical, "大额医疗": it.PBig,
			"失业个人": it.PUnemployment, "个人小计": it.PTotal,
			"养老单位": it.UPension, "医疗+生育单位": it.UMedical, "工伤单位": it.UInjury,
			"失业单位": it.UUnemployment, "单位小计": it.UTotal,
			"是否补缴": it.IsAdjustment,
		})
	}
	snap := hrSIBillSnapshot68{
		Version:     hrSIBillSnapshotVersion68,
		Month:       month,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Summary:     hrSIBillView(summary),
		Items:       itemViews,
		DeptSummary: hrSIDeptSummaryOf(items),
	}
	// 用 map 便于 JSONB 序列化（struct 也可以，但统一 map 读取侧更简单）。
	return map[string]any{
		"version":      snap.Version,
		"month":        snap.Month,
		"generated_at": snap.GeneratedAt,
		"summary":      snap.Summary,
		"items":        snap.Items,
		"dept_summary": snap.DeptSummary,
	}
}

// hrSIBillSnapshotItems68 从快照读取明细（保留原始金额，绝不回查记录表）。
func hrSIBillSnapshotItems68(snap map[string]any) []map[string]any {
	if snap == nil {
		return nil
	}
	raw, ok := snap["items"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []map[string]any:
		return v
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, it := range v {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// hrNum68 快照 JSON 数字/字符串 → float64（千分位与空白先清掉）。
func hrNum68(v any) float64 {
	s := strings.NewReplacer(",", "", "，", "", " ", "").Replace(toStringValue(v))
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// hrSIBillItemsFromSnapshot68 快照明细 → 结构体（复用既有的排序 / 部门汇总逻辑）。
func hrSIBillItemsFromSnapshot68(snap map[string]any) []HRSIBillItem {
	rows := hrSIBillSnapshotItems68(snap)
	out := make([]HRSIBillItem, 0, len(rows))
	num := func(v any) float64 {
		switch x := v.(type) {
		case float64:
			return x
		case float32:
			return float64(x)
		case int:
			return float64(x)
		case int64:
			return float64(x)
		case string:
			return hrNum68(x)
		}
		return 0
	}
	for _, m := range rows {
		it := HRSIBillItem{
			ID: toStringValue(m["id"]), Sequence: toStringValue(m["序号"]),
			Name: toStringValue(m["姓名"]), Department: toStringValue(m["部门"]),
			IDNumber: toStringValue(m["证件号码"]), PersonCode: toStringValue(m["个人编号"]),
			Base:          num(m["基数"]),
			PPension:      num(m["养老个人"]),
			PMedical:      num(m["医疗个人"]),
			PBig:          num(m["大额医疗"]),
			PUnemployment: num(m["失业个人"]),
			PTotal:        num(m["个人小计"]),
			UPension:      num(m["养老单位"]),
			UMedical:      num(m["医疗+生育单位"]),
			UInjury:       num(m["工伤单位"]),
			UUnemployment: num(m["失业单位"]),
			UTotal:        num(m["单位小计"]),
		}
		if b, ok := m["是否补缴"].(bool); ok && b {
			it.IsAdjustment = true
		}
		out = append(out, it)
	}
	return out
}

// hrSIBillSnapshotSummary68 从快照读取汇总行。
func hrSIBillSnapshotSummary68(snap map[string]any) map[string]any {
	if snap == nil {
		return nil
	}
	if m, ok := snap["summary"].(map[string]any); ok && len(m) > 0 {
		return m
	}
	return nil
}

// hrSIBillSnapshotDept68 从快照读取部门汇总。
func hrSIBillSnapshotDept68(snap map[string]any) []map[string]any {
	if snap == nil {
		return nil
	}
	switch v := snap["dept_summary"].(type) {
	case []map[string]any:
		return v
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, it := range v {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// hrSIMarkBillDirty68 该月记录发生变化 → 把已生成的账单标记为「原始数据已变更」。
//
// 返回 true 表示确实存在该月账单（前端据此弹提醒）。
func (h *HRHandler) hrSIMarkBillDirty68(ctx context.Context, tid int64, month string) bool {
	month = strings.TrimSpace(month)
	if month == "" {
		return false
	}
	res := h.db.WithContext(ctx).Model(&HRSIBill{}).
		Where("tenant_id = ? AND month_label = ?", tid, month).
		Updates(map[string]any{"source_dirty": true, "updated_at": time.Now()})
	return res.Error == nil && res.RowsAffected > 0
}

// hrSIBillDirtyPayload68 统一回传格式：前端只认这两个键。
func hrSIBillDirtyPayload68(dirty bool, month string) map[string]any {
	return map[string]any{"bill_dirty_warning": dirty, "dirty_month": month}
}

// hrBackfillSIBillSnapshots68 存量账单补快照（幂等）。
//
// 老账单本身已有 `hr_si_bill_items` 明细快照，这里把「汇总行 + 明细 + 部门汇总」
// 序列化进 detail_snapshot，让读取侧从此只有一条路径（快照）。
// 明细为空的账单无法凭空造快照，跳过并留日志。
func (h *HRHandler) hrBackfillSIBillSnapshots68(ctx context.Context) error {
	var bills []HRSIBill
	if err := h.db.WithContext(ctx).Where("detail_snapshot IS NULL").Find(&bills).Error; err != nil {
		return err
	}
	if len(bills) == 0 {
		return nil
	}
	for i := range bills {
		var items []HRSIBillItem
		if err := h.db.WithContext(ctx).Where("bill_id = ?", bills[i].ID).Find(&items).Error; err != nil {
			continue
		}
		if len(items) == 0 {
			logger.Warnf(ctx, "账单 %s(%s) 无明细，跳过快照回填", bills[i].ID, bills[i].MonthLabel)
			continue
		}
		bills[i].DetailSnapshot = hrSIBuildSnapshot68(bills[i].MonthLabel, bills[i], items)
		if bills[i].GeneratedAt == nil {
			t := bills[i].CreatedAt
			bills[i].GeneratedAt = &t
		}
		// 结构体 Save：map[string]any + serializer:json 才能写进 jsonb。
		if err := h.db.WithContext(ctx).Save(&bills[i]).Error; err != nil {
			logger.Warnf(ctx, "账单 %s 快照回填失败: %v", bills[i].ID, err)
		}
	}
	return nil
}

// hrEnsureSIRecordIndex68 唯一索引兜底：同一 (月, 类型, 险种, 人) 只允许一行。
//
// 存量数据可能有重复，索引建不上只告警（代码层的 WHERE 已强制 bill_month）。
func (h *HRHandler) hrEnsureSIRecordIndex68(ctx context.Context) error {
	err := h.db.WithContext(ctx).Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_hr_si_records_month_person_scheme
		ON hr_si_records (tenant_id, bill_month, record_type, scheme, id_number)
	`).Error
	if err != nil {
		logger.Warnf(ctx, "社保记录按月唯一索引创建失败（存量可能有重复，不阻断）: %v", err)
	}
	return nil
}

// hrRepair68 第 68 轮的三项幂等修复：离职字段全集 / 账单快照回填 / 按月唯一索引。
//
// 必须对**已在用的租户**也执行（seeded=true 分支）——它们只补缺，不覆盖用户配置。
func (h *HRHandler) hrRepair68(ctx context.Context) {
	if err := h.hrMigrateResignFull68(ctx); err != nil {
		logger.Warnf(ctx, "补齐离职人员字段失败: %v", err)
	}
	if err := h.hrBackfillSIBillSnapshots68(ctx); err != nil {
		logger.Warnf(ctx, "回填社保账单快照失败: %v", err)
	}
	_ = h.hrEnsureSIRecordIndex68(ctx)
}
