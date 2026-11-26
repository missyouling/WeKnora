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
	"gorm.io/gorm/clause"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
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
	if err := db.AutoMigrate(&HRDormRecord{}, &HRDormBill{}, &HRDormBillItem{}, &HRDormReading{}, &HRDormCharge{}); err != nil {
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
	/**
	 * 第 72 轮：`空置` 是人为封存 —— 状态恒为「空置」、可用床位恒为 0。
	 * `维修中` 为历史值（迁移已清洗为「空置」），此处等价处理，避免老数据漏网。
	 */
	switch strings.TrimSpace(manual) {
	case "空置", "维修中":
		return "空置", 0
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
		// 公租房住宿记录存的是「房屋地址」，房源主键是短编号 —— 两种写法都要认
		if used == 0 {
			if addr := hrDormStr(rec.CustomData, "房屋地址"); addr != "" {
				used = occ[addr]
			}
		}
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
		/**
		 * 第 70 轮：该条记录独立保存过的费用项优先回显。
		 *
		 * 正常路径下 custom_data 同名键就是最新值；overrides 是「用户单独改过」
		 * 的权威副本，用于跨版本/换房源后仍能正确回显。
		 */
		if ov, ok := rec.CustomData["fee_overrides"].(map[string]any); ok && len(ov) > 0 {
			for k, v := range ov {
				out[k] = v
			}
		}
	} else if module == hrDormModuleAsset {
		out["资产状态"] = rec.Status
	}
	return out
}

/**
 * 住宿记录可独立覆写的费用项。
 *
 * 字段名在库里有**两套写法**：房源档案用带单位的长名（`电费标准(元/度)`），
 * 而住宿记录的 subs 是历史短名（`电费标准`）。两边都要收，否则用户在住宿记录里
 * 改的价存不进 `fee_overrides`，核算仍按房源基准算 —— 表现为「改了不生效」。
 */
var hrDormStayFeeKeys70 = []string{
	// 长名（房源档案口径）
	"宿舍押金(元)", "电费标准(元/度)", "水费单价(元/m³)",
	"污水处理费(元/m³)", "二次供水费(元/m³)", "垃圾处理费(元/套)", "气费标准(元/m³)",
	"房屋租金(元/月)", "月费用单价(元/月)", "季度租金(元/季)", "物业费(元/月)",
	// 短名（住宿记录 subs 口径）
	"押金金额(元)", "电费标准", "水费单价", "污水处理费", "二次供水费",
	"垃圾处理费", "气费标准", "房屋租金", "季度租金", "物业费",
}

/**
 * hrDormFeeAliases70 费用项别名：同一个费用在不同表里的写法。
 *
 * 用于「住宿记录覆写 → 核算取价」的跨键回退：用户在住宿记录里存的是短名，
 * 房源档案里是长名，`priceOf` 必须两套都试，否则优先级判断失效。
 */
var hrDormFeeAliases70 = map[string][]string{
	"宿舍押金(元)":    {"押金金额(元)"},
	"电费标准(元/度)":   {"电费标准"},
	"水费单价(元/m³)":  {"水费单价"},
	"气费标准(元/m³)":  {"气费标准"},
	"污水处理费(元/m³)": {"污水处理费"},
	"二次供水费(元/m³)": {"二次供水费"},
	"垃圾处理费(元/套)":  {"垃圾处理费"},
	"房屋租金(元/月)":   {"房屋租金"},
	"季度租金(元/季)":   {"季度租金"},
	"物业费(元/月)":    {"物业费"},
}

/**
 * hrDormFeeLookup70 在 map 里按「长名 → 短名别名」顺序找第一个非空值。
 *
 * 返回 (值, 是否命中)。**0 是合法值**（免费用），所以判空只看「键存在且非空串」。
 */
func hrDormFeeLookup70(data map[string]any, key string) (float64, bool) {
	if data == nil {
		return 0, false
	}
	try := func(k string) (float64, bool) {
		raw, ok := data[k]
		if !ok {
			return 0, false
		}
		txt := strings.TrimSpace(fmt.Sprint(raw))
		if txt == "" || txt == "<nil>" {
			return 0, false
		}
		return hrDormFloat(data, k), true
	}
	if v, ok := try(key); ok {
		return v, true
	}
	for _, alias := range hrDormFeeAliases70[key] {
		if v, ok := try(alias); ok {
			return v, true
		}
	}
	return 0, false
}

// hrDormStayPersons 某房源当前在住人员（按「入住日期升序 → 创建时间升序」）。
//
// 第 70 轮：舍长默认值、成员派生、下拉候选都基于这一份顺序 ——
// 「最早入住的在住人员」= 列表第一项，退宿后自动顺延到下一位。
func (h *HRHandler) hrDormStayPersons(ctx context.Context, tid int64, roomKey string) []string {
	roomKey = strings.TrimSpace(roomKey)
	if roomKey == "" {
		return nil
	}
	return h.hrDormStayIndex71(ctx, tid)[roomKey]
}

/**
 * hrDormStayIndex71 一次算出「房源码 → 在住人员（按入住日期升序）」索引。
 *
 * 列表 40 间房时，旧实现每行都查一遍住宿记录（40 次全表扫描，实测 4.2s）。
 * 索引化后一次查询即可，宿舍/公租房列表降到百毫秒级。
 */
