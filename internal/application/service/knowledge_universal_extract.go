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

// solarHeaderField 光伏账单表头类字段（非数值），用「标签 → 短值」规则提取。
//
// 为什么单独列：这些字段在 OCR 文本里是「发电户号 户名 纳税人类型 并网电压等级」
// 这类**标签与值被挤在同一行**的形态，且值可能紧贴下一个标签。确定性提取只
// 认「标签后紧跟、到下一个标签/数字/表格竖线为止」的片段，取不到就留空交给 LLM。
type solarHeaderField struct {
	Key    string
	Labels []string
}

var solarHeaderFields = []solarHeaderField{
	{"account_no", []string{"发电户号", "用电户号", "户号"}},
	{"account_name", []string{"户名", "客户名称", "账户名称"}},
	{"taxpayer_type", []string{"纳税人类型", "纳税人性质"}},
	{"voltage_level", []string{"并网电压等级", "并网电压"}},
	{"supply_unit", []string{"服务单位", "供电单位"}},
	{"address", []string{"发电地址", "用电地址", "地址"}},
	{"consumption_mode", []string{"消纳方式", "消纳模式"}},
	{"generation_mode", []string{"发电方式", "发电类型"}},
}

// solarHeaderRules 表头类字段的「形态正则」。
//
// 为什么不按标签位置配对：阶段 1 归一化会把中文之间的空格全部消掉（
// 「发 电 量」→「发电量」），账单表头因此变成一整串
//
//	发电户号户名纳税人类型并网电压等级|电交流380V|费账 5000303216586 重庆星达铜业有限公司一般纳税人
//
// 标签与值都粘连，按 token 计数配对必然错位（实测把「户名」配成「纳税人类型」）。
// 改为按字段的**枚举/形态特征**匹配：纳税人类型、消纳方式、发电方式本身就是
// 封闭枚举；户名以「公司/厂/集团」结尾；服务单位以「供电服务中心/供电公司」
// 结尾；户号是 10~16 位数字（电能表编号 20+ 位，天然排除）。
var solarHeaderRules = map[string]*regexp.Regexp{
	// 发电户号：10~16 位数字（20 位以上是电能表编号，必须排除）
	"account_no": regexp.MustCompile(`(?:发电户号|用电户号|户号|费账|账)\s*([0-9]{10,16})(?:\s|$|[^0-9])`),
	// 户名：以公司/厂/集团结尾，跳过「国网…电力公司」这类抬头
	"account_name":     regexp.MustCompile(`([\p{Han}]{2,28}(?:有限公司|有限责任公司|股份有限公司|公司|厂|集团|合作社))`),
	"taxpayer_type":    regexp.MustCompile(`(一般纳税人|小规模纳税人|非一般纳税人)`),
	"voltage_level":    regexp.MustCompile(`(交流|直流)\s*([0-9]+(?:\.[0-9]+)?\s*(?:kV|KV|kv|V|伏|千伏))`),
	"supply_unit":      regexp.MustCompile(`([\p{Han}]{2,14}(?:供电服务中心|供电服务公司|供电公司|供电所|电力公司))`),
	"address":          regexp.MustCompile(`((?:北京|上海|天津|重庆|(?:[\p{Han}]{2,6}省))[\p{Han}0-9]{4,80}(?:号|路|街道|社区|村|镇|园区)[\p{Han}0-9]{0,20})`),
	"consumption_mode": regexp.MustCompile(`(自发自用余电上网|全额上网|自发自用、余电上网|余电上网|自发自用)`),
	"generation_mode":  regexp.MustCompile(`(光伏发电|风力发电|水力发电|生物质发电|燃气发电)`),
}

// solarHeaderValue 取表头类字段的值：形态正则优先，失败再退回「标签相邻」。
// key 是字段名（account_no 等，即 solarHeaderRules 的键），labels 是票面标签。
func solarHeaderValue(text, key string, labels []string) string {
	if v := solarHeaderByRule(text, key); v != "" {
		return v
	}
	if v, ok := solarHeaderAdjacent(text, labels); ok {
		return v
	}
	return ""
}

