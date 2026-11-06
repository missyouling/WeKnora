package service

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Tencent/WeKnora/internal/types"
)

// 全解析引擎自适应提取器（Universal Extractor Engine）。
//
// 9 种解析引擎（内置 DocReader / Simple / anydoc / MarkItDown / MinerU /
// MinerU Cloud / PaddleOCR-VL / PaddleOCR-VL Cloud / OpenDataLoader）产出的
// 文本形态完全不同：有的给裸 HTML <table>，有的给 Markdown，有的给带大量
// 字间空格的 OCR 文本，有的给全角标点 + 千分位金额。下游提取器此前各写一份
// 清洗，口径不一；本文件把「归一化 → 配置驱动快筛 → 单次结构化 LLM → 正则补洞」
// 收口成四个可单测的纯函数，所有业务模块共用。
//
// 阶段 1：CanonicalizeParsedDocument / AggregateChunksForExtraction
// 阶段 2：FastPathUserFields / BuildAdaptiveExtractPrompt / RegexPatchUserFields
//         FastPathSolarBill（账单专用确定性快筛）
// 阶段 3：原始文件流兜底 ParseOriginalFileToCanonicalText
// 阶段 4：WithAdaptiveExtractTimeout（单次 LLM 25s 硬超时）

// AdaptiveExtractLLMTimeout 单次结构化 LLM 提取的硬超时。
// 旧管线单次最长 5min，且默认 4 次重试 → 单个文件可能挂 20 分钟，
// 页面表现就是「提取中」永不落定、刷新后又被重复投递。
const AdaptiveExtractLLMTimeout = 25 * time.Second

// WithAdaptiveExtractTimeout 给单次 LLM 调用套上阶段 2 的紧凑超时。
func WithAdaptiveExtractTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, AdaptiveExtractLLMTimeout)
}

// ---------------------------------------------------------------------------
// 阶段 1：多解析引擎输出归一化
// ---------------------------------------------------------------------------

var (
	universalHTMLTableRe = regexp.MustCompile(`(?is)<table\b[^>]*>.*?</table>`)
	universalHTMLRowRe   = regexp.MustCompile(`(?is)<tr\b[^>]*>.*?</tr>`)
	universalHTMLCellRe  = regexp.MustCompile(`(?is)<(?:th|td)\b[^>]*>(.*?)</(?:th|td)>`)
	universalHTMLTagRe   = regexp.MustCompile(`(?s)<[^>]+>`)
	// CJK 字间空格：用捕获两侧字符的写法（Go RE2 不支持 lookahead），重复应用即可。
	universalCJKGapRe = regexp.MustCompile(`(\p{Han})[ \t]+(\p{Han})`)
	// 千分位：只处理「数字,三位数字」。Go RE2 不支持负向前瞻，改为循环替换
	// 直到稳定（1,234,567 → 1234,567 → 1234567），最多 4 轮防止病态输入。
	universalThousandsRe  = regexp.MustCompile(`([0-9]),([0-9]{3})`)
	universalUnitSuffix   = regexp.MustCompile(`([0-9])[ \t]*(?:¥|￥|元|kWh|KWH|kwh|千瓦时|度)`)
	universalUnitPrefix   = regexp.MustCompile(`(?:¥|￥)[ \t]*([0-9])`)
	universalBlankLines   = regexp.MustCompile(`\n{3,}`)
	universalTrailingWS   = regexp.MustCompile(`[ \t]+\n`)
	universalHTMLEntities = strings.NewReplacer(
		"&nbsp;", " ", "&#160;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">",
		"&quot;", `"`, "&#39;", "'", "&mdash;", "-",
	)
)

