package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// awardPunishMetadata mirrors the persisted custom_metadata shape for a
// knowledge row in the award/punish KB (see handler awardPunishCustomMetadata).
type awardPunishMetadata struct {
	Kind             string                            `json:"kind"`
	Records          []types.AwardPunishExtractionItem `json:"records"`
	ExtractStatus    string                            `json:"extract_status"`
	ExtractError     string                            `json:"extract_error"`
	AutoDeletedCount int                               `json:"auto_deleted_count"`
	// FleetCertType 文件级归档分类（上传时打标 / 改名传播后的权威值），
	// 分类归属候选链的第一环，见 resolveBusinessCategory。
	FleetCertType string `json:"fleet_cert_type"`
}

// ListAwardPunishRecords returns the paginated award/punish list of one KB.
// Records are NOT deduplicated by 文号 — the knowledge upload layer already
// deduplicates files by hash, and one notice legitimately shares one 文号
// across several persons.
func (s *BusinessExtractService) ListAwardPunishRecords(ctx context.Context, kbID string, filter types.AwardPunishListFilter) (*types.AwardPunishListResult, error) {
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

	records := make([]types.AwardPunishRecord, 0, 256)
	page := 1
	const batch = 1000
	// 分类归属权威集合：启用分类名（服务端从 fleet_categories 实时加载）
	enabledCats := enabledCategorySetFromFilter(filter.EnabledCategories)
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return nil, fmt.Errorf("list knowledge for award/punish records: %w", err)
		}
		if len(knowledges) == 0 {
			break
		}
		// 加载标签（与文档列表保持一致）
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
			// 解析中的文件由顶部状态条与「历史记录 → 正在执行」展示，不进主列表
			if k.ParseStatus == "pending" || k.ParseStatus == "processing" || k.ParseStatus == "finalizing" {
				continue
			}
			var meta awardPunishMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil {
				continue
			}
			// 旧版恢复行兜底：meta 无 kind 但保留 auto_deleted_count → 视作待补录占位行。
			legacyManual := meta.Kind != "award_punish" && meta.AutoDeletedCount > 0
			if meta.Kind != "award_punish" && !legacyManual {
				continue
			}
			tags := make([]string, 0, len(k.Tags))
			for _, t := range k.Tags {
				if t != nil && t.Name != "" {
					tags = append(tags, t.Name)
				}
			}
			// 人工入库/空提取占位行：恢复后 extract_status=manual 或提取后仍无有效字段
			// 的记录必须显示在列表（用户可点开编辑补录），避免刷新后消失。
			// 占位行只保留**人工恢复**这一种（用户点过「重新入库」）。
			// 提取成功但全空的空壳不进主列表（2026-10-22 真机事故）。
			if awardPunishItemsAllBlank(meta.Records) && legacyManual {
				records = append(records, types.AwardPunishRecord{
					RowKey:         k.ID + "-manual",
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
			for idx, r := range meta.Records {
				// 过滤"空提取"记录（无文号、无标题、无类型、无当事人、无部门）
				if awardPunishItemBlank(r) {
					continue
				}
				// 分类归属唯一口径：候选链第一命中同时写 ApType
				resolvedType, _ := resolveBusinessCategory(meta.FleetCertType, r.ApType, "", enabledCats)
				records = append(records, types.AwardPunishRecord{
					RowKey:        fmt.Sprintf("%s-%d", k.ID, idx),
					ApNo:          r.ApNo,
					ApTitle:       r.ApTitle,
					ApType:        resolvedType,
					Person:        r.Person,
					Dept:          r.Dept,
					IssueDept:     r.IssueDept,
					Position:      r.Position,
					Measure:       r.Measure,
					Basis:         r.Basis,
					Signer:        r.Signer,
					SignDate:      r.SignDate,
					EffectiveDate: r.EffectiveDate,
					Scope:         r.Scope,
					Contact:       r.Contact,
					ContactPhone:  r.ContactPhone,
					Urgency:       r.Urgency,
					ValidUntil:    r.ValidUntil,
					Attachments:   r.Attachments,
					Remark:        r.Remark,
					// 摘要复用知识库已生成的文档摘要（与文档详情同源）
					Summary:        ReuseKnowledgeSummary(r.Summary, k.Description),
					Page:           r.Page,
					Fields:         r.Fields,
					KnowledgeID:    k.ID,
					KnowledgeTitle: k.Title,
					FileName:       k.FileName,
					FileType:       k.FileType,
					Tags:           tags,
					ExtractStatus:  meta.ExtractStatus,
					ExtractError:   meta.ExtractError,
					KBID:           k.KnowledgeBaseID,
					CreatedAt:      k.CreatedAt,
				})
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}

	// 过滤（不做文号去重——文件层已按哈希去重）
	kw := strings.ToLower(strings.TrimSpace(filter.Keyword))
	dateFrom, dateTo := strings.TrimSpace(filter.DateFrom), strings.TrimSpace(filter.DateTo)
	kept := records[:0]
	for _, r := range records {
		if filter.ApType != "" {
			// 「其它通知」为排除式桶，与 AwardPunishOverviewStats 卡片口径一致：
			// ap_type 不属于启用分类集合（含空/已删除分类旧值）的记录归入此筛选。
			// 前端卡片计数 = 总数 - 各启用分类卡之和。
			if filter.ApType == "其它通知" {
				if len(filter.KnownCategories) > 0 {
					known := make(map[string]struct{}, len(filter.KnownCategories))
					for _, name := range filter.KnownCategories {
						// 「其它通知」本身是排除式桶的归属目标，不作为「已分类」剔除
						if name == "其它通知" {
							continue
						}
						known[name] = struct{}{}
					}
					if r.ApType != "" {
						if _, ok := known[r.ApType]; ok {
							continue
						}
					}
				} else if r.ApType != "" && r.ApType != "其它通知" {
					// 向后兼容：未传分类集合时，非空且非其它通知的记录排除
					continue
				}
			} else if r.ApType != filter.ApType {
				continue
			}
		}
		if filter.Status != "" && r.ExtractStatus != filter.Status {
			continue
		}
		if !awardPunishDateInRange(r.SignDate, dateFrom, dateTo) {
			continue
		}
		if kw != "" && !awardPunishRecordMatchKeyword(r, kw) {
			continue
		}
		kept = append(kept, r)
	}
	records = kept

	// 排序：源文件创建时间新在前
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
	return &types.AwardPunishListResult{
		Data:     records[start:end],
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}, nil
}

// awardPunishItemsAllBlank 判断奖惩提取列表是否完全没有有效数据（空数组或每项都空提取）。
func awardPunishItemsAllBlank(recs []types.AwardPunishExtractionItem) bool {
	if len(recs) == 0 {
		return true
	}
	for _, r := range recs {
		if !awardPunishItemBlank(r) {
			return false
		}
	}
	return true
}

// awardPunishItemBlank 判断单条奖惩记录是否空提取（无文号、无标题、无类型、无当事人、无部门）。
func awardPunishItemBlank(r types.AwardPunishExtractionItem) bool {
	return strings.TrimSpace(r.ApNo) == "" &&
		strings.TrimSpace(r.ApTitle) == "" &&
		strings.TrimSpace(r.ApType) == "" &&
		strings.TrimSpace(r.Person) == "" &&
		strings.TrimSpace(r.Dept) == ""
}

// awardPunishDateInRange 判断签发日期是否落在 [from, to] 区间（含边界，空则不过滤）。
func awardPunishDateInRange(date, from, to string) bool {
	if from == "" && to == "" {
		return true
	}
	d := strings.TrimSpace(date)
	if d == "" {
		return false
	}
	if from != "" && d < from {
		return false
	}
	if to != "" && d > to {
		return false
	}
	return true
}

// awardPunishRecordMatchKeyword 判断记录是否命中全字段搜索词（大小写不敏感）。
func awardPunishRecordMatchKeyword(r types.AwardPunishRecord, kw string) bool {
	fields := []string{
		r.ApNo, r.ApTitle, r.ApType, r.Person, r.Dept, r.IssueDept, r.Position,
		r.Measure, r.Basis, r.Signer, r.SignDate, r.EffectiveDate,
		r.Remark, r.Summary,
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
	return false
}

// ListAwardPunishTypes 返回该知识库下所有奖惩出现过的去重奖惩类型（含数量），
// 用于前端奖惩类型筛选下拉自动加载。
func (s *BusinessExtractService) ListAwardPunishTypes(ctx context.Context, kbID string) ([]types.AwardPunishTypeCount, error) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	page := 1
	batch := 100
	seen := map[string]int{}
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return nil, err
		}
		if len(knowledges) == 0 {
			break
		}
		for _, k := range knowledges {
			if k == nil || len(k.CustomMetadata) == 0 {
				continue
			}
			// 解析中的文件由顶部状态条与「历史记录 → 正在执行」展示，不进主列表
			if k.ParseStatus == "pending" || k.ParseStatus == "processing" || k.ParseStatus == "finalizing" {
				continue
			}
			var meta awardPunishMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil || meta.Kind != "award_punish" {
				continue
			}
			for _, r := range meta.Records {
				if t := strings.TrimSpace(r.ApType); t != "" {
					seen[t]++
				}
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}
	// 固定枚举顺序优先，再补自定义类型
	ordered := []types.AwardPunishTypeCount{}
	for _, t := range []string{"通知", "公告", "通报", "决定", "其它通知"} {
		if c, ok := seen[t]; ok {
			ordered = append(ordered, types.AwardPunishTypeCount{ApType: t, Count: c})
			delete(seen, t)
		}
	}
	for t, c := range seen {
		ordered = append(ordered, types.AwardPunishTypeCount{ApType: t, Count: c})
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Count > ordered[j].Count
	})
	return ordered, nil
}

// AwardPunishOverviewStats 计算奖惩台账概览统计。口径与 ListAwardPunishRecords 平铺完全一致：
// 遍历知识库下全部 knowledge → 平铺奖惩记录（含待补录占位行）→ 全量聚合，不接收任何筛选参数，
// 因此不受列表类型/日期/搜索筛选影响（发票基准 InvoiceOverviewStats 同款形态）。
func (s *BusinessExtractService) AwardPunishOverviewStats(ctx context.Context, kbID string) (*types.AwardPunishOverviewStats, error) {
	tenantID, _ := ctx.Value(types.TenantIDContextKey).(uint64)

	type flatRec struct {
		apType   string
		signDate string
	}
	records := make([]flatRec, 0, 256)
	parseFailedFiles := 0
	fileCount := 0
	page := 1
	const batch = 1000
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return nil, fmt.Errorf("list knowledge for award_punish overview: %w", err)
		}
		if len(knowledges) == 0 {
			break
		}
		for _, k := range knowledges {
			if k == nil {
				continue
			}
			if k.ParseStatus == "failed" {
				parseFailedFiles++
				fileCount++
			}
			if len(k.CustomMetadata) == 0 {
				continue
			}
			var meta awardPunishMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil {
				continue
			}
			legacyManual := meta.Kind != "award_punish" && meta.AutoDeletedCount > 0
			if meta.Kind != "award_punish" && !legacyManual {
				continue
			}
			if k.ParseStatus != "failed" {
				fileCount++
			}
			// 占位行只保留**人工恢复**这一种（用户点过「重新入库」）。
			// 提取成功但全空的空壳不进主列表（2026-10-22 真机事故）。
			if awardPunishItemsAllBlank(meta.Records) && legacyManual {
				records = append(records, flatRec{})
				continue
			}
			if len(meta.Records) == 0 {
				continue
			}
			for _, r := range meta.Records {
				if awardPunishItemBlank(r) {
					continue
				}
				records = append(records, flatRec{apType: r.ApType, signDate: r.SignDate})
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}

	stats := &types.AwardPunishOverviewStats{
		ByApType:   []types.AwardPunishTypeStat{},
		ByCategory: []types.NoticeCategoryStat{},
	}
	stats.Total = len(records)
	stats.FileCount = fileCount
	stats.ParseFailed = parseFailedFiles

	now := time.Now()
	monthPrefix := now.Format("2006-01")
	monthPrefixCompact := now.Format("200601")
	typeCount := make(map[string]int)
	// 按用户定义分类（ap_type 精确值）分组：ap_type 为空/空白归「未分类」。
	catByMap := map[string]*types.NoticeCategoryStat{}
	for _, r := range records {
		d := strings.TrimSpace(r.signDate)
		if d != "" && (strings.HasPrefix(d, monthPrefix) || strings.HasPrefix(d, monthPrefixCompact)) {
			stats.CurrentMonth++
		}
		t := strings.TrimSpace(r.apType)
		if t == "" {
			t = "其它通知"
		}
		typeCount[t]++

		cat := strings.TrimSpace(r.apType)
		if cat == "" {
			cat = "未分类"
		}
		c := catByMap[cat]
		if c == nil {
			c = &types.NoticeCategoryStat{Category: cat}
			catByMap[cat] = c
		}
		c.Count++
	}
	for t, c := range typeCount {
		stats.ByApType = append(stats.ByApType, types.AwardPunishTypeStat{ApType: t, Count: c})
	}
	sort.Slice(stats.ByApType, func(i, j int) bool { return stats.ByApType[i].Count > stats.ByApType[j].Count })
	for _, t := range catByMap {
		stats.ByCategory = append(stats.ByCategory, *t)
	}
	// 分类桶按份数降序、分类名升序，稳定排序（与发票/合同/制度 ByCategory 一致）
	sort.Slice(stats.ByCategory, func(i, j int) bool {
		if stats.ByCategory[i].Count != stats.ByCategory[j].Count {
			return stats.ByCategory[i].Count > stats.ByCategory[j].Count
		}
		return stats.ByCategory[i].Category < stats.ByCategory[j].Category
	})
	return stats, nil
}