// solarHeaderByRule 用字段形态正则取值。
func solarHeaderByRule(text, key string) string {
	if key == "" {
		return ""
	}
	if re, ok := solarHeaderRules[key]; ok {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			if len(m) < 2 {
				continue
			}
			v := strings.TrimSpace(m[1])
			// 电压等级有两个捕获组（交流/直流 + 数值单位），必须拼回完整值，
			// 否则「交流380V」会被截成「交流」。
			if key == "voltage_level" && len(m) >= 3 {
				v = m[1] + strings.ReplaceAll(strings.TrimSpace(m[2]), " ", "")
			}
			if v == "" {
				continue
			}
			// 抬头噪声：国网/电力公司 不是客户户名
			if key == "account_name" && (strings.Contains(v, "国网") || strings.HasPrefix(v, "国") ||
				strings.Contains(v, "供电")) {
				continue
			}
			return v
		}
	}
	return ""
}

// solarHeaderAdjacent 规则二：标签与值在同一格里相邻（规整文本）。
func solarHeaderAdjacent(text string, labels []string) (string, bool) {
	for _, label := range labels {
		re, err := regexp.Compile(regexp.QuoteMeta(label) + `[：:]\s*([^|\n]{1,64})`)
		if err != nil {
			continue
		}
		m := re.FindStringSubmatch(text)
		if len(m) < 2 {
			continue
		}
		v := strings.TrimSpace(m[1])
		if solarContainsOtherLabel(v, label) {
			continue
		}
		v = strings.TrimSpace(strings.Trim(v, "|:： "))
		if v == "" || isAllDigits(v) || solarIsLabelOnly(v) {
			continue
		}
		return v, true
	}
	return "", false
}

// isAllDigits 判断字符串是否只由数字与小数点组成（用于过滤数值串）。
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	return true
}

// solarContainsOtherLabel 判断值里是否还含其他表头标签。
func solarContainsOtherLabel(v, self string) bool {
	for _, f := range solarHeaderFields {
		for _, l := range f.Labels {
			if l == self {
				continue
			}
			if strings.Contains(v, l) {
				return true
			}
		}
	}
	return false
}

// solarIsLabelOnly 判断片段是否本身就是若干标签的拼接（不含真实值）。
func solarIsLabelOnly(v string) bool {
	rest := v
	for _, f := range solarHeaderFields {
		for _, l := range f.Labels {
			rest = strings.ReplaceAll(rest, l, "")
		}
	}
	return strings.TrimSpace(rest) == ""
}

// solarGatewayRe 切出「上网关口 / 发电关口」明细块起点。
// 表号不在这里捕获：`[^\n]{0,80}` 是贪心的，把表号写进同一正则会被整段吃掉。
var solarGatewayRe = regexp.MustCompile(`(上网关口|发电关口)`)

// solarMeterNoRe 关口段内的电能表编号。
var solarMeterNoRe = regexp.MustCompile(`电能表编号[：:]?\s*([0-9]{6,32})`)

// solarReadingRowRe 电量明细表（示数类型/上期示数/本期示数/倍率/抄见电量/计费电量）。
// 行尾常有 OCR 多出的空单元格（`|a|b|c|d|e|f||`），故用 `\|*` 收尾。
var solarReadingRowRe = regexp.MustCompile(`(?m)^\|(反向有功[^|]*)\|([^|]*)\|([^|]*)\|([^|]*)\|([^|]*)\|([^|]*)\|*\s*$`)

// solarFeeRowRe 电费明细表（类别/电量/电价/电费），同样容忍行尾多余空单元格。
var solarFeeRowRe = regexp.MustCompile(`(?m)^\|([^|]+)\|([^|]*)\|([^|]*)\|([^|]*)\|*\s*$`)

// solarGatewaySections 把归一化文本按关口切成段，供逐段取明细。
func solarGatewaySections(text string) []struct {
	Kind    string
	MeterNo string
	Body    string
} {
	locs := solarGatewayRe.FindAllStringSubmatchIndex(text, -1)
	if len(locs) == 0 {
		return nil
	}
	out := make([]struct {
		Kind    string
		MeterNo string
		Body    string
	}, 0, len(locs))
	for i, loc := range locs {
		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		kind := text[loc[2]:loc[3]]
		body := text[loc[0]:end]
		meter := ""
		if m := solarMeterNoRe.FindStringSubmatch(body); len(m) > 1 {
			meter = strings.TrimSpace(m[1])
		}
		out = append(out, struct {
			Kind    string
			MeterNo string
			Body    string
		}{Kind: kind, MeterNo: meter, Body: body})
	}
	return out
}

