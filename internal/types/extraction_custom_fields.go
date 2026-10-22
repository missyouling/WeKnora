package types

import (
	"encoding/json"
	"reflect"
	"strings"
)

// CollectExtractionCustomFields 从模型返回的原始 JSON 里收集用户自定义字段。
//
// 背景：用户在「设置 → 字段定义 & 提取规则」里新增的字段（如「保证金」）不在
// 提取项的强类型字段上，json.Unmarshal 会把它们**静默丢弃** —— 这是
// 2026-10-22 真机事故「配了保证金规则却永远提不到」的核心根因。
//
// 两个来源合一：
//  1. 顶层未知键（模型按提示词把「保证金」直接放在对象顶层）；
//  2. 显式 fields 子对象（存量数据/部分模型习惯收在 fields 里）。
//
// std 为受调用结构体的标准 json 键集合（可用 StandardJSONKeys 取）；
// base 非 nil 时作为初始值（调用方的 fields 子对象已解析出的部分），
// 来源键不覆盖已有值。无任何自定义字段时返回 nil（不落空 map）。
func CollectExtractionCustomFields(raw map[string]json.RawMessage, std map[string]bool, base map[string]any) map[string]any {
	out := base
	if out == nil {
		out = map[string]any{}
	}
	for k, v := range raw {
		if std[k] {
			continue
		}
		if _, exists := out[k]; exists {
			continue
		}
		var val any
		if err := json.Unmarshal(v, &val); err != nil {
			continue
		}
		out[k] = val
	}
	// 显式 fields 子对象（标准键，上面已跳过，这里单独展开合并）
	if fv, ok := raw["fields"]; ok {
		var sub map[string]json.RawMessage
		if err := json.Unmarshal(fv, &sub); err == nil {
			for k, v := range sub {
				if _, exists := out[k]; exists {
					continue
				}
				var val any
				if err := json.Unmarshal(v, &val); err != nil {
					continue
				}
				out[k] = val
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// StandardJSONKeys 取任意结构体的 json 键集合（去空白、去空 tag）。
// 提取项用它区分「标准字段」与「用户自定义字段」。
func StandardJSONKeys(v any) map[string]bool {
	t := reflect.TypeOf(v)
	if t == nil || t.Kind() != reflect.Struct {
		return map[string]bool{}
	}
	out := make(map[string]bool, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		if idx := strings.Index(tag, ","); idx >= 0 {
			tag = tag[:idx]
		}
		if name := strings.TrimSpace(tag); name != "" {
			out[name] = true
		}
	}
	return out
}
