package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

/**
 * 第 72 轮数据迁移（全部幂等，只动配置与指定脏数据，不碰历史抄表/核算）。
 *
 * 三件事：
 *   1. 宿舍状态四态化：`维修中` → `空置`（含 is_vacant 标记），并给「空置」补持久化字段；
 *   2. 住宿记录契约瘦身：删除三个「入住时*表度数」字段（六同步：subs/列/筛选/抽屉/模板）；
 *   3. 资产档案从「一房一物台账」纠偏为「共用物品字典」：
 *      清掉第 70 轮按 40 间房展开的 477 条种子，保留存量去重项，补齐 12 类标准，
 *      统一重编号 ZC-001…（软删除，不物理删，误操作可回滚）。
 */

// hrDormStateCutoff72 第 70 轮资产的生成时间下界（此前为真实存量）。
// 40 间房的种子记录均由 hrSeedDormAssets70 在 2026-11-25 批量写入。
var hrDormStateCutoff72 = time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)

// hrRepair72 启动幂等修复入口。
func (h *HRHandler) hrRepair72(ctx context.Context) {
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HRDormRecord{}).
		Distinct("tenant_id").Pluck("tenant_id", &tids).Error; err != nil {
		logger.Warnf(ctx, "第 72 轮读取租户失败: %v", err)
		return
	}
	for _, tid := range tids {
		states := h.hrMigrateDormState72(ctx, tid)
		dropped := h.hrDropStayMeterFields72(ctx, tid)
		assets := h.hrConsolidateAssetDict72(ctx, tid)
		// 存量重复床位号（同一房源两人都标 1号床）按入住顺序重排
		beds := h.hrFixDuplicateBedNo72(ctx, tid)
		// 舍长手动清空的历史残留标记（会导致公租房舍长永远为空）
		stale := h.hrClearStaleManagerFlags72(ctx, tid)
		if stale > 0 {
			logger.Infof(ctx, "第 72 轮清理舍长清空残留标记 %d 条（tenant=%d）", stale, tid)
		}
		if states > 0 || dropped || assets > 0 || beds > 0 {
			logger.Infof(ctx,
				"第 72 轮数据修复（tenant=%d）：状态清洗 %d 间，住宿度数字段删除 %v，资产字典收敛 %d 条，重排床位 %d 条",
				tid, states, dropped, assets, beds)
		}
	}
}

// hrMigrateDormState72 `维修中` → `空置`（records + is_vacant 标记）。
func (h *HRHandler) hrMigrateDormState72(ctx context.Context, tid int64) int {
	var rows []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND module IN ?", tid,
			[]string{hrDormModuleRoom, hrDormModulePublic}).Find(&rows).Error; err != nil {
		return 0
	}
	changed := 0
	for i := range rows {
		row := &rows[i]
		if row.CustomData == nil {
			continue
		}
		if strings.TrimSpace(hrDormStr(row.CustomData, "宿舍状态")) != "维修中" {
			continue
		}
		row.CustomData["宿舍状态"] = "空置"
		row.CustomData["空置"] = true
		if err := h.db.WithContext(ctx).Model(row).Select("custom_data").Updates(row).Error; err != nil {
			logger.Warnf(ctx, "第 72 轮状态清洗失败（%s）: %v", row.Code, err)
			continue
		}
		changed++
	}
	return changed
}

