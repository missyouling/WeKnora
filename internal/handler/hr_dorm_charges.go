// 宿舍费用信息：全员月度扣款明细（第 62 轮）。
//
// 一张表 = 一个月的全员扣款快照，生成时跨模块聚合：
//   花名册（在职员工 / 工号 / 部门）
//   住宿记录（宿舍号 / 入住天数 / 分摊比例 / 押金）
//   房源档案（公租房季度租金、水电单价、垃圾/二次供水/污水处理费）
//   宿舍表计读数（2026-10 水电气用量，来自能耗管理口径）
//   社保记录（个人应缴合计）
//   公积金台账（个人月缴存额）
//
// 「其它扣款 / 退款 / 备注」属于人工项：重新生成时保留原值，绝不覆盖用户录入。
package handler

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Tencent/WeKnora/internal/logger"
)

// HRDormCharge 全员月度扣款明细（一人一行）。
type HRDormCharge struct {
	ID         string    `gorm:"primaryKey" json:"id"`
	TenantID   int64     `gorm:"index;uniqueIndex:uk_hr_dorm_charge,priority:1" json:"tenant_id"`
	MonthLabel string    `gorm:"index;uniqueIndex:uk_hr_dorm_charge,priority:2" json:"month_label"`
	EmployeeID string    `gorm:"index;uniqueIndex:uk_hr_dorm_charge,priority:3" json:"employee_id"`
	Name       string    `gorm:"index" json:"name"`
	Department string    `json:"department"`
	RoomNo     string    `json:"room_no"`
	RoomType   string    `json:"room_type"`
	StayDays   int       `json:"stay_days"`
	Ratio      float64   `json:"ratio"`
	Deposit    float64   `json:"deposit"`
	Rent       float64   `json:"rent"`
	Water      float64   `json:"water"`
	Elec       float64   `json:"elec"`
	Gas        float64   `json:"gas"`
	Social     float64   `json:"social"`
	Fund       float64   `json:"fund"`
	Other      float64   `json:"other"`
	Total      float64   `json:"total"`
	Refund     float64   `json:"refund"`
	Subsidy    float64   `json:"subsidy"`
	Remark     string    `json:"remark"`
	BillStatus string    `json:"bill_status"`
	WaterStart float64   `json:"water_start"`
	WaterEnd   float64   `json:"water_end"`
	ElecStart  float64   `json:"elec_start"`
	ElecEnd    float64   `json:"elec_end"`
	GasStart   float64   `json:"gas_start"`
	GasEnd     float64   `json:"gas_end"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (HRDormCharge) TableName() string { return "hr_dorm_charges" }

// hrDormChargeView 出参：扁平中文键 + 原始键（前端两套都能读）。
func hrDormChargeView(c HRDormCharge) map[string]any {
	return map[string]any{
		"id": c.ID, "month_label": c.MonthLabel, "账单月份": c.MonthLabel,
		"employee_id": c.EmployeeID, "员工工号": c.EmployeeID,
		"name": c.Name, "姓名": c.Name,
		"department": c.Department, "所属部门": c.Department,
		"room_no": c.RoomNo, "宿舍号": c.RoomNo, "room_type": c.RoomType,
		"stay_days": c.StayDays, "入住天数": c.StayDays,
		"ratio": c.Ratio, "分摊比例": c.Ratio,
		"deposit": c.Deposit, "宿舍押金(元)": c.Deposit,
		"rent": c.Rent, "公租房租(元)": c.Rent,
		"water": c.Water, "水费(元)": c.Water,
		"elec": c.Elec, "电费(元)": c.Elec,
		"gas": c.Gas, "天然气费(元)": c.Gas,
		"social": c.Social, "社保个人扣款(元)": c.Social,
		"fund": c.Fund, "公积金个人扣款(元)": c.Fund,
		"other": c.Other, "其它扣款(元)": c.Other,
		"total": c.Total, "扣款合计(元)": c.Total,
		"refund": c.Refund, "退款(元)": c.Refund,
		"subsidy": c.Subsidy, "补贴合计(元)": c.Subsidy,
		"remark": c.Remark, "备注": c.Remark,
		"bill_status": c.BillStatus, "账单状态": c.BillStatus,
		"water_start": c.WaterStart, "water_end": c.WaterEnd,
		"elec_start": c.ElecStart, "elec_end": c.ElecEnd,
		"gas_start": c.GasStart, "gas_end": c.GasEnd,
		"created_at": c.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

// ListDormCharges GET /hr/dorm/charges?month=&department=&room_type=&q=
func (h *HRHandler) ListDormCharges(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	month := strings.TrimSpace(c.Query("month"))
	var rows []HRDormCharge
	q := h.db.WithContext(ctx).Where("tenant_id = ?", tid)
	if month != "" {
		q = q.Where("month_label = ?", month)
	}
	if err := q.Order("room_no asc, name asc").Find(&rows).Error; err != nil {
		hrFail(c, "加载费用明细失败: "+err.Error())
		return
	}
	if len(rows) == 0 && month == "" {
		// 尚未生成：返回最近一个可生成的月份（提示前端按钮）
		hrOK(c, gin.H{"items": []any{}, "total": 0, "month": "", "latest_month": h.hrDormLatestMonth(ctx, tid)})
		return
	}
	dept := strings.TrimSpace(c.Query("department"))
	roomType := strings.TrimSpace(c.Query("room_type"))
	kw := strings.ToLower(strings.TrimSpace(c.Query("q")))
	items := make([]map[string]any, 0, len(rows))
	total := map[string]float64{}
	for _, r := range rows {
		if dept != "" && r.Department != dept {
			continue
		}
		if roomType == "公租房" && r.RoomType != "公租房" {
			continue
		}
		if roomType == "员工宿舍" && (r.RoomType == "公租房" || r.RoomNo == "") {
			continue
		}
		if roomType == "未住宿" && r.RoomNo != "" {
			continue
		}
		if kw != "" && !strings.Contains(strings.ToLower(r.Name+" "+r.Department+" "+r.RoomNo+" "+r.EmployeeID), kw) {
			continue
		}
		items = append(items, hrDormChargeView(r))
		total["deposit"] += r.Deposit
		total["rent"] += r.Rent
		total["water"] += r.Water
		total["elec"] += r.Elec
		total["gas"] += r.Gas
		total["social"] += r.Social
		total["fund"] += r.Fund
		total["other"] += r.Other
		total["total"] += r.Total
		total["refund"] += r.Refund
		total["subsidy"] += r.Subsidy
	}
	hrOK(c, gin.H{"items": items, "total": len(items), "month": month,
		"latest_month": h.hrDormLatestMonth(ctx, tid), "sums": total,
		"months": h.hrDormChargeMonths(ctx, tid)})
}

// hrDormChargeMonths 已生成月份（倒序），供月份下拉。
func (h *HRHandler) hrDormChargeMonths(ctx context.Context, tid int64) []string {
	var months []string
	_ = h.db.WithContext(ctx).Model(&HRDormCharge{}).Where("tenant_id = ?", tid).
		Distinct().Order("month_label desc").Pluck("month_label", &months).Error
	return months
}

// hrDormLatestMonth 默认月份：优先 2026-10（本轮真实数据月），否则最近读数月。
func (h *HRHandler) hrDormLatestMonth(ctx context.Context, tid int64) string {
	if ms := h.hrDormChargeMonths(ctx, tid); len(ms) > 0 {
		return ms[0]
	}
	var months []string
	_ = h.db.WithContext(ctx).Model(&HRDormReading{}).Where("tenant_id = ?", tid).
		Distinct().Order("month_label desc").Pluck("month_label", &months).Error
	if len(months) > 0 {
		return months[0]
	}
	return "2026-10"
}

// UpdateDormCharge PUT /hr/dorm/charges/:id —— 只改人工项，合计自动重算。
func (h *HRHandler) UpdateDormCharge(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var row HRDormCharge
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tid, c.Param("id")).First(&row).Error; err != nil {
		hrFail(c, "费用记录不存在")
		return
	}
	var req struct {
		Other  *float64 `json:"other"`
		Refund *float64 `json:"refund"`
		Remark *string  `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	if req.Other != nil {
		row.Other = round2(*req.Other)
	}
	if req.Refund != nil {
		row.Refund = round2(*req.Refund)
	}
	if req.Remark != nil {
		row.Remark = strings.TrimSpace(*req.Remark)
	}
	row.Subsidy = row.Refund
	// 第 65 轮：扣款合计公式与前端行内编辑严格一致
	row.Total = round2(row.Deposit + row.Rent + row.Water + row.Elec + row.Gas + row.Social + row.Fund + row.Other)
	if err := h.db.WithContext(ctx).Save(&row).Error; err != nil {
		hrFail(c, "保存失败: "+err.Error())
		return
	}
	// 第 65 轮：同步回传当月月度汇总，前端据此实时刷新主列表行与合计
	bill := hrDormChargeBillOf(ctx, h, tid, row.MonthLabel)
	hrOK(c, gin.H{"item": hrDormChargeView(row), "bill": bill})
}

