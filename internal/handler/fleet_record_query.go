// 车队记录查询增强（第 28 轮 P0-4）：关键词、字段筛选、分页与统计。
//
// 原实现一次 `Find(&items)` 返回全量、无 total、无搜索 —— 派生 1500+ 条通行费
// 台账后，前端既没法分页也没法按车牌筛。这里对齐发票列表的查询契约：
//
//	GET /fleet/records?type=&month=&vehicle_id=&doc_type=&q=&field_filters=&page=&page_size=
//	→ { success, data: { items, total, page, page_size } }
//
// 关键词与字段筛选都下推到 SQL（`CAST(data AS TEXT) LIKE ?`），不在 Go 侧全表过滤：
// 台账会随发票持续增长，应用层过滤迟早拖垮首屏。
package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
)

// fleetFieldFilter 一个字段筛选条件（键=分类 subs 的中文字段名，值=包含匹配）。
type fleetFieldFilter struct {
	Key   string
	Value string
}

// parseFleetFieldFilters 解析 `key:value;key:value`。
//
// 用分号而不是逗号：车牌/费用类型等值可能带逗号，逗号会截断。
// 保留输入顺序，前端筛选条的展示顺序即用户配置顺序。
func parseFleetFieldFilters(raw string) []fleetFieldFilter {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := make([]fleetFieldFilter, 0, 4)
	for _, part := range strings.Split(raw, ";") {
		key, value, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out = append(out, fleetFieldFilter{Key: key, Value: value})
	}
	return out
}

// normalizeFleetPage 归一化分页参数（默认 1/20，上限 200）。
func normalizeFleetPage(page, size int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	return page, size
}

// fleetLikePattern 把关键词转成 LIKE 包含模式，并转义 % _ \ 三个通配符。
func fleetLikePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}

// fleetRecordListResult 列表响应（items + total，供前端分页与合计）。
type fleetRecordListResult struct {
	Items    []types.FleetRecord `json:"items"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}

// FleetRecordStats 当前菜单下各分类的记录数与金额合计（KPI 卡片用）。
type FleetRecordStats struct {
	ByType map[string]int64 `json:"by_type"`
	Total  int64            `json:"total"`
	Amount float64          `json:"amount"`
}

// FleetRecordStatsHandler 返回若干 record_type 的计数与金额合计。
//
//	GET /fleet/records/stats?types=tire,inspection&month=2026-11
//
// 一次拿全 7 个成本分类的计数，避免前端切菜单时发 7 个请求。
func (h *FleetHandler) FleetRecordStatsHandler(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	rawTypes := strings.TrimSpace(c.Query("types"))
	month := strings.TrimSpace(c.Query("month"))

	type aggRow struct {
		RecordType string
		Count      int64
		SumAmount  float64
	}
	q := h.db.WithContext(ctx).Model(&types.FleetRecord{}).
		Select("record_type, COUNT(*) AS count, COALESCE(SUM(amount), 0) AS sum_amount").
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID)
	if rawTypes != "" {
		typesList := make([]string, 0, 8)
		for _, t := range strings.Split(rawTypes, ",") {
			if t = strings.TrimSpace(t); t != "" {
				typesList = append(typesList, t)
			}
		}
		if len(typesList) > 0 {
			q = q.Where("record_type IN ?", typesList)
		}
	}
	if month != "" {
		q = q.Where("record_month = ?", month)
	}
	var rows []aggRow
	if err := q.Group("record_type").Scan(&rows).Error; err != nil {
		c.Error(errors.NewInternalServerError("record stats failed: " + err.Error()))
		return
	}
	out := FleetRecordStats{ByType: map[string]int64{}}
	for _, r := range rows {
		out.ByType[r.RecordType] = r.Count
		out.Total += r.Count
		out.Amount += r.SumAmount
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
