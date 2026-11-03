package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// 分类专属字段筛选（第 26 轮 O-3）。
//
// 背景：六同步的第 ② 环一直缺 —— 字段定义/列表列/明细浮窗/编辑抽屉/打印都由
// 分类 subs 派生，只有「字段筛选器」写死成「关键词 / 税率 / 开票日期」三项。
// 通行费发票最需要按车牌号筛（运营会问「渝A75R1C 这个月跑了几趟」），
// 此前只能把车牌丢进关键词框，而这会误命中备注里的同串。
//
// 本函数按「字段名 → 期望值」做**结构化包含匹配**：先查票级 fields 扩展桶，
// 再查明细行 extra 桶（车牌号在两处都可能落），全部条件 AND。

func TestMatchFieldFiltersHitsTicketFields(t *testing.T) {
	r := invoiceRecordFixtureWithFields(map[string]any{"车牌号": "渝A75R1C"})
	if !invoiceRecordMatchFieldFilters(r, map[string]string{"车牌号": "渝A75R1C"}) {
		t.Fatal("票级 fields 命中应返回 true")
	}
}

func TestMatchFieldFiltersHitsItemExtra(t *testing.T) {
	r := invoiceRecordFixtureWithExtra(map[string]any{"plate_no": "渝D95256"})
	if !invoiceRecordMatchFieldFilters(r, map[string]string{"plate_no": "渝D95256"}) {
		t.Fatal("明细行 extra 命中应返回 true")
	}
}

func TestMatchFieldFiltersIsContains(t *testing.T) {
	// 用户可能只记得后 5 位 —— 与关键词一致的「包含」语义
	r := invoiceRecordFixtureWithFields(map[string]any{"车牌号": "渝A75R1C"})
	if !invoiceRecordMatchFieldFilters(r, map[string]string{"车牌号": "75R1"}) {
		t.Fatal("应支持子串匹配")
	}
}

func TestMatchFieldFiltersRejectsMismatch(t *testing.T) {
	r := invoiceRecordFixtureWithFields(map[string]any{"车牌号": "渝A75R1C"})
	if invoiceRecordMatchFieldFilters(r, map[string]string{"车牌号": "渝B11111"}) {
		t.Fatal("不匹配必须返回 false")
	}
	if invoiceRecordMatchFieldFilters(r, map[string]string{"车辆类型": "客车"}) {
		t.Fatal("字段不存在时必须返回 false（不能把缺字段当通配）")
	}
}

func TestMatchFieldFiltersAllConditionsMustHold(t *testing.T) {
	r := invoiceRecordFixtureWithFields(map[string]any{"车牌号": "渝A75R1C", "车辆类型": "客车"})
	if !invoiceRecordMatchFieldFilters(r, map[string]string{"车牌号": "渝A75R1C", "车辆类型": "客车"}) {
		t.Fatal("全部命中应返回 true")
	}
	if invoiceRecordMatchFieldFilters(r, map[string]string{"车牌号": "渝A75R1C", "车辆类型": "货车"}) {
		t.Fatal("任一不命中必须返回 false（AND 语义）")
	}
}

func TestMatchFieldFiltersEmptyMeansNoop(t *testing.T) {
	r := invoiceRecordFixtureWithFields(map[string]any{"车牌号": "渝A75R1C"})
	if !invoiceRecordMatchFieldFilters(r, nil) {
		t.Fatal("空条件应放行（不是拦截）")
	}
	if !invoiceRecordMatchFieldFilters(r, map[string]string{}) {
		t.Fatal("空 map 应放行")
	}
	// 值为空串视为未填，不算条件
	if !invoiceRecordMatchFieldFilters(r, map[string]string{"车牌号": ""}) {
		t.Fatal("空值条件应放行")
	}
}

func TestParseFieldFilters(t *testing.T) {
	got := ParseInvoiceFieldFilters("车牌号:渝A75R1C;车辆类型:客车")
	if len(got) != 2 || got["车牌号"] != "渝A75R1C" || got["车辆类型"] != "客车" {
		t.Fatalf("解析结果不对: %v", got)
	}
	// 容错：多余分隔符、空段、无冒号段全部跳过
	got2 := ParseInvoiceFieldFilters(" ; ;车牌号:渝A75R1C;;bad;x:")
	if len(got2) != 1 || got2["车牌号"] != "渝A75R1C" {
		t.Fatalf("容错解析不对: %v", got2)
	}
	if got3 := ParseInvoiceFieldFilters(""); len(got3) != 0 {
		t.Fatalf("空串应返回空 map: %v", got3)
	}
}

// ---- 夹具 ----

func invoiceRecordFixtureWithFields(fields map[string]any) types.InvoiceRecord {
	return types.InvoiceRecord{InvoiceNo: "26507000000315988591", Fields: fields}
}

func invoiceRecordFixtureWithExtra(extra map[string]any) types.InvoiceRecord {
	return types.InvoiceRecord{
		InvoiceNo: "26507000000315988591",
		Items:     []types.InvoiceExtractionItemItems{{Name: "通行费", Extra: extra}},
	}
}
