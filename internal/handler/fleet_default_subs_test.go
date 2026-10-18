package handler

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// invoice 分类的默认字段集与内置标记。
//
// 用户口径（2026-10-18 二轮）：新建发票分类不能是白纸，
// 且「提取状态」「标签」必须默认启用（列表/筛选器/六同步都依赖这两列）；
// 标准字段属于系统契约，前端据此隐藏删除入口。

func TestBusinessInvoiceDefaultSubsCoversSystemFields(t *testing.T) {
	subs := invoiceDefaultStandardSubs()
	if len(subs) == 0 {
		t.Fatal("默认字段集不得为空")
	}
	byName := make(map[string]types.FleetCategorySub, len(subs))
	for _, s := range subs {
		if _, dup := byName[s.Name]; dup {
			t.Fatalf("默认字段集出现重复键: %s", s.Name)
		}
		byName[s.Name] = s
	}
	// 提取状态 / 标签 必须存在且默认启用
	for _, k := range []string{"extractStatus", "tags"} {
		s, ok := byName[k]
		if !ok {
			t.Fatalf("缺少系统字段 %s", k)
		}
		if !s.Enabled {
			t.Fatalf("%s 必须默认启用", k)
		}
	}
	// 发票核心字段必须齐备
	for _, k := range []string{"invoiceNo", "invoiceDate", "invoiceType", "amount", "taxRate", "tax", "totalAmount", "items"} {
		if _, ok := byName[k]; !ok {
			t.Fatalf("缺少标准字段 %s", k)
		}
	}
	// 全量标记 builtin：前端据此隐藏删除入口
	for _, s := range subs {
		if !s.Builtin {
			t.Fatalf("%s 必须标记为内置字段（否则前端会放出去删）", s.Name)
		}
	}
	// items 的标准子项也要齐备且带内置标记
	items := byName["items"]
	if len(items.SubFields) < 7 {
		t.Fatalf("items 应带标准明细子项，实际 %d 个", len(items.SubFields))
	}
	for _, sf := range items.SubFields {
		if !sf.IsEnabled() {
			t.Fatalf("子项 %s 应默认启用", sf.Key)
		}
		if !sf.Builtin {
			t.Fatalf("子项 %s 必须标记 builtin", sf.Key)
		}
	}
}

func TestBusinessEnsureInvoiceSystemFieldsKeepsUserDefinition(t *testing.T) {
	// 用户已有的 invoiceType（自定义表头）必须保留，不被系统底稿覆盖
	user := types.FleetCategorySub{
		Name: "invoiceType", Label: "票据类型", Enabled: true, IsDefault: false, DataType: "text",
	}
	out := ensureInvoiceSystemFields([]types.FleetCategorySub{user})
	// 用户已有 invoiceType → 只补 extractStatus / tags，共 3 条
	if len(out) != 3 {
		t.Fatalf("补齐后条目数异常: %d", len(out))
	}
	if out[0].Label != "票据类型" || out[0].IsDefault {
		t.Fatalf("用户定义被覆盖: %+v", out[0])
	}
	for _, name := range []string{"extractStatus", "tags"} {
		found := false
		for _, s := range out {
			if s.Name == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("应补齐系统字段 %s", name)
		}
	}
}

func TestBusinessMarkBuiltinFieldsUpgradesLegacy(t *testing.T) {
	// 存量发票分类的 subs 没有 builtin 键：seed 时按标准字段清单补标记
	subs := []types.FleetCategorySub{
		{Name: "invoiceNo", Enabled: true, DataType: "text"},
		{Name: "items", Enabled: true, DataType: "items"},
		{Name: "account_no", Label: "户号", Enabled: true, DataType: "text"}, // 用户自建扩展字段
	}
	if !markBuiltinFields(subs, invoiceDefaultStandardSubs()) {
		t.Fatal("存量标准字段应被标记")
	}
	if !subs[0].Builtin || !subs[1].Builtin {
		t.Fatal("标准字段与 items 都应标记 builtin")
	}
	if subs[2].Builtin {
		t.Fatal("用户自建扩展字段不得被标记为内置")
	}
	// 幂等
	if markBuiltinFields(subs, invoiceDefaultStandardSubs()) {
		t.Fatal("已标记的数据不应重复写库")
	}
}

func TestBusinessMarkBuiltinSubFieldsUpgradesLegacy(t *testing.T) {
	// 存量 subs 的 sub_fields 没有 builtin/enabled：seed 时必须补标记
	subFields := []types.FleetItemSubField{
		{Key: "name", Label: "项目名称"},
		{Key: "spec", Label: "规格型号"},
	}
	tpl := invoiceItemSubFieldsStandard()
	changed := markBuiltinSubFields(subFields, tpl)
	if !changed {
		t.Fatal("存量数据应被标记（返回 true 才写库）")
	}
	for _, sf := range subFields {
		if !sf.Builtin {
			t.Fatalf("子项 %s 应被标记 builtin", sf.Key)
		}
		if !sf.IsEnabled() {
			t.Fatalf("子项 %s 应默认启用", sf.Key)
		}
	}
	// 幂等：再跑一次不应再报 changed，避免每次启动都写库
	if markBuiltinSubFields(subFields, tpl) {
		t.Fatal("已标记的数据不应重复写库")
	}
}
