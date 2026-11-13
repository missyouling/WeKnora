// 第 56 轮：社保管理对标公积金架构全面重构（后端）。
//
// 四个独立列表视图各有一张表 / 一组胶囊字段：
//
//	回盘表   hr_si_receipts    ← 《人员增减申请回盘表.xlsx》242 条
//	正常记录 hr_si_records     ← 2026.11/正常社保 9 个文件（375 条聚合）
//	异常记录 hr_si_records     ← 2026.11/异常社保 5 个文件（3 条聚合）
//	月度账单 hr_si_bills + hr_si_bill_items ← 《2026.11社保扣款明细.xlsx》
//
// 铁律：本文件只新增，不改上游核心代码；个人社保账号 100% 来自回盘表「个人编号」，
// 按身份证号回写花名册，绝不人工录入。金额口径沿用 Excel：
// 分项逐行 round2 相加用于列表，整表未取整累加再 round2 用于账单主数字。
package handler

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// ---------------------------------------------------------------------------
// 模型（4 张新表）
// ---------------------------------------------------------------------------

// HRSIReceipt 回盘表：源表 27 个业务字段全量存 custom_data（中文键），
// 主列只放关联用的唯一键与状态，便于筛选与跨表回写。
type HRSIReceipt struct {
	ID            string         `gorm:"primaryKey" json:"id"`
	TenantID      int64          `gorm:"index" json:"tenant_id"`
	Sequence      string         `json:"sequence"`
	PersonCode    string         `gorm:"index" json:"person_code"`
	IDNumber      string         `gorm:"index" json:"id_number"`
	Name          string         `json:"name"`
	Department    string         `json:"department"`
	PensionStatus string         `json:"pension_status"`
	CustomData    map[string]any `gorm:"type:jsonb;serializer:json" json:"custom_data"`
	Source        string         `json:"source"`
	CreatedAt     time.Time      `json:"created_at"`
}

func (HRSIReceipt) TableName() string { return "hr_si_receipts" }

// HRSIRecord 正常 / 异常记录：一行 = (人, 险种, 账单月)。
type HRSIRecord struct {
	ID             string         `gorm:"primaryKey" json:"id"`
	TenantID       int64          `gorm:"index" json:"tenant_id"`
	BillMonth      string         `gorm:"index" json:"bill_month"`
	Period         string         `json:"period"`
	RecordType     string         `gorm:"index" json:"record_type"` // normal / abnormal
	PersonCode     string         `json:"person_code"`
	Name           string         `json:"name"`
	IDNumber       string         `gorm:"index" json:"id_number"`
	Department     string         `json:"department"`
	Scheme         string         `gorm:"index" json:"scheme"`
	Base           float64        `json:"base"`
	UnitRate       string         `json:"unit_rate"`
	UnitAmount     float64        `json:"unit_amount"`
	PersonalRate   string         `json:"personal_rate"`
	PersonalAmount float64        `json:"personal_amount"`
	TotalAmount    float64        `json:"total_amount"`
	AdjustType     string         `json:"adjust_type"`
	SourceFile     string         `json:"source_file"`
	CustomData     map[string]any `gorm:"type:jsonb;serializer:json" json:"custom_data"`
	Source         string         `json:"source"`
	CreatedAt      time.Time      `json:"created_at"`
}

func (HRSIRecord) TableName() string { return "hr_si_records" }

