// 社保原始记录自适应批量导入 + 原始/汇总双视图（第 47 轮）。
//
// 真实社保网站导出文件的特征（已侦察 D:\Desktop\李永娇\社保\2026.10\）：
//
//	· 正常社保：9 个 xlsx，文件名形如
//	  「职工基本养老保险(个人缴纳)_2026-10至2026-10_未申报信息明细.xlsx」
//	  —— 险种、单位/个人、费款所属期全部编码在文件名里；
//	· 表头固定 11 列：序号/姓名/证件类型/证件号码/缴费工资/缴费基数/费率/
//	  应缴费额/减免费额/应补(退)费额/人员编号（人员编号即个人社保编号）；
//	· 异常社保：单位级申报表（PDF/xlsx），无逐人明细，只体现补退金额。
//
// 因此导入必须**从文件名推断 scheme/part/所属期**，不能再要求前端传参。
package handler

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/errors"
)

// hrSIFileMeta 从文件名解析出的元信息。
type hrSIFileMeta struct {
	Scheme    string
	Part      string
	FileType  string
	YearMonth string
}

// 文件名里的费款所属期：2026-10至2026-10 / 2026-10_2026-10 / 202610
var hrSIYearMonthRe = regexp.MustCompile(`((?:19|20)\d{2})[-_.]?(0[1-9]|1[0-2])`)

// hrSIParseFileName 从真实文件名推断险种 / 单位个人 / 正常补退 / 所属期。
//
// 判定顺序很关键：「大额医疗互助」必须先于「医疗」，
// 否则「职工大额医疗互助保险」会被吸到 medical。
func hrSIParseFileName(name string) hrSIFileMeta {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	meta := hrSIFileMeta{FileType: HRFileTypeNormal}

	switch {
	case strings.Contains(base, "大额") || strings.Contains(base, "大病"):
		meta.Scheme = HRSchemeSeriousIllness
	case strings.Contains(base, "养老"):
		meta.Scheme = HRSchemePension
	case strings.Contains(base, "医疗") || strings.Contains(base, "生育"):
		meta.Scheme = HRSchemeMedical
	case strings.Contains(base, "失业"):
		meta.Scheme = HRSchemeUnemployment
	case strings.Contains(base, "工伤"):
		meta.Scheme = HRSchemeInjury
	}
	// 工伤只有单位缴纳；个人缴纳关键字优先判定
	switch {
	case strings.Contains(base, "个人缴纳") || strings.Contains(base, "个人"):
		meta.Part = HRPartPersonal
	case strings.Contains(base, "单位缴纳") || strings.Contains(base, "单位"):
		meta.Part = HRPartUnit
	}
	// 工伤无「个人/单位」字样时默认单位
	if meta.Part == "" && meta.Scheme == HRSchemeInjury {
		meta.Part = HRPartUnit
	}
	if strings.Contains(base, "补缴") || strings.Contains(base, "退费") ||
		strings.Contains(base, "补退") || strings.Contains(base, "异常") {
		meta.FileType = HRFileTypeAdjustment
	}
	if m := hrSIYearMonthRe.FindStringSubmatch(base); len(m) == 3 {
		meta.YearMonth = m[1] + "-" + m[2]
	}
	return meta
}

// hrSIRowPayload 单行解析结果（导出给测试用）。
type hrSIRowPayload struct {
	Name       string
	IDNumber   string
	PaySalary  float64
	PayBase    float64
	RateText   string
	AmountDue  float64
	Deduction  float64
	AmountAdj  float64
	PersonCode string
}

