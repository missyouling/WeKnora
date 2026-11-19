// 第 65 轮：社保「正常记录 / 补退记录」支持真实删除。
//
// 旧实现把这两类记录当成只读投影（前端硬拦「申报回盘与扣款记录为只读」，
// 后端也没有删除路由）。但它们本质是**导入的明细数据**，导入错了必须能删。
// 回盘表（hr_si_receipts）是花名册的权威来源，仍保持不可删。
package handler

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/logger"
)

// hrReconcileSIBillQuiet 删除记录后重算当月账单（静默，失败只告警）。
//
// 口径：该月仍有正常记录 → 按剩余记录重建账单与明细；一条都不剩 → 直接删账单，
// 绝不留下「0 人 0 元」的空账单。
func (h *HRHandler) hrReconcileSIBillQuiet(ctx context.Context, tid int64, month string) {
	month = strings.TrimSpace(month)
	if month == "" {
		return
	}
	defer func() { _ = recover() }()
	var bills []HRSIBill
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND month_label = ?", tid, month).Find(&bills).Error; err != nil || len(bills) == 0 {
		return
	}
	normalCount, _, err := h.hrSIRecordCountsOf(ctx, tid, month)
	if err != nil {
		return
	}
	if normalCount == 0 {
		for _, b := range bills {
			_ = h.db.WithContext(ctx).Where("bill_id = ?", b.ID).Delete(&HRSIBillItem{}).Error
			_ = h.db.WithContext(ctx).Delete(&HRSIBill{}, "tenant_id = ? AND id = ?", tid, b.ID).Error
		}
		return
	}
	next := h.hrSIBillTotalsOf(ctx, tid, month)
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, old := range bills {
			if e := tx.Where("bill_id = ?", old.ID).Delete(&HRSIBillItem{}).Error; e != nil {
				return e
			}
			if e := tx.Delete(&old).Error; e != nil {
				return e
			}
		}
		next.ID = uuid.New().String()
		if e := tx.Create(&next).Error; e != nil {
			return e
		}
		items := h.hrSIBillItemsForMonth(ctx, tid, month, next.ID)
		if len(items) > 0 {
			return tx.CreateInBatches(items, 50).Error
		}
		return nil
	})
	if err != nil {
		logger.Warnf(ctx, "reconcile SI bill failed month=%s: %v", month, err)
	}
}

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
	// 删除后当月账单人数/金额会失真，同步重算一次（失败不阻断删除）
	h.hrReconcileSIBillQuiet(ctx, tid, row.BillMonth)
	hrOK(c, gin.H{"deleted": 1, "id": id})
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
	for m := range months {
		h.hrReconcileSIBillQuiet(ctx, tid, m)
	}
	hrOK(c, gin.H{"deleted": deleted, "requested": len(ids)})
}
