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

// extractionNotStarted 判断提取是否**尚未产生结论**。
//
// 主列表与概览 KPI 的准入条件是 `!extractionNotStarted(...)`：
// 解析/提取中的文件只在顶部进度条与「历史记录」抽屉出现，
// 不进主列表、不计入分类卡片张数。
func extractionNotStarted(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	if s == "" {
		return true
	}
	_, done := extractionCompletedStatuses[s]
	return !done
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
