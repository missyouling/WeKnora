package handler

import "testing"

// 分类双键归一（第 23 轮 issue 2/3）。
//
// 这条规则的存在理由：分类有两个键（invoice_category 新权威 / fleet_cert_type 旧键），
// 读取侧优先新键、写入侧历史只更新旧键 —— 于是「抽屉里改了类型、列表还挂在原分类」。
// 归一函数把两个键强制同步，且**只在本次请求显式提到分类时才动手**
// （用户在改金额/开票人时不能被顺手改分类）。

func TestNormalizeInvoiceCategoryKeysNewWins(t *testing.T) {
	meta := map[string]interface{}{
		"invoice_category": "通行费发票",
		"fleet_cert_type":  "能耗发票",
	}
	if got := normalizeInvoiceCategoryKeys(meta); got != "通行费发票" {
		t.Fatalf("应以 invoice_category 为权威，实际 %q", got)
	}
	if meta["fleet_cert_type"] != "通行费发票" {
		t.Fatalf("旧键必须被同步为新值，实际 %v", meta["fleet_cert_type"])
	}
}

func TestNormalizeInvoiceCategoryKeysOnlyOldKey(t *testing.T) {
	// 历史文件只有旧键：把新键补齐（否则读取侧读到空、落到排除桶）
	meta := map[string]interface{}{"fleet_cert_type": "其它票据"}
	if got := normalizeInvoiceCategoryKeys(meta); got != "其它票据" {
		t.Fatalf("只有一个键时以它为权威，实际 %q", got)
	}
	if meta["invoice_category"] != "其它票据" {
		t.Fatalf("新键必须被补齐，实际 %v", meta["invoice_category"])
	}
}

func TestNormalizeInvoiceCategoryKeysOnlyNewKey(t *testing.T) {
	meta := map[string]interface{}{"invoice_category": "能耗发票"}
	if got := normalizeInvoiceCategoryKeys(meta); got != "能耗发票" {
		t.Fatalf("只有一个键时以它为权威，实际 %q", got)
	}
	if meta["fleet_cert_type"] != "能耗发票" {
		t.Fatalf("旧键必须被补齐，实际 %v", meta["fleet_cert_type"])
	}
}

func TestNormalizeInvoiceCategoryKeysExplicitClear(t *testing.T) {
	// 用户清空类型（两个键都显式给空串）：两个键都要清空，不能留旧值
	meta := map[string]interface{}{
		"invoice_category": "",
		"fleet_cert_type":  "",
	}
	if got := normalizeInvoiceCategoryKeys(meta); got != "" {
		t.Fatalf("显式清空时应返回空串，实际 %q", got)
	}
}

func TestNormalizeInvoiceCategoryKeysUntouchedWhenAbsent(t *testing.T) {
	// 本次请求没提分类（用户在改金额）→ 不得动手，调用方也不落库
	meta := map[string]interface{}{"kind": "invoice", "invoices": []interface{}{}}
	if got := normalizeInvoiceCategoryKeys(meta); got != "" {
		t.Fatalf("未提及分类时应返回空串（表示无需处理），实际 %q", got)
	}
	if _, exists := meta["invoice_category"]; exists {
		t.Fatal("未提及分类时不得凭空写入 invoice_category")
	}
	if _, exists := meta["fleet_cert_type"]; exists {
		t.Fatal("未提及分类时不得凭空写入 fleet_cert_type")
	}
}

func TestNormalizeInvoiceCategoryKeysNilMeta(t *testing.T) {
	if got := normalizeInvoiceCategoryKeys(nil); got != "" {
		t.Fatalf("nil meta 应返回空串，实际 %q", got)
	}
}
