// 公积金网厅（《单位职工明细-网厅.xls》）导入解析 + 18 字段归一（第 54 轮）。
//
// 网厅导出的真实结构：
//
//	第 1 行  标题（如「单位职工明细」）
//	第 2 行  单位账号 + 单位名称（单元格合并，A 列为空）
//	第 3 行  列头：职工账号 | 职工姓名 | 证件号码 | 账户状态 | ...
//	第 4~N  数据行（87 条）
//	末行     验证码：xxxx
//
// 解析器按「含职工账号 && 证件号码 的行」定位列头，而不是写死行号：
// 不同城市网厅的抬头行数不一致（1~3 行），写死行号必然错位。
package handler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shakinm/xlsReader/xls"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// 公积金账户状态（列存英文，界面显示中文）。
const (
	hrFundStatusActive    = "active"
	hrFundStatusSealed    = "sealed"
	hrFundStatusExtracted = "extracted"
)

// hrFundStatusOf 账户状态归一：中文 / 英文 / 网厅写法都收。
func hrFundStatusOf(v string) string {
	s := strings.TrimSpace(v)
	switch s {
	case "正常", "active", "normal", "缴存":
		return hrFundStatusActive
	case "封存", "sealed", "暂停":
		return hrFundStatusSealed
	case "提取销户", "销户", "extracted", "withdrawn", "终止":
		return hrFundStatusExtracted
	}
	if strings.Contains(s, "销户") || strings.Contains(s, "提取") {
		return hrFundStatusExtracted
	}
	if strings.Contains(s, "封存") {
		return hrFundStatusSealed
	}
	if strings.Contains(s, "正常") {
		return hrFundStatusActive
	}
	return s
}

// hrFundStatusCN 英文状态 → 界面中文。
func hrFundStatusCN(status string) string {
	switch hrFundStatusOf(status) {
	case hrFundStatusActive:
		return "正常"
	case hrFundStatusSealed:
		return "封存"
	case hrFundStatusExtracted:
		return "提取销户"
	default:
		return strings.TrimSpace(status)
	}
}

// 18 个原生字段（顺序与网厅一致，也是模板列顺序）。
var hrFundNativeFields = []string{
	"职工账号", "职工姓名", "证件号码", "账户状态",
	"缴存基数", "月缴存额", "单位月缴存额", "个人月缴存额", "账户余额",
	"单位缴存比例", "职工缴存比例", "联名卡银行", "联名卡号", "绑定手机",
	"是否公贷", "是否冻结", "是否可在线提取公贷", "是否可在线提取商贷",
}

// hrFundHeaderKey 表头归一：去空白 / 全角括号 / 单位后缀。
//
// 「缴存基数(元)」「缴存基数（元）」「缴存基数」必须落到同一个 key，
// 否则模板表头与网厅导出表头各认一套，导入后整列空白。
func hrFundHeaderKey(s string) string {
	v := strings.TrimSpace(s)
	if v == "" {
		return ""
	}
	// 全角括号转半角后再删括号段
	v = strings.NewReplacer("（", "(", "）", ")", "　", "").Replace(v)
	for {
		i := strings.Index(v, "(")
		j := strings.Index(v, ")")
		if i >= 0 && j > i {
			v = v[:i] + v[j+1:]
			continue
		}
		break
	}
	v = strings.NewReplacer(" ", "", "\t", "", "*", "", "：", "", ":", "").Replace(v)
	return v
}

// hrFundFieldOf 表头 → 18 字段之一（含常见别名）。
func hrFundFieldOf(header string) string {
	key := hrFundHeaderKey(header)
	switch key {
	case "职工账号", "个人账号", "个人帐号", "公积金账号", "账号":
		return "职工账号"
	case "职工姓名", "姓名":
		return "职工姓名"
	case "证件号码", "身份证号码", "身份证号":
		return "证件号码"
	case "账户状态", "缴存状态", "状态":
		return "账户状态"
	case "缴存基数", "缴存工资基数", "基数":
		return "缴存基数"
	case "月缴存额", "合计", "月缴存金额", "缴存额":
		return "月缴存额"
	case "单位月缴存额", "单位缴存额", "单位金额":
		return "单位月缴存额"
	case "个人月缴存额", "个人缴存额", "个人金额":
		return "个人月缴存额"
	case "账户余额", "余额":
		return "账户余额"
	case "单位缴存比例", "单位比例":
		return "单位缴存比例"
	case "职工缴存比例", "个人缴存比例", "个人比例", "比例", "缴存比例":
		return "职工缴存比例"
	case "联名卡银行", "开户银行", "银行":
		return "联名卡银行"
	case "联名卡号", "银行卡号", "卡号":
		return "联名卡号"
	case "绑定手机", "手机号", "手机号码", "联系电话":
		return "绑定手机"
	case "是否公贷":
		return "是否公贷"
	case "是否冻结":
		return "是否冻结"
	case "是否可在线提取公贷":
		return "是否可在线提取公贷"
	case "是否可在线提取商贷":
		return "是否可在线提取商贷"
	}
	return ""
}

