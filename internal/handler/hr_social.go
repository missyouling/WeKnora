// 社保：账期 CRUD（倒序）、五险原始明细导入（正常覆盖 / 补退累加）、
// validateRequired、buildAggregates 聚合、ProcessAdjustments、增减员 CRUD、扣款查询与导出。
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/logger"
)

// hrCell 安全取可选列：缺列返回空（避免 map 零值 0 误取第一列）。
func hrCell(row []string, indexMap map[string]int, key string) string {
	idx, ok := indexMap[key]
	if !ok {
		return ""
	}
	return getCell(row, idx)
}

func hrValidScheme(s string) bool {
	switch s {
	case HRSchemePension, HRSchemeMedical, HRSchemeSeriousIllness, HRSchemeUnemployment, HRSchemeInjury:
		return true
	}
	return false
}

func hrValidPart(p string) bool { return p == HRPartPersonal || p == HRPartUnit }

func hrValidFileType(f string) bool { return f == HRFileTypeNormal || f == HRFileTypeAdjustment }

// ---------------- 纯函数：校验与聚合（严格照源 buildAggregates） ----------------

// hrRequiredUploads 个人四险 / 单位五险必传（源 processor.go:869-883）。
var hrRequiredUploads = map[string][]string{
	HRPartPersonal: {HRSchemePension, HRSchemeMedical, HRSchemeSeriousIllness, HRSchemeUnemployment},
	HRPartUnit:     {HRSchemePension, HRSchemeMedical, HRSchemeSeriousIllness, HRSchemeUnemployment, HRSchemeInjury},
}

func hrValidateRequired(records []HRSIRawRecord) error {
	found := map[string]map[string]bool{}
	for _, rec := range records {
		if found[rec.Part] == nil {
			found[rec.Part] = map[string]bool{}
		}
		found[rec.Part][rec.Scheme] = true
	}
	for part, schemes := range hrRequiredUploads {
		for _, scheme := range schemes {
			if !found[part][scheme] {
				return fmt.Errorf("missing required data for part=%s scheme=%s", part, scheme)
			}
		}
	}
	return nil
}

type hrPersonAccumulator struct {
	Name         string
	IDNumber     string
	Department   string
	PersonalBase float64
	UnitBase     float64
	Personal     struct{ Pension, Medical, SeriousIllness, Unemployment float64 }
	Unit         struct{ Pension, Medical, SeriousIllness, Injury, Unemployment float64 }
}

// hrBuildAggregates 按 id_number 聚合：个人 4 险 / 单位 5 险；
// 单位医疗 = medical + serious_illness；单位 Base = max(unitBase, personalBase)；
// 姓名/部门优先取花名册，缺失时回落原始明细。
func hrBuildAggregates(records []HRSIRawRecord, employees map[string]HREmployee) []HRSICharge {
	personMap := map[string]*hrPersonAccumulator{}
	for _, rec := range records {
		key := normalizeIDNumber(rec.IDNumber)
		if key == "" {
			continue
		}
		person, ok := personMap[key]
		if !ok {
			name, dept := rec.Name, rec.Department
			if e, exists := employees[key]; exists {
				if e.Name != "" {
					name = e.Name
				}
				if e.Department != "" {
					dept = e.Department
				}
			}
			person = &hrPersonAccumulator{Name: name, IDNumber: key, Department: dept}
			personMap[key] = person
		}
		switch rec.Part {
		case HRPartPersonal:
			if person.PersonalBase == 0 {
				person.PersonalBase = rec.PayBase
			}
			switch rec.Scheme {
			case HRSchemePension:
				person.Personal.Pension += rec.AmountDue
			case HRSchemeMedical:
				person.Personal.Medical += rec.AmountDue
			case HRSchemeSeriousIllness:
				person.Personal.SeriousIllness += rec.AmountDue
			case HRSchemeUnemployment:
				person.Personal.Unemployment += rec.AmountDue
			}
		case HRPartUnit:
			if person.UnitBase == 0 {
				person.UnitBase = rec.PayBase
			}
			switch rec.Scheme {
			case HRSchemePension:
				person.Unit.Pension += rec.AmountDue
			case HRSchemeMedical:
				person.Unit.Medical += rec.AmountDue
			case HRSchemeSeriousIllness:
				person.Unit.SeriousIllness += rec.AmountDue
			case HRSchemeUnemployment:
				person.Unit.Unemployment += rec.AmountDue
			case HRSchemeInjury:
				person.Unit.Injury += rec.AmountDue
			}
		}
	}

	charges := make([]HRSICharge, 0, len(personMap)*2)
	for _, p := range personMap {
		personalSubtotal := round2(p.Personal.Pension + p.Personal.Medical + p.Personal.SeriousIllness + p.Personal.Unemployment)
		charges = append(charges, HRSICharge{
			Part: HRPartPersonal, Name: p.Name, IDNumber: p.IDNumber, Department: p.Department,
			Base: round2(p.PersonalBase), Pension: round2(p.Personal.Pension),
			MedicalMaternity: round2(p.Personal.Medical), SeriousIllness: round2(p.Personal.SeriousIllness),
			Unemployment: round2(p.Personal.Unemployment), Subtotal: personalSubtotal,
		})
		unitMedical := p.Unit.Medical + p.Unit.SeriousIllness
		unitSubtotal := round2(p.Unit.Pension + unitMedical + p.Unit.Injury + p.Unit.Unemployment)
		charges = append(charges, HRSICharge{
			Part: HRPartUnit, Name: p.Name, IDNumber: p.IDNumber, Department: p.Department,
			Base:    round2(maxFloat(p.UnitBase, p.PersonalBase)),
			Pension: round2(p.Unit.Pension), MedicalMaternity: round2(unitMedical),
			SeriousIllness: round2(p.Unit.SeriousIllness), Injury: round2(p.Unit.Injury),
			Unemployment: round2(p.Unit.Unemployment), Subtotal: unitSubtotal,
		})
	}
	sort.Slice(charges, func(i, j int) bool {
		if charges[i].IDNumber != charges[j].IDNumber {
			return charges[i].IDNumber < charges[j].IDNumber
		}
		return charges[i].Part < charges[j].Part
	})
	return charges
}

