package service

import (
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// 分类专属字段筛选（第 26 轮 O-3，补齐六同步的第 ② 环）。
//
// 背景：字段定义 / 列表列 / 悬停明细浮窗 / 编辑抽屉 / 打印都由分类 subs 派生，
// 唯独「字段筛选器」写死成「关键词 / 税率 / 开票日期」三项。通行费发票最需要
// 按车牌号筛（运营会问「渝A75R1C 这个月跑了几趟」），此前只能把车牌丢进关键词框
// —— 那会误命中备注、明细名称里出现同一串的记录。
//
// 语义：
//   - 键 = 分类 subs 的字段名（中文权威名，如「车牌号」；也接受 plate_no 等别名，
//     与前端列/抽屉读取同一套键）；
//   - 值 = 期望子串（与关键词一致的「包含」语义，用户常只记得后 5 位）；
//   - 多条件 AND；空值条件视为未填（放行），不当作「必须为空」。

// ParseInvoiceFieldFilters 解析 `key:value;key:value` 形式的字段筛选参数。
//
// 容错：空段、无冒号段、空 key/空 value 一律跳过 —— 前端清空筛选器时可能
// 传 `车牌号:` 或 `;;`，这些都不该变成「筛不到任何记录」。
//
// 用 `;` 而不是 `,` 分隔：字段值本身可能含逗号（如「重庆市,璧山区」）。
func ParseInvoiceFieldFilters(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := make(map[string]string, 2)
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.IndexAny(part, ":：")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(part[:idx])
		val := strings.TrimSpace(part[idx+1:])
		if key == "" || val == "" {
			continue
		}
		out[key] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// invoiceRecordMatchFieldFilters 判断记录是否满足全部字段筛选条件。
//
// 查找顺序：票级 `Fields`（分类动态字段桶）→ 明细行 `Extra`（车牌号也可能只落在
// 明细行）。两处都是 `map[string]any`，值可能是 string / float64 / bool，
// 统一用 `fieldFilterValueText` 转成文本再比对。
func invoiceRecordMatchFieldFilters(r types.InvoiceRecord, filters map[string]string) bool {
	if len(filters) == 0 {
		return true
	}
	for key, wantRaw := range filters {
		want := strings.TrimSpace(wantRaw)
		if want == "" {
			continue // 空值不算条件
		}
		if !recordHasFieldValue(r, key, want) {
			return false
		}
	}
	return true
}

// recordHasFieldValue 在票级 fields 与明细行 extra 里查找 key，命中即返回 true。
func recordHasFieldValue(r types.InvoiceRecord, key, want string) bool {
	want = strings.ToLower(want)
	if v, ok := r.Fields[key]; ok {
		if strings.Contains(strings.ToLower(fieldFilterValueText(v)), want) {
			return true
		}
	}
	for _, it := range r.Items {
		if v, ok := it.Extra[key]; ok {
			if strings.Contains(strings.ToLower(fieldFilterValueText(v)), want) {
				return true
			}
		}
	}
	return false
}

// fieldFilterValueText 把 JSONB 解出来的任意标量转成可比较文本。
//
// 数字必须走 `%v` 而不是 `%.2f`：车牌号里的数字段、单价等在这里只做子串匹配，
// 固定小数位会凭空造出「.00」后缀导致匹配不上。
func fieldFilterValueText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		// 整数值去掉小数尾零（100 而不是 100.000000）
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", t), "0"), ".")
	default:
		return fmt.Sprintf("%v", t)
	}
}
