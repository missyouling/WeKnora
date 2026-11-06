package service

import (
	"math"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// 电量（定比）权威取数（第 43 轮）。
//
// 唯一权威来源：市电账单 `fee_items` 中 `(5)目录电费` 各行的**计费数量 qty**。
// 这就是账单「账单概况 / 居民电费 → 目录电费（居民）」的计费数量，与
// 「居民电量明细 → 计费电量」是同一个数，且电网票面保证它们相等。
//
// 为什么不能取 `residential_readings[].bill_kwh`：它是**派生缓存**，
// 历史管线在多个月份把它算成了 `本期电量 × 0.015`（甚至乘了两次），
// 真机 17 个月里有 11 个月是错的：
//
//	2026-07 147.32（= 9821×0.015，乘两次）、2026-06 0、2026-04 0、2025-10 0、
//	2026-05 149.79、2026-03 208.95、2026-02 82.65、2025-12 209.52、2025-11 213、
//	2026-01 10906.20、2025-09 14707.35
//
// 而 `(5)目录电费` 的 qty 在全部 17 个月都是正确的（2026-10=9354 … 2025-06=14363）。
//
// 规则：**先读票面分项，读不到才退回派生缓存**；派生缓存与票面不一致时必须
// 以票面为准并回写缓存，绝不允许把已经算好的居民计费电量再乘一次 0.015。

// residentCatalogFeeCategory 居民目录电费的费用类别（票面固定前缀）。
const residentCatalogFeeCategory = "(5)目录电费"

// ResidentBillingKwh 返回该账单的居民（定比）计费电量。
//
// 取 `(5)目录电费` 各行 qty 之和：绝大多数月份只有一行；2025-07 有两行
// （电能表 4501 + 定比 11953 = 16454），必须合计而不是取最大值。
// 没有该分类（老账单/非居民账单）时返回 0，由调用方决定是否退回缓存。
func ResidentBillingKwh(item *types.UtilityBillExtractionItem) float64 {
	if item == nil {
		return 0
	}
	var sum float64
	for _, f := range item.FeeItems {
		if !isResidentCatalogFee(f.Category) {
			continue
		}
		if q := math.Abs(f.Qty); q > 0 {
			sum += q
		}
	}
	return round2(sum)
}

// isResidentCatalogFee 判断是否为居民目录电费分项。
//
// 兼容几种历史写法：`(5)目录电费`、`目录电费（居民）`、`目录电费-居民`。
// 必须排除工商业目录电费（那种行会带「工商业」字样）。
func isResidentCatalogFee(category string) bool {
	c := strings.TrimSpace(category)
	if c == "" {
		return false
	}
	if strings.Contains(c, "工商业") {
		return false
	}
	if !strings.Contains(c, "目录电费") {
		return false
	}
	return true
}

// RepairResidentReadingBillKwh 把居民电量明细的主行 `bill_kwh` 对齐到权威值。
//
// 返回是否发生变更，供启动回填统计与幂等判断。三条不变量：
//  1. 没有权威值（无 `(5)目录电费`）→ 不动。缺权威值时不猜，交给用户手改。
//  2. 居民读数超过一行 → 不动。多行是「电能表 + 定比」双口径（2025-07），
//     每行对应不同物理表计，覆盖主行会破坏另一行的语义。
//  3. 主行已是权威值 → 不动（幂等，重复启动不产生写入）。
func RepairResidentReadingBillKwh(item *types.UtilityBillExtractionItem) bool {
	if item == nil || len(item.ResidentialReadings) == 0 {
		return false
	}
	authoritative := ResidentBillingKwh(item)
	if authoritative <= 0 {
		return false
	}
	// 多行明细不做覆盖（见不变量 2）
	if len(item.ResidentialReadings) > 1 {
		return false
	}
	main := &item.ResidentialReadings[0]
	if main.BillKwh == authoritative {
		return false
	}
	main.BillKwh = authoritative
	return true
}

// ResidentKwhWithFallback 供列表/核算读取用：票面分项优先，缺失时退回主行缓存。
//
// 这条路径只**读**不写，保证前端在未跑回填时也能显示正确值
// （前端 residentInfoOf 用同一套优先级）。
func ResidentKwhWithFallback(item *types.UtilityBillExtractionItem) float64 {
	if v := ResidentBillingKwh(item); v > 0 {
		return v
	}
	if item == nil || len(item.ResidentialReadings) == 0 {
		return 0
	}
	// 与前端一致：优先 ratio≈0.015 的定比行，否则第一行
	main := item.ResidentialReadings[0]
	for _, r := range item.ResidentialReadings {
		if r.Ratio > 0 && r.Ratio < 0.05 {
			main = r
			break
		}
	}
	return round2(math.Abs(main.BillKwh))
}
