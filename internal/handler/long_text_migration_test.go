package handler

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 回归（2026-10-22）：备注/摘要等长文本字段被当成单行文本，抽屉渲染成
// `<t-input>`，长内容（如「业务流水号:2026102215...」）截断显示，
// 用户既看不全也录不全。
//
// 修复：类型体系新增 `textarea`（多行文本），抽屉按 dataType 渲染
// `<t-textarea>`；存量已配为 `text` 的已知长文本字段由启动幂等任务升级。

// longTextSub 造一条 subs 记录（dataType 可省略，缺省 text）。
func longTextSub(name, label, dataType string) types.FleetCategorySub {
	if dataType == "" {
		dataType = "text"
	}
	return types.FleetCategorySub{Name: name, Label: label, DataType: dataType, Enabled: true}
}

func TestUpgradeLongTextSubsMigratesKnownFields(t *testing.T) {
	subs := []types.FleetCategorySub{
		longTextSub("remark", "备注", ""),
		longTextSub("summary", "摘要", ""),
		longTextSub("description", "说明", ""),
		longTextSub("备注", "", ""),              // 中文名直接作为 Name 的存量形态
		longTextSub("内容摘要", "", ""),            // 中文别名
		longTextSub("contract_no", "合同编号", ""), // 非长文本：绝不能动
	}
	got, changed := upgradeLongTextSubs(subs)
	assert.Equal(t, 5, changed, "5 条已知长文本字段应被升级")

	byName := map[string]string{}
	for _, s := range got {
		byName[s.Name] = s.DataType
	}
	assert.Equal(t, "textarea", byName["remark"])
	assert.Equal(t, "textarea", byName["summary"])
	assert.Equal(t, "textarea", byName["description"])
	assert.Equal(t, "textarea", byName["备注"])
	assert.Equal(t, "textarea", byName["内容摘要"])
	assert.Equal(t, "text", byName["contract_no"], "非长文本字段必须保持原类型")
}

// TestUpgradeLongTextSubsSkipsNonTextTypes 锁定类型保护：
// 名字命中长文本清单、但用户已显式选过其它类型时，绝不被强行改回 textarea。
// （例如用户把「摘要」改成数组类型是有意为之，迁移不得覆盖。）
func TestUpgradeLongTextSubsSkipsNonTextTypes(t *testing.T) {
	subs := []types.FleetCategorySub{
		longTextSub("summary", "摘要", "array"), // 用户有意改成数组
		longTextSub("description", "说明", "date"),
		longTextSub("备注", "", "items"),
		longTextSub("remark", "备注", "number"),
	}
	got, changed := upgradeLongTextSubs(subs)
	assert.Zero(t, changed, "非 text 类型一律不得被迁移覆盖（备注类 array 除外，见下）")
	for _, s := range got {
		assert.NotEqual(t, "textarea", s.DataType, "字段 %s 的类型被意外改写", s.Name)
	}
}

// TestUpgradeLongTextSubsCoversCatalogTextareaFields 交叉校验：字段目录里被声明为
// `textarea` 的字段，其 Key/Label 必须同时进入迁移白名单。
//
// 真机事故（2026-10-22 验收）：catalog 已把 notice 的 measure(处理决定)/basis(依据)
// 标成 textarea，但白名单只收 remark/summary，于是**新建分类**的「处理决定」是多行
// 文本、**存量分类**仍是单行 —— 同一个字段按分类新旧渲染不同，正是本需求要消灭的
// 不一致（DB 复核时发现 award_punish 五个分类的 measure 全是 text）。
//
// 断言以 catalog 为准绳而非硬编码清单：以后目录新增 textarea 字段却忘了补白名单，
// 这里会直接失败。
func TestUpgradeLongTextSubsCoversCatalogTextareaFields(t *testing.T) {
	var missing []string
	for _, cat := range service.BusinessFieldCatalogs() {
		for _, f := range cat.AllFields() {
			if f.Type != "textarea" {
				continue
			}
			if !isLongTextFieldName(f.Key) && !isLongTextFieldName(f.Label) {
				missing = append(missing, cat.Scope+"."+f.Key+"("+f.Label+")")
			}
		}
	}
	assert.Empty(t, missing, "目录声明为 textarea 但迁移白名单未收录（存量分类将停在 text）：%v", missing)
}