// hrMergeCharges 补退累加：按 (id_number, part) 把补退金额累加到基础明细。
// Subtotal 口径照源：个人 = 养老+医疗+大病+失业；单位 = 养老+医疗(含大病)+工伤+失业。
func hrMergeCharges(base, adjustments []HRSICharge) []HRSICharge {
	idx := map[string]int{}
	for i := range base {
		idx[base[i].IDNumber+"|"+base[i].Part] = i
	}
	for _, a := range adjustments {
		key := a.IDNumber + "|" + a.Part
		if i, ok := idx[key]; ok {
			b := &base[i]
			b.Base += a.Base
			b.Pension += a.Pension
			b.MedicalMaternity += a.MedicalMaternity
			b.SeriousIllness += a.SeriousIllness
			b.Injury += a.Injury
			b.Unemployment += a.Unemployment
			if b.Part == HRPartPersonal {
				b.Subtotal = round2(b.Pension + b.MedicalMaternity + b.SeriousIllness + b.Unemployment)
			} else {
				b.Subtotal = round2(b.Pension + b.MedicalMaternity + b.Injury + b.Unemployment)
			}
			if b.Department == "" && a.Department != "" {
				b.Department = a.Department
			}
			if b.Name == "" && a.Name != "" {
				b.Name = a.Name
			}
		} else {
			a.IsAdjustment = true
			base = append(base, a)
			idx[key] = len(base) - 1
		}
	}
	sort.Slice(base, func(i, j int) bool {
		if base[i].IDNumber != base[j].IDNumber {
			return base[i].IDNumber < base[j].IDNumber
		}
		return base[i].Part < base[j].Part
	})
	return base
}

// hrComputeCharges 从账期全部原始明细计算扣款：正常明细为基础，补退明细按人累加。
func (h *HRHandler) hrComputeCharges(ctx context.Context, tid int64, periodID string) ([]HRSICharge, bool, error) {
	var period HRSIPeriod
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", periodID, tid).First(&period).Error; err != nil {
		return nil, false, fmt.Errorf("账期不存在")
	}
	var raws []HRSIRawRecord
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND period_id = ?", tid, periodID).Find(&raws).Error; err != nil {
		return nil, false, err
	}
	if len(raws) == 0 {
		return nil, false, fmt.Errorf("账期无原始明细")
	}
	var emps []HREmployee
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&emps).Error; err != nil {
		return nil, false, err
	}
	empMap := map[string]HREmployee{}
	for _, e := range emps {
		if k := normalizeIDNumber(e.IDNumber); k != "" {
			empMap[k] = e
		}
	}
	var normalRaw, adjRaw []HRSIRawRecord
	hasAdj := false
	for _, r := range raws {
		if r.FileType == HRFileTypeAdjustment {
			adjRaw = append(adjRaw, r)
			hasAdj = true
		} else {
			normalRaw = append(normalRaw, r)
		}
	}
	if len(normalRaw) > 0 {
		if err := hrValidateRequired(normalRaw); err != nil {
			return nil, false, err
		}
	}
	charges := hrBuildAggregates(normalRaw, empMap)
	if len(adjRaw) > 0 {
		charges = hrMergeCharges(charges, hrBuildAggregates(adjRaw, empMap))
	}
	for i := range charges {
		charges[i].ID = uuid.New().String()
		charges[i].TenantID = tid
		charges[i].PeriodID = periodID
	}
	return charges, hasAdj, nil
}

