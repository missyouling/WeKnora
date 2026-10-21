package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// 回归（2026-10-21）：6 页 6 票的 PDF 在列表里显示 P1、P2、P3、P4、P5、P5，
// 第 6 张票被误赋成 P5。
//
// 真机样本 knowledge 6845145f-227a-4231-8782-6194b43d4e9e（通行费电子票据汇总单，
// 每页 1 张票）的 chunks 结构：
//
//	chunk 13  locators=[5,5,5,5,5]      ← 第 5 页票面
//	chunk 14  locators=[5]              ← 第 5 页票尾（收款人/复核/开票人）
//	chunk 15  locators=[5,6,6,6,6,6,6]  ← 跨页：第 5 页票尾 + 第 6 页票头（含发票号码）
//	chunk 16  locators=[6,6,6]          ← 第 6 页正文
//
// 旧实现在每个 chunk 起始处只写一个 [Page N] 标记，且 N 取 source_locators 里
// **第一个非零页**（ChunkPage）。于是 chunk 15 整段被标成第 5 页，第 6 张票的
// 发票号码落在该段内 → strings.Index 命中的是 Page 5 → 落库 P5；而 [Page 6]
// 标记被推到 chunk 16，号码早已在它前面。第 6 页因此「有页码锚点却无票」。

// pageSeg 是一段带物理页码的原文片段。
type pageSeg struct {
	page int
	text string
}

// tollPageSegs 造「每页 1 张票」版式的一页：票头（票号在这里）+ 正文 + 票尾。
func tollPageSegs(no string, page int) []pageSeg {
	return []pageSeg{
		{page: page, text: strings.Join([]string{
			"## 重庆增值税电子普通发票",
			"机器编号：499098027960",
			"发票代码: 050002400112",
			"发票号码: " + no,
			"开票日期: 2025年02月08日",
			"校验码: 13475 50230 92029 97633",
			"",
		}, "\n")},
		{page: page, text: strings.Join([]string{
			"购买方: 重庆星达铜业有限公司",
			"销售方: 重庆高速集团有限公司",
			"通行费 渝AAU0092 货车 20250203 20250208 100.05 3% 3.00",
			"价税合计（小写）¥103.05",
			"",
		}, "\n")},
		{page: page, text: strings.Join([]string{
			"收款人: 吴伟",
			"复核：庹昌新",
			"开票人: 王琳琳",
			"",
		}, "\n")},
	}
}

// sixPageTollChunks 复刻真机 19 个 chunk 的「分块跨页」形态：
// 每页正文一个 chunk，页尾与下一页票头合并进同一个 chunk（多 locator、跨两个页码）。
// 页尾/票头 chunk 的 locator 带 quote（真机 mineru 回填的精确锚点文本）。
func sixPageTollChunks(pages [][3]string) []*types.Chunk {
	var chunks []*types.Chunk
	idx := 0
	for p := 0; p < len(pages); p++ {
		segs := tollPageSegs(pages[p][0], p+1)
		// 正文 chunk：票头 + 正文
		var body strings.Builder
		for _, s := range segs[:2] {
			body.WriteString(s.text)
		}
		chunks = append(chunks, &types.Chunk{
			ChunkIndex: idx, ChunkType: types.ChunkTypeText, Content: body.String(),
			SourceLocators: types.SourceLocators{{Type: "pdf", Page: p + 1}, {Type: "pdf", Page: p + 1}},
		})
		idx++
		// 边界 chunk：本页票尾 + 下一页票头（最后一页只有票尾）
		var b strings.Builder
		b.WriteString(segs[2].text)
		locs := types.SourceLocators{{Type: "pdf", Page: p + 1, Quote: "收款人: 吴伟"}}
		if p+1 < len(pages) {
			next := tollPageSegs(pages[p+1][0], p+2)[0]
			b.WriteString(next.text)
			// 下一页票头的每一行都是一个 locator（真机 6 个同页 locator）
			for _, line := range strings.Split(strings.TrimSpace(next.text), "\n") {
				locs = append(locs, types.SourceLocator{Type: "pdf", Page: p + 2, Quote: line})
			}
		}
		chunks = append(chunks, &types.Chunk{
			ChunkIndex: idx, ChunkType: types.ChunkTypeText, Content: b.String(),
			SourceLocators: locs,
		})
		idx++
	}
	return chunks
}

