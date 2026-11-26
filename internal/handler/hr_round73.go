package handler

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
)

// ---------------------------------------------------------------------------
// 第 73 轮：全字段模糊搜索
// ---------------------------------------------------------------------------

/**
 * hrMatchAllFields73 对一条记录的全部业务字段做不区分大小写的模糊匹配。
 *
 * 为什么需要：各列表过去只按 2~3 个硬编码字段过滤（宿舍只看「宿舍编号/房屋地址/
 * 员工姓名/所属部门」），于是搜「星达铜业」「持睿」「1号楼」「E001」全部 0 结果
 * —— 这些值明明就在同一行的 `所属公司` / `宿舍楼/区域` / `宿舍电表号` 里。
 *
 * 口径：
 *   · 只匹配**值**（也匹配中文键名，方便「设施」这类按字段名找）；
 *   · 跳过 id / 时间戳 / 内部状态这类噪声字段，避免搜「2」命中所有行；
 *   · 数组/对象用 fmt.Sprint 展开（配套设施是「、」分隔串，天然可搜）。
 */
func hrMatchAllFields73(q string, record map[string]any) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return true
	}
	for k, v := range record {
		switch k {
		case "id", "created_at", "updated_at", "deleted_at", "module", "status", "page":
			continue
		}
		if v == nil {
			continue
		}
		if strings.Contains(strings.ToLower(fmt.Sprint(v)), q) {
			return true
		}
		if strings.Contains(strings.ToLower(k), q) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// 第 73 轮：能耗历史记录的表名 / 表号与房源档案对齐
// ---------------------------------------------------------------------------

// hrMeterAliasOf73 房源档案 → 该房源定义的表计（表名, 表号, 介质）。
type hrMeterAliasOf73 struct {
	Name string
	No   string
}

/**
 * hrEnergyMeterAliases73 汇总房源档案里定义的全部表计（唯一真相源）。
 *
 * key 用小写表号，值 = 档案里的表名（如 `宿舍301`、`持睿403`、`公租房5-2-6`）。
 */
func (h *HRHandler) hrEnergyMeterAliases73(ctx context.Context, tid int64) map[string]hrMeterAliasOf73 {
	out := map[string]hrMeterAliasOf73{}
	var rooms []HRDormRecord
	_ = h.db.WithContext(ctx).
		Where("tenant_id = ? AND module IN ?", tid,
			[]string{hrDormModuleRoom, hrDormModulePublic}).Find(&rooms).Error
	add := func(name, no string) {
		name = strings.TrimSpace(name)
		no = strings.TrimSpace(no)
		if no == "" {
			return
		}
		out[strings.ToLower(no)] = hrMeterAliasOf73{Name: name, No: no}
	}
	for i := range rooms {
		room := &rooms[i]
		add(hrDormStr(room.CustomData, "宿舍电表名称"), hrDormStr(room.CustomData, "宿舍电表号"))
		add(hrDormStr(room.CustomData, "宿舍水表名称"), hrDormStr(room.CustomData, "宿舍水表号"))
		add(hrDormStr(room.CustomData, "公租房水表名称"), hrDormStr(room.CustomData, "公租房水表号"))
		add(hrDormStr(room.CustomData, "公租房气表名称"), hrDormStr(room.CustomData, "公租房气表号"))
	}
	return out
}

/**
 * hrSyncEnergyHistoryMeterNames73 用房源档案的表名/表号，校正能耗表计与抄表明细的展示名。
 *
 * **红线**：只写 `utility_meters.alias` 与 `utility_meter_items.meter_name`。
 * 起度/止度/用量/单价/金额/倍率/抄表日期/账单月份一律不碰 ——
 * 迁移前后必须校验 123/151/86 条与三笔总额完全一致。
 */
func (h *HRHandler) hrSyncEnergyHistoryMeterNames73(ctx context.Context, tid int64) (meters int, items int) {
	var rooms []HRDormRecord
	_ = h.db.WithContext(ctx).
		Where("tenant_id = ? AND module IN ?", tid,
			[]string{hrDormModuleRoom, hrDormModulePublic}).Find(&rooms).Error
	if len(rooms) == 0 {
		return 0, 0
	}
	type meterRow struct {
		ID       string
		Category string
		MeterNo  string
		Alias    string
	}
	var all []meterRow
	if err := h.db.WithContext(ctx).Raw(
		`SELECT id, category, meter_no, alias FROM utility_meters WHERE tenant_id = ? AND deleted_at IS NULL`, tid).
		Scan(&all).Error; err != nil {
		return 0, 0
	}
	aliasOfMeter := map[string]string{} // meter_id → 档案表名
	rooms0 := map[string]map[string]any{}

	/**
	 * 按「房源 × 介质」逐个对齐，而不是先建全局 name→表计 索引。
	 *
	 * 索引法在 alias 相同（宿舍301 同时有电表和水表）时极易串台：
	 * 上一次迁移就把水表号写进了电表、电表号写进了水表。
	 * 这里显式限定 category，且**只认本介质自己的表号**，天然不会互相污染。
	 */
	apply := func(roomKey, category, nameKey, noKey string) {
		roomKey = strings.TrimSpace(roomKey)
		if roomKey == "" {
			return
		}
		name := strings.TrimSpace(hrDormStr(rooms0[roomKey], nameKey))
		no := strings.TrimSpace(hrDormStr(rooms0[roomKey], noKey))
		if no == "" {
			return
		}
		if name == "" {
			name = roomKey
		}
		for i := range all {
			m := &all[i]
			if m.Category != category {
				continue
			}
			// 只认「别名等于房源码」的表计：这就是历史表计的身份锚点
			if strings.TrimSpace(m.Alias) != roomKey {
				continue
			}
			aliasOfMeter[m.ID] = name
			upd := map[string]any{}
			if strings.TrimSpace(m.Alias) != name {
				upd["alias"] = name
			}
			if strings.TrimSpace(m.MeterNo) != no {
				upd["meter_no"] = no
			}
			if len(upd) == 0 {
				continue
			}
			if err := h.db.WithContext(ctx).Table("utility_meters").
				Where("id = ?", m.ID).Updates(upd).Error; err == nil {
				meters++
				m.Alias, m.MeterNo = name, no
			}
		}
	}

	for i := range rooms {
		r := &rooms[i]
		keys := []string{hrDormStr(r.CustomData, "宿舍编号"), hrDormStr(r.CustomData, "房屋编号"),
			hrDormStr(r.CustomData, "房间号"), r.Code}
		for _, k := range keys {
			if k = strings.TrimSpace(k); k != "" {
				if _, ok := rooms0[k]; !ok {
					rooms0[k] = r.CustomData
				}
			}
		}
		// 公租房住宿记录存「小区名+门牌」，一并注册
		if addr := strings.TrimSpace(hrDormStr(r.CustomData, "房屋地址")); addr != "" {
			if no := strings.TrimSpace(hrDormStr(r.CustomData, "房屋编号")); no != "" {
				if _, ok := rooms0[addr+no]; !ok {
					rooms0[addr+no] = r.CustomData
				}
			}
		}
	}
	for key := range rooms0 {
		apply(key, "electricity", "宿舍电表名称", "宿舍电表号")
		apply(key, "water", "宿舍水表名称", "宿舍水表号")
		apply(key, "water", "公租房水表名称", "公租房水表号")
		apply(key, "gas", "公租房气表名称", "公租房气表号")
	}

	if len(aliasOfMeter) == 0 {
		return meters, 0
	}
	ids := make([]string, 0, len(aliasOfMeter))
	for id := range aliasOfMeter {
		ids = append(ids, id)
	}
	type itemRow struct {
		ID        string
		MeterID   string
		MeterName string
	}
	var items73 []itemRow
	_ = h.db.WithContext(ctx).Raw(
		`SELECT id, meter_id, COALESCE(meter_name, '') AS meter_name FROM utility_meter_items WHERE meter_id IN ?`, ids).
		Scan(&items73).Error
	for _, it := range items73 {
		want := aliasOfMeter[it.MeterID]
		if want == "" || strings.TrimSpace(it.MeterName) == want {
			continue
		}
		if err := h.db.WithContext(ctx).Table("utility_meter_items").
			Where("id = ?", it.ID).Update("meter_name", want).Error; err == nil {
			items++
		}
	}
	return meters, items
}

// ---------------------------------------------------------------------------
// 第 73 轮：真实抄表值索引（费用核算唯一数据源）
// ---------------------------------------------------------------------------

// hrMeterReading73 一条抄表明细（按表号索引）。
type hrMeterReading73 struct {
	Start  float64
	End    float64
	Usage  float64
	Amount float64
	Found  bool
}

/**
 * hrRealMeterIndex73 该月「电/水/气核算」的真实抄表数据，按 **表号** 索引。
 *
 * 费用信息过去读的是 `hr_dorm_readings`（住宿记录里的读表），实测里面是
 * `0 → 73/86/112…` 的占位数据，于是宿舍费用表整片失真。
 * 能耗核算的 `utility_meter_items` 才是真实抄表（表号 E001 = 宿舍301，
 * 2026-10 为 8293→8357 / 64.00），费用必须与它同源。
 */
func (h *HRHandler) hrRealMeterIndex73(ctx context.Context, tid int64, month string) map[string]map[string]hrMeterReading73 {
	out := map[string]map[string]hrMeterReading73{
		"electricity": {}, "water": {}, "gas": {},
	}
	var records []struct {
		ID       string
		Category string
	}
	_ = h.db.WithContext(ctx).Raw(
		`SELECT id, category FROM utility_meter_records
		 WHERE tenant_id = ? AND month = ? AND deleted_at IS NULL`, tid, month).
		Scan(&records).Error
	if len(records) == 0 {
		return out
	}
	byID := map[string]string{}
	ids := make([]string, 0, len(records))
	for _, r := range records {
		byID[r.ID] = r.Category
		ids = append(ids, r.ID)
	}
	type itemRow struct {
		RecordID  string
		MeterID   string
		Start     float64
		End       float64
		Usage     float64
		Amount    float64
		DeepPrev  float64
		DeepCurr  float64
		PeakPrev  float64
		PeakCurr  float64
		FlatPrev  float64
		FlatCurr  float64
		ValleyPrev float64
		ValleyCurr float64
	}
	var items []itemRow
	_ = h.db.WithContext(ctx).Raw(
		`SELECT record_id, meter_id,
		        COALESCE(start_reading,0) AS start, COALESCE(end_reading,0) AS end,
		        COALESCE(usage,0) AS usage, COALESCE(amount,0) AS amount,
		        COALESCE(deep_prev,0) AS deep_prev, COALESCE(deep_curr,0) AS deep_curr,
		        COALESCE(peak_prev,0) AS peak_prev, COALESCE(peak_curr,0) AS peak_curr,
		        COALESCE(flat_prev,0) AS flat_prev, COALESCE(flat_curr,0) AS flat_curr,
		        COALESCE(valley_prev,0) AS valley_prev, COALESCE(valley_curr,0) AS valley_curr
		 FROM utility_meter_items WHERE record_id IN ?`, ids).Scan(&items).Error
	if len(items) == 0 {
		return out
	}
	meterNos := map[string]string{}
	var meters []typesUtilityMeterRow
	_ = h.db.WithContext(ctx).Raw(
		`SELECT id, meter_no FROM utility_meters WHERE tenant_id = ? AND deleted_at IS NULL`, tid).
		Scan(&meters).Error
	for _, m := range meters {
		meterNos[m.ID] = m.MeterNo
	}
	for _, it := range items {
		cat := byID[it.RecordID]
		no := strings.TrimSpace(meterNos[it.MeterID])
		if cat == "" || no == "" {
			continue
		}
		start, end := it.Start, it.End
		// 分时表（工业）四段示数才是真值，start/end 恒 0
		sumPrev := it.DeepPrev + it.PeakPrev + it.FlatPrev + it.ValleyPrev
		sumCurr := it.DeepCurr + it.PeakCurr + it.FlatCurr + it.ValleyCurr
		if start == 0 && end == 0 && (sumPrev > 0 || sumCurr > 0) {
			start, end = sumPrev, sumCurr
		}
		out[cat][strings.ToLower(no)] = hrMeterReading73{
			Start: round2(start), End: round2(end), Usage: round2(it.Usage),
			Amount: round2(it.Amount), Found: true,
		}
	}
	return out
}

// hrLatestMeterMonth73 该分类最新有数据的月份（无数据返回空串）。
func (h *HRHandler) hrLatestMeterMonth73(ctx context.Context, tid int64, category string) string {
	var month string
	_ = h.db.WithContext(ctx).Raw(
		`SELECT month FROM utility_meter_records
		 WHERE tenant_id = ? AND category = ? AND deleted_at IS NULL
		 ORDER BY month DESC LIMIT 1`, tid, category).Scan(&month).Error
	return strings.TrimSpace(month)
}

/**
 * hrFixBedOverflow73 修正「床位号 > 房型床位数」的历史脏数据。
 *
 * 场景：双人间（2 床）里出现 `3号床`。只改**住宿记录**的床位号，
 * 按入住日期顺序重排到 1..N；不触碰人员、日期、费用。
 */
func (h *HRHandler) hrFixBedOverflow73(ctx context.Context, tid int64) int {
	var stays []HRDormRecord
	_ = h.db.WithContext(ctx).Where("tenant_id = ? AND module = ?", tid, hrDormModuleStay).
		Find(&stays).Error
	if len(stays) == 0 {
		return 0
	}
	var rooms []HRDormRecord
	_ = h.db.WithContext(ctx).Where("tenant_id = ? AND module IN ?", tid,
		[]string{hrDormModuleRoom, hrDormModulePublic}).Find(&rooms).Error
	bedsOf := map[string]int{}
	for i := range rooms {
		beds := hrDormInt(rooms[i].CustomData, "床位数")
		if beds <= 0 {
			beds = hrDormBedsOfRoomType72(hrDormStr(rooms[i].CustomData, "房型"))
		}
		if beds <= 0 {
			continue
		}
		for _, k := range []string{rooms[i].Code, hrDormStr(rooms[i].CustomData, "宿舍编号"),
			hrDormStr(rooms[i].CustomData, "房屋编号"), hrDormStr(rooms[i].CustomData, "房屋地址"),
			hrDormStr(rooms[i].CustomData, "宿舍编号/房屋地址")} {
			if k = strings.TrimSpace(k); k != "" {
				bedsOf[k] = beds
			}
		}
	}
	// 按房源分组，按入住日期 + 创建时间排序
	group := map[string][]*HRDormRecord{}
	for i := range stays {
		s := &stays[i]
		if hrDormStr(s.CustomData, "住宿状态") == "已退宿" {
			continue
		}
		room := strings.TrimSpace(hrDormStr(s.CustomData, "宿舍编号/房屋地址"))
		if room == "" {
			continue
		}
		group[room] = append(group[room], s)
	}
	fixed := 0
	for room, list := range group {
		beds := bedsOf[room]
		if beds <= 0 {
			continue
		}
		sort.SliceStable(list, func(i, j int) bool {
			di, dj := hrDormStr(list[i].CustomData, "入住日期"), hrDormStr(list[j].CustomData, "入住日期")
			if di != dj {
				return di < dj
			}
			return list[i].CreatedAt.Before(list[j].CreatedAt)
		})
		used := map[string]bool{}
		for _, s := range list {
			cur := strings.TrimSpace(hrDormStr(s.CustomData, "床位号"))
			curIdx := hrBedIndexValue73(cur)
			// 合法且未被占用 → 保留用户设置
			if cur != "" && curIdx >= 1 && curIdx <= beds && !used[cur] {
				used[cur] = true
				continue
			}
			want := ""
			for n := 1; n <= beds; n++ {
				cand := fmt.Sprintf("%d号床", n)
				if !used[cand] {
					want = cand
					break
				}
			}
			if want == "" {
				continue
			}
			used[want] = true
			if cur == want {
				continue
			}
			s.CustomData["床位号"] = want
			if err := h.db.WithContext(ctx).Model(s).
				Select("custom_data").Updates(s).Error; err == nil {
				fixed++
			}
		}
	}
	return fixed
}

// hrBedIndexValue73 `3号床` → 3（无法解析返回 0）。
func hrBedIndexValue73(bed string) int {
	digits := strings.Builder{}
	for _, ch := range bed {
		if ch >= '0' && ch <= '9' {
			digits.WriteRune(ch)
			continue
		}
		if digits.Len() > 0 {
			break
		}
	}
	n, _ := strconv.Atoi(digits.String())
	return n
}

// hrRepair73 第 73 轮启动幂等修复。
func (h *HRHandler) hrRepair73(ctx context.Context) {
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Distinct("tenant_id").Pluck("tenant_id", &tids).Error; err != nil {
		logger.Warnf(ctx, "第 73 轮读取租户失败: %v", err)
		return
	}
	for _, tid := range tids {
		if tid <= 0 {
			continue
		}
		meters, items := h.hrSyncEnergyHistoryMeterNames73(ctx, tid)
		beds := h.hrFixBedOverflow73(ctx, tid)
		if meters > 0 || items > 0 || beds > 0 {
			logger.Infof(ctx,
				"第 73 轮数据修复（tenant=%d）：能耗表名对齐 %d 张、抄表明细补名 %d 条、越界床位重排 %d 条",
				tid, meters, items, beds)
		}
	}
}
