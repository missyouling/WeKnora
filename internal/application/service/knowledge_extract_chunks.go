package service

import (
	"regexp"
	"strings"

	"github.com/Tencent/WeKnora/internal/infrastructure/docparser"
	"github.com/Tencent/WeKnora/internal/types"
)

// 提取统一取数层。
//
// 背景（2026-10-19 审查发现）：仓储层 `ListChunksByKnowledgeID` 的 SQL 硬编码
// `chunk_type = 'text'`，而 5 个业务提取入口（合同/发票/发票分页/制度/通知）全部
// 调用它。于是 `Build*ExtractionContent` 里那些
// `ChunkType != Text && != ImageOCR && != ImageCaption → continue` 的判断
// **全部是死代码**：纯扫描件（PaddleOCR-VL / Simple+VLM 把正文写进 image_ocr）
// 会拿到 0 个 chunk，直接落到 "no_text" 分支。
//
// 本文件把「该取哪些 chunk」「怎么拼文本」「页码从哪来」收敛成唯一实现，
// 新增业务不要再各自写一遍过滤循环。

// ExtractSourceChunkTypes 是业务提取统一读取的 chunk 类型集合。
// 只认正文与图片理解产物；summary / parent_text / 向量类 chunk 一律不进提取
// （摘要会与正文重复且更易引入模型幻觉）。
func ExtractSourceChunkTypes() []types.ChunkType {
	return []types.ChunkType{
		types.ChunkTypeText,
		types.ChunkTypeImageOCR,
		types.ChunkTypeImageCaption,
	}
}

// extractTypeAllowed 判断某 chunk 类型是否属于提取正文来源。
func extractTypeAllowed(t types.ChunkType) bool {
	return t == types.ChunkTypeText ||
		t == types.ChunkTypeImageOCR ||
		t == types.ChunkTypeImageCaption
}

// ChunksForExtraction 过滤出可参与提取的 chunk：
//  1. 丢弃 nil、非正文类型、内容为空；
//  2. 对**相邻且完全重复**的文本去重 —— 同一张图常被 OCR 与 caption 各产一条
//     内容相同的 chunk，重复喂给模型会放大同一段文字、挤占上下文；
//  3. 保持原始顺序（chunk_index ASC）。
func ChunksForExtraction(chunks []*types.Chunk) []*types.Chunk {
	out := make([]*types.Chunk, 0, len(chunks))
	var prev string
	for _, c := range chunks {
		if c == nil || !extractTypeAllowed(c.ChunkType) {
			continue
		}
		text := strings.TrimSpace(c.Content)
		if text == "" {
			continue
		}
		if text == prev {
			continue
		}
		prev = text
		out = append(out, c)
	}
	return out
}

// ChunkPage 返回 chunk 的 1-based 物理页码；无 SourceLocators 时返回 0
// （调用方据此回退为自增序号）。多个 locator 时取第一个非零页。
func ChunkPage(c *types.Chunk) int {
	if c == nil {
		return 0
	}
	for _, loc := range c.SourceLocators {
		if loc.Page > 0 {
			return loc.Page
		}
	}
	return 0
}

// PageMarker 记录「在拼装文本的第 Offset 个字节处，页码变为 Page」。
// 用字节偏移而非行号，是因为下游用 strings.Index 定位发票号也是字节偏移。
type PageMarker struct {
	Offset int
	Page   int
}

// ExtractSourceText 是「拼装后可直接送规则/模型」的文本及其页码索引。
type ExtractSourceText struct {
	Content string
	Markers []PageMarker
}

// PageAt 返回给定字节偏移所属的物理页码；偏移早于首个标记或全文无标记时返回 0。
func (s ExtractSourceText) PageAt(offset int) int {
	page := 0
	for _, m := range s.Markers {
		if m.Offset > offset {
			break
		}
		page = m.Page
	}
	return page
}

// HasPages 报告本文档是否有任何物理页码信息。
func (s ExtractSourceText) HasPages() bool { return len(s.Markers) > 0 }