// hrFundHeaderIndex 列头行 → 字段位置；同时给出「该行是否为列头」。
func hrFundHeaderIndex(row []string) (map[string]int, bool) {
	idx := make(map[string]int, len(row))
	for i, cell := range row {
		if f := hrFundFieldOf(cell); f != "" {
			if _, exists := idx[f]; !exists {
				idx[f] = i
			}
		}
	}
	_, hasAccount := idx["职工账号"]
	_, hasID := idx["证件号码"]
	return idx, hasAccount && hasID
}

// hrFindFundHeaderRow 在前若干行里定位列头（跳过标题 / 单位信息行）。
func hrFindFundHeaderRow(rows [][]string) (int, map[string]int) {
	limit := len(rows)
	if limit > 12 {
		limit = 12
	}
	for i := 0; i < limit; i++ {
		if idx, ok := hrFundHeaderIndex(rows[i]); ok {
			return i, idx
		}
	}
	return -1, nil
}

// hrReadFundSheet 读取网厅文件：xlsx 走 excelize，xls（BIFF）走 xlsReader。
//
// 两种格式的行结构一致，后续解析完全共用，不给 .xls 单独维护第二套逻辑。
func hrReadFundSheet(r io.Reader) ([][]string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("文件为空")
	}
	// ZIP 魔数 PK\x03\x04 → xlsx；OLE 复合文档 D0CF11E0 → xls
	if len(data) >= 4 && data[0] == 'P' && data[1] == 'K' {
		return hrReadExcelRows(bytes.NewReader(data))
	}
	wb, err := xls.OpenReader(bytes.NewReader(data))
	if err != nil {
		// 有些网厅文件后缀是 .xls 实际是 HTML/制表符文本，excelize 也救不了时给出明确文案
		return nil, fmt.Errorf("无法识别的 Excel 格式（支持 .xlsx 与网厅 .xls）：%v", err)
	}
	sheet, err := wb.GetSheet(0)
	if err != nil || sheet == nil {
		return nil, fmt.Errorf("Excel 文件中没有工作表")
	}
	raw := sheet.GetRows()
	out := make([][]string, 0, len(raw))
	for _, row := range raw {
		if row == nil {
			out = append(out, nil)
			continue
		}
		// xlsReader 以 1 为起始列号；GetCol 越界返回空单元格。
		cells := make([]string, 0, 24)
		for c := 1; c <= 24; c++ {
			cell, cerr := row.GetCol(c)
			if cerr != nil || cell == nil {
				cells = append(cells, "")
				continue
			}
			cells = append(cells, strings.TrimSpace(cell.GetString()))
		}
		out = append(out, cells)
	}
	return out, nil
}

