package handler

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// 车牌号在 data 里的键名不统一：手工表单可能写「车牌号」，派生记录写 plate_no，
// 历史导入可能写「车牌号码」。三者都要能识别，否则车辆自动补建失效。
func TestPickPlateFromRecordData(t *testing.T) {
	cases := []struct {
		name string
		data map[string]any
		want string
	}{
		{"车牌号", map[string]any{"车牌号": " 渝C81022 "}, "渝C81022"},
		{"plate_no", map[string]any{"plate_no": "渝A11111"}, "渝A11111"},
		{"车牌号码", map[string]any{"车牌号码": "渝B22222"}, "渝B22222"},
		{"空值", map[string]any{}, ""},
		{"空串", map[string]any{"车牌号": "  "}, ""},
		{"非字符串", map[string]any{"车牌号": 123}, ""},
	}
	for _, c := range cases {
		if got := pickPlateFromRecordData(c.data); got != c.want {
			t.Fatalf("%s: 期望 %q，实际 %q", c.name, c.want, got)
		}
	}
}

// 车辆必填必须放宽（第 28 轮 Q3）：记录型不再因为缺 vehicle_id 被 400 拦下，
// 否则「轮胎/年检/成本 7 项」在 fleet_vehicles 为空时永远录不进数据。
func TestNormalizeFleetRecordAllowsEmptyVehicle(t *testing.T) {
	rec := types.FleetRecord{
		RecordType: types.FleetRecordTire,
		Data:       map[string]any{"轮胎编号": "T-001"},
	}
	if err := normalizeFleetRecord(&rec); err != nil {
		t.Fatalf("缺 vehicle_id 不应再报错，实际: %v", err)
	}
	// 档案类本来就不要求车辆
	archive := types.FleetRecord{RecordType: types.FleetRecordVehicleArchive}
	if err := normalizeFleetRecord(&archive); err != nil {
		t.Fatalf("档案类不应要求车辆: %v", err)
	}
	// 非法类型仍要拦
	bad := types.FleetRecord{RecordType: "not-a-type"}
	if err := normalizeFleetRecord(&bad); err == nil {
		t.Fatal("非法 record_type 必须报错")
	}
}

// 车牌归一：OCR 与手工各写一种大小写时，必须归到同一辆车。
func TestNormalizePlate(t *testing.T) {
	cases := map[string]string{
		" 渝C81022 ": "渝C81022",
		"渝c81022":   "渝C81022",
		"":          "",
		"  ":        "",
		"京AN8145":   "京AN8145",
		"川AE75938":  "川AE75938",
	}
	for in, want := range cases {
		if got := normalizePlate(in); got != want {
			t.Fatalf("normalizePlate(%q) = %q，期望 %q", in, got, want)
		}
	}
}
