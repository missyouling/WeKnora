package handler

import (
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// 明细子列去重（发票「项目明细」）。
//
// 为什么需要：AI 建议与历史 seed 会同时产出同义子项——
// 通行费发票实测出现 `tax_amount`(税额) 与 `tax`(税额)、`price` 与 `unit_price`。
// 它们是同一个票面栏位，落库后明细浮层会出现两列同名「税额」，用户无法区分，
// 也无法判断该关哪一列。这里在**落库边界**做一次幂等收敛。

// itemSubFieldAliasGroups 同义键分组：同组内只允许存在一个键。
// 每组第一个元素是该语义的标准键（去重时优先保留它）。
var itemSubFieldAliasGroups = [][]string{
	{"name", "item_name", "project_name", "goods_name", "product_name"},
	{"spec", "model", "specification", "type_spec"},
	{"unit", "uom", "measure_unit"},
	{"qty", "quantity", "num", "count"},
	{"price", "unit_price", "deal_price"},
	{"amount", "total", "money", "subtotal"},
	{"tax_rate", "rate", "taxrate", "tax_percent"},
	{"tax_amount", "tax", "taxamt", "tax_money"},
}

// subFieldAliasIndex 小写键 → (组名, 是否为该组标准键)
var subFieldAliasIndex = func() map[string]struct {
	group     string
	canonical bool
} {
	out := make(map[string]struct {
		group     string
		canonical bool
	}, 32)
	for _, g := range itemSubFieldAliasGroups {
		for i, k := range g {
			// 索引用**归一键**（unit_price → unitprice）：查询侧同样是归一键，
			// 两处不一致会让 unit_price 逃过同义判定（实测就是这么漏的）。
			out[canonicalSubFieldKey(k)] = struct {
				group     string
				canonical bool
			}{group: g[0], canonical: i == 0}
		}
	}
	return out
}()

// canonicalSubFieldKey 归一子列键：小写 + 去除非字母数字（Tax_Amount → taxamount）。
func canonicalSubFieldKey(key string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(key)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r > 127 {
			// 中文键保留，避免「备注」「批次号」这类自定义键被归一成空串后互相误判重复
			b.WriteRune(r)
		}
	}
	return b.String()
}

// normalizeSubFieldLabel 列头归一：去空白、中文全角括号统一。
func normalizeSubFieldLabel(label, fallback string) string {
	s := strings.TrimSpace(label)
	if s == "" {
		s = strings.TrimSpace(fallback)
	}
	s = strings.ReplaceAll(s, "（", "(")
	s = strings.ReplaceAll(s, "）", ")")
	return strings.Join(strings.Fields(s), "")
}

// dedupeItemSubFields 幂等去重明细子列。
//
// 三条规则（按优先级）：
//  1. 同义键只留一个，优先保留该语义的标准键（tax_amount 胜过 tax，
//     price 胜过 unit_price）——标准键会被提取链路直接映射到明细行列；
//  2. 列头同名只留一个（同名列在浮层里无法区分）；
//  3. 完全相同的 key 只留一个。
//
// 不改变保留条目的顺序与其它属性（label/width/enabled/builtin 原样保留）。
// 第二个返回值表示是否发生了删除（调用方据此决定要不要写库）。
func dedupeItemSubFields(in []types.FleetItemSubField) ([]types.FleetItemSubField, bool) {
	if len(in) <= 1 {
		return in, false
	}
	keep := make([]bool, len(in))
	for i := range keep {
		keep[i] = true
	}
	changed := false

	// ① + ③：按「同义组 / 归一键」分组，组内择优
	chosen := make(map[string]int, len(in))
	for i := range in {
		key := canonicalSubFieldKey(in[i].Key)
		if key == "" {
			keep[i] = false
			changed = true
			continue
		}
		id := key
		pref := false
		if meta, ok := subFieldAliasIndex[key]; ok {
			id = "grp:" + meta.group
			pref = meta.canonical
		}
		j, seen := chosen[id]
		if !seen {
			chosen[id] = i
			continue
		}
		// 已有候选：标准键优先顶替，否则保留先出现的那条
		if pref && !isCanonicalSubFieldKey(in[j].Key) {
			keep[j] = false
			chosen[id] = i
		} else {
			keep[i] = false
		}
		changed = true
	}

	// ②：列头同名（跨组）只留第一个
	seenLabel := make(map[string]int, len(in))
	for i := range in {
		if !keep[i] {
			continue
		}
		label := normalizeSubFieldLabel(in[i].Label, in[i].Key)
		if label == "" {
			continue
		}
		if j, ok := seenLabel[label]; ok && j != i {
			keep[i] = false
			changed = true
			continue
		}
		seenLabel[label] = i
	}

	if !changed {
		return in, false
	}
	out := make([]types.FleetItemSubField, 0, len(in))
	for i := range in {
		if keep[i] {
			out = append(out, in[i])
		}
	}
	return out, true
}

// isCanonicalSubFieldKey 该键是否为其语义组的标准键。
func isCanonicalSubFieldKey(key string) bool {
	meta, ok := subFieldAliasIndex[canonicalSubFieldKey(key)]
	return ok && meta.canonical
}

// normalizeInvoiceCategorySubs 对发票分类的 items 子列做去重（落库前调用）。
// 非发票 scope 或没有 items 字段时原样返回。
func normalizeInvoiceCategorySubs(scope string, subs []types.FleetCategorySub) ([]types.FleetCategorySub, bool) {
	if scope != types.FleetCategoryScopeInvoice || len(subs) == 0 {
		return subs, false
	}
	changed := false
	for i := range subs {
		if subs[i].Name != "items" || len(subs[i].SubFields) <= 1 {
			continue
		}
		if deduped, ok := dedupeItemSubFields(subs[i].SubFields); ok {
			subs[i].SubFields = deduped
			changed = true
		}
	}
	return subs, changed
}
