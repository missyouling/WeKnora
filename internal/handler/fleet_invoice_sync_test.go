package handler

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
)

func pfloat(v float64) *float64 { return &v }

// 通行费发票识别：分类名或明细名含「通行费」。
func TestIsTollInvoice(t *testing.T) {
	toll := service.InvoiceExtractionItem{Category: "通行费发票"}
	if !isTollInvoice(toll) {
		t.Fatal("分类=通行费发票 必须识别为通行费")
	}
	byItem := service.InvoiceExtractionItem{Items: []service.InvoiceExtractionLine{{Name: "*生产生活服务*通行费"}}}
	if !isTollInvoice(byItem) {
		t.Fatal("明细名含通行费也必须识别")
	}
	other := service.InvoiceExtractionItem{Category: "办公费", Items: []service.InvoiceExtractionLine{{Name: "打印纸"}}}
	if isTollInvoice(other) {
		t.Fatal("办公费不得识别为通行费")
	}
}

// 加油/充电发票识别：成品油、柴油、汽油、充电。
func TestIsFuelInvoice(t *testing.T) {
	cases := []service.InvoiceExtractionItem{
		{Category: "加油费"},
		{Category: "能耗发票", Items: []service.InvoiceExtractionLine{{Name: "0#柴油"}}},
		{Items: []service.InvoiceExtractionLine{{Name: "车用汽油(Ⅵ"}}},
		{Items: []service.InvoiceExtractionLine{{Name: "充电服务费"}}},
	}
	for i, inv := range cases {
		if !isFuelInvoice(inv) {
			t.Fatalf("case %d 应识别为加油充电: %+v", i, inv)
		}
	}
	if isFuelInvoice(service.InvoiceExtractionItem{Category: "通行费发票"}) {
		t.Fatal("通行费不得识别为加油")
	}
}

// 派生映射：路桥费要带车牌/通行日期/含税金额/税额/来源发票号，并能定位源 PDF 页。
func TestBuildTollRecordFromInvoice(t *testing.T) {
	inv := service.InvoiceExtractionItem{
		InvoiceNo: "26507903331100005079", InvoiceDate: "2026-11-02",
		Category: "通行费发票", Amount: pfloat(198.09), Tax: pfloat(17.83),
		TotalAmount: pfloat(215.92), TaxRate: pfloat(0.09),
		Items: []service.InvoiceExtractionLine{{
			Name: "*生产生活服务*通行费", Amount: pfloat(198.09), TaxAmt: pfloat(17.83),
			Extra: map[string]any{"plate_no": "渝C81022", "travel_from": "2026-10-13", "travel_to": "2026-10-27"},
		}},
	}
	rec, ok := buildFleetRecordFromInvoice(10000, "kb-file-1", "通行费电子票据汇总单(发票).pdf", inv, 2)
	if !ok {
		t.Fatal("应能构造记录")
	}
	if rec.RecordType != types.FleetRecordRoadToll {
		t.Fatalf("record_type 应为 road-toll，实际 %s", rec.RecordType)
	}
	if rec.RecordMonth != "2026-11" || rec.RecordDate != "2026-11-02" {
		t.Fatalf("日期/月份映射错误: %s / %s", rec.RecordDate, rec.RecordMonth)
	}
	if rec.Amount != 215.92 {
		t.Fatalf("金额应取价税合计 215.92，实际 %v", rec.Amount)
	}
	if rec.Data["车牌号"] != "渝C81022" {
		t.Fatalf("缺少车牌号: %+v", rec.Data)
	}
	if rec.Data["通行起始日期"] != "2026-10-13" || rec.Data["通行截止日期"] != "2026-10-27" {
		t.Fatalf("通行日期映射错误: %+v", rec.Data)
	}
	if rec.Data["来源发票号"] != "26507903331100005079" {
		t.Fatalf("缺少来源发票号: %+v", rec.Data)
	}
	if rec.Data["源文件ID"] != "kb-file-1" || rec.FileName == "" {
		t.Fatalf("必须带源文件关联以便预览 PDF: %+v", rec)
	}
	if rec.DocKnowledgeID != "" {
		t.Fatalf("派生记录不得写 doc_knowledge_id 列（会撞部分唯一索引）: %q", rec.DocKnowledgeID)
	}
	if rec.SourceRef != "kb-file-1#26507903331100005079" {
		t.Fatalf("去重键错误: %q", rec.SourceRef)
	}
}

// 派生映射：加油充电取数量/单价/油品，金额取价税合计。
func TestBuildFuelRecordFromInvoice(t *testing.T) {
	inv := service.InvoiceExtractionItem{
		InvoiceNo: "26507000000107514773", InvoiceDate: "2026-05-08",
		Category: "能耗发票", Amount: pfloat(1183.02), Tax: pfloat(21.9),
		TotalAmount: pfloat(1204.92),
		Items: []service.InvoiceExtractionLine{{
			Name: "0#柴油", Qty: pfloat(1000), Price: pfloat(6.6),
			Amount: pfloat(1183.02), TaxAmt: pfloat(21.9), TaxRate: pfloat(0.03),
			Extra: map[string]any{"plate_no": "渝C81022"},
		}},
	}
	rec, ok := buildFleetRecordFromInvoice(10000, "kb-file-2", "2026.03月油费发票.pdf", inv, 1)
	if !ok {
		t.Fatal("应能构造记录")
	}
	if rec.RecordType != types.FleetRecordFuelCharge {
		t.Fatalf("record_type 应为 fuel-charge，实际 %s", rec.RecordType)
	}
	if rec.Amount != 1204.92 {
		t.Fatalf("金额应取价税合计，实际 %v", rec.Amount)
	}
	if rec.Data["油品/电价类型"] != "0#柴油" {
		t.Fatalf("油品映射错误: %+v", rec.Data)
	}
	if rec.Data["车牌号"] != "渝C81022" {
		t.Fatalf("缺少车牌号: %+v", rec.Data)
	}
}

// 无金额/无来源的发票不派生（避免产生空壳台账）。
func TestBuildRecordSkipsInvalidInvoice(t *testing.T) {
	_, ok := buildFleetRecordFromInvoice(10000, "k", "f.pdf",
		service.InvoiceExtractionItem{InvoiceNo: "1", Category: "通行费发票"}, 1)
	if ok {
		t.Fatal("金额全空的发票不得派生")
	}
}
