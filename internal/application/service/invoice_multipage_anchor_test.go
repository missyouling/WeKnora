package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// 真机样本：通行费电子票据汇总单(发票).pdf 共 6 页，每页 1 张票。
// 旧实现只落库 4 张（第 5、6 页静默丢失），且因为前 4 张字段完整，
// 半残守卫判「完整」后候选路径不再补提。
const tollInvoicePageTpl = `## 电子发票（普通发票） 国家税务总局

发票号码：%s

开票日期：2026年09月02日

<table>
<tr><td>购买方信息</td><td colspan="3">名称:重庆星达铜业有限公司统一社会信用代码/纳税人识别号:91500227MA5U54AJ0A</td><td>销售方信息</td><td colspan="3">名称:重庆某高速%s统一社会信用代码/纳税人识别号:91500000774866051J</td></tr>
<tr><td colspan="5">项目名称 车牌号 车辆类型 通行日期起</td><td>通行日期止</td><td>金额</td><td>税率/征收率 税额</td></tr>
<tr><td colspan="5">*生产生活服务*通行费 渝AAU0092 货车 20260803</td><td>20260828</td><td>%s</td><td>3%% %s</td></tr>
<tr><td colspan="8">合计 ¥%s ¥%s</td></tr>
<tr><td colspan="2">价税合计(大写)</td><td colspan="4">壹佰零叁圆伍分</td><td>(小写)</td><td>¥%s</td></tr>
</table>

开票人：王琳琳
`

var tollPageNos = []string{
	"26507906970900022705",
	"26507906730900004499",
	"26507904440900007271",
	"26507903750900005764",
	"26507904700900011894",
	"26507903160900007109",
}

// tollChunks 造 6 页票的 chunk（每页一个 chunk）。
// withLocators=false 用于模拟解析引擎没有回填 SourceLocators 的场景。
func tollChunks(withLocators bool) []*types.Chunk {
	chunks := make([]*types.Chunk, 0, len(tollPageNos))
	for i, no := range tollPageNos {
		body := fmt.Sprintf(tollInvoicePageTpl, no, no[len(no)-2:], "100.05", "3.00", "100.05", "3.00", "103.05")
		c := &types.Chunk{
			ChunkIndex:  i,
			ChunkType:   types.ChunkTypeText,
			Content:     body,
			IsEnabled:   true,
			KnowledgeID: "k-1",
		}
		if withLocators {
			c.SourceLocators = []types.SourceLocator{{Type: "pdf", Page: i + 1}}
		}
		chunks = append(chunks, c)
	}
	return chunks
}

// ---- 任务 1.3：正向数学反算（只有金额+税率，缺税额与价税合计）----

// 真机 26507904440700023289：amount=51.95、tax_rate=3%，tax/total 全空。
// 必须补出 tax=1.56、total=53.51，而不是留在列表里显示“-”。
func TestBusinessInvoiceForwardMathFromRate(t *testing.T) {
	rate := 0.03
	inv := InvoiceExtractionItem{Amount: f64ptr(51.95), TaxRate: &rate}
	backfillInvoiceMath(&inv, false)

	require.NotNil(t, inv.Tax, "有金额+税率时必须补出税额")
	require.NotNil(t, inv.TotalAmount, "有金额+税率时必须补出价税合计")
	assert.InDelta(t, 1.56, *inv.Tax, 0.005, "51.95 × 3% = 1.5585 → 1.56")
	assert.InDelta(t, 53.51, *inv.TotalAmount, 0.005, "51.95 + 1.56 = 53.51")
}

// 票面已给税额但税额为 0 的应税票，同样要按税率补齐（0 视为“没抓到”）。
func TestBusinessInvoiceForwardMathWhenZeroTax(t *testing.T) {
	rate := 0.09
	zero := 0.0
	inv := InvoiceExtractionItem{Amount: f64ptr(200), TaxRate: &rate, Tax: &zero}
	backfillInvoiceMath(&inv, false)
	require.NotNil(t, inv.Tax)
	assert.InDelta(t, 18.0, *inv.Tax, 0.01)
	assert.InDelta(t, 218.0, *inv.TotalAmount, 0.01)
}

// 已有正确税额/价税合计的票不得被改动。
func TestBusinessInvoiceForwardMathKeepsExisting(t *testing.T) {
	rate := 0.03
	inv := InvoiceExtractionItem{Amount: f64ptr(100.05), TaxRate: &rate, Tax: f64ptr(3.00), TotalAmount: f64ptr(103.05)}
	backfillInvoiceMath(&inv, false)
	assert.InDelta(t, 3.00, *inv.Tax, 1e-9)
	assert.InDelta(t, 103.05, *inv.TotalAmount, 1e-9)
}

// ---- 任务 2：日期筛选严格只按 invoice_date ----

