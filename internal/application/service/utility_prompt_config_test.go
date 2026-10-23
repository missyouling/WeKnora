package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// P1 提取管线打通的验收口径：
//  1. cfg == nil 时 Prompt 必须与改造前**逐字节一致**（兼容铁律）；
//  2. cfg 非空时按「高级模板 / 内置底座 + 字段规则追加」三段式拼装；
//  3. 自定义字段必须能落进 Fields 动态桶（否则"配了也能提"不成立）。

func TestBuildUtilityBillSystemPromptNilUsesBuiltinVerbatim(t *testing.T) {
	got := BuildUtilityBillSystemPrompt(nil)
	if got != utilityBillExtractionSystemPrompt {
		t.Fatalf("cfg=nil 必须原样返回内置 Prompt（逐字节相等），实际长度 %d vs 内置 %d",
			len(got), len(utilityBillExtractionSystemPrompt))
	}
}

func TestBuildSolarBillSystemPromptNilUsesBuiltinVerbatim(t *testing.T) {
	got := BuildSolarBillSystemPrompt(nil)
	if got != solarBillExtractionSystemPrompt {
		t.Fatalf("cfg=nil 必须原样返回内置光伏 Prompt，实际长度 %d vs 内置 %d",
			len(got), len(solarBillExtractionSystemPrompt))
	}
}

func TestBuildUtilityBillSystemPromptAppendsConfiguredFieldRules(t *testing.T) {
	cfg := &types.KbExtractConfig{
		Fields: []types.ExtractFieldConfig{
			{Name: "account_no", Label: "户号", Type: "string", Enabled: true,
				Desc: "用电户号", Rule: "提取户号栏位，只取数字"},
			{Name: "保证金", Label: "保证金", Type: "string", Enabled: true,
				Rule: "提取账单上的保证金金额"},
			{Name: "ignored", Enabled: true},                       // 无 rule 无 desc → 不注入
			{Name: "disabled_field", Enabled: false, Rule: "不该出现"}, // 禁用 → 不注入
		},
	}
	got := BuildUtilityBillSystemPrompt(cfg)

	// 必须以内置 Prompt 为底座（保住 32 字段多表解析能力）。
	// 注意：有自定义字段时前面会加「最高优先级」前缀，所以断言「包含」而非「前缀」。
	// 注意：自定义字段会注入到内置示例里，所以不能再断言「含完整原文」，
	// 改为断言内置 Prompt 的标志性开头仍在（证明底座没被整体替换）。
	if !strings.Contains(got, "你是一个专业的电费账单信息提取助手") {
		t.Fatal("未开启高级模式时必须以内置 Prompt 为底座，不得整体替换")
	}
	// 自定义字段必须前置（后置指令实测无效，模型只照抄示例）
	if !strings.HasPrefix(got, "【最高优先级") {
		t.Fatal("自定义字段指令必须前置")
	}
	// 字段名还必须注入示例本身，否则模型不会输出该键
	if !strings.Contains(got, `"fields":{"保证金":""}`) {
		t.Fatal("自定义字段必须注入底座 JSON 示例")
	}
	if !strings.Contains(got, "【字段提取规则") {
		t.Fatal("必须追加字段提取规则段落")
	}
	if !strings.Contains(got, "提取户号栏位，只取数字") {
		t.Fatal("配置的 rule 必须进 Prompt")
	}
	if !strings.Contains(got, "保证金") || !strings.Contains(got, "自定义字段") {
		t.Fatal("动态字段必须标注「自定义字段」并提示落到 fields")
	}
	if strings.Contains(got, "ignored") {
		t.Fatal("没配 rule/desc 的字段不应注入 Prompt（稀释注意力）")
	}
	if strings.Contains(got, "不该出现") {
		t.Fatal("禁用字段不得进 Prompt")
	}
	// 收口指令必须在
	if !strings.Contains(got, `"fields"`) {
		t.Fatal("必须包含自定义字段输出收口指令")
	}
}

func TestBuildUtilityBillSystemPromptAdvancedTemplateReplacesBase(t *testing.T) {
	cfg := &types.KbExtractConfig{
		AdvancedEnabled: true,
		PromptTemplate:  "自定义模板：请提取电费账单。",
		Fields: []types.ExtractFieldConfig{
			{Name: "保证金", Enabled: true, Rule: "提取保证金"},
		},
	}
	got := BuildUtilityBillSystemPrompt(cfg)
	if !strings.Contains(got, "自定义模板：请提取电费账单。") {
		t.Fatal("高级模式必须用用户模板作为底座")
	}
	if strings.Contains(got, utilityBillExtractionSystemPrompt[:80]) {
		t.Fatal("高级模式下不得再叠加内置底座（会产生互相矛盾的指令）")
	}
	if !strings.Contains(got, "提取保证金") {
		t.Fatal("高级模式下仍应追加字段规则")
	}
}

func TestBuildUtilityBillSystemPromptAdvancedWithoutTemplateFallsBack(t *testing.T) {
	// 开了高级模式但模板留空：不能退化成空 Prompt
	cfg := &types.KbExtractConfig{AdvancedEnabled: true, PromptTemplate: "   "}
	got := BuildUtilityBillSystemPrompt(cfg)
	if !strings.Contains(got, strings.TrimRight(utilityBillExtractionSystemPrompt, "\n")) {
		t.Fatal("高级模式模板为空时必须回退内置底座")
	}
}

func TestBuildConfiguredFieldRulesNoFieldsReturnsEmpty(t *testing.T) {
	if got := buildConfiguredFieldRules(&types.KbExtractConfig{}); got != "" {
		t.Fatalf("无字段时应返回空串，实际 %q", got)
	}
	if got := buildConfiguredFieldRules(nil); got != "" {
		t.Fatalf("cfg 为 nil 时应返回空串，实际 %q", got)
	}
}

