package handler

import (
	"os"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/service"
)

// ---------------------------------------------------------------------------
// 两个「智能配置」接口的**契约级回归**。
// 命名以 TestExtract 开头，命中 CI 的 -run 白名单
// （da-ci-cd.yml: TestBusiness|TestExtract|TestDailyAffairs|TestGolden）。
//
// 这里断言的是「不该发生的事」，比断言「该发生的事」更重要：
//   - 路由必须注册（漏注册是纯编译期看不出来的）；权限必须与既有配置接口一致；
//   - 两个接口的实现里**不得出现写库调用**——它们是纯建议接口，
//     一旦有人图省事顺手 Save 一下，用户的配置就被 AI 悄悄改掉了。
// ---------------------------------------------------------------------------

func TestExtractAIConfigRoutesRegistered(t *testing.T) {
	root := ".."
	entries, err := os.ReadDir(root + "/router")
	if err != nil {
		t.Fatalf("read router dir: %v", err)
	}
	var joined strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		b, rerr := os.ReadFile(root + "/router/" + e.Name())
		if rerr != nil {
			t.Fatalf("read %s: %v", e.Name(), rerr)
		}
		joined.Write(b)
	}
	src := joined.String()
	for _, route := range []string{
		`/extract-config/ai-suggest-fields`,
		`/extract-config/ai-suggest-rules`,
	} {
		if !strings.Contains(src, route) {
			t.Fatalf("route %s is not registered", route)
		}
	}
	// 权限必须与 SaveExtractConfig 同级（OwnedKBOrAdmin + KBAccessWrite），
	// 不能裸挂在 Viewer 上——建议接口会烧模型 token。
	for _, handler := range []string{"business.AISuggestFields", "business.AISuggestRules"} {
		idx := strings.Index(src, handler)
		if idx < 0 {
			t.Fatalf("handler %s not wired", handler)
		}
		// 取 handler 所在的**整行**（守卫写在 handler 之前的同一行）
		start := strings.LastIndex(src[:idx], "\n") + 1
		end := strings.Index(src[idx:], "\n")
		if end < 0 {
			end = len(src) - idx
		}
		line := strings.TrimSpace(src[start : idx+end])
		if !strings.Contains(line, "g.OwnedKBOrAdmin()") || !strings.Contains(line, `g.KBAccessWrite("id")`) {
			t.Fatalf("handler %s must require admin/write permission, got: %s", handler, line)
		}
	}
}

func TestExtractAIConfigHandlersNeverWriteDB(t *testing.T) {
	b, err := os.ReadFile("extract_config_ai.go")
	if err != nil {
		t.Fatalf("read handler file: %v", err)
	}
	src := string(b)
	// 纯建议接口的红线：任何写库调用都不允许出现。
	// 注意 h.getExtractConfig 是**只读**查询（extract_config.go 里只有 First），
	// 这里禁的是 Create/Update/Save/Delete/Exec 这类变更语句。
	for _, forbidden := range []string{
		"h.db.Create(",
		"h.db.Save(",
		"h.db.Delete(",
		"h.db.Exec(",
		"h.db.Model(",
		"SaveExtractConfig(",
		"UpdateFleetCategory(",
		"CreateFleetCategory(",
	} {
		if strings.Contains(src, forbidden) {
			t.Fatalf("ai suggest handler must not write DB, found %q", forbidden)
		}
	}
	// 也不得投递异步任务（避免"建议"在后台悄悄执行）
	for _, forbidden := range []string{"Enqueue(", "h.asynqClient."} {
		if strings.Contains(src, forbidden) {
			t.Fatalf("ai suggest handler must not enqueue tasks, found %q", forbidden)
		}
	}
}

func TestExtractAIConfigHandlerUsesSharedConstants(t *testing.T) {
	b, err := os.ReadFile("extract_config_ai.go")
	if err != nil {
		t.Fatalf("read handler file: %v", err)
	}
	src := string(b)
	// handler 不得另立文本上限口径，必须走 service 的导出常量
	if !strings.Contains(src, "service.ExtractSuggestMaxTextRunes()") {
		t.Fatalf("handler must reuse service.ExtractSuggestMaxTextRunes instead of hardcoding a limit")
	}
	// 泛用兜底文案必须委托 service 层，防与智能规则的差异化判断漂移
	ecb, err := os.ReadFile("extract_config.go")
	if err != nil {
		t.Fatalf("read extract_config.go: %v", err)
	}
	if !strings.Contains(string(ecb), "return service.InvoiceGenericRuleFor(catName)") {
		t.Fatalf("invoiceGenericRuleFor must delegate to service.InvoiceGenericRuleFor")
	}
	if service.InvoiceGenericRuleFor("普通发票") == service.InvoiceGenericRuleFor("其它票据") {
		t.Fatalf("generic rule fallback must stay category-sensitive")
	}
}

// TestExtractSuggestRegexStatusVocabulary 锁定前端依赖的 regex_status 取值集合。
// 这些字符串同时出现在 extract_suggest_ai.go（后端产出）与
// frontend/src/views/dailyAffairs/AiRuleSuggestPanel.vue（前端展示），
// 改名会让界面上出现"未知状态"而不报错——属于静默漂移，必须有测试兜住。
func TestExtractSuggestRegexStatusVocabulary(t *testing.T) {
	all := []string{
		service.RegexStatusAccepted(),
		service.RegexRejectNotStandard(),
		service.RegexRejectCompile(),
		service.RegexRejectNotUnique(),
		service.RegexRejectNoSample(),
		service.RegexRejectTooBroad(),
		service.RegexRejectEmpty(),
	}
	want := map[string]bool{
		"accepted": true, "rejected_not_standard": true, "rejected_compile": true,
		"rejected_not_unique": true, "rejected_no_sample": true,
		"rejected_too_broad": true, "rejected_empty": true,
	}
	seen := map[string]bool{}
	for _, s := range all {
		if s == "" {
			t.Fatalf("empty regex status is not allowed")
		}
		if !want[s] {
			t.Fatalf("unexpected regex status %q", s)
		}
		if seen[s] {
			t.Fatalf("duplicated regex status %q", s)
		}
		seen[s] = true
	}
	if len(seen) != len(want) {
		t.Fatalf("expected %d distinct statuses, got %d: %v", len(want), len(seen), seen)
	}
}