// BuildExtractSourceText 把 chunk 拼成提取文本，并记录每段的起始偏移与物理页码。
//
// 每个 chunk 的内容先过一次幂等的 `NormalizeHTMLTables`：解析侧虽然
// 已在入库前归一过，但历史 chunk、样票临时解析与部分引擎可能漏掉这一步，
// 裸 HTML 表格会让模型读成噪声。归一化对已是 Markdown 的文本是 no-op。
func BuildExtractSourceText(chunks []*types.Chunk) ExtractSourceText {
	usable := ChunksForExtraction(chunks)
	var sb strings.Builder
	markers := make([]PageMarker, 0, len(usable))
	lastPage := 0
	for _, c := range usable {
		text := strings.TrimSpace(docparser.NormalizeHTMLTables(c.Content))
		if text == "" {
			continue
		}
		// MinerU 会把明细表头多个标签挤进同一个 colspan 单元格，而数据行是
		// 独立单元格——不拆分的话表头与数据格对不齐，明细字段（车牌/金额/
		// 税率/税额）会被静默漏提。在拼装层统一拆好，规则与 LLM 都受益。
		text = splitMultiLabelHeaderCells(text)
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		base := sb.Len()
		var chunkMarkers []PageMarker
		switch {
		case ChunkPage(c) > 0:
			// 一个 chunk 可能跨页（真机 6 页票：chunk 15 = 第 5 页票尾 + 第 6 页票头），
			// 只在 chunk 起点打一个标记会把后半段整段算到前一页。
			chunkMarkers = chunkPageMarkers(base, text, c.SourceLocators)
		case lastPage > 0:
			// 末尾 chunk 缺 SourceLocators：它若自带新票页眉，就是一页新票，
			// 按相邻页序递增，绝不让它卡死在上一页。
			if loc := invoicePageHeaderRe.FindStringIndex(text); loc != nil {
				chunkMarkers = []PageMarker{{Offset: base + loc[0], Page: lastPage + 1}}
			}
		}
		markers = append(markers, chunkMarkers...)
		if n := len(chunkMarkers); n > 0 {
			lastPage = chunkMarkers[n-1].Page
		}
		sb.WriteString(text)
		sb.WriteString("\n")
	}
	// 解析引擎没回填 SourceLocators（纯文本引擎 / 旧 anydoc）时，按票面页眉
	// 「电子发票」出现次数推算物理页码——绝不能让数组序号冒充物理页码
	// （真机 6 页 PDF 曾被标成 P1~P4，用户无法据此去源文件核对）。
	if len(markers) == 0 {
		markers = inferPageMarkersFromInvoiceHeaders(sb.String())
	}
	return ExtractSourceText{Content: sb.String(), Markers: markers}
}

// invoicePageHeaderRe 匹配一页票据的页眉（如「## 电子发票（普通发票） 国家税务总局」、
// 「## 重庆增值税电子普通发票」）。
// 只认行首/带 # 前缀的形式，避免命中正文里偶然提到的「电子发票」。
var invoicePageHeaderRe = regexp.MustCompile(`(?m)^[ \t]*(?:#{1,6}[ \t]*)?[^ \t\n]{0,6}(?:电子发票[（(]|增值税电子(?:普通|专用)发票|电子(?:普通|专用)发票)`)

// summaryPageHintRe 匹配「汇总单」字样，用于识别首页是汇总表而非票据页。
var summaryPageHintRe = regexp.MustCompile(`汇总单|汇总清单|票据汇总`)

// inferPageMarkersFromInvoiceHeaders 用票面页眉推算物理页码。
//
// 规则：
//  1. 每个「电子发票（…）」页眉即一页票据，按出现顺序编号 1..N；
//  2. 若首个页眉之前的内容是汇总表（含「汇总单」等字样），说明首页是汇总页，
//     票据页整体后移一页（P2..PN），与用户在 PDF 预览器里看到的页码一致。
//
// 文本里一个页眉都没有时返回 nil（调用方保持「无页码信息」语义）。
func inferPageMarkersFromInvoiceHeaders(content string) []PageMarker {
	if content == "" {
		return nil
	}
	locs := invoicePageHeaderRe.FindAllStringIndex(content, -1)
	if len(locs) == 0 || len(locs) > 500 {
		return nil
	}
	base := 1
	if summaryPageHintRe.MatchString(content[:locs[0][0]]) {
		base = 2
	}
	markers := make([]PageMarker, 0, len(locs))
	for i, loc := range locs {
		markers = append(markers, PageMarker{Offset: loc[0], Page: base + i})
	}
	return markers
}

