package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// 能耗台账（energy scope）级聚合列表。
//
// 与合同/制度/通知同形态：遍历知识库下全部 knowledge → 平铺能耗记录 →
// 服务端完成全字段搜索、分类/账期筛选、排序、分页与用量/金额聚合。
//
// 与合同列表的**关键差异**：主时间轴是**账期（月）**。
// 合同的 date_from/date_to 打在「签订日期」上；能耗的 period_from/period_to
// 打在「账期」上（YYYY-MM），读数日期另给 read_date_from/read_date_to。
// 这是业务决定的：能耗台账是按月结算的，用日粒度区间过滤账期没有意义。

// energyMetadata 与 handler.energyCustomMetadata 对齐，用于从 custom_metadata
// 反序列化能耗提取结果。
type energyMetadata struct {
	Kind             string           `json:"kind"`
	Records          []map[string]any `json:"records"`
	ExtractStatus    string           `json:"extract_status"`
	ExtractError     string           `json:"extract_error"`
	AutoDeletedCount int              `json:"auto_deleted_count"`
	// FleetCertType 文件级归档分类（上传时打标 / 改名传播后的权威值）。
	FleetCertType string `json:"fleet_cert_type"`
}

// ListEnergyRecords 实现能耗台账级聚合列表。
func (s *BusinessExtractService) ListEnergyRecords(
	ctx context.Context, kbID string, filter types.EnergyListFilter,
) (*types.EnergyListResult, error) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 10000 {
		filter.PageSize = 10000
	}

	records := make([]types.EnergyRecord, 0, 256)
	page := 1
	const batch = 1000
	// 分类归属权威集合：启用分类名（服务端从 fleet_categories 实时加载）。
	enabledCats := enabledCategorySetFromFilter(filter.EnabledCategories)
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return nil, fmt.Errorf("list knowledge for energy records: %w", err)
		}
		if len(knowledges) == 0 {
			break
		}
		// 加载标签（与其它文档列表保持一致）
		{
			ids := make([]string, 0, len(knowledges))
			for _, k := range knowledges {
				if k != nil {
					ids = append(ids, k.ID)
				}
			}
			if tagMap, err := s.repo.GetKnowledgeTags(ctx, ids); err == nil {
				for _, k := range knowledges {
					if k != nil {
						if tags, ok := tagMap[k.ID]; ok {
							k.Tags = tags
						}
					}
				}
			}
		}
		for _, k := range knowledges {
			if k == nil || len(k.CustomMetadata) == 0 {
				continue
			}
			// 解析/提取中的文件不生成占位行（与合同同口径）：它们由顶部状态条
			// 与「历史记录」抽屉承载，主列表只放真实记录。
			if k.ParseStatus == "pending" || k.ParseStatus == "processing" || k.ParseStatus == "finalizing" {
				continue
			}
			var meta energyMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil {
				continue
			}
			legacyManual := meta.Kind != "energy" && meta.AutoDeletedCount > 0
			if meta.Kind != "energy" && !legacyManual {
				continue
			}
			tags := make([]string, 0, len(k.Tags))
			for _, t := range k.Tags {
				if t != nil && t.Name != "" {
					tags = append(tags, t.Name)
				}
			}
			// 待补录占位行：用户从删除历史恢复的记录必须立即可见可编辑。
			if legacyManual {
				records = append(records, types.EnergyRecord{
					KnowledgeID:    k.ID,
					KnowledgeTitle: k.Title,
					FileName:       k.FileName,
					FileType:       k.FileType,
					Tags:           tags,
					ExtractStatus:  "manual",
					ExtractError:   meta.ExtractError,
					KBID:           k.KnowledgeBaseID,
					CreatedAt:      k.CreatedAt,
				})
				continue
			}
			if len(meta.Records) == 0 {
				continue
			}
			for _, rm := range meta.Records {
				// 读数反算 + 用量守卫（口径与提取后处理完全一致：同一纯函数）
				rec := energyRecordFromMap(rm)
				if energyRecordBlank(rec) {
					continue
				}
				energyRecordSystemFields(&rec, k, tags, meta)
				// 分类归属唯一口径：与合同同款候选链
				// [文件级 fleet_cert_type, 记录级 utility_type]
				rec.UtilityType, _ = resolveBusinessCategory(
					meta.FleetCertType, rec.UtilityType, "", enabledCats)
				records = append(records, rec)
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}

	// 过滤
	kw := strings.ToLower(strings.TrimSpace(filter.Keyword))
	kept := records[:0]
	var sumUsage, sumTotal, sumPrev, sumCurr float64
	issueCount := 0
	for _, r := range records {
		if filter.UtilityType != "" && r.UtilityType != filter.UtilityType {
			continue
		}
		if filter.Status != "" && r.ExtractStatus != filter.Status {
			continue
		}
		// 账期精确匹配
		if filter.BillingPeriod != "" && r.BillingPeriod != filter.BillingPeriod {
			continue
		}
		// 账期区间（YYYY-MM 月粒度）
		if !energyPeriodInRange(r.BillingPeriod, filter.PeriodFrom, filter.PeriodTo) {
			continue
		}
		// 抄表日期区间（日粒度）
		if !energyDateInRange(r.ReadDate, filter.ReadDateFrom, filter.ReadDateTo) {
			continue
		}
		if kw != "" && !energyRecordMatchKeyword(r, kw) {
			continue
		}
		kept = append(kept, r)
		if r.UsageAmount != nil {
			sumUsage += *r.UsageAmount
		}
		if r.TotalAmount != nil {
			sumTotal += *r.TotalAmount
		}
		if r.PreviousReading != nil {
			sumPrev += *r.PreviousReading
		}
		if r.CurrentReading != nil {
			sumCurr += *r.CurrentReading
		}
		if len(r.Issues) > 0 {
			issueCount++
		}
	}
	records = kept

	// 排序：源文件创建时间新在前；同文件多记录保持提取顺序。
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})

	total := len(records)
	start := (filter.Page - 1) * filter.PageSize
	if start > total {
		start = total
	}
	end := start + filter.PageSize
	if end > total {
		end = total
	}
	return &types.EnergyListResult{
		Data:        records[start:end],
		Total:       total,
		SumUsage:    sumUsage,
		SumTotal:    sumTotal,
		SumPrevious: sumPrev,
		SumCurrent:  sumCurr,
		IssueCount:  issueCount,
		Page:        filter.Page,
		PageSize:    filter.PageSize,
	}, nil
}

