package handler

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/application/service"
)

// 「账单概况」必须严格是这 20 个业务字段，顺序即列表列顺序。
//
// 为什么钉死：上一轮把两套字典（旧 overview 27 项 + 目录 20 项）合并进 subs，
// 结果「账单概况」膨胀成 34 个字段，且 bill_period_start 与派生 bill_period
// 都叫「账单周期」——列表里出现两列同名，第一列还被截断成 2026-09-0...。
var wantOverviewFields = []struct {
	Key   string
	Label string
}{
	{"bill_period", "账单周期"},
	{"account_no", "户号"},
	{"account_name", "户名"},
	{"usage_category", "用电类别"},
	{"voltage_level", "电压等级"},
	{"market_attr", "市场化属性"},
	{"supply_unit", "供电服务单位"},
	{"address", "用电地址"},
	{"meter_no", "电能表编号"},
	{"total_kwh", "本期电量"},
	{"total_amount", "本期电费"},
	{"prev_kwh", "上期电量"},
	{"mom_change", "环比"},
	{"avg_price", "平均电价"},
	{"power_factor", "功率因素"},
	{"due_date", "交费截止"},
	{"industrial_amount", "工商业电费"},
	{"residential_amount", "居民电费"},
	{"pf_adjust_amount", "力调电费"},
	{"grand_total", "合计"},
}

func TestUtilityOverviewHasExactlyTwentyFields(t *testing.T) {
	fields := electricityDefaultGroupFields("overview")
	if len(fields) != len(wantOverviewFields) {
		t.Fatalf("账单概况应为 %d 个字段，实际 %d 个：%v", len(wantOverviewFields), len(fields), keysOfUtilityFields(fields))
	}
	for i, want := range wantOverviewFields {
		if fields[i].key != want.Key {
			t.Errorf("第 %d 个字段应为 %q，实际 %q", i+1, want.Key, fields[i].key)
		}
		if fields[i].label != want.Label {
			t.Errorf("字段 %q 的显示名应为 %q，实际 %q", want.Key, want.Label, fields[i].label)
		}
	}
}

func TestUtilityOverviewHasNoDuplicateBillingPeriod(t *testing.T) {
	fields := electricityDefaultGroupFields("overview")
	seen := map[string]bool{}
	billingPeriodCount := 0
	for _, f := range fields {
		if seen[f.key] {
			t.Fatalf("账单概况出现重复字段 %q", f.key)
		}
		seen[f.key] = true
		if f.label == "账单周期" {
			billingPeriodCount++
		}
	}
	if billingPeriodCount != 1 {
		t.Fatalf("「账单周期」必须唯一，实际 %d 个", billingPeriodCount)
	}
	// 起止日不得再作为独立列出现（它们是派生 period 的原料，只进详情）
	for _, k := range []string{"bill_period_start", "bill_period_end"} {
		if seen[k] {
			t.Fatalf("账单概况不应包含 %q：它会让列表出现第二个「账单周期」列", k)
		}
	}
}

// 六同步：字段定义（subs 默认值）与提取规则（字段目录）必须逐项一致。
func TestUtilityOverviewMatchesCatalog(t *testing.T) {
	cat, ok := service.BusinessFieldCatalogOf("utility")
	if !ok {
		t.Fatal("utility 目录未注册")
	}
	if len(cat.Standard) < len(wantOverviewFields) {
		t.Fatalf("目录标准字段少于 20（%d）", len(cat.Standard))
	}
	for i, want := range wantOverviewFields {
		spec := cat.Standard[i]
		if spec.Key != want.Key {
			t.Errorf("目录第 %d 个字段应为 %q，实际 %q（字段定义与提取规则漂移）", i+1, want.Key, spec.Key)
		}
		if spec.Label != want.Label {
			t.Errorf("目录字段 %q 显示名应为 %q，实际 %q", want.Key, want.Label, spec.Label)
		}
	}
}

func keysOfUtilityFields(fields []utilityFieldDef) []string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, f.key)
	}
	return out
}
