// 车辆主数据回填与「400 请选择车辆」死结解除（第 28 轮 Q3）。
//
// 事故：`normalizeFleetRecord` 要求记录型必须有 vehicle_id，而 `fleet_vehicles`
// 是空表 → 前端下拉无选项 → 任何「新增记录」都被 400 拦下。实测：
//
//	POST /api/v1/fleet/records {"record_type":"tire",...}
//	→ 400 {"code":1000,"message":"请选择车辆"}
//
// 修复三条：
//  1. 从发票库 items[].plate_no 与存量 fleet_records.data 里收集车牌，幂等补建车辆；
//  2. 记录保存时 vehicle_id 为空 → 用 data 里的车牌自动查/建车辆并回填；
//  3. 连车牌都没有（公司级许可证、待补录）→ 允许 vehicle_id 为空落库。
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// plateKeys 车牌号在 data / fields 里出现过的所有键名。
var plateKeys = []string{"车牌号", "车牌号码", "plate_no", "plateNo"}

// pickPlateFromRecordData 从记录的动态字段里取车牌号（去空白，非字符串返回空）。
func pickPlateFromRecordData(data map[string]any) string {
	for _, k := range plateKeys {
		raw, ok := data[k]
		if !ok {
			continue
		}
		s, ok := raw.(string)
		if !ok {
			continue
		}
		if v := strings.TrimSpace(s); v != "" {
			return v
		}
	}
	return ""
}

// resolveFleetVehicle 确保记录有 vehicle_id：
//  1. 已有 vehicle_id → 原样返回；
//  2. 有车牌号 → 按 (tenant, plate_no) 查找，缺则补建；
//  3. 都没有 → 留空（允许落库，不再 400）。
func (h *FleetHandler) resolveFleetVehicle(ctx context.Context, tenantID int64, rec *types.FleetRecord) {
	if rec == nil || strings.TrimSpace(rec.VehicleID) != "" {
		return
	}
	plate := pickPlateFromRecordData(rec.Data)
	if plate == "" {
		return
	}
	vehicle, err := h.ensureFleetVehicleByPlate(ctx, tenantID, plate)
	if err != nil {
		logger.Warnf(ctx, "resolve fleet vehicle %q failed: %v", plate, err)
		return
	}
	if vehicle != nil {
		rec.VehicleID = vehicle.ID
	}
}

// normalizePlate 车牌归一：去空白、字母转大写。
//
// 真机数据里「渝c81022」与「渝C81022」并存（OCR/手工各写一种），不归一会
// 建出两条车辆主数据，派生台账按车牌筛选就只命中一半。
func normalizePlate(plate string) string {
	p := strings.TrimSpace(plate)
	if p == "" {
		return ""
	}
	// 车牌字母位固定大写；中文省份简称不受影响
	return strings.ToUpper(p)
}

// ensureFleetVehicleByPlate 按车牌查车辆，不存在则建一条最小档案（幂等）。
func (h *FleetHandler) ensureFleetVehicleByPlate(ctx context.Context, tenantID int64, plate string) (*types.FleetVehicle, error) {
	plate = normalizePlate(plate)
	if plate == "" {
		return nil, nil
	}
	var rows []types.FleetVehicle
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND plate_no = ? AND deleted_at IS NULL", tenantID, plate).
		Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return &rows[0], nil
	}
	now := timeNowUTC()
	v := &types.FleetVehicle{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		PlateNo:   plate,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.db.WithContext(ctx).Create(v).Error; err != nil {
		return nil, err
	}
	logger.Infof(ctx, "auto-created fleet vehicle %q (tenant=%d)", plate, tenantID)
	return v, nil
}

