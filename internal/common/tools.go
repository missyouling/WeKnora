package common

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/logger"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// ToInterfaceSlice converts a slice of strings to a slice of empty interfaces.
func ToInterfaceSlice[T any](slice []T) []interface{} {
	interfaceSlice := make([]interface{}, len(slice))
	for i, v := range slice {
		interfaceSlice[i] = v
	}
	return interfaceSlice
}

// []string -> string, " join, space separated
func StringSliceJoin(slice []string) string {
	result := make([]string, len(slice))
	for i, v := range slice {
		result[i] = `"` + v + `"`
	}
	return strings.Join(result, " ")
}

func GetAttrs[A, B any](extract func(A) B, attrs ...A) []B {
	result := make([]B, len(attrs))
	for i, attr := range attrs {
		result[i] = extract(attr)
	}
	return result
}

// Deduplicate removes duplicates from a slice based on a key function
// T: the type of elements in the slice
// K: the type of key used for deduplication
func Deduplicate[T any, K comparable](keyFunc func(T) K, items ...T) []T {
	seen := make(map[K]T)
	for _, item := range items {
		key := keyFunc(item)
		if _, exists := seen[key]; !exists {
			seen[key] = item
		}
	}
	return slices.Collect(maps.Values(seen))
}

// ScoreComparable is an interface for types that have a Score method returning float64
type ScoreComparable interface {
	GetScore() float64
}

// DeduplicateWithScore removes duplicates from a slice based on a key function,
// keeping the item with the highest score for each key, then sorts by score descending
// T: the type of elements in the slice (must implement ScoreComparable)
// K: the type of key used for deduplication
func DeduplicateWithScore[T ScoreComparable, K comparable](keyFunc func(T) K, items ...T) []T {
	seen := make(map[K]T)
	for _, item := range items {
		key := keyFunc(item)
		if existing, exists := seen[key]; !exists {
			seen[key] = item
		} else if item.GetScore() > existing.GetScore() {
			seen[key] = item
		}
	}
	result := slices.Collect(maps.Values(seen))
	// Sort by score descending
	slices.SortFunc(result, func(a, b T) int {
		scoreA := a.GetScore()
		scoreB := b.GetScore()
		if scoreA > scoreB {
			return -1
		} else if scoreA < scoreB {
			return 1
		}
		return 0
	})
	return result
}

// ParseLLMJsonResponse parses a JSON response from LLM, handling cases where JSON is wrapped in code blocks.
// This is useful when LLMs return responses like:
// ```json
// {"key": "value"}
// ```
// or regular JSON responses directly.
// jsonCodeFenceRE extracts a JSON payload wrapped in a Markdown code fence.
// Compiled once: ParseLLMJsonResponse runs on the graph-extraction path.
var jsonCodeFenceRE = regexp.MustCompile("```(?:json)?\\s*([\\s\\S]*?)```")

// 思考块标签：GLM-Z1/DeepSeek-R1/Qwen3 等思考模型会把推理过程包在标签里。
// 只剥离这些明确标签，不猜其它前缀，避免误吃正文。
var (
	// reasoningBlockRE 匹配**闭合**的思考块（跨行、大小写不敏感）。
	reasoningBlockRE = regexp.MustCompile(`(?is)<(?:think|thinking|thought|reasoning)>.*?</(?:think|thinking|thought|reasoning)>`)
	// openReasoningTagRE 定位**未闭合**的思考块开头（思考被 MaxTokens 截断）。
	openReasoningTagRE = regexp.MustCompile(`(?i)<(?:think|thinking|thought|reasoning)>`)
)

