package service

import (
	"reflect"
	"regexp"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// 本文件是蓝图 P2-4 的落地：Schema 驱动的**全字段后置正则补洞**。
//
// 与旧发票实现（extract_regex_fallback.go 的 patchStandardInvoiceFields /
// patchConfiguredInvoiceFields / patchInvoiceItemSubFields）逐项等价的通用版：
// 旧实现绑死 InvoiceExtractionItem 结构体，本实现只认 ScopeSchema 声明 +
// GenericExtractRecord 视图，任何注册了 Schema 的 scope 都能获得同一套补洞能力。
//
// 等价性由 extract_schema_equivalence_test.go 的双跑测试锁死（含放弃路径）。

// SchemaRegexPatch 对一组记录执行 Schema 驱动的三阶段补洞，返回补洞明细。
//
// 阶段顺序与旧实现一致（标准 → 自定义 → 明细子项），同阶段内按 Schema 声明序：
//
//	map 迭代无序会让同一份输入产出不同顺序的补洞记录，日志与 PatchedByRegex
//	必须可复现。
func SchemaRegexPatch(
	records []GenericExtractRecord,
	text string,
	cfg *types.KbExtractConfig,
	schema types.ScopeSchema,
) []RegexFieldPatch {
	if len(records) == 0 || strings.TrimSpace(text) == "" {
		return nil
	}
	patches := make([]RegexFieldPatch, 0, 4)
	patches = append(patches, schemaPatchStandardFields(records, text, cfg, schema)...)
	patches = append(patches, schemaPatchConfiguredFields(records, text, cfg, schema)...)
	patches = append(patches, schemaPatchItemSubFields(records, text, cfg, schema)...)
	if len(patches) == 0 {
		return nil
	}
	return patches
}

// ---- 阶段 1：标准契约字段 ----

func schemaPatchStandardFields(
	records []GenericExtractRecord,
	text string,
	cfg *types.KbExtractConfig,
	schema types.ScopeSchema,
) []RegexFieldPatch {
	var patches []RegexFieldPatch
	for _, fd := range schema.StandardFields {
		// 只有**恰好一条**记录缺该字段时才补：多记录同缺时无法判断匹配值属于谁。
		missingIdx := -1
		missingCount := 0
		for i := range records {
			if schemaFieldText(&records[i], fd) == "" {
				missingIdx = i
				missingCount++
			}
		}
		if missingCount != 1 {
			continue
		}
		for _, pattern := range schemaFieldRegexPatterns(fd, cfg) {
			re, err := regexp.Compile(pattern.re)
			if err != nil {
				continue // 用户配置的正则非法：跳过，不影响其它字段
			}
			value, ok := uniqueRegexCapture(text, re)
			if !ok {
				continue
			}
			value = schemaNormalizePatchedValue(fd, value)
			if value == "" {
				continue
			}
			// 该值已属于同批另一条记录 → 放弃（防重复/错位）。
			if schemaValueUsedElsewhere(records, fd, missingIdx, value) {
				continue
			}
			if !schemaSetFieldValue(&records[missingIdx], fd, value) {
				continue // 数值转换失败：宁可空着
			}
			patches = append(patches, RegexFieldPatch{
				Index: missingIdx, Field: fd.Key, Value: value, Source: pattern.source,
			})
			break // 一个字段补一次即止
		}
	}
	return patches
}

// schemaFieldText 读标准字段的当前文本值（空串 = 缺失）。
// 数值字段 nil → ""，0 → "0"（显式 0 视为已填，与旧读写器口径一致）。
func schemaFieldText(rec *GenericExtractRecord, fd types.FieldDef) string {
	v, _ := rec.textValue(fd.Key)
	return strings.TrimSpace(v)
}

// schemaSetFieldValue 按 Schema 类型写回：number 走数值清洗，其余走字符串。
func schemaSetFieldValue(rec *GenericExtractRecord, fd types.FieldDef, value string) bool {
	if fd.Type == types.FieldTypeNumber {
		return rec.setNumber(fd.Key, value)
	}
	return rec.setString(fd.Key, value)
}

// schemaFieldRegexPatterns 候选正则：用户配置优先（canonical 匹配），其次 Schema 内置。
func schemaFieldRegexPatterns(fd types.FieldDef, cfg *types.KbExtractConfig) []fieldRegexPattern {
	var out []fieldRegexPattern
	if cfg != nil {
		want := canonicalFieldKey(fd.Key)
		for _, f := range cfg.Fields {
			if !f.Enabled || canonicalFieldKey(strings.TrimSpace(f.Name)) != want {
				continue
			}
			if re := strings.TrimSpace(f.Regex); re != "" {
				out = append(out, fieldRegexPattern{re: re, source: regexPatchSourceConfig})
			}
		}
	}
	for _, re := range builtinInvoiceFieldRegexes[fd.Key] {
		out = append(out, fieldRegexPattern{re: re, source: regexPatchSourceBuiltin})
	}
	return out
}

// schemaNormalizePatchedValue 按 Schema 类型归一：date 统一 YYYY-MM-DD（失败即
// 放弃补洞），其余去空白。数值的货币符/千分位清洗在 setNumber 里完成。
func schemaNormalizePatchedValue(fd types.FieldDef, value string) string {
	value = strings.TrimSpace(value)
	if fd.Type == types.FieldTypeDate {
		if normalized, ok := normalizeDateString(value); ok {
			return normalized
		}
		return ""
	}
	return value
}

// schemaValueUsedElsewhere 判断该值是否已属于同批其它记录的同一字段。
func schemaValueUsedElsewhere(records []GenericExtractRecord, fd types.FieldDef, skipIdx int, value string) bool {
	for i := range records {
		if i == skipIdx {
			continue
		}
		if schemaFieldText(&records[i], fd) == value {
			return true
		}
	}
	return false
}

// ---- 阶段 2：配置自定义字段 → DynamicBucket ----
//
// 标准字段在此跳过——它们已由阶段 1 按契约键处理，重复执行只会让同一次提取
// 出现两条补洞记录。

func schemaPatchConfiguredFields(
	records []GenericExtractRecord,
	text string,
	cfg *types.KbExtractConfig,
	schema types.ScopeSchema,
) []RegexFieldPatch {
	if cfg == nil || schema.DynamicBucket == "" {
		return nil
	}
	buckets := make([]map[string]any, len(records))
	for i := range records {
		// 纯读取：没有扩展桶的记录返回 nil，绝不因此建出空 map（未补洞的
		// 结构体必须保持原样，oldFields=nil 与 map{} 在落库 JSON 上不等价）。
		b, _ := records[i].dynamicBucket(schema.DynamicBucket, false)
		buckets[i] = b
	}
	var patches []RegexFieldPatch
	seen := make(map[string]bool, len(cfg.Fields))
	for _, f := range cfg.Fields {
		name := strings.TrimSpace(f.Name)
		if name == "" || !f.Enabled || seen[name] {
			continue
		}
		seen[name] = true
		if schemaIsStandardField(schema, name) {
			continue
		}
		pattern := strings.TrimSpace(f.Regex)
		if pattern == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		missingIdx := -1
		missingCount := 0
		for i := range records {
			if dynamicBucketText(buckets[i], name) == "" {
				missingIdx = i
				missingCount++
			}
		}
		if missingCount != 1 {
			continue
		}
		value, ok := uniqueRegexCapture(text, re)
		if !ok || value == "" {
			continue
		}
		if dynamicBucketValueUsedElsewhere(buckets, name, missingIdx, value) {
			continue
		}
		if buckets[missingIdx] == nil {
			// 写入前才惰性建桶（读取阶段不建，见上）。
			b, ok := records[missingIdx].dynamicBucket(schema.DynamicBucket, true)
			if !ok || b == nil {
				continue // 该 scope 无扩展桶且无法新建：放弃而非 panic
			}
			buckets[missingIdx] = b
		}
		buckets[missingIdx][name] = value
		patches = append(patches, RegexFieldPatch{
			Index: missingIdx, Field: name, Value: value, Source: regexPatchSourceConfig,
		})
	}
	return patches
}

// schemaIsStandardField 判断配置键是否命中标准字段（canonical 归一，覆盖 camelCase 配置名）。
func schemaIsStandardField(schema types.ScopeSchema, name string) bool {
	want := canonicalFieldKey(strings.TrimSpace(name))
	if want == "" {
		return false
	}
	for _, fd := range schema.StandardFields {
		if canonicalFieldKey(fd.Key) == want {
			return true
		}
	}
	return false
}

func dynamicBucketText(bucket map[string]any, name string) string {
	if bucket == nil {
		return ""
	}
	v, ok := bucket[name]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(toStringValue(v))
}

func dynamicBucketValueUsedElsewhere(buckets []map[string]any, name string, skipIdx int, value string) bool {
	for i, b := range buckets {
		if i == skipIdx {
			continue
		}
		if dynamicBucketText(b, name) == value {
			return true
		}
	}
	return false
}

// ---- 阶段 3：明细子项 → ItemBucket 行动态桶 ----
//
// 归属铁律：只有「本批恰好一条记录 + 该记录恰好一行明细」时才回填——多行明细里
// 同一个正则会命中多个值，而每一行的归属无法从文本确定，按序塞就是串号。

func schemaPatchItemSubFields(
	records []GenericExtractRecord,
	text string,
	cfg *types.KbExtractConfig,
	schema types.ScopeSchema,
) []RegexFieldPatch {
	if cfg == nil || schema.ItemBucket == "" || len(records) != 1 {
		return nil
	}
	lines := records[0].itemLines(schema.ItemBucket, schema.ItemField)
	if len(lines) != 1 {
		return nil
	}
	line := &lines[0]
	var patches []RegexFieldPatch
	seen := map[string]bool{}
	for _, f := range cfg.Fields {
		if f.Type != "items" || !f.Enabled {
			continue
		}
		for _, sf := range f.SubFields {
			key := strings.TrimSpace(sf.Key)
			pattern := strings.TrimSpace(sf.Regex)
			if key == "" || pattern == "" || !sf.IsEnabled() || seen[key] {
				continue
			}
			seen[key] = true
			if strings.TrimSpace(lineText(line, key)) != "" {
				continue
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				continue
			}
			value, ok := uniqueRegexCapture(text, re)
			if !ok || value == "" {
				continue
			}
			if !setLineValue(line, key, value) {
				continue
			}
			patches = append(patches, RegexFieldPatch{
				Index: 0, Field: "items." + key, Value: value, Source: regexPatchSourceConfig,
			})
		}
	}
	return patches
}

// ---- ItemLineRecord：明细行视图（与 GenericExtractRecord 同构，浅一层） ----

func newItemLineRecord(index int, elem reflect.Value, itemGoBucket string) ItemLineRecord {
	rec := ItemLineRecord{Index: index, fields: map[string]reflect.Value{}, goNames: map[string]string{}}
	t := elem.Type()
	for i := 0; i < t.NumField(); i++ {
		name := jsonFieldName(t.Field(i))
		if name == "" {
			continue
		}
		rec.fields[name] = elem.Field(i)
		rec.goNames[name] = t.Field(i).Name
	}
	// 动态桶按 **Go 字段名**在结构体上直接找：明细行的扩展桶（发票 Extra）的
	// json tag 是 "-"（与标准键合并输出的自定义编解码），走 json 名永远收集不到。
	if itemGoBucket != "" {
		for i := 0; i < t.NumField(); i++ {
			if t.Field(i).Name != itemGoBucket {
				continue
			}
			fv := elem.Field(i)
			if fv.Kind() != reflect.Map || fv.Type().Key().Kind() != reflect.String {
				break
			}
			rec.hasBucket = true
			rec.bucketField = fv
			if !fv.IsNil() {
				rec.dynamic, _ = fv.Interface().(map[string]any)
			}
			break
		}
	}
	return rec
}

// ensureDynamicBucket 惰性建桶：只有真要写入动态键时才建 map。
func (l *ItemLineRecord) ensureDynamicBucket() bool {
	if l.dynamic != nil {
		return true
	}
	if !l.hasBucket || !l.bucketField.CanSet() {
		return false
	}
	l.bucketField.Set(reflect.MakeMap(l.bucketField.Type()))
	l.dynamic, _ = l.bucketField.Interface().(map[string]any)
	return l.dynamic != nil
}

// lineText 读明细行某子列的当前值：标准列走反射（string/*float64），
// 其余走动态桶（发票 Extra）。
func lineText(line *ItemLineRecord, key string) string {
	if v, ok := line.resolveText(key); ok {
		return v
	}
	if line.dynamic != nil {
		return dynamicBucketText(line.dynamic, key)
	}
	return ""
}

func (l *ItemLineRecord) resolveText(key string) (string, bool) {
	if v, ok := l.fields[key]; ok {
		switch {
		case v.Kind() == reflect.String:
			return v.String(), true
		case v.Kind() == reflect.Ptr && v.Type().Elem().Kind() == reflect.Float64:
			if v.IsNil() {
				return "", true
			}
			return floatFieldText(v.Interface().(*float64)), true
		}
	}
	// camelCase 配置键兜底（与 GenericExtractRecord.resolveField 同思路）
	want := canonicalFieldKey(key)
	for name, v := range l.fields {
		if canonicalFieldKey(name) != want {
			continue
		}
		switch {
		case v.Kind() == reflect.String:
			return v.String(), true
		case v.Kind() == reflect.Ptr && v.Type().Elem().Kind() == reflect.Float64:
			if v.IsNil() {
				return "", true
			}
			return floatFieldText(v.Interface().(*float64)), true
		}
	}
	return "", false
}

// setLineValue 写明细行子列：标准列按类型反射写入，动态键进 Extra 桶。
// 数值键转不成数值返回 false（与旧 setInvoiceItemSubFieldValue 一致）。
func setLineValue(line *ItemLineRecord, key, value string) bool {
	if v, ok := line.fields[key]; ok && v.CanSet() {
		switch v.Kind() {
		case reflect.String:
			v.SetString(value)
			return true
		case reflect.Ptr:
			if v.Type().Elem().Kind() == reflect.Float64 {
				f, ok := parsePatchedNumber(value)
				if !ok {
					return false
				}
				v.Set(reflect.ValueOf(&f))
				return true
			}
		}
	}
	// camelCase 配置键兜底：canonical 命中标准列时写标准列，否则视为动态键。
	want := canonicalFieldKey(key)
	for name, v := range line.fields {
		if canonicalFieldKey(name) != want || !v.CanSet() {
			continue
		}
		switch v.Kind() {
		case reflect.String:
			v.SetString(value)
			return true
		case reflect.Ptr:
			if v.Type().Elem().Kind() == reflect.Float64 {
				f, ok := parsePatchedNumber(value)
				if !ok {
					return false
				}
				v.Set(reflect.ValueOf(&f))
				return true
			}
		}
	}
	if !line.ensureDynamicBucket() {
		return false
	}
	line.dynamic[key] = value
	return true
}