// chunkPageMarkers 计算一个 chunk 内部的页码锚点（chunk 起点 + 跨页边界）。
//
// 关键点：`source_locators` 是按阅读顺序排列的**块级**定位器，一个 chunk 里可能
// 同时含第 5 页的票尾和第 6 页的票头（真机 6 页票 chunk 15 的 locators 是
// [5,6,6,6,6,6,6]）。旧实现只取第一个非零页（ChunkPage）在 chunk 起点打一个
// 标记，后半段就被整体算成前一页 —— 第 6 张票的发票号码落在该段内，反查物理
// 页码时命中 [Page 5]，落库成 P5，页面上于是出现两个 P5、丢掉 P6。
//
// locator 的 Quote 是该块在原文中的精确文本（mineru 回填），用它把页码边界
// 定位到 chunk 内的真实偏移；Quote 缺失或在归一化文本中找不到时退化为
// 「只在 chunk 起点标记」，即保持旧行为而不是猜错位置。
func chunkPageMarkers(base int, text string, locs types.SourceLocators) []PageMarker {
	if len(locs) == 0 {
		return nil
	}
	cur := 0
	for _, l := range locs {
		if l.Page > 0 {
			cur = l.Page
			break
		}
	}
	if cur <= 0 {
		return nil
	}
	out := []PageMarker{{Offset: base, Page: cur}}
	cursor := 0
	for _, l := range locs {
		// 只承认「向后翻页」的 locator：页码回退说明定位器乱序或复用，
		// 据此打标记会把后文错标回前页。
		if l.Page <= cur {
			continue
		}
		quote := strings.TrimSpace(l.Quote)
		if quote == "" {
			continue
		}
		rel := strings.Index(text[cursor:], quote)
		if rel < 0 {
			continue
		}
		off := cursor + rel
		out = append(out, PageMarker{Offset: base + off, Page: l.Page})
		cursor = off + len(quote)
		cur = l.Page
	}
	return out
}

// invoiceNo20Re 匹配 20 位电子发票号码。
var invoiceNo20Re = regexp.MustCompile(`[0-9]{20}`)

// invoiceNoLabelRe 匹配「发票号码：」标签（旧版 8 位票号文件的锚点）。
var invoiceNoLabelRe = regexp.MustCompile(`发票号码\s*[:：]`)

// standaloneDigits 判断 s[start:end] 是否是独立数字串（左右都不是数字）。
// 21 位校验码不能被截出 20 位，否则锚点数会虚高、触发无谓的补提。
func standaloneDigits(s string, start, end int) bool {
	if start > 0 && s[start-1] >= '0' && s[start-1] <= '9' {
		return false
	}
	if end < len(s) && s[end] >= '0' && s[end] <= '9' {
		return false
	}
	return true
}

// CountInvoiceAnchors 统计源文档里「真实存在的票据锚点数」，用于核对提取完整性。
//
// 为什么必须有这个口径：多页 PDF 里只要某 1~2 页版式微变/跨 chunk 截断，
// 规则或模型就会漏抓那两页；而抓到的票字段完整，半残守卫判「完整」后直接
// 短路，漏掉的票被静默丢弃（真机 6 页只落 4 张）。锚点数就是「应该有几十张」。
//
// 优先数 20 位票号（去重，票号是票面唯一标识）；没有 20 位票号时退回数
// 「发票号码：」标签出现次数（一票一标签）。
func CountInvoiceAnchors(chunks []*types.Chunk) int {
	src := BuildExtractSourceText(chunks)
	if src.Content == "" {
		return 0
	}
	seen := make(map[string]struct{}, 8)
	for _, m := range invoiceNo20Re.FindAllStringIndex(src.Content, -1) {
		if !standaloneDigits(src.Content, m[0], m[1]) {
			continue
		}
		seen[src.Content[m[0]:m[1]]] = struct{}{}
	}
	if len(seen) > 0 {
		return len(seen)
	}
	return len(invoiceNoLabelRe.FindAllStringIndex(src.Content, -1))
}

// InvoiceSupplementTarget 是一张「原文里有、提取结果里没有」的票及其补提素材。
type InvoiceSupplementTarget struct {
	InvoiceNo string
	Page      int    // 物理页码（0 表示无法判定）
	Text      string // 该票所在页的原文（含票号/金额/购销方，够独立识别）
}

