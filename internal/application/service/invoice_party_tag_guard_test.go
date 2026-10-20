package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============ 任务4：购销方颠倒 / 同名根治 ============
//
// 真机证据（文件 4488fba3「水暖毯-digital_26507000000067486566.pdf」）：
// 票面文本层是 PDF 竖排标签被打散后的形态——
//
//	购 销 买 名 称**:** 重庆星达铜业有限公司 售 名 称**:** 重庆京东创盟信息技术有限公司 方 方 …
//	统一社会信用代码/纳税人识别号: 91500227MA5U54AJ0A  统一社会信用代码/纳税人识别号: 91500113086322499C
//
// 即「购买方名称」在前、「销售方名称」在后，但 `购销买卖方方` 这些标签字被拆散
// 交错插入，按「出现先后」或按「售」字位置硬匹配都会把两者对调。
// 结果：buyer 被写成京东创盟、seller 被写成星达铜业（购销方整体颠倒）。

// realSwappedPartyChunk 是上述发票的真实 chunk 文本（来自 VM chunks 表）。
const realSwappedPartyChunk = `发票号码: 26507000000067486566

# <u>电子发票(普通发票)</u>开票日期: 2026年01月20日

购 销 买 名 称**:** 重庆星达铜业有限公司 售 名 称**:** 重庆京东创盟信息技术有限公司 方 方 信 信 统一社会信用代码**/**纳税人识别号**:** 91500227MA5U54AJ0A 统一社会信用代码**/**纳税人识别号**:** 91500113086322499C 息 息

项目名称 规格型号 单位 数 量 单 价 金 额 税率/征收率 税 额
`

// TestBusinessInvoicePartiesFromRealChunkNotSwapped 主回归：
// 真实 chunk 的「购…名称 A 售…名称 B」交错布局必须解析为 buyer=A、seller=B。
func TestBusinessInvoicePartiesFromRealChunkNotSwapped(t *testing.T) {
	buyerName, buyerTax, sellerName, sellerTax := parsePartiesFromText(realSwappedPartyChunk)
	assert.Equal(t, "重庆星达铜业有限公司", buyerName, "购买方必须是星达铜业（第一个名称）")
	assert.Equal(t, "91500227MA5U54AJ0A", buyerTax, "购买方税号必须与名称一一对应")
	assert.Equal(t, "重庆京东创盟信息技术有限公司", sellerName, "销售方必须是京东创盟（第二个名称）")
	assert.Equal(t, "91500113086322499C", sellerTax, "销售方税号必须与名称一一对应")
}

// TestBusinessInvoicePartyGuardSwapsInverted 守卫生效：
// 即使上游（LLM）把两者写反，只要 seller 是我司（名称或税号命中），必须自动换回。
func TestBusinessInvoicePartyGuardSwapsInverted(t *testing.T) {
	inv := InvoiceExtractionItem{
		BuyerName:   "重庆京东创盟信息技术有限公司",
		BuyerTaxNo:  "91500113086322499C",
		SellerName:  "重庆星达铜业有限公司",
		SellerTaxNo: "91500227MA5U54AJ0A",
	}
	normalizeInvoiceParties(&inv, "")
	assert.Equal(t, "重庆星达铜业有限公司", inv.BuyerName, "颠倒后必须换回：buyer=我司")
	assert.Equal(t, "91500227MA5U54AJ0A", inv.BuyerTaxNo)
	assert.Equal(t, "重庆京东创盟信息技术有限公司", inv.SellerName, "颠倒后必须换回：seller=真实商户")
	assert.Equal(t, "91500113086322499C", inv.SellerTaxNo)
}

// 仅税号命中我司、名称被写错时，也要按税号判定为销售方并换回。
func TestBusinessInvoicePartyGuardSwapsByTaxNoOnly(t *testing.T) {
	inv := InvoiceExtractionItem{
		BuyerName:   "某某商户",
		BuyerTaxNo:  "91330100MA2XXXXXXX",
		SellerName:  "重庆星达铜业有限公司",
		SellerTaxNo: ourCompanyTaxNo,
	}
	normalizeInvoiceParties(&inv, "")
	assert.Equal(t, ourCompanyName, inv.BuyerName)
	assert.Equal(t, "某某商户", inv.SellerName)
	assert.Equal(t, "91330100MA2XXXXXXX", inv.SellerTaxNo)
}

// 购买方税号是我司但名称写错（真机 09ee1bd0：buyer_name=重庆两山建设投资有限公司
// 而 buyer_tax_no=91500227MA5U54AJ0A）→ 以税号为准把名称纠正为我司。
func TestBusinessInvoicePartyGuardFixesBuyerNameByTaxNo(t *testing.T) {
	inv := InvoiceExtractionItem{
		BuyerName:   "重庆两山建设投资有限公司",
		BuyerTaxNo:  ourCompanyTaxNo,
		SellerName:  "重庆水资源产业股份有限公司璧山自来水分公司",
		SellerTaxNo: "91500120MAE8K2231T",
	}
	normalizeInvoiceParties(&inv, "")
	assert.Equal(t, ourCompanyName, inv.BuyerName, "购买方税号是我司时名称必须纠正为我司")
	assert.Equal(t, "重庆水资源产业股份有限公司璧山自来水分公司", inv.SellerName)
}