// StripReasoningBlocks 移除思考模型输出里的 <think>/<thought>/<reasoning> 块。
//
// 为什么必须做：思考模型常在正文之前输出草稿，草稿里往往也有**形状完整的 JSON**
// （例如 {"invoices":[{"invoice_no":"..."}]}）。若先进入 ExtractBalancedJSON，
// 括号扫描会抓到草稿——字段看起来"有值"但来自推理过程，且思考被截断时草稿
// 只写了一半，解析出来的是残缺结构。所以在**任何扫描之前**先剥离。
//
// 未闭合处理（思考被 MaxTokens 截断）：
//   - 若其后出现 JSON 起始符或代码块围栏 → 切掉标签到该位置之间的思考内容；
//   - 若全文都在思考块内（找不到任何 JSON 痕迹）→ 原样返回，让后续兜底报错，
//     而不是返回空串把错误信息也一起丢掉。
func StripReasoningBlocks(raw string) string {
	if raw == "" {
		return raw
	}
	s := reasoningBlockRE.ReplaceAllString(raw, "")
	// 闭合块移除后，仍残留未闭合的思考标签时按截断处理。
	if loc := openReasoningTagRE.FindStringIndex(s); loc != nil {
		rest := s[loc[1]:]
		cut := -1
		if js := firstJSONStartIndex(rest); js >= 0 {
			cut = js
		}
		if fence := strings.Index(rest, "```"); fence >= 0 && (cut < 0 || fence < cut) {
			cut = fence
		}
		if cut >= 0 {
			s = s[:loc[0]] + rest[cut:]
		}
	}
	return strings.TrimSpace(s)
}

// validJSONThousandNumber 规范的千分位数值（1,234 / -3,258.99）。
// 只有符合该形态的引号字符串才会被去逗号：随便一个带逗号的文本不动。
var validJSONThousandNumber = regexp.MustCompile(`^-?\d{1,3}(?:,\d{3})+(?:\.\d+)?$`)

// bareJSONNumber 去引号后的纯数字形态。
var bareJSONNumber = regexp.MustCompile(`^-?\d+(?:\.\d+)?$`)

// jsonStringLiteralRE 匹配 JSON 字符串字面量（正确处理转义）。
var jsonStringLiteralRE = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)

// jsonTrailingCommaRE 匹配 ] / } 前的多余逗号。
var jsonTrailingCommaRE = regexp.MustCompile(`,\s*([\]}])`)

// SanitizeLooseJSON 对模型输出的"语法上接近 JSON"的文本做**确定性**修复：
//
//  1. 移除尾随逗号：`{"a":1,}` → `{"a":1}`、`[1,2,]` → `[1,2]`。
//  2. 把引号包裹的金额转成裸数字：`"amount":"3,258.99"` → `"amount":3258.99`、
//     `"tax":"¥93.32"` → `"tax":93.32`。
//
// 转换有严格边界（避免把编码类字段改坏）：
//   - 只处理**值**位置，键不动；
//   - 必须含货币符号 / 千分位逗号 / 小数点之一，纯整数串一律不动
//     —— 20 位全电发票号、8 位纸票号、10~12 位发票代码、18 位税号都是纯整数串；
//   - 千分位必须符合 `\d{1,3}(,\d{3})+` 形态，否则视为普通文本；
//   - 整数位超过 15 位不转（超出 float64 精确整数范围，转了就是丢精度）。
func SanitizeLooseJSON(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	s = removeJSONTrailingCommas(s)
	return sanitizeQuotedNumbers(s)
}

// removeJSONTrailingCommas 只在**字符串字面量之外**移除尾随逗号。
// 直接对整串做正则替换会误伤值里恰好含 ",]" 的正常文本（如备注 "逗号,}"）。
func removeJSONTrailingCommas(s string) string {
	locs := jsonStringLiteralRE.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return jsonTrailingCommaRE.ReplaceAllString(s, "$1")
	}
	var b strings.Builder
	b.Grow(len(s))
	last := 0
	for _, loc := range locs {
		b.WriteString(jsonTrailingCommaRE.ReplaceAllString(s[last:loc[0]], "$1"))
		b.WriteString(s[loc[0]:loc[1]])
		last = loc[1]
	}
	b.WriteString(jsonTrailingCommaRE.ReplaceAllString(s[last:], "$1"))
	return b.String()
}