// HRSIBill 月度账单（每月仅 1 条，覆盖需二次确认）。
type HRSIBill struct {
	ID                   string    `gorm:"primaryKey" json:"id"`
	TenantID             int64     `gorm:"index" json:"tenant_id"`
	MonthLabel           string    `gorm:"index" json:"month_label"`
	BillStatus           string    `json:"bill_status"`
	Headcount            int       `json:"headcount"`
	BaseTotal            float64   `json:"base_total"`
	UnitPension          float64   `json:"unit_pension"`
	UnitMedical          float64   `json:"unit_medical"`
	UnitInjury           float64   `json:"unit_injury"`
	UnitUnemployment     float64   `json:"unit_unemployment"`
	UnitTotal            float64   `json:"unit_total"`
	PersonalPension      float64   `json:"personal_pension"`
	PersonalMedical      float64   `json:"personal_medical"`
	PersonalBig          float64   `json:"personal_big"`
	PersonalUnemployment float64   `json:"personal_unemployment"`
	PersonalTotal        float64   `json:"personal_total"`
	GrandTotal           float64   `json:"grand_total"`
	Source               string    `json:"source"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (HRSIBill) TableName() string { return "hr_si_bills" }

// HRSIBillItem 扣款明细快照：75 名员工 + 陈刚补缴行（补缴行不计人数）。
type HRSIBillItem struct {
	ID            string    `gorm:"primaryKey" json:"id"`
	TenantID      int64     `gorm:"index" json:"tenant_id"`
	BillID        string    `gorm:"index" json:"bill_id"`
	Sequence      string    `json:"sequence"`
	Name          string    `json:"name"`
	Department    string    `json:"department"`
	IDNumber      string    `json:"id_number"`
	PersonCode    string    `json:"person_code"`
	Base          float64   `json:"base"`
	PPension      float64   `json:"p_pension"`
	PMedical      float64   `json:"p_medical"`
	PBig          float64   `json:"p_big"`
	PUnemployment float64   `json:"p_unemployment"`
	PTotal        float64   `json:"p_total"`
	UPension      float64   `json:"u_pension"`
	UMedical      float64   `json:"u_medical"`
	UInjury       float64   `json:"u_injury"`
	UUnemployment float64   `json:"u_unemployment"`
	UTotal        float64   `json:"u_total"`
	IsAdjustment  bool      `json:"is_adjustment"`
	Source        string    `json:"source"`
	CreatedAt     time.Time `json:"created_at"`
}

func (HRSIBillItem) TableName() string { return "hr_si_bill_items" }

// ---------------------------------------------------------------------------
// 响应视图：把英文主列翻成中文字段名（列表 / 抽屉 / 六同步共用一个出口）
// ---------------------------------------------------------------------------

func hrSIReceiptView(r HRSIReceipt) map[string]any {
	out := map[string]any{
		"id": r.ID, "sequence": r.Sequence, "person_code": r.PersonCode,
		"id_number": r.IDNumber, "name": r.Name, "department": r.Department,
		"序号": r.Sequence, "个人编号": r.PersonCode, "身份证号": r.IDNumber,
		"姓名": r.Name, "部门": r.Department, "养老参保状态": r.PensionStatus,
		"custom_data": r.CustomData,
	}
	for k, v := range r.CustomData {
		out[k] = v
	}
	return out
}

func hrSIRecordView(r HRSIRecord) map[string]any {
	out := map[string]any{
		"id": r.ID, "bill_month": r.BillMonth, "record_type": r.RecordType,
		"id_number": r.IDNumber, "person_code": r.PersonCode, "scheme": r.Scheme,
		"账单月份": r.BillMonth, "费款所属期": r.Period,
		"个人编号": r.PersonCode, "姓名": r.Name, "身份证号": r.IDNumber,
		"部门": r.Department, "险种类型": r.Scheme, "缴费基数": r.Base,
		"单位缴费比例": r.UnitRate, "单位应缴金额": r.UnitAmount,
		"个人缴费比例": r.PersonalRate, "个人应缴金额": r.PersonalAmount,
		"应缴合计": r.TotalAmount, "补退类型": r.AdjustType,
		"记录类型": map[string]string{"normal": "正常", "abnormal": "异常"}[r.RecordType],
		"源文件":  r.SourceFile, "custom_data": r.CustomData,
	}
	return out
}

func hrSIBillView(b HRSIBill) map[string]any {
	return map[string]any{
		"id": b.ID, "month_label": b.MonthLabel, "账单周期": b.MonthLabel,
		"headcount": b.Headcount, "参保人数": b.Headcount,
		"缴费基数合计": b.BaseTotal,
		"单位养老":   b.UnitPension, "单位医疗+生育": b.UnitMedical,
		"单位工伤": b.UnitInjury, "单位失业": b.UnitUnemployment,
		"社保单位金额": b.UnitTotal,
		"个人养老":   b.PersonalPension, "个人医疗": b.PersonalMedical,
		"个人大额医疗": b.PersonalBig, "个人失业": b.PersonalUnemployment,
		"社保个人金额": b.PersonalTotal, "应缴总计": b.GrandTotal,
		"账单状态": b.BillStatus, "bill_status": b.BillStatus,
		"生成时间": b.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

func hrSIBillItemView(it HRSIBillItem) map[string]any {
	seq := it.Sequence
	if it.IsAdjustment {
		seq = "补缴"
	}
	return map[string]any{
		"id": it.ID, "序号": seq, "姓名": it.Name, "部门": it.Department,
		"证件号码": it.IDNumber, "个人编号": it.PersonCode,
		"基数": it.Base, "缴费基数": it.Base,
		"养老个人": it.PPension, "医疗个人": it.PMedical,
		"大额医疗": it.PBig, "失业个人": it.PUnemployment, "个人小计": it.PTotal,
		"养老单位": it.UPension, "医疗+生育单位": it.UMedical,
		"工伤单位": it.UInjury, "失业单位": it.UUnemployment, "单位小计": it.UTotal,
		"是否补缴": it.IsAdjustment,
	}
}

// hrSIDeptLabel 部门汇总专用展示名（对齐《2026.11社保扣款明细.xlsx》图 2 口径）。
func hrSIDeptLabel(dept string) string {
	switch strings.TrimSpace(dept) {
	case "财务":
		return "财务部"
	case "机电":
		return "机电部"
	case "生产管理":
		return "生管"
	default:
		return strings.TrimSpace(dept)
	}
}

// 图 2 部门/工序固定顺序；未列出的部门按名称排在其后。
var hrSIDeptOrder = []string{
	"财务部", "行政部", "销售部", "机电部", "库房", "过磅", "质检部",
	"生管", "大拉", "中拉", "小拉", "管退", "束绞", "多头",
}

func hrSIDeptRank(d string) int {
	for i, v := range hrSIDeptOrder {
		if v == d {
			return i
		}
	}
	return len(hrSIDeptOrder)
}

// hrSIDeptSummaryOf 按部门汇总人数 / 个人 / 单位 / 合计。
// 补缴行金额并入所在部门，但不增加人数（总计恒为 75 人）。
func hrSIDeptSummaryOf(items []HRSIBillItem) []map[string]any {
	type agg struct {
		dept     string
		count    int
		personal float64
		unit     float64
	}
	order := []string{}
	by := map[string]*agg{}
	for _, it := range items {
		d := strings.TrimSpace(it.Department)
		if d == "" {
			d = "未分配部门"
		}
		label := hrSIDeptLabel(d)
		a := by[label]
		if a == nil {
			a = &agg{dept: label}
			by[label] = a
			order = append(order, label)
		}
		if !it.IsAdjustment {
			a.count++
		}
		a.personal += it.PTotal
		a.unit += it.UTotal
	}
	sort.SliceStable(order, func(i, j int) bool {
		ri, rj := hrSIDeptRank(order[i]), hrSIDeptRank(order[j])
		if ri != rj {
			return ri < rj
		}
		return order[i] < order[j]
	})
	out := make([]map[string]any, 0, len(order))
	for i, k := range order {
		a := by[k]
		out = append(out, map[string]any{
			"序号": i + 1, "部门": k, "人数": a.count,
			"社保个人金额": round2(a.personal), "社保单位金额": round2(a.unit),
			"合计": round2(a.personal + a.unit),
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// 字段配置：四胶囊强制重建（同 hrMigrateFundSubs，保留用户 src_ 引用字段）
// ---------------------------------------------------------------------------

func (h *HRHandler) hrMigrateSocialSubs(ctx context.Context) error {
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("scope = ? AND name = ? AND deleted_at IS NULL", hrScopeSocial, "社保管理").
		Find(&cats).Error; err != nil {
		return err
	}
	want := hrSocialSeedSubs()
	for i := range cats {
		next := append([]types.FleetCategorySub{}, want...)
		for _, sub := range cats[i].Subs {
			if strings.HasPrefix(strings.TrimSpace(sub.Name), "src_") {
				next = append(next, sub)
			}
		}
		if hrFundSubsEqual(cats[i].Subs, next) {
			continue
		}
		cats[i].Subs = next
		cats[i].UpdatedAt = time.Now()
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			return err
		}
		logger.Infof(ctx, "社保字段配置已迁移到四胶囊 76 字段（tenant=%d）", cats[i].TenantID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 真实数据 seed（幂等，失败只告警不阻断启动）
// ---------------------------------------------------------------------------

// hrSINormalizeDate 把源表 8 位日期 19701115 格式化为 1970-11-15。
func hrSINormalizeDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) != 8 {
		return s
	}
	if _, err := strconv.Atoi(s); err != nil {
		return s
	}
	return s[:4] + "-" + s[4:6] + "-" + s[6:8]
}

var hrSIDateKeys = map[string]bool{
	"出生日期": true, "工作日期": true,
	"养老参保时间": true, "失业参保时间": true, "医疗参保时间": true,
	"工伤参保时间": true, "生育参保时间": true,
}

func hrSIReceiptRowToModel(tid int64, header []string, vals []string, deptByID map[string]string) HRSIReceipt {
	data := map[string]any{}
	for i, h := range header {
		v := ""
		if i < len(vals) {
			v = strings.TrimSpace(vals[i])
		}
		if hrSIDateKeys[h] {
			v = hrSINormalizeDate(v)
		}
		data[h] = v
	}
	get := func(k string) string {
		v, _ := data[k].(string)
		return strings.TrimSpace(v)
	}
	idno := get("身份证号")
	dept := deptByID[normalizeIDNumber(idno)]
	if dept == "" {
		dept = get("部门")
	}
	if dept != "" {
		data["部门"] = dept
	}
	return HRSIReceipt{
		ID: uuid.New().String(), TenantID: tid,
		Sequence: get("序号"), PersonCode: get("个人编号"), IDNumber: idno,
		Name: get("姓名"), Department: dept, PensionStatus: get("养老参保状态"),
		CustomData: data, Source: "seed",
	}
}

func hrSIRecordRowToModel(tid int64, d hrSIRecordSeed, deptByID map[string]string) HRSIRecord {
	dept := d.Dept
	if dept == "" {
		dept = deptByID[normalizeIDNumber(d.IDNo)]
	}
	kind := "normal"
	adjust := "正常"
	if d.Kind == "异常" {
		kind = "abnormal"
		adjust = "补缴"
	}
	data := map[string]any{
		"账单月份": d.BillMonth, "费款所属期": d.Period, "个人编号": d.PersonCode,
		"姓名": d.Name, "身份证号": d.IDNo, "部门": dept, "险种类型": d.Scheme,
		"缴费基数": d.Base, "单位缴费比例": d.UnitRate, "单位应缴金额": d.UnitAmount,
		"个人缴费比例": d.PersonalRate, "个人应缴金额": d.PersonalAmount,
		"应缴合计": d.Total, "补退类型": adjust,
		"记录类型": d.Kind, "源文件": "",
	}
	return HRSIRecord{
		ID: uuid.New().String(), TenantID: tid,
		BillMonth: d.BillMonth, Period: d.Period, RecordType: kind,
		PersonCode: d.PersonCode, Name: d.Name, IDNumber: d.IDNo,
		Department: dept, Scheme: d.Scheme, Base: d.Base,
		UnitRate: d.UnitRate, UnitAmount: d.UnitAmount,
		PersonalRate: d.PersonalRate, PersonalAmount: d.PersonalAmount,
		TotalAmount: d.Total, AdjustType: d.AdjustType,
		CustomData: data, Source: "seed",
	}
}

func hrSIBillItemRowToModel(tid int64, billID string, d hrSIBillItemSeed) HRSIBillItem {
	return HRSIBillItem{
		ID: uuid.New().String(), TenantID: tid, BillID: billID,
		Sequence: d.Seq, Name: d.Name, Department: hrSIDeptLabel(d.Dept),
		IDNumber: d.IDNo, PersonCode: d.PersonCode, Base: d.Base,
		PPension: d.PPension, PMedical: d.PMedical, PBig: d.PBig,
		PUnemployment: d.PUnemployment, PTotal: d.PTotal,
		UPension: d.UPension, UMedical: d.UMedical, UInjury: d.UInjury,
		UUnemployment: d.UUnemployment, UTotal: d.UTotal,
		IsAdjustment: d.IsAdjustment, Source: "seed",
	}
}

func hrSIBillOfSeed(tid int64, s hrSIBillSeed) HRSIBill {
	return HRSIBill{
		ID: uuid.New().String(), TenantID: tid,
		MonthLabel: s.MonthLabel, BillStatus: s.BillStatus, Headcount: s.Headcount,
		BaseTotal:   s.BaseTotal,
		UnitPension: s.UnitPension, UnitMedical: s.UnitMedical,
		UnitInjury: s.UnitInjury, UnitUnemployment: s.UnitUnemployment, UnitTotal: s.UnitTotal,
		PersonalPension: s.PersonalPension, PersonalMedical: s.PersonalMedical,
		PersonalBig: s.PersonalBig, PersonalUnemployment: s.PersonalUnemployment,
		PersonalTotal: s.PersonalTotal, GrandTotal: s.GrandTotal, Source: "seed",
	}
}

// hrSeedSocialRound56 灌入四视图真实数据 + 清理第 47 轮旧账期/扣款脏数据。
func (h *HRHandler) hrSeedSocialRound56(ctx context.Context) error {
	for _, tid := range hrSeedTenantIDs(ctx, h.db) {
		if err := h.hrCleanLegacySocial(ctx, tid); err != nil {
			logger.Warnf(ctx, "clean legacy social data failed (tenant=%d): %v", tid, err)
		}
		deptByID := h.hrEmployeeDeptMap(ctx, tid)
		// 回盘表 242 条（表空才灌，用户后续导入的数据不会被启动覆盖）
		var receiptCount int64
		if err := h.db.WithContext(ctx).Model(&HRSIReceipt{}).Where("tenant_id = ?", tid).Count(&receiptCount).Error; err != nil {
			return err
		}
		if receiptCount == 0 {
			receipts := make([]HRSIReceipt, 0, len(hrSIReceiptSeedRows))
			for _, row := range hrSIReceiptSeedRows {
				receipts = append(receipts, hrSIReceiptRowToModel(tid, hrSIReceiptSeedHeader, row, deptByID))
			}
			if err := h.db.WithContext(ctx).CreateInBatches(receipts, 50).Error; err != nil {
				return err
			}
		}
		// 正常 / 异常记录 378 条
		var recordCount int64
		if err := h.db.WithContext(ctx).Model(&HRSIRecord{}).Where("tenant_id = ?", tid).Count(&recordCount).Error; err != nil {
			return err
		}
		if recordCount == 0 {
			records := make([]HRSIRecord, 0, len(hrSIRecordSeedRows))
			for _, d := range hrSIRecordSeedRows {
				records = append(records, hrSIRecordRowToModel(tid, d, deptByID))
			}
			if err := h.db.WithContext(ctx).CreateInBatches(records, 50).Error; err != nil {
				return err
			}
		}
		// 2026-11 月度账单 + 76 条扣款明细
		var billCount int64
		if err := h.db.WithContext(ctx).Model(&HRSIBill{}).
			Where("tenant_id = ? AND month_label = ?", tid, hrSIBillSeedValue.MonthLabel).
			Count(&billCount).Error; err != nil {
			return err
		}
		if billCount == 0 {
			bill := hrSIBillOfSeed(tid, hrSIBillSeedValue)
			if err := h.db.WithContext(ctx).Create(&bill).Error; err != nil {
				return err
			}
			items := make([]HRSIBillItem, 0, len(hrSIBillItemSeedRows))
			for _, d := range hrSIBillItemSeedRows {
				items = append(items, hrSIBillItemRowToModel(tid, bill.ID, d))
			}
			if err := h.db.WithContext(ctx).CreateInBatches(items, 50).Error; err != nil {
				return err
			}
		} else {
			var bill HRSIBill
			if err := h.db.WithContext(ctx).
				Where("tenant_id = ? AND month_label = ?", tid, hrSIBillSeedValue.MonthLabel).
				First(&bill).Error; err == nil {
				var n int64
				_ = h.db.WithContext(ctx).Model(&HRSIBillItem{}).Where("bill_id = ?", bill.ID).Count(&n).Error
				if n == 0 {
					items := make([]HRSIBillItem, 0, len(hrSIBillItemSeedRows))
					for _, d := range hrSIBillItemSeedRows {
						items = append(items, hrSIBillItemRowToModel(tid, bill.ID, d))
					}
					_ = h.db.WithContext(ctx).CreateInBatches(items, 50).Error
				}
			}
		}
		h.hrSyncSocialAccountFromReceipts(ctx, tid)
	}
	return nil
}

// hrCleanLegacySocial 清空第 47 轮的旧账期 / 原始明细 / 扣款脏数据。
// 增减员（hr_si_changes）是独立业务台账，不在清理范围内。
func (h *HRHandler) hrCleanLegacySocial(ctx context.Context, tid int64) error {
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).Delete(&HRSIRawRecord{}).Error; err != nil {
		return err
	}
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).Delete(&HRSICharge{}).Error; err != nil {
		return err
	}
	return h.db.WithContext(ctx).
		Where("tenant_id = ? AND year_month <> ?", tid, hrSIBillSeedValue.MonthLabel).
		Delete(&HRSIPeriod{}).Error
}

// hrSIReceiptByID 按身份证取主记录。
//
// 真实回盘表里同一人可能有两条记录（例：刘正均序号 1「参保缴费」与序号 168
// 状态空白的历史行），242 条都要在列表里完整展示，但花名册社保账号与联动
// 预警必须取有效参保状态那条，不能被空白历史行覆盖。
func hrSIReceiptByID(rows []HRSIReceipt) map[string]HRSIReceipt {
	out := map[string]HRSIReceipt{}
	for _, r := range rows {
		id := normalizeIDNumber(r.IDNumber)
		if id == "" {
			continue
		}
		cur, ok := out[id]
		if !ok || (strings.TrimSpace(cur.PensionStatus) == "" && strings.TrimSpace(r.PensionStatus) != "") {
			out[id] = r
		}
	}
	return out
}

// hrSyncSocialAccountFromReceipts 花名册「个人社保账号」唯一权威回写：
// 回盘表「个人编号」按身份证号匹配，强制覆盖主列 + custom_data 中文键。
func (h *HRHandler) hrSyncSocialAccountFromReceipts(ctx context.Context, tid int64) int {
	var receipts []HRSIReceipt
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND person_code <> '' AND id_number <> ''", tid).
		Find(&receipts).Error; err != nil {
		return 0
	}
	codeByID := map[string]string{}
	for id, r := range hrSIReceiptByID(receipts) {
		codeByID[id] = r.PersonCode
	}
	var employees []HREmployee
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&employees).Error; err != nil {
		return 0
	}
	changed := 0
	for i := range employees {
		code := codeByID[normalizeIDNumber(employees[i].IDNumber)]
		if code == "" || employees[i].SocialInsuranceNumber == code {
			continue
		}
		employees[i].SocialInsuranceNumber = code
		if employees[i].CustomData == nil {
			employees[i].CustomData = map[string]any{}
		}
		employees[i].CustomData["个人社保账号"] = code
		if err := h.db.WithContext(ctx).Save(&employees[i]).Error; err == nil {
			changed++
		}
	}
	return changed
}

// ---------------------------------------------------------------------------
// 全员联动预警（待补录社保 / 离职待减员）
// ---------------------------------------------------------------------------

type hrSocialAlertCounts struct {
	pending int
	toSeal  int
}

func (h *HRHandler) hrSocialAlerts(ctx context.Context, tid int64) hrSocialAlertCounts {
	var out hrSocialAlertCounts
	var employees []HREmployee
	if err := h.db.WithContext(ctx).
		Select("id, name, status, id_number, social_insurance_number").
		Where("tenant_id = ?", tid).Find(&employees).Error; err != nil {
		return out
	}
	var receipts []HRSIReceipt
	_ = h.db.WithContext(ctx).Select("id_number, person_code, pension_status").
		Where("tenant_id = ?", tid).Find(&receipts).Error
	byID := hrSIReceiptByID(receipts)
	for _, e := range employees {
		r, ok := byID[normalizeIDNumber(e.IDNumber)]
		if normalizeEmployeeStatus(e.Status) == "active" {
			// 在职但回盘表没有参保记录 / 没带出个人编号 → 待补录（待增员）
			if !ok || strings.TrimSpace(e.SocialInsuranceNumber) == "" || strings.TrimSpace(r.PersonCode) == "" {
				out.pending++
			}
			continue
		}
		// 已离职但回盘表仍「参保缴费」，或压根没有减员回盘记录 → 待减员
		if !ok || strings.TrimSpace(r.PensionStatus) == "参保缴费" {
			out.toSeal++
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// 列表 API
// ---------------------------------------------------------------------------

func (h *HRHandler) ListSIReceipts(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var items []HRSIReceipt
	q := h.db.WithContext(ctx).Model(&HRSIReceipt{}).Where("tenant_id = ?", tid)
	if kw := strings.TrimSpace(c.Query("q")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("name LIKE ? OR person_code LIKE ? OR id_number LIKE ?", like, like, like)
	}
	if dept := strings.TrimSpace(c.Query("department")); dept != "" {
		q = q.Where("department = ?", dept)
	}
	if err := q.Order("id ASC").Find(&items).Error; err != nil {
		hrFail(c, "查询回盘表失败: "+err.Error())
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		return hrSISeqNum(items[i].Sequence) < hrSISeqNum(items[j].Sequence)
	})
	out := make([]map[string]any, 0, len(items))
	depts := map[string]bool{}
	for _, r := range items {
		out = append(out, hrSIReceiptView(r))
		if r.Department != "" {
			depts[r.Department] = true
		}
	}
	hrOK(c, gin.H{"items": out, "total": len(out), "departments": sortedKeys(depts)})
}

func (h *HRHandler) ListSIRecords(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	kind := strings.TrimSpace(c.Query("record_type"))
	if kind != "abnormal" {
		kind = "normal"
	}
	q := h.db.WithContext(ctx).Model(&HRSIRecord{}).Where("tenant_id = ? AND record_type = ?", tid, kind)
	if month := strings.TrimSpace(c.Query("month")); month != "" {
		q = q.Where("bill_month = ?", month)
	}
	if dept := strings.TrimSpace(c.Query("department")); dept != "" {
		q = q.Where("department = ?", dept)
	}
	if scheme := strings.TrimSpace(c.Query("scheme")); scheme != "" && scheme != "全部险种" && scheme != "全部" {
		q = q.Where("scheme = ?", scheme)
	}
	if kw := strings.TrimSpace(c.Query("q")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("name LIKE ? OR person_code LIKE ? OR id_number LIKE ?", like, like, like)
	}
	var items []HRSIRecord
	if err := q.Order("name ASC, scheme ASC, id ASC").Find(&items).Error; err != nil {
		hrFail(c, "查询社保记录失败: "+err.Error())
		return
	}
	out := make([]map[string]any, 0, len(items))
	depts := map[string]bool{}
	for _, r := range items {
		out = append(out, hrSIRecordView(r))
		if r.Department != "" {
			depts[r.Department] = true
		}
	}
	hrOK(c, gin.H{"items": out, "total": len(out), "departments": sortedKeys(depts)})
}

func (h *HRHandler) ListSIBills(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var items []HRSIBill
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).
		Order("month_label DESC, id ASC").Find(&items).Error; err != nil {
		hrFail(c, "查询社保月度账单失败: "+err.Error())
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, b := range items {
		out = append(out, hrSIBillView(b))
	}
	hrOK(c, gin.H{"items": out, "total": len(out)})
}

func (h *HRHandler) GetSIBill(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	id := c.Param("id")
	var bill HRSIBill
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tid, id).First(&bill).Error; err != nil {
		hrFail(c, "账单不存在")
		return
	}
	var items []HRSIBillItem
	if err := h.db.WithContext(ctx).Where("bill_id = ?", bill.ID).
		Order("id ASC").Find(&items).Error; err != nil {
		hrFail(c, "查询扣款明细失败: "+err.Error())
		return
	}
	// 75 条正常行按序号升序，陈刚补缴行固定在最后。
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].IsAdjustment != items[j].IsAdjustment {
			return !items[i].IsAdjustment
		}
		return hrSISeqNum(items[i].Sequence) < hrSISeqNum(items[j].Sequence)
	})
	out := make([]map[string]any, 0, len(items))
	depts := map[string]bool{}
	for _, it := range items {
		out = append(out, hrSIBillItemView(it))
		depts[it.Department] = true
	}
	hrOK(c, gin.H{
		"bill": hrSIBillView(bill), "items": out,
		"departments": sortedKeys(depts), "dept_summary": hrSIDeptSummaryOf(items),
	})
}

// ---------------------------------------------------------------------------
// 生成账单（同月只能一条；覆盖需前端二次确认后 overwrite=true）
// ---------------------------------------------------------------------------

// hrSIBillTotalsOf 汇总口径：2026-11 以《2026.11社保扣款明细.xlsx》为准；
// 其它月份按当月正常 + 异常记录逐项累加后 round2。
func (h *HRHandler) hrSIBillTotalsOf(ctx context.Context, tid int64, month string) HRSIBill {
	if month == hrSIBillSeedValue.MonthLabel {
		return hrSIBillOfSeed(tid, hrSIBillSeedValue)
	}
	var records []HRSIRecord
	_ = h.db.WithContext(ctx).Where("tenant_id = ? AND bill_month = ?", tid, month).Find(&records).Error
	people := map[string]bool{}
	b := HRSIBill{TenantID: tid, MonthLabel: month, BillStatus: "已申报", Source: "generated"}
	for _, r := range records {
		people[normalizeIDNumber(r.IDNumber)] = true
		b.BaseTotal += r.Base
		switch r.Scheme {
		case "养老保险":
			b.UnitPension += r.UnitAmount
			b.PersonalPension += r.PersonalAmount
		case "医疗保险", "生育保险", "大额医疗":
			b.UnitMedical += r.UnitAmount
			if r.Scheme == "大额医疗" {
				b.PersonalBig += r.PersonalAmount
			} else {
				b.PersonalMedical += r.PersonalAmount
			}
		case "工伤保险":
			b.UnitInjury += r.UnitAmount
		case "失业保险":
			b.UnitUnemployment += r.UnitAmount
			b.PersonalUnemployment += r.PersonalAmount
		}
	}
	b.Headcount = len(people)
	b.UnitPension, b.UnitMedical = round2(b.UnitPension), round2(b.UnitMedical)
	b.UnitInjury, b.UnitUnemployment = round2(b.UnitInjury), round2(b.UnitUnemployment)
	b.PersonalPension, b.PersonalMedical = round2(b.PersonalPension), round2(b.PersonalMedical)
	b.PersonalBig, b.PersonalUnemployment = round2(b.PersonalBig), round2(b.PersonalUnemployment)
	b.UnitTotal = round2(b.UnitPension + b.UnitMedical + b.UnitInjury + b.UnitUnemployment)
	b.PersonalTotal = round2(b.PersonalPension + b.PersonalMedical + b.PersonalBig + b.PersonalUnemployment)
	b.GrandTotal = round2(b.UnitTotal + b.PersonalTotal)
	b.BaseTotal = round2(b.BaseTotal)
	return b
}

func (h *HRHandler) CreateSIBill(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req struct {
		MonthLabel string `json:"month_label"`
		Month      string `json:"month"`
		Overwrite  bool   `json:"overwrite"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	month := strings.TrimSpace(req.MonthLabel)
	if month == "" {
		month = strings.TrimSpace(req.Month)
	}
	if len(month) != 7 || month[4] != '-' {
		hrFail(c, "账单月份必须为 YYYY-MM")
		return
	}
	var existing []HRSIBill
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND month_label = ?", tid, month).Find(&existing).Error; err != nil {
		hrFail(c, "检查账单失败: "+err.Error())
		return
	}
	if len(existing) > 0 && !req.Overwrite {
		hrFail(c, fmt.Sprintf("「%s」已存在社保月度账单，是否覆盖？", month))
		return
	}
	next := h.hrSIBillTotalsOf(ctx, tid, month)
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, old := range existing {
			if e := tx.Where("bill_id = ?", old.ID).Delete(&HRSIBillItem{}).Error; e != nil {
				return e
			}
			if e := tx.Delete(&old).Error; e != nil {
				return e
			}
		}
		next.ID = uuid.New().String()
		if e := tx.Create(&next).Error; e != nil {
			return e
		}
		// 明细：当月已有扣款明细则沿用，否则从正常/异常记录按人聚合生成
		items := h.hrSIBillItemsForMonth(ctx, tid, month, next.ID)
		if len(items) > 0 {
			return tx.CreateInBatches(items, 50).Error
		}
		return nil
	})
	if err != nil {
		hrFail(c, "生成账单失败: "+err.Error())
		return
	}
	hrOK(c, hrSIBillView(next))
}

