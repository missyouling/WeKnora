// 第 70 轮：宿舍/公租房数据与表计定义权迁移（幂等）。
//
// 三条红线：
//  1. **绝不动历史核算**（电费 123 / 水费 151 / 气费 86 条与 2026-10 分摊）；
//  2. 只补缺、只改空值，用户改过的字段一律保留；
//  3. 表计从「能耗表计设置」迁到「房源档案」时，原 utility_meters 记录**保留**
//     （核算与抄表历史仍按 meter_no 引用），只是把元数据复制一份到房源。
package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// hrDorm401To420Rooms70 需要补齐的 18 间（401~416、418、420；417/419 已存在）。
func hrDorm401To420Rooms70() []string {
	out := make([]string, 0, 18)
	for i := 401; i <= 416; i++ {
		out = append(out, fmt.Sprintf("%d", i))
	}
	out = append(out, "418", "420")
	return out
}

// hrSeedDorm401To420_70 幂等补全 1 号楼 4 层房源（401~416 / 418 / 420）。
//
// 归属口径：需求指定 1 号楼 401–420 统一挂「重庆星达铜业有限公司」。
// 能耗表计里已存在的「持睿4XX」属于持睿公司，**不覆盖、不删除**，
// 只在本函数里跳过同名房号（403/404/405/408/409/410/412/413/414/416/418），
// 由交付报告单独列出供用户裁决。
func (h *HRHandler) hrSeedDorm401To420_70(ctx context.Context, tid int64) int {
	exists := map[string]bool{}
	var rows []HRDormRecord
	_ = h.db.WithContext(ctx).
		Where("tenant_id = ? AND module = ?", tid, hrDormModuleRoom).Find(&rows).Error
	for _, r := range rows {
		if no := hrDormStr(r.CustomData, "房间号"); no != "" {
			exists[no] = true
		}
	}
	created := 0
	for _, no := range hrDorm401To420Rooms70() {
		if exists[no] {
			continue
		}
		data := map[string]any{
			"宿舍编号":     "宿舍" + no,
			"宿舍楼/区域":   "1号楼",
			"房间号":       no,
			"房型":        "双人间",
			"床位数":       float64(2),
			"性别限制":      "男",
			"房间面积(㎡)":   float64(28),
			"所属公司":      "重庆星达铜业有限公司",
			"宿舍状态":      "空闲",
			"电费标准(元/度)": float64(1),
			"水费单价(元/m³)": float64(5),
			"宿舍押金(元)":   float64(100),
			"宿舍电表名称":    "宿舍" + no,
			"宿舍电表号":     "E" + no,
			"宿舍水表名称":    "宿舍" + no,
			"宿舍水表号":     "W" + no,
		}
		rec := HRDormRecord{
			ID: uuid.New().String(), TenantID: tid, Module: hrDormModuleRoom,
			Code: hrDormCodeOf(hrDormModuleRoom, data),
			Name: hrDormNameOf(hrDormModuleRoom, data),
			Status: hrDormStatusOf(hrDormModuleRoom, data),
			CustomData: data,
		}
		if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
			logger.Warnf(ctx, "补齐宿舍%s失败: %v", no, err)
			continue
		}
		created++
	}
	return created
}

// hrDormStandardAssets70 12 类标准宿舍物品（名称、分类、数量、单位）。
var hrDormStandardAssets70 = []struct {
	Name  string
	Cat   string
	Qty   float64
	Unit  string
	Width int
}{
	{"晾衣杆", "家具", 2, "根", 0},
	{"毛巾杆", "家具", 2, "根", 0},
	{"衣柜", "家具", 6, "个", 0},
	{"床柜", "家具", 4, "个", 0},
	{"床头柜", "家具", 2, "个", 0},
	{"空调遥控器", "家电", 1, "个", 0},
	{"空调", "家电", 1, "台", 0},
	{"电热水器", "家电", 1, "台", 0},
	{"床", "家具", 2, "张", 0},
	{"床垫", "家具", 2, "张", 0},
	{"钥匙", "其他", 2, "把", 0},
	{"窗帘", "家具", 1, "副", 0},
}