// energyRecordSystemFields 补齐能耗记录的系统字段（知识库归属 / 标签 / 状态 / 时间）。
//
// 数值指针化已在反算层完成（energyRecordFromMap），此处只补系统维度，
// 避免同一份数值在两层里各转一次（历史上发票就出过这种双转漂移）。
func energyRecordSystemFields(rec *types.EnergyRecord, k *types.Knowledge, tags []string, meta energyMetadata) {
	rec.KnowledgeID = k.ID
	rec.KnowledgeTitle = k.Title
	rec.FileName = k.FileName
	rec.FileType = k.FileType
	rec.Tags = tags
	rec.ExtractStatus = meta.ExtractStatus
	rec.ExtractError = meta.ExtractError
	rec.KBID = k.KnowledgeBaseID
	rec.CreatedAt = k.CreatedAt
	// 文件级解析状态必须带回去：状态列徽标的判定依赖它，
	// 缺了会让已提取成功的记录被判成「待解析」。
	rec.ParseStatus = k.ParseStatus
}

// energyRecordBlank 判断能耗记录是否为空提取（所有关键字段都空）。
func energyRecordBlank(r types.EnergyRecord) bool {
	if strings.TrimSpace(r.UtilityNo) != "" || strings.TrimSpace(r.BillingPeriod) != "" ||
		strings.TrimSpace(r.UtilityType) != "" || strings.TrimSpace(r.AccountName) != "" {
		return false
	}
	if r.UsageAmount != nil && *r.UsageAmount != 0 {
		return false
	}
	if r.TotalAmount != nil && *r.TotalAmount != 0 {
		return false
	}
	if len(r.Items) > 0 {
		return false
	}
	return true
}

