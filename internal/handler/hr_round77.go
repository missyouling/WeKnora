package handler

// 第 77 轮：表计类型同源体系。
//
// 一条「表计类型」字段贯穿三类档案与三张核算表：
//   · 工业表计（utility_meters.meter_kind）= 工商业（旧值 production 归一）
//   · 宿舍档案.subs「表计类型」= 宿舍（40 间存量补齐）
//   · 公租房档案.subs「表计类型」= 公租房（19 套存量补齐）
// 核算页的筛选下拉直接从这三处真实数据汇总，不再写死枚举。

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	hrMeterTypeField77 = "表计类型"
	hrMeterTypeDorm77  = "宿舍"
	hrMeterTypePub77   = "公租房"
	hrMeterTypeBiz77   = "工商业"
)

// hrSeedMeterType77 幂等补齐宿舍/公租房档案的「表计类型」字段与存量取值。
func (h *HRHandler) hrSeedMeterType77(ctx context.Context, tid int64) (dorm, public int) {
	var cat types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND scope = ? AND name = ? AND deleted_at IS NULL", tid, "hr-dorm", "宿舍管理").
		First(&cat).Error; err != nil {
		logger.Warnf(ctx, "第 77 轮读取宿舍字段配置失败: %v", err)
		return
	}
	subs := append([]types.FleetCategorySub(nil), cat.Subs...)
	for _, group := range []string{"宿舍档案", "公租房档案"} {
		if !hrHasSubInGroup77(subs, group, hrMeterTypeField77) {
			// 插到该分组末尾：字段顺序 = 用户当前顺序 + 新字段（不打断既有列）
			subs = append(subs, types.FleetCategorySub{
				Name:      hrMeterTypeField77,
				Label:     hrMeterTypeField77,
				Enabled:   true,
				IsDefault: true,
				DataType:  "text",
				Width:     90,
				Group:     group,
				GroupLabel: group,
			})
		}
	}
	// 只在确有变化时写库（避免每次启动刷新 updated_at）
	changed := len(subs) != len(cat.Subs)
	if changed {
		if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
			Where("id = ?", cat.ID).
			Update("subs", mustJSON77(subs)).Error; err != nil {
			logger.Warnf(ctx, "第 77 轮写入宿舍字段配置失败: %v", err)
		}
	}

	// 存量房源补齐取值（只填空值，尊重用户后续手工改过的类型）
	var rows []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND module IN ?", tid, []string{hrDormModuleRoom, hrDormModulePublic}).
		Find(&rows).Error; err != nil {
		return
	}
	for i := range rows {
		row := &rows[i]
		want := hrMeterTypeDorm77
		if row.Module == hrDormModulePublic {
			want = hrMeterTypePub77
		}
		// 只补空值：首轮迁移把 40 间宿舍 / 19 套公租房统一填成各自类型；
		// 之后用户手工改过的类型必须保留（重启不能被覆盖）。
		if strings.TrimSpace(hrDormStr(row.CustomData, hrMeterTypeField77)) != "" {
			continue
		}
		if row.CustomData == nil {
			row.CustomData = map[string]any{}
		}
		row.CustomData[hrMeterTypeField77] = want
		// JSONB 必须走 serializer（map 直写会被 database/sql 拒绝）
		if err := h.db.WithContext(ctx).Model(row).Select("custom_data").Updates(row).Error; err == nil {
			if row.Module == hrDormModulePublic {
				public++
			} else {
				dorm++
			}
		}
	}
	return
}

// hrNormalizeMeterKinds77 工业表计旧值 production → 工商业（幂等）。
func (h *HRHandler) hrNormalizeMeterKinds77(ctx context.Context, tid int64) int64 {
	res := h.db.WithContext(ctx).Model(&types.UtilityMeter{}).
		Where("tenant_id = ? AND meter_kind = ?", tid, "production").
		Update("meter_kind", hrMeterTypeBiz77)
	if res.Error != nil {
		logger.Warnf(ctx, "第 77 轮归一工业表计类型失败: %v", res.Error)
		return 0
	}
	return res.RowsAffected
}

func hrHasSubInGroup77(subs []types.FleetCategorySub, group, name string) bool {
	for _, s := range subs {
		if strings.TrimSpace(s.Group) == group && strings.TrimSpace(s.Name) == name {
			return true
		}
	}
	return false
}

func mustJSON77(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// hrRepair77 第 77 轮启动迁移入口（幂等）。
func (h *HRHandler) hrRepair77(ctx context.Context) {
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Distinct("tenant_id").Pluck("tenant_id", &tids).Error; err != nil {
		return
	}
	for _, tid := range tids {
		if tid <= 0 {
			continue
		}
		dorm, public := h.hrSeedMeterType77(ctx, tid)
		kinds := h.hrNormalizeMeterKinds77(ctx, tid)
		if dorm+public > 0 || kinds > 0 {
			logger.Infof(ctx, "第 77 轮数据修复（tenant=%d）：宿舍表计类型 %d 间、公租房 %d 套、工业表计类型归一 %d 张",
				tid, dorm, public, kinds)
		}
	}
}
