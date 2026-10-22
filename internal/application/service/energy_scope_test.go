package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// energy scope 必须与其它 document 业务一样拥有显式声明的标准字段契约：
// 无标准字段 → AI 字典拿不到字段语义、后置正则补洞进不来（RegexPatchable=false）、
// 设置页「提取规则」是空白、列表/抽屉/过滤器无字段可渲染。
func TestEnergyScopeHasStandardFields(t *testing.T) {
	s, ok := GetScopeSchema("energy")
	require.True(t, ok, "energy scope 必须已注册")
	assert.Equal(t, "document", s.Kind)
	assert.Equal(t, "能耗类型", s.CategoryLabel)
	assert.NotEmpty(t, s.StandardFields, "energy 必须声明标准字段")
	assert.True(t, s.RegexPatchable, "energy 必须接入正则补洞")
}

// 产品清单的 10 个系统级标准字段必须齐全且顺序正确，且锁定不可删。
func TestEnergyScopeStandardFieldSet(t *testing.T) {
	cat, ok := BusinessFieldCatalogOf("energy")
	require.True(t, ok, "energy 未注册字段目录")

	standardKeys := make([]string, 0, len(cat.Standard))
	for _, f := range cat.Standard {
		standardKeys = append(standardKeys, f.Key)
	}
	// 前 10 个是产品清单指定的系统级标准字段（顺序即界面顺序）
	require.GreaterOrEqual(t, len(standardKeys), 10)
	assert.Equal(t, []string{
		"utility_no", "billing_period", "utility_type",
		"previous_reading", "current_reading", "multiplier",
		"usage_amount", "unit_price", "total_amount", "extract_status",
	}, standardKeys[:10], "能耗标准字段顺序必须与产品清单一致")

	// 数值型字段的类型必须是 number（抽屉渲染数字输入框）
	wantNumber := map[string]bool{
		"previous_reading": true, "current_reading": true, "multiplier": true,
		"usage_amount": true, "unit_price": true, "total_amount": true,
	}
	for _, f := range cat.Standard {
		if wantNumber[f.Key] {
			assert.Equal(t, "number", f.Type, "%s 必须是 number 类型", f.Key)
		}
	}
}

// 系统内置字段锁定不可删：分类主轴 + 提取状态 + 标签。
func TestEnergyScopeBuiltinFields(t *testing.T) {
	cat, ok := BusinessFieldCatalogOf("energy")
	require.True(t, ok)
	builtin := map[string]bool{}
	for _, f := range cat.AllFields() {
		if f.Builtin {
			builtin[f.Key] = true
		}
	}
	for _, want := range []string{"utility_no", "utility_type", "extract_status", "tags"} {
		assert.True(t, builtin[want], "%s 必须是系统内置不可删字段", want)
	}
}

// 每个 scope 必须有高级模式 Prompt 模板（且保留字段插槽）。
func TestEnergyScopeHasPromptTemplate(t *testing.T) {
	tpl := DefaultBusinessPromptTemplate("energy")
	assert.Contains(t, tpl, "{{fields_schema}}", "能耗高级模板必须保留字段插槽")
	assert.Contains(t, tpl, "账期", "能耗模板必须说明账期口径")
}

// 默认提取字段底稿：每个字段都自带描述、规则、类型；数值字段必须带正则锚点。
func TestEnergyDefaultExtractFields(t *testing.T) {
	fields := DefaultExtractFields("energy", "")
	require.NotEmpty(t, fields)
	byName := map[string]bool{}
	for _, f := range fields {
		assert.NotEmpty(t, f.Name)
		assert.NotEmpty(t, f.Label, "%s 缺中文名", f.Name)
		assert.NotEmpty(t, f.Desc, "%s 缺描述", f.Name)
		assert.NotEmpty(t, f.Rule, "%s 缺提取规则", f.Name)
		assert.NotEmpty(t, f.Type, "%s 缺类型", f.Name)
		byName[f.Name] = true
	}
	for _, want := range []string{
		"utility_no", "billing_period", "utility_type",
		"previous_reading", "current_reading", "multiplier",
		"usage_amount", "unit_price", "total_amount",
	} {
		assert.True(t, byName[want], "能耗默认底稿缺字段 %s", want)
	}
}

// ScopeSchema 的标准字段必须与字段目录同源（防止两处漂移）。
func TestEnergyScopeSchemaMatchesCatalog(t *testing.T) {
	s, ok := GetScopeSchema("energy")
	require.True(t, ok)
	cat, ok := BusinessFieldCatalogOf("energy")
	require.True(t, ok)
	require.Equal(t, len(cat.Standard), len(s.StandardFields))
	for i := range s.StandardFields {
		assert.Equal(t, cat.Standard[i].Key, s.StandardFields[i].Key, "第 %d 项键名必须一致", i)
		assert.Equal(t, cat.Standard[i].Label, s.StandardFields[i].Label, "第 %d 项中文名必须一致", i)
	}
}