// energyPeriodInRange 判断账期（YYYY-MM）是否落在 [from, to]（含，空端不限）。
//
// 月粒度比较：去掉分隔符后按字符串比（YYYYMM 字典序 = 时间序），
// 因此 "2026-09" 与 "202609" 等价。
func energyPeriodInRange(period, from, to string) bool {
	if from == "" && to == "" {
		return true
	}
	period = strings.TrimSpace(period)
	if period == "" {
		return false
	}
	norm := func(s string) string {
		s = strings.TrimSpace(s)
		s = strings.ReplaceAll(s, "-", "")
		s = strings.ReplaceAll(s, "年", "")
		s = strings.ReplaceAll(s, "月", "")
		s = strings.ReplaceAll(s, "/", "")
		s = strings.ReplaceAll(s, ".", "")
		return s
	}
	p := norm(period)
	// 账期只写到月，区间端点可能给到日（2026-09-15）：取前 6 位比较
	if len(p) > 6 {
		p = p[:6]
	}
	if len(p) != 6 {
		return false
	}
	if from != "" {
		f := norm(from)
		if len(f) > 6 {
			f = f[:6]
		}
		if len(f) == 6 && p < f {
			return false
		}
	}
	if to != "" {
		t := norm(to)
		if len(t) > 6 {
			t = t[:6]
		}
		if len(t) == 6 && p > t {
			return false
		}
	}
	return true
}

// energyDateInRange 判断日期（YYYY-MM-DD）是否落在 [from, to]（含，空端不限）。
func energyDateInRange(date, from, to string) bool {
	date = strings.TrimSpace(date)
	if from == "" && to == "" {
		return true
	}
	if date == "" {
		return false
	}
	norm := func(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), "-", "") }
	d := norm(date)
	if from != "" && d < norm(from) {
		return false
	}
	if to != "" && d > norm(to) {
		return false
	}
	return true
}

// energyRecordMatchKeyword 判断记录是否命中全字段搜索词（大小写不敏感）。
//
// 搜索词在函数内部自行归一（小写 + 去空白）：不依赖调用方预处理 ——
// 否则新调用方漏做 ToLower 就会静默漏匹配（这类"契约靠自觉"的写法历史上
// 在合同/发票模块都踩过：搜索框输入大写字母永远搜不到）。
func energyRecordMatchKeyword(r types.EnergyRecord, kw string) bool {
	kw = strings.ToLower(strings.TrimSpace(kw))
	if kw == "" {
		return true
	}
	fields := []string{
		r.UtilityNo, r.BillingPeriod, r.UtilityType,
		r.AccountName, r.MeterNo, r.Address,
		r.PeriodStart, r.PeriodEnd, r.ReadDate,
		r.UsageCategory, r.VoltageLevel, r.Remark,
		r.FileName, r.KnowledgeTitle, r.ExtractStatus, r.ExtractError,
	}
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), kw) {
			return true
		}
	}
	for _, t := range r.Tags {
		if strings.Contains(strings.ToLower(t), kw) {
			return true
		}
	}
	for _, v := range []*float64{
		r.PreviousReading, r.CurrentReading, r.Multiplier,
		r.UsageAmount, r.UnitPrice, r.TotalAmount,
	} {
		if v != nil && strings.Contains(strconv.FormatFloat(*v, 'f', -1, 64), kw) {
			return true
		}
	}
	// 明细行里的表号也参与搜索
	for _, it := range r.Items {
		if strings.Contains(strings.ToLower(it.MeterNo), kw) ||
			strings.Contains(strings.ToLower(it.MeterType), kw) {
			return true
		}
	}
	return false
}

