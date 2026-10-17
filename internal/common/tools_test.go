package common

import (
	"testing"
	"unicode/utf8"
)

func TestParseLLMJsonResponse(t *testing.T) {
	type payload struct {
		Key string `json:"key"`
	}

	tests := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{"direct json", `{"key":"value"}`, "value", false},
		{"fenced json", "```json\n{\"key\":\"fenced\"}\n```", "fenced", false},
		{"fenced no lang", "```\n{\"key\":\"plain\"}\n```", "plain", false},
		{"prose around fence", "Here you go:\n```json\n{\"key\":\"wrapped\"}\n```\nThanks", "wrapped", false},
		{"prose around bare json", `Sure: {"key":"bare"} hope that helps`, "bare", false},
		// Trailing bracket-like prose must not truncate the payload.
		{"trailing citation", `{"key":"cited"} — see section [1].`, "cited", false},
		// Braces inside a string literal must not close the object early.
		{"braces in string", `Result: {"key":"a}b"} done`, "a}b", false},
		// MaxTokens 截断场景：反引号开头但围栏未闭合、对象被截断。
		{"unclosed fence prefix", "```json\n{\"key\":\"fence\"}", "fence", false},
		{"unclosed fence truncated", "```json\n{\"key\":\"trunc", "trunc", false},
		{"bare backtick prefix", "`{\"key\":\"tick\"}", "tick", false},
		{"truncated object", `{"key":"trunc`, "trunc", false},
		{"truncated array", `{"key":["a","b`, "", true},
		{"truncated array of str", `{"key":["a","b"`, "", true},
		{"truncated nested partial", `{"key":"x","sub":{"a":"b`, "x", false},
		{"truncated string keeps prose", `Sure: {"key":"trunc} done`, "trunc} done", false},
		{"bare open brace", `{`, "", false},
		{"not json", "just some text", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got payload
			err := ParseLLMJsonResponse(tt.content, &got)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got.Key != tt.want {
				t.Errorf("Key = %q, want %q", got.Key, tt.want)
			}
		})
	}
}

func TestCleanInvalidUTF8(t *testing.T) {
	input := "valid " + string([]byte{0xef, 0xbc, 0x2e}) + "\x00tail"

	got := CleanInvalidUTF8(input)
	if !utf8.ValidString(got) {
		t.Fatalf("CleanInvalidUTF8 returned invalid UTF-8: % x", []byte(got))
	}
	if want := "valid .tail"; got != want {
		t.Fatalf("CleanInvalidUTF8() = %q, want %q", got, want)
	}
}

func BenchmarkParseLLMJsonResponse_Fenced(b *testing.B) {
	const content = "```json\n{\"name\":\"Acme\",\"slug\":\"entity/acme\",\"aliases\":[\"A\",\"B\"]}\n```"
	type payload struct {
		Name string `json:"name"`
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var p payload
		_ = ParseLLMJsonResponse(content, &p)
	}
}

// ---- Phase A：思考模型兼容与脏 JSON 容错 ----

func TestStripReasoningBlocks(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"闭合 think 整体移除", `<think>{"draft":1}</think>{"real":2}`, `{"real":2}`},
		{"闭合 thought 整体移除", "<thought>hmm</thought>\n{\"real\":2}", "{\"real\":2}"},
		{"未闭合 think 切到 JSON 起始", `<think>让我想想……{"real":2}`, `{"real":2}`},
		{"未闭合 think 切到围栏", "<think>思考中\n```json\n{\"real\":2}", "```json\n{\"real\":2}"},
		{"无 think 原样返回", `{"real":2}`, `{"real":2}`},
		{"全文都在 think 内保留原串", `<think>只有思考，没有 JSON`, `<think>只有思考，没有 JSON`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripReasoningBlocks(tt.in); got != tt.want {
				t.Errorf("StripReasoningBlocks(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// 思考模型混合输出：草稿 JSON 在 <think> 内、正式结果在围栏内且带尾逗号。
// 若不做剥离，ExtractBalancedJSON 可能抓到草稿 {"wrong":1}。
func TestParseLLMJsonResponseStripsThinkAndTrailingComma(t *testing.T) {
	type payload struct {
		Right int `json:"right"`
		Wrong int `json:"wrong"`
	}
	content := "<think>{\"wrong\": 1}</think>\n```json\n{\"right\": 2,}\n```"
	var got payload
	if err := ParseLLMJsonResponse(content, &got); err != nil {
		t.Fatalf("应能解析，实际报错: %v", err)
	}
	if got.Right != 2 {
		t.Errorf("right = %d, want 2", got.Right)
	}
	if got.Wrong != 0 {
		t.Errorf("不应采用 <think> 内的草稿：wrong = %d", got.Wrong)
	}
}

// 千分位/货币符号金额必须能解析为浮点；长编码（发票号/税号）必须原样保留，
// 不能被转成裸数字丢精度。
func TestParseLLMJsonResponseSanitizesMoneyKeepsCodes(t *testing.T) {
	type payload struct {
		Amount    float64 `json:"amount"`
		Tax       float64 `json:"tax"`
		InvoiceNo string  `json:"invoice_no"`
		Code      string  `json:"code"`
	}
	content := `{"amount": "3,258.99", "tax": "¥93.32", "invoice_no": "12345678901234567890", "code": "050002400112"}`
	var got payload
	if err := ParseLLMJsonResponse(content, &got); err != nil {
		t.Fatalf("应能解析，实际报错: %v", err)
	}
	if got.Amount != 3258.99 {
		t.Errorf("amount = %v, want 3258.99", got.Amount)
	}
	if got.Tax != 93.32 {
		t.Errorf("tax = %v, want 93.32", got.Tax)
	}
	if got.InvoiceNo != "12345678901234567890" {
		t.Errorf("20 位发票号被破坏: %q", got.InvoiceNo)
	}
	if got.Code != "050002400112" {
		t.Errorf("发票代码被破坏: %q", got.Code)
	}
}

func TestSanitizeLooseJSONKeepsNormalText(t *testing.T) {
	// 正常文本里的逗号、中文说明不得被改动。
	in := `{"name": "重庆, 中国", "note": "金额 3,258.99 元"}`
	if got := SanitizeLooseJSON(in); got != in {
		t.Errorf("非数值字符串不应被改写:\n in=%s\nout=%s", in, got)
	}
}

// 尾逗号修复必须只作用于结构位置，不能改到字符串内容。
func TestSanitizeLooseJSONKeepsCommaBraceInText(t *testing.T) {
	in := `{"note": "逗号,}", "list": [1,2,],}`
	want := `{"note": "逗号,}", "list": [1,2]}`
	if got := SanitizeLooseJSON(in); got != want {
		t.Errorf("SanitizeLooseJSON:/n in=%s\nout=%s\nwant=%s", in, got, want)
	}
}
