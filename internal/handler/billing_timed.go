package handler

// 分时电费核算：基准电量（计费电量 + 差额分摊电量）与分时电费的唯一口径。
//
// 为什么单独抽出来：
//   这套算法在三个地方要用（生成账单的「电量明细」卡片、分时电费子项、
//   前端列表与悬浮说明），任何一处口径漂移都会让「列表 54,369.99、标准核算表
//   54,422.79」这种对不上账的问题重现。这里只放纯函数，输入抄见电量与账单损耗，
//   输出逐表逐时段的计费/差额/基准电量，便于用标准核算表做精确断言。
//
// 标准口径（2026-09 持睿汽车为例）：
//   1) 逐表逐时段抄见电量 u = (止度 - 起度) × 倍率；
//   2) 计费电量 c = u + 损耗 + 加减电量（账单「电量明细」的变损+线损与加减，
//      按账单口径整体落在第一块表上，其余分表不加成 —— 与人工核算表一致）；
//   3) 差额电量 ΔQ = 星达分表总电量 - 各分表抄见电量合计，
//      按各表各时段计费电量占 C总 的比例分摊：d = ΔQ × c / C总；
//   4) 基准电量 b = c + d，分时电费 = b × 时段单价（尖/峰/平/谷各自单价）。

// timedPeriodOrder 分时时段顺序（尖 → 峰 → 平 → 谷）。
var timedPeriodOrder = []string{"deep", "peak", "flat", "valley"}

// timedMeterUsage 一块分时表当月的抄见读数（按 period 键：deep/peak/flat/valley）。
type timedMeterUsage struct {
	Key   string
	Name  string
	Rate  float64
	Usage map[string]float64
}

// timedMeterRow 某块表某个时段的电量明细（对应「电量明细（工商业）」卡片一行）。
type timedMeterRow struct {
	Period  string
	Usage   float64
	Loss    float64
	Adjust  float64
	BillKwh float64 // 计费电量 c
	DiffKwh float64 // 差额分摊电量 d
	BaseKwh float64 // 基准电量 b = c + d
}

// timedAllocation 整月分时电量分摊结果。
type timedAllocation struct {
	// TotalUsage 各分表抄见电量合计（ΔQ 的减数）。
	TotalUsage float64
	// TotalBill 各表各时段计费电量合计（差额分摊的分母）。
	TotalBill float64
	// DeltaQ 差额电量（星达分表总 - 各分表合计）。
	DeltaQ float64
	// PeriodBase 各时段基准电量合计（两位小数，用于明细展示与 quantity）。
	PeriodBase map[string]float64
	// PeriodBaseExact 各时段基准电量合计（**不取整**）。
	//
	// 分时电费必须用未取整的电量乘单价：先 round2 再乘会让每段误差 ±0.005 度，
	// 四段累加后总额差 1 分（真机：零售交易电费 54,422.78 vs 标准表 54,422.79）。
	PeriodBaseExact map[string]float64
	// Rows 逐表逐时段明细，key = 表 key。
	Rows map[string][]timedMeterRow
}

// allocateTimedKwh 按标准口径分摊分时电量。
//
// loss/adjust 来自市电账单「电量明细」的（变损+线损）/加减电量，按 period 键；
// deltaQ 为星达分表总电量与各分表抄见合计之差（为负时同样按比例分摊）。
func allocateTimedKwh(meters []timedMeterUsage, loss, adjust map[string]float64, deltaQ float64) timedAllocation {
	out := timedAllocation{
		PeriodBase:      make(map[string]float64, len(timedPeriodOrder)),
		PeriodBaseExact: make(map[string]float64, len(timedPeriodOrder)),
		Rows:            make(map[string][]timedMeterRow, len(meters)),
	}
	// 1) 抄见电量与计费电量
	for i, m := range meters {
		for _, p := range timedPeriodOrder {
			u := round2(m.Usage[p])
			out.TotalUsage = round2(out.TotalUsage + u)
			// 损耗/加减整体落在第一块表：账单的电量明细是按表 1 列示的，
			// 摊到每块表上会让「表 2/表 3 计费电量 = 抄见电量」这条人工核对规律失效。
			var l, a float64
			if i == 0 {
				l = round2(loss[p])
				a = round2(adjust[p])
			}
			c := round2(u + l + a)
			out.TotalBill = round2(out.TotalBill + c)
			out.Rows[m.Key] = append(out.Rows[m.Key], timedMeterRow{
				Period: p, Usage: u, Loss: l, Adjust: a, BillKwh: c,
			})
		}
	}
	// 2) 差额分摊 + 基准电量
	for i := range out.Rows {
		rows := out.Rows[i]
		for j := range rows {
			c := rows[j].BillKwh
			d := 0.0
			if out.TotalBill > 0 {
				d = deltaQ * c / out.TotalBill
			}
			rows[j].DiffKwh = round2(d)
			rows[j].BaseKwh = round2(c + d)
			out.PeriodBase[rows[j].Period] = round2(out.PeriodBase[rows[j].Period] + rows[j].BaseKwh)
			out.PeriodBaseExact[rows[j].Period] += c + d
		}
		out.Rows[i] = rows
	}
	out.DeltaQ = round2(deltaQ)
	return out
}
