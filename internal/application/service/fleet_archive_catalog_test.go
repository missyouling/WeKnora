package service

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// 字段清单必须来自 subs（用户在界面上加的字段也要能提取），
// 规则按证照类型富化；停用字段不参与。
func TestFleetArchiveFieldsFromSubs(t *testing.T) {
	subs := []types.FleetCategorySub{
		{Name: "车牌号", Enabled: true, DataType: "text"},
		{Name: "有效期止", Enabled: true, DataType: "date"},
		{Name: "停用字段", Enabled: false, DataType: "text"},
		{Name: "用户自建字段", Enabled: true, DataType: "text"},
	}
	fields := FleetArchiveFieldsFromSubs(subs, "行驶证")
	if len(fields) != 3 {
		t.Fatalf("want 3 enabled fields, got %d", len(fields))
	}
	byName := map[string]types.ExtractFieldConfig{}
	for _, f := range fields {
		byName[f.Name] = f
	}
	// 有专门规则的字段必须带 Rule + Regex
	if byName["车牌号"].Regex == "" || byName["车牌号"].Rule == "" {
		t.Errorf("车牌号 should have rule+regex, got %+v", byName["车牌号"])
	}
	if byName["有效期止"].Type != "date" {
		t.Errorf("有效期止 type = %s, want date", byName["有效期止"].Type)
	}
	// 用户自建字段没有专门规则，但仍要出现在清单里（否则提取不到）
	if _, ok := byName["用户自建字段"]; !ok {
		t.Errorf("custom field dropped from extract fields")
	}
}

// 所有注册的正则必须是 Go RE2 可编译的（写错一个会让整条提取链路 500）。
func TestFleetArchiveRegexesCompile(t *testing.T) {
	checked := 0
	for certType, m := range fleetCertFieldRules {
		for name, r := range m {
			if r.Regex == "" {
				continue
			}
			if _, err := regexp.Compile(r.Regex); err != nil {
				t.Errorf("%s/%s regex invalid: %v (%s)", certType, name, err, r.Regex)
			}
			checked++
		}
	}
	for name, r := range fleetCommonFieldRules {
		if r.Regex == "" {
			continue
		}
		if _, err := regexp.Compile(r.Regex); err != nil {
			t.Errorf("common/%s regex invalid: %v", name, err)
		}
		checked++
	}
	if checked < 15 {
		t.Fatalf("expected a meaningful number of regexes, got %d", checked)
	}
}

// 车牌 / VIN / 身份证 三类高频正则必须真能匹配真实样本。
func TestFleetArchiveRegexMatchesSamples(t *testing.T) {
	cases := []struct{ cert, field, sample string }{
		{"道路运输证", "车牌号", "渝C81022"},
		{"行驶证", "车牌号码", "川AE75938"},
		{"车辆登记证书", "车辆识别代号", "LSVAM4180E2101234"},
		{"驾驶证", "驾驶证号", "510232197907131353"},
		{"道路运输经营许可证", "许可证号", "渝交运管许可字500227007986号"},
	}
	for _, c := range cases {
		r := FleetArchiveFieldRule(c.cert, c.field)
		if r.Regex == "" {
			t.Fatalf("%s/%s has no regex", c.cert, c.field)
		}
		re := regexp.MustCompile(r.Regex)
		if !re.MatchString(c.sample) {
			t.Errorf("%s/%s regex %q does not match sample %q", c.cert, c.field, r.Regex, c.sample)
		}
	}
}

func TestFleetArchivePromptTemplate(t *testing.T) {
	p := FleetArchivePromptTemplate("驾驶证")
	for _, must := range []string{"{{fields_schema}}", "{{document_text}}", "驾驶证", "YYYY-MM-DD"} {
		if !strings.Contains(p, must) {
			t.Errorf("prompt missing %q", must)
		}
	}
	if IsFleetArchiveScope("tire") {
		t.Errorf("tire should not be an archive scope")
	}
	if !IsFleetArchiveScope("vehicle") || !IsFleetArchiveScope("driver") || !IsFleetArchiveScope("maintain") {
		t.Errorf("archive scopes must include vehicle/driver/maintain")
	}
}

// 真实 subs 里的字段名必须能命中规则（别名表兜底），否则等于没配。
func TestFleetArchiveAliasFieldsCovered(t *testing.T) {
	cases := []struct{ cert, field string }{
		{"行驶证", "VIN码/车架号"},
		{"保险单", "交强险保单号"},
		{"保险单", "商业险保费"},
		{"一般维修", "维修日期"},
		{"合格证", "承修方"},
		{"驾驶证", "档案编号"},
		{"道路运输证", "吨（座）位"},
	}
	for _, c := range cases {
		r := FleetArchiveFieldRule(c.cert, c.field)
		if strings.TrimSpace(r.Rule) == "" {
			t.Errorf("%s/%s 未命中任何规则（别名缺失）", c.cert, c.field)
		}
	}
}