// buildSolarGateways 从分段文本里解析关口明细（电量行 + 电费行）。
//
// 这一步是本次修复的核心：旧版快筛只产出 4 个概览字段、从不构建 gateways，
// 于是「上期示数/本期示数/倍率/抄见电量/计费电量」全部落 0（真机抽屉一屏
// 「请输入」）。现在把账单原文里的明细表原样结构化，不再依赖模型。
func buildSolarGateways(text string) []types.SolarGateway {
	sections := solarGatewaySections(text)
	if len(sections) == 0 {
		return nil
	}
	gateways := make([]types.SolarGateway, 0, len(sections))
	for _, sec := range sections {
		g := types.SolarGateway{GatewayType: sec.Kind, MeterNo: sec.MeterNo}
		// 项目名称：紧跟在「上网关口」段里的独立一行（如 1.4MW屋顶分布式光伏发电项目）
		if m := regexp.MustCompile(`([0-9.]+MW[^|\n]{0,40}光伏[^|\n]{0,20})`).FindStringSubmatch(sec.Body); len(m) > 1 {
			g.ProjectName = strings.TrimSpace(m[1])
		}
		for _, m := range solarReadingRowRe.FindAllStringSubmatch(sec.Body, -1) {
			g.Readings = append(g.Readings, types.SolarReading{
				MeterType:  strings.TrimSpace(m[1]),
				Prev:       parseFlexibleNumber(m[2]),
				Curr:       parseFlexibleNumber(m[3]),
				Multiplier: parseFlexibleNumber(m[4]),
				ReadingKwh: parseFlexibleNumber(m[5]),
				BillKwh:    parseFlexibleNumber(m[6]),
			})
		}
		// 电费行去重：OCR 会把同一张「本期电费明细」表复制两遍（真机发电关口
		// 就出现两条完全相同的「分布式电源发电补助 19856 0 0」）。按
		// 类别+电量+电价+电费 四元组去重，避免下游重复累计。
		seenFee := make(map[string]bool, 4)
		for _, m := range solarFeeRowRe.FindAllStringSubmatch(sec.Body, -1) {
			cat := strings.TrimSpace(m[1])
			// 表头行与分隔行跳过
			if cat == "" || cat == "类别" || strings.HasPrefix(cat, "---") || strings.Contains(cat, "示数类型") {
				continue
			}
			qty := parseFlexibleNumber(m[2])
			rate := parseFlexibleNumber(m[3])
			fee := parseFlexibleNumber(m[4])
			if qty == 0 && rate == 0 && fee == 0 {
				continue
			}
			key := fmt.Sprintf("%s|%v|%v|%v", cat, qty, rate, fee)
			if seenFee[key] {
				continue
			}
			seenFee[key] = true
			g.Fees = append(g.Fees, types.SolarFeeItem{Category: cat, Qty: qty, Rate: rate, Fee: fee})
		}
		if g.GatewayType == "" && g.MeterNo == "" && len(g.Readings) == 0 && len(g.Fees) == 0 {
			continue
		}
		gateways = append(gateways, g)
	}
	return gateways
}

// parseFlexibleNumber 解析表格单元格里的数字（容忍空值、千分位、单位后缀）。
func parseFlexibleNumber(raw string) float64 {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0
	}
	s = strings.ReplaceAll(s, ",", "")
	re := regexp.MustCompile(`-?[0-9]+(?:\.[0-9]+)?`)
	if m := re.FindString(s); m != "" {
		v, err := strconv.ParseFloat(m, 64)
		if err == nil {
			return v
		}
	}
	return 0
}

// solarCumulativeKwh 年累计上网电量。
//
// 不用 canonicalNumberAfterLabel：它要求标签前是行首/竖线/空白，而账单原文是
// 「您的年累计上网电量为117960」——标签前是「的」，会被漏掉。
func solarCumulativeKwh(text string) float64 {
	re := regexp.MustCompile(`(?:年累计上网电量|年累计发电量|累计上网电量)[^0-9\n]{0,12}([0-9][0-9,]*(?:\.[0-9]+)?)`)
	if m := re.FindStringSubmatch(text); len(m) >= 2 {
		if v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64); err == nil && v > 0 {
			return v
		}
	}
	return 0
}

// solarMomChange 本期上网电量环比（原文形如「增长-83.08%」）。
func solarMomChange(text string) string {
	// 原文形如「较上期 上网关口 (电能表编号:...)重庆星达铜业有限公司增长-83.08%」，
	// 「较上期」与百分比之间隔了整段关口信息，窗口必须放宽到 200 字符。
	re := regexp.MustCompile(`(?:较上期|环比)[^\n%]{0,200}?(增长|下降|减少|上升)?\s*(-?[0-9.]+)%`)
	if m := re.FindStringSubmatch(text); len(m) >= 3 {
		return strings.TrimSpace(m[1]) + m[2] + "%"
	}
	return ""
}