func (h *HRHandler) hrPersistCharges(ctx context.Context, tid int64, periodID string, charges []HRSICharge) error {
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND period_id = ?", tid, periodID).Delete(&HRSICharge{}).Error; err != nil {
			return err
		}
		if len(charges) > 0 {
			return tx.Create(&charges).Error
		}
		return nil
	})
}

// ---------------- 账期 ----------------

// ListPeriods GET 严格 ORDER BY year_month DESC。
// hrSummaryOfCharges 汇总账期扣款明细，写回 hr_si_periods.summary_data。
// 口径与列表列一一对应：参保人数 = 个人明细行数，个人/单位合计 = 各自 subtotal 累加。
func hrSummaryOfCharges(charges []HRSICharge) map[string]any {
	headcount := 0
	personalTotal, unitTotal := 0.0, 0.0
	for _, c := range charges {
		if c.Part == HRPartPersonal {
			headcount++
			personalTotal += c.Subtotal
		} else if c.Part == HRPartUnit {
			unitTotal += c.Subtotal
		}
	}
	return map[string]any{
		"headcount":      headcount,
		"record_count":   headcount,
		"personal_total": round2(personalTotal),
		"unit_total":     round2(unitTotal),
	}
}

// hrPeriodView 把账期行展开为响应 map，并把 summary_data 的键提升到顶层。
// 列表列定义按「账期 / 参保人数 / 个人扣款合计 / 单位扣款合计」多别名取值，
// 顶层展开可让前端不必关心 JSONB 嵌套，也便于导出与打印复用同一份数据。
func hrPeriodView(p HRSIPeriod) map[string]any {
	raw, _ := json.Marshal(p)
	view := map[string]any{}
	_ = json.Unmarshal(raw, &view)
	for k, v := range p.SummaryData {
		view[k] = v
	}
	// 参保状态列：已汇总账期视为「正常」，草稿账期视为「待处理」。
	if _, ok := view["insure_status"]; !ok {
		if p.Status == "processed" {
			view["insure_status"] = "normal"
		} else {
			view["insure_status"] = "pending"
		}
	}
	return view
}

func (h *HRHandler) ListPeriods(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var rows []HRSIPeriod
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).
		Order("year_month DESC").Find(&rows).Error; err != nil {
		hrFail(c, "查询账期失败: "+err.Error())
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, p := range rows {
		items = append(items, hrPeriodView(p))
	}
	hrOK(c, gin.H{"items": items, "total": len(items)})
}