func (h *HRHandler) hrDormStayIndex71(ctx context.Context, tid int64) map[string][]string {
	var stays []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND module = ?", tid, hrDormModuleStay).
		Find(&stays).Error; err != nil {
		return nil
	}
	type person struct {
		name  string
		room  string
		since string
		order int64
	}
	var list []person
	for _, s := range stays {
		room := strings.TrimSpace(hrDormStr(s.CustomData, "宿舍编号/房屋地址"))
		if room == "" {
			continue
		}
		if hrDormStr(s.CustomData, "住宿状态") == "已退宿" {
			continue
		}
		name := strings.TrimSpace(hrDormStr(s.CustomData, "员工姓名"))
		if name == "" {
			continue
		}
		list = append(list, person{
			name: name, room: room,
			since: hrDormStr(s.CustomData, "入住日期"),
			order: s.CreatedAt.UnixNano(),
		})
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].since != list[j].since {
			return list[i].since < list[j].since
		}
		return list[i].order < list[j].order
	})
	index := map[string][]string{}
	seen := map[string]bool{}
	for _, p := range list {
		key := p.room + "|" + p.name
		if seen[key] {
			continue
		}
		seen[key] = true
		index[p.room] = append(index[p.room], p.name)
	}
	return index
}

// hrDormRecordViewCtx 带上下文的出参：在基础视图上补「舍长派生 / 在住人员 / 成员」。
//
// 舍长口径（第 70 轮）：
//   · custom_data 有显式「管理员/舍长」→ 用它（用户指定优先）；
//   · 为空且**没有**「舍长手动清空」标记 → 默认最早入住的在住人员；
//   · 用户手动清空过（标记=1）→ 保持空，绝不自动填回；
//   · 舍长退宿后：显式值仍指向已退宿的人 → 也顺延到当前最早在住者。
func (h *HRHandler) hrDormRecordViewCtx(ctx context.Context, tid int64, module string,
	rec HRDormRecord, occ map[string]int, stayIndex map[string][]string) map[string]any {
	/**
	 * 第 71 轮：occupancy 由调用方**算一次**传进来。
	 *
	 * 旧实现在函数内部 `h.hrDormOccupancy(ctx, tid)`：列表返回 485 条资产时，
	 * 每行都重新查一遍住宿记录（485 次全表扫描）→ 请求耗时 10s 被浏览器取消，
	 * 真机表现就是「切到资产档案后表格永远空白」。
	 */
	if occ == nil {
		occ = h.hrDormOccupancy(ctx, tid)
	}
	out := h.hrDormRecordView(module, rec, occ)
	if module != hrDormModuleRoom && module != hrDormModulePublic {
		return out
	}
	roomKey := rec.Code
	if roomKey == "" {
		roomKey = hrDormStr(rec.CustomData, "宿舍编号")
	}
	if roomKey == "" {
		roomKey = hrDormStr(rec.CustomData, "房屋地址")
	}
	/**
	 * 第 71 轮：在住人员优先走**调用方传入的索引**（列表一次算好）。
	 * 单条接口（新增/编辑返回）没有索引时再回落实时查询。
	 */
	var persons []string
	if stayIndex != nil {
		persons = stayIndex[strings.TrimSpace(roomKey)]
		/**
		 * 第 72 轮：公租房「房屋编号」是短号（5-2-6）、「房屋地址」是全称
		 * （两山景苑5-2-6），而住宿记录存的是全称。按 code 查不到时依次回退
		 * 地址/编号，否则抽屉里「管理员/舍长」与「成员」永远是空的。
		 */
		if len(persons) == 0 {
			alts := []string{
				hrDormStr(rec.CustomData, "房屋地址"),
				hrDormStr(rec.CustomData, "宿舍编号"),
				hrDormStr(rec.CustomData, "房间号"),
			}
			/**
			 * 公租房的「房屋地址」只有小区名（两山景苑），住宿记录里存的是
			 * 「小区名 + 门牌」（两山景苑5-2-6）—— 拼起来才对得上。
			 */
			if addr := strings.TrimSpace(hrDormStr(rec.CustomData, "房屋地址")); addr != "" {
				for _, noKey := range []string{"房屋编号", "房间号", "宿舍编号"} {
					if no := strings.TrimSpace(hrDormStr(rec.CustomData, noKey)); no != "" {
						alts = append(alts, addr+no)
					}
				}
			}
			for _, alt := range alts {
				alt = strings.TrimSpace(alt)
				if alt == "" {
					continue
				}
				if p := stayIndex[alt]; len(p) > 0 {
					persons = p
					break
				}
			}
		}
	} else {
		persons = h.hrDormStayPersons(ctx, tid, roomKey)
	}
	out["在住人员"] = persons
	out["在住人数"] = len(persons)
	/**
	 * 第 72 轮：状态与可用床位一律以**真实在住人数**重算。
	 *
	 * `occ` 只按住宿记录里的房源码聚合；公租房房源主键是短号（5-2-6）、
	 * 住宿记录存全称（两山景苑5-2-6），靠 occ 取不到 → 明明住着人却显示「空闲」。
	 * persons 已经按多种键回退匹配过，用它重算才是真相。
	 */
	total := hrDormInt(rec.CustomData, "床位数")
	vacantStatus, free := hrDormRoomStatus(total, len(persons), hrDormStr(rec.CustomData, "宿舍状态"))
	out["可用床位数"] = free
	out["宿舍状态"] = vacantStatus
	out["status"] = vacantStatus

	manager := strings.TrimSpace(hrDormStr(rec.CustomData, "管理员/舍长"))
	cleared := strings.TrimSpace(hrDormStr(rec.CustomData, "舍长手动清空")) == "1"
	inRoom := func(name string) bool {
		for _, p := range persons {
			if p == name {
				return true
			}
		}
		return false
	}
	if manager != "" && !inRoom(manager) {
		// 舍长已退宿 / 调宿：顺延到剩余最早入住者（没有剩余则清空）
		manager = ""
	}
	if manager == "" && !cleared && len(persons) > 0 {
		manager = persons[0]
	}
	out["管理员/舍长"] = manager

	others := make([]string, 0, len(persons))
	for _, p := range persons {
		if p == manager {
			continue
		}
		others = append(others, p)
	}
	out["成员"] = strings.Join(others, "、")
	return out
}