// hrParseSIRows 解析真实社保明细行；表头必须是 11 列标准结构。
// 返回解析出的行与识别到的表头列数（0 表示不是标准社保明细表）。
func hrParseSIRows(rows [][]string) ([]hrSIRowPayload, error) {
	if len(rows) < 2 {
		return nil, fmt.Errorf("文件中没有数据行")
	}
	indexMap := map[string]int{}
	for idx, cell := range rows[0] {
		if key, ok := headerMap[strings.TrimSpace(cell)]; ok {
			indexMap[key] = idx
		}
	}
	for _, key := range []string{"name", "id_number", "base", "amount_due"} {
		if _, ok := indexMap[key]; !ok {
			return nil, fmt.Errorf("表头缺少必需列: %s", key)
		}
	}
	out := make([]hrSIRowPayload, 0, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		name := strings.TrimSpace(hrCell(row, indexMap, "name"))
		idNumber := normalizeIDNumber(hrCell(row, indexMap, "id_number"))
		if name == "" && idNumber == "" {
			continue
		}
		out = append(out, hrSIRowPayload{
			Name:       name,
			IDNumber:   idNumber,
			PaySalary:  toFloat(hrCell(row, indexMap, "salary")),
			PayBase:    toFloat(hrCell(row, indexMap, "base")),
			RateText:   strings.TrimSpace(hrCell(row, indexMap, "rate")),
			AmountDue:  toFloat(hrCell(row, indexMap, "amount_due")),
			Deduction:  toFloat(hrCell(row, indexMap, "deduction")),
			AmountAdj:  toFloat(hrCell(row, indexMap, "amount_adjust")),
			PersonCode: strings.TrimSpace(hrCell(row, indexMap, "person_code")),
		})
	}
	return out, nil
}

// ImportSocialBatch POST /social/import —— 多文件自适应批量导入。
//
// 每个文件独立推断 scheme/part/file_type/所属期；所属期不存在则自动创建账期。
// 正常文件覆盖同 (账期, 险种, 单位个人) 的旧记录，补退文件累加。
func (h *HRHandler) ImportSocialBatch(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	form, err := c.MultipartForm()
	if err != nil {
		hrFail(c, "缺少上传文件: "+err.Error())
		return
	}
	files := form.File["files"]
	if len(files) == 0 {
		files = form.File["file"]
	}
	if len(files) == 0 {
		hrFail(c, "请至少选择一个社保文件")
		return
	}
	// 允许前端显式指定所属期（文件名解析失败时兜底）
	forcedMonth := strings.TrimSpace(c.PostForm("year_month"))

	type fileResult struct {
		File      string `json:"file"`
		Scheme    string `json:"scheme"`
		Part      string `json:"part"`
		FileType  string `json:"file_type"`
		YearMonth string `json:"year_month"`
		Rows      int    `json:"rows"`
		Status    string `json:"status"`
		Message   string `json:"message,omitempty"`
	}
	results := make([]fileResult, 0, len(files))
	importedRows := 0
	touchedPeriods := map[string]bool{}

	for _, fh := range files {
		res := fileResult{File: fh.Filename}
		meta := hrSIParseFileName(fh.Filename)
		res.Scheme, res.Part, res.FileType = meta.Scheme, meta.Part, meta.FileType
		res.YearMonth = meta.YearMonth
		if res.YearMonth == "" {
			res.YearMonth = forcedMonth
		}
		if res.Scheme == "" || res.Part == "" {
			res.Status = "skipped"
			res.Message = "文件名无法识别险种或单位/个人"
			results = append(results, res)
			continue
		}
		if res.YearMonth == "" {
			res.Status = "skipped"
			res.Message = "文件名与请求均未提供费款所属期"
			results = append(results, res)
			continue
		}
		parsed, perr := hrParseSocialUpload(fh)
		if perr != nil {
			res.Status = "failed"
			res.Message = perr.Error()
			results = append(results, res)
			continue
		}
		res.Rows = len(parsed)
		if len(parsed) == 0 {
			res.Status = "skipped"
			res.Message = "没有有效数据行"
			results = append(results, res)
			continue
		}
		period, perr := h.hrEnsurePeriod(ctx, tid, res.YearMonth)
		if perr != nil {
			res.Status = "failed"
			res.Message = perr.Error()
			results = append(results, res)
			continue
		}
		if err := h.hrReplaceSIRecords(ctx, tid, period.ID, meta, parsed); err != nil {
			res.Status = "failed"
			res.Message = err.Error()
			results = append(results, res)
			continue
		}
		importedRows += len(parsed)
		touchedPeriods[res.YearMonth] = true
		res.Status = "ok"
		results = append(results, res)
	}

	// 导入后按身份证把「人员编号」同步到花名册的个人社保账号
	synced := h.hrSyncSocialNumbers(ctx, tid)

	hrOK(c, gin.H{
		"files": results, "rows": importedRows, "synced": synced,
		"periods": len(touchedPeriods),
	})
}

