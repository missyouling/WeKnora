package service

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// 本文件是提取中台的**通用（非发票）质量闭环**：把 Phase B 里只给发票用的
// 确定性校验（日期归一化 / 必填判定 / 配置级正则补洞）扩面到合同、制度、
// 通知公告、电费账单、光伏账单等全部业务。
//
// 为什么不是照抄发票那一套：
//   - 发票的结果是「一个数组里 N 张票」，天然需要"多张都缺同一字段就放弃"这类
//     防串号边界；其它业务绝大多数是「一份文件一条记录」。
//   - 发票有专属业务规则（号码/代码二选一、明细合计与价税合计交叉校验），
//     这些规则对合同/制度毫无意义，所以留在 validateInvoiceExtraction 里。
//
// 因此这里只做两件**与业务无关**的事：
//  1. 日期字段就地归一化为 YYYY-MM-DD（排序/筛选/按期统计的前提）；
//  2. 用户配置为必填（Enabled && Required）且仍为空的字段，先用该字段配置的
//     Regex 在原文里补一次，再判 error。
//
// 结构体字段访问用反射 + json tag 映射：各业务的提取结果是**强类型结构体**
// （可读性、编译期校验都靠它），不可能改成 map[string]any。反射只做一层
// 「json key → 字段」映射，写入仅限 string 字段（日期都是 string）。

// GenericExtractRecord 一条提取记录的统一「按 json 字段名读写」视图。
type GenericExtractRecord struct {
	// Index 记录在结果数组中的下标（问题清单定位用；与发票的 index 语义一致）
	Index int
	// fields json 字段名 → 可寻址字段值（只收顶层字段）
	fields map[string]reflect.Value
	// mapFields 非 nil 时代表"扁平 map"视图（提取测试沙箱）：读写直接落到 map 上。
	// 与 fields 互斥，mapFields 优先。
	mapFields map[string]any
}

// canonicalFieldKey 归一化字段 key：小写 + 去掉非字母数字。
//
// 为什么需要它：前端配置里用户看到的是 camelCase（invoiceNo / signDate / partyATaxNo），
// 后端结构体的 json tag 是 snake_case（invoice_no / sign_date / party_a_tax_no）。
// 不做归一的话，界面上打开的"必填/正则兜底"永远匹配不到字段，功能等于没接。
// 归一只用于**查找**，日志与问题清单里仍保留用户配置的原始键名，便于对账。
func canonicalFieldKey(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// NewGenericRecordFromMap 用扁平 map[string]any 构造单条记录视图（提取测试接口用）。
// map 是引用语义：setString 会直接改写调用方的 map，补洞值可直接回给前端。
func NewGenericRecordFromMap(index int, m map[string]any) GenericExtractRecord {
	if m == nil {
		m = map[string]any{}
	}
	return GenericExtractRecord{Index: index, mapFields: m}
}

// isMapView 报告该记录是否为"扁平 map"视图（提取测试沙箱）。
// 与强类型结构体视图的关键差异：map 里键缺失就是模型没提取到（等同空值，
// 必须参与必填判定，handler 的 missing 列表同样是"缺键即缺失"口径）；
// 而结构体视图字段缺失只代表用户配了结构体装不下的自定义 key（跳过，避免误报）。
func (r GenericExtractRecord) isMapView() bool {
	return r.mapFields != nil
}

// resolveField 把（可能是 camelCase 的）字段名解析成记录里真实存在的键。
// 先精确命中，再按 canonical 归一命中；都不中返回 ok=false。
func (r GenericExtractRecord) resolveField(field string) (string, bool) {
	if r.mapFields != nil {
		if _, ok := r.mapFields[field]; ok {
			return field, true
		}
		want := canonicalFieldKey(field)
		for k := range r.mapFields {
			if canonicalFieldKey(k) == want {
				return k, true
			}
		}
		return "", false
	}
	if _, ok := r.fields[field]; ok {
		return field, true
	}
	want := canonicalFieldKey(field)
	for k := range r.fields {
		if canonicalFieldKey(k) == want {
			return k, true
		}
	}
	return "", false
}

// NewGenericRecords 把强类型结果切片适配成统一视图。
// items 必须是结构体切片；非结构体元素会被跳过（调用方无需关心，校验会静默略过）。
func NewGenericRecords[T any](items []T) []GenericExtractRecord {
	out := make([]GenericExtractRecord, 0, len(items))
	for i := range items {
		out = append(out, newGenericRecord(i, &items[i]))
	}
	return out
}

func newGenericRecord(index int, ptr any) GenericExtractRecord {
	rec := GenericExtractRecord{Index: index, fields: map[string]reflect.Value{}}
	if ptr == nil {
		return rec
	}
	rv := reflect.ValueOf(ptr)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return rec
	}
	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return rec
	}
	t := elem.Type()
	for i := 0; i < t.NumField(); i++ {
		name := jsonFieldName(t.Field(i))
		if name == "" {
			continue
		}
		rec.fields[name] = elem.Field(i)
	}
	return rec
}

