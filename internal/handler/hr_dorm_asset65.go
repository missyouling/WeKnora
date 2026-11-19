// 第 65 轮：资产档案字段契约迁移（幂等）。
//
//  1. 彻底删除维修 4 字段：报修日期 / 维修状态 / 维修费用(元) / 维修说明
//     —— 默认定义、落库 subs、列表列、字段筛选器、设置抽屉、导入模板全部消失；
//  2. 「资产状态」由只读 system 改为可编辑 enum（正常 / 损坏 / 报废），
//     并去掉 builtin 锁定，新增资产时默认「正常」；
//  3. 存量资产记录：清掉维修相关的 custom_data 键，缺资产状态补「正常」。
package handler

import (
	"context"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	hrAssetRepairKey65 = "hr_asset_repair_purge_v65"
	hrAssetStatusKey65 = "hr_asset_status_enum_v65"
)

// hrAssetRepairFields65 需要彻底删除的维修字段（含历史别名）。
var hrAssetRepairFields65 = map[string]bool{
	"报修日期":   true,
	"维修状态":   true,
	"维修费用(元)": true,
	"维修费用":   true,
	"维修说明":   true,
}

// hrMigrateAssetFields65 启动时幂等执行。
func (h *HRHandler) hrMigrateAssetFields65(ctx context.Context) error {
	if !h.hrSystemFlag(ctx, hrAssetRepairKey65) {
		if err := h.hrPurgeAssetRepairSubs65(ctx); err != nil {
			return err
		}
		h.hrMarkSystemFlag(ctx, hrAssetRepairKey65)
	}
	if !h.hrSystemFlag(ctx, hrAssetStatusKey65) {
		if err := h.hrNormalizeAssetStatusSub65(ctx); err != nil {
			return err
		}
		if err := h.hrCleanAssetRecords65(ctx); err != nil {
			return err
		}
		h.hrMarkSystemFlag(ctx, hrAssetStatusKey65)
	}
	return nil
}

// hrDormCategories65 宿舍 scope 下的全部分类（五胶囊同属一个分类）。
func (h *HRHandler) hrDormCategories65(ctx context.Context) ([]types.FleetCategory, error) {
	var cats []types.FleetCategory
	err := h.db.WithContext(ctx).
		Where("scope = ? AND deleted_at IS NULL", hrScopeDorm).Find(&cats).Error
	return cats, err
}

// hrPurgeAssetRepairSubs65 从落库 subs 里删掉维修 4 字段。
func (h *HRHandler) hrPurgeAssetRepairSubs65(ctx context.Context) error {
	cats, err := h.hrDormCategories65(ctx)
	if err != nil {
		return err
	}
	removed := 0
	for i := range cats {
		out := make([]types.FleetCategorySub, 0, len(cats[i].Subs))
		for _, s := range cats[i].Subs {
			if s.Group == hrDormGroupAsset && hrAssetRepairFields65[strings.TrimSpace(s.Name)] {
				removed++
				continue
			}
			out = append(out, s)
		}
		if removed == 0 {
			continue
		}
		cats[i].Subs = out
		cats[i].UpdatedAt = time.Now()
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			return err
		}
	}
	logger.Infof(ctx, "HR asset repair fields purged: %d", removed)
	return nil
}

// hrNormalizeAssetStatusSub65 把「资产状态」重写为可编辑 enum（去 builtin 锁、默认「正常」）。
func (h *HRHandler) hrNormalizeAssetStatusSub65(ctx context.Context) error {
	cats, err := h.hrDormCategories65(ctx)
	if err != nil {
		return err
	}
	for i := range cats {
		subs := cats[i].Subs
		found := false
		for j := range subs {
			if subs[j].Group != hrDormGroupAsset || strings.TrimSpace(subs[j].Name) != "资产状态" {
				continue
			}
			subs[j].DataType = "enum"
			subs[j].Enabled = true
			subs[j].IsDefault = true
			subs[j].Builtin = false
			subs[j].Label = "资产状态"
			found = true
			break
		}
		if !found {
			// 存量分类缺该字段：补在「备注」之后（资产分组末尾）
			item := types.FleetCategorySub{
				Name: "资产状态", Label: "资产状态", Enabled: true, IsDefault: true,
				DataType: "enum", Group: hrDormGroupAsset, GroupLabel: hrDormGroupAsset,
			}
			subs = append(subs, item)
		}
		cats[i].Subs = subs
		cats[i].UpdatedAt = time.Now()
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			return err
		}
	}
	logger.Infof(ctx, "HR asset status normalized to editable enum: %d categories", len(cats))
	return nil
}

// hrCleanAssetRecords65 清掉存量资产记录里的维修键，并补齐资产状态默认值。
func (h *HRHandler) hrCleanAssetRecords65(ctx context.Context) error {
	var rows []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("module = ?", hrDormModuleAsset).Find(&rows).Error; err != nil {
		return err
	}
	touched := 0
	for i := range rows {
		d := rows[i].CustomData
		if d == nil {
			continue
		}
		changed := false
		for k := range d {
			if hrAssetRepairFields65[strings.TrimSpace(k)] {
				delete(d, k)
				changed = true
			}
		}
		st := strings.TrimSpace(hrDormStr(d, "资产状态"))
		if st == "" {
			d["资产状态"] = "正常"
			changed = true
		}
		if !changed {
			continue
		}
		rows[i].CustomData = d
		rows[i].Status = hrDormStatusOf(hrDormModuleAsset, d)
		if err := h.db.WithContext(ctx).Save(&rows[i]).Error; err != nil {
			return err
		}
		touched++
	}
	logger.Infof(ctx, "HR asset records cleaned: %d", touched)
	return nil
}
