// 宿舍管理（第 61 轮）：五标签模块的模型、种子、CRUD、住宿办理与月度费用分摊。
//
// 三层模型：房源（宿舍档案 / 公租房档案）→ 床位（房源的床位数）→ 住宿记录。
// 「可用床位数 / 宿舍状态」一律由住宿记录实时派生，绝不落库成静态值，
// 否则办理入住/退宿后会出现「床位显示还有，实际已住满」的脏数据。
//
// 费用信息：按月生成账单，房租 / 物业 / 水 / 电 / 气按每位员工**当月实际入住天数**
// 占比分摊到人，尾差由最后一人吸收（分转元后 round2，保证合计闭合）。
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
)

// ---------------------------------------------------------------------------
// 模型
// ---------------------------------------------------------------------------

// HRDormRecord 宿舍模块的通用记录表（module: dorm / public / stay / asset）。
type HRDormRecord struct {
	ID         string         `gorm:"primaryKey" json:"id"`
	TenantID   int64          `gorm:"index" json:"tenant_id"`
	Module     string         `gorm:"index" json:"module"`
	Code       string         `gorm:"index" json:"code"`
	Name       string         `json:"name"`
	Status     string         `gorm:"index" json:"status"`
	CustomData map[string]any `gorm:"type:jsonb;serializer:json" json:"custom_data"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

func (HRDormRecord) TableName() string { return "hr_dorm_records" }

// HRDormBill 宿舍月度费用账单（每月 1 条，覆盖需二次确认）。
type HRDormBill struct {
	ID            string    `gorm:"primaryKey" json:"id"`
	TenantID      int64     `gorm:"index" json:"tenant_id"`
	MonthLabel    string    `gorm:"index" json:"month_label"`
	RoomCount     int       `json:"room_count"`
	Headcount     int       `json:"headcount"`
	RentTotal     float64   `json:"rent_total"`
	PropertyTotal float64   `json:"property_total"`
	UtilityTotal  float64   `json:"utility_total"`
	GrandTotal    float64   `json:"grand_total"`
	BillStatus    string    `json:"bill_status"`
	CreatedAt     time.Time `json:"created_at"`
}

func (HRDormBill) TableName() string { return "hr_dorm_bills" }

// HRDormBillItem 账单明细：一人一行（含房间汇总口径的原始金额）。
type HRDormBillItem struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	TenantID     int64     `gorm:"index" json:"tenant_id"`
	BillID       string    `gorm:"index" json:"bill_id"`
	MonthLabel   string    `gorm:"index" json:"month_label"`
	RoomCode     string    `gorm:"index" json:"room_code"`
	RoomAddress  string    `json:"room_address"`
	RoomType     string    `json:"room_type"`
	EmployeeName string    `json:"employee_name"`
	Department   string    `json:"department"`
	StayDays     int       `json:"stay_days"`
	Rent         float64   `json:"rent"`
	Property     float64   `json:"property"`
	Water        float64   `json:"water"`
	Elec         float64   `json:"elec"`
	Gas          float64   `json:"gas"`
	Total        float64   `json:"total"`
	Sequence     string    `json:"sequence"`
	CreatedAt    time.Time `json:"created_at"`
}

func (HRDormBillItem) TableName() string { return "hr_dorm_bill_items" }

// HRDormReading 宿舍表计读数（联动能耗管理宿舍水电气表的当月用量与费用）。
//
// 生成账单时按 (month, room) 取数；没有读数的月份该项按 0 计，绝不臆造。
type HRDormReading struct {
	ID        string  `gorm:"primaryKey" json:"id"`
	TenantID  int64   `gorm:"index" json:"tenant_id"`
	MonthLabel string `gorm:"index" json:"month_label"`
	RoomCode  string  `gorm:"index" json:"room_code"`
	ElecKwh   float64 `json:"elec_kwh"`
	ElecFee   float64 `json:"elec_fee"`
	WaterTon  float64 `json:"water_ton"`
	WaterFee  float64 `json:"water_fee"`
	GasM3     float64 `json:"gas_m3"`
	GasFee    float64 `json:"gas_fee"`
}

func (HRDormReading) TableName() string { return "hr_dorm_readings" }

// hrDormAutoMigrate 建表（失败只告警，不阻断启动）。
func hrDormAutoMigrate(db *gorm.DB) {
	if err := db.AutoMigrate(&HRDormRecord{}, &HRDormBill{}, &HRDormBillItem{}, &HRDormReading{}); err != nil {
		logger.Warnf(context.Background(), "AutoMigrate HR dorm tables failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 通用工具
// ---------------------------------------------------------------------------

func hrDormStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func hrDormFloat(m map[string]any, key string) float64 {
	s := hrDormStr(m, key)
	if s == "" {
		return 0
	}
	s = strings.ReplaceAll(s, ",", "")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func hrDormInt(m map[string]any, key string) int {
	return int(hrDormFloat(m, key))
}

func hrDormMonthRange(month string) (time.Time, time.Time, bool) {
	start, err := time.Parse("2006-01", month)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	return start, start.AddDate(0, 1, -1), true
}

func hrDormDateOf(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02", "2006/1/2", "2006/01/02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// hrDormBusyDays 住宿记录在 [from, to] 区间内的实际入住天数（含首尾）。
func hrDormBusyDays(stay HRDormRecord, from, to time.Time) int {
	in, ok := hrDormDateOf(hrDormStr(stay.CustomData, "入住日期"))
	if !ok {
		return 0
	}
	out, hasOut := hrDormDateOf(hrDormStr(stay.CustomData, "退宿日期"))
	start := in
	if start.Before(from) {
		start = from
	}
	end := to
	if hasOut && out.Before(to) {
		end = out
	}
	if end.Before(start) {
		return 0
	}
	return int(end.Sub(start).Hours()/24) + 1
}

// ---------------------------------------------------------------------------
// 派生字段：可用床位数 / 宿舍状态
// ---------------------------------------------------------------------------

// hrDormOccupancy 每个房源的「在住人数」（只统计住宿状态=在住）。
func (h *HRHandler) hrDormOccupancy(ctx context.Context, tid int64) map[string]int {
	out := map[string]int{}
	var stays []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND module = ?", tid, hrDormModuleStay).Find(&stays).Error; err != nil {
		return out
	}
	for _, s := range stays {
		code := hrDormStr(s.CustomData, "宿舍编号/房屋地址")
		if code == "" {
			code = s.Code
		}
		status := hrDormStr(s.CustomData, "住宿状态")
		if status == "已退宿" {
			continue
		}
		out[code]++
	}
	return out
}

// hrDormRoomStatus 空床位数推导四态：维修中 > 满员 > 部分入住 > 空闲。
func hrDormRoomStatus(total, used int, manual string) (string, int) {
	free := total - used
	if free < 0 {
		free = 0
	}
	if strings.TrimSpace(manual) == "维修中" {
		return "维修中", free
	}
	switch {
	case total > 0 && free == 0:
		return "满员", free
	case used > 0:
		return "部分入住", free
	default:
		return "空闲", free
	}
}

// hrDormRecordView 记录出参：补派生列 + 扁平的 custom_data 键（前端按中文列名取值）。
func (h *HRHandler) hrDormRecordView(module string, rec HRDormRecord, occ map[string]int) map[string]any {
	out := map[string]any{
		"id": rec.ID, "module": rec.Module, "code": rec.Code, "name": rec.Name,
		"status": rec.Status, "created_at": rec.CreatedAt, "updated_at": rec.UpdatedAt,
	}
	for k, v := range rec.CustomData {
		out[k] = v
	}
	if module == hrDormModuleRoom || module == hrDormModulePublic {
		code := rec.Code
		if code == "" {
			code = hrDormStr(rec.CustomData, "宿舍编号")
		}
		total := hrDormInt(rec.CustomData, "床位数")
		used := occ[code]
		status, free := hrDormRoomStatus(total, used, hrDormStr(rec.CustomData, "宿舍状态"))
		out["可用床位数"] = free
		out["宿舍状态"] = status
		out["在住人数"] = used
		out["status"] = status
	} else if module == hrDormModuleStay {
		code := hrDormStr(rec.CustomData, "宿舍编号/房屋地址")
		out["住宿状态"] = rec.Status
		if out["住宿状态"] == "" {
			out["住宿状态"] = "在住"
		}
		out["房源码"] = code
	} else if module == hrDormModuleAsset {
		out["资产状态"] = rec.Status
	}
	return out
}

// ---------------------------------------------------------------------------
// 列表 / 统计 / CRUD
// ---------------------------------------------------------------------------

// ListDormRecords GET /hr/dorm/records?module=&q=&month=
func (h *HRHandler) ListDormRecords(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	module := strings.TrimSpace(c.Query("module"))
	if module == "" {
		module = hrDormModuleRoom
	}
	q := strings.TrimSpace(c.Query("q"))
	var rows []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND module = ?", tid, module).
		Order("code asc, created_at asc").Find(&rows).Error; err != nil {
		hrFail(c, "加载宿舍数据失败: "+err.Error())
		return
	}
	occ := h.hrDormOccupancy(ctx, tid)
	items := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		v := h.hrDormRecordView(module, r, occ)
		if q != "" {
			hay := strings.ToLower(fmt.Sprint(v["宿舍编号"]) + " " + fmt.Sprint(v["房屋地址"]) +
				" " + fmt.Sprint(v["员工姓名"]) + " " + fmt.Sprint(v["所属部门"]) +
				" " + fmt.Sprint(v["资产编号"]) + " " + fmt.Sprint(v["资产名称/类型"]) + " " + r.Code + " " + r.Name)
			if !strings.Contains(hay, strings.ToLower(q)) {
				continue
			}
		}
		items = append(items, v)
	}
	hrOK(c, gin.H{"items": items, "total": len(items)})
}

// CreateDormRecord POST /hr/dorm/records
func (h *HRHandler) CreateDormRecord(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req struct {
		Module string         `json:"module"`
		Data   map[string]any `json:"data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	if req.Module == "" {
		hrFail(c, "缺少 module")
		return
	}
	rec := HRDormRecord{
		ID: uuid.New().String(), TenantID: tid, Module: req.Module,
		Code: hrDormCodeOf(req.Module, req.Data), Name: hrDormNameOf(req.Module, req.Data),
		Status: hrDormStatusOf(req.Module, req.Data), CustomData: req.Data,
	}
	if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
		hrFail(c, "新增失败: "+err.Error())
		return
	}
	hrOK(c, h.hrDormRecordView(req.Module, rec, h.hrDormOccupancy(ctx, tid)))
}