// TestUpgradeLongTextSubsMigratesMeasureAndBasis 收口上面交叉校验的具体行为：
// 处理决定 / 依据 的存量 text 必须被升级，用户显式改过的其它类型不覆盖。
func TestUpgradeLongTextSubsMigratesMeasureAndBasis(t *testing.T) {
	subs := []types.FleetCategorySub{
		longTextSub("measure", "处理决定", "text"),
		longTextSub("basis", "依据", "text"),
		longTextSub("处理决定", "", "text"),         // 中文名直接当 Name 的形态
		longTextSub("measure", "处理决定", "number"), // 用户显式改过的类型不得覆盖
	}
	got, changed := upgradeLongTextSubs(subs)
	require.Equal(t, 3, changed, "三条纯文本方言应被升级，number 不动")
	// 按下标断言：subs 里 name=measure 出现两次，用 map 会互相覆盖。
	assert.Equal(t, "textarea", got[0].DataType, "measure(text) 应升级")
	assert.Equal(t, "textarea", got[1].DataType, "basis(text) 应升级")
	assert.Equal(t, "textarea", got[2].DataType, "中文名「处理决定」应升级")
	assert.Equal(t, "number", got[3].DataType, "用户显式选的 number 绝不能被覆盖")
}

// TestUpgradeLongTextSubsForcesRemarkArray 备注类的 array 是历史种子的错配
// （渲染成 t-tag-input，长备注被切成标签），必须纠回 textarea；但 array 的
// summary 属用户有意配置，不得连带改动。
func TestUpgradeLongTextSubsForcesRemarkArray(t *testing.T) {
	subs := []types.FleetCategorySub{
		longTextSub("remark", "备注", "array"),
		longTextSub("summary", "摘要", "array"),
	}
	got, changed := upgradeLongTextSubs(subs)
	assert.Equal(t, 1, changed, "只有备注类 array 被纠正")
	byName := map[string]string{}
	for _, s := range got {
		byName[s.Name] = s.DataType
	}
	assert.Equal(t, "textarea", byName["remark"])
	assert.Equal(t, "array", byName["summary"], "摘要数组可能是用户有意配置")
}

// TestUpgradeLongTextSubsIdempotent 迁移必须幂等：第二次执行零改动。
func TestUpgradeLongTextSubsIdempotent(t *testing.T) {
	subs := []types.FleetCategorySub{
		longTextSub("remark", "备注", ""),
		longTextSub("summary", "摘要", ""),
	}
	first, n1 := upgradeLongTextSubs(subs)
	require.Equal(t, 2, n1)
	second, n2 := upgradeLongTextSubs(first)
	assert.Zero(t, n2, "第二次迁移不得再有改动")
	assert.Equal(t, first, second, "幂等：两次结果必须一致")
}

// TestUpgradeLongTextSubsNoLongTextFields 没有长文本字段时零改动、原样返回。
func TestUpgradeLongTextSubsNoLongTextFields(t *testing.T) {
	subs := []types.FleetCategorySub{
		longTextSub("contract_no", "合同编号", ""),
		longTextSub("amount", "合同金额", "number"),
	}
	got, changed := upgradeLongTextSubs(subs)
	assert.Zero(t, changed)
	assert.Equal(t, subs, got)
}

// TestUpgradeLongTextSubsEmpty 空/ nil 输入不得 panic。
func TestUpgradeLongTextSubsEmpty(t *testing.T) {
	got, changed := upgradeLongTextSubs(nil)
	assert.Zero(t, changed)
	assert.Empty(t, got)

	got2, changed2 := upgradeLongTextSubs([]types.FleetCategorySub{})
	assert.Zero(t, changed2)
	assert.Empty(t, got2)
}

