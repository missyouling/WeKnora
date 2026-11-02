package service

import "strings"

// 提取完成态白名单（第 23 轮 issue 1）。
//
// 只有这三种状态代表「提取已经有结论」，其余（空串 / pending / processing /
// queued / running / 任何未来新增的中间态）一律视为**尚未开始或进行中**：
//
//   - success：提取成功（可能 0 张，但已经跑完，属于结论）；
//   - manual ：人工入库/待补录（用户可见、可编辑，属于结论）；
//   - failed ：提取失败（有明确失败结论，归「历史记录」抽屉）。
//
// 为什么用白名单而不是黑名单：后端多处在异步链路里写状态，新增中间态时
// 黑名单会漏（真机事故：`pending` 被当成完成态，解析中的文件直接在主列表
// 渲染成「发票号码: 提取中 / 提取状态: 待提取」的假记录）。
var extractionCompletedStatuses = map[string]struct{}{
	"success": {},
	"manual":  {},
	"failed":  {},
}

// extractionInFlightStatuses 是**明确的进行中状态**白名单。
//
// 与 extractionCompletedStatuses 互补，但用途不同：
//   - 前者判「能不能展示」；
//   - 本表判「是不是正在跑」—— 只有明确列出的中间态才算在途，
//     空串/未知值不算（历史数据状态字段可能整片缺失）。
//
// 这是第 24 轮把「状态缺失」与「状态在途」区分开的唯一依据：
// 两者都会被 extractionNotStarted 判为「未完成」，但拦截策略必须相反。
var extractionInFlightStatuses = map[string]struct{}{
	"pending":    {},
	"processing": {},
	"queued":     {},
	"running":    {},
	"finalizing": {},
	"deleting":   {},
	"waiting":    {},
	"extracting": {},
}

// extractionNotStarted 判断提取是否**尚未产生结论**。
//
// 主列表与概览 KPI 的准入条件是 `!extractionNotStarted(...)`。
func extractionNotStarted(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	if s == "" {
		return true
	}
	_, done := extractionCompletedStatuses[s]
	return !done
}

// extractionInFlight 判断提取是否**明确处于进行中**（而非仅仅「状态缺失」）。
//
// 用途：解析/提取中的文档，其**所有分页子记录**都必须挡在主列表外，
// 哪怕库里已经有半成品子记录（前端提前用 chunk 正则生成的、或上一轮中断
// 留下的残片）。仅靠「有没有子记录」放行会把这类半成品漏进列表 ——
// 真机事故：状态栏还显示「正在解析 2026.9-10油费发票.pdf」，
// 列表第 17~20 行已经出现了该文件的 P1~P4（第 20 行还是「待提取 + 0.00」），
// KPI 卡片也提前变了。
func extractionInFlight(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	if s == "" {
		return false
	}
	_, flying := extractionInFlightStatuses[s]
	return flying
}

// extractionParseIncomplete 判断解析流水线是否尚未完成。
//
// 只有 `completed`（及兼容的 success）才算完成；空串视为「没有解析状态字段」
// （历史数据）→ **不算未完成**（否则所有老数据都会被打入冷宫）。
// failed/cancelled 也不算「未完成」：解析失败的文件文本 chunk 可能已完整落库，
// 业务重提只依赖 chunk 文本，不该被解析失败状态永久挡死。
func extractionParseIncomplete(parseStatus string) bool {
	s := strings.ToLower(strings.TrimSpace(parseStatus))
	if s == "" {
		return false
	}
	switch s {
	case "completed", "success", "failed", "cancelled", "canceled":
		return false
	}
	// 其余（pending / processing / finalizing / deleting / parsing…）都算未完成
	return true
}

// extractionShouldHide 是**主列表 / KPI 的统一准入判据**（第 24 轮重写）。
//
// 拦（返回 true，即「不展示」）的三种情况：
//
//  1. 解析未完成（parse_status 明确处于流水线中）：文档还在解析，
//     此时任何子记录都是半成品，一律不展示；
//  2. 提取明确在途（extract_status ∈ extractionInFlightStatuses）：
//     哪怕已有子记录也不展示 —— 那是上一轮中断的残片或提前生成的半成品；
//  3. 提取无结论且一条子记录都没有：全新上传、还没开始提取。
//
// **放行**的关键一类：extract_status 为空/未知 **且已有子记录** ——
// 这是历史数据（状态字段是后来才加的），不能因为字段缺失就整体隐身。
// 这与第 23 轮的口径一致，也是本条最容易改错的地方。
func extractionShouldHide(parseStatus, extractStatus string, itemCounts ...int) bool {
	// ① 解析没完成 → 一律挡（不看子记录）
	if extractionParseIncomplete(parseStatus) {
		return true
	}
	// ② 提取明确在途 → 一律挡（不看子记录）
	if extractionInFlight(extractStatus) {
		return true
	}
	// ③ 提取无结论 + 一条都没有 → 挡（全新文件，还没开始提取）
	if extractionNotStarted(extractStatus) && hasNoExtractedItems(itemCounts...) {
		return true
	}
	return false
}

// hasNoExtractedItems 报告各类业务元数据是否「一条子记录都没有」。
//
// 与 extractionNotStarted 组合使用：**只有「还没跑完 + 一条都没提出来」才拦**。
// 已经提出若干条的存量数据（哪怕状态字段是空的）必须照常展示 ——
// 数据不能因为状态字段缺失就整体隐身。
func hasNoExtractedItems(counts ...int) bool {
	for _, n := range counts {
		if n > 0 {
			return false
		}
	}
	return true
}