// sanitizeQuotedNumbers 逐个字符串字面量判断是否需要去引号转数字。
func sanitizeQuotedNumbers(s string) string {
	locs := jsonStringLiteralRE.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	last := 0
	for _, loc := range locs {
		b.WriteString(s[last:loc[0]])
		lit := s[loc[0]:loc[1]]
		if jsonKeyFollows(s, loc[1]) {
			b.WriteString(lit) // 是对象的键，保持原样
		} else {
			b.WriteString(looseNumericLiteral(lit))
		}
		last = loc[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

// jsonKeyFollows 判断 end 之后（跳过空白）第一个字符是否为 ':'。
func jsonKeyFollows(s string, end int) bool {
	for i := end; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\n', '\r':
			continue
		case ':':
			return true
		default:
			return false
		}
	}
	return false
}

// looseNumericLiteral 把 `"¥3,258.99"` 这类金额字面量转成裸数字，其它一律原样返回。
func looseNumericLiteral(lit string) string {
	if len(lit) < 2 {
		return lit
	}
	body := lit[1 : len(lit)-1]
	if body == "" || strings.ContainsRune(body, '\\') {
		return lit
	}
	trimmed := strings.TrimSpace(body)
	hadCurrency := false
	for _, sym := range []string{"¥", "￥", "$", "€", "£"} {
		if strings.Contains(trimmed, sym) {
			hadCurrency = true
			trimmed = strings.ReplaceAll(trimmed, sym, "")
		}
	}
	if strings.HasSuffix(trimmed, "元") {
		hadCurrency = true
		trimmed = strings.TrimSuffix(trimmed, "元")
	}
	trimmed = strings.TrimSpace(trimmed)

	hadComma := strings.Contains(trimmed, ",")
	if hadComma {
		if !validJSONThousandNumber.MatchString(trimmed) {
			return lit // 不是规范千分位（如 "重庆, 中国"），不动
		}
		trimmed = strings.ReplaceAll(trimmed, ",", "")
	}
	hadDecimal := strings.Contains(trimmed, ".")
	if !hadCurrency && !hadComma && !hadDecimal {
		return lit // 纯整数串：发票号/税号/编码，绝不能转
	}
	if !bareJSONNumber.MatchString(trimmed) {
		return lit
	}
	if integerDigitCount(trimmed) > 15 {
		return lit // 超长整数，转 float64 会丢精度
	}
	return trimmed
}

// integerDigitCount 返回数值整数部分的位数（忽略负号）。
func integerDigitCount(num string) int {
	num = strings.TrimPrefix(num, "-")
	if idx := strings.IndexByte(num, '.'); idx >= 0 {
		num = num[:idx]
	}
	return len(num)
}

// unmarshalLoose 先按标准 JSON 解析；失败时用 SanitizeLooseJSON 修一遍再试。
// 修复无效时返回**原始**错误（信息更贴近真实问题，便于诊断）。
func unmarshalLoose(data []byte, target interface{}) error {
	raw := string(data)
	err := json.Unmarshal(data, target)
	if err == nil {
		return nil
	}
	if cleaned := SanitizeLooseJSON(raw); cleaned != raw {
		if cleanErr := json.Unmarshal([]byte(cleaned), target); cleanErr == nil {
			return nil
		}
	}
	return err
}

func ParseLLMJsonResponse(content string, target interface{}) error {
	// 思考模型：先剥离 <think>/<thought> 块，否则括号扫描会抓到思考草稿。
	content = StripReasoningBlocks(content)

	// First, try to parse directly as JSON
	err := unmarshalLoose([]byte(content), target)
	if err == nil {
		return nil
	}

	// If direct parsing fails, try to extract JSON from code blocks
	matches := jsonCodeFenceRE.FindStringSubmatch(content)
	if len(matches) >= 2 {
		// Extract the JSON content within the code block
		jsonContent := strings.TrimSpace(matches[1])
		if fenceErr := unmarshalLoose([]byte(jsonContent), target); fenceErr == nil {
			return nil
		}
	}

	// Last resort: models often wrap the payload in prose ("Sure, here is
	// the JSON: {...}"). Scan for a balanced object/array so trailing
	// commentary — including bracket-like text such as "[1]" — cannot
	// truncate the payload.
	if extracted := ExtractBalancedJSON(content); extracted != "" {
		if scanErr := unmarshalLoose([]byte(extracted), target); scanErr == nil {
			return nil
		}
	}

	// Truncation-tolerant recovery: LLM replies cut off by MaxTokens can leave
	// (a) an unclosed markdown fence before an otherwise complete payload,
	// (b) a JSON object/array whose tail is missing. Strip a dangling fence
	// prefix first, then try to repair a truncated payload by dropping the
	// incomplete tail and re-closing open brackets.
	if cleaned := stripUnclosedFencePrefix(content); cleaned != content {
		if cleanErr := unmarshalLoose([]byte(cleaned), target); cleanErr == nil {
			return nil
		}
		if extracted := ExtractBalancedJSON(cleaned); extracted != "" {
			if scanErr := unmarshalLoose([]byte(extracted), target); scanErr == nil {
				return nil
			}
		}
	}
	if repaired := repairTruncatedJSON(content); repaired != "" {
		if repErr := unmarshalLoose([]byte(repaired), target); repErr == nil {
			return nil
		}
	}

	// Report the direct-parse failure, which is the most descriptive one.
	return err
}

// stripUnclosedFencePrefix removes a leading markdown code-fence line
// ("```json" / "```") when the closing fence is missing — a typical MaxTokens
// truncation artifact — so the payload behind it can still be parsed.
func stripUnclosedFencePrefix(s string) string {
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, "```") {
		return s
	}
	if idx := strings.IndexByte(t, '\n'); idx >= 0 {
		return t[idx+1:]
	}
	return ""
}

