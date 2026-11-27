package handler

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

/**
 * 第 75 轮启动幂等修复（数据安全优先）。
 *
 *  ① 恢复工业表计定义：电 6 / 水 4 / 气 1（按分类 + 表号 upsert，绝不重复建）；
 *  ② 修正宿舍表号错配：持睿 403~418 的 W0xx 电表号 ↔ E0xx 水表号互换，
 *     并同步房源档案「宿舍电表号 / 宿舍水表号」；起止度数与金额零改动；
 *  ③ 「本期水费」数据类型统一为 amount（fleet_categories.subs 幂等迁移）；
 *  ④ 历史月份费用清单按新单位匹配口径重算刷写（只覆盖已存在的账单行）。
 */

// hrIndustrialMeter75 一张工业表计的期望定义。
type hrIndustrialMeter75 struct {
	Category string
	MeterNo  string
	Alias    string
	MeterType string // 电表 normal|time；水表 total|sub|fire；气表 normal
	Rate     float64
	Price    float64
	UseUnit  string
	OwnerUnit string
	Manager  string
	MeterMode string
}

var hrIndustrialMeters75 = []hrIndustrialMeter75{
	// 工业电表（6 张，保持当前正确定义）
	{"electricity", "CR-E001", "持睿工业分表1", "time", 200, 0, "持睿汽车部件有限公司", "持睿汽车部件有限公司", "李锶雨", "manual"},
	{"electricity", "CR-E002", "持睿工业分表2", "time", 150, 0, "持睿汽车部件有限公司", "持睿汽车部件有限公司", "李锶雨", "manual"},
	{"electricity", "CR-E003", "持睿工业分表3", "time", 200, 0, "持睿汽车部件有限公司", "持睿汽车部件有限公司", "李锶雨", "manual"},
	{"electricity", "XD-E001", "星达配电分表1", "normal", 100, 0, "重庆星达铜业有限公司", "重庆星达铜业有限公司", "文德强", "manual"},
	{"electricity", "XD-E002", "星达配电分表2", "normal", 300, 0, "重庆星达铜业有限公司", "重庆星达铜业有限公司", "文德强", "manual"},
	{"electricity", "XD-E003", "星达配电分表3", "normal", 200, 0, "重庆星达铜业有限公司", "重庆星达铜业有限公司", "文德强", "manual"},
	// 工业水表（4 张，按真实水费核算记录恢复）
	{"water", "CRM01", "持睿水表分表1", "sub", 1, 5.22, "持睿汽车部件有限公司", "重庆星达铜业有限公司", "李锶雨", "manual"},
	{"water", "CRM02", "持睿水表分表2", "sub", 1, 5.22, "持睿汽车部件有限公司", "重庆星达铜业有限公司", "李锶雨", "manual"},
	{"water", "W032", "星达自来水总表", "total", 1, 5.22, "重庆星达铜业有限公司", "重庆星达铜业有限公司", "文德强", "manual"},
	{"water", "W033", "星达消防总表", "fire", 1, 5.22, "重庆星达铜业有限公司", "重庆星达铜业有限公司", "文德强", "manual"},
	// 工业气表（1 张）
	{"gas", "G001", "食堂气表", "normal", 1, 3.4, "重庆星达铜业有限公司", "重庆星达铜业有限公司", "李永娇", "auto"},
}

// RestoreIndustrialMeters75 幂等恢复 11 张工业表计，返回新增/修正条数。
func (h *UtilityHandler) RestoreIndustrialMeters75(ctx context.Context, tenantID int64) (created, updated int) {
	for i, spec := range hrIndustrialMeters75 {
		var row types.UtilityMeter
		err := h.db.WithContext(ctx).
			Where("tenant_id = ? AND category = ? AND meter_no = ? AND deleted_at IS NULL",
				tenantID, spec.Category, spec.MeterNo).
			First(&row).Error
		if err == gorm.ErrRecordNotFound {
			now := timeNowUTC()
			row = types.UtilityMeter{
				ID: uuid.NewString(), TenantID: tenantID,
				Category: spec.Category, Alias: spec.Alias, MeterNo: spec.MeterNo,
				MeterType: spec.MeterType, MeterKind: "production",
				OwnerUnit: spec.OwnerUnit, UseUnit: spec.UseUnit,
				Rate: spec.Rate, DefaultUnitPrice: spec.Price,
				Manager: spec.Manager, MeterMode: spec.MeterMode,
				SortOrder: i, Enabled: true,
				CreatedAt: now, UpdatedAt: now,
			}
			if e := h.db.WithContext(ctx).Create(&row).Error; e != nil {
				logger.Warnf(ctx, "第 75 轮恢复工业表计 %s 失败: %v", spec.MeterNo, e)
				continue
			}
			created++
			continue
		}
		if err != nil {
			logger.Warnf(ctx, "第 75 轮查询工业表计 %s 失败: %v", spec.MeterNo, err)
			continue
		}
		// 只修正定义字段，不动 sort/启用状态以外的历史引用
		changed := row.Alias != spec.Alias || row.MeterType != spec.MeterType ||
			row.MeterKind != "production" || row.Rate != spec.Rate ||
			row.DefaultUnitPrice != spec.Price || row.UseUnit != spec.UseUnit ||
			row.OwnerUnit != spec.OwnerUnit || row.Manager != spec.Manager ||
			row.MeterMode != spec.MeterMode
		if !changed {
			continue
		}
		row.Alias, row.MeterType, row.MeterKind = spec.Alias, spec.MeterType, "production"
		row.Rate, row.DefaultUnitPrice = spec.Rate, spec.Price
		row.UseUnit, row.OwnerUnit, row.Manager, row.MeterMode = spec.UseUnit, spec.OwnerUnit, spec.Manager, spec.MeterMode
		row.UpdatedAt = timeNowUTC()
		if e := h.db.WithContext(ctx).Model(&row).
			Select("alias", "meter_type", "meter_kind", "rate", "default_unit_price",
				"use_unit", "owner_unit", "manager", "meter_mode", "updated_at").
			Updates(&row).Error; e != nil {
			logger.Warnf(ctx, "第 75 轮修正工业表计 %s 失败: %v", spec.MeterNo, e)
			continue
		}
		updated++
	}
	return
}