// hrParseSocialUpload 读取上传文件；xlsx 走 excelize，csv 走纯文本分列。
func hrParseSocialUpload(fh *multipart.FileHeader) ([]hrSIRowPayload, error) {
	src, err := fh.Open()
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer func() { _ = src.Close() }()
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if ext == ".csv" {
		rows, err := hrReadCSVRows(src)
		if err != nil {
			return nil, err
		}
		return hrParseSIRows(rows)
	}
	rows, err := hrReadExcelRows(src)
	if err != nil {
		return nil, fmt.Errorf("解析 Excel 失败: %w", err)
	}
	return hrParseSIRows(rows)
}

// hrEnsurePeriod 取或建账期（幂等）。
func (h *HRHandler) hrEnsurePeriod(ctx context.Context, tid int64, yearMonth string) (HRSIPeriod, error) {
	ym, err := hrNormalizeMonthLabel(yearMonth)
	if err != nil {
		return HRSIPeriod{}, fmt.Errorf("费款所属期格式不正确: %s", yearMonth)
	}
	var period HRSIPeriod
	findErr := h.db.WithContext(ctx).
		Where("tenant_id = ? AND year_month = ?", tid, ym).First(&period).Error
	if findErr == nil {
		return period, nil
	}
	period = HRSIPeriod{
		ID: uuid.New().String(), TenantID: tid, YearMonth: ym, Status: "pending",
	}
	if err := h.db.WithContext(ctx).Create(&period).Error; err != nil {
		// 并发下可能被别的请求抢先插入，再查一次
		if e2 := h.db.WithContext(ctx).
			Where("tenant_id = ? AND year_month = ?", tid, ym).First(&period).Error; e2 == nil {
			return period, nil
		}
		return HRSIPeriod{}, fmt.Errorf("创建账期失败: %w", err)
	}
	return period, nil
}

// hrReplaceSIRecords 正常文件先清同组旧记录再写；补退文件直接追加。
func (h *HRHandler) hrReplaceSIRecords(
	ctx context.Context, tid int64, periodID string, meta hrSIFileMeta, rows []hrSIRowPayload,
) error {
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if meta.FileType == HRFileTypeNormal {
			if err := tx.Where(
				"tenant_id = ? AND period_id = ? AND scheme = ? AND part = ? AND file_type = ?",
				tid, periodID, meta.Scheme, meta.Part, HRFileTypeNormal,
			).Delete(&HRSIRawRecord{}).Error; err != nil {
				return err
			}
		}
		records := make([]HRSIRawRecord, 0, len(rows))
		for i, r := range rows {
			records = append(records, HRSIRawRecord{
				ID: uuid.New().String(), TenantID: tid, PeriodID: periodID,
				Sequence: i + 1, Name: r.Name, IDType: "居民身份证", IDNumber: r.IDNumber,
				PaySalary: r.PaySalary, PayBase: r.PayBase, RateText: r.RateText,
				AmountDue: r.AmountDue, AmountAdjust: r.AmountAdj, PersonCode: r.PersonCode,
				Scheme: meta.Scheme, Part: meta.Part, FileType: meta.FileType,
			})
		}
		if len(records) == 0 {
			return nil
		}
		return tx.Create(&records).Error
	})
}

