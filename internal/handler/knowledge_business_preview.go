package handler

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	filesvc "github.com/Tencent/WeKnora/internal/application/service/file"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// localBaseDirEnv mirrors the LOCAL_STORAGE_BASE_DIR fallback used by the file
// service factory so handlers can resolve tenant local storage the same way.
func localBaseDirEnv() string {
	dir := strings.TrimSpace(os.Getenv("LOCAL_STORAGE_BASE_DIR"))
	if dir == "" {
		return "/data/files"
	}
	return dir
}

// GetKnowledgePreviewURL returns a browser-loadable URL for the source file of
// the given knowledge entry:
//   - COS-backed files get a COS presigned URL (24h) so the browser streams
//     directly from COS (HTTP Range-capable, no VPS bandwidth).
//   - local-backed files get a short-lived /r/ access-grant URL served by the
//     app proxy with Range support (falls back to the same mechanism whenever
//     the resolved driver cannot produce a usable URL).
func (h *BusinessExtractHandler) GetKnowledgePreviewURL(c *gin.Context) {
	ctx := c.Request.Context()
	knowledgeID := c.Param("knowledgeId")
	if strings.TrimSpace(knowledgeID) == "" {
		knowledgeID = c.Param("id")
	}
	if strings.TrimSpace(knowledgeID) == "" {
		c.Error(errors.NewBadRequestError("knowledge id cannot be empty"))
		return
	}
	_, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, knowledgeID, types.OrgRoleViewer)
	if err != nil {
		c.Error(err)
		return
	}
	knowledge, err := h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil || knowledge == nil {
		logger.Error(ctx, "GetKnowledgePreviewURL: knowledge not found", err)
		c.Error(errors.NewNotFoundError("knowledge not found"))
		return
	}
	handle, ok := types.ParseResourcePath(knowledge.FilePath)
	if !ok {
		logger.Errorf(ctx, "GetKnowledgePreviewURL: knowledge %s file_path is not a resource reference", knowledgeID)
		c.Error(errors.NewNotFoundError("source file not found"))
		return
	}
	resourceRef := types.ResourceScheme + handle

	var res types.StoredResource
	if err := h.db.Where("handle = ?", handle).First(&res).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("source file not found"))
		} else {
			logger.Error(ctx, "GetKnowledgePreviewURL: query resource failed", err)
			c.Error(errors.NewInternalServerError("failed to resolve source file"))
		}
		return
	}
	if res.State != types.ResourceStateActive {
		c.Error(errors.NewNotFoundError("source file is not available"))
		return
	}

	url, kind, err := h.resolvePreviewURL(ctx, &res, resourceRef)
	if err != nil {
		logger.Error(ctx, "GetKnowledgePreviewURL: resolve preview URL failed", err)
		c.Error(errors.NewInternalServerError("failed to generate preview URL"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"url":       url,
			"kind":      kind,
			"filename":  res.OriginalName,
			"file_name": knowledge.FileName,
			"file_type": knowledge.FileType,
			"size":      res.Size,
		},
	})
}

// resolvePreviewURL builds the best direct-loadable URL for a stored resource:
//   - local-backed files prefer the driver's own URL (signed /r/ proxy), falling
//     back to a fresh /r/ access-grant token when the driver cannot produce one
//     (e.g. APP_EXTERNAL_URL unset).
//   - COS-backed files always go through the /r/ app proxy: Tencent COS buckets
//     created after 2024-01-01 force Content-Disposition: attachment on their
//     default domain (including presigned URLs), which makes Chrome abort
//     in-iframe PDF previews. The app proxy re-streams from COS with headers we
//     control (inline + Accept-Ranges).
func (h *BusinessExtractHandler) resolvePreviewURL(ctx context.Context, res *types.StoredResource, resourceRef string) (string, string, error) {
	ownerTenant := &types.Tenant{ID: res.TenantID}
	svc, provider, err := h.storageResolver.ResolveFileService(ctx, ownerTenant, res.StorageBackendID, res.Provider, localBaseDirEnv())
	if err == nil && provider != "cos" {
		var url string
		url, err = svc.GetFileURL(ctx, resourceRef)
		// Only accept truly loadable URLs (presigned https / /r/ relative); local
		// backends return physical storage:// paths which the browser cannot load.
		if err == nil && url != "" && !strings.HasPrefix(url, "local://") && !strings.HasPrefix(url, "storage://") &&
			(strings.HasPrefix(url, "http") || strings.HasPrefix(url, "/")) {
			return url, provider, nil
		}
	}
	// Fallback: short-lived /r/ access-grant URL (app proxy, Range-capable,
	// inline response headers). Used for COS files and driver failures alike.
	token, grantErr := h.resourceCatalog.CreateAccessGrant(ctx, resourceRef, 2*time.Hour)
	if grantErr != nil {
		if err != nil {
			return "", "", fmt.Errorf("GetFileURL: %w", err)
		}
		return "", "", grantErr
	}
	return "/r/" + token, "proxy", nil
}