// hrSIBillItemsForMonth 生成账单明细快照：2026-11 直接复用真实扣款明细；
// 其它月份按 (人, 正常/补缴) 从 hr_si_records 聚合。
func (h *HRHandler) hrSIBillItemsForMonth(ctx context.Context, tid int64, month, billID string) []HRSIBillItem {
	if month == hrSIBillSeedValue.MonthLabel {
		out := make([]HRSIBillItem, 0, len(hrSIBillItemSeedRows))
		for _, d := range hrSIBillItemSeedRows {
			it := hrSIBillItemRowToModel(tid, billID, d)
			it.Source = "generated"
			out = append(out, it)
		}
		return out
	}
	var records []HRSIRecord
	_ = h.db.WithContext(ctx).Where("tenant_id = ? AND bill_month = ?", tid, month).Find(&records).Error
	type key struct{ id, kind string }
	by := map[key]*HRSIBillItem{}
	order := []key{}
	for _, r := range records {
		k := key{normalizeIDNumber(r.IDNumber), r.RecordType}
		it := by[k]
		if it == nil {
			it = &HRSIBillItem{
				ID: uuid.New().String(), TenantID: tid, BillID: billID,
				Name: r.Name, Department: r.Department, IDNumber: r.IDNumber,
				PersonCode: r.PersonCode, Base: r.Base,
				IsAdjustment: r.RecordType == "abnormal", Source: "generated",
			}
			by[k] = it
			order = append(order, k)
		}
		switch r.Scheme {
		case "养老保险":
			it.UPension += r.UnitAmount
			it.PPension += r.PersonalAmount
		case "医疗保险", "生育保险", "大额医疗":
			it.UMedical += r.UnitAmount
			if r.Scheme == "大额医疗" {
				it.PBig += r.PersonalAmount
			} else {
				it.PMedical += r.PersonalAmount
			}
		case "工伤保险":
			it.UInjury += r.UnitAmount
		case "失业保险":
			it.UUnemployment += r.UnitAmount
			it.PUnemployment += r.PersonalAmount
		}
	}
	out := make([]HRSIBillItem, 0, len(order))
	for i, k := range order {
		it := by[k]
		it.Sequence = strconv.Itoa(i + 1)
		it.PTotal = round2(it.PPension + it.PMedical + it.PBig + it.PUnemployment)
		it.UTotal = round2(it.UPension + it.UMedical + it.UInjury + it.UUnemployment)
		out = append(out, *it)
	}
	return out
}

