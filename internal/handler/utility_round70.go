package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

/**
 * 第 70 轮：核算侧表计源 = 工商业表（utility_meters）∪ 房源档案定义的表。
 *
 * 背景：水电气表的**定义权**已从「能耗 → 表计设置」移交到「宿舍 / 公租房档案」，
 * 表计设置只剩工商业分组。但核算页（电费 / 水费 / 气费）仍要能选到宿舍与公租房的表，
 * 否则新建抄表无从下手。
 *
 * 实现约束（数据安全红线）：
 *   · **只追加不修改** —— 已有表号（E001–E020 / 老水表）继续走 utility_meters 原记录，
 *     单价、倍率、历史引用全部保持不变；
 *   · 按 `meter_no` 去重，档案项仅在「库里没有同号表」时补进来
 *     （E401–E420、公租房水表/气表这类新增项）；
 *   · 不写库：返回项的 ID 前缀 `hr-`，只作为前端下拉的 value，落库仍是 meter_no。
 */
func (h *UtilityHandler) appendPropertyMeters70(
	ctx context.Context, tenantID uint64, category string, meters []types.UtilityMeter,
) []types.UtilityMeter {
	tid := int64(tenantID)
	var rooms []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND module IN ?", tid,
			[]string{hrDormModuleRoom, hrDormModulePublic}).Find(&rooms).Error; err != nil {
		return meters
	}
	seen := make(map[string]bool, len(meters))
	for _, m := range meters {
		seen[strings.ToUpper(strings.TrimSpace(m.MeterNo))] = true
	}

	add := func(room *HRDormRecord, nameKey, noKey, priceKey, rateKey, unitKey string) {
		no := strings.TrimSpace(hrDormStr(room.CustomData, noKey))
		if no == "" || seen[strings.ToUpper(no)] {
			return
		}
		name := strings.TrimSpace(hrDormStr(room.CustomData, nameKey))
		if name == "" {
			name = no
		}
		kind := hrDormModuleRoom
		if room.Module == hrDormModulePublic {
			kind = hrDormModulePublic
		}
		// 公租房用「租赁单位」，空则回落「所属公司」；宿舍直接「所属公司」
		unit := strings.TrimSpace(hrDormStr(room.CustomData, unitKey))
		if unit == "" {
			unit = strings.TrimSpace(hrDormStr(room.CustomData, "所属公司"))
		}
		rate := hrDormFloat(room.CustomData, rateKey)
		if rate <= 0 {
			rate = 1
		}
		seen[strings.ToUpper(no)] = true
		meters = append(meters, types.UtilityMeter{
			ID:               fmt.Sprintf("hr-%s-%s", kind, no),
			TenantID:         tid,
			Category:         category,
			Alias:            name,
			MeterNo:          no,
			MeterKind:        kind,
			Rate:             rate,
			DefaultUnitPrice: hrDormFloat(room.CustomData, priceKey),
			UseUnit:          unit,
			OwnerUnit:        unit,
			MeterMode:        "manual",
			Enabled:          true,
		})
	}

	for i := range rooms {
		room := &rooms[i]
		isPublic := room.Module == hrDormModulePublic
		switch category {
		case "electricity":
			// 公租房档案**不定义电表**（需求明确），只取宿舍
			if !isPublic {
				add(room, "宿舍电表名称", "宿舍电表号", "电费标准(元/度)", "电表倍率", "所属公司")
			}
		case "water":
			if isPublic {
				add(room, "公租房水表名称", "公租房水表号", "水费单价(元/m³)", "水表倍率", "租赁单位")
			} else {
				add(room, "宿舍水表名称", "宿舍水表号", "水费单价(元/m³)", "水表倍率", "所属公司")
			}
		case "gas":
			if isPublic {
				add(room, "公租房气表名称", "公租房气表号", "气费标准(元/m³)", "气表倍率", "租赁单位")
			}
		}
	}
	return meters
}