// hrSyncSocialNumbers 把原始记录里的「人员编号」按身份证回填花名册个人社保账号。
// 只填空值，不覆盖用户手填；返回更新条数。
func (h *HRHandler) hrSyncSocialNumbers(ctx context.Context, tid int64) int {
	var raws []HRSIRawRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND person_code <> ''", tid).
		Order("created_at DESC").Find(&raws).Error; err != nil {
		return 0
	}
	best := map[string]string{}
	for _, r := range raws {
		k := normalizeIDNumber(r.IDNumber)
		if k == "" || r.PersonCode == "" {
			continue
		}
		// 纯数字编号优先（医疗类的 JM 前缀是另一套编码，尽量保留社保号本号）
		if cur, ok := best[k]; !ok {
			best[k] = r.PersonCode
			continue
		} else if len(r.PersonCode) < len(cur) {
			best[k] = r.PersonCode
		}
	}
	if len(best) == 0 {
		return 0
	}
	keys := make([]string, 0, len(best))
	for k := range best {
		keys = append(keys, k)
	}
	var emps []HREmployee
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND id_number IN ?", tid, keys).Find(&emps).Error; err != nil {
		return 0
	}
	updated := 0
	for i := range emps {
		e := &emps[i]
		code, ok := best[normalizeIDNumber(e.IDNumber)]
		if !ok || code == "" {
			continue
		}
		if strings.TrimSpace(e.SocialInsuranceNumber) != "" {
			continue
		}
		e.SocialInsuranceNumber = code
		if e.CustomData == nil {
			e.CustomData = map[string]any{}
		}
		e.CustomData["个人社保账号"] = code
		if err := h.db.WithContext(ctx).Save(e).Error; err == nil {
			updated++
		}
	}
	return updated
}

// ListSIRawRecords GET /social/raw —— 原始记录列表（按账期倒序、员工排序）。
func (h *HRHandler) ListSIRawRecords(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	q := h.db.WithContext(ctx).Model(&HRSIRawRecord{}).Where("tenant_id = ?", tid)
	if ym := strings.TrimSpace(c.Query("year_month")); ym != "" {
		var period HRSIPeriod
		if err := h.db.WithContext(ctx).
			Where("tenant_id = ? AND year_month = ?", tid, ym).First(&period).Error; err == nil {
			q = q.Where("period_id = ?", period.ID)
		} else {
			hrOK(c, gin.H{"items": []any{}, "total": 0})
			return
		}
	}
	if scheme := strings.TrimSpace(c.Query("scheme")); scheme != "" && scheme != "all" {
		q = q.Where("scheme = ?", scheme)
	}
	var rows []HRSIRawRecord
	if err := q.Order("period_id DESC, scheme ASC, part ASC, sequence ASC").Find(&rows).Error; err != nil {
		hrFail(c, "查询原始记录失败: "+err.Error())
		return
	}
	// 账期与部门映射
	periodMonths := map[string]string{}
	var periods []HRSIPeriod
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&periods).Error; err == nil {
		for _, p := range periods {
			periodMonths[p.ID] = p.YearMonth
		}
	}
	deptMap := h.hrEmployeeDeptMap(ctx, tid)
	items := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		items = append(items, map[string]any{
			"id": r.ID, "period_id": r.PeriodID,
			"账期":     periodMonths[r.PeriodID],
			"记录性质":   hrSIFileTypeText(r.FileType),
			"险种":     hrSISchemeText(r.Scheme),
			"缴费类型":   hrSIPartText(r.Part),
			"个人社保编号": r.PersonCode,
			"姓名":     r.Name,
			"证件号码":   r.IDNumber,
			"部门":     deptMap[normalizeIDNumber(r.IDNumber)],
			"缴费基数":   round2(r.PayBase),
			"费率":     r.RateText,
			"单位应缴":   hrSIPartAmount(r, HRPartUnit),
			"个人应缴":   hrSIPartAmount(r, HRPartPersonal),
			"合计金额":   round2(r.AmountDue),
			"补退金额":   round2(r.AmountAdjust),
			"scheme": r.Scheme, "part": r.Part, "file_type": r.FileType,
			"person_code": r.PersonCode, "name": r.Name, "id_number": r.IDNumber,
			"pay_base": round2(r.PayBase), "amount_due": round2(r.AmountDue),
		})
	}
	hrOK(c, gin.H{"items": items, "total": len(items)})
}