func (h *HRHandler) DeleteSIBill(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var bill HRSIBill
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tid, c.Param("id")).
		First(&bill).Error; err != nil {
		hrFail(c, "账单不存在")
		return
	}
	_ = h.db.WithContext(ctx).Where("bill_id = ?", bill.ID).Delete(&HRSIBillItem{}).Error
	if err := h.db.WithContext(ctx).Delete(&bill).Error; err != nil {
		hrFail(c, "删除账单失败: "+err.Error())
		return
	}
	hrOK(c, gin.H{"deleted": true})
}

// ---------------------------------------------------------------------------
// 导入：回盘表 Excel / 正常与异常社保多文件
// ---------------------------------------------------------------------------

func (h *HRHandler) ImportSIReceipts(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	fh, err := c.FormFile("file")
	if err != nil {
		hrFail(c, "缺少 file 参数")
		return
	}
	src, err := fh.Open()
	if err != nil {
		hrFail(c, "打开上传文件失败: "+err.Error())
		return
	}
	defer func() { _ = src.Close() }()
	rows, err := hrReadExcelRows(src)
	if err != nil {
		hrFail(c, "解析回盘表失败: "+err.Error())
		return
	}
	headerIdx := -1
	header := []string{}
	for i, row := range rows {
		joined := strings.Join(row, "|")
		if strings.Contains(joined, "个人编号") && strings.Contains(joined, "身份证号") {
			headerIdx, header = i, row
			break
		}
	}
	if headerIdx < 0 {
		hrFail(c, "未找到回盘表表头（需含「个人编号」「身份证号」）")
		return
	}
	deptByID := h.hrEmployeeDeptMap(ctx, tid)
	items := make([]HRSIReceipt, 0, len(rows))
	for _, row := range rows[headerIdx+1:] {
		if len(row) < 3 || strings.TrimSpace(row[0]) == "" && strings.TrimSpace(row[1]) == "" {
			continue
		}
		m := hrSIReceiptRowToModel(tid, header, row, deptByID)
		if m.PersonCode == "" || m.IDNumber == "" {
			continue
		}
		m.Source = "import"
		items = append(items, m)
	}
	if len(items) == 0 {
		hrFail(c, "回盘表中没有可导入的记录")
		return
	}
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Where("tenant_id = ? AND source = ?", tid, "import").Delete(&HRSIReceipt{}).Error; e != nil {
			return e
		}
		return tx.CreateInBatches(items, 50).Error
	})
	if err != nil {
		hrFail(c, "导入回盘表失败: "+err.Error())
		return
	}
	h.hrSyncSocialAccountFromReceipts(ctx, tid)
	hrOK(c, gin.H{"rows": len(items)})
}

