package service

import (
	"encoding/json"
	"reflect"
	"strings"
)

// MergeExtractionItems 把同一份源文件的多个批次提取结果按「非空优先」合并成一条记录。
//
// 合同 / 制度 / 通知公告都是「一文件一档案」：一份上传文件在列表中必须严格对应
// 一条记录。分批提取只是把长文档切小以适配上下文，绝不能把批次结果追加成多条
// 记录——真机事故：一份《厂房租赁协议书》被切成两批，第 1 批拿到甲乙方与金额，
// 第 2 批拿到尾页签订日期，最终落库成 HT-20261021-001（无日期）+
// HT-20261021-002（有日期）两条假记录，列表比上传文件多一条。
//
// 合并口径：
//   - 先出现的非空值优先（批次按文档顺序，前面的更接近合同首部，可信度更高）；
//   - 后续批次的非空值只用于填补前面缺失的字段；
//   - 数字 0 / 空串 / nil / 空集合一律视为空，避免"后批次的 0 覆盖前批次的真实值"。
//
// 泛型实现走 JSON 视图，三个业务的结构体都能直接用（字段增减无需改这里）。
func MergeExtractionItems[T any](items []T) (T, bool) {
	var zero T
	if len(items) == 0 {
		return zero, false
	}
	if len(items) == 1 {
		return items[0], true
	}
	merged := make(map[string]any, 32)
	for i := range items {
		raw, err := json.Marshal(items[i])
		if err != nil {
			continue
		}
		var view map[string]any
		if err := json.Unmarshal(raw, &view); err != nil {
			continue
		}
		for k, v := range view {
			if isEmptyMergeValue(v) {
				continue
			}
			if cur, ok := merged[k]; ok && !isEmptyMergeValue(cur) {
				continue
			}
			merged[k] = v
		}
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return items[0], true
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return items[0], true
	}
	return out, true
}

// isEmptyMergeValue 合并视图里的「空值」判定：nil / 空串（含纯空白）/ 0 / false / 空集合。
func isEmptyMergeValue(v any) bool {
	if v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case bool:
		return !t
	case float64:
		return t == 0
	case json.Number:
		f, err := t.Float64()
		return err == nil && f == 0
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return rv.IsNil()
	}
	return false
}