// jsonFieldName 取结构体字段的 json 名（去掉 ,omitempty 等选项）；无可用 tag 时返回空。
func jsonFieldName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "" || tag == "-" {
		return ""
	}
	if i := strings.Index(tag, ","); i >= 0 {
		tag = tag[:i]
	}
	return strings.TrimSpace(tag)
}

// hasField 报告该记录是否真的持有这个 json 字段。
// 没有的字段（用户配置的自定义 key、明细子列等）一律跳过——强类型结果装不下它，
// 强行判"缺失"只会误报。
func (r GenericExtractRecord) hasField(field string) bool {
	_, ok := r.resolveField(field)
	return ok
}

// isEmpty 报告字段是否为空。第二个返回值为 false 表示字段不存在。
// 判定口径：string 空串、指针 nil、数值 0、切片/映射长度 0 视为空。
func (r GenericExtractRecord) isEmpty(field string) (bool, bool) {
	if r.mapFields != nil {
		key, ok := r.resolveField(field)
		if !ok {
			return false, false
		}
		return isZeroAny(r.mapFields[key]), true
	}
	key, ok := r.resolveField(field)
	if !ok {
		return false, false
	}
	return isZeroValue(r.fields[key]), true
}

// isZeroAny map 视图的空值判定：与强类型口径保持一致（nil / 空串 / 0 / 空集合）。
func isZeroAny(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	case float64:
		return t == 0
	case int:
		return t == 0
	case int64:
		return t == 0
	case bool:
		return false
	default:
		return false
	}
}

// stringValue 读取 string（或 *string）字段的值；非字符串字段返回 ok=false。
func (r GenericExtractRecord) stringValue(field string) (string, bool) {
	if r.mapFields != nil {
		key, ok := r.resolveField(field)
		if !ok {
			return "", false
		}
		s, isStr := r.mapFields[key].(string)
		return s, isStr
	}
	key, ok := r.resolveField(field)
	if !ok {
		return "", false
	}
	v := r.fields[key]
	if v.Kind() == reflect.Ptr {
		if v.IsNil() || v.Type().Elem().Kind() != reflect.String {
			return "", false
		}
		return v.Elem().String(), true
	}
	if v.Kind() != reflect.String {
		return "", false
	}
	return v.String(), true
}

// setString 写入 string 字段（非 string / 不可写时返回 false）。
func (r GenericExtractRecord) setString(field, value string) bool {
	if r.mapFields != nil {
		// map 视图：与强类型结构体"字段一定存在、只是零值"同口径——模型整键漏提取时，
		// 正则从原文恢复的值也要能建键回填（resolveField 命中已有键则原样写）。
		key, ok := r.resolveField(field)
		if !ok {
			key = field
		}
		r.mapFields[key] = value
		return true
	}
	key, ok := r.resolveField(field)
	if !ok {
		return false
	}
	v := r.fields[key]
	if v.Kind() != reflect.String || !v.CanSet() {
		return false
	}
	v.SetString(value)
	return true
}

func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return strings.TrimSpace(v.String()) == ""
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	case reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Bool:
		// 布尔字段（如发票 void_flag）不参与"必填"语义，一律视为非空，避免误报。
		return false
	default:
		return false
	}
}

// standardDateFields 各业务真实存在的日期字段（JSON key）。
//
// 为什么内置一份：用户配置里往往只声明 Type=string，不会特地把日期标成 date；
// 但这些字段一旦不是 YYYY-MM-DD，列表排序、按期间筛选、环比统计全都会错。
// 只在这些字段**整串**就是一个日期时才改写，避免把"2026-08-01至2026-08-31"
// 这类区间值截成半截。
var standardDateFields = map[string]struct{}{
	// 合同
	"sign_date": {}, "effective_date": {}, "expiry_date": {},
	// 制度 / 通知公告
	"issue_date": {},
	// 电费 / 光伏账单
	"bill_period_start": {}, "bill_period_end": {},
	"print_date": {}, "due_date": {},
}

