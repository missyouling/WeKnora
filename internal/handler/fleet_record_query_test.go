package handler

import (
	"testing"
)

// 字段筛选串口径与发票模块一致：`key:value;key:value`（分号分隔，值可含逗号）。
func TestParseFleetFieldFilters(t *testing.T) {
	got := parseFleetFieldFilters("车牌号:渝C81022;费用类型:ETC通行费")
	if len(got) != 2 {
		t.Fatalf("应解析出 2 个条件，实际 %d: %+v", len(got), got)
	}
	if got[0].Key != "车牌号" || got[0].Value != "渝C81022" {
		t.Fatalf("解析结果错误: %+v", got)
	}
	// 空串 / 无冒号 / 空值 一律忽略
	if v := parseFleetFieldFilters(""); len(v) != 0 {
		t.Fatalf("空串应返回空切片，实际 %+v", v)
	}
	if v := parseFleetFieldFilters("车牌号"); len(v) != 0 {
		t.Fatalf("无冒号项应忽略，实际 %+v", v)
	}
	if v := parseFleetFieldFilters("车牌号: ;费用类型:ETC"); len(v) != 1 {
		t.Fatalf("空值项应忽略，实际 %+v", v)
	}
	// 顺序必须保留（前端按声明顺序展示筛选条件）
	if v := parseFleetFieldFilters("b:2;a:1"); v[0].Key != "b" || v[1].Key != "a" {
		t.Fatalf("顺序被改变: %+v", v)
	}
}

// 分页参数归一：缺省第 1 页 20 条；越界夹到上限，防止一次拉全表。
func TestNormalizeFleetPage(t *testing.T) {
	cases := []struct{ page, size, wantPage, wantSize int }{
		{0, 0, 1, 20},
		{-3, -1, 1, 20},
		{2, 50, 2, 50},
		{1, 9999, 1, 200},
	}
	for _, c := range cases {
		p, s := normalizeFleetPage(c.page, c.size)
		if p != c.wantPage || s != c.wantSize {
			t.Fatalf("(%d,%d) → (%d,%d)，期望 (%d,%d)", c.page, c.size, p, s, c.wantPage, c.wantSize)
		}
	}
}

// LIKE 通配符必须转义，否则用户搜 "%" 会匹配全表。
func TestFleetLikePattern(t *testing.T) {
	if got := fleetLikePattern("渝C81022"); got != "%渝C81022%" {
		t.Fatalf("普通关键词应包成包含匹配，实际 %q", got)
	}
	if got := fleetLikePattern("100%"); got != "%100\\%%" {
		t.Fatalf("%% 必须转义，实际 %q", got)
	}
	if got := fleetLikePattern("a_b"); got != "%a\\_b%" {
		t.Fatalf("_ 必须转义，实际 %q", got)
	}
}