// EnergyOverviewStats 计算能耗台账概览统计。口径与 ListEnergyRecords 平铺完全一致：
// 全量聚合，不接收任何筛选参数，因此不受列表的分类/账期/搜索筛选影响。
func (s *BusinessExtractService) EnergyOverviewStats(
	ctx context.Context, kbID string, enabledCategories ...string,
) (*types.EnergyOverviewStats, error) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	enabledCats := enabledCategorySetFromFilter(enabledCategories)

	stats := &types.EnergyOverviewStats{
		ByUtilityType: []types.EnergyTypeStat{},
		ByCategory:    []types.EnergyCategoryStat{},
	}
	typeBuckets := map[string]*types.EnergyTypeStat{}
	catBuckets := map[string]*types.EnergyCategoryStat{}

	page := 1
	const batch = 1000
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return nil, fmt.Errorf("list knowledge for energy overview: %w", err)
		}
		if len(knowledges) == 0 {
			break
		}
		for _, k := range knowledges {
			if k == nil {
				continue
			}
			stats.FileCount++
			if k.ParseStatus == "failed" {
				stats.ParseFailed++
			}
			if len(k.CustomMetadata) == 0 {
				continue
			}
			var meta energyMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil {
				continue
			}
			if meta.Kind != "energy" {
				continue
			}
			if meta.ExtractStatus == "failed" {
				stats.ExtractFailed++
			}
			for _, rm := range meta.Records {
				rec := energyRecordFromMap(rm)
				if energyRecordBlank(rec) {
					continue
				}
				rec.UtilityType, _ = resolveBusinessCategory(
					meta.FleetCertType, rec.UtilityType, "", enabledCats)
				stats.Total++
				usage := 0.0
				if rec.UsageAmount != nil {
					usage = *rec.UsageAmount
				}
				total := 0.0
				if rec.TotalAmount != nil {
					total = *rec.TotalAmount
				}
				stats.SumUsage += usage
				stats.SumTotal += total
				if len(rec.Issues) > 0 {
					stats.IssueCount++
				}
				// 能耗类型桶
				ut := rec.UtilityType
				if ut == "" {
					ut = "其它能耗"
				}
				b, ok := typeBuckets[ut]
				if !ok {
					b = &types.EnergyTypeStat{UtilityType: ut}
					typeBuckets[ut] = b
				}
				b.Count++
				b.SumUsage += usage
				b.SumTotal += total
				// 分类桶（与类型同口径，用户自定义分类名）
				cb, ok := catBuckets[ut]
				if !ok {
					cb = &types.EnergyCategoryStat{Category: ut}
					catBuckets[ut] = cb
				}
				cb.Count++
				cb.SumUsage += usage
				cb.SumTotal += total
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}

	for _, b := range typeBuckets {
		stats.ByUtilityType = append(stats.ByUtilityType, *b)
	}
	sort.Slice(stats.ByUtilityType, func(i, j int) bool {
		return stats.ByUtilityType[i].Count > stats.ByUtilityType[j].Count
	})
	for _, b := range catBuckets {
		stats.ByCategory = append(stats.ByCategory, *b)
	}
	sort.Slice(stats.ByCategory, func(i, j int) bool {
		return stats.ByCategory[i].Count > stats.ByCategory[j].Count
	})
	return stats, nil
}

// ListEnergyTypes 返回该知识库下所有能耗类型（去重 + 计数），供筛选下拉使用。
func (s *BusinessExtractService) ListEnergyTypes(ctx context.Context, kbID string) ([]types.EnergyTypeCount, error) {
	result, err := s.ListEnergyRecords(ctx, kbID, types.EnergyListFilter{Page: 1, PageSize: 10000})
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, r := range result.Data {
		t := strings.TrimSpace(r.UtilityType)
		if t == "" {
			continue
		}
		counts[t]++
	}
	out := make([]types.EnergyTypeCount, 0, len(counts))
	for t, c := range counts {
		out = append(out, types.EnergyTypeCount{UtilityType: t, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].UtilityType < out[j].UtilityType
	})
	return out, nil
}