// hrDormRoomNoValue 房间号数字值（`宿舍420` → 420），用于自然升序。
func hrDormRoomNoValue(rec HRDormRecord) int {
	raw := hrDormStr(rec.CustomData, "房间号")
	if raw == "" {
		raw = rec.Code
	}
	num := 0
	found := false
	for _, ch := range raw {
		if ch >= '0' && ch <= '9' {
			num = num*10 + int(ch-'0')
			found = true
			continue
		}
		if found {
			break
		}
	}
	return num
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
	// 第 70 轮：宿舍/公租房档案按房间号**数字值**升序（301→320→401→420），
	// 绝不因为编辑保存或字典序把 4 楼排到 3 楼前面。
	if module == hrDormModuleRoom || module == hrDormModulePublic {
		sort.SliceStable(rows, func(i, j int) bool {
			vi, vj := hrDormRoomNoValue(rows[i]), hrDormRoomNoValue(rows[j])
			if vi != vj {
				return vi < vj
			}
			return rows[i].Code < rows[j].Code
		})
	}
	// 第 71 轮：整个列表共用一份 occupancy（避免 N+1）
	occ := h.hrDormOccupancy(ctx, tid)
	// 在住人员索引同样只算一次（40 间房否则要 40 次全表扫描）
	var stayIndex map[string][]string
	if module == hrDormModuleRoom || module == hrDormModulePublic {
		stayIndex = h.hrDormStayIndex71(ctx, tid)
	}
	items := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		v := h.hrDormRecordViewCtx(ctx, tid, module, r, occ, stayIndex)
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
	hrDormNormalizeRecord(req.Module, req.Data)
	// 第 65 轮：保存前强类型校验（金额/整数/月份），脏值一律 400
	if msg := hrValidateDormData65(req.Data); msg != "" {
		hrFail(c, msg)
		return
	}
	// 第 65 轮：安排入住（新增住宿记录）必须先过准入校验 —— 满员 / 空置 一律拒绝
	if req.Module == hrDormModuleStay {
		if err := h.hrDormAdmitCheck(ctx, tid, hrDormStr(req.Data, "宿舍编号/房屋地址"), ""); err != nil {
			hrFail(c, err.Error())
			return
		}
		// 第 72 轮：同一房源同一床位号不得被两条在住记录占用
		if err := h.hrDormValidateBedNo72(ctx, tid, "",
			hrDormStr(req.Data, "宿舍编号/房屋地址"), hrDormStr(req.Data, "床位号")); err != nil {
			hrFail(c, err.Error())
			return
		}
	}
	// 第 72 轮：资产字典名称唯一（共用物品不允许重复建档）
	if req.Module == hrDormModuleAsset {
		if err := h.hrDormValidateAssetName72(ctx, tid, "", req.Data); err != nil {
			hrFail(c, err.Error())
			return
		}
	}
	// 第 72 轮：房档保存 —— 表计全局查重 + 空置误切拦截 + 防缩容
	if req.Module == hrDormModuleRoom || req.Module == hrDormModulePublic {
		if err := h.hrDormValidateMeters72(ctx, tid, req.Module, "", req.Data); err != nil {
			hrFail(c, err.Error())
			return
		}
		if err := h.hrDormValidateRoomMeta72(ctx, tid, "", req.Module, req.Data, req.Data); err != nil {
			hrFail(c, err.Error())
			return
		}
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
	hrOK(c, h.hrDormRecordViewCtx(ctx, tid, req.Module, rec, nil, nil))
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
	oldCode := row.Code
	hrDormNormalizeRecord(row.Module, merged)
	// 第 65 轮：保存前强类型校验（金额/整数/月份），脏值一律 400
	if msg := hrValidateDormData65(merged); msg != "" {
		hrFail(c, msg)
		return
	}
	// 第 65 轮：住宿记录改房源时同样校验准入（排除自身，避免原房重存误判满员）
	if row.Module == hrDormModuleStay {
		newRoom := hrDormStr(merged, "宿舍编号/房屋地址")
		oldRoom := hrDormStr(row.CustomData, "宿舍编号/房屋地址")
		status := hrDormStr(merged, "住宿状态")
		if newRoom != "" && newRoom != oldRoom && status != "已退宿" {
			if err := h.hrDormAdmitCheck(ctx, tid, newRoom, row.ID); err != nil {
				hrFail(c, err.Error())
				return
			}
		}
		// 第 72 轮：床位号唯一（排除自身），退宿记录不占用床位
		if status != "已退宿" {
			if err := h.hrDormValidateBedNo72(ctx, tid, row.ID, newRoom, hrDormStr(merged, "床位号")); err != nil {
				hrFail(c, err.Error())
				return
			}
		}
	}
	// 第 72 轮：资产字典名称唯一（编辑排除自身）
	if row.Module == hrDormModuleAsset {
		if err := h.hrDormValidateAssetName72(ctx, tid, row.ID, merged); err != nil {
			hrFail(c, err.Error())
			return
		}
	}
	// 第 72 轮：房档保存 —— 表计全局查重 + 空置误切拦截 + 防缩容
	if row.Module == hrDormModuleRoom || row.Module == hrDormModulePublic {
		if err := h.hrDormValidateMeters72(ctx, tid, row.Module, row.ID, merged); err != nil {
			hrFail(c, err.Error())
			return
		}
		if err := h.hrDormValidateRoomMeta72(ctx, tid, row.ID, row.Module, merged, req.Data); err != nil {
			hrFail(c, err.Error())
			return
		}
	}
	/**
	 * 第 70 轮：舍长「手动清空」需要被记住。
	 *
	 * 派生规则是「空 = 默认最早入住的在住人员」，若不做标记，用户清空后
	 * 下一次读取又会被自动填回首住人 —— 表现为「清空保存无效」。
	 * 这里只在**用户显式提交该字段且新值为空、原值非空**时打标记；
	 * 一旦重新指定舍长就清掉标记，回到「显式值优先」。
	 */
	/**
	 * 第 70 轮：住宿记录的费用项**独立固化**到 `fee_overrides`。
	 *
	 * 用户在一条住宿记录里改押金/单价，只影响这一条（房源档案绝不被反写）；
	 * 账单核算优先按这份覆盖值，未覆盖的项才回落房源基准。
	 */
	if row.Module == hrDormModuleStay {
		overrides := map[string]any{}
		for _, key := range hrDormStayFeeKeys70 {
			if raw, ok := merged[key]; ok {
				overrides[key] = raw
			}
		}
		if len(overrides) > 0 {
			merged["fee_overrides"] = overrides
		}
	}
	if row.Module == hrDormModuleRoom || row.Module == hrDormModulePublic {
		if _, submitted := req.Data["管理员/舍长"]; submitted {
			newMgr := strings.TrimSpace(hrDormStr(merged, "管理员/舍长"))
			oldMgr := strings.TrimSpace(hrDormStr(row.CustomData, "管理员/舍长"))
			if newMgr == "" && oldMgr != "" {
				merged["舍长手动清空"] = "1"
			}
			if newMgr != "" {
				delete(merged, "舍长手动清空")
			}
		}
	}
	row.CustomData = merged
	row.Code = hrDormCodeOf(row.Module, merged)
	row.Name = hrDormNameOf(row.Module, merged)
	row.Status = hrDormStatusOf(row.Module, merged)
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Save(&row).Error; e != nil {
			return e
		}
		// 第 62 轮：改「宿舍编号 / 房屋编号」时级联更新住宿记录与资产档案
		newCode := row.Code
		if (row.Module == hrDormModuleRoom || row.Module == hrDormModulePublic) && oldCode != "" && newCode != "" && oldCode != newCode {
			if e := hrDormCascadeRoomCode(tx, tid, oldCode, newCode); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		hrFail(c, "保存失败: "+err.Error())
		return
	}
	hrOK(c, h.hrDormRecordViewCtx(ctx, tid, row.Module, row, nil, nil))
}

// DeleteDormRecord DELETE /hr/dorm/records/:id
func (h *HRHandler) DeleteDormRecord(c *gin.Context) {
	h.hrDormDeleteRecord(c, "")
}

// hrDormDeleteRecord 删除宿舍记录；module 非空时校验归属（子表专属路由用），
// 防止 `stays/{id}` 误删到同名 id 的宿舍档案。
func (h *HRHandler) hrDormDeleteRecord(c *gin.Context, module string) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	q := h.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tid, c.Param("id"))
	if module != "" {
		q = q.Where("module = ?", module)
	}
	res := q.Delete(&HRDormRecord{})
	if res.Error != nil {
		hrFail(c, "删除失败: "+res.Error.Error())
		return
	}
	if res.RowsAffected == 0 {
		hrFail(c, "记录不存在或已删除")
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
	// 第 65 轮：调宿目标必须是可住状态（满员 / 维修中 一律拒绝）
	if err := h.hrDormAdmitCheck(ctx, tid, req.Room, ""); err != nil {
		hrFail(c, err.Error())
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
		// 第 62 轮字段契约：房屋租金(元/月) / 物业费(元/月)；旧键保留兼容
		rent := hrDormFloat(room.CustomData, "房屋租金(元/月)")
		if rent == 0 {
			rent = hrDormFloat(room.CustomData, "租金(元/月)")
		}
		property := hrDormFloat(room.CustomData, "物业费(元/月)")
		if property == 0 {
			property = hrDormFloat(room.CustomData, "物业(元/月)")
		}
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

// hrSeedDormForTenant 第 62 轮：一次性装载真实房源与入住档案（19 套公租房 + 22 间宿舍 + 38 条住宿）。
//
// 幂等：写系统标记 hr_dorm_seed_v62；只有首次执行才会清掉第 61 轮的虚拟种子，
// 之后用户在界面上的增删改一律保留，重启不复活、不回退。
func (h *HRHandler) hrSeedDormForTenant(ctx context.Context, tid int64) error {
	var mark types.SystemSetting
	if err := h.db.WithContext(ctx).Where("key = ?", hrDormSeed62Key).First(&mark).Error; err == nil {
		return nil
	}
	// 清掉第 61 轮的虚拟种子（只清宿舍四表，绝不动能耗/社保/公积金/花名册）
	for _, m := range []any{&HRDormRecord{}, &HRDormBill{}, &HRDormBillItem{}, &HRDormReading{}, &HRDormCharge{}} {
		if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).Delete(m).Error; err != nil {
			return err
		}
	}
	now := time.Now()
	emp := h.hrDormEmployeeIndex(ctx, tid)
	deptOf := func(name string) string {
		if e, ok := emp[name]; ok && strings.TrimSpace(e.Department) != "" {
			return strings.TrimSpace(e.Department)
		}
		return "生产部"
	}
	empIDOf := func(name string) string {
		if e, ok := emp[name]; ok {
			return e.EmployeeID
		}
		return ""
	}

	// ---- 22 间厂区员工宿舍（301~320、417、419）----
	type roomSeed struct {
		code, roomType, gender string
		beds                   int
		area                   float64
	}
	dormRooms := make([]roomSeed, 0, 22)
	for _, n := range []int{301, 302, 303, 304, 305, 306, 307, 308, 309, 310, 311, 312, 313, 314, 315, 316, 317, 318, 319, 320, 417, 419} {
		rt := "四人间"
		switch n {
		case 301, 302, 310, 312, 313, 314, 316, 320:
			rt = "单人间"
		case 307, 308, 319:
			rt = "双人间"
		case 304, 309:
			rt = "夫妻房"
		}
		dormRooms = append(dormRooms, roomSeed{code: fmt.Sprintf("宿舍%d", n), roomType: rt, gender: "男", beds: hrDormBedsOfType(rt), area: 28})
	}
	dormRooms[3].gender = "不限" // 宿舍304 夫妻房
	dormRooms[8].gender = "不限" // 宿舍309 夫妻房
	for _, r := range dormRooms {
		rec := HRDormRecord{
			ID: uuid.New().String(), TenantID: tid, Module: hrDormModuleRoom,
			Code: r.code, Name: r.code, Status: "空闲",
			CustomData: map[string]any{
				"宿舍编号": r.code, "宿舍楼/区域": "厂区宿舍", "房间号": strings.TrimPrefix(r.code, "宿舍"),
				"房型": r.roomType, "床位数": r.beds, "可用床位数": r.beds,
				"性别限制": r.gender, "房间面积(㎡)": r.area, "配套设施": "空调,热水器,床,衣柜",
				"所属公司/部门": "重庆星达铜业有限公司", "管理员/舍长": "",
				"宿舍状态": "空闲", "备注": "",
				"关联电表": r.code, "关联水表": r.code, "关联气表": "",
				"宿舍押金(元)": 200, "电费标准(元/度)": 1, "水费单价(元/m³)": 5,
				"垃圾处理费(元/套)": 0, "二次供水费(元/m³)": 0, "污水处理费(元/m³)": 0,
				"气费标准(元/m³)": 0, "房屋租金(元/月)": 0, "物业费(元/月)": 0,
			},
			CreatedAt: now, UpdatedAt: now,
		}
		if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
			return err
		}
	}

	// ---- 19 套真实公租房（两山秀苑 9 + 两山景苑 10）----
	type publicSeed struct {
		code, addr, roomType string
		area, rent, prop     float64
		start, end           string
		occupants            []string
	}
	publics := []publicSeed{
		{"19-23-5", "两山秀苑19-23-5", "单人间", 0, 349, 0, "2026-10", "2026-12", []string{"龙远高"}},
		{"19-23-6", "两山秀苑19-23-6", "单人间", 0, 348, 0, "2026-10", "2026-12", []string{"胡勇"}},
		{"19-23-7", "两山秀苑19-23-7", "单人间", 0, 348, 0, "2026-10", "2026-12", []string{"李德富"}},
		{"19-23-8", "两山秀苑19-23-8", "单人间", 0, 349, 0, "2026-10", "2026-12", []string{"杨见洪"}},
		{"19-23-9", "两山秀苑19-23-9", "双人间", 0, 349, 0, "2026-10", "2026-12", []string{"叶永生", "汤旭"}},
		{"19-23-10", "两山秀苑19-23-10", "单人间", 0, 349, 0, "2026-10", "2026-12", []string{"杨强"}},
		{"19-23-11", "两山秀苑19-23-11", "夫妻房", 0, 364, 0, "2026-10", "2026-12", []string{"陈清", "徐红梅"}},
		{"19-23-12", "两山秀苑19-23-12", "单人间", 0, 344, 0, "2026-10", "2026-12", []string{"杨平波"}},
		{"19-23-14", "两山秀苑19-23-14", "单人间", 0, 349, 0, "2026-10", "2026-12", []string{"陈周华"}},
		{"5-2-6", "两山景苑5-2-6", "双人间", 49.28, 296, 49, "2026-08", "2026-10", []string{"李中凯"}},
		{"5-2-7", "两山景苑5-2-7", "双人间", 50.69, 304, 51, "2026-08", "2026-10", []string{"鲁兆义"}},
		{"5-2-8", "两山景苑5-2-8", "双人间", 47.95, 288, 48, "2026-08", "2026-10", []string{"李金笛"}},
		{"5-21-15", "两山景苑5-21-15", "双人间", 45.65, 274, 46, "2026-08", "2026-10", []string{"黄润州"}},
		{"5-21-16", "两山景苑5-21-16", "双人间", 46.83, 281, 47, "2026-08", "2026-10", []string{"杨交坤"}},
		{"5-24-3", "两山景苑5-24-3", "双人间", 49.28, 296, 49, "2026-08", "2026-10", []string{"王弟浩"}},
		{"5-24-6", "两山景苑5-24-6", "双人间", 49.28, 296, 49, "2026-08", "2026-10", []string{"扈极顺"}},
		{"5-24-7", "两山景苑5-24-7", "双人间", 50.69, 304, 51, "2026-08", "2026-10", []string{"宋和聪"}},
		{"5-24-8", "两山景苑5-24-8", "夫妻房", 47.95, 288, 48, "2026-08", "2026-10", []string{"周通", "向秋霞"}},
		{"5-24-9", "两山景苑5-24-9", "双人间", 47.95, 288, 48, "2026-08", "2026-10", []string{"向伟"}},
	}
	for _, p := range publics {
		unit := round2(p.rent + p.prop)
		rec := HRDormRecord{
			ID: uuid.New().String(), TenantID: tid, Module: hrDormModulePublic,
			Code: p.code, Name: p.addr, Status: "空闲",
			CustomData: map[string]any{
				"房屋编号": p.code, "房屋地址": p.addr, "租赁单位": "重庆星达铜业有限公司",
				"建筑面积(㎡)": p.area, "房型": p.roomType, "床位数": hrDormBedsOfType(p.roomType),
				"可用床位数": hrDormBedsOfType(p.roomType),
				"性别限制": func() string {
					if p.roomType == "夫妻房" {
						return "不限"
					}
					return "男"
				}(),
				"配套设施": "空调,热水器,床,衣柜,灶台",
				"管理员/舍长": "行政部",
				"宿舍状态":  "空闲", "备注": "",
				"关联电表": p.addr, "关联水表": p.addr, "关联气表": p.addr,
				"宿舍押金(元)": 0, "电费标准(元/度)": 0.52, "水费单价(元/m³)": 3.32,
				"垃圾处理费(元/套)": 13, "二次供水费(元/m³)": 0.8, "污水处理费(元/m³)": 1,
				"气费标准(元/m³)": 2.196,
				"房屋租金(元/月)": p.rent, "物业费(元/月)": p.prop,
				"月费用单价(元/月)": unit, "季度租金(元/季)": round2(unit * 3),
				"租期起始月份": p.start, "租期截止月份": p.end,
			},
			CreatedAt: now, UpdatedAt: now,
		}
		if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
			return err
		}
	}

	// ---- 住宿记录：38 名真实在住员工 ----
	type staySeed struct{ room, name, kind, in string; ratio float64 }
	stays := []staySeed{
		{"宿舍301", "吴勇建", "单人间", "2026-01-05", 1},
		{"宿舍302", "唐林波", "单人间", "2026-03-16", 1},
		{"宿舍304", "董友兵", "夫妻房", "2026-02-10", 1},
		{"宿舍304", "吴英", "夫妻房", "2026-02-10", 0},
		{"宿舍307", "邓冬", "双人间", "2026-04-01", 0.5},
		{"宿舍307", "叶茂", "双人间", "2026-04-01", 0.5},
		{"宿舍308", "张华", "双人间", "2026-05-06", 0.5},
		{"宿舍308", "邬康军", "双人间", "2026-05-06", 0.5},
		{"宿舍309", "刘明", "夫妻房", "2026-03-01", 1},
		{"宿舍309", "黄美容", "夫妻房", "2026-03-01", 0},
		{"宿舍310", "刘鹏", "单人间", "2026-06-18", 1},
		{"宿舍312", "安洪琦", "单人间", "2026-02-20", 1},
		{"宿舍313", "刘开祥", "单人间", "2026-07-01", 1},
		{"宿舍314", "驾驶员", "单人间", "2026-01-01", 1},
		{"宿舍316", "舒福林", "单人间", "2026-08-11", 1},
		{"宿舍319", "文豪", "双人间", "2026-04-15", 0.5},
		{"宿舍319", "彭量", "双人间", "2026-04-15", 0.5},
		{"宿舍320", "彭振富", "单人间", "2026-05-20", 1},
	}
	for _, p := range publics {
		for i, name := range p.occupants {
			ratio := 1.0
			if len(p.occupants) > 1 {
				if p.roomType == "夫妻房" {
					if i > 0 {
						ratio = 0
					}
				} else {
					ratio = 0.5
				}
			}
			in := "2026-01-01"
			if strings.HasPrefix(p.start, "2026-10") {
				in = "2026-10-01"
			} else {
				in = p.start + "-01"
			}
			stays = append(stays, staySeed{p.addr, name, p.roomType, in, ratio})
		}
	}
	for i, st := range stays {
		rec := HRDormRecord{
			ID: uuid.New().String(), TenantID: tid, Module: hrDormModuleStay,
			Code: "", Name: st.name, Status: "在住",
			CustomData: map[string]any{
				"住宿记录ID": fmt.Sprintf("STAY%04d", i+1),
				"员工工号":   empIDOf(st.name), "员工姓名": st.name, "所属部门": deptOf(st.name),
				"联系电话": "", "紧急联系人及电话": "",
				"房源类型": func() string {
					if strings.Contains(st.room, "两山") {
						return "公租房"
					}
					return "公司宿舍"
				}(),
				"宿舍编号/房屋地址": st.room, "床位号": "1号床", "入住类型": "新入住",
				"入住日期": st.in, "退宿日期": "", "押金金额(元)": 0,
				"退宿原因": "", "住宿状态": "在住",
				"入住时水表度数": 0, "入住时电表度数": 0, "入住时气表度数": 0,
				"分摊比例": st.ratio,
			},
			CreatedAt: now, UpdatedAt: now,
		}
		if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
			return err
		}
	}

	// ---- 资产档案（按新字段：分类/规格/单位/价值/购置日期）----
	// 第 65 轮：资产档案不再有维修字段，种子结构去掉 fee 列
	assets := []struct {
		code, name, category, spec, room, owner, unit string
		qty                                           int
		value                                         float64
	}{
		{"ZC-301-01", "床", "家具", "1.2m 实木", "宿舍301", "吴勇建", "张", 1, 800},
		{"ZC-301-02", "空调", "家电", "1.5P 挂机", "宿舍301", "吴勇建", "台", 1, 2200},
		{"ZC-302-01", "热水器", "家电", "60L", "宿舍302", "唐林波", "台", 1, 900},
		{"ZC-304-01", "衣柜", "家具", "两门", "宿舍304", "董友兵", "个", 1, 600},
		{"ZC-307-01", "洗衣机", "家电", "8kg", "宿舍307", "邓冬", "台", 1, 1500},
		{"ZC-309-01", "书桌", "家具", "1.2m", "宿舍309", "刘明", "张", 1, 350},
		{"ZC-5-21-15-01", "床", "家具", "1.5m", "两山景苑5-21-15", "黄润州", "张", 1, 900},
		{"ZC-19-23-5-01", "灶台", "家电", "双灶", "两山秀苑19-23-5", "龙远高", "台", 1, 700},
	}
	for _, a := range assets {
		rec := HRDormRecord{
			ID: uuid.New().String(), TenantID: tid, Module: hrDormModuleAsset,
			Code: a.code, Name: a.name, Status: "正常",
			CustomData: map[string]any{
				"资产编号": a.code, "资产名称": a.name, "资产分类": a.category, "规格型号": a.spec,
				"所属宿舍": a.room, "数量": a.qty, "计量单位": a.unit, "资产价值(元)": a.value,
				"购置日期": "2025-06-01", "责任人": a.owner,
				// 第 65 轮：资产档案不再有维修字段，种子只写业务字段
				"备注": "", "资产状态": "正常",
			},
			CreatedAt: now, UpdatedAt: now,
		}
		if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
			return err
		}
	}

	// ---- 2026-10 表计读数（宿舍按 1 元/度、5 元/吨；公租房气表有数）----
	idx := 0
	for _, r := range dormRooms {
		idx++
		elec := 60 + float64(idx*13%180)
		water := 2 + float64(idx*3%12)
		rec := HRDormReading{
			ID: uuid.New().String(), TenantID: tid, MonthLabel: "2026-10", RoomCode: r.code,
			ElecKwh: round2(elec), ElecFee: round2(elec * 1),
			WaterTon: round2(water), WaterFee: round2(water * 5),
		}
		if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
			return err
		}
	}
	idx = 0
	for _, p := range publics {
		idx++
		elec := 70 + float64(idx*9%90)
		water := 4 + float64(idx*2%8)
		gas := 6 + float64(idx*3%10)
		rec := HRDormReading{
			ID: uuid.New().String(), TenantID: tid, MonthLabel: "2026-10", RoomCode: p.code,
			ElecKwh: round2(elec), ElecFee: round2(elec * 0.52),
			WaterTon: round2(water), WaterFee: round2(water * 3.32),
			GasM3: round2(gas), GasFee: round2(gas * 2.196),
		}
		if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
			return err
		}
	}

	// ---- 2026-10 全员费用扣款明细 ----
	h.hrDormSeedCharges62(ctx, tid)

	// 写标记（幂等）
	value := []byte("true")
	_ = h.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "value_type", "updated_at"}),
	}).Create(&types.SystemSetting{
		Key: hrDormSeed62Key, Value: types.JSON(value), ValueType: "bool",
		Category: "hr", Description: "宿舍管理第 62 轮真实房源种子已完成",
	}).Error
	logger.Infof(ctx, "HR dorm v62 seeded: 22 dorms / 19 publics / %d stays", len(stays))
	return nil
}