// ---- 动态字段桶 ----

func TestCollectUtilityBillCustomFieldsCapturesTopLevelAndNested(t *testing.T) {
	raw := `{
		"account_no": "5001654065328",
		"total_kwh": 596168,
		"保证金": "10000",
		"fields": {"用电地址": "XX路1号"}
	}`
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		t.Fatal(err)
	}
	item := &types.UtilityBillExtractionItem{}
	CollectUtilityBillCustomFields(item, obj)

	if item.Fields == nil {
		t.Fatal("自定义字段必须被收集进 Fields")
	}
	// 顶层未知键
	if item.Fields["保证金"] != "10000" {
		t.Fatalf("顶层未知键未收集：%#v", item.Fields["保证金"])
	}
	// 显式 fields 子对象
	if item.Fields["用电地址"] != "XX路1号" {
		t.Fatalf("fields 子对象未展开：%#v", item.Fields["用电地址"])
	}
	// 标准字段不得混进动态桶
	if _, ok := item.Fields["account_no"]; ok {
		t.Fatal("标准字段不应进动态桶（否则列表会出现重复列）")
	}
	if _, ok := item.Fields["total_kwh"]; ok {
		t.Fatal("标准字段不应进动态桶")
	}
}

func TestCollectUtilityBillCustomFieldsKeepsNilWhenNoCustom(t *testing.T) {
	raw := `{"account_no":"123","total_kwh":100}`
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		t.Fatal(err)
	}
	item := &types.UtilityBillExtractionItem{}
	CollectUtilityBillCustomFields(item, obj)
	// 空 map 或 nil 都可接受，但不能凭空塞标准字段
	for k := range item.Fields {
		t.Fatalf("不应产出任何动态字段，却得到 %q", k)
	}
}

func TestCollectSolarBillCustomFields(t *testing.T) {
	raw := `{"account_no":"123","补贴金额":"888.88"}`
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		t.Fatal(err)
	}
	item := &types.SolarBillExtractionItem{}
	CollectSolarBillCustomFields(item, obj)
	if item.Fields["补贴金额"] != "888.88" {
		t.Fatalf("光伏自定义字段未收集：%#v", item.Fields)
	}
	if _, ok := item.Fields["account_no"]; ok {
		t.Fatal("标准字段不应进动态桶")
	}
}

// ---- 字段目录与 schema 注册 ----

func TestUtilityAndSolarCatalogsRegistered(t *testing.T) {
	for _, scope := range []string{"utility", "solar"} {
		cat, ok := BusinessFieldCatalogOf(scope)
		if !ok {
			t.Fatalf("%s 必须注册字段目录（否则正则补洞无面可补）", scope)
		}
		if len(cat.Standard) < 15 {
			t.Fatalf("%s 标准字段过少（%d），账单关键字段缺失", scope, len(cat.Standard))
		}
		// 每个字段都要有 desc 与 rule，否则「提取规则」页会出现空规则
		for _, f := range cat.Standard {
			if strings.TrimSpace(f.Desc) == "" {
				t.Fatalf("%s.%s 缺 desc", scope, f.Key)
			}
			if strings.TrimSpace(f.Rule) == "" && f.Key != "remark" {
				t.Fatalf("%s.%s 缺 rule", scope, f.Key)
			}
		}
	}
}

func TestUtilitySolarSchemasAreRegexPatchable(t *testing.T) {
	for _, scope := range []string{"utility", "solar", "energy"} {
		s, ok := GetScopeSchema(scope)
		if !ok {
			t.Fatalf("%s schema 未注册", scope)
		}
		if !s.RegexPatchable {
			t.Fatalf("%s 必须开启 RegexPatchable（否则 ValidateGenericExtraction 的正则补洞静默失效）", scope)
		}
		if len(s.StandardFields) == 0 {
			t.Fatalf("%s 必须有 StandardFields（否则补洞无字段可补）", scope)
		}
	}
}

// ---- 关键键名对齐：目录 Key 必须等于结构体 json tag ----

func TestUtilityCatalogKeysMatchStructJSONTags(t *testing.T) {
	std := structJSONKeys(types.UtilityBillExtractionItem{})
	cat, _ := BusinessFieldCatalogOf("utility")
	for _, f := range cat.Standard {
		// 只校验**配了正则**的字段：正则补洞按字段名匹配记录，名字对不上会静默失效。
		// bill_period / *_amount 这类是前端派生列（拼接值、按类别汇总），
		// 不参与正则补洞，也不需要是结构体字段。
		if strings.TrimSpace(f.Regex) == "" {
			continue
		}
		if !std[f.Key] {
			t.Fatalf("utility 目录键 %q 配了正则但不在结构体 json tag 里——补洞会静默失效", f.Key)
		}
	}
}

func TestSolarCatalogKeysMatchStructJSONTags(t *testing.T) {
	std := structJSONKeys(types.SolarBillExtractionItem{})
	cat, _ := BusinessFieldCatalogOf("solar")
	for _, f := range cat.Standard {
		if strings.TrimSpace(f.Regex) == "" {
			continue
		}
		if !std[f.Key] {
			t.Fatalf("solar 目录键 %q 配了正则但不在结构体 json tag 里", f.Key)
		}
	}
}

func TestDefaultPromptTemplatesNonEmpty(t *testing.T) {
	if n := len(UtilityBillDefaultPromptTemplate()); n < 10000 {
		t.Fatalf("电费默认模板过短（%d），应为完整的 1.6 万字国网 Prompt", n)
	}
	if n := len(SolarBillDefaultPromptTemplate()); n < 3000 {
		t.Fatalf("光伏默认模板过短（%d）", n)
	}
}