// MigrateLocalToCOS migrates all active local-backed resources of the calling
// tenant to the System COS backend and rewrites their resource records, then
// removes the local disk copies. Pass dry_run=true to only count (default).
// The System LOCAL backend record itself is left intact (reads of other rows
// still resolve through it), and the operation is idempotent: rows already on
// COS are skipped because the query only selects provider='local'.
func (h *BusinessExtractHandler) MigrateLocalToCOS(c *gin.Context) {
	ctx := c.Request.Context()
	tenant, ok := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
	if !ok || tenant == nil || tenant.ID == 0 {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	dryRun := strings.EqualFold(c.DefaultQuery("dry_run", "true"), "true")

	var cosBackend types.StorageBackend
	if err := h.db.Where("tenant_id = ? AND source = ? AND provider = ? AND status = ?",
		tenant.ID, types.StorageBackendSourceEnv, "cos", types.StorageBackendStatusActive).
		Order("created_at ASC").First(&cosBackend).Error; err != nil {
		logger.Error(ctx, "MigrateLocalToCOS: no System COS backend", err)
		c.Error(errors.NewInternalServerError("no active System COS backend configured"))
		return
	}
	if strings.TrimSpace(cosBackend.Config.BucketName) == "" || strings.TrimSpace(cosBackend.Config.Region) == "" {
		c.Error(errors.NewInternalServerError("System COS backend is missing bucket/region config"))
		return
	}

	var localBackend types.StorageBackend
	if err := h.db.Where("tenant_id = ? AND source = ? AND provider = ? AND status = ?",
		tenant.ID, types.StorageBackendSourceEnv, "local", types.StorageBackendStatusActive).
		Order("created_at ASC").First(&localBackend).Error; err != nil {
		logger.Error(ctx, "MigrateLocalToCOS: no System LOCAL backend", err)
		c.Error(errors.NewInternalServerError("no active System LOCAL backend configured"))
		return
	}

	var resources []types.StoredResource
	if err := h.db.Where("tenant_id = ? AND state = ? AND provider = ?",
		tenant.ID, types.ResourceStateActive, "local").Find(&resources).Error; err != nil {
		logger.Error(ctx, "MigrateLocalToCOS: list local resources failed", err)
		c.Error(errors.NewInternalServerError("failed to list local resources"))
		return
	}

	localSvc, _, err := h.storageResolver.ResolveFileService(ctx, tenant, localBackend.ID, "local", localBaseDirEnv())
	if err != nil {
		logger.Error(ctx, "MigrateLocalToCOS: resolve local file service failed", err)
		c.Error(errors.NewInternalServerError("failed to resolve local storage"))
		return
	}

	pathPrefix := strings.Trim(strings.TrimSpace(cosBackend.Config.PathPrefix), "/")
	migrated, failed := 0, 0
	failures := make([]gin.H, 0)
	for _, res := range resources {
		_, providerPath, scoped := types.ParseStorageBackendPath(res.PhysicalPath)
		if !scoped {
			providerPath = res.PhysicalPath
		}
		rel := strings.TrimPrefix(providerPath, "local://")
		if rel == "" || rel == providerPath {
			logger.Warnf(ctx, "MigrateLocalToCOS: skip resource %s (unexpected physical_path %q)", res.ID, res.PhysicalPath)
			continue
		}
		objectKey := rel
		if pathPrefix != "" {
			objectKey = pathPrefix + "/" + rel
		}

		if dryRun {
			continue
		}
		reader, err := localSvc.GetFile(ctx, res.PhysicalPath)
		if err != nil {
			logger.Errorf(ctx, "MigrateLocalToCOS: read local file failed resource=%s", res.ID)
			failed++
			failures = append(failures, gin.H{"id": res.ID, "handle": res.Handle, "error": "read local file: " + err.Error()})
			continue
		}
		uploadErr := filesvc.PutCOSObject(ctx, cosBackend.Config.BucketName, cosBackend.Config.Region,
			cosBackend.Config.AccessKeyID, cosBackend.Config.SecretAccessKey, objectKey, reader)
		_ = reader.Close()
		if uploadErr != nil {
			logger.Errorf(ctx, "MigrateLocalToCOS: upload to COS failed resource=%s", res.ID)
			failed++
			failures = append(failures, gin.H{"id": res.ID, "handle": res.Handle, "error": uploadErr.Error()})
			continue
		}
		newPath := types.BuildStorageBackendPath(cosBackend.ID,
			fmt.Sprintf("cos://%s/%s/%s", cosBackend.Config.BucketName, cosBackend.Config.Region, objectKey))
		if err := h.db.Model(&types.StoredResource{}).Where("id = ?", res.ID).Updates(map[string]any{
			"storage_backend_id": cosBackend.ID,
			"provider":           "cos",
			"physical_path":      newPath,
		}).Error; err != nil {
			logger.Errorf(ctx, "MigrateLocalToCOS: update resource failed id=%s", res.ID)
			failed++
			failures = append(failures, gin.H{"id": res.ID, "handle": res.Handle, "error": err.Error()})
			continue
		}
		if err := localSvc.DeleteFile(ctx, res.PhysicalPath); err != nil {
			logger.Warnf(ctx, "MigrateLocalToCOS: local cleanup failed id=%s err=%v", res.ID, err)
		}
		migrated++
	}

	result := gin.H{
		"dry_run":  dryRun,
		"total":    len(resources),
		"migrated": migrated,
		"failed":   failed,
		"failures": failures,
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