// FastPathSolarBill 光伏账单确定性快筛。
//
// 只有「账单周期 + 光伏发电量 + 上网电量 + 结算金额」四个核心字段全部精确命中、
// 且通过数值校验（发电量 ≥ 上网电量、金额为正、单价在合理区间）才返回 true，
// 调用方据此**完全跳过大模型**。任意一个没命中就回退到 LLM 管线，绝不产出全 0 记录。
//
// cfg 里的字段级 regex 与 required 实时生效：用户改了正则/规则立刻反映到结果。
//
// 第 43 轮补齐：命中后**同一份原文**继续做表头字段（发电户号/户名/纳税人类型/
// 并网电压/消纳方式/发电方式/服务单位/发电地址）、年累计、环比与关口明细，
// 并上提 meter_prev/curr/ratio/reading/bill 顶层镜像。旧版只产出 4 个概览字段，
// 导致真机抽屉除发电量/上网电量/结算金额外全是「请输入」/0。
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

	// ---- 表头类字段（非数值）：确定性取「标签 → 短值」 ----
	for _, f := range solarHeaderFields {
		if v := solarHeaderValue(canonical, f.Key, f.Labels); v != "" {
			switch f.Key {
			case "account_no":
				item.AccountNo = v
			case "account_name":
				item.AccountName = v
			case "taxpayer_type":
				item.TaxpayerType = v
			case "voltage_level":
				item.VoltageLevel = v
			case "supply_unit":
				item.SupplyUnit = v
			case "address":
				item.Address = v
			case "consumption_mode":
				item.ConsumptionMode = v
			case "generation_mode":
				item.GenerationMode = v
			}
			item.Fields[f.Key] = v
		}
	}
	// 并网电压等级在 OCR 里常被拆成「电 交流380V」「费 账」等噪声前缀，
	// 单独兜一次「交流/直流 + 电压」形态。
	if item.VoltageLevel == "" {
		if m := regexp.MustCompile(`(交流|直流)\s*([0-9]+(?:\.[0-9]+)?V)`).FindStringSubmatch(canonical); len(m) >= 3 {
			item.VoltageLevel = m[1] + m[2]
			item.Fields["voltage_level"] = item.VoltageLevel
		}
	}
	// 发电户号：表头常被 OCR 打散成「费 账 5000303216586」，用长数字兜底。
	if item.AccountNo == "" {
		if m := regexp.MustCompile(`(?:发电户号|户号)[^0-9]{0,24}([0-9]{10,20})`).FindStringSubmatch(canonical); len(m) >= 2 {
			item.AccountNo = m[1]
			item.Fields["account_no"] = m[1]
		}
	}

	// ---- 年累计 / 环比 ----
	if v := solarCumulativeKwh(canonical); v > 0 {
		item.CumulativeKwh = round2(v)
		item.Fields["cumulative_kwh"] = item.CumulativeKwh
	}
	if v := solarMomChange(canonical); v != "" {
		item.MomChange = v
		item.Fields["mom_change"] = v
	}

	// ---- 关口明细（上网关口 + N 个发电关口） ----
	if gateways := buildSolarGateways(canonical); len(gateways) > 0 {
		item.Gateways = gateways
		// 顶层镜像上期示数/本期示数/倍率/抄见电量/计费电量，
		// 否则列表有值、抽屉一屏「请输入」（历史事故）。
		upliftSolarMeterReading(item)
		if item.MeterPrev != 0 {
			item.Fields["meter_prev"] = item.MeterPrev
		}
		if item.MeterCurr != 0 {
			item.Fields["meter_curr"] = item.MeterCurr
		}
		if item.MeterRatio != 0 {
			item.Fields["meter_ratio"] = item.MeterRatio
		}
		if item.MeterReading != 0 {
			item.Fields["meter_reading"] = item.MeterReading
		}
		if item.MeterBill != 0 {
			item.Fields["meter_bill"] = item.MeterBill
		}
	}

	// ---- 用户配置的其余字段：同一份原文正则补洞（快筛路径也必须执行） ----
	// 旧版快筛命中后 handler 直接 break 并跳过 ApplySolarRegexPatch，
	// 于是用户自定义字段在快筛路径上永久缺失。
	ApplySolarRegexPatch(cfg, item, canonical)

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