// 购销方同名/同税号：严禁保留一份重复值。
func TestBusinessInvoicePartyGuardRejectsSameParty(t *testing.T) {
	inv := InvoiceExtractionItem{
		BuyerName:   "重庆高速公路集团有限公司",
		BuyerTaxNo:  "9150010076177498X",
		SellerName:  "重庆高速公路集团有限公司",
		SellerTaxNo: "9150010076177498X",
	}
	normalizeInvoiceParties(&inv, "")
	assert.NotEqual(t, inv.BuyerName, inv.SellerName, "购销方绝不允许同名")
}

// 购销方本身就正确时不得被守卫改动（避免误伤）。
func TestBusinessInvoicePartyGuardKeepsCorrectPair(t *testing.T) {
	inv := InvoiceExtractionItem{
		BuyerName:   ourCompanyName,
		BuyerTaxNo:  ourCompanyTaxNo,
		SellerName:  "重庆永璧永津高速公路有限公司",
		SellerTaxNo: "91500118MA618KLJ66",
	}
	normalizeInvoiceParties(&inv, "")
	assert.Equal(t, ourCompanyName, inv.BuyerName)
	assert.Equal(t, "重庆永璧永津高速公路有限公司", inv.SellerName)
	assert.Equal(t, "91500118MA618KLJ66", inv.SellerTaxNo)
}

// ============ 任务3：标签参与关键词搜索（必须跨页生效） ============

// tagSearchRepo 只实现列表与标签查询：标签只存在于 knowledge_tags 关联表，
// 不回填到 Knowledge.Tags（复刻真实链路：列表查询不 preload 标签）。
type tagSearchRepo struct {
	taxRateFakeRepo
	tagMap map[string][]*types.KnowledgeTag
	// askedIDs 记录过滤阶段实际请求过标签的 knowledge id（验证预载发生在过滤前）
	askedIDs []string
}

func (f *tagSearchRepo) GetKnowledgeTags(_ context.Context, ids []string) (map[string][]*types.KnowledgeTag, error) {
	f.askedIDs = append(f.askedIDs, ids...)
	out := map[string][]*types.KnowledgeTag{}
	for _, id := range ids {
		if v, ok := f.tagMap[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

// TestBusinessInvoiceSearchMatchesKnowledgeTagsAcrossPages 主回归：
// 带标签的文件排在第二页时，按标签名搜索仍必须命中（标签预载发生在过滤前）。
func TestBusinessInvoiceSearchMatchesKnowledgeTagsAcrossPages(t *testing.T) {
	var items []*types.Knowledge
	// 第 1 页 20 条无标签记录，把目标文件挤到第 2 页
	for i := 0; i < 20; i++ {
		items = append(items, &types.Knowledge{
			ID: "pad", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			CustomMetadata: types.JSON(mustJSON(t, map[string]interface{}{
				"kind": "invoice", "extract_status": "success",
				"invoices": []interface{}{map[string]interface{}{
					"invoice_no": "PAD-" + string(rune('A'+i)), "total_amount": 10,
					"seller_name": "某商户",
				}},
			})),
		})
	}
	items = append(items, &types.Knowledge{
		ID: "k-tagged", KnowledgeBaseID: "kb1", ParseStatus: "completed",
		CustomMetadata: types.JSON(mustJSON(t, map[string]interface{}{
			"kind": "invoice", "extract_status": "success",
			"invoices": []interface{}{map[string]interface{}{
				"invoice_no": "TAGGED-001", "total_amount": 20, "seller_name": "高速商户",
			}},
		})),
	})
	repo := &tagSearchRepo{
		taxRateFakeRepo: taxRateFakeRepo{items: items},
		tagMap: map[string][]*types.KnowledgeTag{
			"k-tagged": {{ID: "t1", Name: "通行费"}},
		},
	}
	svc := &BusinessExtractService{repo: repo}

	res, err := svc.ListInvoiceRecords(catCtxV1(), "kb1", types.InvoiceListFilter{
		Keyword: "通行费", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Len(t, res.Data, 1, "按标签名搜索必须命中第二页的记录")
	assert.Equal(t, "TAGGED-001", res.Data[0].InvoiceNo)
	assert.Contains(t, res.Data[0].Tags, "通行费", "命中的记录应带回标签供列表展示")
}

// 关键词不含标签时也应能命中（回归保护：不要因为预载逻辑破坏原有全字段搜索）。
func TestBusinessInvoiceSearchStillMatchesInvoiceNo(t *testing.T) {
	items := []*types.Knowledge{{
		ID: "k1", KnowledgeBaseID: "kb1", ParseStatus: "completed",
		CustomMetadata: types.JSON(mustJSON(t, map[string]interface{}{
			"kind": "invoice", "extract_status": "success",
			"invoices": []interface{}{map[string]interface{}{
				"invoice_no": "26507000000067486566", "total_amount": 20, "seller_name": "京东创盟",
			}},
		})),
	}}
	svc := &BusinessExtractService{repo: &tagSearchRepo{taxRateFakeRepo: taxRateFakeRepo{items: items}}}
	res, err := svc.ListInvoiceRecords(catCtxV1(), "kb1", types.InvoiceListFilter{
		Keyword: "67486566", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	assert.Len(t, res.Data, 1)
}