// hrDropStayMeterFields72 删除住宿记录的三个「入住时*表度数」字段（subs 幂等清理）。
func (h *HRHandler) hrDropStayMeterFields72(ctx context.Context, tid int64) bool {
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("scope = ? AND deleted_at IS NULL", "hr-dorm").Find(&cats).Error; err != nil {
		return false
	}
	drop := map[string]bool{
		"入住时水表度数": true, "入住时电表度数": true, "入住时气表度数": true,
		// 住宿记录ID 同属弃用字段（内部主键只做路由参数，不对用户展示）
		"住宿记录ID": true,
	}
	changed := false
	for i := range cats {
		cat := &cats[i]
		kept := make([]types.FleetCategorySub, 0, len(cat.Subs))
		for _, sub := range cat.Subs {
			if strings.TrimSpace(sub.Group) == "住宿记录" && drop[strings.TrimSpace(sub.Name)] {
				changed = true
				continue
			}
			kept = append(kept, sub)
		}
		if len(kept) == len(cat.Subs) {
			continue
		}
		cat.Subs = kept
		if err := h.db.WithContext(ctx).Model(cat).Select("subs").Updates(cat).Error; err != nil {
			logger.Warnf(ctx, "第 72 轮删除度数字段失败（%s）: %v", cat.Name, err)
		}
	}
	return changed
}