// hrSeedDormAssets70 幂等补齐 12 类标准资产（每间宿舍一份，已存在同名则跳过）。
//
// 只补**宿舍**（module=dorm）房间，不碰公租房：公租房的家具配置与宿舍不同，
// 需求只要求「标准宿舍资产档案」。已有资产（301 的床/空调、302 的热水器…）不重复建。
func (h *HRHandler) hrSeedDormAssets70(ctx context.Context, tid int64) int {
	var rooms []HRDormRecord
	_ = h.db.WithContext(ctx).
		Where("tenant_id = ? AND module = ?", tid, hrDormModuleRoom).Find(&rooms).Error
	if len(rooms) == 0 {
		return 0
	}
	type key struct{ room, name string }
	have := map[key]bool{}
	var assets []HRDormRecord
	_ = h.db.WithContext(ctx).
		Where("tenant_id = ? AND module = ?", tid, hrDormModuleAsset).Find(&assets).Error
	for _, a := range assets {
		have[key{hrDormStr(a.CustomData, "所属宿舍"), hrDormStr(a.CustomData, "资产名称")}] = true
	}
	created := 0
	for _, room := range rooms {
		roomName := hrDormStr(room.CustomData, "宿舍编号")
		if roomName == "" {
			roomName = room.Code
		}
		for _, std := range hrDormStandardAssets70 {
			if have[key{roomName, std.Name}] {
				continue
			}
			data := map[string]any{
				"资产编号":   fmt.Sprintf("ZC-%s-%s", strings.TrimPrefix(roomName, "宿舍"), std.Name),
				"资产名称":   std.Name,
				"资产分类":   std.Cat,
				"所属宿舍":   roomName,
				"数量":     std.Qty,
				"计量单位":   std.Unit,
				"资产状态":   "正常",
				"规格型号":   "",
				"资产价值(元)": float64(0),
			}
			rec := HRDormRecord{
				ID: uuid.New().String(), TenantID: tid, Module: hrDormModuleAsset,
				Code: hrDormCodeOf(hrDormModuleAsset, data),
				Name: hrDormNameOf(hrDormModuleAsset, data),
				Status: hrDormStatusOf(hrDormModuleAsset, data),
				CustomData: data,
			}
			if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
				continue
			}
			have[key{roomName, std.Name}] = true
			created++
		}
	}
	return created
}

// hrMigrateDormMeters70 把能耗表计设置里的「宿舍 / 公租房」表计元数据回填到房源档案。
//
// 迁移方向（第 70 轮定义权反转）：
//
//	utility_meters(meter_kind=dorm)     → 宿舍档案 宿舍电表/宿舍水表
//	utility_meters(meter_kind=custom1)  → 公租房档案 公租房水表/公租房气表
//
// 只填空值：房源档案里已有表号/名称的字段不覆盖（用户在档案里改过价，以档案为准）。
// 原 utility_meters 记录**不删除** —— 抄表历史与费用核算仍按 meter_no 引用它。
//
// 返回（回填宿舍表数、回填公租房表数、未匹配表数）。
func (h *HRHandler) hrMigrateDormMeters70(ctx context.Context, tid int64) (int, int, int) {
	var meters []typesUtilityMeterRow
	if err := h.db.WithContext(ctx).Raw(
		`SELECT id, category, alias, meter_no, rate, default_unit_price, use_unit, meter_kind
		   FROM utility_meters WHERE tenant_id = ? AND deleted_at IS NULL`, tid).
		Scan(&meters).Error; err != nil {
		logger.Warnf(ctx, "读取能耗表计失败: %v", err)
		return 0, 0, 0
	}
	if len(meters) == 0 {
		return 0, 0, 0
	}
	var rooms []HRDormRecord
	_ = h.db.WithContext(ctx).
		Where("tenant_id = ? AND module IN ?", tid,
			[]string{hrDormModuleRoom, hrDormModulePublic}).Find(&rooms).Error
	byCode := map[string]*HRDormRecord{}
	for i := range rooms {
		r := &rooms[i]
		byCode[hrDormStr(r.CustomData, "宿舍编号")] = r
		byCode[hrDormStr(r.CustomData, "房屋地址")] = r
		byCode[r.Code] = r
	}
	dormHit, publicHit, missed := 0, 0, 0
	for _, m := range meters {
		name := strings.TrimSpace(m.Alias)
		if name == "" {
			name = strings.TrimSpace(m.MeterNo)
		}
		if name == "" {
			continue
		}
		room := byCode[name]
		if room == nil {
			// 持睿 403~418 这类「表计名 ≠ 星达房源名」的，按房间号再试一次
			for _, no := range hrDorm401To420Rooms70() {
				if strings.HasSuffix(name, no) {
					if r := byCode["宿舍"+no]; r != nil {
						room = r
						break
					}
				}
			}
		}
		if room == nil {
			missed++
			continue
		}
		isPublic := room.Module == hrDormModulePublic
		changed := false
		setIfEmpty := func(k string, v any) {
			if _, ok := room.CustomData[k]; ok {
				if s := strings.TrimSpace(fmt.Sprint(room.CustomData[k])); s != "" && s != "<nil>" && s != "0" {
					return
				}
			}
			room.CustomData[k] = v
			changed = true
		}
		switch {
		case !isPublic && m.Category == "electricity":
			setIfEmpty("宿舍电表名称", name)
			setIfEmpty("宿舍电表号", m.MeterNo)
			if m.DefaultUnitPrice > 0 {
				setIfEmpty("电费标准(元/度)", m.DefaultUnitPrice)
			}
			if m.Rate > 0 {
				setIfEmpty("电表倍率", m.Rate)
			}
		case !isPublic && m.Category == "water":
			setIfEmpty("宿舍水表名称", name)
			setIfEmpty("宿舍水表号", m.MeterNo)
			if m.DefaultUnitPrice > 0 {
				setIfEmpty("水费单价(元/m³)", m.DefaultUnitPrice)
			}
			if m.Rate > 0 {
				setIfEmpty("水表倍率", m.Rate)
			}
		case isPublic && m.Category == "water":
			setIfEmpty("公租房水表名称", name)
			setIfEmpty("公租房水表号", m.MeterNo)
			if m.DefaultUnitPrice > 0 {
				setIfEmpty("水费单价(元/m³)", m.DefaultUnitPrice)
			}
			if m.Rate > 0 {
				setIfEmpty("水表倍率", m.Rate)
			}
		case isPublic && m.Category == "gas":
			setIfEmpty("公租房气表名称", name)
			setIfEmpty("公租房气表号", m.MeterNo)
			if m.DefaultUnitPrice > 0 {
				setIfEmpty("气费标准(元/m³)", m.DefaultUnitPrice)
			}
			if m.Rate > 0 {
				setIfEmpty("气表倍率", m.Rate)
			}
		default:
			continue
		}
		if !changed {
			continue
		}
		// JSONB 列必须走 serializer：`Update("custom_data", map)` 会被 database/sql
		// 判为 unsupported type（第 69 轮已踩过），Select+Updates 才会序列化。
		if err := h.db.WithContext(ctx).Model(room).
			Select("custom_data").Updates(room).Error; err != nil {
			logger.Warnf(ctx, "回填房源表计失败（%s）: %v", room.Code, err)
			continue
		}
		if isPublic {
			publicHit++
		} else {
			dormHit++
		}
	}
	return dormHit, publicHit, missed
}