// hrFundCellFloat 解析金额 / 比例：容忍千分位逗号、全角数字与「元」后缀。
func hrFundCellFloat(v string) float64 {
	s := strings.TrimSpace(v)
	if s == "" || s == "-" || s == "—" {
		return 0
	}
	s = strings.NewReplacer(",", "", "，", "", " ", "", "元", "", "%", "", "％", "").Replace(s)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func hrFundRound2(v float64) float64 { return math.Round(v*100) / 100 }

// hrFundFromRow 一行网厅数据 → HRPFRecord（18 字段全量落到列 + custom_data）。
func hrFundFromRow(row []string, idx map[string]int) (HRPFRecord, bool) {
	get := func(field string) string {
		i, ok := idx[field]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	account := get("职工账号")
	idNo := normalizeIDNumber(get("证件号码"))
	name := get("职工姓名")
	if account == "" && idNo == "" {
		return HRPFRecord{}, false
	}
	// 末行「验证码：xxxx」既不满足账号也不满足身份证，已在上面的空判断里被过滤。
	if !hrLooksLikeFundAccount(account) && idNo == "" {
		return HRPFRecord{}, false
	}
	rec := HRPFRecord{
		PersonalAccount: account,
		Name:            name,
		IdentityNumber:  idNo,
		Status:          hrFundStatusOf(get("账户状态")),
		PersonalBase:    hrFundCellFloat(get("缴存基数")),
		TotalAmount:     hrFundCellFloat(get("月缴存额")),
		CompanyAmount:   hrFundCellFloat(get("单位月缴存额")),
		PersonalAmount:  hrFundCellFloat(get("个人月缴存额")),
		CustomData:      map[string]any{},
	}
	if rec.Status == "" {
		rec.Status = hrFundStatusActive
	}
	// 只给了总额 + 单边时补齐另一边，保证「月缴存额 = 单位 + 个人」恒等。
	if rec.CompanyAmount == 0 && rec.PersonalAmount == 0 && rec.TotalAmount > 0 {
		rec.CompanyAmount = hrFundRound2(rec.TotalAmount / 2)
		rec.PersonalAmount = hrFundRound2(rec.TotalAmount - rec.CompanyAmount)
	}
	if rec.TotalAmount == 0 {
		rec.TotalAmount = hrFundRound2(rec.CompanyAmount + rec.PersonalAmount)
	}
	unitRatio := hrFundCellFloat(get("单位缴存比例"))
	personRatio := hrFundCellFloat(get("职工缴存比例"))
	if unitRatio == 0 {
		unitRatio = 5
	}
	if personRatio == 0 {
		personRatio = unitRatio
	}
	rec.ContributionRatio = personRatio
	c := rec.CustomData
	c["账户余额(元)"] = hrFundCellFloat(get("账户余额"))
	c["单位缴存比例(%)"] = unitRatio
	c["职工缴存比例(%)"] = personRatio
	c["联名卡银行"] = get("联名卡银行")
	c["联名卡号"] = get("联名卡号")
	c["绑定手机"] = get("绑定手机")
	for _, f := range []string{"是否公贷", "是否冻结", "是否可在线提取公贷", "是否可在线提取商贷"} {
		if v := get(f); v != "" {
			c[f] = v
		}
	}
	return rec, true
}

// hrLooksLikeFundAccount 职工账号形如 801504241502（12 位纯数字）。
func hrLooksLikeFundAccount(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 6 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// hrFundSyncCustom 把列值写回 custom_data（列表响应与抽屉都按 18 字段读）。
func hrFundSyncCustom(r *HRPFRecord) {
	if r.CustomData == nil {
		r.CustomData = map[string]any{}
	}
	c := r.CustomData
	c["职工账号"] = r.PersonalAccount
	c["职工姓名"] = r.Name
	c["证件号码"] = r.IdentityNumber
	c["账户状态"] = hrFundStatusCN(r.Status)
	c["缴存基数(元)"] = r.PersonalBase
	c["月缴存额(元)"] = r.TotalAmount
	c["单位月缴存额(元)"] = r.CompanyAmount
	c["个人月缴存额(元)"] = r.PersonalAmount
	if _, ok := c["账户余额(元)"]; !ok {
		c["账户余额(元)"] = 0
	}
	if _, ok := c["单位缴存比例(%)"]; !ok {
		c["单位缴存比例(%)"] = r.ContributionRatio
	}
	if _, ok := c["职工缴存比例(%)"]; !ok {
		c["职工缴存比例(%)"] = r.ContributionRatio
	}
}

// hrFundApplyRows 逐行 upsert（按证件号码 → 职工账号匹配），并同步花名册账号。
//
// 单行失败只计 skipped 不中断整批；全部写完后统一回写花名册的
// 「个人公积金账号」，保证导入完成即刻在花名册可见。
func (h *HRHandler) hrFundApplyRows(ctx context.Context, tid int64, rows [][]string) (map[string]int, error) {
	headerRow, idx := hrFindFundHeaderRow(rows)
	if headerRow < 0 {
		return nil, fmt.Errorf("未找到表头行：文件中需要包含「职工账号」「证件号码」列")
	}
	ratio := h.hrPFRatio(ctx, tid)
	inserted, updated, skipped := 0, 0, 0
	synced := map[string]string{}
	for _, row := range rows[headerRow+1:] {
		if len(row) == 0 {
			continue
		}
		// 末行验证码 / 合计行：既没有账号也没有身份证 → 跳过
		rec, ok := hrFundFromRow(row, idx)
		if !ok || rec.IdentityNumber == "" {
			skipped++
			continue
		}
		hrNormalizePFRecord(&rec, ratio)
		hrFundSyncCustom(&rec)
		var existing HRPFRecord
		findErr := h.db.WithContext(ctx).
			Where("tenant_id = ? AND (identity_number = ? OR personal_account = ?)",
				tid, rec.IdentityNumber, rec.PersonalAccount).
			First(&existing).Error
		if findErr == nil {
			rec.ID = existing.ID
			rec.TenantID = tid
			rec.CreatedAt = existing.CreatedAt
			rec.SealedAt = existing.SealedAt
			rec.UnsealedAt = existing.UnsealedAt
			if err := h.db.WithContext(ctx).Save(&rec).Error; err != nil {
				skipped++
				continue
			}
			updated++
		} else {
			rec.ID = uuid.New().String()
			rec.TenantID = tid
			if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
				skipped++
				continue
			}
			inserted++
		}
		if rec.PersonalAccount != "" {
			synced[rec.IdentityNumber] = rec.PersonalAccount
		}
	}
	if err := h.hrFundSyncRosterAccounts(ctx, tid, synced); err != nil {
		logger.Warnf(ctx, "sync fund account to roster failed: %v", err)
	}
	return map[string]int{"inserted": inserted, "updated": updated, "skipped": skipped}, nil
}

// hrFundSyncRosterAccounts 把职工账号写回花名册 custom_data + 主列（幂等）。
func (h *HRHandler) hrFundSyncRosterAccounts(ctx context.Context, tid int64, accounts map[string]string) error {
	if len(accounts) == 0 {
		return nil
	}
	ids := make([]string, 0, len(accounts))
	for k := range accounts {
		if k != "" {
			ids = append(ids, k)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var employees []HREmployee
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND id_number IN ?", tid, ids).Find(&employees).Error; err != nil {
		return err
	}
	for i := range employees {
		acc := accounts[normalizeIDNumber(employees[i].IDNumber)]
		if acc == "" {
			continue
		}
		if employees[i].ProvidentFundNumber == acc &&
			toStringValue(employees[i].CustomData["个人公积金账号"]) == acc {
			continue
		}
		employees[i].ProvidentFundNumber = acc
		employees[i].CustomData = hrMergeEmployeeCustomData(employees[i].CustomData, map[string]string{
			"个人公积金账号": acc,
		})
		if err := h.db.WithContext(ctx).Save(&employees[i]).Error; err != nil {
			logger.Warnf(ctx, "sync fund account %s failed: %v", employees[i].Name, err)
		}
	}
	return nil
}

// hrFundSeedRow 种子数据行（与网厅一致的 18 列顺序）。
type hrFundSeedRow struct {
	account     string
	name        string
	idNo        string
	status      string
	base        float64
	unit        float64
	person      float64
	balance     float64
	bank        string
	card        string
	phone       string
	unitRatio   float64
	personRatio float64
	yesLoan     string
	frozen      string
	onlineGong  string
	onlineShang string
}

// hrFundBillSeedRow 月度账单明细（真实来源：单位汇缴清册回单 PDF）。
type hrFundBillSeedRow struct {
	account  string
	name     string
	idNo     string
	base     float64
	total    float64
	personal float64
	unit     float64
}

// hrSeedFundRecords 用真实网厅 87 条数据初始化公积金台账（幂等）。
//
// 第 55 轮：数据源改为附件《单位职工明细-网厅.xls》，不再使用构造占位账号。
// 迁移口径：先删掉「12 位数字但不在真实账号集合」的旧构造行，再补齐缺失的真实行；
// 已存在的真实行（按证件号码）只更新 18 字段，不重复插入。
func (h *HRHandler) hrSeedFundRecords(ctx context.Context) error {
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Distinct().Pluck("tenant_id", &tids).Error; err != nil {
		return err
	}
	want := map[string]bool{}
	for _, row := range hrWebhallRows {
		want[normalizeIDNumber(row.idNo)] = true
	}
	for _, tid := range tids {
		var exist []HRPFRecord
		if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).
			Find(&exist).Error; err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, r := range exist {
			key := normalizeIDNumber(r.IdentityNumber)
			// 旧构造行（账号是 12 位数字但不在真实清单里）→ 删除，避免 87 条被撑成 174 条
			if !want[key] && (hrLooksLikeFundAccount(r.PersonalAccount) ||
				strings.HasPrefix(r.PersonalAccount, "R45-") || strings.HasPrefix(r.Name, "真机公积金")) {
				_ = h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", r.ID, tid).
					Delete(&HRPFRecord{}).Error
				continue
			}
			seen[key] = true
		}
		ratio := h.hrPFRatio(ctx, tid)
		for _, row := range hrWebhallRows {
			key := normalizeIDNumber(row.idNo)
			if key == "" {
				continue
			}
			rec := hrFundRecordOfSeed(row, tid)
			hrNormalizePFRecord(&rec, ratio)
			hrFundSyncCustom(&rec)
			if seen[key] {
				var cur HRPFRecord
				if err := h.db.WithContext(ctx).
					Where("tenant_id = ? AND identity_number = ?", tid, key).
					First(&cur).Error; err == nil {
					rec.ID = cur.ID
					rec.CreatedAt = cur.CreatedAt
					_ = h.db.WithContext(ctx).Save(&rec).Error
				}
				continue
			}
			if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
				logger.Warnf(ctx, "seed fund record %s failed: %v", row.name, err)
				continue
			}
			seen[key] = true
		}
		// 职工账号实时回写花名册
		accounts := map[string]string{}
		for _, row := range hrWebhallRows {
			accounts[normalizeIDNumber(row.idNo)] = row.account
		}
		if err := h.hrFundSyncRosterAccounts(ctx, tid, accounts); err != nil {
			logger.Warnf(ctx, "sync fund account to roster failed: %v", err)
		}
		logger.Infof(ctx, "公积金网厅真实数据已就绪（tenant=%d，87 条）", tid)
	}
	return nil
}

// hrFundRecordOfSeed 真实网厅行 → HRPFRecord（18 字段全量落列 + custom_data）。
func hrFundRecordOfSeed(row hrFundSeedRow, tid int64) HRPFRecord {
	defaultYesNo := func(v string, fallback string) string {
		if strings.TrimSpace(v) == "" {
			return fallback
		}
		return v
	}
	return HRPFRecord{
		ID: uuid.New().String(), TenantID: tid,
		PersonalAccount: row.account, Name: row.name,
		IdentityNumber: normalizeIDNumber(row.idNo), Status: row.status,
		PersonalBase: row.base, CompanyAmount: row.unit, PersonalAmount: row.person,
		TotalAmount: hrFundRound2(row.unit + row.person),
		CustomData: map[string]any{
			"账户余额(元)":   row.balance,
			"单位缴存比例(%)": row.unitRatio,
			"职工缴存比例(%)": row.personRatio,
			"联名卡银行":     row.bank,
			"联名卡号":      row.card,
			"绑定手机":      row.phone,
			"是否公贷":      defaultYesNo(row.yesLoan, "否"),
			"是否冻结":      defaultYesNo(row.frozen, "否"),
			"是否可在线提取公贷": defaultYesNo(row.onlineGong, "否"),
			"是否可在线提取商贷": defaultYesNo(row.onlineShang, "否"),
		},
	}
}

// hrSeedFundBills 真实来源《2025.05单位汇缴清册回单.pdf》：1 条月度账单 + 18 条明细。
// 幂等：按 (tenant_id, month_label) 唯一；已存在则只更新汇总与明细。
func (h *HRHandler) hrSeedFundBills(ctx context.Context) error {
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Distinct().Pluck("tenant_id", &tids).Error; err != nil {
		return err
	}
	for _, tid := range tids {
		var bill HRPFBill
		err := h.db.WithContext(ctx).
			Where("tenant_id = ? AND month_label = ?", tid, "2025-05").First(&bill).Error
		if err == nil {
			// 已存在：按真实回单重算汇总 + 重建明细（ID 保持不变）
		} else {
			bill = HRPFBill{ID: uuid.New().String(), TenantID: tid}
		}
		bill.MonthLabel = "2025-05"
		bill.RecordCount = len(hrHuijiao202505Rows)
		bill.BillStatus = "已汇缴"
		bill.BaseAmountTotal = 0
		bill.PersonalAmountTotal = 0
		bill.CompanyAmountTotal = 0
		bill.CombinedAmountTotal = 0
		// 部门按花名册证件号码关联（回单里没有部门列）
		var employees []HREmployee
		_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&employees).Error
		deptOf := map[string]string{}
		for _, e := range employees {
			deptOf[normalizeIDNumber(e.IDNumber)] = e.Department
		}
		items := make([]HRPFBillItem, 0, len(hrHuijiao202505Rows))
		for _, row := range hrHuijiao202505Rows {
			bill.BaseAmountTotal += row.base
			bill.PersonalAmountTotal += row.personal
			bill.CompanyAmountTotal += row.unit
			bill.CombinedAmountTotal += row.total
			items = append(items, HRPFBillItem{
				ID: uuid.New().String(), TenantID: tid, BillID: bill.ID,
				PersonalAccount: row.account, Name: row.name,
				IdentityNumber: row.idNo, Department: deptOf[normalizeIDNumber(row.idNo)],
				PersonalBase: row.base, PersonalAmount: row.personal,
				CompanyAmount: row.unit, TotalAmount: row.total,
			})
		}
		if err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// 已存在时先清旧明细（避免重复），再按真实回单写入
			if err := tx.Where("tenant_id = ? AND bill_id = ?", tid, bill.ID).
				Delete(&HRPFBillItem{}).Error; err != nil {
				return err
			}
			if err := tx.Save(&bill).Error; err != nil {
				return err
			}
			return tx.Create(&items).Error
		}); err != nil {
			logger.Warnf(ctx, "seed fund bill 2025-05 failed: %v", err)
		}
	}
	return nil
}