func (h *HRHandler) CreatePeriod(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req struct {
		YearMonth        string `json:"year_month"`
		Status           string `json:"status"`
		AllowAdjustments bool   `json:"allow_adjustments"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	ym, err := hrNormalizeMonthLabel(req.YearMonth)
	if err != nil {
		hrFail(c, "year_month 格式应为 YYYY-MM")
		return
	}
	var cnt int64
	h.db.WithContext(ctx).Model(&HRSIPeriod{}).
		Where("tenant_id = ? AND year_month = ?", tid, ym).Count(&cnt)
	if cnt > 0 {
		hrFail(c, "该账期已存在")
		return
	}
	period := HRSIPeriod{
		ID: uuid.New().String(), TenantID: tid, YearMonth: ym,
		Status: req.Status, AllowAdjustments: req.AllowAdjustments,
	}
	if period.Status == "" {
		period.Status = "pending"
	}
	if err := h.db.WithContext(ctx).Create(&period).Error; err != nil {
		hrFail(c, "创建账期失败: "+err.Error())
		return
	}
	hrOK(c, period)
}

func (h *HRHandler) DeletePeriod(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	pid := c.Param("id")
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, model := range []any{&HRSIRawRecord{}, &HRSICharge{}, &HRSIChange{}} {
			if err := tx.Where("tenant_id = ? AND period_id = ?", tid, pid).Delete(model).Error; err != nil {
				return err
			}
		}
		return tx.Where("id = ? AND tenant_id = ?", pid, tid).Delete(&HRSIPeriod{}).Error
	})
	if err != nil {
		hrFail(c, "删除账期失败: "+err.Error())
		return
	}
	hrOK(c, gin.H{"deleted": true})
}

// ImportSocialFile multipart(file, scheme, part, file_type)；正常文件覆盖 / 补退文件累加。
func (h *HRHandler) ImportSocialFile(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	pid := c.Param("id")
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
	scheme := strings.TrimSpace(c.PostForm("scheme"))
	part := strings.TrimSpace(c.PostForm("part"))
	fileType := strings.TrimSpace(c.PostForm("file_type"))
	if fileType == "" {
		fileType = HRFileTypeNormal
	}
	if !hrValidScheme(scheme) || !hrValidPart(part) || !hrValidFileType(fileType) {
		hrFail(c, "scheme/part/file_type 不合法")
		return
	}
	var period HRSIPeriod
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", pid, tid).First(&period).Error; err != nil {
		hrFail(c, "账期不存在")
		return
	}
	rows, err := hrReadExcelRows(src)
	if err != nil {
		hrFail(c, "解析 Excel 失败: "+err.Error())
		return
	}
	if len(rows) < 2 {
		hrFail(c, "Excel 中没有数据行")
		return
	}
	indexMap := map[string]int{}
	for idx, cell := range rows[0] {
		if key, ok := headerMap[strings.TrimSpace(cell)]; ok {
			indexMap[key] = idx
		}
	}
	for _, key := range []string{"seq", "name", "id_number", "salary", "base", "rate", "amount_due"} {
		if _, ok := indexMap[key]; !ok {
			hrFail(c, "missing required column: "+key)
			return
		}
	}
	var records []HRSIRawRecord
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		seq := hrCell(row, indexMap, "seq")
		name := strings.TrimSpace(hrCell(row, indexMap, "name"))
		idNumber := normalizeIDNumber(hrCell(row, indexMap, "id_number"))
		if seq == "" || name == "" || idNumber == "" {
			continue
		}
		rec := HRSIRawRecord{
			Name: name, IDNumber: idNumber, Sequence: toInt(seq),
			IDType:     strings.TrimSpace(hrCell(row, indexMap, "id_type")),
			Department: strings.TrimSpace(hrCell(row, indexMap, "department")),
			PaySalary:  toFloat(hrCell(row, indexMap, "salary")),
			PayBase:    toFloat(hrCell(row, indexMap, "base")),
			RateText:   strings.TrimSpace(hrCell(row, indexMap, "rate")),
			AmountDue:  toFloat(hrCell(row, indexMap, "amount_due")),
		}
		if _, ok := indexMap["amount_adjust"]; ok {
			rec.AmountAdjust = toFloat(hrCell(row, indexMap, "amount_adjust"))
		} else {
			rec.AmountAdjust = rec.AmountDue
		}
		if _, ok := indexMap["person_code"]; ok {
			rec.PersonCode = strings.TrimSpace(hrCell(row, indexMap, "person_code"))
		}
		records = append(records, rec)
	}
	if len(records) == 0 {
		hrFail(c, "Excel 中没有找到有效的数据行")
		return
	}
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if fileType == HRFileTypeNormal {
			if err := tx.Where("tenant_id = ? AND period_id = ? AND scheme = ? AND part = ? AND file_type = ?",
				tid, pid, scheme, part, HRFileTypeNormal).Delete(&HRSIRawRecord{}).Error; err != nil {
				return err
			}
		}
		for i := range records {
			records[i].ID = uuid.New().String()
			records[i].TenantID = tid
			records[i].PeriodID = pid
			records[i].Scheme = scheme
			records[i].Part = part
			records[i].FileType = fileType
		}
		return tx.Create(&records).Error
	})
	if err != nil {
		hrFail(c, "导入明细失败: "+err.Error())
		return
	}
	hrOK(c, gin.H{"imported": len(records), "scheme": scheme, "part": part, "file_type": fileType})
}

// ProcessPeriod 聚合并落库（个人/单位扣款）。
func (h *HRHandler) ProcessPeriod(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	pid := c.Param("id")
	charges, _, err := h.hrComputeCharges(ctx, tid, pid)
	if err != nil {
		hrFail(c, err.Error())
		return
	}
	if err := h.hrPersistCharges(ctx, tid, pid, charges); err != nil {
		hrFail(c, "聚合失败: "+err.Error())
		return
	}
	// summary_data 是 serializer:json 字段，必须走结构体 Save；
	// Updates(map) 不经过 serializer，PG 下会把 map 当 record 写入而报错（42804）。
	var period HRSIPeriod
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", pid, tid).First(&period).Error; err == nil {
		period.Status = "processed"
		period.SummaryData = hrSummaryOfCharges(charges)
		period.UpdatedAt = time.Now()
		if err := h.db.WithContext(ctx).Save(&period).Error; err != nil {
			logger.Warnf(ctx, "persist HR period summary failed: %v", err)
		}
	}
	hrOK(c, gin.H{"period_id": pid, "count": len(charges), "charges": charges})
}

// ProcessAdjustments 补退累加：要求账期含补退文件，重算并把结果标记为补退记录。
func (h *HRHandler) ProcessAdjustments(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	pid := c.Param("id")
	charges, hasAdj, err := h.hrComputeCharges(ctx, tid, pid)
	if err != nil {
		hrFail(c, err.Error())
		return
	}
	if !hasAdj {
		hrFail(c, "账期无补退数据")
		return
	}
	for i := range charges {
		charges[i].IsAdjustment = true
	}
	if err := h.hrPersistCharges(ctx, tid, pid, charges); err != nil {
		hrFail(c, "补退累加失败: "+err.Error())
		return
	}
	hrOK(c, gin.H{"period_id": pid, "count": len(charges), "charges": charges})
}

// ListCharges GET /social/periods/:id/charges?part=。
func (h *HRHandler) ListCharges(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	pid := c.Param("id")
	q := h.db.WithContext(ctx).Model(&HRSICharge{}).Where("tenant_id = ? AND period_id = ?", tid, pid)
	if part := strings.TrimSpace(c.Query("part")); part != "" {
		q = q.Where("part = ?", part)
	}
	var items []HRSICharge
	if err := q.Order("id_number ASC, part ASC").Find(&items).Error; err != nil {
		hrFail(c, "查询扣款明细失败: "+err.Error())
		return
	}
	hrOK(c, gin.H{"items": items, "total": len(items)})
}

var hrChargeExportHeaders = []string{
	"姓名", "身份证号码", "部门", "类别", "缴费基数", "养老", "医疗", "大病", "工伤", "失业", "小计",
}

func (h *HRHandler) ExportCharges(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	pid := c.Param("id")
	q := h.db.WithContext(ctx).Model(&HRSICharge{}).Where("tenant_id = ? AND period_id = ?", tid, pid)
	if part := strings.TrimSpace(c.Query("part")); part != "" {
		q = q.Where("part = ?", part)
	}
	var items []HRSICharge
	if err := q.Order("id_number ASC, part ASC").Find(&items).Error; err != nil {
		hrFail(c, "导出扣款明细失败: "+err.Error())
		return
	}
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{
			it.Name, it.IDNumber, it.Department, it.Part,
			hrFloatStr(it.Base), hrFloatStr(it.Pension), hrFloatStr(it.MedicalMaternity),
			hrFloatStr(it.SeriousIllness), hrFloatStr(it.Injury), hrFloatStr(it.Unemployment),
			hrFloatStr(it.Subtotal),
		})
	}
	hrExcelDownload(c, "hr_si_charges.xlsx", hrChargeExportHeaders, rows)
}

// ---------------- 增减员 ----------------

func (h *HRHandler) ListChanges(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	pid := c.Param("id")
	var items []HRSIChange
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND period_id = ?", tid, pid).
		Order("created_at DESC, id ASC").Find(&items).Error; err != nil {
		hrFail(c, "查询增减员失败: "+err.Error())
		return
	}
	hrOK(c, gin.H{"items": items, "total": len(items)})
}

func (h *HRHandler) CreateChange(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req HRSIChange
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	req.ID = uuid.New().String()
	req.TenantID = tid
	req.PeriodID = c.Param("id")
	if err := h.db.WithContext(ctx).Create(&req).Error; err != nil {
		hrFail(c, "新增增减员失败: "+err.Error())
		return
	}
	hrOK(c, req)
}

func (h *HRHandler) UpdateChange(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	pid := c.Param("id")
	var existing HRSIChange
	if err := h.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ? AND period_id = ?", c.Param("changeId"), tid, pid).
		First(&existing).Error; err != nil {
		hrFail(c, "增减员记录不存在")
		return
	}
	var req HRSIChange
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	req.ID = existing.ID
	req.TenantID = existing.TenantID
	req.PeriodID = existing.PeriodID
	req.CreatedAt = existing.CreatedAt
	if err := h.db.WithContext(ctx).Save(&req).Error; err != nil {
		hrFail(c, "更新增减员失败: "+err.Error())
		return
	}
	hrOK(c, req)
}

func (h *HRHandler) DeleteChange(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	res := h.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ? AND period_id = ?", c.Param("changeId"), tid, c.Param("id")).
		Delete(&HRSIChange{})
	if res.Error != nil {
		hrFail(c, "删除增减员失败: "+res.Error.Error())
		return
	}
	hrOK(c, gin.H{"deleted": res.RowsAffected})
}