// NormalizeFullWidth 统一全角字符为半角（：（）０-９，．％ 等），全角空格转半角。
// 只映射 U+FF01..U+FF5E，避免动到中文标点以外的语义。
func NormalizeFullWidth(s string) string {
	if !strings.ContainsAny(s, "：（）０１２３４５６７８９，．％＋－／　") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\u3000':
			b.WriteByte(' ')
		case r >= '\uFF01' && r <= '\uFF5E':
			b.WriteRune(r - 0xFEE0)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CanonicalizeHTMLTables 把 HTML <table> 无损转换为
// 「标准 Markdown 表格行」+「表头: 单元格值」键值对文本。
//
// 为什么两份都要：MinerU / PaddleOCR-VL / OpenDataLoader 输出的表头经常是
// 合并单元格或跨行，模型读 Markdown 表格能对齐列，但确定性正则更依赖
// 「字段名: 值」这种扁平形态。两份并列可以让规则快筛与 LLM 各取所需。
func CanonicalizeHTMLTables(s string) string {
	return universalHTMLTableRe.ReplaceAllStringFunc(s, func(table string) string {
		rows := universalHTMLRowRe.FindAllString(table, -1)
		if len(rows) == 0 {
			return universalHTMLEntities.Replace(universalHTMLTagRe.ReplaceAllString(table, " "))
		}
		grid := make([][]string, 0, len(rows))
		for _, row := range rows {
			cells := universalHTMLCellRe.FindAllStringSubmatch(row, -1)
			line := make([]string, 0, len(cells))
			for _, c := range cells {
				v := universalHTMLTagRe.ReplaceAllString(c[1], " ")
				v = universalHTMLEntities.Replace(v)
				line = append(line, strings.Join(strings.Fields(v), " "))
			}
			if len(line) > 0 {
				grid = append(grid, line)
			}
		}
		if len(grid) == 0 {
			return universalHTMLEntities.Replace(universalHTMLTagRe.ReplaceAllString(table, " "))
		}
		var b strings.Builder
		b.WriteString("\n| ")
		b.WriteString(strings.Join(grid[0], " | "))
		b.WriteString(" |\n| ")
		seps := make([]string, len(grid[0]))
		for i := range seps {
			seps[i] = "---"
		}
		b.WriteString(strings.Join(seps, " | "))
		b.WriteString(" |\n")
		for _, row := range grid[1:] {
			b.WriteString("| ")
			b.WriteString(strings.Join(row, " | "))
			b.WriteString(" |\n")
			for i, v := range row {
				if i < len(grid[0]) && strings.TrimSpace(grid[0][i]) != "" && strings.TrimSpace(v) != "" {
					b.WriteString(strings.TrimSpace(grid[0][i]))
					b.WriteString(": ")
					b.WriteString(v)
					b.WriteString("\n")
				}
			}
		}
		b.WriteString("\n")
		return b.String()
	})
}

// CanonicalizeParsedDocument 阶段 1 主入口：异构格式统一清洗。
//
// 顺序固定：HTML 表格结构化 → 全角归一 → OCR 字间空格 → 数字千分位/单位 →
// 空行压缩。幂等（对已归一文本二次调用结果不变）。
func CanonicalizeParsedDocument(raw string) string {
	if raw == "" {
		return ""
	}
	s := strings.ReplaceAll(raw, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = CanonicalizeHTMLTables(s)
	s = NormalizeFullWidth(s)
	// 单位必须在 CJK 空格清理**之前**去掉：否则「66026 千瓦时 上网电量」里
	// 的「时 上」会被当成 OCR 字间空格粘成「时上」，单位删除后数字与下一个
	// 标签就直接贴死（真机：发电量 66026上网电量 2970结算金额）。
	for i := 0; i < 4; i++ {
		next := universalThousandsRe.ReplaceAllString(s, "${1}${2}")
		if next == s {
			break
		}
		s = next
	}
	s = universalUnitSuffix.ReplaceAllString(s, "${1}")
	s = universalUnitPrefix.ReplaceAllString(s, "${1}")
	// 「发 电 量」→「发电量」：每轮消掉一层间隔，3 轮足以覆盖 4 字字段。
	for i := 0; i < 3; i++ {
		s = universalCJKGapRe.ReplaceAllString(s, "${1}${2}")
	}
	s = universalBlankLines.ReplaceAllString(s, "\n\n")
	s = universalTrailingWS.ReplaceAllString(s, "\n")
	return strings.TrimSpace(s)
}

// AggregateChunksForExtraction 完整聚合全文：按顺序拼接**所有**可用于提取的
// Chunk（text / image_ocr / image_caption），不取第 1 个、不截断表格。
//
// 分页交给下游 Build*ExtractionBatches 按 rune 预算切片；这里只负责"一个字都不少"。
func AggregateChunksForExtraction(chunks []*types.Chunk) string {
	usable := ChunksForExtraction(chunks)
	parts := make([]string, 0, len(usable))
	for _, c := range usable {
		if t := CanonicalizeParsedDocument(c.Content); t != "" {
			parts = append(parts, t)
		}
	}
	return CanonicalizeParsedDocument(strings.Join(parts, "\n\n"))
}

// CanonicalTextRunes 返回可用于判断「解析是否产出有效文本」的非空白字符数。
func CanonicalTextRunes(text string) int {
	n := 0
	for _, r := range text {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	return n
}

// SplitCanonicalBatches 按 rune 预算把归一化全文切成批次。
// 账单类必须整份优先（所有明细落同一条记录），只有超长才拆。
func SplitCanonicalBatches(text string, maxRunes int) []string {
	if maxRunes <= 0 {
		maxRunes = 60000
	}
	return splitTextBatches(text, maxRunes+1, maxRunes)
}

// ParseOriginalFileToCanonicalText 阶段 1 兜底：解析引擎不可用 / 产出空文本时，
// 直接从原始 PDF/图片字节流抽文本，绝不产出全 0 记录、也绝不无限等待。
// 复用样票解析链路（纯计算：不落库、不投递任务、不碰 chunks）。
func ParseOriginalFileToCanonicalText(ctx context.Context, deps ExtractSuggestParser, in ExtractSuggestInput) (string, error) {
	res, err := ParseFileForExtractSuggest(ctx, deps, in)
	if err != nil {
		return "", err
	}
	return CanonicalizeParsedDocument(res.Text), nil
}

// ---------------------------------------------------------------------------
// 阶段 2：用户配置（Prompt + 正则）实时驱动
// ---------------------------------------------------------------------------

// ConfiguredFieldRegex 取用户为某字段配置的正则（用户配置优先于内置）。
func ConfiguredFieldRegex(cfg *types.KbExtractConfig, names ...string) string {
	if cfg == nil {
		return ""
	}
	for _, name := range names {
		for _, f := range cfg.Fields {
			if !f.Enabled || f.Regex == "" {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(f.Name), strings.TrimSpace(name)) ||
				strings.EqualFold(strings.TrimSpace(f.Label), strings.TrimSpace(name)) {
				return f.Regex
			}
		}
	}
	return ""
}

// FastPathUserFields 用用户配置的字段级 regex 在归一化文本上做快筛。
// 返回 (字段值, 是否所有 required 字段都命中)。命中时不调用大模型。
func FastPathUserFields(cfg *types.KbExtractConfig, text string) (map[string]string, bool) {
	values := make(map[string]string)
	if cfg == nil || strings.TrimSpace(text) == "" {
		return values, false
	}
	complete := false
	seen := 0
	for _, f := range cfg.Fields {
		if !f.Enabled {
			continue
		}
		seen++
		val := ""
		if f.Regex != "" {
			if re, err := regexp.Compile(f.Regex); err == nil {
				m := re.FindStringSubmatch(text)
				if len(m) > 1 {
					val = strings.TrimSpace(m[1])
				} else if len(m) > 0 {
					val = strings.TrimSpace(m[0])
				}
			}
		}
		if val != "" {
			values[f.Name] = val
		} else if f.Required {
			return values, false
		}
	}
	complete = seen > 0 && len(values) == seen
	return values, complete
}

// RegexPatchUserFields 大模型返回后，用用户配置的 regex 对缺失字段补洞。
// 返回本次补上的字段名，供可观测性面板展示。
func RegexPatchUserFields(cfg *types.KbExtractConfig, values map[string]string, text string) []string {
	if cfg == nil || values == nil {
		return nil
	}
	patched := make([]string, 0)
	for _, f := range cfg.Fields {
		if !f.Enabled || f.Regex == "" {
			continue
		}
		if v := strings.TrimSpace(values[f.Name]); v != "" {
			continue
		}
		re, err := regexp.Compile(f.Regex)
		if err != nil {
			continue
		}
		m := re.FindStringSubmatch(text)
		val := ""
		if len(m) > 1 {
			val = strings.TrimSpace(m[1])
		} else if len(m) > 0 {
			val = strings.TrimSpace(m[0])
		}
		if val != "" {
			values[f.Name] = val
			patched = append(patched, f.Name)
		}
	}
	return patched
}

// BuildAdaptiveExtractPrompt 把用户为每个字段写的描述 / 提取规则动态组装成
// **单次**紧凑 JSON Schema 提取指令：去除多余空行、不带示例文档，压缩 Token。
func BuildAdaptiveExtractPrompt(cfg *types.KbExtractConfig, text string) string {
	var b strings.Builder
	b.WriteString("从文档提取字段，只返回严格 JSON 对象（键=name，值类型按 type），不要解释、不要代码块。\n")
	b.WriteString("字段 schema（name/type/required/rule）：\n")
	if cfg != nil {
		for _, f := range cfg.Fields {
			if !f.Enabled {
				continue
			}
			rule := strings.TrimSpace(strings.Join([]string{f.Desc, f.Rule}, " "))
			rule = strings.Join(strings.Fields(rule), " ")
			if rule == "" {
				rule = "按票面原文提取"
			}
			b.WriteString(fmt.Sprintf("{%q,%q,%t,%q}\n", f.Name, f.Type, f.Required, rule))
		}
		if tpl := strings.TrimSpace(cfg.PromptTemplate); tpl != "" {
			b.WriteString("用户模板：" + strings.Join(strings.Fields(tpl), " ") + "\n")
		}
	}
	b.WriteString("<document>\n")
	b.WriteString(universalBlankLines.ReplaceAllString(strings.TrimSpace(text), "\n"))
	b.WriteString("\n</document>")
	return b.String()
}

// ---------------------------------------------------------------------------
// 阶段 2（账单专用）：确定性快筛，命中即毫秒级返回、跳过 LLM
// ---------------------------------------------------------------------------

var (
	universalLabeledNumberPrefix = `(?m)(?:^|[|\s])(`
	universalLabeledNumberSuffix = `)[^\n0-9\-]{0,24}(-?[0-9][0-9,]*(?:\.[0-9]+)?)`
	universalBillPeriodRe        = regexp.MustCompile(`(20\d{2})[-/.年](\d{1,2})`)
	universalBillPeriodLabel     = regexp.MustCompile(`(?m)(?:账单周期|结算周期|电费月份|所属期)[^\n0-9]{0,16}(20\d{2})[-/.年](\d{1,2})`)
)

// canonicalNumberAfterLabel 在归一化文本里按「标签 → 数字」取值。
// 用户配置了字段级 regex 时优先用用户正则（改配置立即生效）。
func canonicalNumberAfterLabel(text, userRegex string, labels []string) (float64, bool) {
	if userRegex != "" {
		if re, err := regexp.Compile(userRegex); err == nil {
			if m := re.FindStringSubmatch(text); len(m) > 0 {
				raw := m[len(m)-1]
				if v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(raw), ",", ""), 64); err == nil {
					return v, true
				}
			}
		}
	}
	quoted := make([]string, 0, len(labels))
	for _, l := range labels {
		quoted = append(quoted, regexp.QuoteMeta(l))
	}
	re, err := regexp.Compile(universalLabeledNumberPrefix + strings.Join(quoted, "|") + universalLabeledNumberSuffix)
	if err != nil {
		return 0, false
	}
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		raw := strings.ReplaceAll(strings.TrimSpace(m[2]), ",", "")
		v, err := strconv.ParseFloat(raw, 64)
		if err == nil && v != 0 {
			return v, true
		}
	}
	return 0, false
}