func hrSIPartAmount(r HRSIRawRecord, part string) float64 {
	if r.Part == part {
		return round2(r.AmountDue)
	}
	return 0
}

func hrSIFileTypeText(t string) string {
	if t == HRFileTypeAdjustment {
		return "异常补退"
	}
	return "正常"
}

func hrSIPartText(p string) string {
	if p == HRPartUnit {
		return "单位"
	}
	return "个人"
}

func hrSISchemeText(s string) string {
	switch s {
	case HRSchemePension:
		return "养老"
	case HRSchemeMedical:
		return "医疗"
	case HRSchemeSeriousIllness:
		return "大病"
	case HRSchemeUnemployment:
		return "失业"
	case HRSchemeInjury:
		return "工伤"
	default:
		return s
	}
}

// hrEmployeeDeptMap 身份证 → 部门（原始记录展示用）。
func (h *HRHandler) hrEmployeeDeptMap(ctx context.Context, tid int64) map[string]string {
	out := map[string]string{}
	var emps []HREmployee
	if err := h.db.WithContext(ctx).Select("id_number, department").
		Where("tenant_id = ?", tid).Find(&emps).Error; err != nil {
		return out
	}
	for _, e := range emps {
		if k := normalizeIDNumber(e.IDNumber); k != "" {
			out[k] = e.Department
		}
	}
	return out
}

// GenerateSISummary POST /social/summary —— 按账期把原始记录聚合为每月 1 条汇总。
func (h *HRHandler) GenerateSISummary(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req struct {
		YearMonth string   `json:"year_month"`
		PeriodIDs []string `json:"period_ids"`
		IDs       []string `json:"ids"`
	}
	_ = c.ShouldBindJSON(&req)

	// 目标账期：显式 year_month 优先，其次从勾选记录反推
	months := map[string]bool{}
	if ym := strings.TrimSpace(req.YearMonth); ym != "" {
		months[ym] = true
	}
	if len(req.IDs) > 0 {
		var picked []HRSIRawRecord
		if err := h.db.WithContext(ctx).
			Where("tenant_id = ? AND id IN ?", tid, req.IDs).Find(&picked).Error; err == nil {
			periodIDs := map[string]bool{}
			for _, r := range picked {
				periodIDs[r.PeriodID] = true
			}
			var periods []HRSIPeriod
			if err := h.db.WithContext(ctx).
				Where("tenant_id = ? AND id IN ?", tid, keysOf(periodIDs)).Find(&periods).Error; err == nil {
				for _, p := range periods {
					months[p.YearMonth] = true
				}
			}
		}
	}
	for _, pid := range req.PeriodIDs {
		var p HRSIPeriod
		if err := h.db.WithContext(ctx).
			Where("tenant_id = ? AND id = ?", tid, pid).First(&p).Error; err == nil {
			months[p.YearMonth] = true
		}
	}
	if len(months) == 0 {
		var periods []HRSIPeriod
		if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&periods).Error; err == nil {
			for _, p := range periods {
				months[p.YearMonth] = true
			}
		}
	}
	if len(months) == 0 {
		hrFail(c, "没有可汇总的账期")
		return
	}

	generated := make([]map[string]any, 0, len(months))
	for ym := range months {
		var period HRSIPeriod
		if err := h.db.WithContext(ctx).
			Where("tenant_id = ? AND year_month = ?", tid, ym).First(&period).Error; err != nil {
			continue
		}
		var raws []HRSIRawRecord
		if err := h.db.WithContext(ctx).
			Where("tenant_id = ? AND period_id = ?", tid, period.ID).Find(&raws).Error; err != nil {
			continue
		}
		if len(raws) == 0 {
			continue
		}
		summary := hrBuildPeriodSummary(raws)
		period.Status = "processed"
		period.SummaryData = summary
		if err := h.db.WithContext(ctx).Save(&period).Error; err != nil {
			hrFail(c, "保存汇总失败: "+err.Error())
			return
		}
		generated = append(generated, map[string]any{"year_month": ym, "summary": summary})
	}
	if len(generated) == 0 {
		hrFail(c, "勾选的账期没有原始记录，无法汇总")
		return
	}
	hrOK(c, gin.H{"generated": generated, "count": len(generated)})
}