// UpdateDormRecord PUT /hr/dorm/records/:id
func (h *HRHandler) UpdateDormRecord(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var row HRDormRecord
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tid, c.Param("id")).First(&row).Error; err != nil {
		hrFail(c, "记录不存在")
		return
	}
	var req struct {
		Data map[string]any `json:"data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	merged := map[string]any{}
	for k, v := range row.CustomData {
		merged[k] = v
	}
	for k, v := range req.Data {
		merged[k] = v
	}
	row.CustomData = merged
	row.Code = hrDormCodeOf(row.Module, merged)
	row.Name = hrDormNameOf(row.Module, merged)
	row.Status = hrDormStatusOf(row.Module, merged)
	if err := h.db.WithContext(ctx).Save(&row).Error; err != nil {
		hrFail(c, "保存失败: "+err.Error())
		return
	}
	hrOK(c, h.hrDormRecordView(row.Module, row, h.hrDormOccupancy(ctx, tid)))
}

// DeleteDormRecord DELETE /hr/dorm/records/:id
func (h *HRHandler) DeleteDormRecord(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	res := h.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tid, c.Param("id")).Delete(&HRDormRecord{})
	if res.Error != nil {
		hrFail(c, "删除失败: "+res.Error.Error())
		return
	}
	hrOK(c, gin.H{"deleted": res.RowsAffected})
}

func hrDormCodeOf(module string, d map[string]any) string {
	switch module {
	case hrDormModuleRoom:
		return hrDormStr(d, "宿舍编号")
	case hrDormModulePublic:
		return hrDormStr(d, "房屋编号")
	case hrDormModuleStay:
		if v := hrDormStr(d, "住宿记录ID"); v != "" {
			return v
		}
		return ""
	case hrDormModuleAsset:
		return hrDormStr(d, "资产编号")
	}
	return ""
}

func hrDormNameOf(module string, d map[string]any) string {
	switch module {
	case hrDormModuleRoom:
		return hrDormStr(d, "宿舍编号")
	case hrDormModulePublic:
		return hrDormStr(d, "房屋地址")
	case hrDormModuleStay:
		return hrDormStr(d, "员工姓名")
	case hrDormModuleAsset:
		return hrDormStr(d, "资产名称/类型")
	}
	return ""
}

func hrDormStatusOf(module string, d map[string]any) string {
	switch module {
	case hrDormModuleRoom, hrDormModulePublic:
		return hrDormStr(d, "宿舍状态")
	case hrDormModuleStay:
		if v := hrDormStr(d, "住宿状态"); v != "" {
			return v
		}
		return "在住"
	case hrDormModuleAsset:
		if v := hrDormStr(d, "资产状态"); v != "" {
			return v
		}
		return "正常"
	}
	return ""
}

// ---------------------------------------------------------------------------
// 住宿办理：调宿 / 退宿（批量）
// ---------------------------------------------------------------------------

// TransferDormStays POST /hr/dorm/stays/transfer {ids, room, bed}
func (h *HRHandler) TransferDormStays(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req struct {
		IDs  []string `json:"ids"`
		Room string   `json:"room"`
		Bed  string   `json:"bed"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 || strings.TrimSpace(req.Room) == "" {
		hrFail(c, "请选择记录并填写目标房源")
		return
	}
	done := 0
	for _, id := range req.IDs {
		var row HRDormRecord
		if err := h.db.WithContext(ctx).Where("tenant_id = ? AND id = ? AND module = ?", tid, id, hrDormModuleStay).
			First(&row).Error; err != nil {
			continue
		}
		d := row.CustomData
		if d == nil {
			d = map[string]any{}
		}
		d["宿舍编号/房屋地址"] = strings.TrimSpace(req.Room)
		if strings.TrimSpace(req.Bed) != "" {
			d["床位号"] = strings.TrimSpace(req.Bed)
		}
		d["入住类型"] = "调宿"
		row.CustomData = d
		row.Code = hrDormCodeOf(row.Module, d)
		row.Status = "在住"
		if err := h.db.WithContext(ctx).Save(&row).Error; err != nil {
			continue
		}
		done++
	}
	hrOK(c, gin.H{"updated": done})
}