// FindSupplementTargets 定位「原文有锚点但结果里没有」的票，供上层补提。
//
// 行为约定：
//   - 结果里一张票都没有时返回 nil（调用方应走整份文档的常规提取路径）；
//   - 同一页有多个漏提锚点时只产出一个目标（一次调用即可覆盖整页）；
//   - 每段文本取该页的完整原文，最多 6000 字节（防止单页异常放大烧 token）。
func FindSupplementTargets(chunks []*types.Chunk, extracted []InvoiceExtractionItem, max int) []InvoiceSupplementTarget {
	if max <= 0 {
		max = 8
	}
	if len(extracted) == 0 {
		return nil
	}
	have := make(map[string]struct{}, len(extracted))
	for _, inv := range extracted {
		if no := strings.TrimSpace(inv.InvoiceNo); no != "" {
			have[no] = struct{}{}
		}
	}
	src := BuildExtractSourceText(chunks)
	if src.Content == "" {
		return nil
	}

	type anchor struct {
		no     string
		offset int
	}
	anchors := make([]anchor, 0, 8)
	seen := make(map[string]struct{}, 8)
	for _, m := range invoiceNo20Re.FindAllStringIndex(src.Content, -1) {
		if !standaloneDigits(src.Content, m[0], m[1]) {
			continue
		}
		no := src.Content[m[0]:m[1]]
		if _, dup := seen[no]; dup {
			continue
		}
		seen[no] = struct{}{}
		anchors = append(anchors, anchor{no: no, offset: m[0]})
	}

	out := make([]InvoiceSupplementTarget, 0, 2)
	usedPage := make(map[int]struct{}, 2)
	for _, a := range anchors {
		if _, ok := have[a.no]; ok {
			continue // 这张已经提出来了
		}
		page := src.PageAt(a.offset)
		if page > 0 {
			if _, dup := usedPage[page]; dup {
				continue // 同页只补一次
			}
			usedPage[page] = struct{}{}
		}
		text := pageSegmentText(src, a.offset, page)
		if text == "" {
			continue
		}
		out = append(out, InvoiceSupplementTarget{InvoiceNo: a.no, Page: page, Text: text})
		if len(out) >= max {
			break
		}
	}
	return out
}

// pageSegmentText 取出锚点所在页的原文；无页码信息时退回锚点附近的窗口。
func pageSegmentText(src ExtractSourceText, offset, page int) string {
	start, end := 0, len(src.Content)
	if page > 0 && len(src.Markers) > 0 {
		pageStart := -1
		for _, m := range src.Markers {
			if m.Page == page {
				pageStart = m.Offset
				break
			}
		}
		if pageStart >= 0 {
			pageEnd := len(src.Content)
			for _, m := range src.Markers {
				if m.Offset > pageStart {
					pageEnd = m.Offset
					break
				}
			}
			start, end = pageStart, pageEnd
		}
	}
	if page <= 0 {
		start = offset - 1200
		if start < 0 {
			start = 0
		}
		end = offset + 2400
		if end > len(src.Content) {
			end = len(src.Content)
		}
	}
	if start >= end {
		return ""
	}
	const maxBytes = 6000
	if end-start > maxBytes {
		end = start + maxBytes
	}
	seg := strings.TrimSpace(src.Content[start:end])
	if seg == "" {
		return ""
	}
	// 页级片段重新归一一次表格：拼接时表格可能被 chunk 边界切断（`<table>` 在
	// 前一个 chunk、数据行在后一个），整页切片后才是完整的 table，此时归一
	// 才能把 HTML 表格变成规则引擎认得的 Markdown 行——不归一的话补提只能靠模型。
	seg = docparser.NormalizeHTMLTables(seg)
	seg = strings.TrimSpace(splitMultiLabelHeaderCells(seg))
	return seg
}