// hrConsolidateAssetDict72 资产档案收敛为共用物品字典。
//
// 顺序很重要：
//  1. 先软删「第 70 轮种子」（created_at >= cutoff）—— 它们是一房一物的展开；
//  2. 再按名称对**存量**去重（同名保留最早一条）；
//  3. 补齐 12 类标准物品（缺失才建，不覆盖用户改过的名称/数量）；
//  4. 按名称排序统一重编号 ZC-001…（编号可手改，这里是首次建档的规范值）。
func (h *HRHandler) hrConsolidateAssetDict72(ctx context.Context, tid int64) int {
	/**
	 * `hr_dorm_records` **没有 deleted_at 列**（模型只有 created_at / updated_at），
	 * 所以这里是物理删除。安全前提：只删「第 70 轮按 40 间房展开的种子」，
	 * 判定双条件 —— 创建时间 >= cutoff 且资产编号符合 `ZC-<房间号>-<物品>` 形态。
	 * 删除前把 id 清单写进日志，便于追查与人工恢复。
	 */
	var seedIDs []string
	seedCond := `tenant_id = ? AND module = ? AND created_at >= ?
		AND (custom_data->>'资产编号' LIKE 'ZC-3%-%' OR custom_data->>'资产编号' LIKE 'ZC-4%-%'
		     OR custom_data->>'资产编号' LIKE 'ZC-成都%-%')`
	_ = h.db.WithContext(ctx).Model(&HRDormRecord{}).
		Where(seedCond, tid, hrDormModuleAsset, hrDormStateCutoff72).
		Pluck("id", &seedIDs).Error
	if len(seedIDs) > 0 {
		sample := seedIDs
		if len(sample) > 5 {
			sample = sample[:5]
		}
		logger.Infof(ctx, "第 72 轮清除资产种子 %d 条（tenant=%d，样例 id：%v）",
			len(seedIDs), tid, sample)
		if err := h.db.WithContext(ctx).
			Where(seedCond, tid, hrDormModuleAsset, hrDormStateCutoff72).
			Delete(&HRDormRecord{}).Error; err != nil {
			logger.Warnf(ctx, "第 72 轮清理资产种子失败: %v", err)
			return 0
		}
	}

	// 2) 存量
	var rows []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND module = ?", tid, hrDormModuleAsset).
		Find(&rows).Error; err != nil {
		return 0
	}
	keep := map[string]*HRDormRecord{}
	var order []string
	for i := range rows {
		row := &rows[i]
		name := strings.TrimSpace(hrDormStr(row.CustomData, "资产名称"))
		if name == "" {
			name = strings.TrimSpace(row.Name)
		}
		if name == "" {
			continue
		}
		if prev, ok := keep[name]; ok {
			/**
			 * 同名只保留遍历到的第一条。
			 *
			 * 旧实现用 `CreatedAt.After(prev)` 判定，而第 70 轮的种子是**同一批**
			 * 写入的（时间戳完全相同），After 为 false → 一条都删不掉，
			 * 收敛后仍然重复（真机看到两条「床」）。
			 */
			if prev.ID != row.ID {
				_ = h.db.WithContext(ctx).Where("id = ?", row.ID).Delete(&HRDormRecord{}).Error
			}
			continue
		}
		keep[name] = row
		order = append(order, name)
	}

	// 3) 补 12 类标准（数量/单位按需求口径）
	type dictItem struct {
		name string
		qty  int
		unit string
		cat  string
	}
	standard := []dictItem{
		{"晾衣杆", 2, "根", "家具"},
		{"毛巾杆", 2, "根", "家具"},
		{"衣柜", 6, "个", "家具"},
		{"床柜", 4, "个", "家具"},
		{"床头柜", 2, "个", "家具"},
		{"空调遥控器", 1, "个", "家电"},
		{"空调", 1, "台", "家电"},
		{"电热水器", 1, "台", "家电"},
		{"床", 2, "张", "家具"},
		{"床垫", 2, "张", "家具"},
		{"钥匙", 2, "把", "其他"},
		{"窗帘", 1, "副", "家具"},
	}
	added := 0
	for _, it := range standard {
		if _, ok := keep[it.name]; ok {
			continue
		}
		rec := HRDormRecord{
			ID:       fmt.Sprintf("asset-dict-%s", it.name),
			TenantID: tid, Module: hrDormModuleAsset,
			Code: it.name, Name: it.name,
			CustomData: map[string]any{
				"资产名称": it.name, "资产分类": it.cat,
				"数量": it.qty, "计量单位": it.unit,
				"资产状态": "正常", "所属宿舍": "",
			},
		}
		if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
			logger.Warnf(ctx, "第 72 轮补建字典项失败（%s）: %v", it.name, err)
			continue
		}
		keep[it.name] = &rec
		order = append(order, it.name)
		added++
	}

	// 4) 统一编号 ZC-001…
	// 先按「12 类标准顺序」再按名称，保证编号稳定可读
	rank := map[string]int{}
	for i, it := range standard {
		rank[it.name] = i
	}
	sortNames := append([]string(nil), order...)
	for i := 0; i < len(sortNames); i++ {
		for j := i + 1; j < len(sortNames); j++ {
			ri, iok := rank[sortNames[i]]
			rj, jok := rank[sortNames[j]]
			swap := false
			switch {
			case iok && jok:
				swap = ri > rj
			case iok && !jok:
				swap = false
			case !iok && jok:
				swap = true
			default:
				swap = sortNames[i] > sortNames[j]
			}
			if swap {
				sortNames[i], sortNames[j] = sortNames[j], sortNames[i]
			}
		}
	}
	for idx, name := range sortNames {
		row := keep[name]
		if row == nil {
			continue
		}
		code := fmt.Sprintf("ZC-%03d", idx+1)
		if row.CustomData == nil {
			row.CustomData = map[string]any{}
		}
		changed := false
		oldCode := strings.TrimSpace(hrDormStr(row.CustomData, "资产编号"))
		if oldCode != code || row.Code != code {
			row.CustomData["资产编号"] = code
			row.Code = code
			changed = true
		}
		/**
		 * 12 类标准物品的数量/单位/分类按**规范值**收敛（衣柜 6 个、床 2 张…）。
		 *
		 * 注意：这段不能放在「编号已正确就 continue」之后 —— 首次收敛时编号已经
		 * 写对了，第二次启动就会跳过，于是「衣柜 1 个」这类旧数量永远修不掉
		 * （真机 6.1 期望 6 个却读到 1）。现在改成逐字段比较，只在有差异时写库。
		 */
		for _, it := range standard {
			if it.name != name {
				continue
			}
			if strings.TrimSpace(hrDormStr(row.CustomData, "计量单位")) != it.unit {
				row.CustomData["计量单位"] = it.unit
				changed = true
			}
			if strings.TrimSpace(hrDormStr(row.CustomData, "资产分类")) != it.cat {
				row.CustomData["资产分类"] = it.cat
				changed = true
			}
			if hrDormFloat(row.CustomData, "数量") != float64(it.qty) {
				row.CustomData["数量"] = it.qty
				changed = true
			}
			if strings.TrimSpace(hrDormStr(row.CustomData, "所属宿舍")) != "" {
				row.CustomData["所属宿舍"] = ""
				changed = true
			}
			break
		}
		if !changed {
			continue
		}
		if err := h.db.WithContext(ctx).Model(row).
			Select("code", "custom_data").Updates(row).Error; err != nil {
			logger.Warnf(ctx, "第 72 轮资产重编号失败（%s）: %v", name, err)
		}
	}
	return len(seedIDs) + added
}