func TestBusinessInvoiceDateFilterStrictInvoiceDate(t *testing.T) {
	rec := types.InvoiceRecord{
		InvoiceDate: "2026-10-08",
		Items: []types.InvoiceExtractionItemItems{{
			Extra: map[string]any{"travel_from": "2026-09-02", "travel_to": "2026-09-02"},
		}},
	}
	// 命中过：通行日期在 9 月 → 旧实现会把 10 月开票的票筛进 9 月结果
	assert.False(t, invoiceRecordDateInRange(rec, "2026-09-01", "2026-09-30"),
		"开票日期 2026-10-08 不得因通行日期在 9 月而命中 9 月区间")
	// 按 10 月筛才应命中
	assert.True(t, invoiceRecordDateInRange(rec, "2026-10-01", "2026-10-31"))
	// 无筛选条件时不过滤
	assert.True(t, invoiceRecordDateInRange(rec, "", ""))
}

func TestBusinessInvoiceDateFilterIgnoresMissingInvoiceDate(t *testing.T) {
	rec := types.InvoiceRecord{
		Items: []types.InvoiceExtractionItemItems{{
			Extra: map[string]any{"travel_from": "2026-09-02", "travel_to": "2026-09-02"},
		}},
	}
	assert.False(t, invoiceRecordDateInRange(rec, "2026-09-01", "2026-09-30"),
		"没有开票日期时不得用通行日期兜底命中")
}

// ---- 任务 1.1 / 1.2：票面锚点数核对 + 物理页码 ----

func TestBusinessInvoiceAnchorCountMatchesTicketPages(t *testing.T) {
	assert.Equal(t, 6, CountInvoiceAnchors(tollChunks(true)))
	// 无 SourceLocators 时同样要数得出来（只依赖文本）
	assert.Equal(t, 6, CountInvoiceAnchors(tollChunks(false)))
}

// 规则只提了 4 张时必须能定位漏掉的两张（票号 + 物理页码 + 补提文本）。
func TestBusinessInvoiceMissingAnchorsLocatesPages(t *testing.T) {
	chunks := tollChunks(true)
	extracted := make([]InvoiceExtractionItem, 0, 4)
	for _, no := range tollPageNos[:4] {
		extracted = append(extracted, InvoiceExtractionItem{InvoiceNo: no})
	}
	targets := FindSupplementTargets(chunks, extracted, 10)
	require.Len(t, targets, 2, "6 页票只提 4 张 → 必须定位到 2 个漏提锚点")
	assert.Equal(t, tollPageNos[4], targets[0].InvoiceNo)
	assert.Equal(t, 5, targets[0].Page, "第 5 张票的物理页码是 P5")
	assert.Equal(t, tollPageNos[5], targets[1].InvoiceNo)
	assert.Equal(t, 6, targets[1].Page, "第 6 张票的物理页码是 P6")
	// 补提文本必须包含该票的票号、金额与购销方，够模型独立识别
	for _, tg := range targets {
		assert.Contains(t, tg.Text, tg.InvoiceNo)
		assert.Contains(t, tg.Text, "重庆星达铜业有限公司")
	}
}

// 解析引擎没给 SourceLocators 时，按票面页眉「电子发票」出现次数推算物理页码，
// 杜绝“数组序号冒充物理页码”（真机 6 页 PDF 曾被标成 P1~P4）。
func TestBusinessInvoicePagesInferredFromTicketHeaders(t *testing.T) {
	chunks := tollChunks(false)
	src := BuildExtractSourceText(chunks)
	require.True(t, src.HasPages(), "无 locator 时应从票面页眉推断出页码锚点")

	invs := make([]InvoiceExtractionItem, 0, len(tollPageNos))
	for _, no := range tollPageNos {
		invs = append(invs, InvoiceExtractionItem{InvoiceNo: no})
	}
	assigned := AssignPhysicalPages(src, invs)
	assert.Equal(t, len(tollPageNos), assigned, "每张票都应回填物理页码")
	for i := range invs {
		assert.Equal(t, i+1, invs[i].PageNo, "第 %d 张票应落在 P%d", i+1, i+1)
	}
}

// 全部提全时不得产生补提目标（避免无谓的模型调用）。
func TestBusinessInvoiceNoSupplementWhenComplete(t *testing.T) {
	invs := make([]InvoiceExtractionItem, 0, len(tollPageNos))
	for _, no := range tollPageNos {
		invs = append(invs, InvoiceExtractionItem{InvoiceNo: no})
	}
	assert.Empty(t, FindSupplementTargets(tollChunks(true), invs, 10))
}

// 20 位数字必须是独立数字（不能被 21 位数字截断）且去重。
func TestBusinessInvoiceAnchorCountDedupAndBoundary(t *testing.T) {
	body := strings.Join([]string{
		"发票号码：26507906970900022705",
		"重复出现 26507906970900022705",
		"校验码 123456789012345678901", // 21 位，不得截出 20 位
	}, "\n")
	chunks := []*types.Chunk{{ChunkIndex: 0, ChunkType: types.ChunkTypeText, Content: body}}
	assert.Equal(t, 1, CountInvoiceAnchors(chunks))
}
