package handler

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// 上游数据源变更 → 费用清单自动重算（第 43 轮）。
//
// 产品要求：市电账单 / 光伏账单 / 电费核算 / 水费核算 / 气费核算 任一处
// 新增、修改、删除后，费用清单必须自动同步，用户不必再手点「重新核算」。
//
// 实现口径：**费用清单的行是从上游数据现算出来的**（前端 TenantBilling 的
// buildRows 每次 loadAllData 都用最新的账单/核算/水气读数重建），所以「重算」
// 不需要在后端逐月写库，只需要让前端知道「该重载了」。但后端仍要提供一个
// 幂等入口给「按月份精准重算」用（RegenerateBillingByPeriod），
// 供手动按钮与未来的定时任务复用。

// RegenerateBillingByPeriod 按账单周期重算该月**所有使用单位**的费用清单。
//
// 只重算 `billing_records` 里已存在的行（它承载持睿汽车等租户的分摊结果）；
// 重庆星达（业主）视角的行是前端从市电账单现算的，无落库行可写。
//
// 幂等：内部走 GenerateBillingRecord 的同一套 prepareBillingInputs →
// allocateTimed → buildFeeItems → persistRecord，重跑覆盖同一 (tenant, month)。
func (h *BillingHandler) RegenerateBillingByPeriod(ctx context.Context, month string) (int, error) {
	if h == nil || h.db == nil || month == "" {
		return 0, nil
	}
	var tenantIDs []string
	if err := h.db.WithContext(ctx).
		Table("billing_records").
		Where("month = ? AND deleted_at IS NULL", month).
		Distinct("billing_tenant_id").
		Pluck("billing_tenant_id", &tenantIDs).Error; err != nil {
		logger.Warnf(ctx, "RegenerateBillingByPeriod: list tenants failed: %v", err)
		return 0, err
	}
	done := 0
	for _, tid := range tenantIDs {
		if _, _, err := h.regenerateRecord(ctx, tid, month); err != nil {
			logger.Warnf(ctx, "RegenerateBillingByPeriod: regenerate %s/%s failed: %v", month, tid, err)
			continue
		}
		done++
	}
	return done, nil
}

// notifyBillingUpstreamChanged 上游写操作完成后的统一联动钩子。
//
// 目前是「记录一条可观测日志」的最小实现：真正的重算由前端监听
// ENERGY_DATA_CHANGED_EVENT 后 loadAllData 完成（避免在每次抄表保存时
// 同步跑一次全量账单生成，把写接口拖慢几百毫秒）。
// 需要强制落库重算时（手动「重新核算」/运维脚本），调用
// RegenerateBillingByPeriod 即可。
func notifyBillingUpstreamChanged(ctx context.Context, source, month string) {
	if month == "" {
		return
	}
	logger.Infof(ctx, "billing upstream changed: source=%s month=%s (client will reload)", source, month)
}

// regenerateRecord 账单生成内核（第 43 轮抽出）。
//
// 原本这段逻辑内联在 GenerateBillingRecord 里，只能从 HTTP 入口触发；
// 抽出后 RegenerateBillingByPeriod / 上游变更联动都能复用同一套口径，
// 避免出现「按钮走一套、自动重算走另一套」的双口径。
//
// 幂等：persistRecord 按 (billing_tenant_id, month) 覆盖写，
// 重跑同一月份不会产生重复行，也不会改变已锁定的分摊金额（输入未变即输出未变）。
func (h *BillingHandler) regenerateRecord(
	ctx context.Context, tenantID, month string,
) (*types.BillingRecord, []types.BillingRecordItem, error) {
	in, err := h.prepareBillingInputs(ctx, tenantID, month)
	if err != nil {
		return nil, nil, err
	}

	now := timeNowUTC()
	recordID := uuid.NewString()
	plan := allocateTimed(in, recordID, now)
	fees := buildFeeItems(in, plan, recordID, now)

	industrialFee := round2(fees.IndustrialFee)
	totalFee := round2(industrialFee + in.DormFee + in.WaterFee)
	rec := types.BillingRecord{
		ID: recordID, BillingTenantID: tenantID, Month: month, Status: "generated",
		TotalKwh: round2(in.StarTotal), LineLoss: in.LineLoss, Ratio: in.Ratio,
		BillTotalAmount: in.Bill.TotalAmount,
		DormKwh:         round2(in.DormKwh), DormFee: in.DormFee,
		WaterUsage: round2(in.WaterUsage), WaterFee: in.WaterFee,
		IndustrialFee: industrialFee, TotalFee: totalFee,
		// 两个合计列（第 22 轮 P1-9）：口径写死在落库处，页面与报表不必再各自相加。
		// 电费合计 = 工业电费 + 宿舍电费；水费合计 = 各水表之和（当前无工业/宿舍分档）。
		ElecTotalFee:  round2(industrialFee + in.DormFee),
		WaterTotalFee: in.WaterFee,
		CreatedAt:     now, UpdatedAt: now,
	}
	if err := h.persistRecord(ctx, rec, fees.Items, plan.MeterRows, in.WaterRows); err != nil {
		return nil, nil, errors.NewInternalServerError("generate billing record failed")
	}
	return &rec, fees.Items, nil
}