// hrBuildPeriodSummary 把原始记录聚合成 1 条月度汇总（任务 §1.2 汇总记录字段）。
func hrBuildPeriodSummary(raws []HRSIRawRecord) map[string]any {
	people := map[string]bool{}
	var pensionU, pensionP, medicalU, medicalP, serious, unemployU, unemployP, injuryU float64
	var adjust, totalU, totalP float64
	for _, r := range raws {
		key := normalizeIDNumber(r.IDNumber)
		if key != "" {
			people[key] = true
		}
		amount := r.AmountDue
		if r.FileType == HRFileTypeAdjustment {
			adjust += r.AmountAdjust
			amount += r.AmountAdjust
		}
		if r.Part == HRPartUnit {
			totalU += amount
			switch r.Scheme {
			case HRSchemePension:
				pensionU += amount
			case HRSchemeMedical:
				medicalU += amount
			case HRSchemeSeriousIllness:
				serious += amount
			case HRSchemeUnemployment:
				unemployU += amount
			case HRSchemeInjury:
				injuryU += amount
			}
		} else {
			totalP += amount
			switch r.Scheme {
			case HRSchemePension:
				pensionP += amount
			case HRSchemeMedical:
				medicalP += amount
			case HRSchemeSeriousIllness:
				serious += amount
			case HRSchemeUnemployment:
				unemployP += amount
			}
		}
	}
	return map[string]any{
		"headcount":         len(people),
		"record_count":      len(people),
		"pension_unit":      round2(pensionU),
		"pension_personal":  round2(pensionP),
		"medical_unit":      round2(medicalU),
		"medical_personal":  round2(medicalP),
		"serious_illness":   round2(serious),
		"unemploy_unit":     round2(unemployU),
		"unemploy_personal": round2(unemployP),
		"injury_unit":       round2(injuryU),
		"adjust_amount":     round2(adjust),
		"unit_total":        round2(totalU),
		"personal_total":    round2(totalP),
		"grand_total":       round2(totalU + totalP),
		"status":            "已汇总",
	}
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// GetSIPeriodDetail GET /social/periods/:id/detail —— 月度费用清单抽屉数据。
//
// 返回该账期所有员工的各险种明细（按员工一行），并附部门/险种筛选项。
func (h *HRHandler) GetSIPeriodDetail(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	pid := c.Param("id")
	var period HRSIPeriod
	if err := h.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", pid, tid).First(&period).Error; err != nil {
		c.Error(errors.NewNotFoundError("账期不存在"))
		return
	}
	var raws []HRSIRawRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND period_id = ?", tid, pid).Find(&raws).Error; err != nil {
		hrFail(c, "查询明细失败: "+err.Error())
		return
	}
	deptMap := h.hrEmployeeDeptMap(ctx, tid)
	empMap := h.hrEmployeeBriefMap(ctx, tid)

	type acc struct {
		name, idNumber, dept, empNo, personCode string
		base                                    float64
		pensionU, pensionP, medicalU, medicalP  float64
		serious, unemployU, unemployP, injuryU  float64
		adjust, totalU, totalP                  float64
	}
	order := make([]string, 0)
	people := map[string]*acc{}
	for _, r := range raws {
		key := normalizeIDNumber(r.IDNumber)
		if key == "" {
			continue
		}
		a, ok := people[key]
		if !ok {
			brief := empMap[key]
			name := r.Name
			if brief.Name != "" {
				name = brief.Name
			}
			dept := r.Department
			if dept == "" {
				dept = deptMap[key]
			}
			a = &acc{name: name, idNumber: key, dept: dept, empNo: brief.EmployeeID, personCode: r.PersonCode}
			people[key] = a
			order = append(order, key)
		}
		if r.PayBase > a.base {
			a.base = r.PayBase
		}
		if a.personCode == "" && r.PersonCode != "" {
			a.personCode = r.PersonCode
		}
		amount := r.AmountDue
		if r.FileType == HRFileTypeAdjustment {
			a.adjust += r.AmountAdjust
			amount += r.AmountAdjust
		}
		if r.Part == HRPartUnit {
			a.totalU += amount
			switch r.Scheme {
			case HRSchemePension:
				a.pensionU += amount
			case HRSchemeMedical:
				a.medicalU += amount
			case HRSchemeSeriousIllness:
				a.serious += amount
			case HRSchemeUnemployment:
				a.unemployU += amount
			case HRSchemeInjury:
				a.injuryU += amount
			}
		} else {
			a.totalP += amount
			switch r.Scheme {
			case HRSchemePension:
				a.pensionP += amount
			case HRSchemeMedical:
				a.medicalP += amount
			case HRSchemeSeriousIllness:
				a.serious += amount
			case HRSchemeUnemployment:
				a.unemployP += amount
			}
		}
	}

	items := make([]map[string]any, 0, len(order))
	depts := map[string]bool{}
	for _, key := range order {
		a := people[key]
		if a.dept != "" {
			depts[a.dept] = true
		}
		items = append(items, map[string]any{
			"id": key, "工号": a.empNo, "姓名": a.name, "部门": a.dept,
			"证件号码": a.idNumber, "个人社保编号": a.personCode,
			"缴费基数": round2(a.base),
			"养老单位": round2(a.pensionU), "养老个人": round2(a.pensionP),
			"医疗单位": round2(a.medicalU), "医疗个人": round2(a.medicalP),
			"大病":   round2(a.serious),
			"失业单位": round2(a.unemployU), "失业个人": round2(a.unemployP),
			"工伤单位": round2(a.injuryU),
			"补退金额": round2(a.adjust),
			"单位小计": round2(a.totalU), "个人小计": round2(a.totalP),
			"总计":          round2(a.totalU + a.totalP),
			"employee_id": a.empNo, "name": a.name, "department": a.dept,
			"id_number": a.idNumber,
		})
	}
	deptList := make([]string, 0, len(depts))
	for d := range depts {
		deptList = append(deptList, d)
	}
	hrOK(c, gin.H{
		"period": period, "items": items, "total": len(items),
		"departments": deptList,
		"summary":     period.SummaryData,
	})
}

// hrEmployeeBriefMap 身份证 → 工号/姓名（明细展示用）。
func (h *HRHandler) hrEmployeeBriefMap(ctx context.Context, tid int64) map[string]HREmployee {
	out := map[string]HREmployee{}
	var emps []HREmployee
	if err := h.db.WithContext(ctx).Select("id_number, employee_id, name, department").
		Where("tenant_id = ?", tid).Find(&emps).Error; err != nil {
		return out
	}
	for _, e := range emps {
		if k := normalizeIDNumber(e.IDNumber); k != "" {
			out[k] = e
		}
	}
	return out
}

// hrReadCSVRows 读 CSV（社保网站偶有导出为 csv 的情况）。
func hrReadCSVRows(r io.Reader) ([][]string, error) {
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	text := strings.TrimPrefix(string(buf), "\ufeff")
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([][]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, strings.Split(line, ","))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("CSV 中没有内容")
	}
	return out, nil
}