// firstJSONStartIndex returns the index of the first '{' or '[' in s, choosing
// whichever opens first; -1 when neither exists.
func firstJSONStartIndex(s string) int {
	obj := strings.IndexByte(s, '{')
	arr := strings.IndexByte(s, '[')
	switch {
	case obj < 0 && arr < 0:
		return -1
	case obj < 0:
		return arr
	case arr < 0:
		return obj
	case obj < arr:
		return obj
	default:
		return arr
	}
}

// repairTruncatedJSON attempts to salvage a JSON payload that was cut off
// mid-stream (e.g. by MaxTokens). It starts at the first '{' or '[' and walks
// backward from the end, dropping the incomplete tail — jumping straight past
// a dangling string literal when present — and re-closing any still-open
// brackets, until the prefix parses as valid JSON. Returns "" when nothing
// usable can be recovered.
func repairTruncatedJSON(content string) string {
	start := firstJSONStartIndex(content)
	if start < 0 || start >= len(content) {
		return ""
	}
	s := content[start:]
	if closed := tryCloseTruncatedJSON(s); closed != "" {
		return closed
	}
	// Truncated inside a string literal: jump back to its opening quote so we
	// don't drop the string one byte at a time.
	if q := trailingStringStart(s); q >= 0 {
		if closed := tryCloseTruncatedJSON(s[:q]); closed != "" {
			return closed
		}
	}
	// General bounded walk-back (worst case stays linear in the input length).
	const maxTries = 1024
	for i := 0; i < maxTries && len(s) > 1; i++ {
		s = s[:len(s)-1]
		if closed := tryCloseTruncatedJSON(s); closed != "" {
			return closed
		}
	}
	return ""
}

// trailingStringStart returns the index of the opening quote of the trailing
// (unclosed) string literal in s, or -1 when the tail is not inside a string.
func trailingStringStart(s string) int {
	escaped := false
	for i := len(s) - 1; i >= 0; i-- {
		c := s[i]
		switch {
		case c == '\\' && !escaped:
			escaped = true
			continue
		case c == '"' && !escaped:
			return i
		default:
			escaped = false
		}
	}
	return -1
}

// tryCloseTruncatedJSON repairs s (which must start with '{' or '[') by closing
// a dangling string literal and appending the minimum closing brackets needed to
// balance it, returning the repaired JSON when it parses as valid JSON;
// otherwise "". Returns "" when s is already balanced — the caller then drops
// trailing characters and retries.
func tryCloseTruncatedJSON(s string) string {
	depth := 0
	inString := false
	escaped := false
	closers := make([]byte, 0, 16)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
			closers = append(closers, '}')
		case '[':
			depth++
			closers = append(closers, ']')
		case '}':
			if depth > 0 {
				depth--
				closers = closers[:len(closers)-1]
			}
		case ']':
			if depth > 0 {
				depth--
				closers = closers[:len(closers)-1]
			}
		}
		if depth < 0 {
			return ""
		}
	}
	repaired := s
	if inString {
		repaired += `"`
	}
	if depth > 0 {
		// closers were appended in open order ({ then [), so close them in
		// reverse (innermost first).
		for i := len(closers) - 1; i >= 0; i-- {
			repaired += string(closers[i])
		}
	}
	if repaired == s {
		return ""
	}
	if json.Valid([]byte(repaired)) {
		return repaired
	}
	return ""
}

