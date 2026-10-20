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
func TestNoticeAndAwardPunishFieldsDisjoint(t *testing.T) {
	notice, ok1 := GetScopeSchema("notice")
	award, ok2 := GetScopeSchema("award_punish")
	require.True(t, ok1 && ok2)

	// 三件套（提取状态/标签/备注）是各 scope 共有基元，允许重复；
	// 除此之外两套业务字段必须零交集。
	shared := map[string]struct{}{"remark": {}, "extractstatus": {}, "tags": {}}
	nk := make(map[string]struct{}, len(notice.StandardFields))
	for _, f := range notice.StandardFields {
		nk[canonicalFieldKeyForTest(f.Key)] = struct{}{}
	}
	overlap := 0
	for _, f := range award.StandardFields {
		k := canonicalFieldKeyForTest(f.Key)
		if _, isShared := shared[k]; isShared {
			continue
		}
		if _, dup := nk[k]; dup {
			overlap++
			assert.False(t, dup, "业务字段 %s 不得同时属于 notice 与 award_punish", f.Key)
		}
	}
	assert.Equal(t, 0, overlap, "两套业务字段必须零交集")
	// 双方各自的主业务字段必须存在
	assert.Contains(t, nk, "apno")
	assert.Contains(t, nk, "issuedept")
	awardKeys := make(map[string]struct{}, len(award.StandardFields))
	for _, f := range award.StandardFields {
		awardKeys[canonicalFieldKeyForTest(f.Key)] = struct{}{}
	}
	assert.Contains(t, awardKeys, "measure")
	assert.Contains(t, awardKeys, "basis")
}

// 系统内置三件套：分类主轴字段 + 提取状态 + 标签，三者缺一不可。
func TestDocumentScopesBuiltinTriple(t *testing.T) {
	mainAxis := map[string]string{
		"contract": "contractType", "regulation": "regType",
		"notice": "apType", "award_punish": "measure",
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
