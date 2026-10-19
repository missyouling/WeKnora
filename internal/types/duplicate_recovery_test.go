package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 上传去重冲突的「智能恢复」判定：
// 文件已在库但 invoices 为空 / 提取失败 → 可自动重提取（CanReextract）；
// 已有正常票据 → 只提示，不自动覆盖（Normal）。

func metaJSON(t *testing.T, v interface{}) JSON {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return JSON(b)
}

// TestBusinessDuplicateRecoveryCanReextractWhenNoInvoices 锁定核心自愈条件：
// 库里有文件但提取不出任何票据时，必须判定可自动重提取，让上传不再被拦截。
func TestBusinessDuplicateRecoveryCanReextractWhenNoInvoices(t *testing.T) {
	k := &Knowledge{
		ID:          "k1",
		FileName:    "通行费电子票据汇总单(发票).pdf",
		ParseStatus: "completed",
		CustomMetadata: metaJSON(t, map[string]interface{}{
			"kind":           "invoice",
			"extract_status": "failed",
			"extract_error":  "llm timeout",
			"invoices":       []interface{}{},
		}),
	}
	r := SummarizeDuplicateRecovery(k)
	require.NotNil(t, r)
	assert.True(t, r.CanReextract, "无发票数据时必须允许自动重提取")
	assert.False(t, r.Normal)
	assert.Equal(t, "failed", r.ExtractStatus)
}

// TestBusinessDuplicateRecoveryNormalWhenHasInvoices：已有正常票据时不自动覆盖。
func TestBusinessDuplicateRecoveryNormalWhenHasInvoices(t *testing.T) {
	k := &Knowledge{
		ID:          "k1",
		FileName:    "通行费电子票据汇总单(发票).pdf",
		ParseStatus: "completed",
		CustomMetadata: metaJSON(t, map[string]interface{}{
			"kind":            "invoice",
			"extract_status":  "success",
			"fleet_cert_type": "通行费发票",
			"invoices": []interface{}{
				map[string]interface{}{
					"invoice_no": "64299470", "invoice_date": "2025-10-09",
					"category": "通行费发票",
				},
				map[string]interface{}{"invoice_no": "64042300", "invoice_date": "2025-10-09"},
			},
		}),
	}
	r := SummarizeDuplicateRecovery(k)
	require.NotNil(t, r)
	assert.True(t, r.Normal, "已有正常票据时不得自动覆盖")
	assert.False(t, r.CanReextract)
	assert.Equal(t, 2, r.InvoiceCount, "必须给出已提取张数")
	assert.Equal(t, "通行费发票", r.Category, "必须给出当前分类，避免用户找不到")
	assert.Equal(t, "2025-10-09", r.InvoiceDate, "必须给出开票日期")
}

// TestBusinessDuplicateRecoveryNeverAutoWhenDeleting：文件正在删除中时
// 绝不能自动重提取（会被 worker 的删除覆盖，或与删除竞争）。
func TestBusinessDuplicateRecoveryNeverAutoWhenDeleting(t *testing.T) {
	k := &Knowledge{
		ID:          "k1",
		ParseStatus: "deleting",
		CustomMetadata: metaJSON(t, map[string]interface{}{
			"kind": "invoice", "extract_status": "success", "invoices": []interface{}{},
		}),
	}
	r := SummarizeDuplicateRecovery(k)
	require.NotNil(t, r)
	assert.False(t, r.CanReextract, "删除中的文件不得自动重提取")
	assert.NotEmpty(t, r.Reason, "必须给出拒绝原因")
}

// TestBusinessDuplicateRecoveryNotInvoiceKind：被判定为非票据的文件不应
// 对发票上传自动重提取（应由用户显式改类型）。
func TestBusinessDuplicateRecoveryNotInvoiceKind(t *testing.T) {
	k := &Knowledge{
		ID:          "k1",
		ParseStatus: "completed",
		CustomMetadata: metaJSON(t, map[string]interface{}{
			"kind": "not_invoice", "extract_status": "success", "invoices": []interface{}{},
		}),
	}
	r := SummarizeDuplicateRecovery(k)
	require.NotNil(t, r)
	assert.False(t, r.CanReextract)
}

// TestBusinessDuplicateRecoveryNilSafe：knowledge 为空或 metadata 损坏时
// 必须返回可展示的降级摘要，绝不让上传弹窗拿到空对象后崩在前端。
func TestBusinessDuplicateRecoveryNilSafe(t *testing.T) {
	assert.Nil(t, SummarizeDuplicateRecovery(nil), "knowledge 为 nil 时不出摘要，前端按最保守提示")
	r := SummarizeDuplicateRecovery(&Knowledge{ID: "x", ParseStatus: "completed", CustomMetadata: JSON("{not json")})
	require.NotNil(t, r)
	assert.False(t, r.CanReextract)
	assert.NotEmpty(t, r.Reason)
}