// 日期整串匹配：2026-09-02 / 2026/9/2 / 2026.09.02 / 2026年9月2日 / 20260902。
// 与 validateInvoiceExtraction 里的宽松版区别是**必须整串命中**：通用校验要作用于
// 所有业务的日期字段，宽松匹配会把区间/时间戳截成半截日期。
var (
	reDateCNStrict  = regexp.MustCompile(`^(\d{4})\s*年\s*(\d{1,2})\s*月\s*(\d{1,2})\s*日?$`)
	reDateSepStrict = regexp.MustCompile(`^(\d{4})\s*[-/.]\s*(\d{1,2})\s*[-/.]\s*(\d{1,2})\s*日?$`)
)

// NormalizeDateValue 把"整串就是一个日期"的值归一为 YYYY-MM-DD；否则返回 false。
func NormalizeDateValue(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	var y, m, d string
	switch {
	case reDateCNStrict.MatchString(s):
		g := reDateCNStrict.FindStringSubmatch(s)
		y, m, d = g[1], g[2], g[3]
	case reDateSepStrict.MatchString(s):
		g := reDateSepStrict.FindStringSubmatch(s)
		y, m, d = g[1], g[2], g[3]
	case reDateCompact.MatchString(s):
		g := reDateCompact.FindStringSubmatch(s)
		y, m, d = g[1], g[2], g[3]
	default:
		return "", false
	}
	mi, di := atoi2(m), atoi2(d)
	if mi < 1 || mi > 12 || di < 1 || di > 31 {
		return "", false
	}
	return fmt.Sprintf("%s-%02d-%02d", y, mi, di), true
}

// reISO8601Date 已经是目标形态（含零填充）时不再改写。
var reISO8601Date = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// ValidateGenericExtraction 对非发票业务的提取结果做通用校验，返回
// （问题清单, 正则补洞明细）。
//
// text 必须是**本次提取所依据的原文**（单份文档/单批），因为正则补洞只在
// 唯一明确命中时回填。
//
// 安全边界（与发票补洞同口径）：
//   - 只有**一条记录**时才允许正则补洞。多条记录同时缺同一字段时，匹配值到底属于
//     谁无从判断，按序猜测会把 A 合同的信息写到 B 合同上；
//   - 只补空字段，绝不覆盖模型已给出的值；
//   - 用户未配置 Regex 的字段不做补洞（不做业务猜值）。
func ValidateGenericExtraction(
	records []GenericExtractRecord,
	text string,
	cfg *types.KbExtractConfig,
) ([]ExtractFieldIssue, []RegexFieldPatch) {
	if len(records) == 0 {
		return nil, nil
	}
	issues := make([]ExtractFieldIssue, 0)
	patches := make([]RegexFieldPatch, 0)
	allowRegexPatch := len(records) == 1

	for _, rec := range records {
		// 1) 日期归一化（用户声明 Type=date 的字段 + 内置标准日期字段）
		for _, field := range genericDateFieldOrder(rec, cfg) {
			value, ok := rec.stringValue(field)
			if !ok || strings.TrimSpace(value) == "" || reISO8601Date.MatchString(strings.TrimSpace(value)) {
				continue
			}
			if norm, ok := NormalizeDateValue(value); ok {
				rec.setString(field, norm)
				continue
			}
			// 归一失败只对**用户显式声明为 date** 的字段告警：内置日期字段可能装了
			// 区间（"2026-08-01至2026-08-31"）或"自发布之日起生效"之类的原文，报错纯噪音。
			if genericDateFieldConfigured(cfg, field) {
				issues = append(issues, ExtractFieldIssue{
					Index: rec.Index, Field: field, Kind: "format",
					Detail:   fmt.Sprintf("日期无法归一为 YYYY-MM-DD: %q", value),
					Severity: issueSeverityWarn,
				})
			}
		}

		// 2) 必填（Enabled && Required）：先正则补洞，补不到才报 error
		if cfg == nil {
			continue
		}
		for _, f := range cfg.Fields {
			if !f.Enabled || !f.Required {
				continue
			}
			name := strings.TrimSpace(f.Name)
			if name == "" {
				continue
			}
			// 强类型结果里没有这个 json 字段（用户配了自定义 key）→ 跳过避免误报；
			// 扁平 map 沙箱里键缺失就是模型没提取到 → 视同空值参与判定。
			if !rec.isMapView() && !rec.hasField(name) {
				continue
			}
			// isEmpty 第二返回值为 false 表示"键不存在"：map 视图缺键按空处理，
			// 先试正则补洞（允许建键回填），补不到才报 error。
			if empty, ok := rec.isEmpty(name); ok && !empty {
				continue
			}
			if patched := genericRegexPatch(rec, name, f.Regex, text, allowRegexPatch, cfg); patched != nil {
				patches = append(patches, *patched)
				continue
			}
			issues = append(issues, ExtractFieldIssue{
				Index: rec.Index, Field: name, Kind: "missing",
				Detail: "用户配置为必填，但未提取到该字段", Severity: issueSeverityError,
			})
		}
	}
	if len(issues) == 0 {
		issues = nil
	}
	if len(patches) == 0 {
		patches = nil
	}
	return issues, patches
}