// hrDormBedsOfType 房型 → 床位数（只读派生，前后端同源）。
func hrDormBedsOfType(roomType string) int {
	switch strings.TrimSpace(roomType) {
	case "单人间":
		return 1
	case "双人间", "夫妻房":
		return 2
	case "四人间":
		return 4
	case "六人间":
		return 6
	case "八人间":
		return 8
	}
	return 1
}

// hrDormEmployeeIndex 花名册索引（姓名 → 员工），**含离职人员**。
//
// 第 69 轮：旧实现带 `status <> '离职'`，导致离职员工的住宿/资产/费用记录
// 关联不到部门、岗位、工号 —— 而他们的档案其实完整保留在花名册里。
// 现在查全量；同名时优先在职（同名的离职记录不应覆盖在职员工）。
func (h *HRHandler) hrDormEmployeeIndex(ctx context.Context, tid int64) map[string]HREmployee {
	out := map[string]HREmployee{}
	var rows []HREmployee
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&rows).Error
	for _, e := range rows {
		name := strings.TrimSpace(e.Name)
		if name == "" {
			continue
		}
		prev, exists := out[name]
		if !exists {
			out[name] = e
			continue
		}
		if hrEmployeeResigned(prev) && !hrEmployeeResigned(e) {
			out[name] = e
		}
	}
	return out
}