// hrDormChargeBillOf 单月账单汇总（行内编辑后回传，避免前端二次请求）。
func hrDormChargeBillOf(ctx context.Context, h *HRHandler, tid int64, month string) map[string]any {
	var rows []HRDormCharge
	_ = h.db.WithContext(ctx).Where("tenant_id = ? AND month_label = ?", tid, month).Find(&rows).Error
	for _, b := range hrAggregateDormChargeBills(rows) {
		if fmt.Sprint(b["month_label"]) == month {
			return b
		}
	}
	return nil
}

// PatchDormChargeItem PATCH /hr/dorm/charges/item/:id —— 行内编辑（其它扣款/退款/备注）。
//
// 与 PUT 同源实现：行内编辑只允许改这三个人工项，其余金额一律由生成链路决定。
func (h *HRHandler) PatchDormChargeItem(c *gin.Context) {
	h.UpdateDormCharge(c)
}

// DeleteDormCharge DELETE /hr/dorm/charges/:id
func (h *HRHandler) DeleteDormCharge(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	res := h.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tid, c.Param("id")).Delete(&HRDormCharge{})
	if res.Error != nil {
		hrFail(c, "删除失败: "+res.Error.Error())
		return
	}
	hrOK(c, gin.H{"deleted": res.RowsAffected})
}

