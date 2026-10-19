package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 本组测试锁定「上传所选发票分类 = 唯一权威分类」铁律。
//
// 真实事故（VM 库 2026-10-19 04:16 同批上传 5 个文件）：
//   - 3 个文件拿到 fleet_cert_type=通行费发票，逐票 category=通行费发票（正确）；
//   - 2 个文件 fleet_cert_type 为空、逐票 category 为空串 → 落进「其它票据」。
//
// 根因：分类是 parse 完成后由前端**另发一次 PUT** 写入的，而提取流程落库时
// `SaveInvoiceCustomMetadata` 会把 custom_metadata **整列替换**成提取结果 JSON
// （不含 fleet_cert_type）。谁写在后谁赢 → 交替丢失。
// 更兼 `invoice_category` 这个字段**从来没有人写过**。

// TestExtractMergePreservesUserCategoryWhenFreshOmitsIt 锁定核心不变量：
// 提取结果不含分类键时，必须从库中现值保留，绝不能被整列覆盖冲掉。
func TestExtractMergePreservesUserCategoryWhenFreshOmitsIt(t *testing.T) {
	existing := mustJSON(t, map[string]interface{}{
		"kind":               "invoice",
		"invoice_category":   "通行费发票",
		"fleet_cert_type":    "通行费发票",
		"fleet_scope":        "invoice",
		"auto_deleted_count": 1,
	})
	// 提取结果的真实形状：只有 kind/invoices/extract_status，没有分类键
	fresh := mustJSON(t, map[string]interface{}{
		"kind":           "invoice",
		"extract_status": "success",
		"invoices": []interface{}{
			map[string]interface{}{"invoice_no": "49367445", "category": "", "invoice_type": ""},
		},
	})

	merged := mergeInvoiceMetaPreservingUserKeys(existing, fresh)
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(merged, &got))

	assert.Equal(t, "通行费发票", got["invoice_category"], "上传所选分类不得被提取结果冲掉")
	assert.Equal(t, "通行费发票", got["fleet_cert_type"], "文件级打标不得被提取结果冲掉")
	assert.Equal(t, "invoice", got["fleet_scope"])
	assert.EqualValues(t, 1, got["auto_deleted_count"], "删除历史计数不得被冲掉")
	assert.Equal(t, "success", got["extract_status"], "提取状态必须用新值")
	assert.NotEmpty(t, got["invoices"])
}

// TestExtractMergeExplicitCategoryUpdateWins：用户显式改分类时，新值必须生效
// （否则「其它票据 → 通行费发票」的人工纠正永远存不下来）。
func TestExtractMergeExplicitCategoryUpdateWins(t *testing.T) {
	existing := mustJSON(t, map[string]interface{}{
		"kind": "invoice", "invoice_category": "其它票据", "fleet_cert_type": "其它票据",
	})
	fresh := mustJSON(t, map[string]interface{}{
		"kind": "invoice", "invoice_category": "通行费发票", "fleet_cert_type": "通行费发票",
	})
	merged := mergeInvoiceMetaPreservingUserKeys(existing, fresh)
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(merged, &got))
	assert.Equal(t, "通行费发票", got["invoice_category"], "显式更新必须覆盖旧分类")
	assert.Equal(t, "通行费发票", got["fleet_cert_type"])
}

// TestExtractMergeBrokenExistingFallsBackToFresh：库中 metadata 损坏时不得报错，
// 直接用新值（宁可丢分类也不能让整次提取失败）。
func TestExtractMergeBrokenExistingFallsBackToFresh(t *testing.T) {
	fresh := mustJSON(t, map[string]interface{}{"kind": "invoice", "extract_status": "success"})
	merged := mergeInvoiceMetaPreservingUserKeys([]byte("{not json"), fresh)
	assert.JSONEq(t, string(fresh), string(merged))
}

// TestExtractResolveCertTypePreference 锁定分类来源优先级：
// 请求入参 > invoice_category（上传即持久化的权威值）> fleet_cert_type（旧字段）。
func TestExtractResolveCertTypePreference(t *testing.T) {
	cases := []struct {
		name                     string
		request, invoiceCategory string
		fleetCert                string
		want                     string
	}{
		{"请求入参最高优先", "水电发票", "通行费发票", "通行费发票", "水电发票"},
		{"invoice_category 优先于旧字段", "", "通行费发票", "其它票据", "通行费发票"},
		{"只有旧字段时回退旧字段", "", "", "通行费发票", "通行费发票"},
		{"全空返回空", "", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, resolveInvoiceCertType(c.request, c.invoiceCategory, c.fleetCert))
		})
	}
}

// TestExtractCategoryLockForcesInvoiceTypeAndCategory 锁定「提取严禁篡改」：
// certType 非空时，每张票的 invoice_type 与 category 都必须被强制写成该分类，
// 模型返回的任何值都不得生效（含空值/其它票据）。
func TestExtractInvoiceCategoryLockApplied(t *testing.T) {
	res := &InvoiceExtractionResult{Kind: "invoice", Invoices: []InvoiceExtractionItem{
		{InvoiceNo: "1", InvoiceType: "普通发票", Category: "其它票据"},
		{InvoiceNo: "2", InvoiceType: "", Category: ""},
		{InvoiceNo: "3", InvoiceType: "专用发票", Category: ""},
	}}
	LockInvoiceCategory(res, "通行费发票")
	for i, inv := range res.Invoices {
		assert.Equal(t, "通行费发票", inv.InvoiceType, "第 %d 张 invoice_type 必须锁定", i+1)
		assert.Equal(t, "通行费发票", inv.Category, "第 %d 张 category 必须锁定", i+1)
	}

	// 空 certType 不得做任何改动（保持既有语义）
	untouched := &InvoiceExtractionResult{Kind: "invoice", Invoices: []InvoiceExtractionItem{
		{InvoiceNo: "1", InvoiceType: "普通发票", Category: "其它票据"},
	}}
	LockInvoiceCategory(untouched, "")
	assert.Equal(t, "普通发票", untouched.Invoices[0].InvoiceType)
	assert.Equal(t, "其它票据", untouched.Invoices[0].Category)
}
