package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
)

// 能耗内置分类的 subs 必须与字段目录逐项一致（六同步的字段契约层）。
//
// 种子与目录两处漂移会导致：用户在「字段定义」里看到的字段与提取器读到的字段
// 不是一套（列缺列 / 抽屉多字段），且改名同步会失效。
func TestEnergySeedSubsMatchCatalog(t *testing.T) {
	cat, ok := service.BusinessFieldCatalogOf("energy")
	require.True(t, ok, "energy 未注册字段目录")

	seed := energySeedSubs()
	seedKeys := make(map[string]types.FleetCategorySub, len(seed))
	for _, s := range seed {
		seedKeys[s.Name] = s
	}
	// 目录里的每个字段都必须出现在种子里（缺一个 → 该列/该抽屉字段永远不存在）
	for _, f := range cat.AllFields() {
		s, ok := seedKeys[f.Key]
		require.True(t, ok, "能耗种子缺字段 %s（目录有、种子没有）", f.Key)
		assert.Equal(t, f.Type, s.DataType, "字段 %s 的类型与目录不一致", f.Key)
	}
	// 种子不得有目录外的字段（多一个 → 抽屉多出一个无契约的字段）
	for _, s := range seed {
		found := false
		for _, f := range cat.AllFields() {
			if f.Key == s.Name {
				found = true
				break
			}
		}
		assert.True(t, found, "能耗种子有多余字段 %s（目录没有）", s.Name)
	}
}

// 种子不得包含列表系统列 fileName：它不是提取字段，
// seed 进去会渗进字段定义面板与提取规则表，成为用户看不到来历的幽灵字段。
func TestEnergySeedSubsNoListSystemColumn(t *testing.T) {
	for _, s := range energySeedSubs() {
		assert.NotEqual(t, "fileName", s.Name, "种子不得包含列表系统列")
	}
}

// 读数类字段必须是 number 类型（抽屉渲染数字输入框）。
func TestEnergySeedReadingFieldsAreNumber(t *testing.T) {
	byName := map[string]string{}
	for _, s := range energySeedSubs() {
		byName[s.Name] = s.DataType
	}
	for _, k := range []string{
		"previous_reading", "current_reading", "multiplier",
		"usage_amount", "unit_price", "total_amount",
	} {
		assert.Equal(t, "number", byName[k], "%s 必须是 number 类型", k)
	}
	assert.Equal(t, "text", byName["billing_period"], "账期是月粒度文本")
}

// 合并 subs：按 seed 顺序重排，保留用户改过的展示属性，不丢用户自建字段。
func TestMergeEnergySeedSubs(t *testing.T) {
	existing := []types.FleetCategorySub{
		// 用户把倍率改成了 number 且关掉了默认显示
		{Name: "multiplier", Enabled: true, IsDefault: false, DataType: "number"},
		// 用户自建字段（seed 里没有）
		{Name: "custom_field", Enabled: true, IsDefault: false, DataType: "text"},
	}
	merged := mergeEnergySeedSubs(existing)

	// 顺序以 seed 为准
	seed := energySeedSubs()
	require.GreaterOrEqual(t, len(merged), len(seed))
	for i, f := range seed {
		assert.Equal(t, f.Name, merged[i].Name, "第 %d 项顺序与 seed 不一致", i)
	}
	// 用户改过的属性被保留
	var mult types.FleetCategorySub
	for _, s := range merged {
		if s.Name == "multiplier" {
			mult = s
		}
	}
	assert.False(t, mult.IsDefault, "用户关掉的默认显示不得被 seed 覆盖回 true")
	// 用户自建字段不丢
	found := false
	for _, s := range merged {
		if s.Name == "custom_field" {
			found = true
		}
	}
	assert.True(t, found, "用户自建字段不得被 seed 合并丢掉")
}

// 幂等：连续合并两次结果一致（否则每次启动都在改库）。
func TestMergeEnergySeedSubsIdempotent(t *testing.T) {
	once := mergeEnergySeedSubs(nil)
	twice := mergeEnergySeedSubs(once)
	assert.Len(t, twice, len(once))
	for i := range once {
		assert.Equal(t, once[i].Name, twice[i].Name)
		assert.Equal(t, once[i].DataType, twice[i].DataType)
		assert.Equal(t, once[i].IsDefault, twice[i].IsDefault)
	}
}