// FastPathSolarBill 光伏账单确定性快筛。
//
// 只有「账单周期 + 光伏发电量 + 上网电量 + 结算金额」四个核心字段全部精确命中、
// 且通过数值校验（发电量 ≥ 上网电量、金额为正、单价在合理区间）才返回 true，
// 调用方据此**完全跳过大模型**。任意一个没命中就回退到 LLM 管线，绝不产出全 0 记录。
//
// cfg 里的字段级 regex 与 required 实时生效：用户改了正则/规则立刻反映到结果。
func FastPathSolarBill(text, fileName string, cfg *types.KbExtractConfig) (*types.SolarBillExtractionItem, bool) {
	canonical := CanonicalizeParsedDocument(text)
	if CanonicalTextRunes(canonical) < 24 {
		return nil, false
	}

	gen, okGen := canonicalNumberAfterLabel(canonical,
		ConfiguredFieldRegex(cfg, "generation_kwh", "光伏发电量", "发电量"),
		[]string{"光伏发电量", "发电量", "总发电量"})
	grid, okGrid := canonicalNumberAfterLabel(canonical,
		ConfiguredFieldRegex(cfg, "grid_kwh", "上网电量"),
		[]string{"上网电量", "余电上网电量"})
	amount, okAmount := canonicalNumberAfterLabel(canonical,
		ConfiguredFieldRegex(cfg, "settlement_amount", "结算金额"),
		[]string{"结算金额", "本期结算金额", "结算电费", "应付金额"})
	selfUse, _ := canonicalNumberAfterLabel(canonical,
		ConfiguredFieldRegex(cfg, "self_use_kwh", "自发自用电量"),
		[]string{"自发自用电量", "自用电量"})
	unitPrice, okPrice := canonicalNumberAfterLabel(canonical,
		ConfiguredFieldRegex(cfg, "unit_price", "结算单价"),
		[]string{"结算单价", "上网电价", "电价"})

	period := ""
	if m := universalBillPeriodLabel.FindStringSubmatch(canonical); len(m) >= 3 {
		mm, _ := strconv.Atoi(m[2])
		period = fmt.Sprintf("%s-%02d", m[1], mm)
	}
	if period == "" {
		hay := canonical + "\n" + fileName
		if m := universalBillPeriodRe.FindStringSubmatch(hay); len(m) >= 3 {
			mm, _ := strconv.Atoi(m[2])
			period = fmt.Sprintf("%s-%02d", m[1], mm)
		}
	}
	if !(okGen && okGrid && okAmount) || period == "" {
		return nil, false
	}
	// 数值校验：任何一个明显不合理的组合都退回 LLM，避免把快筛误命中写进库。
	if gen <= 0 || grid <= 0 || amount <= 0 || gen < grid {
		return nil, false
	}
	if okPrice && (unitPrice <= 0 || unitPrice > 10) {
		return nil, false
	}
	if ratio := amount / grid; ratio <= 0 || ratio > 5 {
		return nil, false
	}

	start := period + "-01"
	end := period + "-31"
	item := &types.SolarBillExtractionItem{
		BillPeriodStart:  start,
		BillPeriodEnd:    end,
		BillPeriod:       period,
		GenerationKwh:    round2(gen),
		GridKwh:          round2(grid),
		SettlementAmount: round2(amount),
		Remark:           "由通用自适应提取器确定性快筛生成（跳过 LLM）",
		Fields: map[string]any{
			"bill_period":       period,
			"generation_kwh":    round2(gen),
			"grid_kwh":          round2(grid),
			"settlement_amount": round2(amount),
		},
	}
	if selfUse > 0 {
		item.Fields["self_use_kwh"] = round2(selfUse)
	}
	if okPrice && unitPrice > 0 {
		item.Fields["unit_price"] = unitPrice
	}
	return item, true
}