/** 表号前缀互换：持睿 403~418 的电表误用水表号（W021…），水表误用电表号（E024…）。 */
func swapDormMeterPrefix75(no, wantPrefix string) string {
	s := strings.TrimSpace(no)
	if len(s) < 2 || !strings.EqualFold(s[:1], "w") && !strings.EqualFold(s[:1], "e") {
		return no
	}
	cur := strings.ToUpper(s[:1])
	if cur == wantPrefix {
		return no
	}
	return wantPrefix + s[1:]
}

// hrFixDormMeterNumbers75 修正持睿宿舍电/水表号错配并同步房源档案。
func (h *HRHandler) hrFixDormMeterNumbers75(ctx context.Context, tid int64) (meters, rooms int) {
	var list []types.UtilityMeter
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND category IN ? AND deleted_at IS NULL AND meter_kind = ?",
			tid, []string{"electricity", "water"}, "dorm").
		Find(&list).Error; err != nil {
		logger.Warnf(ctx, "第 75 轮读取宿舍表计失败: %v", err)
		return
	}
	want := map[string]string{"electricity": "E", "water": "W"}
	// 键 = 别名 + "#" + 介质：同一间房的电表与水表别名相同（持睿403）
	byAlias := map[string]*types.UtilityMeter{}
	for i := range list {
		m := &list[i]
		byAlias[m.Alias+"#"+m.Category] = m
		no := strings.TrimSpace(m.MeterNo)
		if len(no) < 2 {
			continue
		}
		prefix := strings.ToUpper(no[:1])
		if prefix != "W" && prefix != "E" {
			continue
		}
		// 只修持睿宿舍的错配；星达宿舍仅水表历史错号（E314）需要改
		if !strings.Contains(m.Alias, "持睿") && !unitNamesMatch(m.UseUnit, "持睿汽车") && m.Category != "water" {
			continue
		}
		next := swapDormMeterPrefix75(no, want[m.Category])
		if next == no {
			continue
		}
		// 幂等防撞：新表号已被占用则跳过，绝不制造重复表
		var dup int64
		h.db.WithContext(ctx).Model(&types.UtilityMeter{}).
			Where("tenant_id = ? AND category = ? AND meter_no = ? AND id <> ? AND deleted_at IS NULL",
				tid, m.Category, next, m.ID).Count(&dup)
		if dup > 0 {
			continue
		}
		if err := h.db.WithContext(ctx).Model(&types.UtilityMeter{}).
			Where("id = ?", m.ID).Update("meter_no", next).Error; err == nil {
			m.MeterNo = next
			meters++
		}
	}

	var rows []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND module = ?", tid, hrDormModuleRoom).Find(&rows).Error; err != nil {
		return
	}
	for i := range rows {
		row := &rows[i]
		company := strings.TrimSpace(hrDormStr(row.CustomData, "所属公司"))
		if !unitNamesMatch(company, "持睿汽车") {
			continue
		}
		code := strings.TrimSpace(hrDormStr(row.CustomData, "宿舍编号"))
		digits := strings.TrimPrefix(code, "宿舍")
		if digits == "" {
			continue
		}
		alias := "持睿" + digits
		elec := byAlias[alias+"#electricity"]
		water := byAlias[alias+"#water"]
		changed := false
		if elec != nil && hrDormStr(row.CustomData, "宿舍电表号") != elec.MeterNo {
			row.CustomData["宿舍电表号"] = elec.MeterNo
			changed = true
		}
		if water != nil && hrDormStr(row.CustomData, "宿舍水表号") != water.MeterNo {
			row.CustomData["宿舍水表号"] = water.MeterNo
			changed = true
		}
		if !changed {
			continue
		}
		// JSONB 必须走 serializer（Update("custom_data", map) 会被 database/sql 拒绝）
		if err := h.db.WithContext(ctx).Model(row).Select("custom_data").Updates(row).Error; err == nil {
			rooms++
		}
	}
	return
}