// hrBuildFundSeedRows 由员工表生成 87 行（81 在职 + 6 销户）。
func hrBuildFundSeedRows(employees []HREmployee) []hrFundSeedRow {
	// 指定账号（与网厅真实数据一致）
	fixed := map[string]string{
		"寇江":  "801504241502",
		"李永娇": "801364932880",
		"贺伟":  "801507495053",
	}
	banks := []string{"中国建设银行", "中国银行", "中国工商银行", "中国农业银行", "农村商业银行", "招商银行"}
	// 7 名封存：取工号尾号为 3 的倍数的前 7 人，分布在不同部门
	sealedNames := []string{"谢家芬", "刘正均", "曾均华", "陈洁", "龙远高", "秦克章", "刘开祥"}
	sealed := map[string]bool{}
	for _, n := range sealedNames {
		sealed[n] = true
	}
	rows := make([]hrFundSeedRow, 0, 87)
	used := map[string]bool{}
	seq := 0
	for i, e := range employees {
		if len(rows) >= 81 {
			break
		}
		idNo := normalizeIDNumber(e.IDNumber)
		if idNo == "" || used[idNo] {
			continue
		}
		name := strings.TrimSpace(e.Name)
		acc := fixed[name]
		if acc == "" {
			// 801 + 9 位：确定性地由工号/序号派生，重启不变
			seq++
			acc = fmt.Sprintf("801%09d", 500000000+seq*137+i%97)
		}
		base := float64(4000 + (i%9)*500)
		rate := 0.05
		if i%7 == 0 {
			rate = 0.07
		} else if i%11 == 0 {
			rate = 0.12
		}
		person := hrFundRound2(base * rate)
		unit := person
		balance := hrFundRound2(person * 2 * float64(12+(i%24)))
		status := hrFundStatusActive
		if sealed[name] {
			status = hrFundStatusSealed
		}
		bank := banks[i%len(banks)]
		card := fmt.Sprintf("6217%015d", 300000000000+(i*7919)%999999)
		phone := "138" + fmt.Sprintf("%08d", 10000000+(i*3571)%89999999)
		rows = append(rows, hrFundSeedRow{
			account: acc, name: name, idNo: idNo, status: status,
			base: base, unit: unit, person: person, balance: balance,
			bank: bank, card: card, phone: phone,
		})
		used[idNo] = true
	}
	// 6 名已销户（不在花名册内，网厅仍有记录）
	closed := []struct {
		name, idNo string
	}{
		{"陈天友", "510232196204153319"},
		{"周德芳", "510232196511082245"},
		{"刘文全", "510232195809124412"},
		{"杨光明", "510232196307263354"},
		{"邓淑华", "510232196912085528"},
		{"何远志", "510232197001152216"},
	}
	for i, c := range closed {
		rows = append(rows, hrFundSeedRow{
			account: fmt.Sprintf("801%09d", 600000000+i*7919),
			name:    c.name,
			idNo:    c.idNo,
			status:  hrFundStatusExtracted,
			base:    0,
			unit:    0,
			person:  0,
			balance: 0,
			bank:    banks[i%len(banks)],
			card:    fmt.Sprintf("6217%015d", 400000000000+i*131),
			phone:   fmt.Sprintf("139%08d", 20000000+i*977),
		})
	}
	return rows
}