// TestIsLongTextFieldName 锁定字段名判定：key 与中文名双路、大小写不敏感。
func TestIsLongTextFieldName(t *testing.T) {
	for _, name := range []string{
		"remark", "Remark", "REMARK",
		"summary", "Summary",
		"description", "comment", "notes", "note", "memo",
		"measure", "basis",
		"备注", "摘要", "内容摘要", "说明", "概述", "评论", "注释",
	} {
		assert.True(t, isLongTextFieldName(name), "「%s」应识别为长文本字段", name)
	}
	for _, name := range []string{
		"contract_no", "amount", "sign_date", "party_a", "tags",
		"合同编号", "金额", "签订日期", "甲方", "标签", "",
	} {
		assert.False(t, isLongTextFieldName(name), "「%s」不应识别为长文本字段", name)
	}
}

// TestUpgradeExtractConfigFieldTypes 提取配置里的字段类型必须同步升级，
// 否则「字段定义」显示多行文本、「提取规则」仍显示字符串，六同步断裂。
//
// 方言陷阱（真机实际数据形态）：提取配置的类型值是 `string`（不是 `text`）——
// normalizeExtractFields 缺省填 "string"，invoiceRuleTypeFor 也把 text 归一到
// "string"。只认 `text` 会让迁移在配置侧一条都命中不了。
func TestUpgradeExtractConfigFieldTypes(t *testing.T) {
	fields := []types.ExtractFieldConfig{
		{Name: "备注", Type: "string", Enabled: true},    // 真机形态
		{Name: "summary", Type: "text", Enabled: true}, // subs 方言遗留
		{Name: "description", Type: "", Enabled: true}, // 空值按 string 兜底
		{Name: "contract_no", Type: "string", Enabled: true},
		{Name: "摘要", Type: "array", Enabled: true},      // 非文本：不动
		{Name: "remark", Type: "number", Enabled: true}, // 非文本：不动
	}
	got, changed := upgradeExtractConfigFieldTypes(fields)
	assert.Equal(t, 3, changed, "string/text/空 三种纯文本方言都要升级")
	byName := map[string]string{}
	for _, f := range got {
		byName[f.Name] = f.Type
	}
	assert.Equal(t, "textarea", byName["备注"])
	assert.Equal(t, "textarea", byName["summary"])
	assert.Equal(t, "textarea", byName["description"])
	assert.Equal(t, "string", byName["contract_no"], "非长文本字段必须保持原类型")
	assert.Equal(t, "array", byName["摘要"])
	assert.Equal(t, "number", byName["remark"])
}

// TestUpgradeExtractConfigFieldTypesByIdempotent 配置侧迁移同样必须幂等。
func TestUpgradeExtractConfigFieldTypesIdempotent(t *testing.T) {
	fields := []types.ExtractFieldConfig{{Name: "remark", Label: "备注", Type: "string", Enabled: true}}
	first, n1 := upgradeExtractConfigFieldTypes(fields)
	require.Equal(t, 1, n1)
	second, n2 := upgradeExtractConfigFieldTypes(first)
	assert.Zero(t, n2, "第二次迁移不得再有改动")
	assert.Equal(t, first, second)
}

// TestUpgradeExtractConfigFieldTypesByLabel 只看中文 Label 的自定义字段
// （Name 是英文/拼音）也要能命中。
func TestUpgradeExtractConfigFieldTypesByLabel(t *testing.T) {
	fields := []types.ExtractFieldConfig{{Name: "custom_a", Label: "备注", Type: "string", Enabled: true}}
	got, changed := upgradeExtractConfigFieldTypes(fields)
	require.Equal(t, 1, changed)
	assert.Equal(t, "textarea", got[0].Type)
}

// TestUpgradeExtractConfigFieldTypesRemarkArray 配置侧与 subs 侧同一口径：
// 备注被错配成 array 时同步纠回 textarea，避免「字段定义=多行文本、提取规则=数组」。
func TestUpgradeExtractConfigFieldTypesRemarkArray(t *testing.T) {
	fields := []types.ExtractFieldConfig{
		{Name: "remark", Label: "备注", Type: "array", Enabled: true},
		{Name: "summary", Label: "摘要", Type: "array", Enabled: true},
	}
	got, changed := upgradeExtractConfigFieldTypes(fields)
	require.Equal(t, 1, changed)
	assert.Equal(t, "textarea", got[0].Type)
	assert.Equal(t, "array", got[1].Type, "摘要数组不得被连带改写")
}