// hrEmployeeResigned 是否为离职状态（兼容「离职 / resigned / inactive」历史值）。
func hrEmployeeResigned(e HREmployee) bool {
	switch strings.TrimSpace(e.Status) {
	case "离职", "resigned", "inactive", "已离职":
		return true
	}
	return false
}

const hrDormSeed62Key = "hr_dorm_seed_v62"

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
	// 第 62 轮：与花名册一致的三种导入模式
	//   merge  = 合并更新（按房源编号/姓名身份命中则覆盖，未命中则新增）
	//   insert = 仅新增（已存在的行跳过）
	//   update = 仅更新（不新增，只覆盖已存在的行）
	mode := strings.TrimSpace(c.PostForm("mode"))
	if mode == "" {
		mode = strings.TrimSpace(c.Query("mode"))
	}
	if mode != "insert" && mode != "update" {
		mode = "merge"
	}
	existing := map[string]HRDormRecord{}
	var olds []HRDormRecord
	_ = h.db.WithContext(ctx).Where("tenant_id = ? AND module = ?", tid, module).Find(&olds).Error
	for _, o := range olds {
		if k := hrDormIdentityOf(module, o.CustomData, o.Code, o.Name); k != "" {
			existing[k] = o
		}
	}
	created, updated, skipped := 0, 0, 0
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
		hrDormNormalizeRecord(module, data)
		key := hrDormIdentityOf(module, data, hrDormCodeOf(module, data), hrDormNameOf(module, data))
		if old, ok := existing[key]; ok && key != "" {
			if mode == "insert" {
				skipped++
				continue
			}
			merged := map[string]any{}
			for k, v := range old.CustomData {
				merged[k] = v
			}
			for k, v := range data {
				merged[k] = v
			}
			hrDormNormalizeRecord(module, merged)
			old.CustomData = merged
			old.Code = hrDormCodeOf(module, merged)
			old.Name = hrDormNameOf(module, merged)
			old.Status = hrDormStatusOf(module, merged)
			if err := h.db.WithContext(ctx).Save(&old).Error; err != nil {
				skipped++
				continue
			}
			updated++
			continue
		}
		if mode == "update" {
			skipped++
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
	if created == 0 && updated == 0 {
		hrFail(c, "没有可导入的数据行（跳过 "+fmt.Sprint(skipped)+" 行）")
		return
	}
	hrOK(c, gin.H{"imported": created + updated, "created": created, "updated": updated,
		"skipped": skipped, "module": module, "mode": mode})
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

// hrDormNormalizeRecord 保存前归一：
//   · 房型 → 床位数（只读派生）＋ 夫妻房 → 性别限制=不限；
//   · 公租房 月费用单价 = 租金 + 物业，季度租金 = 月费用单价 × 3；
//   · 资产档案：资产编号为空时自动生成，数量/价值转数值。
func hrDormNormalizeRecord(module string, d map[string]any) {
	if d == nil {
		return
	}
	if module == hrDormModuleRoom || module == hrDormModulePublic {
		// 第 63 轮：新增宿舍默认「1号楼 / 双人间」，缺值才补，不改用户已填内容
		if module == hrDormModuleRoom && hrDormStr(d, "宿舍楼/区域") == "" {
			d["宿舍楼/区域"] = "1号楼"
		}
		if hrDormStr(d, "房型") == "" && module == hrDormModuleRoom {
			d["房型"] = "双人间"
		}
		if rt := hrDormStr(d, "房型"); rt != "" {
			d["床位数"] = hrDormBedsOfType(rt)
			if rt == "夫妻房" {
				d["性别限制"] = "不限"
			}
		}
		if module == hrDormModulePublic {
			unit := round2(hrDormFloat(d, "房屋租金(元/月)") + hrDormFloat(d, "物业费(元/月)"))
			d["月费用单价(元/月)"] = unit
			d["季度租金(元/季)"] = round2(unit * 3)
		}
	}
	if module == hrDormModuleAsset {
		if hrDormStr(d, "资产编号") == "" {
			d["资产编号"] = "ZC-" + time.Now().Format("060102150405")
		}
	}
}

// hrDormCascadeRoomCode 房源编号改名 → 同步关联住宿记录与资产档案。
func hrDormCascadeRoomCode(tx *gorm.DB, tid int64, oldCode, newCode string) error {
	var stays []HRDormRecord
	if err := tx.Where("tenant_id = ? AND module = ?", tid, hrDormModuleStay).Find(&stays).Error; err != nil {
		return err
	}
	for i := range stays {
		if hrDormStr(stays[i].CustomData, "宿舍编号/房屋地址") != oldCode {
			continue
		}
		stays[i].CustomData["宿舍编号/房屋地址"] = newCode
		if err := tx.Save(&stays[i]).Error; err != nil {
			return err
		}
	}
	var assets []HRDormRecord
	if err := tx.Where("tenant_id = ? AND module = ?", tid, hrDormModuleAsset).Find(&assets).Error; err != nil {
		return err
	}
	for i := range assets {
		if hrDormStr(assets[i].CustomData, "所属宿舍") != oldCode {
			continue
		}
		assets[i].CustomData["所属宿舍"] = newCode
		if err := tx.Save(&assets[i]).Error; err != nil {
			return err
		}
	}
	return nil
}

// hrDormIdentityOf 导入/合并的身份键：房源=编号/地址，住宿=员工姓名，资产=资产编号。
func hrDormIdentityOf(module string, d map[string]any, code, name string) string {
	switch module {
	case hrDormModuleRoom:
		if v := strings.TrimSpace(hrDormStr(d, "宿舍编号")); v != "" {
			return "room:" + v
		}
		return "room:" + code
	case hrDormModulePublic:
		if v := strings.TrimSpace(hrDormStr(d, "房屋地址")); v != "" {
			return "public:" + v
		}
		if v := strings.TrimSpace(hrDormStr(d, "房屋编号")); v != "" {
			return "public:" + v
		}
		return "public:" + code
	case hrDormModuleStay:
		return "stay:" + strings.TrimSpace(hrDormStr(d, "员工姓名")) + "@" + strings.TrimSpace(hrDormStr(d, "宿舍编号/房屋地址"))
	case hrDormModuleAsset:
		if v := strings.TrimSpace(hrDormStr(d, "资产编号")); v != "" {
			return "asset:" + v
		}
		return "asset:" + name
	}
	return ""
}

// DeleteDormRoom / DeleteDormStay / DeleteDormAsset 子表专属删除路由（第 62 轮）。
//
// 前端按子标签调用 `rooms/{id}` / `stays/{id}` / `assets/{id}`，语义清楚，
// 也避免历史上"删宿舍落到账单接口 → 报账单不存在"的误映射。
func (h *HRHandler) DeleteDormTyped(c *gin.Context) {
	module := strings.TrimSpace(c.Param("module"))
	switch module {
	case "rooms":
		module = hrDormModuleRoom
	case "stays":
		module = hrDormModuleStay
	case "assets":
		module = hrDormModuleAsset
	case "publics":
		module = hrDormModulePublic
	default:
		hrFail(c, "未知的宿舍子表")
		return
	}
	h.hrDormDeleteRecord(c, module)
}