// genericRegexPatch 用配置正则从原文补一个空字段；成功返回补洞明细。
func genericRegexPatch(
	rec GenericExtractRecord,
	field string,
	pattern string,
	text string,
	allowed bool,
	cfg *types.KbExtractConfig,
) *RegexFieldPatch {
	pattern = strings.TrimSpace(pattern)
	if !allowed || pattern == "" || strings.TrimSpace(text) == "" {
		return nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil // 用户填了非法正则：跳过，不影响其它字段
	}
	value, ok := uniqueRegexCapture(text, re)
	if !ok {
		return nil
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	// 日期型字段补洞后同样要归一，否则补进来的还是"2026年7月2日"。
	// 除了内置标准日期键，用户显式声明 Type=date 的字段（可能写成 signDate）也要归一。
	if isDateFieldName(field) || genericDateFieldConfigured(cfg, field) {
		norm, ok := NormalizeDateValue(value)
		if !ok {
			return nil
		}
		value = norm
	}
	if !rec.setString(field, value) {
		return nil
	}
	return &RegexFieldPatch{Index: rec.Index, Field: field, Value: value, Source: regexPatchSourceConfig}
}

// genericDateFieldOrder 返回本次要检查的日期字段（顺序稳定，便于日志复现）：
// 先按配置声明顺序，再补内置标准字段（字典序）。
func genericDateFieldOrder(rec GenericExtractRecord, cfg *types.KbExtractConfig) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, 8)
	if cfg != nil {
		for _, f := range cfg.Fields {
			if !f.Enabled || !strings.EqualFold(strings.TrimSpace(f.Type), "date") {
				continue
			}
			name := strings.TrimSpace(f.Name)
			if name == "" || !rec.hasField(name) {
				continue
			}
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	builtin := make([]string, 0, len(standardDateFields))
	for name := range standardDateFields {
		if _, dup := seen[name]; dup || !rec.hasField(name) {
			continue
		}
		builtin = append(builtin, name)
	}
	sort.Strings(builtin)
	return append(out, builtin...)
}

// genericDateFieldConfigured 判断该字段是否被用户显式声明为 date。
func genericDateFieldConfigured(cfg *types.KbExtractConfig, field string) bool {
	if cfg == nil {
		return false
	}
	for _, f := range cfg.Fields {
		if !f.Enabled || strings.TrimSpace(f.Name) != field {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(f.Type), "date") {
			return true
		}
	}
	return false
}

// isDateFieldName 判断字段是否按日期处理（内置标准日期字段）。
func isDateFieldName(field string) bool {
	_, ok := standardDateFields[field]
	return ok
}

// PatchedFieldNames 从补洞明细提取字段名（去重、保序），供 custom_metadata
// 的 patched_by_regex 使用。
func PatchedFieldNames(patches []RegexFieldPatch) []string {
	if len(patches) == 0 {
		return nil
	}
	out := make([]string, 0, len(patches))
	for _, p := range patches {
		if !stringInSlice(out, p.Field) {
			out = append(out, p.Field)
		}
	}
	return out
}
