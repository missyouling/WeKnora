package handler

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBusinessMergeFailedStateMetaTranslatesRateLimit 锁定「失败落库前中文化」：
// 原始错误是英文调用栈 + 裸 JSON，直接写进 custom_metadata.extract_error 后，
// 「历史记录」Tooltip 会展示一串用户看不懂的技术文本。
//
// 第 25 轮补充：**已有可用发票行时失败不降级状态**（问题 1 的真根因）。
// 真机 74 份文件的发票早已进主列表，一次重试撞 429 就把文档状态刷成 failed，
// 于是列表绿「提取成功」、抽屉红「提取失败」。现在错误只记 last_extract_error。
func TestBusinessMergeFailedStateMetaTranslatesRateLimit(t *testing.T) {
	// prev 带一张可用发票 → 属于「重试失败但已有成果」的场景
	prev := []byte(`{"kind":"invoice","invoices":[{"invoice_no":"12345678"}],"fleet_cert_type":"通行费发票","fleet_scope":"invoice"}`)
	raw := `invoice extraction failed: extract invoice fields: create chat completion: API request failed with status 429: {"error":{"code":"1302","message":"您的账户已达到速率限制，请您控制请求频率"}}`

	got := mergeFailedStateMeta(prev, "invoice", raw)
	var meta map[string]any
	if err := json.Unmarshal(got, &meta); err != nil {
		t.Fatalf("结果不是合法 JSON: %v", err)
	}
	// 已有可用记录 → 状态不得降级（否则列表与抽屉自相矛盾）
	if meta["extract_status"] != "success" {
		t.Fatalf("已有可用发票行时状态不得降级，got %v", meta["extract_status"])
	}
	want := "请求频率超限（已触发大模型限流保护），请稍后重试"
	if meta["last_extract_error"] != want {
		t.Fatalf("中文化后的错误应记入 last_extract_error，got %v", meta["last_extract_error"])
	}
	// 展示字段 extract_error 不得被写入（否则详情抽屉会闪红）
	if _, ok := meta["extract_error"]; ok {
		t.Fatalf("已有成果时不得写 extract_error（会让抽屉显示提取失败），got %v", meta["extract_error"])
	}
	// 原文保留供排障，但不可混进展示字段
	if meta["last_extract_error_raw"] != raw {
		t.Fatalf("last_extract_error_raw 应保留原始错误")
	}
	if strings.Contains(want, "{") {
		t.Fatalf("展示字段不得残留裸 JSON: %v", want)
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

// TestBusinessMergeFailedStateMetaFailsWhenNothingExtracted 没有可用记录时必须
// 落 failed —— 否则文件会永远停在「提取中」而无法重试。
func TestBusinessMergeFailedStateMetaFailsWhenNothingExtracted(t *testing.T) {
	for _, prev := range [][]byte{
		nil,
		[]byte(`{"kind":"invoice","invoices":[]}`),
		[]byte(`{"kind":"invoice","invoices":[{"invoice_no":"","amount":0}]}`),
	} {
		got := mergeFailedStateMeta(prev, "invoice", "模型返回为空，未识别到发票内容")
		var meta map[string]any
		if err := json.Unmarshal(got, &meta); err != nil {
			t.Fatalf("结果不是合法 JSON: %v", err)
		}
		if meta["extract_status"] != "failed" {
			t.Fatalf("无可用记录时必须落 failed（prev=%s），got %v", string(prev), meta["extract_status"])
		}
		// 不硬编码具体文案：FormatExtractErrorMessage 会把若干已知错误
		// 规范化成标准中文短句，这里只锁「有可展示的中文错误且不含裸 JSON」。
		msg, _ := meta["extract_error"].(string)
		if strings.TrimSpace(msg) == "" || strings.Contains(msg, "{") {
			t.Fatalf("extract_error 应为可展示的中文短句，got %v", meta["extract_error"])
		}
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