// ExtractBalancedJSON returns the first balanced JSON object or array embedded
// in s, or an empty string when there is none. Whichever bracket type opens
// first wins, and quoted strings are skipped so braces inside string literals
// do not unbalance the scan. The result is not validated as JSON; callers must
// still unmarshal it.
func ExtractBalancedJSON(s string) string {
	objStart := strings.IndexByte(s, '{')
	arrStart := strings.IndexByte(s, '[')
	var open, closeCh byte
	var start int
	switch {
	case objStart < 0 && arrStart < 0:
		return ""
	case objStart < 0:
		open, closeCh, start = '[', ']', arrStart
	case arrStart < 0:
		open, closeCh, start = '{', '}', objStart
	case objStart < arrStart:
		open, closeCh, start = '{', '}', objStart
	default:
		open, closeCh, start = '[', ']', arrStart
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case open:
			depth++
		case closeCh:
			depth--
			if depth == 0 {
				return strings.TrimSpace(s[start : i+1])
			}
		}
	}
	return ""
}

// CleanInvalidUTF8 移除字符串中的非法 UTF-8 字符和 \x00
func CleanInvalidUTF8(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			// 非法 UTF-8 字节，跳过
			i++
			continue
		}
		if r == 0 {
			// NULL 字符 \x00，跳过
			i += size
			continue
		}
		b.WriteRune(r)
		i += size
	}

	return b.String()
}

const (
	pipelineLogValueMaxRune = 300
	defaultPipelineStage    = "PIPELINE"
	defaultPipelineAction   = "info"
	pipelineLogPrefix       = "[PIPELINE]"
	pipelineTruncateEll     = "..."
)

// PipelineLog builds a structured pipeline log string.
func PipelineLog(stage, action string, fields map[string]interface{}) string {
	if stage == "" {
		stage = defaultPipelineStage
	}
	if action == "" {
		action = defaultPipelineAction
	}

	builder := strings.Builder{}
	builder.Grow(128)
	builder.WriteString(pipelineLogPrefix)
	builder.WriteString(" stage=")
	builder.WriteString(stage)
	builder.WriteString(" action=")
	builder.WriteString(action)

	if len(fields) > 0 {
		keys := make([]string, 0, len(fields))
		for k := range fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			builder.WriteString(" ")
			builder.WriteString(key)
			builder.WriteString("=")
			builder.WriteString(secutils.SanitizeForLog(formatPipelineLogValue(fields[key])))
		}
	}
	return builder.String()
}

// PipelineInfo logs pipeline info level entries.
func PipelineInfo(ctx context.Context, stage, action string, fields map[string]interface{}) {
	logger.GetLogger(ctx).Info(PipelineLog(stage, action, fields))
}

// PipelineWarn logs pipeline warning level entries.
func PipelineWarn(ctx context.Context, stage, action string, fields map[string]interface{}) {
	logger.GetLogger(ctx).Warn(PipelineLog(stage, action, fields))
}

// PipelineError logs pipeline error level entries.
func PipelineError(ctx context.Context, stage, action string, fields map[string]interface{}) {
	logger.GetLogger(ctx).Error(PipelineLog(stage, action, fields))
}

func formatPipelineLogValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strconv.Quote(truncatePipelineValue(v))
	case fmt.Stringer:
		return strconv.Quote(truncatePipelineValue(v.String()))
	case json.RawMessage:
		bytes, _ := v.MarshalJSON()
		return string(bytes)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func truncatePipelineValue(content string) string {
	content = strings.ReplaceAll(content, "\n", "\\n")
	runes := []rune(content)
	if len(runes) <= pipelineLogValueMaxRune {
		return content
	}
	return string(runes[:pipelineLogValueMaxRune]) + pipelineTruncateEll
}

func TruncateForLog(content string) string {
	return truncatePipelineValue(content)
}