// ApplySolarRegexPatch 阶段 2 收口：LLM 返回后，用用户配置的字段级 regex
// 对缺失字段补洞，并把补到的值写进记录的动态字段桶。
//
// 用户在「提取规则」里改任意一条正则后，下一次提取立即生效，无需改代码。
// 快筛路径已经用同一份 ConfiguredFieldRegex 取过值，不重复执行。
func ApplySolarRegexPatch(cfg *types.KbExtractConfig, item *types.SolarBillExtractionItem, text string) []string {
	if cfg == nil || item == nil {
		return nil
	}
	values := make(map[string]string, len(item.Fields)+8)
	for k, v := range item.Fields {
		if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
			values[k] = s
		}
	}
	// 顶层固定字段也参与「是否已命中」判定，避免重复补洞。
	values["bill_period"] = item.BillPeriod
	values["generation_kwh"] = strconv.FormatFloat(item.GenerationKwh, 'f', -1, 64)
	values["grid_kwh"] = strconv.FormatFloat(item.GridKwh, 'f', -1, 64)
	values["settlement_amount"] = strconv.FormatFloat(item.SettlementAmount, 'f', -1, 64)
	patched := RegexPatchUserFields(cfg, values, CanonicalizeParsedDocument(text))
	if len(patched) == 0 {
		return nil
	}
	if item.Fields == nil {
		item.Fields = make(map[string]any)
	}
	for _, k := range patched {
		item.Fields[k] = values[k]
	}
	return patched
}
