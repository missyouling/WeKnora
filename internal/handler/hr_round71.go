package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

/**
 * 第 71 轮「表房合一」：把持睿 4 楼宿舍的房档表计对齐回原能耗真实表计。
 *
 * 背景（第 70 轮遗留的冲突）：
 *   · 原能耗表计里有 11 张持睿宿舍表（`持睿403`=E021/W021 … `持睿418`=E031/W031）；
 *   · 第 70 轮按「4XX」规则给房档生成了 E403/W403 这类虚拟表号，于是同一间房
 *     出现了两套表号：历史抄表挂在 E021，房档/新增抄表却指向 E403。
 *
 * 本轮裁决：**表房合一** —— 房档改用原持睿真实表计（表名/表号/单价/倍率），
 * 所属公司改为 `持睿汽车部件有限公司`，虚拟表号彻底退役。
 *
 * 硬约束：
 *   · 不动 `utility_meters` 一条记录（历史抄表与费用清单按它挂账）；
 *   · 不动 `utility_meter_items` 任何明细（电 123 / 水 151 / 气 86 条、金额零波动）；
 *   · 只改宿舍档案的 `custom_data`，且值已一致时跳过（幂等）。
 */
func (h *HRHandler) hrAlignChiruiDormMeters71(ctx context.Context, tid int64) (int, error) {
	var meters []typesUtilityMeterRow
	if err := h.db.WithContext(ctx).Raw(
		`SELECT id, category, alias, meter_no, rate, default_unit_price, use_unit, meter_kind, enabled
		   FROM utility_meters
		  WHERE tenant_id = ? AND deleted_at IS NULL AND meter_kind = ? AND alias LIKE ?`,
		tid, hrDormModuleRoom, "持睿%").Scan(&meters).Error; err != nil {
		return 0, err
	}
	if len(meters) == 0 {
		return 0, nil
	}

	// 房间号 → 电表/水表（alias 形如「持睿403」，取其中数字段）
	type meterPair struct {
		electric *typesUtilityMeterRow
		water    *typesUtilityMeterRow
	}
	pairs := map[string]*meterPair{}
	for i := range meters {
		m := &meters[i]
		no := hrDormDigitsOf71(m.Alias)
		if no == "" {
			continue
		}
		if pairs[no] == nil {
			pairs[no] = &meterPair{}
		}
		switch strings.TrimSpace(m.Category) {
		case "electricity":
			pairs[no].electric = m
		case "water":
			pairs[no].water = m
		}
	}

	var rooms []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND module = ?", tid, hrDormModuleRoom).
		Find(&rooms).Error; err != nil {
		return 0, err
	}

	aligned := 0
	for i := range rooms {
		room := &rooms[i]
		no := strings.TrimSpace(hrDormStr(room.CustomData, "房间号"))
		pair := pairs[no]
		if pair == nil || (pair.electric == nil && pair.water == nil) {
			continue
		}
		if room.CustomData == nil {
			room.CustomData = map[string]any{}
		}
		changed := false
		set := func(key string, v any) {
			old := strings.TrimSpace(fmt.Sprint(room.CustomData[key]))
			want := strings.TrimSpace(fmt.Sprint(v))
			if old == want {
				return
			}
			room.CustomData[key] = v
			changed = true
		}
		// 所属公司取表计的使用单位（第 70 轮已归一到单位设置全名）
		unit := ""
		if pair.electric != nil {
			unit = strings.TrimSpace(pair.electric.UseUnit)
		}
		if unit == "" && pair.water != nil {
			unit = strings.TrimSpace(pair.water.UseUnit)
		}
		if unit != "" {
			set("所属公司", unit)
		}
		if m := pair.electric; m != nil {
			set("宿舍电表名称", strings.TrimSpace(m.Alias))
			set("宿舍电表号", strings.TrimSpace(m.MeterNo))
			if m.DefaultUnitPrice > 0 {
				set("电费标准(元/度)", m.DefaultUnitPrice)
			}
			if m.Rate > 0 {
				set("电表倍率", m.Rate)
			}
		}
		if m := pair.water; m != nil {
			set("宿舍水表名称", strings.TrimSpace(m.Alias))
			set("宿舍水表号", strings.TrimSpace(m.MeterNo))
			if m.DefaultUnitPrice > 0 {
				set("水费单价(元/m³)", m.DefaultUnitPrice)
			}
			if m.Rate > 0 {
				set("水表倍率", m.Rate)
			}
		}
		if !changed {
			continue
		}
		// JSONB 必须走 serializer：Model(row).Select("custom_data").Updates(row)
		if err := h.db.WithContext(ctx).Model(room).
			Select("custom_data").Updates(room).Error; err != nil {
			logger.Warnf(ctx, "第 71 轮持睿表房合一对齐失败（房间 %s）: %v", no, err)
			continue
		}
		aligned++
	}
	return aligned, nil
}

// hrDormDigitsOf71 取字符串里的第一段数字（`持睿403` → `403`）。
func hrDormDigitsOf71(s string) string {
	var b strings.Builder
	started := false
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			b.WriteRune(ch)
			started = true
			continue
		}
		if started {
			break
		}
	}
	return b.String()
}

// hrRepair71 第 71 轮启动幂等修复（持睿 11 间宿舍的表房合一对齐）。
func (h *HRHandler) hrRepair71(ctx context.Context) {
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HRDormRecord{}).
		Distinct("tenant_id").Pluck("tenant_id", &tids).Error; err != nil {
		logger.Warnf(ctx, "第 71 轮读取租户失败: %v", err)
		return
	}
	for _, tid := range tids {
		n, err := h.hrAlignChiruiDormMeters71(ctx, tid)
		if err != nil {
			logger.Warnf(ctx, "第 71 轮持睿表房合一失败（tenant=%d）: %v", tid, err)
			continue
		}
		if n > 0 {
			logger.Infof(ctx, "第 71 轮持睿表房合一（tenant=%d）：对齐 %d 间宿舍", tid, n)
		}
	}
}

// 显式引用 types，避免未来删字段时静默丢编译约束。
var _ = types.UtilityMeter{}