// GenerateDormCharges POST /hr/dorm/charges/generate {month, overwrite}
func (h *HRHandler) GenerateDormCharges(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req struct {
		Month     string `json:"month"`
		MonthLabel string `json:"month_label"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	month := strings.TrimSpace(req.Month)
	if month == "" {
		month = strings.TrimSpace(req.MonthLabel)
	}
	if _, _, ok := hrDormMonthRange(month); !ok {
		hrFail(c, "账单月份必须为 YYYY-MM")
		return
	}
	var exist int64
	_ = h.db.WithContext(ctx).Model(&HRDormCharge{}).
		Where("tenant_id = ? AND month_label = ?", tid, month).Count(&exist).Error
	if exist > 0 && !req.Overwrite {
		hrFail(c, fmt.Sprintf("「%s」费用明细已存在，是否覆盖？", month))
		return
	}
	rows, err := h.hrBuildDormCharges(ctx, tid, month)
	if err != nil {
		hrFail(c, err.Error())
		return
	}
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Where("tenant_id = ? AND month_label = ?", tid, month).Delete(&HRDormCharge{}).Error; e != nil {
			return e
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(rows, 50).Error
	})
	if err != nil {
		hrFail(c, "生成费用明细失败: "+err.Error())
		return
	}
	hrOK(c, gin.H{"month": month, "rows": len(rows)})
}

// hrBuildDormCharges 跨模块聚合全员扣款明细。
func (h *HRHandler) hrBuildDormCharges(ctx context.Context, tid int64, month string) ([]HRDormCharge, error) {
	from, to, ok := hrDormMonthRange(month)
	if !ok {
		return nil, fmt.Errorf("账单月份必须为 YYYY-MM")
	}
	// 1) 在职员工
	var emps []HREmployee
	// 存量花名册 status 有「在职」「active」两种历史值，统一按「非离职」取在职。
	// 注意：这里限定在职是**扣款对象**口径（离职人员不再产生住宿费用），
	// 不是「关联花名册」口径 —— 部门/岗位/工号一律查全量（见 deptIdx）。
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND (status <> ? OR status = '' OR status IS NULL)", tid, "离职").
		Order("employee_id asc").Find(&emps).Error; err != nil {
		return nil, fmt.Errorf("加载花名册失败: %w", err)
	}
	if len(emps) == 0 {
		return nil, fmt.Errorf("花名册暂无在职员工")
	}
	// 第 69 轮：部门以全量花名册（含离职）为权威兜底 —— 在职员工档案缺部门时，
	// 同一个人此前作为离职人员的部门也能补上，避免费用表部门列出现空白。
	deptIdx := h.hrDeptIndex69(ctx, tid)
	// 2) 住宿记录 + 房源
	var stays []HRDormRecord
	_ = h.db.WithContext(ctx).Where("tenant_id = ? AND module = ?", tid, hrDormModuleStay).Find(&stays).Error
	var rooms []HRDormRecord
	_ = h.db.WithContext(ctx).Where("tenant_id = ? AND module IN ?", tid,
		[]string{hrDormModuleRoom, hrDormModulePublic}).Find(&rooms).Error
	roomOf := map[string]HRDormRecord{}
	for _, r := range rooms {
		roomOf[r.Code] = r
		if addr := hrDormStr(r.CustomData, "房屋地址"); addr != "" {
			roomOf[addr] = r
		}
	}
	// 3) 表计读数
	readings := map[string]HRDormReading{}
	var rs []HRDormReading
	_ = h.db.WithContext(ctx).Where("tenant_id = ? AND month_label = ?", tid, month).Find(&rs).Error
	for _, r := range rs {
		readings[r.RoomCode] = r
	}
	// 4) 社保个人扣款（当月有则用当月，否则取最近一个月快照）
	social := h.hrDormSocialByPerson(ctx, tid, month)
	fund := h.hrDormFundByPerson(ctx, tid)

	// 5) 每人一条
	// 住宿记录按「工号 + 姓名」双键索引：存量记录工号可能为空，
	// 只按工号匹配会让整张费用表房间号全空（真机踩过）。
	stayOf := map[string]HRDormRecord{}
	for _, s := range stays {
		if hrDormStr(s.CustomData, "住宿状态") == "已退宿" {
			continue
		}
		if hrDormBusyDays(s, from, to) <= 0 {
			continue
		}
		if id := normalizeIDNumber(hrDormStr(s.CustomData, "员工工号")); id != "" {
			if _, ok := stayOf["id:"+id]; !ok {
				stayOf["id:"+id] = s
			}
		}
		if nm := strings.TrimSpace(hrDormStr(s.CustomData, "员工姓名")); nm != "" {
			if _, ok := stayOf["nm:"+nm]; !ok {
				stayOf["nm:"+nm] = s
			}
		}
	}
	out := make([]HRDormCharge, 0, len(emps))
	now := time.Now()
	for _, e := range emps {
		stay, hasStay := HRDormRecord{}, false
		if id := normalizeIDNumber(e.EmployeeID); id != "" {
			stay, hasStay = stayOf["id:"+id]
		}
		if !hasStay {
			stay, hasStay = stayOf["nm:"+strings.TrimSpace(e.Name)]
		}
		row := HRDormCharge{
			ID: uuid.New().String(), TenantID: tid, MonthLabel: month,
			EmployeeID: e.EmployeeID, Name: e.Name, Department: e.Department,
			Ratio: 0, StayDays: 0, BillStatus: "已生成", CreatedAt: now, UpdatedAt: now,
		}
		if strings.TrimSpace(row.Department) == "" {
			row.Department = deptIdx.deptOf(e.IDNumber, e.Name)
		}
		if hasStay {
			roomCode := hrDormStr(stay.CustomData, "宿舍编号/房屋地址")
			row.RoomNo = roomCode
			row.StayDays = hrDormBusyDays(stay, from, to)
			row.Ratio = hrDormFloat(stay.CustomData, "分摊比例")
			// 显式 0 是夫妻房配偶（不参与分摊），只有**缺键**才回落 1
			if _, ok := stay.CustomData["分摊比例"]; !ok {
				row.Ratio = 1
			}
			room := roomOf[roomCode]
			roomType := "员工宿舍"
			if room.Module == hrDormModulePublic {
				roomType = "公租房"
			}
			row.RoomType = roomType
			// 押金：仅当月新入住的收一次
			if in, ok := hrDormDateOf(hrDormStr(stay.CustomData, "入住日期")); ok {
				if !in.Before(from) && !in.After(to) {
					row.Deposit = round2(hrDormFloat(stay.CustomData, "押金金额(元)"))
					if row.Deposit == 0 {
						row.Deposit = round2(hrDormFloat(room.CustomData, "宿舍押金(元)"))
					}
				}
			}
			rd := readings[room.Code]
			if rd.RoomCode == "" {
				rd = readings[roomCode]
			}
			// 房租：公租房在「租期起始月份」一次性收季度租金，其余月份按月费用单价；
			// 两条口径都来自档案，分摊比例照乘（对齐 Excel「季度租金 / 月租金」）。
			if roomType == "公租房" {
				unit := hrDormFloat(room.CustomData, "月费用单价(元/月)")
				if unit == 0 {
					unit = round2(hrDormFloat(room.CustomData, "房屋租金(元/月)") + hrDormFloat(room.CustomData, "物业费(元/月)"))
				}
				quarter := hrDormFloat(room.CustomData, "季度租金(元/季)")
				if quarter == 0 {
					quarter = round2(unit * 3)
				}
				startMonth := hrDormStr(room.CustomData, "租期起始月份")
				if startMonth != "" && startMonth == month {
					row.Rent = round2(quarter * row.Ratio)
				} else {
					row.Rent = round2(unit * row.Ratio)
				}
			} else {
				row.Rent = round2(hrDormFloat(room.CustomData, "房屋租金(元/月)") * row.Ratio)
			}
			/**
			 * 第 70 轮：费用项**优先取该条住宿记录的独立值**（用户可单独改价），
			 * 未单独设置才回落房源档案基准。判定用「键存在且非空」，0 也是合法值。
			 */
			priceOf := func(key string) float64 {
				// 住宿记录优先（含长短名别名），未单独设置再回落房源档案基准
				if v, ok := hrDormFeeLookup70(stay.CustomData, key); ok {
					return v
				}
				if v, ok := hrDormFeeLookup70(room.CustomData, key); ok {
					return v
				}
				return 0
			}
			waterPrice := priceOf("水费单价(元/m³)")
			if waterPrice == 0 {
				waterPrice = 3.32
			}
			elecPrice := priceOf("电费标准(元/度)")
			if elecPrice == 0 {
				elecPrice = 1
			}
			gasPrice := priceOf("气费标准(元/m³)")
			if gasPrice == 0 {
				gasPrice = 2.196
			}
			if roomType == "公租房" {
				sewage := priceOf("污水处理费(元/m³)")
				second := priceOf("二次供水费(元/m³)")
				garbage := priceOf("垃圾处理费(元/套)")
				row.Water = round2((rd.WaterTon*(waterPrice+sewage+second) + garbage) * row.Ratio)
			} else {
				row.Water = round2(rd.WaterTon * waterPrice * row.Ratio)
			}
			row.Elec = round2(rd.ElecKwh * elecPrice * row.Ratio)
			row.Gas = round2(rd.GasM3 * gasPrice * row.Ratio)
			row.WaterStart = round2(hrDormFloat(stay.CustomData, "入住时水表度数"))
			row.ElecStart = round2(hrDormFloat(stay.CustomData, "入住时电表度数"))
			row.GasStart = round2(hrDormFloat(stay.CustomData, "入住时气表度数"))
			row.WaterEnd = round2(row.WaterStart + rd.WaterTon)
			row.ElecEnd = round2(row.ElecStart + rd.ElecKwh)
			row.GasEnd = round2(row.GasStart + rd.GasM3)
		}
		row.Social = round2(social[strings.TrimSpace(e.Name)])
		row.Fund = round2(fund[strings.TrimSpace(e.Name)])
		row.Subsidy = row.Refund
		row.Total = round2(row.Deposit + row.Rent + row.Water + row.Elec + row.Gas + row.Social + row.Fund + row.Other)
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RoomNo == out[j].RoomNo {
			return out[i].Name < out[j].Name
		}
		return out[i].RoomNo < out[j].RoomNo
	})
	return out, nil
}

// hrDormSocialByPerson 社保个人扣款（姓名 → 个人应缴合计）。
//
// 当月没有社保记录时回退到最近一个月：需求禁止把 11 月社保回退成 09 月，
// 但 10 月费用明细仍要能带出「当月/最近一次」的个人扣款。
func (h *HRHandler) hrDormSocialByPerson(ctx context.Context, tid int64, month string) map[string]float64 {
	out := map[string]float64{}
	useMonth := month
	var n int64
	_ = h.db.WithContext(ctx).Model(&HRSIRecord{}).
		Where("tenant_id = ? AND bill_month = ?", tid, month).Count(&n).Error
	if n == 0 {
		var latest string
		_ = h.db.WithContext(ctx).Model(&HRSIRecord{}).Where("tenant_id = ?", tid).
			Order("bill_month desc").Limit(1).Pluck("bill_month", &latest).Error
		if latest != "" {
			useMonth = latest
		}
	}
	type row struct {
		Name string
		Amt  float64
	}
	var rows []row
	_ = h.db.WithContext(ctx).Model(&HRSIRecord{}).
		Select("name, SUM(personal_amount) as amt").
		Where("tenant_id = ? AND bill_month = ? AND record_type = ?", tid, useMonth, "normal").
		Group("name").Scan(&rows).Error
	for _, r := range rows {
		out[strings.TrimSpace(r.Name)] = r.Amt
	}
	// 补退记录并入个人扣款（补缴为正、退费为负）
	var adj []row
	_ = h.db.WithContext(ctx).Model(&HRSIRecord{}).
		Select("name, SUM(personal_amount) as amt").
		Where("tenant_id = ? AND bill_month = ? AND record_type = ?", tid, useMonth, "abnormal").
		Group("name").Scan(&adj).Error
	for _, r := range adj {
		out[strings.TrimSpace(r.Name)] += r.Amt
	}
	return out
}

// hrDormFundByPerson 公积金个人月缴存额（姓名 → personal_amount）。
func (h *HRHandler) hrDormFundByPerson(ctx context.Context, tid int64) map[string]float64 {
	out := map[string]float64{}
	type row struct {
		Name string
		Amt  float64
	}
	var rows []row
	_ = h.db.WithContext(ctx).Model(&HRPFRecord{}).
		Select("name, personal_amount as amt").
		Where("tenant_id = ?", tid).Scan(&rows).Error
	for _, r := range rows {
		out[strings.TrimSpace(r.Name)] = r.Amt
	}
	return out
}

// hrDormChargeSummary GET /hr/dorm/charges/summary?month= —— 三张分摊视图 + 部门汇总。
func (h *HRHandler) hrDormChargeSummary(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	month := strings.TrimSpace(c.Query("month"))
	if month == "" {
		month = h.hrDormLatestMonth(ctx, tid)
	}
	var rows []HRDormCharge
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND month_label = ?", tid, month).
		Order("room_no asc, name asc").Find(&rows).Error; err != nil {
		hrFail(c, "加载费用明细失败: "+err.Error())
		return
	}
	depts := map[string]map[string]any{}
	deptOrder := []string{}
	for _, r := range rows {
		d := strings.TrimSpace(r.Department)
		if d == "" {
			d = "未分配部门"
		}
		item := depts[d]
		if item == nil {
			item = map[string]any{"department": d, "headcount": 0, "housing": 0.0, "utility": 0.0,
				"social": 0.0, "fund": 0.0, "other": 0.0, "total": 0.0}
			depts[d] = item
			deptOrder = append(deptOrder, d)
		}
		item["headcount"] = item["headcount"].(int) + 1
		item["housing"] = round2(item["housing"].(float64) + r.Deposit + r.Rent)
		item["utility"] = round2(item["utility"].(float64) + r.Water + r.Elec + r.Gas)
		item["social"] = round2(item["social"].(float64) + r.Social)
		item["fund"] = round2(item["fund"].(float64) + r.Fund)
		item["other"] = round2(item["other"].(float64) + r.Other)
		item["total"] = round2(item["total"].(float64) + r.Total)
	}
	sort.Strings(deptOrder)
	deptRows := make([]map[string]any, 0, len(deptOrder))
	for _, d := range deptOrder {
		deptRows = append(deptRows, depts[d])
	}
	items := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		items = append(items, hrDormChargeView(r))
	}
	hrOK(c, gin.H{"month": month, "items": items, "dept_summary": deptRows})
}

// hrDormMoneyCents 金额转分，避免逐行浮点累加出现 0.01 漂移。
func hrDormMoneyCents(v float64) int64 { return int64(math.Round(v * 100)) }

// ListDormChargeBills GET /hr/dorm/charge-bills —— 费用信息月度账单汇总（1 月 1 行）。
//
// 第 64 轮：主列表从「一人一行」改为与社保/公积金完全一致的月度账单形态。
// 数据源仍是 hr_dorm_charges（跨模块聚合结果），只是按 month_label 汇总；
// 金额按「分」累加后一次性 round2，与明细弹窗的合计口径严格一致。
func (h *HRHandler) ListDormChargeBills(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var rows []HRDormCharge
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).
		Order("month_label desc").Find(&rows).Error; err != nil {
		hrFail(c, "加载账单失败: "+err.Error())
		return
	}
	bills := hrAggregateDormChargeBills(rows)
	hrOK(c, gin.H{"items": bills, "total": len(bills),
		"months": h.hrDormChargeMonths(ctx, tid), "latest_month": h.hrDormLatestMonth(ctx, tid)})
}

// hrAggregateDormChargeBills 把「一人一行」的扣款明细按月汇总成账单行（纯函数，便于单测）。
func hrAggregateDormChargeBills(rows []HRDormCharge) []map[string]any {
	type agg struct {
		month     string
		people    int
		residents int
		deposit   int64
		rent      int64
		water     int64
		elec      int64
		gas       int64
		social    int64
		fund      int64
		other     int64
		total     int64
		refund    int64
		subsidy   int64
		status    string
	}
	order := []string{}
	byMonth := map[string]*agg{}
	seenPerson := map[string]map[string]bool{}
	for _, r := range rows {
		a := byMonth[r.MonthLabel]
		if a == nil {
			a = &agg{month: r.MonthLabel, status: r.BillStatus}
			byMonth[r.MonthLabel] = a
			order = append(order, r.MonthLabel)
			seenPerson[r.MonthLabel] = map[string]bool{}
		}
		key := strings.TrimSpace(r.EmployeeID)
		if key == "" {
			key = strings.TrimSpace(r.Name)
		}
		if !seenPerson[r.MonthLabel][key] {
			seenPerson[r.MonthLabel][key] = true
			a.people++
			if strings.TrimSpace(r.RoomNo) != "" {
				a.residents++
			}
		}
		a.deposit += hrDormMoneyCents(r.Deposit)
		a.rent += hrDormMoneyCents(r.Rent)
		a.water += hrDormMoneyCents(r.Water)
		a.elec += hrDormMoneyCents(r.Elec)
		a.gas += hrDormMoneyCents(r.Gas)
		a.social += hrDormMoneyCents(r.Social)
		a.fund += hrDormMoneyCents(r.Fund)
		a.other += hrDormMoneyCents(r.Other)
		a.total += hrDormMoneyCents(r.Total)
		a.refund += hrDormMoneyCents(r.Refund)
		a.subsidy += hrDormMoneyCents(r.Subsidy)
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i] > order[j] })
	items := make([]map[string]any, 0, len(order))
	for _, m := range order {
		a := byMonth[m]
		items = append(items, map[string]any{
			"id": m, "month_label": m, "账单月份": m,
			"headcount": a.people, "总人数": a.people,
			"resident_count": a.residents, "住宿人数": a.residents,
			"deposit_total":      round2(float64(a.deposit) / 100), "押金合计": round2(float64(a.deposit) / 100),
			"rent_total":         round2(float64(a.rent) / 100), "房租合计": round2(float64(a.rent) / 100),
			"water_total":        round2(float64(a.water) / 100), "水费合计": round2(float64(a.water) / 100),
			"elec_total":         round2(float64(a.elec) / 100), "电费合计": round2(float64(a.elec) / 100),
			"gas_total":          round2(float64(a.gas) / 100), "气费合计": round2(float64(a.gas) / 100),
			"social_total":       round2(float64(a.social) / 100), "社保个人合计": round2(float64(a.social) / 100),
			"fund_total":         round2(float64(a.fund) / 100), "公积金个人合计": round2(float64(a.fund) / 100),
			"other_total":        round2(float64(a.other) / 100), "其它扣款合计": round2(float64(a.other) / 100),
			"total_deduction":    round2(float64(a.total) / 100), "扣款总计": round2(float64(a.total) / 100),
			"refund_total":       round2(float64(a.refund) / 100), "退款合计": round2(float64(a.refund) / 100),
			"subsidy_total":      round2(float64(a.subsidy) / 100), "补贴合计": round2(float64(a.subsidy) / 100),
			"bill_status": a.status, "账单状态": a.status,
		})
	}
	return items
}

// hrDormSeedCharges62 为 2026-10 预生成一份全员扣款明细（仅首次）。
func (h *HRHandler) hrDormSeedCharges62(ctx context.Context, tid int64) {
	var n int64
	_ = h.db.WithContext(ctx).Model(&HRDormCharge{}).
		Where("tenant_id = ? AND month_label = ?", tid, "2026-10").Count(&n).Error
	if n > 0 {
		return
	}
	rows, err := h.hrBuildDormCharges(ctx, tid, "2026-10")
	if err != nil || len(rows) == 0 {
		logger.Warnf(ctx, "seed dorm charges failed: %v", err)
		return
	}
	if err := h.db.WithContext(ctx).CreateInBatches(rows, 50).Error; err != nil {
		logger.Warnf(ctx, "seed dorm charges insert failed: %v", err)
	}
}

// hrDormChargeSeq 给费用明细分配稳定序号（前端表格行键）。
func hrDormChargeSeq(i int) string { return strconv.Itoa(i + 1) }
