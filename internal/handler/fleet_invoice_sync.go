// 发票 → 成本台账的幂等派生（第 28 轮 Q1 决策）。
//
// 口径：
// 通行费发票（含明细名带「通行费」）→ 成本管理 / 路桥费（road-toll）；
// 成品油 / 柴油 / 汽油 / 充电发票 → 成本管理 / 加油充电（fuel-charge）。
//
// 为什么派生而不是重录：发票库里已提取 1530 张通行费票，明细自带车牌号、
// 通行日期起止、金额、税额 —— 让用户再手打一遍既费时又必然对不上账。
// 发票是原始凭据、台账是汇总视图，两者靠 (knowledge_id, invoice_no) 去重。
//
// 幂等：SourceRef = `<knowledge_id>#<invoice_no>`；已存在则只刷新金额/日期等
// 派生字段，不覆盖用户改过的备注与自定义字段。
package handler

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// tollHints / fuelHints 发票分类或明细名里的关键词。
var (
	tollHints = []string{"通行费", "过路费", "过桥费", "ETC"}
	fuelHints = []string{"加油", "油费", "成品油", "柴油", "汽油", "充电"}
)

// invoiceTextOf 把分类名 + 所有明细名拼成用于关键词判定的文本。
func invoiceTextOf(inv service.InvoiceExtractionItem) string {
	var b strings.Builder
	b.WriteString(inv.Category)
	b.WriteString(" ")
	b.WriteString(inv.InvoiceType)
	for _, it := range inv.Items {
		b.WriteString(" ")
		b.WriteString(it.Name)
	}
	return b.String()
}

func containsAny(text string, hints []string) bool {
	for _, h := range hints {
		if strings.Contains(text, h) {
			return true
		}
	}
	return false
}

// isTollInvoice 是否通行费发票。
func isTollInvoice(inv service.InvoiceExtractionItem) bool {
	return containsAny(invoiceTextOf(inv), tollHints)
}

// isFuelInvoice 是否加油/充电发票（通行费优先，两者互斥）。
func isFuelInvoice(inv service.InvoiceExtractionItem) bool {
	if isTollInvoice(inv) {
		return false
	}
	return containsAny(invoiceTextOf(inv), fuelHints)
}

