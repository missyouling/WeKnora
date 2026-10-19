package service

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// 本文件是提取中台的**后置字段级正则补洞**：LLM（含自愈）跑完之后，对仍然为空的
// 字段，用确定性正则在**本批原文**里回填。
//
// 为什么需要它：发票的三套规则提取器是**前置抢跑**（命中就不调模型），一旦落入 LLM
// 路径，模型漏掉单个字段（如 invoice_no 输出了空串）就没有任何兜底——校验器只报
// error，字段照样空着入库。这里补上确定性的一层。
//
// 安全边界（比召回更重要）：
//   - 作用域限定"单批原文"，绝不跨批匹配（跨批必然串号）；
//   - 同批多张票、且多张都缺同一字段时**直接放弃**——按序猜测会把 A 票的号码写到
//     B 票上（历史上踩过"张冠李戴"）；
//   - 匹配出的值若已属于同批另一张票，同样放弃；
//   - 只碰空字段，绝不覆盖模型已给出的值。
//
// 2026-10-19 重构（蓝图 P2-4）：补洞执行体已 **Schema 驱动化**
// （extract_schema_patch.go 的 SchemaRegexPatch，三阶段：标准字段 → 自定义
// 扩展字段 → 明细子项），本文件只剩发票入口的薄封装与共享工具函数。
// 等价性由 extract_schema_equivalence_test.go 的双跑测试锁死后，旧的发票
// 结构体专用实现（约 400 行）已删除；新 scope 注册 Schema 即获得同一套补洞。

// RegexFieldPatch 一次补洞记录，用于日志与前端"该字段由正则补齐"的溯源展示。
type RegexFieldPatch struct {
	Index  int    `json:"index"`  // 发票在结果数组中的下标
	Field  string `json:"field"`  // 字段 key，如 invoice_no；明细子项为 items.<key>
	Value  string `json:"value"`  // 回填值
	Source string `json:"source"` // config（用户配置的正则） | builtin（内置高置信度正则）
}

const (
	regexPatchSourceConfig  = "config"
	regexPatchSourceBuiltin = "builtin"
)

// builtinInvoiceFieldRegexes 内置兜底正则，按「标签优先、形状其次」排序。
//
// 关键的取舍：绝不用裸 `\b\d{8}\b` 去猜 8 位发票号——真实票据里 8 位数字串有 136 个
// （日期 20251024、税号片段 91500227、电话 41557211……），猜出来的号码比空着更有害
// （用户会当成真数据用）。所以 8 位只接受「发票号码」标签后的值；裸形状只认 20 位
// 全电票号（形状足够唯一）。
//
// 金额类字段**故意不给内置正则**：一张票上至少有三个金额（金额/税额/价税合计），
// 任何不锚定标签的形状匹配都会串位。这类字段只认用户自己配的、带标签锚点的正则。
var builtinInvoiceFieldRegexes = map[string][]string{
	"invoice_no": {
		`发票号码\s*[:：]?\s*([0-9]{8,20})`,
		`\b(\d{20})\b`,
	},
	"invoice_code": {
		`发票代码\s*[:：]?\s*([0-9]{10,12})`,
	},
	"invoice_date": {
		`开票日期\s*[:：]?\s*(\d{4}\s*[-/年.]\s*\d{1,2}\s*[-/月.]\s*\d{1,2}\s*日?)`,
	},
	// 购销方税号：以标签定位，避免同批两个 18 位码互相干扰。
	"buyer_tax_no": {
		`购买方[\s\S]{0,120}?([0-9A-HJ-NPQRTUWXY]{2}\d{6}[0-9A-HJ-NPQRTUWXY]{10})`,
	},
	"seller_tax_no": {
		`销售方[\s\S]{0,120}?([0-9A-HJ-NPQRTUWXY]{2}\d{6}[0-9A-HJ-NPQRTUWXY]{10})`,
	},
}

// RegexFallbackInvoiceFields 对发票结果执行后置正则补洞，返回本次补洞明细。
//
// 实现是 Schema 驱动统一补洞（SchemaRegexPatch）的薄封装：发票 ScopeSchema
// 已在注册表里声明标准字段序、动态桶（Fields）与明细桶（items/Extra）。
// 行为与旧的结构体专用实现逐项等价（双跑测试锁定，含全部放弃路径）。
//
// cfg 为用户提取配置（可为 nil）：其中 ExtractFieldConfig.Regex 若非空，优先于内置
// 正则使用（用户口径 > 系统默认）。补洞结果同时写入 res.PatchedByRegex（字段名去重）
// 与 res.RegexPatches（明细），供前端展示"该字段来源"。
func RegexFallbackInvoiceFields(res *InvoiceExtractionResult, text string, cfg *types.KbExtractConfig) []RegexFieldPatch {
	if res == nil || res.Kind == "not_invoice" || len(res.Invoices) == 0 {
		return nil
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	schema, ok := GetScopeSchema(types.FleetCategoryScopeInvoice)
	if !ok {
		return nil
	}
	patches := SchemaRegexPatch(NewGenericRecords(res.Invoices), text, cfg, schema)
	for _, p := range patches {
		if !stringInSlice(res.PatchedByRegex, p.Field) {
			res.PatchedByRegex = append(res.PatchedByRegex, p.Field)
		}
	}
	res.RegexPatches = append(res.RegexPatches, patches...)
	return patches
}

// ---- 共享工具（Schema 补洞与既有调用方共用） ----

// fieldRegexPattern 一条候选正则及其来源。
type fieldRegexPattern struct {
	re     string
	source string
}

// uniqueRegexCapture 在文本中匹配，且**全批只允许一个不同取值**时返回它。
// 取首个捕获组（有则用），否则取整个匹配。
func uniqueRegexCapture(text string, re *regexp.Regexp) (string, bool) {
	matches := re.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return "", false
	}
	seen := make(map[string]struct{}, len(matches))
	value := ""
	for _, m := range matches {
		v := m[0]
		if len(m) > 1 && strings.TrimSpace(m[1]) != "" {
			v = m[1]
		}
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		seen[v] = struct{}{}
		value = v
	}
	if len(seen) != 1 {
		return "", false
	}
	return value, true
}

// parsePatchedNumber 统一清洗货币符与千分位后转 float64。
//
// 覆盖实测票面形态：`¥1,234.56`、`￥1,234.56`、`1,234.56 元`、`13%`（税率）。
// 关键取舍：清洗后仍解析不了的（「壹仟贰佰元整」）一律失败返回 false——
// 写成 0 会被用户当成真实金额，比留空更糟。
func parsePatchedNumber(raw string) (float64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}
	s = strings.NewReplacer(
		"¥", "", "￥", "", "$", "", "＄", "",
		",", "", "，", "",
		"元", "", " ", "", "\u00a0", "", "\u3000", "",
	).Replace(s)
	s = strings.TrimSuffix(s, "%")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// floatFieldText 把可空数值字段转成用于"是否已填"判断的字符串。
func floatFieldText(f *float64) string {
	if f == nil {
		return ""
	}
	return strconv.FormatFloat(*f, 'f', -1, 64)
}

// toStringValue 把 JSON 反序列化出的 any 值收敛成字符串（Fields 桶可能是数字/布尔）。
func toStringValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return ""
	}
}

func stringInSlice(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