/**
 * hrClearStaleManagerFlags72 清理「舍长手动清空」的历史残留标记。
 *
 * 背景：第 70 轮引入了 `舍长手动清空=1` —— 用户清空舍长后不再自动填回。
 * 但早期批量保存（以及若干次迁移）把**本来就为空**的房源也打上了这个标记，
 * 结果公租房（5-2-6 等）明明住着人，抽屉里舍长却永远是空、成员把舍长也算进去，
 * 与「默认最早入住的在住人员」的规则冲突。
 *
 * 处置：只清标记、不动任何业务值。清掉后由派生规则接管（有在住人员 → 首住人）；
 * 用户若确实想留空，重新清空一次会再次打标记。
 */
func (h *HRHandler) hrClearStaleManagerFlags72(ctx context.Context, tid int64) int {
	var rows []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND module IN ?", tid,
			[]string{hrDormModuleRoom, hrDormModulePublic}).Find(&rows).Error; err != nil {
		return 0
	}
	cleared := 0
	for i := range rows {
		row := &rows[i]
		if row.CustomData == nil {
			continue
		}
		if strings.TrimSpace(hrDormStr(row.CustomData, "管理员/舍长")) != "" {
			continue
		}
		if strings.TrimSpace(hrDormStr(row.CustomData, "舍长手动清空")) != "1" {
			continue
		}
		delete(row.CustomData, "舍长手动清空")
		if err := h.db.WithContext(ctx).Model(row).
			Select("custom_data").Updates(row).Error; err == nil {
			cleared++
		}
	}
	return cleared
}

// ---------------------------------------------------------------------------
// 第 72 轮保存校验（表计查重 / 空置误切 / 防缩容 / 床位唯一）
// ---------------------------------------------------------------------------

// hrDormMeterConflict 占位信息：哪个房源用了这个表号/表名。
type hrDormMeterConflict struct {
	Room string
	Kind string // 电表 / 水表 / 气表
	No   string
	Name string
}

/**
 * hrDormMeterPool72 汇总表计查重池。
 *
 * 池子构成（需求 2④）：
 *   · 宿舍档案全部电表号/名称 + 工商业电表（utility_meters category=electricity）；
 *   · 宿舍 + 公租房的水表号/名称 + 工商业水表；
 *   · 公租房的气表号/名称 + 工商业气表。
 *
 * `selfID` 用于编辑时排除自身。
 */
func (h *HRHandler) hrDormMeterPool72(ctx context.Context, tid int64, selfID string) []hrDormMeterConflict {
	var rows []HRDormRecord
	_ = h.db.WithContext(ctx).
		Where("tenant_id = ? AND module IN ?", tid,
			[]string{hrDormModuleRoom, hrDormModulePublic}).Find(&rows).Error
	out := make([]hrDormMeterConflict, 0, len(rows)*4)
	for i := range rows {
		r := &rows[i]
		if selfID != "" && r.ID == selfID {
			continue
		}
		room := strings.TrimSpace(hrDormStr(r.CustomData, "房间号"))
		if room == "" {
			room = r.Code
		}
		if room == "" {
			room = strings.TrimSpace(hrDormStr(r.CustomData, "房屋编号"))
		}
		isPublic := r.Module == hrDormModulePublic
		if isPublic {
			out = append(out,
				hrDormMeterConflict{Room: room, Kind: "水表", No: hrDormStr(r.CustomData, "公租房水表号"), Name: hrDormStr(r.CustomData, "公租房水表名称")},
				hrDormMeterConflict{Room: room, Kind: "气表", No: hrDormStr(r.CustomData, "公租房气表号"), Name: hrDormStr(r.CustomData, "公租房气表名称")},
			)
		} else {
			out = append(out,
				hrDormMeterConflict{Room: room, Kind: "电表", No: hrDormStr(r.CustomData, "宿舍电表号"), Name: hrDormStr(r.CustomData, "宿舍电表名称")},
				hrDormMeterConflict{Room: room, Kind: "水表", No: hrDormStr(r.CustomData, "宿舍水表号"), Name: hrDormStr(r.CustomData, "宿舍水表名称")},
			)
		}
	}
	// 工商业表计（表号 + 别名）
	var meters []types.UtilityMeter
	_ = h.db.WithContext(ctx).
		Where("tenant_id = ? AND deleted_at IS NULL", tid).Find(&meters).Error
	for _, m := range meters {
		kind := "电表"
		switch strings.TrimSpace(m.Category) {
		case "water":
			kind = "水表"
		case "gas":
			kind = "气表"
		}
		out = append(out, hrDormMeterConflict{
			Room: strings.TrimSpace(m.Alias), Kind: kind,
			No: strings.TrimSpace(m.MeterNo), Name: strings.TrimSpace(m.Alias),
		})
	}
	return out
}