// hrSISchemeOfFile 文件名 → 险种中文名（与 hrSIParseFileName 同口径）。
func hrSISchemeOfFile(name string) string {
	switch {
	case strings.Contains(name, "大额") || strings.Contains(name, "大病"):
		return "大额医疗"
	case strings.Contains(name, "养老"):
		return "养老保险"
	case strings.Contains(name, "医疗") || strings.Contains(name, "生育"):
		return "医疗保险"
	case strings.Contains(name, "失业"):
		return "失业保险"
	case strings.Contains(name, "工伤"):
		return "工伤保险"
	}
	return "其他"
}

func (h *HRHandler) ImportSIRecords(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	form, err := c.MultipartForm()
	if err != nil {
		hrFail(c, "读取上传文件失败: "+err.Error())
		return
	}
	files := form.File["files"]
	if len(files) == 0 {
		hrFail(c, "没有上传任何文件")
		return
	}
	kind := strings.TrimSpace(c.PostForm("record_type"))
	if kind != "abnormal" {
		kind = "normal"
	}
	month := strings.TrimSpace(c.PostForm("month"))
	deptByID := h.hrEmployeeDeptMap(ctx, tid)
	type aggKey struct{ id, scheme string }
	by := map[aggKey]*HRSIRecord{}
	order := []aggKey{}
	parsed := 0
	for _, fh := range files {
		payloads, perr := hrParseSocialUpload(fh)
		if perr != nil || len(payloads) == 0 {
			continue
		}
		meta := hrSIParseFileName(fh.Filename)
		scheme := hrSISchemeOfFile(fh.Filename)
		billMonth := month
		if billMonth == "" {
			billMonth = meta.YearMonth
		}
		period := meta.YearMonth
		for _, p := range payloads {
			k := aggKey{normalizeIDNumber(p.IDNumber), scheme}
			r := by[k]
			if r == nil {
				r = &HRSIRecord{
					ID: uuid.New().String(), TenantID: tid,
					BillMonth: billMonth, Period: period, RecordType: kind,
					PersonCode: p.PersonCode, Name: p.Name, IDNumber: p.IDNumber,
					Department: deptByID[k.id], Scheme: scheme, Base: p.PayBase,
					SourceFile: fh.Filename, Source: "import",
				}
				if kind == "abnormal" {
					r.AdjustType = "补缴"
				} else {
					r.AdjustType = "正常"
				}
				by[k] = r
				order = append(order, k)
			}
			amount := p.AmountAdj
			if amount == 0 {
				amount = p.AmountDue
			}
			if meta.Part == HRPartPersonal {
				r.PersonalRate = p.RateText
				r.PersonalAmount = round2(r.PersonalAmount + amount)
			} else {
				r.UnitRate = p.RateText
				r.UnitAmount = round2(r.UnitAmount + amount)
			}
			r.TotalAmount = round2(r.UnitAmount + r.PersonalAmount)
			parsed++
		}
	}
	if len(order) == 0 {
		hrFail(c, "文件中没有可识别的社保明细")
		return
	}
	items := make([]HRSIRecord, 0, len(order))
	for _, k := range order {
		r := by[k]
		r.CustomData = map[string]any{
			"账单月份": r.BillMonth, "费款所属期": r.Period, "个人编号": r.PersonCode,
			"姓名": r.Name, "身份证号": r.IDNumber, "部门": r.Department,
			"险种类型": r.Scheme, "缴费基数": r.Base,
			"单位缴费比例": r.UnitRate, "单位应缴金额": r.UnitAmount,
			"个人缴费比例": r.PersonalRate, "个人应缴金额": r.PersonalAmount,
			"应缴合计": r.TotalAmount, "补退类型": r.AdjustType,
			"记录类型": map[string]string{"normal": "正常", "abnormal": "异常"}[kind],
			"源文件":  r.SourceFile,
		}
		items = append(items, *r)
	}
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Where("tenant_id = ? AND record_type = ? AND source = ?", tid, kind, "import").
			Delete(&HRSIRecord{}).Error; e != nil {
			return e
		}
		return tx.CreateInBatches(items, 50).Error
	})
	if err != nil {
		hrFail(c, "导入社保记录失败: "+err.Error())
		return
	}
	hrOK(c, gin.H{"rows": len(items), "files": len(files), "parsed": parsed})
}

// hrSISeqNum 序号列在 PG 里是字符串，SQL 层 CAST(... AS INTEGER) 遇到
// 「补缴」这类中文会直接报 22P02；排序一律在 Go 侧做。
func hrSISeqNum(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 1 << 30
	}
	return n
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		if k != "" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