// mergeDuplicateVehiclesByPlate 把「同车牌不同大小写」的重复车辆合并到最早一条。
//
// 只改 fleet_records.vehicle_id 指向，并软删多余车辆；不删除任何记录。
func (h *FleetHandler) mergeDuplicateVehiclesByPlate(ctx context.Context) {
	var list []types.FleetVehicle
	if err := h.db.WithContext(ctx).Where("deleted_at IS NULL").Order("created_at ASC").Find(&list).Error; err != nil {
		logger.Warnf(ctx, "merge duplicate vehicles: list failed: %v", err)
		return
	}
	keep := map[string]types.FleetVehicle{}
	merged := 0
	for i := range list {
		v := list[i]
		key := fmt.Sprintf("%d|%s", v.TenantID, normalizePlate(v.PlateNo))
		if v.PlateNo == "" {
			continue
		}
		canonical, ok := keep[key]
		if !ok {
			keep[key] = v
			// 顺手把车牌写成规范形态
			if v.PlateNo != normalizePlate(v.PlateNo) {
				h.db.WithContext(ctx).Model(&types.FleetVehicle{}).Where("id = ?", v.ID).
					Update("plate_no", normalizePlate(v.PlateNo))
			}
			continue
		}
		// 重复项：把记录指向规范车辆，再软删重复车辆
		h.db.WithContext(ctx).Model(&types.FleetRecord{}).
			Where("vehicle_id = ? AND deleted_at IS NULL", v.ID).
			Update("vehicle_id", canonical.ID)
		now := timeNowUTC()
		h.db.WithContext(ctx).Model(&types.FleetVehicle{}).Where("id = ?", v.ID).
			Updates(map[string]any{"deleted_at": now, "updated_at": now})
		merged++
		logger.Infof(ctx, "merged duplicate vehicle %q into %q", v.PlateNo, canonical.PlateNo)
	}
	if merged > 0 {
		logger.Infof(ctx, "merged %d duplicate fleet vehicle(s)", merged)
	}
}

// backfillFleetVehicles 从发票库与存量记录里收集车牌，幂等补建车辆主数据。
//
// 数据源优先级：
//  1. `knowledges.custom_metadata.invoices[].items[].plate_no|车牌号`（发票明细，最权威）；
//  2. `knowledges.custom_metadata.invoices[].fields.车牌号`；
//  3. `fleet_records.data` 里的车牌（手工录入的存量）。
//
// 只读全表 400 行 JSON，启动跑一次，代价可忽略。返回新建车辆数。
func (h *FleetHandler) backfillFleetVehicles(ctx context.Context) int {
	plates := map[int64]map[string]bool{}
	add := func(tenantID int64, plate string) {
		plate = strings.TrimSpace(plate)
		if plate == "" {
			return
		}
		if plates[tenantID] == nil {
			plates[tenantID] = map[string]bool{}
		}
		plates[tenantID][plate] = true
	}

	// 1) 发票（含明细与票级 fields）
	var knowledges []types.Knowledge
	if err := h.db.WithContext(ctx).
		Select("id, tenant_id, custom_metadata").
		Where("deleted_at IS NULL AND custom_metadata IS NOT NULL").
		Find(&knowledges).Error; err == nil {
		for i := range knowledges {
			if len(knowledges[i].CustomMetadata) == 0 {
				continue
			}
			var meta struct {
				Invoices []struct {
					Fields map[string]any `json:"fields"`
					Items  []struct {
						PlateNo string `json:"plate_no"`
						PlateCN string `json:"车牌号"`
					} `json:"items"`
				} `json:"invoices"`
			}
			if jerr := json.Unmarshal(knowledges[i].CustomMetadata, &meta); jerr != nil {
				continue
			}
			tenantID := int64(knowledges[i].TenantID)
			for _, inv := range meta.Invoices {
				if s, ok := inv.Fields["车牌号"].(string); ok {
					add(tenantID, s)
				}
				for _, it := range inv.Items {
					if it.PlateNo != "" {
						add(tenantID, it.PlateNo)
					}
					if it.PlateCN != "" {
						add(tenantID, it.PlateCN)
					}
				}
			}
		}
	}

	// 2) 存量记录
	var records []types.FleetRecord
	if err := h.db.WithContext(ctx).
		Select("id, tenant_id, data").
		Where("deleted_at IS NULL").
		Find(&records).Error; err == nil {
		for i := range records {
			add(records[i].TenantID, pickPlateFromRecordData(records[i].Data))
		}
	}

	h.mergeDuplicateVehiclesByPlate(ctx)
	var before int64
	h.db.WithContext(ctx).Model(&types.FleetVehicle{}).Where("deleted_at IS NULL").Count(&before)
	for tenantID, set := range plates {
		for plate := range set {
			if _, err := h.ensureFleetVehicleByPlate(ctx, tenantID, plate); err != nil {
				logger.Warnf(ctx, "backfill fleet vehicle %q failed: %v", plate, err)
			}
		}
	}
	var after int64
	h.db.WithContext(ctx).Model(&types.FleetVehicle{}).Where("deleted_at IS NULL").Count(&after)
	created := int(after - before)
	if created > 0 {
		logger.Infof(ctx, "backfilled %d fleet vehicle(s) from invoices/records", created)
	}
	return created
}