/**
 * hrDormValidateMeters72 表计全局唯一性校验（表号与表名都查）。
 *
 * 只校验**本次提交的非空字段**；空串表示不设表，不参与查重。
 * 返回中文错误，指明冲突来源。
 */
func (h *HRHandler) hrDormValidateMeters72(
	ctx context.Context, tid int64, module string, selfID string, data map[string]any,
) error {
	if module != hrDormModuleRoom && module != hrDormModulePublic {
		return nil
	}
	pool := h.hrDormMeterPool72(ctx, tid, selfID)
	isPublic := module == hrDormModulePublic
	type probe struct{ kind, noKey, nameKey string }
	probes := []probe{}
	if isPublic {
		probes = append(probes,
			probe{"水表", "公租房水表号", "公租房水表名称"},
			probe{"气表", "公租房气表号", "公租房气表名称"},
			probe{"水表", "水表号", "水表名称"},
			probe{"气表", "气表号", "气表名称"},
		)
	} else {
		probes = append(probes,
			probe{"电表", "宿舍电表号", "宿舍电表名称"},
			probe{"水表", "宿舍水表号", "宿舍水表名称"},
			probe{"电表", "电表号", "电表名称"},
			probe{"水表", "水表号", "水表名称"},
		)
	}
	for _, pr := range probes {
		no := strings.TrimSpace(hrDormStr(data, pr.noKey))
		name := strings.TrimSpace(hrDormStr(data, pr.nameKey))
		if no == "" && name == "" {
			continue
		}
		for _, c := range pool {
			if c.Kind != pr.kind || strings.TrimSpace(c.Room) == "" {
				continue
			}
			// 同一房源（表计来自 utility_meters 时 Room 是别名，靠表号相等兜住）
			if no != "" && strings.EqualFold(strings.TrimSpace(c.No), no) {
				return fmt.Errorf("%s号 %q 已被「%s」占用，请勿重复录入", pr.kind, no, c.Room)
			}
			if name != "" && strings.EqualFold(strings.TrimSpace(c.Name), name) {
				return fmt.Errorf("%s名称 %q 已被「%s」占用，请勿重复录入", pr.kind, name, c.Room)
			}
		}
	}
	return nil
}

/**
 * hrDormValidateCapacity72 防缩容 + 空置误切。
 *
 * `occupied` 为该房源当前在住人数；`beds` 为本次提交的床位数；
 * `vacant` 为本次是否打开空置开关。
 */
func hrDormValidateCapacity72(room string, beds, occupied int, vacant bool) error {
	if vacant && occupied > 0 {
		return fmt.Errorf("「%s」当前仍有 %d 名在住人员，请先办理退宿后再设为空置", room, occupied)
	}
	if beds > 0 && occupied > beds {
		return fmt.Errorf("该房源当前在住 %d 人，床位数不能少于在住人数（拟设 %d 张床）", occupied, beds)
	}
	return nil
}