// CheckoutDormStays POST /hr/dorm/stays/checkout {ids, date, reason}
func (h *HRHandler) CheckoutDormStays(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req struct {
		IDs    []string `json:"ids"`
		Date   string   `json:"date"`
		Reason string   `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		hrFail(c, "请选择要退宿的记录")
		return
	}
	date := strings.TrimSpace(req.Date)
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	done := 0
	for _, id := range req.IDs {
		var row HRDormRecord
		if err := h.db.WithContext(ctx).Where("tenant_id = ? AND id = ? AND module = ?", tid, id, hrDormModuleStay).
			First(&row).Error; err != nil {
			continue
		}
		d := row.CustomData
		if d == nil {
			d = map[string]any{}
		}
		d["退宿日期"] = date
		if strings.TrimSpace(req.Reason) != "" {
			d["退宿原因"] = strings.TrimSpace(req.Reason)
		}
		d["住宿状态"] = "已退宿"
		row.CustomData = d
		row.Status = "已退宿"
		if err := h.db.WithContext(ctx).Save(&row).Error; err != nil {
			continue
		}
		done++
	}
	hrOK(c, gin.H{"updated": done})
}

// ---------------------------------------------------------------------------
// 费用账单
// ---------------------------------------------------------------------------

// ListDormBills GET /hr/dorm/bills
func (h *HRHandler) ListDormBills(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var bills []HRDormBill
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).
		Order("month_label desc").Find(&bills).Error; err != nil {
		hrFail(c, "加载账单失败: "+err.Error())
		return
	}
	items := make([]map[string]any, 0, len(bills))
	for _, b := range bills {
		items = append(items, hrDormBillView(b))
	}
	hrOK(c, gin.H{"items": items, "total": len(items)})
}

func hrDormBillView(b HRDormBill) map[string]any {
	return map[string]any{
		"id": b.ID, "month_label": b.MonthLabel, "账单周期": b.MonthLabel,
		"room_count": b.RoomCount, "房源数": b.RoomCount,
		"headcount": b.Headcount, "入住人数": b.Headcount,
		"rent_total": b.RentTotal, "房租合计": b.RentTotal,
		"property_total": b.PropertyTotal, "物业合计": b.PropertyTotal,
		"utility_total": b.UtilityTotal, "水电费合计": b.UtilityTotal,
		"grand_total": b.GrandTotal, "费用总计": b.GrandTotal,
		"bill_status": b.BillStatus, "账单状态": b.BillStatus,
		"created_at": b.CreatedAt.Format("2006-01-02 15:04:05"),
		"生成时间":     b.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

func hrDormBillItemView(it HRDormBillItem) map[string]any {
	return map[string]any{
		"id": it.ID, "month_label": it.MonthLabel, "room_code": it.RoomCode,
		"room_address": it.RoomAddress, "room_type": it.RoomType,
		"employee_name": it.EmployeeName, "department": it.Department,
		"stay_days": it.StayDays, "rent": it.Rent, "property": it.Property,
		"water": it.Water, "elec": it.Elec, "gas": it.Gas, "total": it.Total,
		"sequence": it.Sequence,
	}
}

// CreateDormBill POST /hr/dorm/bills {month_label, overwrite}
func (h *HRHandler) CreateDormBill(c *gin.Context) {
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
	if _, _, ok := hrDormMonthRange(month); !ok {
		hrFail(c, "账单月份必须为 YYYY-MM")
		return
	}
	var existing []HRDormBill
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND month_label = ?", tid, month).
		Find(&existing).Error; err != nil {
		hrFail(c, "检查账单失败: "+err.Error())
		return
	}
	if len(existing) > 0 && !req.Overwrite {
		hrFail(c, fmt.Sprintf("「%s」宿舍费用账单已存在，是否覆盖？", month))
		return
	}
	bill, items, err := h.hrBuildDormBill(ctx, tid, month)
	if err != nil {
		hrFail(c, err.Error())
		return
	}
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, old := range existing {
			if e := tx.Where("bill_id = ?", old.ID).Delete(&HRDormBillItem{}).Error; e != nil {
				return e
			}
			if e := tx.Delete(&old).Error; e != nil {
				return e
			}
		}
		if e := tx.Create(&bill).Error; e != nil {
			return e
		}
		if len(items) > 0 {
			return tx.CreateInBatches(items, 50).Error
		}
		return nil
	})
	if err != nil {
		hrFail(c, "生成账单失败: "+err.Error())
		return
	}
	hrOK(c, hrDormBillView(bill))
}

// GetDormBill GET /hr/dorm/bills/:id → {bill, items, rooms, dept_summary}
func (h *HRHandler) GetDormBill(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var bill HRDormBill
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tid, c.Param("id")).First(&bill).Error; err != nil {
		hrFail(c, "账单不存在")
		return
	}
	var items []HRDormBillItem
	if err := h.db.WithContext(ctx).Where("bill_id = ?", bill.ID).Order("sequence asc").Find(&items).Error; err != nil {
		hrFail(c, "加载明细失败: "+err.Error())
		return
	}
	viewItems := make([]map[string]any, 0, len(items))
	rooms := map[string]map[string]any{}
	depts := map[string]map[string]any{}
	order := []string{}
	deptOrder := []string{}
	for _, it := range items {
		viewItems = append(viewItems, hrDormBillItemView(it))
		r := rooms[it.RoomCode]
		if r == nil {
			r = map[string]any{"room_code": it.RoomCode, "room_address": it.RoomAddress,
				"room_type": it.RoomType, "headcount": 0, "rent": 0.0, "property": 0.0,
				"water": 0.0, "elec": 0.0, "gas": 0.0, "total": 0.0}
			rooms[it.RoomCode] = r
			order = append(order, it.RoomCode)
		}
		r["headcount"] = r["headcount"].(int) + 1
		r["rent"] = round2(r["rent"].(float64) + it.Rent)
		r["property"] = round2(r["property"].(float64) + it.Property)
		r["water"] = round2(r["water"].(float64) + it.Water)
		r["elec"] = round2(r["elec"].(float64) + it.Elec)
		r["gas"] = round2(r["gas"].(float64) + it.Gas)
		r["total"] = round2(r["total"].(float64) + it.Total)

		dept := strings.TrimSpace(it.Department)
		if dept == "" {
			dept = "未分配部门"
		}
		d := depts[dept]
		if d == nil {
			d = map[string]any{"department": dept, "headcount": 0, "rent": 0.0, "utility": 0.0, "total": 0.0}
			depts[dept] = d
			deptOrder = append(deptOrder, dept)
		}
		d["headcount"] = d["headcount"].(int) + 1
		d["rent"] = round2(d["rent"].(float64) + it.Rent + it.Property)
		d["utility"] = round2(d["utility"].(float64) + it.Water + it.Elec + it.Gas)
		d["total"] = round2(d["total"].(float64) + it.Total)
	}
	roomRows := make([]map[string]any, 0, len(order))
	for _, k := range order {
		roomRows = append(roomRows, rooms[k])
	}
	deptRows := make([]map[string]any, 0, len(deptOrder))
	for _, k := range deptOrder {
		deptRows = append(deptRows, depts[k])
	}
	sort.SliceStable(deptRows, func(i, j int) bool {
		return fmt.Sprint(deptRows[i]["department"]) < fmt.Sprint(deptRows[j]["department"])
	})
	hrOK(c, gin.H{"bill": hrDormBillView(bill), "items": viewItems,
		"rooms": roomRows, "dept_summary": deptRows})
}

// DeleteDormBill DELETE /hr/dorm/bills/:id
func (h *HRHandler) DeleteDormBill(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var bill HRDormBill
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tid, c.Param("id")).First(&bill).Error; err != nil {
		hrFail(c, "账单不存在")
		return
	}
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Where("bill_id = ?", bill.ID).Delete(&HRDormBillItem{}).Error; e != nil {
			return e
		}
		return tx.Delete(&bill).Error
	})
	if err != nil {
		hrFail(c, "删除失败: "+err.Error())
		return
	}
	hrOK(c, gin.H{"deleted": 1})
}

// hrBuildDormBill 按月聚合：房源租金/物业 + 宿舍表计水电气，按入住天数加权分摊。
func (h *HRHandler) hrBuildDormBill(ctx context.Context, tid int64, month string) (HRDormBill, []HRDormBillItem, error) {
	from, to, ok := hrDormMonthRange(month)
	if !ok {
		return HRDormBill{}, nil, fmt.Errorf("账单月份必须为 YYYY-MM")
	}
	var stays []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND module = ?", tid, hrDormModuleStay).Find(&stays).Error; err != nil {
		return HRDormBill{}, nil, fmt.Errorf("加载住宿记录失败: %w", err)
	}
	// 房源索引：宿舍 + 公租房
	var rooms []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND module IN ?", tid, []string{hrDormModuleRoom, hrDormModulePublic}).
		Find(&rooms).Error; err != nil {
		return HRDormBill{}, nil, fmt.Errorf("加载房源失败: %w", err)
	}
	roomByCode := map[string]HRDormRecord{}
	for _, r := range rooms {
		code := r.Code
		if code == "" {
			code = hrDormStr(r.CustomData, "宿舍编号")
		}
		roomByCode[code] = r
		// 住宿记录里公租房填的是「房屋地址」（两山景苑公租房5栋2-6），
		// 房源主键是短编号（5栋2-6）—— 两种写法都要能命中，否则房租水电全算 0。
		if addr := hrDormStr(r.CustomData, "房屋地址"); addr != "" {
			roomByCode[addr] = r
		}
	}
	readings := map[string]HRDormReading{}
	var rs []HRDormReading
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND month_label = ?", tid, month).Find(&rs).Error; err == nil {
		for _, r := range rs {
			readings[r.RoomCode] = r
		}
	}

	type stayDay struct {
		stay HRDormRecord
		days int
	}
	byRoom := map[string][]stayDay{}
	roomOrder := []string{}
	totalDays := map[string]int{}
	for _, s := range stays {
		if hrDormStr(s.CustomData, "住宿状态") == "已退宿" {
			continue
		}
		code := hrDormStr(s.CustomData, "宿舍编号/房屋地址")
		if code == "" {
			code = s.Code
		}
		if code == "" {
			continue
		}
		d := hrDormBusyDays(s, from, to)
		if d <= 0 {
			continue
		}
		if _, ok := byRoom[code]; !ok {
			roomOrder = append(roomOrder, code)
		}
		byRoom[code] = append(byRoom[code], stayDay{stay: s, days: d})
		totalDays[code] += d
	}
	if len(byRoom) == 0 {
		return HRDormBill{}, nil, fmt.Errorf("该月份暂无住宿记录，无法生成费用账单")
	}

	bill := HRDormBill{
		ID: uuid.New().String(), TenantID: tid, MonthLabel: month,
		BillStatus: "已生成", CreatedAt: time.Now(),
	}
	bill.RoomCount = len(byRoom)
	items := make([]HRDormBillItem, 0)
	seq := 0
	for _, code := range roomOrder {
		group := byRoom[code]
		room := roomByCode[code]
		roomType := "厂区宿舍"
		address := code
		if room.Module == hrDormModulePublic {
			roomType = "公租房"
			if v := hrDormStr(room.CustomData, "房屋地址"); v != "" {
				address = v
			}
		}
		rent := hrDormFloat(room.CustomData, "租金(元/月)")
		property := hrDormFloat(room.CustomData, "物业(元/月)")
		// 读数按房源主键（短编号）索引；住宿记录里公租房填的是地址，必须回落。
		rd := readings[room.Code]
		if rd.RoomCode == "" {
			rd = readings[code]
		}
		// 房间总成本（分转元口径：先按元累加，分摊后 round2，尾差由最后一人吸收）
		roomRent, roomProp := round2(rent), round2(property)
		roomWater, roomElec, roomGas := round2(rd.WaterFee), round2(rd.ElecFee), round2(rd.GasFee)
		roomTotal := round2(roomRent + roomProp + roomWater + roomElec + roomGas)
		sumDays := totalDays[code]
		accRent, accProp := 0.0, 0.0
		accWater, accElec, accGas := 0.0, 0.0, 0.0
		accTotal := 0.0
		for i, sd := range group {
			weight := float64(sd.days) / float64(sumDays)
			last := i == len(group)-1
			var r, p, w, e, g float64
			if last {
				// 尾差闭合：最后一人 = 总额 - 已分出去的部分
				r = round2(roomRent - accRent)
				p = round2(roomProp - accProp)
				w = round2(roomWater - accWater)
				e = round2(roomElec - accElec)
				g = round2(roomGas - accGas)
			} else {
				r = round2(roomRent * weight)
				p = round2(roomProp * weight)
				w = round2(roomWater * weight)
				e = round2(roomElec * weight)
				g = round2(roomGas * weight)
			}
			accRent, accProp = round2(accRent+r), round2(accProp+p)
			accWater, accElec, accGas = round2(accWater+w), round2(accElec+e), round2(accGas+g)
			total := round2(r + p + w + e + g)
			accTotal = round2(accTotal + total)
			seq++
			items = append(items, HRDormBillItem{
				ID: uuid.New().String(), TenantID: tid, BillID: bill.ID, MonthLabel: month,
				RoomCode: code, RoomAddress: address, RoomType: roomType,
				EmployeeName: hrDormStr(sd.stay.CustomData, "员工姓名"),
				Department:   hrDormStr(sd.stay.CustomData, "所属部门"),
				StayDays:     sd.days, Rent: r, Property: p, Water: w, Elec: e, Gas: g,
				Total: total, Sequence: strconv.Itoa(seq), CreatedAt: time.Now(),
			})
		}
		bill.RentTotal = round2(bill.RentTotal + roomRent)
		bill.PropertyTotal = round2(bill.PropertyTotal + roomProp)
		bill.UtilityTotal = round2(bill.UtilityTotal + roomWater + roomElec + roomGas)
		bill.Headcount += len(group)
		_ = roomTotal
	}
	bill.GrandTotal = round2(bill.RentTotal + bill.PropertyTotal + bill.UtilityTotal)
	return bill, items, nil
}

// ---------------------------------------------------------------------------
// 种子数据
// ---------------------------------------------------------------------------

// hrSeedDorm 首次初始化：18 间厂区宿舍 + 10 套公租房 + 住宿/资产/读数 + 2026-10 账单。
//
// 幂等：每类记录只在「该 module 一条都没有」时写入，绝不复活用户删掉的行。
func (h *HRHandler) hrSeedDorm(ctx context.Context) error {
	// 每个有员工的租户各播一份（老库升级 / 演示租户 10000 都要有数据）。
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Distinct().Pluck("tenant_id", &tids).Error; err != nil {
		return err
	}
	for _, tid := range tids {
		if err := h.hrSeedDormForTenant(ctx, tid); err != nil {
			return err
		}
	}
	return nil
}

// hrSeedDormForTenant 单租户播种；已有宿舍档案则跳过（幂等）。
func (h *HRHandler) hrSeedDormForTenant(ctx context.Context, tid int64) error {
	var n int64
	if err := h.db.WithContext(ctx).Model(&HRDormRecord{}).
		Where("tenant_id = ? AND module = ?", tid, hrDormModuleRoom).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	now := time.Now()

	// ---- 18 间厂区宿舍 ----
	dorms := []struct {
		code, building, room, roomType, gender, admin string
		beds, area                                     int
		facility                                       string
	}{
		{"宿舍302", "1号楼", "302", "四人间", "男", "李思远", 4, 28, "空调,热水器,独立卫浴"},
		{"宿舍304", "1号楼", "304", "四人间", "男", "罗佳怡", 4, 28, "空调,热水器,独立卫浴"},
		{"持睿403", "持睿公寓", "403", "双人间", "男", "汪艳", 2, 20, "空调,热水器"},
		{"持睿404", "持睿公寓", "404", "双人间", "男", "王瑞", 2, 20, "空调,热水器"},
		{"持睿405", "持睿公寓", "405", "双人间", "男", "黄洲", 2, 20, "空调,热水器"},
		{"持睿406", "持睿公寓", "406", "双人间", "男", "刘志强", 2, 20, "空调,热水器"},
		{"持睿407", "持睿公寓", "407", "双人间", "女", "徐海燕", 2, 20, "空调,热水器"},
		{"持睿408", "持睿公寓", "408", "双人间", "女", "李永梅", 2, 20, "空调,热水器"},
		{"持睿409", "持睿公寓", "409", "双人间", "女", "暂缺", 2, 20, "空调,热水器"},
		{"持睿410", "持睿公寓", "410", "双人间", "女", "暂缺", 2, 20, "空调,热水器"},
		{"持睿411", "持睿公寓", "411", "双人间", "男", "暂缺", 2, 20, "空调,热水器"},
		{"持睿412", "持睿公寓", "412", "双人间", "男", "暂缺", 2, 20, "空调,热水器"},
		{"持睿413", "持睿公寓", "413", "单人间", "男", "贺伟", 1, 16, "空调,热水器,书桌"},
		{"持睿414", "持睿公寓", "414", "单人间", "女", "暂缺", 1, 16, "空调,热水器,书桌"},
		{"持睿415", "持睿公寓", "415", "单人间", "男", "暂缺", 1, 16, "空调,热水器,书桌"},
		{"持睿416", "持睿公寓", "416", "单人间", "女", "暂缺", 1, 16, "空调,热水器,书桌"},
		{"持睿417", "持睿公寓", "417", "单人间", "男", "暂缺", 1, 16, "空调,热水器,书桌"},
		{"持睿418", "持睿公寓", "418", "单人间", "女", "暂缺", 1, 16, "空调,热水器,书桌"},
	}
	for _, d := range dorms {
		rec := HRDormRecord{
			ID: uuid.New().String(), TenantID: tid, Module: hrDormModuleRoom,
			Code: d.code, Name: d.code, Status: "空闲",
			CustomData: map[string]any{
				"宿舍编号": d.code, "宿舍楼/区域": d.building, "房间号": d.room,
				"房型": d.roomType, "床位数": d.beds, "可用床位数": d.beds,
				"性别限制": d.gender, "房间面积(㎡)": d.area, "配套设施": d.facility,
				"关联电表": d.code, "关联水表": d.code, "所属公司/部门": "重庆星达铜业有限公司",
				"管理员/舍长": d.admin, "宿舍状态": "空闲", "备注": "",
			},
			CreatedAt: now, UpdatedAt: now,
		}
		if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
			return err
		}
	}

	// ---- 10 套两山景苑公租房（图 7 原值）----
	publics := []struct {
		code, addr, name string
		area, rent, prop float64
	}{
		{"5栋2-6", "两山景苑公租房5栋2-6", "5栋2-6", 49.28, 296, 49},
		{"5栋2-7", "两山景苑公租房5栋2-7", "5栋2-7", 50.69, 304, 51},
		{"5栋2-8", "两山景苑公租房5栋2-8", "5栋2-8", 47.95, 288, 48},
		{"5栋21-15", "两山景苑公租房5栋21-15", "5栋21-15", 45.65, 274, 46},
		{"5栋21-16", "两山景苑公租房5栋21-16", "5栋21-16", 46.83, 281, 47},
		{"5栋24-3", "两山景苑公租房5栋24-3", "5栋24-3", 49.28, 296, 49},
		{"5栋24-6", "两山景苑公租房5栋24-6", "5栋24-6", 49.28, 296, 49},
		{"5栋24-7", "两山景苑公租房5栋24-7", "5栋24-7", 50.69, 304, 51},
		{"5栋24-8", "两山景苑公租房5栋24-8", "5栋24-8", 47.95, 288, 48},
		{"5栋24-9", "两山景苑公租房5栋24-9", "5栋24-9", 47.95, 288, 48},
	}
	for _, p := range publics {
		first := round2(p.rent + p.prop)
		rec := HRDormRecord{
			ID: uuid.New().String(), TenantID: tid, Module: hrDormModulePublic,
			Code: p.code, Name: p.addr, Status: "空闲",
			CustomData: map[string]any{
				"房屋编号": p.code, "房屋地址": p.addr, "租赁单位": "重庆星达铜业有限公司",
				"建筑面积(㎡)": p.area, "房型": "双人间", "床位数": 2, "可用床位数": 2,
				"租金(元/月)": p.rent, "物业(元/月)": p.prop,
				"首月费用(元)": first, "季度(元/季)": round2(first * 3),
				"关联电表": p.name, "关联水表": p.name, "关联气表": p.name,
				"管理员/舍长": "行政部", "宿舍状态": "空闲", "备注": "",
			},
			CreatedAt: now, UpdatedAt: now,
		}
		if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
			return err
		}
	}

	// ---- 住宿记录：优先按姓名命中花名册，不足时按在职员工顺序补 ----
	type emp struct {
		Name       string
		Department string
		EmployeeID string
		Phone      string
	}
	var emps []emp
	_ = h.db.WithContext(ctx).Table("hr_employees").
		Select("name, department, employee_id, phone").
		Where("tenant_id = ? AND status = ?", tid, hrRosterStatusActive).
		Order("employee_id asc").Limit(40).Scan(&emps).Error
	pick := func(want string, idx int) emp {
		for _, e := range emps {
			if strings.TrimSpace(e.Name) == want {
				return e
			}
		}
		if idx < len(emps) {
			return emps[idx]
		}
		if len(emps) > 0 {
			return emps[0]
		}
		return emp{Name: want, Department: "生产部"}
	}
	stays := []struct {
		room, bed, in, kind string
		name                string
	}{
		{"宿舍302", "1号床", "2026-03-01", "新入住", "李思远"},
		{"宿舍302", "2号床", "2026-05-12", "新入住", "罗佳怡"},
		{"宿舍304", "1号床", "2026-04-08", "新入住", "汪艳"},
		{"持睿403", "1号床", "2026-06-01", "新入住", "王瑞"},
		{"持睿403", "2号床", "2026-06-01", "新入住", "黄洲"},
		{"持睿404", "1号床", "2026-07-15", "新入住", "刘志强"},
		{"持睿407", "1号床", "2026-08-01", "新入住", "徐海燕"},
		{"两山景苑公租房5栋21-15", "1号床", "2026-02-01", "新入住", "廖宏"},
		{"两山景苑公租房5栋2-6", "1号床", "2026-09-01", "新入住", "曾均华"},
	}
	for i, s := range stays {
		e := pick(s.name, i)
		rec := HRDormRecord{
			ID: uuid.New().String(), TenantID: tid, Module: hrDormModuleStay,
			Code: "", Name: e.Name, Status: "在住",
			CustomData: map[string]any{
				"住宿记录ID": "STAY" + fmt.Sprintf("%04d", i+1),
				"员工工号": e.EmployeeID, "员工姓名": e.Name, "所属部门": e.Department,
				"联系电话": e.Phone, "紧急联系人及电话": "",
				"房源类型": func() string {
					if strings.Contains(s.room, "公租房") {
						return "公租房"
					}
					return "厂区宿舍"
				}(),
				"宿舍编号/房屋地址": s.room, "床位号": s.bed, "入住类型": s.kind,
				"入住日期": s.in, "退宿日期": "", "押金金额(元)": 200,
				"退宿原因": "", "住宿状态": "在住",
			},
			CreatedAt: now, UpdatedAt: now,
		}
		if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
			return err
		}
	}

	// ---- 资产档案 ----
	assets := []struct {
		code, name, room, owner, maintain string
		qty, fee                          int
	}{
		{"ZC-302-01", "床", "宿舍302", "李思远", "无维修", 4, 0},
		{"ZC-302-02", "空调", "宿舍302", "李思远", "已修复", 1, 260},
		{"ZC-304-01", "床", "宿舍304", "汪艳", "无维修", 4, 0},
		{"ZC-304-02", "热水器", "宿舍304", "汪艳", "待维修", 1, 0},
		{"ZC-403-01", "衣柜", "持睿403", "王瑞", "无维修", 2, 0},
		{"ZC-403-02", "洗衣机", "持睿403", "王瑞", "维修中", 1, 180},
		{"ZC-5-21-15-01", "床", "两山景苑公租房5栋21-15", "廖宏", "无维修", 2, 0},
		{"ZC-5-2-6-01", "书桌", "两山景苑公租房5栋2-6", "曾均华", "无维修", 1, 0},
	}
	for _, a := range assets {
		state := "正常"
		if a.maintain == "待维修" || a.maintain == "维修中" {
			state = "损坏"
		}
		rec := HRDormRecord{
			ID: uuid.New().String(), TenantID: tid, Module: hrDormModuleAsset,
			Code: a.code, Name: a.name, Status: state,
			CustomData: map[string]any{
				"资产编号": a.code, "资产名称/类型": a.name, "所属宿舍": a.room,
				"数量": a.qty, "责任人": a.owner, "报修日期": func() string {
					if a.maintain == "无维修" {
						return ""
					}
					return "2026-09-18"
				}(),
				"维修状态": a.maintain, "维修费用(元)": a.fee,
				"维修说明": func() string {
					if a.maintain == "无维修" {
						return ""
					}
					return "已联系维修师傅上门处理"
				}(),
				"资产状态": state,
			},
			CreatedAt: now, UpdatedAt: now,
		}
		if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
			return err
		}
	}

	// ---- 表计读数（2026-10 与 2026-11，联动能耗管理宿舍水电表）----
	for _, month := range []string{"2026-10", "2026-11"} {
		idx := 0
		for _, d := range dorms {
			idx++
			elec := 42 + float64(idx*7%60)
			water := 3.2 + float64(idx*3%9)
			rec := HRDormReading{
				ID: uuid.New().String(), TenantID: tid, MonthLabel: month, RoomCode: d.code,
				ElecKwh: round2(elec), ElecFee: round2(elec * 0.92),
				WaterTon: round2(water), WaterFee: round2(water * 3.5),
				GasM3: 0, GasFee: 0,
			}
			if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
				return err
			}
		}
		idx = 0
		for _, p := range publics {
			idx++
			elec := 88 + float64(idx*11%70)
			water := 5.6 + float64(idx*4%8)
			gas := 11 + float64(idx*3%10)
			rec := HRDormReading{
				ID: uuid.New().String(), TenantID: tid, MonthLabel: month, RoomCode: p.code,
				ElecKwh: round2(elec), ElecFee: round2(elec * 0.92),
				WaterTon: round2(water), WaterFee: round2(water * 3.5),
				GasM3: round2(gas), GasFee: round2(gas * 2.1),
			}
			if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
				return err
			}
		}
	}

	// ---- 预生成 2026-10 账单（费用信息标签首屏有数据可点开）----
	bill, items, err := h.hrBuildDormBill(ctx, tid, "2026-10")
	if err != nil {
		logger.Warnf(ctx, "seed HR dorm bill failed: %v", err)
		return nil
	}
	if err := h.db.WithContext(ctx).Create(&bill).Error; err != nil {
		return err
	}
	if len(items) > 0 {
		return h.db.WithContext(ctx).CreateInBatches(items, 50).Error
	}
	return nil
}

// hrSeedDormIfNeeded 在首次初始化链路里调用（与其它 HR 种子同门禁）。
func (h *HRHandler) hrSeedDormIfNeeded(ctx context.Context) error {
	return h.hrSeedDorm(ctx)
}

// ---------------------------------------------------------------------------
// 导入 / 模板
// ---------------------------------------------------------------------------

// hrDormGroupOfModule module → 胶囊名（导入表头与模板口径）。
func hrDormGroupOfModule(module string) string {
	switch module {
	case hrDormModulePublic:
		return hrDormGroupPublic
	case hrDormModuleStay:
		return hrDormGroupStay
	case hrDormModuleAsset:
		return hrDormGroupAsset
	default:
		return hrDormGroupRoom
	}
}

// hrDormDerivedFields 只读派生列，导入时忽略（不允许用户覆盖计算值）。
var hrDormDerivedFields = map[string]bool{
	"可用床位数": true, "首月费用(元)": true, "季度(元/季)": true, "在住人数": true,
}

// ImportDormRecords POST /hr/dorm/records/import?module=xxx —— 按中文表头导入。
func (h *HRHandler) ImportDormRecords(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	module := strings.TrimSpace(c.Query("module"))
	if module == "" {
		module = strings.TrimSpace(c.PostForm("module"))
	}
	if module == "" {
		module = hrDormModuleRoom
	}
	group := hrDormGroupOfModule(module)
	fields := map[string]bool{}
	for _, d := range hrDormFieldsOf(group) {
		fields[d.Name] = true
	}
	fh, err := c.FormFile("file")
	if err != nil {
		hrFail(c, "缺少上传文件")
		return
	}
	f, err := fh.Open()
	if err != nil {
		hrFail(c, "读取文件失败: "+err.Error())
		return
	}
	defer func() { _ = f.Close() }()
	rows, err := hrReadExcelRows(f)
	if err != nil {
		hrFail(c, "解析 Excel 失败: "+err.Error())
		return
	}
	if len(rows) < 2 {
		hrFail(c, "文件中没有可导入的数据行")
		return
	}
	header := rows[0]
	colOf := map[int]string{}
	for i, hName := range header {
		name := strings.TrimSpace(hName)
		if fields[name] && !hrDormDerivedFields[name] {
			colOf[i] = name
		}
	}
	if len(colOf) == 0 {
		hrFail(c, "表头与「"+group+"」字段不匹配，请先下载模板")
		return
	}
	created := 0
	for _, row := range rows[1:] {
		data := map[string]any{}
		for i, name := range colOf {
			if i >= len(row) {
				continue
			}
			if v := strings.TrimSpace(row[i]); v != "" {
				data[name] = v
			}
		}
		if len(data) == 0 {
			continue
		}
		rec := HRDormRecord{
			ID: uuid.New().String(), TenantID: tid, Module: module,
			Code: hrDormCodeOf(module, data), Name: hrDormNameOf(module, data),
			Status: hrDormStatusOf(module, data), CustomData: data,
		}
		if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
			continue
		}
		created++
	}
	if created == 0 {
		hrFail(c, "没有可导入的数据行")
		return
	}
	hrOK(c, gin.H{"imported": created, "module": module})
}

// DormTemplate GET /hr/dorm/template?module=xxx —— 中文表头模板。
func (h *HRHandler) DormTemplate(c *gin.Context) {
	module := strings.TrimSpace(c.Query("module"))
	if module == "" {
		module = hrDormModuleRoom
	}
	group := hrDormGroupOfModule(module)
	headers := make([]string, 0)
	for _, d := range hrDormFieldsOf(group) {
		if hrDormDerivedFields[d.Name] {
			continue
		}
		headers = append(headers, d.Name)
	}
	hrExcelDownload(c, "宿舍"+group+"模板.xlsx", headers, [][]string{})
}

// DormStats GET /hr/dorm/stats —— KPI 卡片主数字（房源总条数）。
func (h *HRHandler) DormStats(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var dorm, public, stay, asset, bills int64
	h.db.WithContext(ctx).Model(&HRDormRecord{}).Where("tenant_id = ? AND module = ?", tid, hrDormModuleRoom).Count(&dorm)
	h.db.WithContext(ctx).Model(&HRDormRecord{}).Where("tenant_id = ? AND module = ?", tid, hrDormModulePublic).Count(&public)
	h.db.WithContext(ctx).Model(&HRDormRecord{}).Where("tenant_id = ? AND module = ?", tid, hrDormModuleStay).Count(&stay)
	h.db.WithContext(ctx).Model(&HRDormRecord{}).Where("tenant_id = ? AND module = ?", tid, hrDormModuleAsset).Count(&asset)
	h.db.WithContext(ctx).Model(&HRDormBill{}).Where("tenant_id = ?", tid).Count(&bills)
	hrOK(c, gin.H{"dorm": dorm, "public": public, "stay": stay, "asset": asset,
		"bills": bills, "total": dorm + public})
}
