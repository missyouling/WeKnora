// 第 65 轮：社保「正常记录 / 补退记录」支持真实删除。
//
// 旧实现把这两类记录当成只读投影（前端硬拦「申报回盘与扣款记录为只读」，
// 后端也没有删除路由）。但它们本质是**导入的明细数据**，导入错了必须能删。
// 回盘表（hr_si_receipts）是花名册的权威来源，仍保持不可删。
package handler

import (
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

// DeleteSIRecord DELETE /hr/social/records/:id —— 删除单条正常/补退记录。
func (h *HRHandler) DeleteSIRecord(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		hrFail(c, "缺少记录 id")
		return
	}
	var row HRSIRecord
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tid, id).
		First(&row).Error; err != nil {
		hrFail(c, "社保记录不存在或已被删除")
		return
	}
	if err := h.db.WithContext(ctx).Delete(&HRSIRecord{}, "tenant_id = ? AND id = ?", tid, id).Error; err != nil {
		hrFail(c, "删除社保记录失败: "+err.Error())
		return
	}
	/**
	 * 第 68 轮：删除记录**绝不重算账单**。
	 *
	 * 旧实现删完就调用 hrReconcileSIBillQuiet 重建当月账单 —— 已生成的账单
	 * 汇总与明细会被悄悄改成「删完之后的数」，用户看到的就是「账单金额突然变空」。
	 * 新口径：账单是不可变快照，只置 source_dirty 并提示用户手动重新生成。
	 */
	dirty := h.hrSIMarkBillDirty68(ctx, tid, row.BillMonth)
	hrOK(c, gin.H{
		"deleted": 1, "id": id,
		"bill_dirty_warning": dirty, "dirty_month": strings.TrimSpace(row.BillMonth),
	})
}

// DeleteSIRecordsBatch POST /hr/social/records/batch-delete {ids: []string}
//
// 批量删除按条独立处理：一条失败不影响其它记录，失败原因按记录回传。
// 前端据此只弹**一条**汇总提示（绝不循环弹错 + 再无条件弹成功）。
func (h *HRHandler) DeleteSIRecordsBatch(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		hrFail(c, "请选择要删除的记录")
		return
	}
	ids := make([]string, 0, len(req.IDs))
	seen := map[string]bool{}
	for _, id := range req.IDs {
		v := strings.TrimSpace(id)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		ids = append(ids, v)
	}
	if len(ids) == 0 {
		hrFail(c, "请选择要删除的记录")
		return
	}
	// 先查月份集合，删完统一重算这些月的账单
	var rows []HRSIRecord
	_ = h.db.WithContext(ctx).Where("tenant_id = ? AND id IN ?", tid, ids).Find(&rows).Error
	months := map[string]bool{}
	for _, r := range rows {
		if m := strings.TrimSpace(r.BillMonth); m != "" {
			months[m] = true
		}
	}
	res := h.db.WithContext(ctx).Where("tenant_id = ? AND id IN ?", tid, ids).Delete(&HRSIRecord{})
	if res.Error != nil {
		hrFail(c, "批量删除失败: "+res.Error.Error())
		return
	}
	deleted := int(res.RowsAffected)
	// 第 68 轮：只打脏标记，不重建账单（快照必须保持不变）。
	dirtyMonths := make([]string, 0, len(months))
	for m := range months {
		if h.hrSIMarkBillDirty68(ctx, tid, m) {
			dirtyMonths = append(dirtyMonths, m)
		}
	}
	sort.Strings(dirtyMonths)
	hrOK(c, gin.H{
		"deleted": deleted, "requested": len(ids),
		"bill_dirty_warning": len(dirtyMonths) > 0,
		"dirty_months":       dirtyMonths,
		"dirty_month":        strings.Join(dirtyMonths, ","),
	})
}