/** hrDormBedsOfRoomType72 房型 → 标准床位数（0 表示未知房型，不覆盖）。 */
func hrDormBedsOfRoomType72(roomType string) int {
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
	return 0
}

/** hrDormBool 兼容 bool / "1" / "true" / "是" 的布尔读取。 */
func hrDormBool(data map[string]any, key string) bool {
	if data == nil {
		return false
	}
	raw, ok := data[key]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case int:
		return v != 0
	}
	switch strings.TrimSpace(fmt.Sprint(raw)) {
	case "1", "true", "TRUE", "是", "开启":
		return true
	}
	return false
}

/**
 * hrDormValidateRoomMeta72 房档保存元数据校验 + 派生字段归一。
 *
 * 需求 1 的服务端兜底：
 *   · 提交了「房型」→ 按房型重算床位数（单 1 / 双 2 / 夫妻 2 / 四 4 / 六 6 / 八 8）；
 *   · 床位数 < 当前在住人数 → 400「防缩容」；
 *   · 打开空置但仍有在住人员 → 400「请先办理退宿」；
 *   · 无论前端传什么，`可用床位数` 一律按「床位数 − 真实在住人数」重算，
 *     空置时强制 0。这样列表/抽屉永远不可能出现「1 张床却有 2 个空位」。
 */
func (h *HRHandler) hrDormValidateRoomMeta72(
	ctx context.Context, tid int64, selfID, module string, merged, submitted map[string]any,
) error {
	if module != hrDormModuleRoom && module != hrDormModulePublic {
		return nil
	}
	roomKey := strings.TrimSpace(hrDormStr(merged, "房间号"))
	if roomKey == "" {
		roomKey = strings.TrimSpace(hrDormStr(merged, "房屋编号"))
	}
	if roomKey == "" {
		roomKey = strings.TrimSpace(hrDormStr(merged, "宿舍编号"))
	}
	// 在住人数：occupancy 以住宿记录的 room 字段为键，公租房存全称
	occ := h.hrDormOccupancy(ctx, tid)
	/**
	 * 在住人数要对**所有可能的房源键**取最大值。
	 *
	 * 房源档案里同一间房有多个标识：房间号（401）、宿舍编号（宿舍401）、
	 * 房屋地址（两山景苑5-2-6）。住宿记录只存其中一个 —— 只看 roomKey
	 * 会把「住了 2 人的 305」判成 0 人，防缩容与空置拦截双双失效。
	 */
	used := occ[roomKey]
	if used == 0 {
		alts := []string{
			hrDormStr(merged, "房屋地址"),
			hrDormStr(merged, "宿舍编号"),
			hrDormStr(merged, "房间号"),
			"宿舍" + strings.TrimSpace(hrDormStr(merged, "房间号")),
		}
		// 公租房：地址只有小区名，住宿记录存「小区名+门牌」，拼接后再匹配
		if addr := strings.TrimSpace(hrDormStr(merged, "房屋地址")); addr != "" {
			for _, noKey := range []string{"房屋编号", "房间号", "宿舍编号"} {
				if no := strings.TrimSpace(hrDormStr(merged, noKey)); no != "" {
					alts = append(alts, addr+no)
				}
			}
		}
		for _, alt := range alts {
			alt = strings.TrimSpace(alt)
			if alt == "" {
				continue
			}
			if n := occ[alt]; n > 0 {
				used = n
				break
			}
		}
	}

	// 房型变更 → 床位数重算（仅在本次显式提交了房型时）
	if submitted != nil {
		if _, ok := submitted["房型"]; ok {
			if beds := hrDormBedsOfRoomType72(hrDormStr(merged, "房型")); beds > 0 {
				merged["床位数"] = beds
			}
		}
	}
	beds := hrDormInt(merged, "床位数")
	statusRaw := strings.TrimSpace(hrDormStr(merged, "宿舍状态"))
	/**
	 * 空置判定要看**本次是否显式提交了开关**。
	 *
	 * 关闭空置时前端只会把 `空置` 置为 false，而 `宿舍状态` 仍是库里的 "空置"；
	 * 若只看状态就会「关不掉」（真机 A2.8 现象）。显式提交时以开关为准；
	 * 未提交（例如只改备注）才回退到状态字面值。
	 */
	vacant := false
	if submitted != nil {
		if _, has := submitted["空置"]; has {
			vacant = hrDormBool(merged, "空置")
		} else {
			vacant = hrDormBool(merged, "空置") || statusRaw == "空置" || statusRaw == "维修中"
		}
	} else {
		vacant = hrDormBool(merged, "空置") || statusRaw == "空置" || statusRaw == "维修中"
	}
	if err := hrDormValidateCapacity72(roomKey, beds, used, vacant); err != nil {
		return err
	}
	if vacant {
		merged["宿舍状态"] = "空置"
		merged["空置"] = true
		merged["可用床位数"] = 0
		return nil
	}
	delete(merged, "空置")
	free := beds - used
	if free < 0 {
		free = 0
	}
	merged["可用床位数"] = free
	// 关闭空置 / 改房型后状态必须按真实入住重算，不能把 "空置" 留在库里
	derivedStatus, _ := hrDormRoomStatus(beds, used, "")
	merged["宿舍状态"] = derivedStatus
	return nil
}

