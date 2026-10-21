package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 合同/制度/通知公告/奖惩 四个 document scope 必须与发票一样拥有**显式声明**的
// 标准字段契约：此前它们都是「无标准字段」的空壳，AI 字典拿不到字段语义、
// 后置正则补洞也进不来（RegexPatchable=false）。

func TestDocumentScopesHaveStandardFields(t *testing.T) {
	for _, scope := range []string{"contract", "regulation", "notice", "award_punish"} {
		s, ok := GetScopeSchema(scope)
		require.True(t, ok, "scope %s 必须已注册", scope)
		assert.NotEmpty(t, s.StandardFields, "scope %s 必须声明标准字段", scope)
		assert.True(t, s.RegexPatchable, "scope %s 必须接入正则补洞", scope)
		assert.Equal(t, "document", s.Kind)
	}
}

// 分类主轴字段必须与各业务语义一致（此前 notice 被写成「通知标题」、
// award_punish 被写成「通知名称」，两者语义互换，分类卡片文案全错）。
func TestDocumentScopeCategoryLabels(t *testing.T) {
	cases := map[string]string{
		"contract":     "合同类型",
		"regulation":   "制度类型",
		"notice":       "公告类型",
		"award_punish": "奖惩类型",
		"invoice":      "发票类型",
	}
	for scope, want := range cases {
		s, ok := GetScopeSchema(scope)
		require.True(t, ok)
		assert.Equal(t, want, s.CategoryLabel, "scope %s 分类主轴文案", scope)
	}
}

// notice 与 award_punish 不得再有字段交集（旧 NOTICE_FIELD_LABELS 把两套字段
// 混在一张表，AI 建议会把「处理决定」喂给公告）。
// notice 与 award_punish 是同一个业务（通知公告）的两个入口：字段集必须**完全一致**
// 且来自同一份字段目录。历史上的"零交集"设计让设置页写 notice、提取器读
// award_punish，配置与字段各说各话——这正是本次要消灭的双轨分裂。
func TestNoticeAndAwardPunishShareOneCatalog(t *testing.T) {
	notice, ok1 := GetScopeSchema("notice")
	award, ok2 := GetScopeSchema("award_punish")
	require.True(t, ok1 && ok2)

	assert.Equal(t, len(notice.StandardFields), len(award.StandardFields), "两个入口字段数必须一致")
	for i := range notice.StandardFields {
		assert.Equal(t,
			canonicalFieldKeyForTest(notice.StandardFields[i].Key),
			canonicalFieldKeyForTest(award.StandardFields[i].Key),
			"字段顺序与键必须逐项一致（同一目录派生）")
	}
	keys := make(map[string]struct{}, len(award.StandardFields))
	for _, f := range award.StandardFields {
		keys[canonicalFieldKeyForTest(f.Key)] = struct{}{}
	}
	for _, want := range []string{"apno", "aptitle", "aptype", "issuedept", "signdate", "effectivedate", "scope"} {
		assert.Contains(t, keys, want, "通知公告标准字段 %s 缺失", want)
	}
	// 真实样文（通报类）需要的业务字段必须能装进扩展字段，不能因为统一而丢失
	cat, ok := BusinessFieldCatalogOf("award_punish")
	require.True(t, ok)
	extKeys := map[string]struct{}{}
	for _, f := range cat.Extended {
		extKeys[f.Key] = struct{}{}
	}
	for _, want := range []string{"signer", "contact", "contact_phone", "urgency", "valid_until", "attachments", "person", "measure", "basis", "summary"} {
		assert.Contains(t, extKeys, want, "通知公告扩展字段 %s 缺失", want)
	}
}

func TestDocumentScopesBuiltinTriple(t *testing.T) {
	mainAxis := map[string]string{
		"contract": "contractType", "regulation": "regType",
		"notice": "apType", "award_punish": "apType",
	}
	for scope, axis := range mainAxis {
		s, _ := GetScopeSchema(scope)
		keys := make(map[string]struct{}, len(s.StandardFields))
		for _, f := range s.StandardFields {
			keys[canonicalFieldKeyForTest(f.Key)] = struct{}{}
		}
		assert.Contains(t, keys, canonicalFieldKeyForTest(axis), "%s 分类主轴字段", scope)
		assert.Contains(t, keys, "extractstatus", "%s 提取状态", scope)
		assert.Contains(t, keys, "tags", "%s 标签", scope)
	}
}

func canonicalFieldKeyForTest(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') {
			if r >= 'A' && r <= 'Z' {
				r += 32
			}
			out = append(out, r)
		}
	}
	return string(out)
}
