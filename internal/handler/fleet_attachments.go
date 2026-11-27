package handler

// 随单「非解析附件袋」（第 78 轮）。
//
// 业务诉求：二级维护竣工合格证、维修结算清单这些单据在现实中是**一叠纸**——
// 主证之外还有竣工检测单、维修发票、过程照片。这些附件只需要「随单留存 + 页内秒看」，
// **完全不需要 OCR/LLM 提取**。
//
// 铁律（本文件必须遵守）：
//  1. 纯文件直存 PostgreSQL BYTEA，上传链路**严禁**触发任何解析任务或大模型调用
//     （TestRound78 会扫描本文件确认不含 Extract* / chatModel 调用）；
//  2. 预览走**无后缀路由** `/fleet/attachments/view?id=...`：生产 VPS 的边缘 Nginx
//     会用静态资源正则拦掉 `.png` / `.pdf` 这类带后缀的路径，请求根本到不了后端
//     （第 61 轮离职证明 404 的同一个坑）；
//  3. 列表接口不返回 BYTEA（几十 MB 的图片会把列表接口拖垮）。

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// 附件袋允许的 MIME 类型（与前端 accept 一致）。
var fleetAttachmentMimeTypes = map[string]bool{
	"application/pdf": true,
	"image/png":       true,
	"image/jpeg":      true,
	"image/jpg":       true,
	"image/webp":      true,
}

// 单个附件上限 20MB：合格证扫描件 300dpi 约 2~5MB，20MB 足够且不会打爆 PG 行大小。
const fleetAttachmentMaxBytes = 20 << 20

// 附件标签白名单（前端下拉同源）。
var fleetAttachmentTags = []string{"竣工检测单", "维修发票", "结算清单", "过程照片", "其它附件"}