// AssignPhysicalPages 按「发票号在原文中的首次出现位置」反查物理页码。
//
// 为什么用发票号定位而不是「结果数组下标」：规则提取（Line / MarkdownTable /
// FlatLayout）都是按出现顺序产出，但每张票可能跨多个 chunk，下标与 chunk 不是
// 一一对应；而发票号是票面唯一标识，`strings.Index` 找到的位置必然落在该票
// 所在的 chunk 段内，进而由 PageAt 得到它真实的物理页。
//
// 行为约定：
//   - 发票号为空（模型没给）→ 跳过，交给调用方回退序号；
//   - 发票号在原文中找不到 → 跳过（宁可不填，也不猜错页）；
//   - 同一发票号重复出现 → 按顺序消费，避免两张同号票都落到第一次出现的位置。
//
// 返回实际回填的张数。
func AssignPhysicalPages(src ExtractSourceText, invs []InvoiceExtractionItem) int {
	if len(src.Markers) == 0 || len(invs) == 0 || src.Content == "" {
		return 0
	}
	assigned := 0
	cursor := 0
	for i := range invs {
		no := strings.TrimSpace(invs[i].InvoiceNo)
		if no == "" {
			continue
		}
		rel := indexInvoiceNumber(src.Content, cursor, no)
		if rel < 0 {
			// 允许回退到全文再找一次（发票号可能在更早的 chunk 里被提及）
			rel = indexInvoiceNumber(src.Content, 0, no)
			if rel < 0 {
				continue
			}
		}
		page := src.PageAt(rel)
		if page <= 0 {
			continue
		}
		invs[i].PageNo = page
		cursor = rel + len(no)
		assigned++
	}
	// 兜底：跨块合并导致尾票与前一票同页（P5、P5）时，用「票号确实出现在
	// 下一物理页区间」这一证据把它推到下一页，不靠单纯数张数猜测。
	FixDuplicateTailPage(src, invs)
	return assigned
}

// indexInvoiceNumber 从 from 起查找发票号，优先独立数字串。
//
// 为什么要求独立：8 位票号（旧版票）会作为子串出现在 20 位电子票号或 21 位
// 校验码里，直接 strings.Index 会抢先命中错误位置、把 A 票的页码算到 B 票头上。
// 整篇都找不到独立出现时返回 -1（宁可让该票走序号兜底，也不拿嵌在更长数字串
// 里的子串当锚点——那正是「两个 P5」这类错标的来源）。
func indexInvoiceNumber(s string, from int, no string) int {
	if no == "" || from < 0 || from > len(s) {
		return -1
	}
	for i := from; i+len(no) <= len(s); {
		rel := strings.Index(s[i:], no)
		if rel < 0 {
			return -1
		}
		abs := i + rel
		if standaloneDigits(s, abs, abs+len(no)) {
			return abs
		}
		i = abs + 1
	}
	return -1
}

// FixDuplicateTailPage 修正「尾票与前一票同页」的错标。
//
// 触发条件（缺一不可，避免误伤真实的「一页两票」）：
//  1. 至少两张票，且前 N-1 张页码严格按 +1 单调递增（说明是「每页 1 张票」版式）；
//  2. 最后一张与倒数第二张页码相同；
//  3. 原文里确实存在第 page+1 页的锚点；
//  4. 最后一张票的发票号码出现在该锚点之后 —— 它是下一物理页上的票，
//     只是所在 chunk 从上一页起头，被 chunks 起点标记带偏了。
//
// 真实的「一页两票」不满足 3/4：两张票的票号都在同一页区间内，且后面通常
// 还有内容页锚点但没有该票号，因此不会被改动。
func FixDuplicateTailPage(src ExtractSourceText, invs []InvoiceExtractionItem) int {
	n := len(invs)
	if n < 2 || len(src.Markers) == 0 || src.Content == "" {
		return 0
	}
	last := &invs[n-1]
	if last.PageNo <= 0 || invs[n-2].PageNo != last.PageNo {
		return 0
	}
	for i := 1; i <= n-2; i++ {
		if invs[i].PageNo != invs[i-1].PageNo+1 {
			return 0
		}
	}
	next := last.PageNo + 1
	nextStart := -1
	for _, m := range src.Markers {
		if m.Page == next {
			nextStart = m.Offset
			break
		}
	}
	if nextStart < 0 {
		return 0
	}
	no := strings.TrimSpace(last.InvoiceNo)
	if no == "" {
		return 0
	}
	if idx := indexInvoiceNumber(src.Content, nextStart, no); idx < 0 {
		return 0
	}
	last.PageNo = next
	return 1
}

// extractPageMarker 生成写进批次文本的页码锚点。
// 单独成行，避免与票面字段粘连影响正则（Markdown 表格行以 ^| 开头，锚点行不会误命中）。
func extractPageMarker(page int) string {
	return "[Page " + itoa(page) + "]"
}

// itoa 小工具，避免为一处转换引入 strconv 依赖噪音。
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
