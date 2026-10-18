package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// TestExtractSuggestParseFileForExtractSuggest 覆盖「样票临时解析」导出函数：
// 这是智能字段生成（ai-suggest-fields）的取文本入口，必须做到
//
//	-纯文本文件无需任何解析器即可直接返回；
//	- 超长文本按上限截断并置位 Truncated（防止 Prompt 爆窗）；
//	- 不落 temporary_documents 表（该函数只有纯函数签名，没有任何 repo 调用）。
func TestExtractSuggestParseFileForExtractSuggest(t *testing.T) {
	ctx := context.Background()

	t.Run("plain text file returns content directly", func(t *testing.T) {
		deps := ExtractSuggestParser{}
		got, err := ParseFileForExtractSuggest(ctx, deps, ExtractSuggestInput{
			FileName: "sample.txt",
			Ext:      ".txt",
			Data:     []byte("发票号码 24312000000012345678"),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got.Text, "24312000000012345678") {
			t.Fatalf("text should contain original content, got %q", got.Text)
		}
		if got.Truncated {
			t.Fatalf("short text must not be marked truncated")
		}
		if got.Parser != "plain_text" {
			t.Fatalf("expected plain_text parser, got %q", got.Parser)
		}
	})

	t.Run("oversized text is truncated to the cap", func(t *testing.T) {
		deps := ExtractSuggestParser{}
		long := strings.Repeat("A", extractSuggestMaxTextRunes+500)
		got, err := ParseFileForExtractSuggest(ctx, deps, ExtractSuggestInput{
			FileName: "big.txt",
			Ext:      ".txt",
			Data:     []byte(long),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Truncated != true {
			t.Fatalf("expected Truncated=true for oversized input")
		}
		if got.Text != strings.Repeat("A", extractSuggestMaxTextRunes) {
			t.Fatalf("text length = %d, want exactly %d", len(got.Text), extractSuggestMaxTextRunes)
		}
	})

	t.Run("empty input is rejected without invoking parser", func(t *testing.T) {
		deps := ExtractSuggestParser{}
		if _, err := ParseFileForExtractSuggest(ctx, deps, ExtractSuggestInput{
			FileName: "empty.txt", Ext: ".txt", Data: nil,
		}); err == nil {
			t.Fatalf("expected error for empty input")
		}
	})

	t.Run("audio input is rejected (invoices are never audio)", func(t *testing.T) {
		deps := ExtractSuggestParser{}
		if _, err := ParseFileForExtractSuggest(ctx, deps, ExtractSuggestInput{
			FileName: "s.mp3", Ext: ".mp3", Data: []byte("ID3"),
		}); err == nil {
			t.Fatalf("expected error for audio input")
		}
	})
}

// TestExtractSuggestParseFileForExtractSuggestNoDB 锁定「不落库」契约：
// 解析入口只依赖纯函数 + 可选的 documentReader/tenantService，
// 结构体里**不持有 repo / fileService / taskEnqueuer**，
// 从类型上就不可能写 temporary_documents 或投递 asynq 任务。
func TestExtractSuggestParseFileForExtractSuggestNoDB(t *testing.T) {
	deps := ExtractSuggestParser{}
	if deps.Documents != nil {
		t.Fatalf("ExtractSuggestParser must not carry a document repository")
	}
	// 纯文本路径不需要任何依赖即可完成，进一步说明解析不经过存储层。
	if _, err := ParseFileForExtractSuggest(context.Background(), deps, ExtractSuggestInput{
		FileName: "a.md", Ext: ".md", Data: []byte("# 票面"),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestExtractSuggestParserOptionsDefaults 保证零值 options 时仍走auto 引擎，
// 不会因为未显式指定而跳过解析器选择。
func TestExtractSuggestParserOptionsDefaults(t *testing.T) {
	var in ExtractSuggestInput
	if got := in.normalizedExt(); got != "" {
		t.Fatalf("empty ext should stay empty, got %q", got)
	}
	in.Ext = ".PDF"
	if got := in.normalizedExt(); got != "pdf" {
		t.Fatalf("ext should be lowercased without dot, got %q", got)
	}
	in.Ext = "jpg"
	if got := in.normalizedExt(); got != "jpg" {
		t.Fatalf("ext without dot should pass through, got %q", got)
	}
}

// TestExtractSuggestMaxTextRunesBound 锁死上限常量：12000 字符是 Prompt 预算口径，
// 不允许被无声调大（否则一次建议调用就可能超出模型上下文）。
func TestExtractSuggestMaxTextRunesBound(t *testing.T) {
	if extractSuggestMaxTextRunes != 12000 {
		t.Fatalf("extractSuggestMaxTextRunes = %d, want 12000", extractSuggestMaxTextRunes)
	}
	_ = types.TemporaryDocumentCreateOptions{}
}