// RegenerateBillingByPeriodHandler 按账单周期重算**全部使用单位**（第 43 轮）。
//
// 业主视角（重庆星达）没有自己的 billing_records 行，它的费用清单是前端从
// 市电账单/核算数据现算的；但「重新核算」仍必须真的从上游重取一次，
// 因此提供这个跨租户入口：按月份扫出所有租户并逐月重算。
//
// 请求：{ months: ["2026-07", ...] }（可空 → 重算库内出现过的全部月份）
func (h *BillingHandler) RegenerateBillingByPeriodHandler(c *gin.Context) {
	ctx := c.Request.Context()
	var req struct {
		Months []string `json:"months"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	months := make([]string, 0, len(req.Months))
	for _, m := range req.Months {
		m = strings.TrimSpace(m)
		if len(m) == 7 {
			months = append(months, m)
		}
	}
	if len(months) == 0 {
		if err := h.db.WithContext(ctx).
			Table("billing_records").
			Where("deleted_at IS NULL").
			Distinct("month").
			Order("month DESC").
			Pluck("month", &months).Error; err != nil {
			logger.Errorf(ctx, "regenerate by period: list months failed: %v", err)
			c.Error(errors.NewInternalServerError("list months failed"))
			return
		}
	}
	okMonths := make([]string, 0, len(months))
	failed := make([]map[string]string, 0)
	for _, m := range months {
		n, err := h.RegenerateBillingByPeriod(ctx, m)
		if err != nil {
			failed = append(failed, map[string]string{"month": m, "error": err.Error()})
			continue
		}
		_ = n
		okMonths = append(okMonths, m)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"regenerated": okMonths,
			"failed":      failed,
			"count":       len(okMonths),
		},
	})
}

// RegenerateBillingBatch 按月份批量重算费用清单（第 43 轮「重新核算」后端入口）。
//
// 请求：{ months: ["2026-07", ...] }（可空 → 重算该租户全部月份）
// 语义：**从上游源头重取** —— 每个月都走 prepareBillingInputs 重新读市电账单
// （含目录电费计费数量、各分项电费）、光伏账单、电/水/气核算与抄表读数，
// 再重算并覆写该月记录，绝不复用 billing_records 旧缓存字段自循环。
//
// 幂等：按月覆盖写；输入未变时输出逐分不变（已锁定的分摊金额不会漂移）。
func (h *BillingHandler) RegenerateBillingBatch(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	var req struct {
		Months []string `json:"months"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}

	months := make([]string, 0, len(req.Months))
	for _, m := range req.Months {
		m = strings.TrimSpace(m)
		if len(m) != 7 {
			continue
		}
		months = append(months, m)
	}
	// 未指定月份 → 取该租户已有记录的全部月份（全量重算）
	if len(months) == 0 {
		if err := h.db.WithContext(ctx).
			Table("billing_records").
			Where("billing_tenant_id = ? AND deleted_at IS NULL", id).
			Distinct("month").
			Order("month DESC").
			Pluck("month", &months).Error; err != nil {
			logger.Errorf(ctx, "regenerate batch: list months failed: %v", err)
			c.Error(errors.NewInternalServerError("list months failed"))
			return
		}
	}

	okMonths := make([]string, 0, len(months))
	failed := make([]map[string]string, 0)
	for _, m := range months {
		if _, _, err := h.regenerateRecord(ctx, id, m); err != nil {
			logger.Warnf(ctx, "regenerate billing %s/%s failed: %v", id, m, err)
			failed = append(failed, map[string]string{"month": m, "error": err.Error()})
			continue
		}
		okMonths = append(okMonths, m)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"regenerated": okMonths,
			"failed":      failed,
			"count":       len(okMonths),
		},
	})
}
