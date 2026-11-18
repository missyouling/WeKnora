// 第 62 轮：宿舍字段契约 v2 迁移 + 劳动合同签约公司归一。
package handler

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm/clause"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	hrDormSubsKey62      = "hr_dorm_subs_v62"
	hrContractCompany62  = "hr_contract_company_v62"
	hrContractCompanyName = "重庆星达铜业有限公司"
)

// hrMigrateDormSubs62 用 v2 字段契约重建宿舍分类（一次性，之后尊重用户配置）。
//
// 为什么整份重建：第 62 轮把「租金(元/月)/物业(元/月)/首月费用/季度」改成
// 「房屋租金/物业费/月费用单价/季度租金」并新增费用项目 12 字段，
// 逐字段 merge 会留下两套同义列（旧列还占着默认表头）。
func (h *HRHandler) hrMigrateDormSubs62(ctx context.Context) error {
	if h.hrSystemFlag(ctx, hrDormSubsKey62) {
		return nil
	}
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).Where("scope = ? AND deleted_at IS NULL", hrScopeDorm).Find(&cats).Error; err != nil {
		return err
	}
	for i := range cats {
		cats[i].Subs = hrDormSeedSubs()
		cats[i].UpdatedAt = time.Now()
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			return err
		}
	}
	h.hrMarkSystemFlag(ctx, hrDormSubsKey62)
	logger.Infof(ctx, "HR dorm subs migrated to v62: %d categories", len(cats))
	return nil
}

// hrFixContractCompany62 劳动合同「签约公司」统一为重庆星达铜业有限公司。
//
// 只补写空值与旧值（历史测试数据写的别家公司），已是目标值的不动，幂等。
func (h *HRHandler) hrFixContractCompany62(ctx context.Context) error {
	if h.hrSystemFlag(ctx, hrContractCompany62) {
		return nil
	}
	var rows []HREmployee
	if err := h.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return err
	}
	fixed := 0
	for i := range rows {
		changed := false
		if rows[i].CustomData == nil {
			rows[i].CustomData = map[string]any{}
		}
		for _, key := range []string{"签约公司", "现合同签约公司", "合同签约公司"} {
			cur := strings.TrimSpace(hrDormStr(rows[i].CustomData, key))
			if cur == hrContractCompanyName {
				continue
			}
			// 只处理「合同类」行：有合同编号/合同类型/签约公司任一键
			if cur == "" && key != "签约公司" {
				continue
			}
			if key == "签约公司" && cur == "" && hrDormStr(rows[i].CustomData, "合同编号") == "" && hrDormStr(rows[i].CustomData, "现合同编号") == "" {
				continue
			}
			rows[i].CustomData[key] = hrContractCompanyName
			changed = true
		}
		if changed {
			// custom_data 是 datatypes.JSON：必须整行 Save 让它走序列化，
			// Update("custom_data", map) 会报 unsupported type map[string]interface{}。
			if err := h.db.WithContext(ctx).Save(&rows[i]).Error; err != nil {
				return err
			}
			fixed++
		}
	}
	h.hrMarkSystemFlag(ctx, hrContractCompany62)
	logger.Infof(ctx, "HR contract company normalized: %d rows", fixed)
	return nil
}

// hrSystemFlag 读布尔型系统标记（不存在 / 值非法 = false）。
func (h *HRHandler) hrSystemFlag(ctx context.Context, key string) bool {
	var row types.SystemSetting
	if err := h.db.WithContext(ctx).Where("key = ?", key).First(&row).Error; err != nil {
		return false
	}
	if row.ValueType != "bool" {
		return false
	}
	ok, err := row.AsBool()
	return err == nil && ok
}

// hrMarkSystemFlag 写布尔型系统标记（幂等 upsert）。
func (h *HRHandler) hrMarkSystemFlag(ctx context.Context, key string) {
	value, _ := json.Marshal(true)
	_ = h.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "value_type", "updated_at"}),
	}).Create(&types.SystemSetting{
		Key: key, Value: types.JSON(value), ValueType: "bool",
		Category: "hr", Description: "第 62 轮一次性迁移标记",
	}).Error
}