// typesUtilityMeterRow 只取迁移需要的列（不引入完整实体，避免误写）。
type typesUtilityMeterRow struct {
	ID               string  `gorm:"column:id"`
	Category         string  `gorm:"column:category"`
	Alias            string  `gorm:"column:alias"`
	MeterNo          string  `gorm:"column:meter_no"`
	Rate             float64 `gorm:"column:rate"`
	DefaultUnitPrice float64 `gorm:"column:default_unit_price"`
	MeterKind        string  `gorm:"column:meter_kind"`
}

// ---------------------------------------------------------------------------
// 公司改名全局同步
// ---------------------------------------------------------------------------

// hrSyncBusinessUnitName70 单位改名后，业务数据（宿舍域 + 能耗表计）的事务内级联。
//
// 花名册的「签约公司」由既有 `hrCascadeUnitRename` 负责（第 6 轮），
// 劳动合同视图的数据源也是花名册，所以这里只补它没覆盖的两块：
//   - 宿舍 / 公租房 / 住宿记录 / 资产档案（所属公司 / 租赁单位 / 所属部门 / 归属单位）；
//   - 能耗表计（使用单位 / 归属单位）。
//
// 只替换「整值完全等于旧名」的字段：已是新名的记录不误改，长文本不被截断。
func hrSyncBusinessUnitName70(tx *gorm.DB, tid int64, oldName, newName string) (int64, error) {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" || oldName == newName {
		return 0, nil
	}
	var affected int64
	var dormRows []HRDormRecord
	if err := tx.Where("tenant_id = ?", tid).Find(&dormRows).Error; err != nil {
		return affected, err
	}
	for i := range dormRows {
		row := &dormRows[i]
		if row.CustomData == nil {
			continue
		}
		changed := false
		for _, key := range []string{"所属公司", "租赁单位", "所属部门", "归属单位", "使用单位"} {
			if strings.TrimSpace(hrDormStr(row.CustomData, key)) == oldName {
				row.CustomData[key] = newName
				changed = true
			}
		}
		if !changed {
			continue
		}
		if err := tx.Model(row).Select("custom_data").Updates(row).Error; err != nil {
			return affected, err
		}
		affected++
	}
	// `now()` 是 PG 函数，SQLite（测试库）没有 —— 时间由 Go 侧传入，跨库一致
	now := time.Now().UTC()
	res := tx.Exec(`UPDATE utility_meters SET use_unit = ?, updated_at = ?
		WHERE tenant_id = ? AND deleted_at IS NULL AND use_unit = ?`, newName, now, tid, oldName)
	if res.Error != nil {
		return affected, res.Error
	}
	affected += res.RowsAffected
	res = tx.Exec(`UPDATE utility_meters SET owner_unit = ?, updated_at = ?
		WHERE tenant_id = ? AND deleted_at IS NULL AND owner_unit = ?`, newName, now, tid, oldName)
	if res.Error != nil {
		return affected, res.Error
	}
	affected += res.RowsAffected
	return affected, nil
}