// FleetRecordAttachment 随单附件（元数据 + BYTEA）。
//
// Content 用 json:"-" 排除在序列化之外：列表接口直接复用本结构体，
// 只要不显式赋值 Content，BYTEA 永远不会出现在 JSON 里。
type FleetRecordAttachment struct {
	ID        string    `gorm:"primaryKey;size:64" json:"id"`
	TenantID  int64     `gorm:"index" json:"tenant_id"`
	RecordID  string    `gorm:"index;size:64" json:"record_id"`
	FileName  string    `gorm:"size:255" json:"file_name"`
	MimeType  string    `gorm:"size:128" json:"mime_type"`
	FileSize  int64     `json:"file_size"`
	Tag       string    `gorm:"size:64" json:"tag"`
	Content   []byte    `gorm:"type:bytea" json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName 固定表名（不随 GORM 复数规则漂移）。
func (FleetRecordAttachment) TableName() string { return "fleet_record_attachments" }

// EnsureFleetAttachmentSchema 启动期建表（由 RegisterFleetRoutes 调用一次）。
//
// 与请求期 ensure 同体：启动建表能保证「打开抽屉看附件」这条只读路径
// 不依赖「先有人上传过」。
func (h *FleetHandler) EnsureFleetAttachmentSchema() {
	h.ensureFleetAttachmentSchema(context.Background())
}

// ensureFleetAttachmentSchema 幂等建表。
//
// 为什么不用 AutoMigrate 全库跑：WeKnora 的迁移由上游管理，本表是二开新增，
// 用 AutoMigrate 单表 + IF NOT EXISTS 语义最安全（重复执行无害）。
func (h *FleetHandler) ensureFleetAttachmentSchema(ctx context.Context) {
	if err := h.db.WithContext(ctx).AutoMigrate(&FleetRecordAttachment{}); err != nil {
		logger.Warnf(ctx, "ensure fleet_record_attachments schema failed: %v", err)
	}
}

// fleetAttachmentMimeOf 归一 MIME：浏览器上传的 image/jpg 统一成 image/jpeg。
func fleetAttachmentMimeOf(raw, fileName string) string {
	ct := strings.ToLower(strings.TrimSpace(strings.Split(raw, ";")[0]))
	if ct == "image/jpg" {
		ct = "image/jpeg"
	}
	if fleetAttachmentMimeTypes[ct] {
		return ct
	}
	// 浏览器有时给 application/octet-stream：按扩展名兜底
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	}
	return ""
}

// ListFleetRecordAttachments GET /fleet/records/:id/attachments
//
// 只返回元数据（不含 BYTEA），按上传时间升序。
func (h *FleetHandler) ListFleetRecordAttachments(c *gin.Context) {
	ctx := c.Request.Context()
	uid, _ := fleetTenantID(c)
	tenantID := int64(uid)
	recordID := strings.TrimSpace(c.Param("id"))
	if recordID == "" {
		c.Error(errors.NewBadRequestError("记录编号不能为空"))
		return
	}
	// 建表必须在这里也做一次：用户可能先打开抽屉（List）再上传，
	// 此时表还不存在 → 直接 500（真机 A5.4 的根因）。
	h.ensureFleetAttachmentSchema(ctx)
	var rows []FleetRecordAttachment
	// Select 显式列出列，绝不带 content —— 这是「列表不返回 BYTEA」的硬保证
	if err := h.db.WithContext(ctx).
		Select("id, tenant_id, record_id, file_name, mime_type, file_size, tag, created_at").
		Where("tenant_id = ? AND record_id = ?", tenantID, recordID).
		Order("created_at ASC").Find(&rows).Error; err != nil {
		logger.Errorf(ctx, "list fleet attachments failed: %v", err)
		c.Error(errors.NewInternalServerError("list attachments failed"))
		return
	}
	if rows == nil {
		rows = []FleetRecordAttachment{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"items": rows,
		"tags":  fleetAttachmentTags,
	}})
}

// UploadFleetRecordAttachments POST /fleet/records/:id/attachments
//
// 支持一次多文件（multipart/form-data，字段名 files）。可选 tag 字段统一作用于本批。
//
// **绝不触发解析**：本函数只做 MIME 校验 + 落库，不调用任何 Extract*/LLM 链路。
func (h *FleetHandler) UploadFleetRecordAttachments(c *gin.Context) {
	ctx := c.Request.Context()
	uid, _ := fleetTenantID(c)
	tenantID := int64(uid)
	recordID := strings.TrimSpace(c.Param("id"))
	if recordID == "" {
		c.Error(errors.NewBadRequestError("记录编号不能为空"))
		return
	}
	// 记录必须存在且属于当前租户：否则会挂出「孤儿附件」，永远打不开
	var rec types.FleetRecord
	if err := h.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", recordID, tenantID).
		First(&rec).Error; err != nil {
		c.Error(errors.NewNotFoundError("记录不存在"))
		return
	}
	h.ensureFleetAttachmentSchema(ctx)

	tag := strings.TrimSpace(c.PostForm("tag"))
	if tag == "" {
		tag = "其它附件"
	}
	if !fleetAttachmentTagAllowed(tag) {
		c.Error(errors.NewBadRequestError("附件标签不合法"))
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.Error(errors.NewBadRequestError("请以 multipart/form-data 上传附件"))
		return
	}
	files := form.File["files"]
	if len(files) == 0 {
		files = form.File["file"]
	}
	if len(files) == 0 {
		c.Error(errors.NewBadRequestError("请选择要上传的附件"))
		return
	}

	saved := make([]FleetRecordAttachment, 0, len(files))
	for _, fh := range files {
		if fh.Size <= 0 {
			continue
		}
		if fh.Size > fleetAttachmentMaxBytes {
			c.Error(errors.NewBadRequestError("单个附件不能超过 20MB"))
			return
		}
		f, oerr := fh.Open()
		if oerr != nil {
			c.Error(errors.NewInternalServerError("读取附件失败"))
			return
		}
		content, rerr := io.ReadAll(io.LimitReader(f, fleetAttachmentMaxBytes+1))
		_ = f.Close()
		if rerr != nil {
			c.Error(errors.NewInternalServerError("读取附件失败"))
			return
		}
		if len(content) == 0 {
			continue
		}
		mime := fleetAttachmentMimeOf(fh.Header.Get("Content-Type"), fh.Filename)
		if mime == "" {
			c.Error(errors.NewBadRequestError("仅支持 PDF / JPG / PNG / WEBP 附件"))
			return
		}
		row := FleetRecordAttachment{
			ID:        uuid.NewString(),
			TenantID:  tenantID,
			RecordID:  recordID,
			FileName:  strings.TrimSpace(fh.Filename),
			MimeType:  mime,
			FileSize:  int64(len(content)),
			Tag:       tag,
			Content:   content,
			CreatedAt: time.Now(),
		}
		if err := h.db.WithContext(ctx).Create(&row).Error; err != nil {
			logger.Errorf(ctx, "save fleet attachment failed: %v", err)
			c.Error(errors.NewInternalServerError("保存附件失败"))
			return
		}
		// 返回体剔除 BYTEA（Content 已是 json:"-"，这里再置空省内存）
		row.Content = nil
		saved = append(saved, row)
	}
	if len(saved) == 0 {
		c.Error(errors.NewBadRequestError("没有可保存的附件"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": saved, "count": len(saved)}})
}

// ViewFleetAttachment GET /fleet/attachments/view?id=xxx
//
// 无后缀预览路由：路径里不出现 .png/.pdf，边缘 Nginx 静态正则无从劫持。
func (h *FleetHandler) ViewFleetAttachment(c *gin.Context) {
	ctx := c.Request.Context()
	uid, _ := fleetTenantID(c)
	tenantID := int64(uid)
	id := strings.TrimSpace(c.Query("id"))
	if id == "" {
		c.Error(errors.NewBadRequestError("附件编号不能为空"))
		return
	}
	var row FleetRecordAttachment
	if err := h.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).First(&row).Error; err != nil {
		c.Error(errors.NewNotFoundError("附件不存在"))
		return
	}
	if len(row.Content) == 0 {
		c.Error(errors.NewNotFoundError("附件内容为空"))
		return
	}
	ct := row.MimeType
	if ct == "" {
		ct = "application/octet-stream"
	}
	// 内联展示（预览），文件名只作为下载时的建议名
	c.Header("Content-Type", ct)
	c.Header("Content-Disposition", "inline")
	c.Header("Cache-Control", "private, max-age=600")
	c.Data(http.StatusOK, ct, row.Content)
}

// DeleteFleetAttachment DELETE /fleet/attachments/:attID
func (h *FleetHandler) DeleteFleetAttachment(c *gin.Context) {
	ctx := c.Request.Context()
	uid, _ := fleetTenantID(c)
	tenantID := int64(uid)
	id := strings.TrimSpace(c.Param("attID"))
	if id == "" {
		c.Error(errors.NewBadRequestError("附件编号不能为空"))
		return
	}
	res := h.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Delete(&FleetRecordAttachment{})
	if res.Error != nil {
		logger.Errorf(ctx, "delete fleet attachment failed: %v", res.Error)
		c.Error(errors.NewInternalServerError("删除附件失败"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"deleted": res.RowsAffected}})
}

// CountFleetRecordAttachments 批量统计附件数（列表「附件」列用）。
//
// 单独一个函数而不是在列表 SQL 里 LEFT JOIN：列表按需分页 20 行，
// 一次 GROUP BY 比 20 次子查询快得多，也不会污染 ListFleetRecords 的既有查询。
func (h *FleetHandler) CountFleetRecordAttachments(ctx context.Context, tenantID int64, recordIDs []string) map[string]int {
	out := map[string]int{}
	if len(recordIDs) == 0 {
		return out
	}
	type row struct {
		RecordID string
		N        int
	}
	// 表还没建时直接返回空映射：列表页首次进入不该因为「附件功能未初始化」而 500
	if !h.db.WithContext(ctx).Migrator().HasTable(&FleetRecordAttachment{}) {
		return out
	}
	var rows []row
	if err := h.db.WithContext(ctx).Model(&FleetRecordAttachment{}).
		Select("record_id, count(*) as n").
		Where("tenant_id = ? AND record_id IN ?", tenantID, recordIDs).
		Group("record_id").Scan(&rows).Error; err != nil {
		return out
	}
	for _, r := range rows {
		out[r.RecordID] = r.N
	}
	return out
}

func fleetAttachmentTagAllowed(tag string) bool {
	for _, t := range fleetAttachmentTags {
		if t == tag {
			return true
		}
	}
	return false
}