/**
 * hrDormValidateBedNo72 同一房源同一床位号不可被两条「在住」记录占用。
 *
 * 编辑时通过 selfID 排除自身，避免「保存当前记录的同一床位」被自己拦下。
 */
func (h *HRHandler) hrDormValidateBedNo72(
	ctx context.Context, tid int64, selfID, room, bedNo string,
) error {
	room = strings.TrimSpace(room)
	bedNo = strings.TrimSpace(bedNo)
	if room == "" || bedNo == "" {
		return nil
	}
	var stays []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND module = ?", tid, hrDormModuleStay).Find(&stays).Error; err != nil {
		return nil
	}
	/**
	 * 第 73 轮：床位号不得超出房型床位数。
	 *
	 * 真机出现过「双人间里有 3号床」：旧校验只看重复，不看上限。
	 * 这里先按房源档案取床位数，越界直接 400（存量脏数据由
	 * hrFixBedOverflow73 在启动时按入住顺序重排）。
	 */
	var rooms []HRDormRecord
	_ = h.db.WithContext(ctx).Where("tenant_id = ? AND module IN ?", tid,
		[]string{hrDormModuleRoom, hrDormModulePublic}).Find(&rooms).Error
	for i := range rooms {
		r := &rooms[i]
		if strings.TrimSpace(r.Code) != room &&
			strings.TrimSpace(hrDormStr(r.CustomData, "宿舍编号")) != room &&
			strings.TrimSpace(hrDormStr(r.CustomData, "房屋编号")) != room &&
			strings.TrimSpace(hrDormStr(r.CustomData, "宿舍编号/房屋地址")) != room {
			continue
		}
		beds := hrDormInt(r.CustomData, "床位数")
		if beds <= 0 {
			beds = hrDormBedsOfRoomType72(hrDormStr(r.CustomData, "房型"))
		}
		if beds > 0 {
			if idx := hrBedIndexValue73(bedNo); idx > beds {
				return fmt.Errorf("「%s」只有 %d 个床位，%s 超出范围", room, beds, bedNo)
			}
		}
		break
	}
	for i := range stays {
		s := &stays[i]
		if selfID != "" && s.ID == selfID {
			continue
		}
		if strings.TrimSpace(hrDormStr(s.CustomData, "宿舍编号/房屋地址")) != room {
			continue
		}
		if strings.TrimSpace(hrDormStr(s.CustomData, "住宿状态")) == "已退宿" {
			continue
		}
		if strings.TrimSpace(hrDormStr(s.CustomData, "床位号")) != bedNo {
			continue
		}
		name := strings.TrimSpace(hrDormStr(s.CustomData, "员工姓名"))
		return fmt.Errorf("「%s」的%s已被「%s」占用，请选择其他床位", room, bedNo, name)
	}
	return nil
}

