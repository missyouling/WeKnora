package handler

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// BackfillFleetRecordKnowledgeIDs 幂等回填车队台账的源文件关联（第 44 轮）。
//
// 真机事故（图 2）：在「车辆档案 → 道路运输证」勾选一条已提取的记录后，
// 底部浮动工具栏的「重新解析 / 重新提取」恒为禁用。前端直接成因是它按
// `row.__file` 找文件 ID（台账行没有这个标记），但**数据侧**还有一个隐患：
// 历史回填（第 39 轮）与外部导入写下的 `fleet_records` 行可能没有
// `doc_knowledge_id`，即使前端改对了也拿不到文件。
//
// 这里按「同租户 + 同文件名」把台账行关联回 `knowledges`，规则刻意保守：
//   - 只补空值，已有 `doc_knowledge_id` 的行一行都不碰（幂等）；
//   - 文件名必须在**同租户内唯一命中**，多个同名文件（重复上传）宁可留空，
//     也不猜关联 —— 猜错会让「重新提取」覆盖另一份文件的结果；
//   - 只处理三个上传提取类 record_type（手工台账没有源文件）。
func (h *FleetHandler) backfillFleetRecordKnowledgeIDs(ctx context.Context) {
	if h == nil || h.db == nil {
		return
	}
	archiveTypes := []string{
		types.FleetRecordVehicleArchive,
		types.FleetRecordDriverArchive,
		types.FleetRecordMaintainArchive,
	}

	type orphanRow struct {
		ID        string `gorm:"column:id"`
		TenantID  int64  `gorm:"column:tenant_id"`
		FileName  string `gorm:"column:file_name"`
	}
	var orphans []orphanRow
	if err := h.db.WithContext(ctx).
		Table("fleet_records").
		Select("id, tenant_id, file_name").
		Where("deleted_at IS NULL").
		Where("record_type IN ?", archiveTypes).
		Where("COALESCE(doc_knowledge_id, '') = ''").
		Where("COALESCE(file_name, '') <> ''").
		Find(&orphans).Error; err != nil {
		logger.Warnf(ctx, "fleet knowledge-id backfill: query records failed: %v", err)
		return
	}
	if len(orphans) == 0 {
		return
	}

	// 按租户收敛文件名集合，一次查回候选文件（避免逐行查库）。
	type key struct {
		tenant int64
		name   string
	}
	namesByTenant := make(map[int64][]string, 4)
	want := make(map[key]bool, len(orphans))
	for _, o := range orphans {
		name := strings.TrimSpace(o.FileName)
		if name == "" {
			continue
		}
		want[key{o.TenantID, name}] = true
		namesByTenant[o.TenantID] = append(namesByTenant[o.TenantID], name)
	}

	type knowledgeRow struct {
		ID       string `gorm:"column:id"`
		TenantID int64  `gorm:"column:tenant_id"`
		FileName string `gorm:"column:file_name"`
	}
	// 同租户 + 同文件名 → 命中次数；只接受唯一命中。
	hits := make(map[key][]string, len(orphans))
	for tenant, names := range namesByTenant {
		var rows []knowledgeRow
		if err := h.db.WithContext(ctx).
			Table("knowledges").
			Select("id, tenant_id, file_name").
			Where("deleted_at IS NULL AND tenant_id = ?", tenant).
			Where("file_name IN ?", names).
			Find(&rows).Error; err != nil {
			logger.Warnf(ctx, "fleet knowledge-id backfill: query knowledges failed (tenant=%d): %v", tenant, err)
			continue
		}
		for _, r := range rows {
			k := key{r.TenantID, strings.TrimSpace(r.FileName)}
			if !want[k] {
				continue
			}
			hits[k] = append(hits[k], r.ID)
		}
	}

	linked := 0
	for _, o := range orphans {
		k := key{o.TenantID, strings.TrimSpace(o.FileName)}
		ids := hits[k]
		if len(ids) != 1 {
			continue
		}
		if err := h.db.WithContext(ctx).
			Table("fleet_records").
			Where("id = ? AND COALESCE(doc_knowledge_id, '') = ''", o.ID).
			Update("doc_knowledge_id", ids[0]).Error; err != nil {
			logger.Warnf(ctx, "fleet knowledge-id backfill: update %s failed: %v", o.ID, err)
			continue
		}
		linked++
	}
	if linked > 0 {
		logger.Infof(ctx, "fleet knowledge-id backfill: linked %d records to source files", linked)
	}
}
