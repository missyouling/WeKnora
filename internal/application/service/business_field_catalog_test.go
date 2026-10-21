package service

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// 四模块（合同/制度/通知公告 + 发票字段扩展）在「设置 -> 提取规则」里不允许出现
// 大片「未配置」：每个字段都必须自带描述、自然语言规则；编号/日期/金额/税号等
// 形状唯一的字段还必须自带 RE2 正则；每个 scope 必须有高级模式 Prompt 模板。
func TestBusinessCatalogsComplete(t *testing.T) {
	for _, scope := range []string{"contract", "regulation", "award_punish"} {
		cat, ok := BusinessFieldCatalogOf(scope)
		require.True(t, ok, "%s 未注册字段目录", scope)
		require.NotEmpty(t, cat.Standard, "%s 缺标准字段", scope)
		require.NotEmpty(t, cat.Extended, "%s 缺扩展字段", scope)
		assert.Contains(t, cat.PromptTemplate, "{{fields_schema}}", "%s 高级模板必须保留字段插槽", scope)

		fields := DefaultExtractFields(scope, "")
		require.NotEmpty(t, fields)
		withRegex := 0
		for _, f := range fields {
			assert.NotEmpty(t, strings.TrimSpace(f.Name), "%s 存在空字段名", scope)
			assert.NotEmpty(t, strings.TrimSpace(f.Label), "%s.%s 缺中文名", scope, f.Name)
			assert.NotEmpty(t, strings.TrimSpace(f.Desc), "%s.%s 缺描述", scope, f.Name)
			assert.NotEmpty(t, strings.TrimSpace(f.Rule), "%s.%s 缺提取规则", scope, f.Name)
			assert.NotEmpty(t, strings.TrimSpace(f.Type), "%s.%s 缺类型", scope, f.Name)
			if strings.TrimSpace(f.Regex) != "" {
				withRegex++
			}
		}
		// 编号 / 日期 / 金额 / 甲乙方等形状唯一的字段必须有正则
		assert.GreaterOrEqual(t, withRegex, 6, "%s 内置正则过少（%d）", scope, withRegex)
	}
}

// 合同字段体系严格按产品清单与顺序落地：11 标准 + 15 扩展，键名即落库契约。
func TestContractFieldCatalogOrder(t *testing.T) {
	cat, ok := BusinessFieldCatalogOf("contract")
	require.True(t, ok)
	standardKeys := make([]string, 0, len(cat.Standard))
	for _, f := range cat.Standard {
		standardKeys = append(standardKeys, f.Key)
	}
	assert.Equal(t, []string{
		"contract_no", "contract_name", "contract_type", "party_a", "party_b",
		"sign_date", "expire_date", "amount", "extract_status", "performance_status", "tags",
	}, standardKeys, "合同标准字段顺序必须与产品清单一致")

	extKeys := make([]string, 0, len(cat.Extended))
	for _, f := range cat.Extended {
		extKeys = append(extKeys, f.Key)
	}
	assert.Equal(t, []string{
		"party_a_contact", "party_a_tax_no", "party_b_contact", "party_b_tax_no",
		"party_a_address", "party_a_phone", "party_a_bank", "party_a_account",
		"party_b_address", "party_b_phone", "party_b_bank", "party_b_account",
		"payment_method", "sign_location", "summary",
	}, extKeys, "合同扩展字段顺序必须与产品清单一致")

	// 系统内置三件套：分类主轴 + 提取状态 + 标签
	builtin := map[string]bool{}
	for _, f := range cat.AllFields() {
		if f.Builtin {
			builtin[f.Key] = true
		}
	}
	assert.Equal(t, map[string]bool{"contract_type": true, "extract_status": true, "tags": true}, builtin)
}

// 每组正则会作为「LLM 漏提后的补洞规则」跑在原文上，必须能被 Go RE2 编译；
// 除系统字段（提取状态 / 履约状态 / 标签）外，每个字段都要有可用的正则锚点。
func TestBusinessCatalogRegexCoverage(t *testing.T) {
	systemField := map[string]bool{"extract_status": true, "performance_status": true, "tags": true}
	for _, scope := range []string{"contract", "regulation", "award_punish", "notice"} {
		cat, ok := BusinessFieldCatalogOf(scope)
		require.True(t, ok)
		checked := 0
		for _, f := range cat.AllFields() {
			if systemField[f.Key] {
				assert.Empty(t, strings.TrimSpace(f.Regex), "%s.%s 是系统字段，不应配正则（会误伤原文）", scope, f.Key)
				continue
			}
			if f.Type == "array" {
				continue
			}
			if !assert.NotEmpty(t, strings.TrimSpace(f.Regex), "%s.%s 缺正则锚点", scope, f.Key) {
				continue
			}
			_, err := regexp.Compile(f.Regex)
			assert.NoError(t, err, "%s.%s 正则无法编译: %s", scope, f.Key, f.Regex)
			checked++
		}
		assert.GreaterOrEqual(t, checked, 10, "%s 正则覆盖过少（%d）", scope, checked)
	}
}

// 回填：空配置直接套用默认底稿；已有配置只补空值，绝不覆盖用户改过的文案。
func TestBackfillExtractConfigDefaults(t *testing.T) {
	empty := &types.KbExtractConfig{Scope: "contract", CertType: "服务合同"}
	require.True(t, BackfillExtractConfigDefaults(empty))
	assert.NotEmpty(t, empty.Fields)
	assert.NotEmpty(t, empty.PromptTemplate)

	custom := &types.KbExtractConfig{
		Scope:    "contract",
		CertType: "服务合同",
		Fields: []types.ExtractFieldConfig{
			{Name: "contract_no", Enabled: true, Rule: "用户自定义规则"},
			{Name: "amount", Enabled: true},
		},
		PromptTemplate: "用户自定义模板 {{fields_schema}}",
	}
	require.True(t, BackfillExtractConfigDefaults(custom))
	assert.Equal(t, "用户自定义规则", custom.Fields[0].Rule, "用户改过的规则不得被覆盖")
	assert.NotEmpty(t, custom.Fields[0].Desc, "空描述要补")
	assert.NotEmpty(t, custom.Fields[1].Rule, "空规则要补")
	assert.Equal(t, "用户自定义模板 {{fields_schema}}", custom.PromptTemplate)

	// 幂等：再跑一次不应产生改动
	again := &types.KbExtractConfig{Scope: "contract", CertType: "服务合同"}
	BackfillExtractConfigDefaults(again)
	assert.False(t, BackfillExtractConfigDefaults(again), "回填必须幂等")
}

// camelCase（界面 subs）与 snake_case（落库契约）之间的唯一转换点。
func TestSnakeKeyNormalization(t *testing.T) {
	cases := map[string]string{
		"contractNo":        "contract_no",
		"partyAContact":     "party_a_contact",
		"performanceStatus": "performance_status",
		"amount":            "amount",
		"contract_no":       "contract_no",
		"apNo":              "ap_no",
	}
	for in, want := range cases {
		assert.Equal(t, want, SnakeKey(in), "SnakeKey(%q)", in)
	}
	cfg := &types.KbExtractConfig{Scope: "contract", Fields: []types.ExtractFieldConfig{{Name: "partyA"}, {Name: "amount"}}}
	converted := WithSnakeFieldKeys(cfg)
	assert.Equal(t, "party_a", converted.Fields[0].Name)
	assert.Equal(t, "partyA", cfg.Fields[0].Name, "原配置（界面回显用）不得被改写")
}