// hrCompanyKey70 公司名归一化：去掉「重庆/重庆市」前缀与「有限公司/有限责任公司」后缀。
//
// 只用于**存量对齐**的候选匹配（`重庆持睿汽车部件有限公司` ↔ `持睿汽车部件有限公司`），
// 绝不用于新建写库，也不会把两个真正不同的公司误判成同一个（前缀/后缀之外的字符必须完全一致）。
func hrCompanyKey70(name string) string {
	s := strings.TrimSpace(name)
	for _, p := range []string{"重庆市", "重庆"} {
		if strings.HasPrefix(s, p) {
			s = strings.TrimPrefix(s, p)
			break
		}
	}
	for _, suf := range []string{"有限责任公司", "有限公司", "公司"} {
		if strings.HasSuffix(s, suf) {
			s = strings.TrimSuffix(s, suf)
			break
		}
	}
	return strings.TrimSpace(s)
}

// hrAlignCompanyNames70 存量公司名对齐（幂等）。
//
// 场景：单位设置里已把 `重庆持睿汽车部件有限公司` 改成 `持睿汽车部件有限公司`，
// 但 417 宿舍等历史记录仍存旧名 —— 前端「所属公司」下拉只有新名，打开抽屉看起来是空的。
// 这里把业务数据里「归一化后与某个现行单位名相同、但字面不同」的值统一成现行名。
func (h *HRHandler) hrAlignCompanyNames70(ctx context.Context, tid int64) int {
	var units []HROrgUnit
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&units).Error
	if len(units) == 0 {
		return 0
	}
	byKey := map[string]string{}
	names := map[string]bool{}
	for _, u := range units {
		n := strings.TrimSpace(u.Name)
		if n == "" {
			continue
		}
		names[n] = true
		byKey[hrCompanyKey70(n)] = n
	}
	align := func(v string) (string, bool) {
		v = strings.TrimSpace(v)
		if v == "" || names[v] {
			return "", false
		}
		if target, ok := byKey[hrCompanyKey70(v)]; ok && target != v {
			return target, true
		}
		return "", false
	}
	fixed := 0

	var emps []HREmployee
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&emps).Error
	for i := range emps {
		e := &emps[i]
		if e.CustomData == nil {
			continue
		}
		changed := false
		for _, key := range []string{"签约公司", "现合同签约公司", "合同公司"} {
			if target, ok := align(toStringValue(e.CustomData[key])); ok {
				e.CustomData[key] = target
				changed = true
			}
		}
		if !changed {
			continue
		}
		if err := h.db.WithContext(ctx).Model(e).
			Select("custom_data").Updates(e).Error; err == nil {
			fixed++
		}
	}

	var dormRows []HRDormRecord
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&dormRows).Error
	for i := range dormRows {
		row := &dormRows[i]
		if row.CustomData == nil {
			continue
		}
		changed := false
		for _, key := range []string{"所属公司", "租赁单位", "归属单位", "使用单位"} {
			if target, ok := align(hrDormStr(row.CustomData, key)); ok {
				row.CustomData[key] = target
				changed = true
			}
		}
		if !changed {
			continue
		}
		if err := h.db.WithContext(ctx).Model(row).
			Select("custom_data").Updates(row).Error; err == nil {
			fixed++
		}
	}
	return fixed
}

// hrRepair70 第 70 轮启动幂等修复：补房源、补资产、迁表计。
func (h *HRHandler) hrRepair70(ctx context.Context) {
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Distinct().Pluck("tenant_id", &tids).Error; err != nil {
		logger.Warnf(ctx, "第 70 轮修复：读取租户失败: %v", err)
		return
	}
	for _, tid := range tids {
		if tid == 0 {
			continue
		}
		aligned := h.hrAlignCompanyNames70(ctx, tid)
		rooms := h.hrSeedDorm401To420_70(ctx, tid)
		assets := h.hrSeedDormAssets70(ctx, tid)
		dormMeters, publicMeters, missed := h.hrMigrateDormMeters70(ctx, tid)
		if aligned > 0 || rooms > 0 || assets > 0 || dormMeters > 0 || publicMeters > 0 {
			logger.Infof(ctx,
				"第 70 轮数据修复（tenant=%d）：公司名对齐 %d 条，补房源 %d 间，补标准资产 %d 条，宿舍表回填 %d 张，公租房表回填 %d 张，未匹配 %d 张",
				tid, aligned, rooms, assets, dormMeters, publicMeters, missed)
		}
	}
}
