package handler

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBusinessMergeFailedStateMetaTranslatesRateLimit 锁定「失败落库前中文化」：
// 原始错误是英文调用栈 + 裸 JSON，直接写进 custom_metadata.extract_error 后，
// 「历史记录」Tooltip 会展示一串用户看不懂的技术文本。
func TestBusinessMergeFailedStateMetaTranslatesRateLimit(t *testing.T) {
	prev := []byte(`{"kind":"invoice","invoices":[{"invoice_no":"12345678"}],"fleet_cert_type":"通行费发票","fleet_scope":"invoice"}`)
	raw := `invoice extraction failed: extract invoice fields: create chat completion: API request failed with status 429: {"error":{"code":"1302","message":"您的账户已达到速率限制，请您控制请求频率"}}`

	got := mergeFailedStateMeta(prev, "invoice", raw)
	var meta map[string]any
	if err := json.Unmarshal(got, &meta); err != nil {
		t.Fatalf("结果不是合法 JSON: %v", err)
	}
	if meta["extract_status"] != "failed" {
		t.Fatalf("extract_status = %v, want failed", meta["extract_status"])
	}
	want := "请求频率超限（已触发大模型限流保护），请稍后重试"
	if meta["extract_error"] != want {
		t.Fatalf("extract_error = %v, want %q", meta["extract_error"], want)
	}
	// 原文保留供排障，但不可混进展示字段
	if meta["extract_error_raw"] != raw {
		t.Fatalf("extract_error_raw 应保留原始错误")
	}
	if strings.Contains(meta["extract_error"].(string), "{") {
		t.Fatalf("展示字段不得残留裸 JSON: %v", meta["extract_error"])
	}
	// 必须保留已提取记录与分类标记（失败不得清空数据）
	invs, _ := meta["invoices"].([]any)
	if len(invs) != 1 {
		t.Fatalf("失败落库不得清空既有发票记录，got %d", len(invs))
	}
	if meta["fleet_cert_type"] != "通行费发票" {
		t.Fatalf("失败落库不得丢失分类标记")
	}
}

// TestBusinessMergeFailedStateMetaKeepsChinese 已中文化的错误原样保留（幂等）。
func TestBusinessMergeFailedStateMetaKeepsChinese(t *testing.T) {
	got := mergeFailedStateMeta(nil, "invoice", "提取超时，文档较大或模型响应缓慢")
	var meta map[string]any
	if err := json.Unmarshal(got, &meta); err != nil {
		t.Fatalf("结果不是合法 JSON: %v", err)
	}
	if meta["extract_error"] != "提取超时，文档较大或模型响应缓慢" {
		t.Fatalf("中文错误不应被改写，got %v", meta["extract_error"])
	}
	if _, ok := meta["extract_error_raw"]; ok {
		t.Fatalf("未发生转译时不应写入 extract_error_raw")
	}
}