/**
 * hrFixDuplicateBedNo72 存量重复床位号幂等收敛。
 *
 * 同一房源内多条「在住」记录占同一床位号时，按「入住日期升序 → 创建时间」重排为
 * 1号床、2号床…；只处理确实重复的组，正常组零改动。
 */
func (h *HRHandler) hrFixDuplicateBedNo72(ctx context.Context, tid int64) int {
	var stays []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND module = ?", tid, hrDormModuleStay).Find(&stays).Error; err != nil {
		return 0
	}
	byRoom := map[string][]HRDormRecord{}
	for _, s := range stays {
		if strings.TrimSpace(hrDormStr(s.CustomData, "住宿状态")) == "已退宿" {
			continue
		}
		room := strings.TrimSpace(hrDormStr(s.CustomData, "宿舍编号/房屋地址"))
		if room == "" {
			continue
		}
		byRoom[room] = append(byRoom[room], s)
	}
	fixed := 0
	for room, list := range byRoom {
		if len(list) < 2 {
			continue
		}
		// 该房源是否真的存在重复床位号
		seen := map[string]int{}
		dup := false
		for _, s := range list {
			no := strings.TrimSpace(hrDormStr(s.CustomData, "床位号"))
			if no == "" {
				continue
			}
			seen[no]++
			if seen[no] > 1 {
				dup = true
			}
		}
		if !dup {
			continue
		}
		// 按入住日期 + 创建时间升序重排
		for i := 0; i < len(list); i++ {
			for j := i + 1; j < len(list); j++ {
				si := hrDormStr(list[i].CustomData, "入住日期")
				sj := hrDormStr(list[j].CustomData, "入住日期")
				swap := false
				if si != sj {
					swap = si > sj
				} else {
					swap = list[i].CreatedAt.After(list[j].CreatedAt)
				}
				if swap {
					list[i], list[j] = list[j], list[i]
				}
			}
		}
		for idx := range list {
			row := &list[idx]
			want := fmt.Sprintf("%d号床", idx+1)
			cur := strings.TrimSpace(hrDormStr(row.CustomData, "床位号"))
			if cur == want {
				continue
			}
			if row.CustomData == nil {
				row.CustomData = map[string]any{}
			}
			row.CustomData["床位号"] = want
			if err := h.db.WithContext(ctx).Model(row).
				Select("custom_data").Updates(row).Error; err != nil {
				logger.Warnf(ctx, "第 72 轮重排床位失败（%s / %s）: %v", room, cur, err)
				continue
			}
			fixed++
		}
	}
	return fixed
}

/**
 * hrDormValidateAssetName72 资产字典「资产名称」唯一性（新增/编辑都查，编辑排除自身）。
 *
 * 第 72 轮把资产档案定位为**共用物品字典**：同名建档会让配套设施下拉出现重复项。
 */
func (h *HRHandler) hrDormValidateAssetName72(
	ctx context.Context, tid int64, selfID string, data map[string]any,
) error {
	name := strings.TrimSpace(hrDormStr(data, "资产名称"))
	if name == "" {
		return nil
	}
	var rows []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND module = ?", tid, hrDormModuleAsset).
		Find(&rows).Error; err != nil {
		return nil
	}
	for i := range rows {
		r := &rows[i]
		if selfID != "" && r.ID == selfID {
			continue
		}
		other := strings.TrimSpace(hrDormStr(r.CustomData, "资产名称"))
		if other == "" {
			other = strings.TrimSpace(r.Name)
		}
		if strings.EqualFold(other, name) {
			return fmt.Errorf("资产名称 %q 已存在（%s），请勿重复建档", name, r.Code)
		}
	}
	return nil
}

// 保证 gorm 引用不因未来裁剪而失效（本文件用到了 *gorm.DB 的方法集）。
var _ *gorm.DB
