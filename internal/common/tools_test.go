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