// hrFundSealTime 封存时间（种子与导入共用）：无则返回 nil。
func hrFundSealTime(status string, now time.Time) *time.Time {
	if hrFundStatusOf(status) == hrFundStatusSealed {
		t := now.AddDate(0, -1, 0)
		return &t
	}
	return nil
}

// hrMigrateFundSubs 把存量「公积金管理」分类的旧 10 字段换成 18 个网厅原生字段。
//
// 为什么必须重建而不是 merge：旧字段名（个人账号 / 姓名 / 身份证号码 / 缴存比例 /
// 合计…）与新契约语义不同（职工账号 / 职工姓名 / 证件号码 / 月缴存额），
// mergeCategorySubs 只增不删，结果就是 28 个字段、表头取到旧列、整列空白。
// 用户自己通过「搜索引用」加的 src_ 字段保留并追加在末尾。
func (h *HRHandler) hrMigrateFundSubs(ctx context.Context) error {
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("scope = ? AND name = ? AND deleted_at IS NULL", hrScopeFund, "公积金管理").
		Find(&cats).Error; err != nil {
		return err
	}
	for i := range cats {
		next := hrFundSeedSubs()
		// 保留用户新增的跨模块引用字段
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
		logger.Infof(ctx, "公积金字段配置已迁移到网厅 18 字段（tenant=%d）", cats[i].TenantID)
	}
	return nil
}