// invoicePlateOf 取票据级或明细级的车牌号。
func invoicePlateOf(inv service.InvoiceExtractionItem) string {
	if inv.Fields != nil {
		if s, ok := inv.Fields["车牌号"].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	for _, it := range inv.Items {
		if it.Extra == nil {
			continue
		}
		for _, k := range plateKeys {
			if s, ok := it.Extra[k].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// round2Money 金额保留两位（与发票侧口径一致）。
func round2Money(v float64) float64 {
	if v >= 0 {
		return float64(int64(v*100+0.5)) / 100
	}
	return float64(int64(v*100-0.5)) / 100
}

// buildFleetRecordFromInvoice 把一张发票映射成一条成本台账记录。
//
// 返回 ok=false 的情形：无发票号（无法去重）或金额全空（派生出来是空壳）。
// 金额取**价税合计**：台账是"实际付了多少钱"，与发票列表的口径一致。
func buildFleetRecordFromInvoice(
	tenantID int64, knowledgeID, fileName string, inv service.InvoiceExtractionItem, page int,
) (types.FleetRecord, bool) {
	var rec types.FleetRecord
	invoiceNo := strings.TrimSpace(inv.InvoiceNo)
	if invoiceNo == "" {
		return rec, false
	}
	toll := isTollInvoice(inv)
	fuel := isFuelInvoice(inv)
	if !toll && !fuel {
		return rec, false
	}
	amount := 0.0
	if inv.TotalAmount != nil {
		amount = *inv.TotalAmount
	} else if inv.Amount != nil && inv.Tax != nil {
		amount = round2Money(*inv.Amount + *inv.Tax)
	} else if inv.Amount != nil {
		amount = *inv.Amount
	}
	if amount <= 0 {
		return rec, false
	}

	plate := invoicePlateOf(inv)
	data := map[string]any{
		"来源发票号": invoiceNo,
		"来源文件":  fileName,
		"车牌号":   plate,
	}
	if page > 0 {
		data["页码"] = page
	}

	if toll {
		rec.RecordType = types.FleetRecordRoadToll
		data["费用类型"] = "ETC通行费"
		if inv.Amount != nil {
			data["金额"] = *inv.Amount
		}
		if inv.Tax != nil {
			data["税额"] = *inv.Tax
		}
		if inv.TaxRate != nil {
			data["税率"] = *inv.TaxRate
		}
		data["含税金额"] = amount
		data["票据号码"] = invoiceNo
		if len(inv.Items) > 0 {
			it := inv.Items[0]
			if it.Extra != nil {
				if v, ok := it.Extra["travel_from"].(string); ok && v != "" {
					data["通行起始日期"] = v
				}
				if v, ok := it.Extra["travel_to"].(string); ok && v != "" {
					data["通行截止日期"] = v
				}
			}
		}
	} else {
		rec.RecordType = types.FleetRecordFuelCharge
		data["交易类型"] = "加油"
		data["消费类型"] = "加油"
		if len(inv.Items) > 0 {
			it := inv.Items[0]
			if name := strings.TrimSpace(it.Name); name != "" {
				data["油品/电价类型"] = name
			}
			if it.Qty != nil {
				data["数量"] = *it.Qty
			}
			if it.Price != nil {
				data["单价"] = *it.Price
			}
			if it.TaxRate != nil {
				data["税率"] = *it.TaxRate
			}
			if it.TaxAmt != nil {
				data["税额"] = *it.TaxAmt
			}
		}
		if strings.Contains(invoiceTextOf(inv), "充电") {
			data["交易类型"] = "充电"
			data["消费类型"] = "充电"
		}
	}

	recordDate := strings.TrimSpace(inv.InvoiceDate)
	month := ""
	if len(recordDate) >= 7 {
		month = recordDate[:7]
	}
	rec.TenantID = tenantID
	rec.RecordMonth = month
	rec.RecordDate = recordDate
	rec.Amount = amount
	rec.Data = data
	rec.FileName = fileName
	// 注意：**不能**写 DocKnowledgeID 列。fleet_records 有部分唯一索引
	// idx_fleet_records_kid_uq (tenant_id, record_type, doc_knowledge_id)，
	// 它假设「一个文件一种类型一条记录」（档案类口径）；而一份汇总单会派生出
	// 几十张票的台账，写列会直接撞唯一约束。源文件改存 data，供抽屉预览取用。
	rec.Data["源文件ID"] = knowledgeID
	rec.DocType = strings.TrimSpace(inv.Category)
	if rec.DocType == "" {
		if toll {
			rec.DocType = "通行费发票"
		} else {
			rec.DocType = "加油发票"
		}
	}
	rec.Remark = "由发票自动派生：" + invoiceNo
	rec.SourceKind = "invoice"
	rec.SourceRef = knowledgeID + "#" + invoiceNo
	return rec, true
}

// InvoiceSyncStats 一次派生同步的统计。
type InvoiceSyncStats struct {
	Scanned  int `json:"scanned"`  // 扫到的发票数
	Created  int `json:"created"`  // 新建台账
	Updated  int `json:"updated"`  // 已存在、刷新了来源字段
	Skipped  int `json:"skipped"`  // 无票号/无金额/非通行费与加油
	Vehicles int `json:"vehicles"` // 顺带补建的车辆数
}

// SyncInvoicesToFleetCost 扫描发票库并把通行费/加油发票派生到成本台账（幂等）。
//
// 性能：1500+ 张票逐条 SELECT+INSERT 会跑十几分钟（远程池每次约 250ms）。
// 这里先把已派生的 SourceRef 一次读进内存，再批量 CreateInBatches；
// 车辆也一次性建索引，避免每条记录都查一次 fleet_vehicles。
func (h *FleetHandler) SyncInvoicesToFleetCost(ctx context.Context, tenantID int64) (InvoiceSyncStats, error) {
	var stats InvoiceSyncStats
	var rows []types.Knowledge
	q := h.db.WithContext(ctx).
		Select("id, tenant_id, title, file_name, custom_metadata").
		Where("deleted_at IS NULL AND custom_metadata IS NOT NULL")
	if tenantID > 0 {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return stats, err
	}

	// 已派生记录：SourceRef -> 记录（用于判重与增量刷新）
	var existingRows []types.FleetRecord
	h.db.WithContext(ctx).
		Where("deleted_at IS NULL AND source_kind = ?", "invoice").
		Find(&existingRows)
	existing := make(map[string]types.FleetRecord, len(existingRows))
	for _, r := range existingRows {
		existing[r.SourceRef] = r
	}

	// 车辆索引：plate_no -> vehicle_id（一次加载，缺失时按需补建）
	vehicles := h.fleetVehicleIndex(ctx)
	initialVehicles := len(vehicles)

	var toCreate []types.FleetRecord
	var toUpdate []types.FleetRecord
	for i := range rows {
		row := rows[i]
		var meta service.InvoiceCustomMetadata
		if len(row.CustomMetadata) == 0 || json.Unmarshal(row.CustomMetadata, &meta) != nil {
			continue
		}
		if meta.Kind != "invoice" || len(meta.Invoices) == 0 {
			continue
		}
		fileName := strings.TrimSpace(row.FileName)
		if fileName == "" {
			fileName = row.Title
		}
		for idx, inv := range meta.Invoices {
			page := inv.PageNo
			if page <= 0 {
				page = inv.Page
			}
			if page <= 0 {
				page = idx + 1
			}
			rec, ok := buildFleetRecordFromInvoice(int64(row.TenantID), row.ID, fileName, inv, page)
			if !ok {
				stats.Skipped++
				continue
			}
			stats.Scanned++
			if plate := pickPlateFromRecordData(rec.Data); plate != "" {
				rec.VehicleID = h.vehicleIDForPlate(ctx, vehicles, rec.TenantID, plate)
			}
			if old, dup := existing[rec.SourceRef]; dup {
				merged := map[string]any{}
				for k, v := range old.Data {
					merged[k] = v
				}
				for k, v := range rec.Data {
					merged[k] = v
				}
				if old.Amount == rec.Amount && old.RecordMonth == rec.RecordMonth &&
					old.RecordDate == rec.RecordDate &&
					sameJSONMap(old.Data, merged) {
					continue
				}
				rec.ID = old.ID
				rec.Data = merged
				toUpdate = append(toUpdate, rec)
				continue
			}
			rec.ID = uuid.NewString()
			rec.CreatedAt = timeNowUTC()
			rec.UpdatedAt = rec.CreatedAt
			toCreate = append(toCreate, rec)
		}
	}

	if len(toCreate) > 0 {
		for i := 0; i < len(toCreate); i += 200 {
			end := i + 200
			if end > len(toCreate) {
				end = len(toCreate)
			}
			if err := h.db.WithContext(ctx).CreateInBatches(toCreate[i:end], 200).Error; err != nil {
				logger.Warnf(ctx, "batch create derived fleet records failed: %v", err)
			} else {
				stats.Created += end - i
			}
		}
	}
	for i := range toUpdate {
		rec := toUpdate[i]
		updates := map[string]any{
			"amount":       rec.Amount,
			"record_month": rec.RecordMonth,
			"record_date":  rec.RecordDate,
			"data":         rec.Data,
			"file_name":    rec.FileName,
			"vehicle_id":   rec.VehicleID,
			"updated_at":   timeNowUTC(),
		}
		if err := h.db.WithContext(ctx).Model(&types.FleetRecord{}).
			Where("id = ?", rec.ID).Updates(updates).Error; err != nil {
			logger.Warnf(ctx, "update derived fleet record %s failed: %v", rec.SourceRef, err)
			continue
		}
		stats.Updated++
	}
	stats.Vehicles = len(vehicles) - initialVehicles
	if stats.Created > 0 || stats.Updated > 0 {
		logger.Infof(ctx, "fleet invoice sync: scanned=%d created=%d updated=%d skipped=%d",
			stats.Scanned, stats.Created, stats.Updated, stats.Skipped)
	}
	return stats, nil
}

// fleetVehicleIndex 加载 tenant 车辆索引并返回「plate -> vehicle_id」。
func (h *FleetHandler) fleetVehicleIndex(ctx context.Context) map[string]string {
	out := map[string]string{}
	var list []types.FleetVehicle
	if err := h.db.WithContext(ctx).Where("deleted_at IS NULL").Find(&list).Error; err != nil {
		logger.Warnf(ctx, "load fleet vehicles for invoice sync failed: %v", err)
		return out
	}
	for i := range list {
		out[list[i].PlateNo] = list[i].ID
	}
	return out
}

// vehicleIDForPlate 从索引取车辆，缺失则补建并写回索引（每个新牌最多一次 INSERT）。
func (h *FleetHandler) vehicleIDForPlate(ctx context.Context, idx map[string]string, tenantID int64, plate string) string {
	if id, ok := idx[plate]; ok {
		return id
	}
	v, err := h.ensureFleetVehicleByPlate(ctx, tenantID, plate)
	if err != nil || v == nil {
		return ""
	}
	idx[plate] = v.ID
	return v.ID
}

// SyncInvoices 手动触发发票 → 成本台账派生（POST /fleet/records/sync-invoices）。
// 启动时已自动跑一次；用户在发票侧补录/改分类后可点一次立即对齐。
//
// 注意：派生是**全库扫描**（1500+ 张票，几十秒量级），不能直接绑请求 context ——
// 前端切菜单/关页面就会把 HTTP 连接断掉，`context canceled` 让半途的批量写白跑。
// 这里用不随请求取消的独立 context，并把「已取消」降级成警告而不是错误日志。
func (h *FleetHandler) SyncInvoices(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Minute)
	defer cancel()
	tenantID, _ := fleetTenantID(c)
	stats, err := h.SyncInvoicesToFleetCost(ctx, int64(tenantID))
	if err != nil {
		if stdErrors.Is(err, context.Canceled) || stdErrors.Is(err, context.DeadlineExceeded) {
			logger.Warnf(ctx, "fleet invoice sync stopped early: %v", err)
			c.Error(errors.NewInternalServerError("sync invoices interrupted"))
			return
		}
		logger.Errorf(ctx, "manual fleet invoice sync failed: %v", err)
		c.Error(errors.NewInternalServerError("sync invoices failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": stats})
}

// upsertDerivedRecord 按 SourceRef 幂等写入：存在则合并派生字段（保留用户备注）。
func (h *FleetHandler) upsertDerivedRecord(ctx context.Context, rec types.FleetRecord) (created, changed bool, err error) {
	var existing []types.FleetRecord
	if err = h.db.WithContext(ctx).
		Where("tenant_id = ? AND source_ref = ? AND deleted_at IS NULL", rec.TenantID, rec.SourceRef).
		Limit(1).Find(&existing).Error; err != nil {
		return false, false, err
	}
	now := timeNowUTC()
	if len(existing) == 0 {
		rec.ID = uuid.NewString()
		rec.SourceKind = "invoice"
		rec.CreatedAt = now
		rec.UpdatedAt = now
		h.resolveFleetVehicle(ctx, rec.TenantID, &rec)
		if err = h.db.WithContext(ctx).Create(&rec).Error; err != nil {
			return false, false, err
		}
		return true, false, nil
	}
	old := existing[0]
	merged := map[string]any{}
	for k, v := range old.Data {
		merged[k] = v
	}
	for k, v := range rec.Data {
		merged[k] = v
	}
	// 来源字段变了才写库（幂等：重复同步不产生更新）
	changed = old.Amount != rec.Amount || old.RecordMonth != rec.RecordMonth ||
		old.RecordDate != rec.RecordDate || !sameJSONMap(old.Data, merged)
	if !changed {
		return false, false, nil
	}
	h.resolveFleetVehicle(ctx, rec.TenantID, &rec)
	updates := map[string]any{
		"amount":       rec.Amount,
		"record_month": rec.RecordMonth,
		"record_date":  rec.RecordDate,
		"data":         merged,
		"file_name":    rec.FileName,
		"vehicle_id":   rec.VehicleID,
		"updated_at":   now,
	}
	if err = h.db.WithContext(ctx).Model(&types.FleetRecord{}).
		Where("id = ?", old.ID).Updates(updates).Error; err != nil {
		return false, false, err
	}
	return false, true, nil
}

// sameJSONMap 判断两个 map 的键值是否完全一致（值用 JSON 序列化比较）。
func sameJSONMap(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok {
			return false
		}
		ja, ea := json.Marshal(va)
		jb, eb := json.Marshal(vb)
		if ea != nil || eb != nil || string(ja) != string(jb) {
			return false
		}
	}
	return true
}