// 6 页 6 票：修复后必须严格 P1..P6，不得出现两个 P5。
func TestAssignPhysicalPagesSplitsCrossPageChunk(t *testing.T) {
	nos := []string{"35209009", "22856521", "29340739", "35923781", "28446584", "29836101"}
	pages := make([][3]string, 0, len(nos))
	for _, no := range nos {
		pages = append(pages, [3]string{no})
	}
	chunks := sixPageTollChunks(pages)
	src := BuildExtractSourceText(chunks)
	require.True(t, src.HasPages())

	// 锚点精度：第 6 张票的票号必须落在「第 6 页」区间内。
	idx := strings.Index(src.Content, nos[5])
	require.GreaterOrEqual(t, idx, 0)
	assert.Equal(t, 6, src.PageAt(idx), "第 6 张票的票号必须归属第 6 页（旧实现落在第 5 页）")

	invs := make([]InvoiceExtractionItem, 0, len(nos))
	for _, no := range nos {
		invs = append(invs, InvoiceExtractionItem{InvoiceNo: no})
	}
	AssignPhysicalPages(src, invs)
	got := make([]int, len(invs))
	for i := range invs {
		got[i] = invs[i].PageNo
	}
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6}, got, "6 页 6 票必须是 P1..P6，不得出现两个 P5")
}

// 真实同页两张票必须保留：第 13 页上下两张小票仍是 [.., 13, 13, ..]。
func TestAssignPhysicalPagesKeepsTwoTicketsOnSamePage(t *testing.T) {
	// 3 页 4 票：P1、P2、P3 上下两张。
	pages := [][3]string{
		{"11111111"},
		{"22222222"},
		{"33333333"},
	}
	chunks := sixPageTollChunks(pages)
	// 把第 3 页票尾 chunk 之后追加第二张票（同页），模拟「一页两票」。
	last := chunks[len(chunks)-1]
	last.Content += "\n## 重庆增值税电子普通发票\n发票代码: 050002400112\n发票号码: 44444444\n开票日期: 2025年02月08日\n"

	src := BuildExtractSourceText(chunks)
	require.True(t, src.HasPages())
	// 4 张票：P1、P2、P3、P3
	nos := []string{"11111111", "22222222", "33333333", "44444444"}
	invs := make([]InvoiceExtractionItem, 0, len(nos))
	for _, no := range nos {
		invs = append(invs, InvoiceExtractionItem{InvoiceNo: no})
	}
	AssignPhysicalPages(src, invs)
	got := make([]int, len(invs))
	for i := range invs {
		got[i] = invs[i].PageNo
	}
	assert.Equal(t, []int{1, 2, 3, 3}, got, "同一页上下两张票必须保持同页，不得被递增守卫误改")
}

// 尾部 chunk 没有 SourceLocators 时，若它自带新票页眉，必须按相邻页序递增，
// 而不是原地卡死在上一个 [Page N]。
func TestAssignPhysicalPagesInfersPageForTrailingChunkWithoutLocators(t *testing.T) {
	chunks := []*types.Chunk{
		{ChunkIndex: 0, ChunkType: types.ChunkTypeText,
			Content:        "## 电子发票（普通发票）\n发票号码: 10000001\n开票日期: 2026年01月01日\n",
			SourceLocators: types.SourceLocators{{Type: "pdf", Page: 1}}},
		{ChunkIndex: 1, ChunkType: types.ChunkTypeText,
			Content:        "## 电子发票（普通发票）\n发票号码: 10000002\n开票日期: 2026年01月02日\n",
			SourceLocators: nil},
	}
	src := BuildExtractSourceText(chunks)
	invs := []InvoiceExtractionItem{{InvoiceNo: "10000001"}, {InvoiceNo: "10000002"}}
	AssignPhysicalPages(src, invs)
	assert.Equal(t, 1, invs[0].PageNo)
	assert.Equal(t, 2, invs[1].PageNo, "无 locator 的尾部新票必须按相邻页序递增")
}