// hrFundSubsEqual 只比字段名与顺序（宽度/默认表头由用户维护，不参与判定）。
func hrFundSubsEqual(a, b []types.FleetCategorySub) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i].Name) != strings.TrimSpace(b[i].Name) {
			return false
		}
	}
	return true
}

// hrMigrateRosterRefSubs 把花名册两个账号字段升级为「只读跨模块引用」。
//
// 存量 subs 是第 52 轮以普通文本字段种下的（没有 source、is_default=false），
// 只改契约不会动已落库的 subs：字段设置里看不到来源徽标、列表默认表头也不显示。
// 这里补 source / 打开默认表头 / 强制启用，幂等。
func (h *HRHandler) hrMigrateRosterRefSubs(ctx context.Context) error {
	refs := map[string]types.FleetCategorySubSource{
		"个人社保账号":  {Category: "社保管理", Field: "个人社保账号", Mode: "field", Label: "个人社保账号"},
		"个人公积金账号": {Category: "公积金管理", Field: "职工账号", Mode: "field", Label: "职工账号"},
	}
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("scope = ? AND name = ? AND deleted_at IS NULL", hrScopeRoster, "花名册").
		Find(&cats).Error; err != nil {
		return err
	}
	for i := range cats {
		changed := false
		for j := range cats[i].Subs {
			src, ok := refs[strings.TrimSpace(cats[i].Subs[j].Name)]
			if !ok {
				continue
			}
			if cats[i].Subs[j].Source == nil || cats[i].Subs[j].Source.Field != src.Field {
				s := src
				cats[i].Subs[j].Source = &s
				changed = true
			}
			if !cats[i].Subs[j].IsDefault {
				cats[i].Subs[j].IsDefault = true
				changed = true
			}
			if !cats[i].Subs[j].Enabled {
				cats[i].Subs[j].Enabled = true
				changed = true
			}
		}
		if !changed {
			continue
		}
		cats[i].UpdatedAt = time.Now()
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			return err
		}
		logger.Infof(ctx, "花名册账号字段已升级为跨模块引用（tenant=%d）", cats[i].TenantID)
	}
	return nil
}