// hrFixWaterFeeType75 把「本期水费」统一为 amount（幂等）。
func (h *HRHandler) hrFixWaterFeeType75(ctx context.Context, tid int64) (fixed int) {
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND scope = ? AND deleted_at IS NULL", tid, "energy").
		Find(&cats).Error; err != nil {
		return 0
	}
	for i := range cats {
		cat := &cats[i]
		changed := false
		for j := range cat.Subs {
			sub := &cat.Subs[j]
			name := strings.TrimSpace(sub.Name)
			label := strings.TrimSpace(sub.Label)
			if name != "current_water_fee" && name != "本期水费" && label != "本期水费" {
				continue
			}
			if sub.DataType == "amount" {
				continue
			}
			sub.DataType = "amount"
			changed = true
		}
		if !changed {
			continue
		}
		cat.UpdatedAt = timeNowUTC()
		if err := h.db.WithContext(ctx).Model(cat).Select("subs", "updated_at").Updates(cat).Error; err == nil {
			fixed++
		}
	}
	return
}

// hrRepair75 第 75 轮启动幂等修复入口。
func (h *HRHandler) hrRepair75(ctx context.Context) {
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Distinct("tenant_id").Pluck("tenant_id", &tids).Error; err != nil {
		logger.Warnf(ctx, "第 75 轮读取租户失败: %v", err)
		return
	}
	dirty := false
	for _, tid := range tids {
		if tid <= 0 {
			continue
		}
		uh := &UtilityHandler{db: h.db}
		created, updated := uh.RestoreIndustrialMeters75(ctx, tid)
		meters, rooms := h.hrFixDormMeterNumbers75(ctx, tid)
		typesFixed := h.hrFixWaterFeeType75(ctx, tid)
		// 历史抄表快照补名（只改 meter_name，不动任何数值）
		named := h.hrSyncMeterItemNames75(ctx, tid)
		if created > 0 || updated > 0 || meters > 0 || rooms > 0 || typesFixed > 0 || named > 0 {
			dirty = true
			logger.Infof(ctx,
				"第 75 轮数据修复（tenant=%d）：工业表计新增 %d/修正 %d、宿舍表号 %d 张、房源同步 %d 间、本期水费类型 %d 处、抄表名 %d 条",
				tid, created, updated, meters, rooms, typesFixed, named)
		}
	}
	/**
	 * 只有本次真的修了数据才重算账单：hrRepair75 每次启动都会跑，
	 * 无脑重算会把用户手工调整过的 billing_records 反复覆盖。
	 */
	if dirty {
		h.hrRegenerateBilling75(ctx)
	}
}

// hrSyncMeterItemNames75 历史抄表行的表计名称与当前别名对齐（只改名称列）。
func (h *HRHandler) hrSyncMeterItemNames75(ctx context.Context, tid int64) (n int) {
	var meters []types.UtilityMeter
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND deleted_at IS NULL AND category IN ?", tid,
			[]string{"electricity", "water", "gas"}).
		Find(&meters).Error; err != nil {
		return 0
	}
	for _, m := range meters {
		res := h.db.WithContext(ctx).Model(&types.UtilityMeterItem{}).
			Where("meter_id = ? AND (meter_name IS NULL OR meter_name = '' OR meter_name <> ?)", m.ID, m.Alias).
			Update("meter_name", m.Alias)
		if res.Error == nil && res.RowsAffected > 0 {
			n += int(res.RowsAffected)
		}
	}
	return n
}

// hrRegenerateBilling75 按单位归一后的口径重算历史月费用清单（只刷已存在的账单行）。
func (h *HRHandler) hrRegenerateBilling75(ctx context.Context) {
	var months []string
	if err := h.db.WithContext(ctx).Model(&types.BillingRecord{}).
		Distinct("month").Where("deleted_at IS NULL").Order("month ASC").
		Pluck("month", &months).Error; err != nil {
		logger.Warnf(ctx, "第 75 轮读取账单月份失败: %v", err)
		return
	}
	bh := &BillingHandler{db: h.db}
	for _, month := range months {
		if strings.TrimSpace(month) == "" {
			continue
		}
		if n, err := bh.RegenerateBillingByPeriod(ctx, month); err != nil {
			logger.Warnf(ctx, "第 75 轮重算费用清单 %s 失败: %v", month, err)
		} else if n > 0 {
			logger.Infof(ctx, "第 75 轮重算费用清单 %s：%d 个使用单位", month, n)
		}
	}
}
