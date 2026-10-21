package handler

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// 存量页码修补（2026-10-21）：
//   - 候选判定必须能命中「6 页 6 票尾票卡在 P5」的签名，同时不误伤
//     「一页两票」（同页重复出现在中间或末尾但后面没有该票号）；
//   - 落库只改 page_no，其余键逐字节保留。

func TestInvoicePagesSuspicious(t *testing.T) {
	cases := []struct {
		name  string
		pages []int
		want  bool
	}{
		{"6 页 6 票尾票卡在 P5", []int{1, 2, 3, 4, 5, 5}, true},
		{"非单调（后票页码更小）", []int{1, 2, 1, 4}, true},
		{"正常 6 页 6 票", []int{1, 2, 3, 4, 5, 6}, false},
		{"一页两票在中间", []int{1, 2, 3, 3, 4, 5}, false},
		{"一页两票在末尾但前缀不连续", []int{1, 2, 3, 3}, true}, // 仅进入候选，是否改由原文证据决定
		{"单票", []int{1}, false},
		{"缺页码", []int{0, 0}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, invoicePagesSuspicious(tc.pages))
		})
	}
}

func TestSetInvoicePageNosPreservesOtherKeys(t *testing.T) {
	raw := types.JSON(`{
		"kind":"invoice","extract_status":"success","invoice_category":"通行费发票",
		"invoices":[
			{"invoice_no":"35209009","page_no":1,"total_amount":103.05,"fields":{"车牌号":"渝AAU0092"},"items":[{"name":"通行费"}]},
			{"invoice_no":"29836101","page_no":5,"total_amount":88.10,"fields":{"车牌号":"渝AAU0093"}}
		],
		"auto_deleted_count":0
	}`)
	out, ok := setInvoicePageNos(raw, []int{1, 6})
	require.True(t, ok)

	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out, &got))
	var invs []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(got["invoices"], &invs))
	require.Len(t, invs, 2)

	// 只改 page_no
	var p int
	require.NoError(t, json.Unmarshal(invs[1]["page_no"], &p))
	assert.Equal(t, 6, p, "尾票必须刷成 P6")

	// 其它键原样保留
	assert.Contains(t, string(invs[1]["fields"]), "渝AAU0093")
	assert.Contains(t, string(invs[0]["items"]), "通行费")
	assert.Contains(t, string(got["invoice_category"]), "通行费发票")
	assert.Contains(t, string(got["kind"]), "invoice")
	assert.Contains(t, string(got["auto_deleted_count"]), "0")
}

// 页码已正确时不产生任何写库（幂等）。
func TestSetInvoicePageNosNoopWhenSame(t *testing.T) {
	raw := types.JSON(`{"invoices":[{"invoice_no":"1","page_no":1},{"invoice_no":"2","page_no":2}]}`)
	_, ok := setInvoicePageNos(raw, []int{1, 2})
	assert.False(t, ok, "页码未变时不得改写 metadata")
}

// 数组长度不一致时宁可不动，也不能错位改写。
func TestSetInvoicePageNosRejectsLengthMismatch(t *testing.T) {
	raw := types.JSON(`{"invoices":[{"invoice_no":"1","page_no":1}]}`)
	_, ok := setInvoicePageNos(raw, []int{1, 2})
	assert.False(t, ok)
}
