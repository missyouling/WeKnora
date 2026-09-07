package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	goerrors "errors"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/tracing/langfuse"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/Tencent/WeKnora/internal/utils"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
)

// KnowledgeHandler processes HTTP requests related to knowledge resources
type KnowledgeHandler struct {
	cfg               *config.Config
	kgService         interfaces.KnowledgeService
	kbService         interfaces.KnowledgeBaseService
	kbShareService    interfaces.KBShareService
	agentShareService interfaces.AgentShareService
	asynqClient       interfaces.TaskEnqueuer
	spanRepo          repository.KnowledgeSpanRepository
	chunkService      interfaces.ChunkService
	modelService      interfaces.ModelService
}

// NewKnowledgeHandler creates a new knowledge handler instance
func NewKnowledgeHandler(
	cfg *config.Config,
	kgService interfaces.KnowledgeService,
	kbService interfaces.KnowledgeBaseService,
	kbShareService interfaces.KBShareService,
	agentShareService interfaces.AgentShareService,
	asynqClient interfaces.TaskEnqueuer,
	spanRepo repository.KnowledgeSpanRepository,
	chunkService interfaces.ChunkService,
	modelService interfaces.ModelService,
) *KnowledgeHandler {
	return &KnowledgeHandler{
		cfg:               cfg,
		kgService:         kgService,
		kbService:         kbService,
		kbShareService:    kbShareService,
		agentShareService: agentShareService,
		asynqClient:       asynqClient,
		spanRepo:          spanRepo,
		chunkService:      chunkService,
		modelService:      modelService,
	}
}

// requireKBOwnershipOrAdmin enforces the same "KB creator OR Admin+" matrix
// used by OwnedKBOrAdmin for routes whose KB id comes from the request body.
func (h *KnowledgeHandler) requireKBOwnershipOrAdmin(c *gin.Context, kbID string) error {
	creator, err := resolveKBCreatorByKBID(c, h.kbService, kbID)
	evalErr := middleware.EvaluateOwnershipOrRole(
		c.Request.Context(),
		h.cfg,
		types.TenantRoleAdmin,
		creator,
		err,
	)
	if evalErr == nil {
		return nil
	}
	if goerrors.Is(evalErr, middleware.ErrResourceNotFound) {
		return errors.NewNotFoundError("knowledge base not found")
	}
	if goerrors.Is(evalErr, middleware.ErrOwnershipForbidden) {
		return errors.NewForbiddenError("No permission to operate on this knowledge base")
	}
	logger.ErrorWithFields(c.Request.Context(), evalErr, map[string]interface{}{
		"kb_id": secutils.SanitizeForLog(kbID),
	})
	return errors.NewInternalServerError("cannot verify knowledge base ownership")
}

// validateKnowledgeBaseAccess validates access permissions to a knowledge base
// using the ":id" URL path parameter. It delegates to validateKnowledgeBaseAccessWithKBID.
func (h *KnowledgeHandler) validateKnowledgeBaseAccess(c *gin.Context) (*types.KnowledgeBase, string, uint64, types.OrgMemberRole, error) {
	kbID := secutils.SanitizeForLog(c.Param("id"))
	return h.validateKnowledgeBaseAccessWithKBID(c, kbID)
}

// validateKnowledgeBaseAccessWithKBID validates access to the given knowledge base ID (e.g. from query or body).
// Enforces per-API-key KB scope before tenant/share/agent resolution.
// Returns the knowledge base, kbID, effective tenant ID, permission, and error.
func (h *KnowledgeHandler) validateKnowledgeBaseAccessWithKBID(c *gin.Context, kbID string) (*types.KnowledgeBase, string, uint64, types.OrgMemberRole, error) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		logger.Error(ctx, "Failed to get tenant ID")
		return nil, "", 0, "", errors.NewUnauthorizedError("Unauthorized")
	}
	userID, userExists := c.Get(types.UserIDContextKey.String())
	callerTenantRole := types.TenantRoleFromContext(ctx)
	kbID = secutils.SanitizeForLog(kbID)
	if kbID == "" {
		return nil, "", 0, "", errors.NewBadRequestError("Knowledge base ID cannot be empty")
	}
	if err := requireTenantAPIKeyKnowledgeBase(ctx, kbID); err != nil {
		return nil, kbID, 0, "", err
	}
	kb, err := h.kbService.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		// Same not-found-vs-real-error split as knowledgebase.go's
		// validateAndGetKnowledgeBase: ErrKnowledgeBaseNotFound is the
		// expected outcome for a probed/stale kb id and must surface as
		// 404, not the generic 500 the original code produced.
		if goerrors.Is(err, repository.ErrKnowledgeBaseNotFound) {
			return nil, kbID, 0, "", errors.NewNotFoundError("knowledge base not found")
		}
		logger.ErrorWithFields(ctx, err, nil)
		return nil, kbID, 0, "", errors.NewInternalServerError(err.Error())
	}
	if kb.TenantID == tenantID {
		return kb, kbID, tenantID, types.OrgRoleAdmin, nil
	}
	if h.kbShareService != nil {
		permission, isShared, permErr := h.kbShareService.CheckTenantKBPermission(ctx, kbID, tenantID, callerTenantRole)
		if permErr == nil && isShared {
			sourceTenantID, srcErr := h.kbShareService.GetKBSourceTenant(ctx, kbID)
			if srcErr == nil {
				logger.Infof(ctx, "Tenant %d accessing shared KB %s with permission %s, source tenant: %d",
					tenantID, kbID, permission, sourceTenantID)
				return kb, kbID, sourceTenantID, permission, nil
			}
		}
	}
	if h.agentShareService != nil {
		can, err := h.agentShareService.TenantCanAccessKBViaSomeSharedAgent(ctx, tenantID, callerTenantRole, kb)
		if err == nil && can {
			logger.Infof(ctx, "Tenant %d accessing KB %s via some shared agent", tenantID, kbID)
			return kb, kbID, kb.TenantID, types.OrgRoleViewer, nil
		}
	}
	_ = userID
	_ = userExists
	logger.Warnf(ctx, "Permission denied to access KB %s, tenant ID: %d, KB tenant: %d", kbID, tenantID, kb.TenantID)
	return nil, kbID, 0, "", errors.NewForbiddenError("Permission denied to access this knowledge base")
}

// resolveKnowledgeAndValidateKBAccess resolves knowledge by ID and validates KB access (owner or shared with required permission).
// Returns the knowledge, context with effectiveTenantID set for downstream service calls, and error.
func (h *KnowledgeHandler) resolveKnowledgeAndValidateKBAccess(c *gin.Context, knowledgeID string, requiredPermission types.OrgMemberRole) (*types.Knowledge, context.Context, error) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		return nil, ctx, errors.NewUnauthorizedError("Unauthorized")
	}
	userID, userExists := c.Get(types.UserIDContextKey.String())
	callerTenantRole := types.TenantRoleFromContext(ctx)

	knowledge, err := h.kgService.GetKnowledgeByIDOnly(ctx, knowledgeID)
	if err != nil {
		return nil, ctx, errors.NewNotFoundError("Knowledge not found")
	}
	if err := requireTenantAPIKeyKnowledgeBase(ctx, knowledge.KnowledgeBaseID); err != nil {
		return nil, ctx, err
	}

	// Owner: knowledge belongs to caller's tenant
	if knowledge.TenantID == tenantID {
		return knowledge, context.WithValue(ctx, types.TenantIDContextKey, tenantID), nil
	}

	// Shared KB: check organization permission
	if h.kbShareService != nil {
		permission, isShared, permErr := h.kbShareService.CheckTenantKBPermission(ctx, knowledge.KnowledgeBaseID, tenantID, callerTenantRole)
		if permErr == nil && isShared && permission.HasPermission(requiredPermission) {
			effectiveTenantID := knowledge.TenantID
			return knowledge, context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID), nil
		}
	}
	// Shared agent: request passes agent_id, or user has any shared agent that can access this KB
	if h.agentShareService != nil && requiredPermission == types.OrgRoleViewer {
		agentID := c.Query("agent_id")
		if agentID != "" {
			sourceTenantID, parseErr := types.ParseAgentSourceTenantID(c.Query(types.AgentSourceTenantIDParam))
			if parseErr != nil {
				return nil, ctx, errors.NewBadRequestError(parseErr.Error())
			}
			agent, err := h.agentShareService.GetSharedAgentForTenant(ctx, tenantID, callerTenantRole, agentID, sourceTenantID)
			if err == nil && agent != nil {
				if knowledge.TenantID != agent.TenantID {
					return nil, ctx, errors.NewForbiddenError("Permission denied to access this knowledge")
				}
				mode := agent.Config.KBSelectionMode
				if mode == "none" {
					return nil, ctx, errors.NewForbiddenError("Permission denied to access this knowledge")
				}
				if mode == "all" {
					return knowledge, context.WithValue(ctx, types.TenantIDContextKey, knowledge.TenantID), nil
				}
				if mode == "selected" {
					for _, kbID := range agent.Config.KnowledgeBases {
						if kbID == knowledge.KnowledgeBaseID {
							return knowledge, context.WithValue(ctx, types.TenantIDContextKey, knowledge.TenantID), nil
						}
					}
					return nil, ctx, errors.NewForbiddenError("Permission denied to access this knowledge")
				}
			}
		} else {
			kbRef := &types.KnowledgeBase{ID: knowledge.KnowledgeBaseID, TenantID: knowledge.TenantID}
			can, err := h.agentShareService.TenantCanAccessKBViaSomeSharedAgent(ctx, tenantID, callerTenantRole, kbRef)
			if err == nil && can {
				return knowledge, context.WithValue(ctx, types.TenantIDContextKey, knowledge.TenantID), nil
			}
		}
	}
	_ = userID
	_ = userExists
	return nil, ctx, errors.NewForbiddenError("Permission denied to access this knowledge")
}

// handleDuplicateKnowledgeError handles cases where duplicate knowledge is detected
// Returns true if the error was a duplicate error and was handled, false otherwise
func (h *KnowledgeHandler) handleDuplicateKnowledgeError(c *gin.Context,
	err error, knowledge *types.Knowledge, duplicateType string,
) bool {
	if dupErr, ok := err.(*types.DuplicateKnowledgeError); ok {
		ctx := c.Request.Context()
		logger.Warnf(ctx, "Detected duplicate %s: %s", duplicateType, secutils.SanitizeForLog(dupErr.Error()))
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": dupErr.Error(),
			"data":    knowledge, // knowledge contains the existing document
			"code":    fmt.Sprintf("duplicate_%s", duplicateType),
		})
		return true
	}
	return false
}

// enqueueKnowledgeListDelete enqueues an async batch-delete task for the
// given knowledge IDs and returns the asynq task ID.
func (h *KnowledgeHandler) enqueueKnowledgeListDelete(
	ctx context.Context, tenantID uint64, ids []string,
) (string, error) {
	payload := types.KnowledgeListDeletePayload{
		TenantID:     tenantID,
		KnowledgeIDs: ids,
		Initiator:    types.TaskInitiatorFromContext(ctx),
	}
	langfuse.InjectTracing(ctx, &payload)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	task := asynq.NewTask(types.TypeKnowledgeListDelete, payloadBytes,
		asynq.Queue(types.QueueMaintenance), asynq.MaxRetry(3), asynq.Timeout(2*time.Hour))
	info, err := h.asynqClient.Enqueue(task)
	if err != nil {
		return "", fmt.Errorf("enqueue task: %w", err)
	}
	return info.ID, nil
}

// enqueueKnowledgeListReparse enqueues an async batch-reparse task for the
// given knowledge IDs and returns the asynq task ID.
func (h *KnowledgeHandler) enqueueKnowledgeListReparse(
	ctx context.Context, tenantID uint64, ids []string, processConfig *types.KnowledgeProcessOverrides,
) (string, error) {
	payload := types.KnowledgeListReparsePayload{
		TenantID:      tenantID,
		KnowledgeIDs:  ids,
		ProcessConfig: processConfig,
		Initiator:     types.TaskInitiatorFromContext(ctx),
	}
	langfuse.InjectTracing(ctx, &payload)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	task := asynq.NewTask(types.TypeKnowledgeListReparse, payloadBytes,
		asynq.Queue(types.QueueMaintenance), asynq.MaxRetry(3), asynq.Timeout(time.Hour))
	info, err := h.asynqClient.Enqueue(task)
	if err != nil {
		return "", fmt.Errorf("enqueue task: %w", err)
	}
	return info.ID, nil
}

// CreateKnowledgeFromFile godoc
// @Summary      从文件创建知识
// @Description  上传文件并创建知识条目
// @Tags         知识管理
// @Accept       multipart/form-data
// @Produce      json
// @Param        id                path      string  true   "知识库ID"
// @Param        file              formData  file    true   "上传的文件"
// @Param        fileName          formData  string  false  "自定义文件名"
// @Param        metadata          formData  string  false  "元数据JSON"
// @Param        enable_multimodel formData  bool    false  "启用多模态处理"
// @Param        tag_ids       formData  string  false  "分类ID列表，逗号分隔"
// @Param        process_config    formData  string  false  "处理配置JSON（KnowledgeProcessOverrides）"
// @Success      200               {object}  map[string]interface{}  "创建的知识"
// @Failure      400               {object}  errors.AppError         "请求参数错误"
// @Failure      409               {object}  map[string]interface{}  "文件重复"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/file [post]
func (h *KnowledgeHandler) CreateKnowledgeFromFile(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start creating knowledge from file")

	// Validate access to the knowledge base (only owner or admin/editor can create)
	_, kbID, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccess(c)
	if err != nil {
		c.Error(err)
		return
	}
	ctx = context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	// Check write permission
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to create knowledge"))
		return
	}

	// Validate file size — read MAX_FILE_SIZE_MB env (50MB default).
	// Deliberately not a runtime system_setting; see filesize.go for the
	// rationale (nginx / docreader / browser bundle all cache this at
	// container startup, so a UI knob would silently mismatch).
	maxSizeMB := utils.GetMaxFileSizeMB()
	maxSize := maxSizeMB * 1024 * 1024
	// Capped before the multipart parse, not after: FormFile buffers the whole
	// body first, so the size check below only ever sees an upload we already
	// accepted. nginx location /api/ still enforces MAX_FILE_SIZE; this is the
	// same cap for requests that reach the app without that proxy.
	limitUploadBody(c, maxSize)

	// Get the uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		if isRequestBodyTooLarge(err) {
			logger.Error(ctx, "File size too large")
			c.Error(errors.NewBadRequestError(fmt.Sprintf("文件大小不能超过%dMB", maxSizeMB)))
			return
		}
		logger.Error(ctx, "File upload failed", err)
		c.Error(errors.NewBadRequestError("File upload failed").WithDetails(err.Error()))
		return
	}
	if file.Size > maxSize {
		logger.Error(ctx, "File size too large")
		c.Error(errors.NewBadRequestError(fmt.Sprintf("文件大小不能超过%dMB", maxSizeMB)))
		return
	}

	// Get custom filename if provided (for folder uploads with path)
	customFileName := c.PostForm("fileName")
	customFileName = secutils.SanitizeForLog(customFileName)
	displayFileName := file.Filename
	displayFileName = secutils.SanitizeForLog(displayFileName)
	if customFileName != "" {
		displayFileName = customFileName
		logger.Infof(ctx, "Using custom filename: %s (original: %s)", customFileName, displayFileName)
	}

	logger.Infof(ctx, "File upload successful, filename: %s, size: %.2f KB", displayFileName, float64(file.Size)/1024)
	logger.Infof(ctx, "Creating knowledge, knowledge base ID: %s, filename: %s", kbID, displayFileName)

	// Parse metadata if provided
	var metadata map[string]string
	metadataStr := c.PostForm("metadata")
	if metadataStr != "" {
		if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
			logger.Error(ctx, "Failed to parse metadata", err)
			c.Error(errors.NewBadRequestError("Invalid metadata format").WithDetails(err.Error()))
			return
		}
		logger.Infof(ctx, "Received file metadata: %s", secutils.SanitizeForLog(fmt.Sprintf("%v", metadata)))
	}

	enableMultimodelForm := c.PostForm("enable_multimodel")
	var enableMultimodel *bool
	if enableMultimodelForm != "" {
		parseBool, err := strconv.ParseBool(enableMultimodelForm)
		if err != nil {
			logger.Error(ctx, "Failed to parse enable_multimodel", err)
			c.Error(errors.NewBadRequestError("Invalid enable_multimodel format").WithDetails(err.Error()))
			return
		}
		enableMultimodel = &parseBool
	}

	var processOverrides *types.KnowledgeProcessOverrides
	if raw := c.PostForm("process_config"); raw != "" {
		processOverrides = &types.KnowledgeProcessOverrides{}
		if err := json.Unmarshal([]byte(raw), processOverrides); err != nil {
			logger.Error(ctx, "Failed to parse process_config", err)
			c.Error(errors.NewBadRequestError("Invalid process_config format").WithDetails(err.Error()))
			return
		}
	}
	if enableMultimodel != nil && (processOverrides == nil || processOverrides.EnableMultimodel == nil) {
		if processOverrides == nil {
			processOverrides = &types.KnowledgeProcessOverrides{EnableMultimodel: enableMultimodel}
		} else {
			processOverrides.EnableMultimodel = enableMultimodel
		}
	}

	// 获取分类ID列表（如果提供），逗号分隔，用于知识多标签分类管理
	tagIDs := parseCommaSeparatedTagIDs(c.PostForm("tag_ids"))

	channel := c.PostForm("channel")

	// Create knowledge entry from the file
	knowledge, err := h.kgService.CreateKnowledgeFromFile(ctx, kbID, file, metadata, enableMultimodel, customFileName, tagIDs, channel, processOverrides)
	// Check for duplicate knowledge error
	if err != nil {
		if h.handleDuplicateKnowledgeError(c, err, knowledge, "file") {
			return
		}
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(
		ctx,
		"Knowledge created successfully, ID: %s, title: %s",
		secutils.SanitizeForLog(knowledge.ID),
		secutils.SanitizeForLog(knowledge.Title),
	)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    knowledge,
	})
}

// CreateKnowledgeFromURL godoc
// @Summary      从URL创建知识
// @Description  从指定URL抓取内容并创建知识条目。当提供 file_name/file_type 或 URL 路径含已知文件扩展名时，自动切换为文件下载模式
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id       path      string  true  "知识库ID"
// @Param        request  body      object{url=string,file_name=string,file_type=string,enable_multimodel=bool,title=string,tag_ids=[]string}  true  "URL请求"
// @Success      201      {object}  map[string]interface{}  "创建的知识"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Failure      409      {object}  map[string]interface{}  "URL重复"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/url [post]
func (h *KnowledgeHandler) CreateKnowledgeFromURL(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start creating knowledge from URL")

	// Validate access to the knowledge base (only owner or admin/editor can create)
	_, kbID, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccess(c)
	if err != nil {
		c.Error(err)
		return
	}
	ctx = context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	// Check write permission
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to create knowledge"))
		return
	}

	// Parse URL from request body
	var req struct {
		URL              string                           `json:"url" binding:"required"`
		FileName         string                           `json:"file_name"`
		FileType         string                           `json:"file_type"`
		EnableMultimodel *bool                            `json:"enable_multimodel"`
		Title            string                           `json:"title"`
		TagIDs           []string                         `json:"tag_ids"`
		Channel          string                           `json:"channel"`
		ProcessConfig    *types.KnowledgeProcessOverrides `json:"process_config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse URL request", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	logger.Infof(ctx, "Received URL request: %s, file_name: %s, file_type: %s",
		secutils.SanitizeForLog(req.URL),
		secutils.SanitizeForLog(req.FileName),
		secutils.SanitizeForLog(req.FileType),
	)

	// SSRF validation for user-supplied URL
	if err := secutils.ValidateURLForSSRF(req.URL); err != nil {
		logger.Warnf(ctx, "SSRF validation failed for knowledge URL: %v", err)
		c.Error(errors.NewBadRequestError(secutils.FormatSSRFError("URL", req.URL, err)))
		return
	}

	logger.Infof(ctx,
		"Creating knowledge from URL, knowledge base ID: %s, URL: %s",
		secutils.SanitizeForLog(kbID),
		secutils.SanitizeForLog(req.URL),
	)

	// Create knowledge entry from the URL
	knowledge, err := h.kgService.CreateKnowledgeFromURL(
		ctx, kbID, req.URL, req.FileName, req.FileType, req.EnableMultimodel, req.Title, req.TagIDs, req.Channel, req.ProcessConfig,
	)
	// Check for duplicate knowledge error
	if err != nil {
		if h.handleDuplicateKnowledgeError(c, err, knowledge, "url") {
			return
		}
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(
		ctx,
		"Knowledge created successfully from URL, ID: %s, title: %s",
		secutils.SanitizeForLog(knowledge.ID),
		secutils.SanitizeForLog(knowledge.Title),
	)
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    knowledge,
	})
}

// CreateManualKnowledge godoc
// @Summary      手工创建知识
// @Description  手工录入Markdown格式的知识内容
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id       path      string                       true  "知识库ID"
// @Param        request  body      types.ManualKnowledgePayload true  "手工知识内容"
// @Success      200      {object}  map[string]interface{}       "创建的知识"
// @Failure      400      {object}  errors.AppError              "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/manual [post]
func (h *KnowledgeHandler) CreateManualKnowledge(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start creating manual knowledge")

	// Validate access to the knowledge base (only owner or admin/editor can create)
	_, kbID, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccess(c)
	if err != nil {
		c.Error(err)
		return
	}
	ctx = context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	// Check write permission
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to create knowledge"))
		return
	}

	var req types.ManualKnowledgePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse manual knowledge request", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	knowledge, err := h.kgService.CreateKnowledgeFromManual(ctx, kbID, &req, req.Channel)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"kb_id": kbID,
		})
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Manual knowledge created successfully, knowledge ID: %s",
		secutils.SanitizeForLog(knowledge.ID))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    knowledge,
	})
}

// GetKnowledge godoc
// @Summary      获取知识详情
// @Description  根据ID获取知识条目详情
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "知识ID"
// @Success      200  {object}  map[string]interface{}  "知识详情"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      404  {object}  errors.AppError         "知识不存在"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge/{id} [get]
func (h *KnowledgeHandler) GetKnowledge(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start retrieving knowledge")

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Knowledge ID is empty")
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	// Resolve knowledge and validate KB access (at least viewer)
	knowledge, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.OrgRoleViewer)
	if err != nil {
		c.Error(err)
		return
	}

	// Re-fetch with tenant-scoped service so tags and other joined fields are populated.
	if knowledge, err = h.kgService.GetKnowledgeByID(effCtx, id); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewNotFoundError("Knowledge not found"))
		return
	}

	logger.Infof(ctx, "Knowledge retrieved successfully, ID: %s, title: %s",
		secutils.SanitizeForLog(knowledge.ID), secutils.SanitizeForLog(knowledge.Title))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    knowledge,
	})
}

// GetKnowledgeSpans godoc
// @Summary      获取知识文档解析的 Span 树（含历史尝试）
// @Description  返回该知识在解析流水线的 trace tree（root → stage → subspan）：每段状态、耗时、input/output、错误码、langfuse_trace_id。支持 ?attempt=N 查看历史尝试；不传则返回最新尝试。前端用于渲染时间线 + 多模态/embedding 子节点 + 一键跳转 Langfuse。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id        path   string  true   "知识ID"
// @Param        attempt   query  int     false  "指定尝试号；省略=最新"
// @Success      200       {object}  map[string]interface{}
// @Router       /api/v1/knowledge/{id}/spans [get]
//
// Always returns the canonical 5-stage timeline; missing stage rows are
// synthesized as "pending" so the frontend timeline always renders five
// segments. Subspans (multimodal.image[i], generation.*) ride along under
// each stage as children when present.
func (h *KnowledgeHandler) GetKnowledgeSpans(c *gin.Context) {
	ctx := c.Request.Context()

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	knowledge, _, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.OrgRoleViewer)
	if err != nil {
		c.Error(err)
		return
	}

	// Pick attempt: explicit ?attempt=N wins; otherwise pull the
	// latest attempt from the spans table. Lite-mode / fresh installs
	// with zero rows fall through to attempt=0, in which case we
	// return a placeholder tree (5 pending stages, no root, no
	// children) so the UI still renders.
	requestedAttempt := 0
	if v := strings.TrimSpace(c.Query("attempt")); v != "" {
		if n, perr := strconv.Atoi(v); perr == nil && n > 0 {
			requestedAttempt = n
		}
	}

	rows := []types.KnowledgeProcessingSpan{}
	currentAttempt := 0
	latestAttempt := 0
	if h.spanRepo != nil {
		latest, lerr := h.spanRepo.LatestAttempt(ctx, knowledge.ID)
		if lerr != nil {
			logger.Warnf(ctx, "spans LatestAttempt failed for %s: %v", knowledge.ID, lerr)
		} else {
			latestAttempt = latest
		}
		if requestedAttempt > 0 {
			currentAttempt = requestedAttempt
		} else {
			currentAttempt = latestAttempt
		}
		if currentAttempt > 0 {
			rows, err = h.spanRepo.ListByAttempt(ctx, knowledge.ID, currentAttempt)
			if err != nil {
				logger.Warnf(ctx, "spans ListByAttempt failed kid=%s attempt=%d: %v",
					knowledge.ID, currentAttempt, err)
				rows = nil
			}
		}
	}

	// Build tree: index by SpanID, then attach to parents. Stages
	// missing from the DB are synthesized as "pending" placeholders
	// under a synthetic (or real, if present) root so the timeline
	// always renders five segments. parse_status threads through so
	// pre-tracker historical knowledge (no rows but parse_status is
	// already terminal) renders as done/failed instead of pending —
	// otherwise legacy completed documents would forever look like
	// they're still waiting in the queue.
	tree, currentStageName, lastErr := buildSpanTree(knowledge.ID, currentAttempt, rows, knowledge.ParseStatus)

	resp := gin.H{
		"knowledge_id":    knowledge.ID,
		"attempt":         currentAttempt,
		"latest_attempt":  latestAttempt,
		"parse_status":    knowledge.ParseStatus,
		"current_attempt": currentAttempt,
		"current_stage":   currentStageName,
		"trace":           tree,
	}
	if lastError := knowledgeSpansLastError(
		currentAttempt,
		latestAttempt,
		knowledge.ParseStatus,
		knowledge.ErrorMessage,
		knowledge.UpdatedAt,
		lastErr,
	); lastError != nil {
		resp["last_error"] = lastError
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}

// knowledgeSpansLastError builds the last_error payload for GetKnowledgeSpans.
// Span failures win when present; otherwise a failed knowledge row without a
// matching span error (recovery / dead-letter paths) surfaces ErrorMessage.
func knowledgeSpansLastError(
	currentAttempt, latestAttempt int,
	parseStatus, knowledgeErrorMessage string,
	knowledgeUpdatedAt time.Time,
	spanFailure *types.KnowledgeProcessingSpan,
) gin.H {
	if spanFailure != nil {
		return gin.H{
			"stage":         spanFailure.Name,
			"code":          spanFailure.ErrorCode,
			"message":       spanFailure.ErrorMessage,
			"name":          spanFailure.Name,
			"error_code":    spanFailure.ErrorCode,
			"error_message": spanFailure.ErrorMessage,
			"finished_at":   spanFailure.FinishedAt,
		}
	}
	if currentAttempt != latestAttempt || parseStatus != types.ParseStatusFailed || knowledgeErrorMessage == "" {
		return nil
	}
	errorCode := "UNKNOWN"
	if strings.EqualFold(strings.TrimSpace(knowledgeErrorMessage),
		"Task interrupted due to application restart") {
		errorCode = "SERVER_RESTART"
	}
	return gin.H{
		"stage":         "knowledge_processing",
		"code":          errorCode,
		"message":       knowledgeErrorMessage,
		"name":          "knowledge_processing",
		"error_code":    errorCode,
		"error_message": knowledgeErrorMessage,
		"finished_at":   knowledgeUpdatedAt,
	}
}

// buildSpanTree assembles a flat list of span rows into a parent-child
// tree rooted at the (knowledge, attempt)'s root span. Missing canonical
// stages are filled in with pending placeholders so the UI always renders
// the five timeline segments. Returns the root, the current_stage name
// (the running stage if any), and the most recent failed span if one
// exists.
//
// parseStatus is the knowledge.parse_status string. When the spans table
// has zero rows for this attempt (legacy data parsed before tracking, or
// a fresh knowledge before the pipeline starts), the placeholder status
// is inferred from parseStatus: completed → done, failed → failed,
// otherwise pending. Without this, every historical knowledge would
// render as "all 5 stages pending" forever despite having actually
// completed parsing.
func buildSpanTree(knowledgeID string, attempt int, rows []types.KnowledgeProcessingSpan, parseStatus string) (
	root *types.SpanTreeNode, currentStage string, lastFailure *types.KnowledgeProcessingSpan,
) {
	now := time.Now()
	// Build node lookup, identify root.
	nodes := make(map[string]*types.SpanTreeNode, len(rows))
	var rootRow *types.KnowledgeProcessingSpan
	stageRowByName := map[string]*types.KnowledgeProcessingSpan{}
	for i := range rows {
		r := rows[i]
		nodes[r.SpanID] = &types.SpanTreeNode{KnowledgeProcessingSpan: r}
		if r.Kind == types.SpanKindRoot && rootRow == nil {
			cp := r
			rootRow = &cp
		}
		if r.Kind == types.SpanKindStage {
			cp := r
			stageRowByName[r.Name] = &cp
		}
		if r.Status == types.SpanStatusRunning && r.Kind == types.SpanKindStage && currentStage == "" {
			currentStage = r.Name
		}
		if r.Status == types.SpanStatusFailed {
			cp := r
			lastFailure = &cp
		}
	}

	// Pick the synthesized stage status from parse_status. Without this,
	// historical knowledge that completed before span tracking was wired
	// would render as "5 pending stages" forever — the rows simply
	// weren't recorded, but parse_status correctly reads "completed".
	// The synthesized stages don't carry duration/timing data; they
	// just communicate the inferred terminal state.
	syntheticStatus := types.SpanStatusPending
	switch parseStatus {
	case types.ParseStatusCompleted:
		syntheticStatus = types.SpanStatusDone
	case types.ParseStatusFailed:
		syntheticStatus = types.SpanStatusFailed
	}

	// Synthesize root if no rows came back so the API contract stays
	// stable (frontend always expects a `trace` object).
	if rootRow == nil {
		root = &types.SpanTreeNode{KnowledgeProcessingSpan: types.KnowledgeProcessingSpan{
			KnowledgeID: knowledgeID,
			Attempt:     attempt,
			SpanID:      "",
			Name:        "knowledge_processing",
			Kind:        types.SpanKindRoot,
			Status:      syntheticStatus,
			CreatedAt:   now,
			UpdatedAt:   now,
		}}
	} else {
		root = nodes[rootRow.SpanID]
	}

	// Link real children to their parents. Walk `rows` (not the
	// `nodes` map!) so the append order matches the repo's stable
	// `ORDER BY id ASC`. Iterating the map directly would give
	// callers a different child ordering on every request — Go map
	// iteration is intentionally randomised — and the UI would
	// flicker subspans into a different order on each refresh.
	for i := range rows {
		r := &rows[i]
		n := nodes[r.SpanID]
		if n == nil || n == root {
			continue
		}
		if r.ParentSpanID == "" {
			// Real top-level row with no parent and not the root
			// itself — attach to root so it doesn't dangle.
			root.Children = append(root.Children, n)
			continue
		}
		parent, ok := nodes[r.ParentSpanID]
		if !ok {
			// Unknown parent (orphan); attach to root.
			root.Children = append(root.Children, n)
			continue
		}
		parent.Children = append(parent.Children, n)
	}

	// Synthesize missing stage rows as children of root so the timeline
	// always shows 5 segments. Status mirrors the synthesized root —
	// pending while the pipeline is still running, done/failed for
	// historical knowledge whose terminal state we know but whose
	// per-stage timing was never recorded. Appended in AllStages order
	// so the canonical stage layout is deterministic regardless of
	// which rows are missing.
	for _, name := range types.AllStages {
		if _, ok := stageRowByName[name]; ok {
			continue
		}
		placeholder := types.KnowledgeProcessingSpan{
			KnowledgeID: knowledgeID,
			Attempt:     attempt,
			Name:        name,
			Kind:        types.SpanKindStage,
			Status:      syntheticStatus,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		root.Children = append(root.Children, &types.SpanTreeNode{KnowledgeProcessingSpan: placeholder})
	}

	return root, currentStage, lastFailure
}

// ListKnowledge godoc
// @Summary      获取知识列表
// @Description  获取知识库下的知识列表，支持分页和筛选
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id         path      string  true   "知识库ID"
// @Param        page       query     int     false  "页码"
// @Param        page_size  query     int     false  "每页数量"
// @Param        tag_ids       query     string  false  "标签ID筛选，逗号分隔（OR语义）"
// @Param        keyword       query     string  false  "关键词搜索"
// @Param        file_type     query     string  false  "文件类型筛选"
// @Param        parse_status  query     string  false  "解析状态筛选 (pending/processing/completed/failed)"
// @Param        source        query     string  false  "来源/渠道筛选 (web/api/feishu/notion/yuque/wechat/...，或 manual/url 按 type 过滤)"
// @Param        start_time    query     string  false  "更新时间起点，RFC3339 格式"
// @Param        end_time      query     string  false  "更新时间终点，RFC3339 格式"
// @Param        folder_path      query     string  false  "文件夹路径筛选，空字符串表示知识库根目录；不传该参数则不按文件夹过滤"
// @Param        folder_recursive query     bool    false  "为 true 时同时返回子文件夹内的文档"
// @Success      200        {object}  map[string]interface{}  "知识列表"
// @Failure      400        {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge [get]
func (h *KnowledgeHandler) ListKnowledge(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start retrieving knowledge list")

	// Validate access to the knowledge base (read access - any permission level)
	_, kbID, effectiveTenantID, _, err := h.validateKnowledgeBaseAccess(c)
	if err != nil {
		c.Error(err)
		return
	}

	// Update context with effective tenant ID for shared KB access
	ctx = context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	// Parse pagination parameters from query string
	var pagination types.Pagination
	if err := c.ShouldBindQuery(&pagination); err != nil {
		logger.Error(ctx, "Failed to parse pagination parameters", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	filter := types.KnowledgeListFilter{
		TagIDs:      parseCommaSeparatedTagIDs(c.Query("tag_ids")),
		Keyword:     c.Query("keyword"),
		FileType:    c.Query("file_type"),
		ParseStatus: c.Query("parse_status"),
		Source:      c.Query("source"),
	}
	if raw := c.Query("start_time"); raw != "" {
		t, err := parseFilterTime(raw)
		if err != nil {
			c.Error(errors.NewBadRequestError("invalid start_time: " + err.Error()))
			return
		}
		filter.UpdatedFrom = t
	}
	if raw := c.Query("end_time"); raw != "" {
		t, err := parseFilterTime(raw)
		if err != nil {
			c.Error(errors.NewBadRequestError("invalid end_time: " + err.Error()))
			return
		}
		filter.UpdatedTo = t
	}
	// The folder dimension is opt-in by parameter *presence*: an empty
	// folder_path is meaningful (the knowledge base root), so it cannot be
	// distinguished from "no folder filter" by value alone.
	if raw, ok := c.GetQuery("folder_path"); ok {
		filter.FolderPath = types.NormalizeKnowledgeFolderPath(raw)
		filter.FolderScope = types.FolderScopeExact
		if recursive, err := strconv.ParseBool(c.DefaultQuery("folder_recursive", "false")); err == nil && recursive {
			filter.FolderScope = types.FolderScopeSubtree
		}
	}

	logger.Infof(
		ctx,
		"Retrieving knowledge list under knowledge base, kb_id=%s tag_ids=%s keyword=%s file_type=%s parse_status=%s source=%s start_time=%s end_time=%s folder_path=%s folder_scope=%s page=%d page_size=%d effectiveTenantID=%d",
		secutils.SanitizeForLog(kbID),
		secutils.SanitizeForLog(strings.Join(filter.TagIDs, ",")),
		secutils.SanitizeForLog(filter.Keyword),
		secutils.SanitizeForLog(filter.FileType),
		secutils.SanitizeForLog(filter.ParseStatus),
		secutils.SanitizeForLog(filter.Source),
		secutils.SanitizeForLog(c.Query("start_time")),
		secutils.SanitizeForLog(c.Query("end_time")),
		secutils.SanitizeForLog(filter.FolderPath),
		string(filter.FolderScope),
		pagination.Page,
		pagination.PageSize,
		effectiveTenantID,
	)

	// Retrieve paginated knowledge entries
	result, err := h.kgService.ListPagedKnowledgeByKnowledgeBaseID(ctx, kbID, &pagination, filter)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(
		ctx,
		"Knowledge list retrieved successfully, knowledge base ID: %s, total: %d",
		secutils.SanitizeForLog(kbID),
		result.Total,
	)
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"data":      result.Data,
		"total":     result.Total,
		"page":      result.Page,
		"page_size": result.PageSize,
	})
}

// ListKnowledgeFolders godoc
// @Summary      获取知识库文件夹目录树
// @Description  返回知识库内由文件夹上传形成的目录树，包含每个文件夹的直接文档数与含子目录的总数
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "知识库ID"
// @Success      200  {object}  map[string]interface{}  "目录树"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/folders [get]
func (h *KnowledgeHandler) ListKnowledgeFolders(c *gin.Context) {
	ctx := c.Request.Context()

	// Read access mirrors ListKnowledge so the sidebar tree is available to
	// every viewer of a shared knowledge base.
	_, kbID, effectiveTenantID, _, err := h.validateKnowledgeBaseAccess(c)
	if err != nil {
		c.Error(err)
		return
	}
	ctx = context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	tree, err := h.kgService.ListKnowledgeFolderTree(ctx, kbID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Knowledge folder tree retrieved, kb_id=%s folders=%d",
		secutils.SanitizeForLog(kbID), len(tree.Folders))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tree,
	})
}

// MoveKnowledgeToFolderRequest is the body schema for POST /knowledge/folder.
type MoveKnowledgeToFolderRequest struct {
	KBID string   `json:"kb_id" binding:"required"`
	IDs  []string `json:"knowledge_ids" binding:"required"`
	// FolderPath is the destination folder; the empty string is the knowledge
	// base top level. It is deliberately not `binding:"required"` so documents
	// can be moved back out of every folder.
	FolderPath string `json:"folder_path"`
}

// MoveKnowledgeToFolder godoc
// @Summary      移动知识到文件夹
// @Description  批量修改知识条目所属文件夹。文件夹由路径推导而来，因此目标路径不存在时会自动创建；空路径表示知识库顶层。仅调整归类，不会重新解析文档
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        request  body      MoveKnowledgeToFolderRequest  true  "移动请求"
// @Success      200      {object}  map[string]interface{}        "移动成功"
// @Failure      400      {object}  errors.AppError               "请求参数错误"
// @Failure      403      {object}  errors.AppError               "权限不足"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge/folder [post]
func (h *KnowledgeHandler) MoveKnowledgeToFolder(c *gin.Context) {
	ctx := c.Request.Context()

	var req MoveKnowledgeToFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("Invalid request parameters: " + err.Error()))
		return
	}

	ids := dedupeKnowledgeIDs(req.IDs)
	if len(ids) == 0 {
		c.Error(errors.NewBadRequestError("knowledge_ids cannot be empty"))
		return
	}
	const maxBatch = 200
	if len(ids) > maxBatch {
		c.Error(errors.NewBadRequestError(fmt.Sprintf("too many ids (max %d per batch)", maxBatch)))
		return
	}

	kbID, effectiveTenantID, err := h.requireKnowledgeWriteAccess(c, req.KBID)
	if err != nil {
		c.Error(err)
		return
	}
	ctx = context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	// Guard against cross-KB moves: the service layer scopes by tenant, so the
	// handler must confirm every entry belongs to the requested knowledge base.
	if err := h.requireKnowledgeInKB(ctx, effectiveTenantID, kbID, ids); err != nil {
		c.Error(err)
		return
	}

	affected, err := h.kgService.MoveKnowledgeToFolder(ctx, kbID, ids, req.FolderPath)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"moved_count": affected,
			"folder_path": types.NormalizeKnowledgeFolderPath(req.FolderPath),
		},
	})
}

// RenameKnowledgeFolderRequest is the body schema for
// PUT /knowledge-bases/:id/knowledge/folders.
type RenameKnowledgeFolderRequest struct {
	From string `json:"from" binding:"required"`
	To   string `json:"to"   binding:"required"`
}

// RenameKnowledgeFolder godoc
// @Summary      重命名或移动文件夹
// @Description  把一个文件夹及其所有子目录改到新路径。目标路径已存在时两个文件夹合并；不能移动到自身子目录下
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id       path      string                        true  "知识库ID"
// @Param        request  body      RenameKnowledgeFolderRequest  true  "重命名请求"
// @Success      200      {object}  map[string]interface{}        "重命名成功"
// @Failure      400      {object}  errors.AppError               "请求参数错误"
// @Failure      403      {object}  errors.AppError               "权限不足"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/folders [put]
func (h *KnowledgeHandler) RenameKnowledgeFolder(c *gin.Context) {
	ctx := c.Request.Context()

	var req RenameKnowledgeFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("Invalid request parameters: " + err.Error()))
		return
	}

	_, kbID, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccess(c)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to modify knowledge"))
		return
	}
	if err := h.requireKBOwnershipOrAdmin(c, kbID); err != nil {
		c.Error(err)
		return
	}
	ctx = context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	affected, err := h.kgService.RenameKnowledgeFolder(ctx, kbID, req.From, req.To)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"moved_count": affected,
			"folder_path": types.NormalizeKnowledgeFolderPath(req.To),
		},
	})
}

// dedupeKnowledgeIDs trims, drops empty and de-duplicates a batch of IDs while
// preserving the caller's order.
func dedupeKnowledgeIDs(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	ids := make([]string, 0, len(raw))
	for _, item := range raw {
		id := strings.TrimSpace(item)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

// requireKnowledgeWriteAccess resolves a knowledge base from a request body and
// enforces the editor-or-admin plus ownership gate shared by the batch routes.
func (h *KnowledgeHandler) requireKnowledgeWriteAccess(
	c *gin.Context,
	requestedKBID string,
) (string, uint64, error) {
	_, kbID, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, requestedKBID)
	if err != nil {
		return "", 0, err
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		return "", 0, errors.NewForbiddenError("No permission to modify knowledge")
	}
	if err := h.requireKBOwnershipOrAdmin(c, kbID); err != nil {
		return "", 0, err
	}
	return kbID, effectiveTenantID, nil
}

// requireKnowledgeInKB verifies every ID exists and belongs to the given
// knowledge base, so a batch operation cannot reach across knowledge bases.
func (h *KnowledgeHandler) requireKnowledgeInKB(
	ctx context.Context,
	tenantID uint64,
	kbID string,
	ids []string,
) error {
	knowledgeList, err := h.kgService.GetKnowledgeBatch(ctx, tenantID, ids)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		return errors.NewInternalServerError(err.Error())
	}
	if len(knowledgeList) != len(ids) {
		return errors.NewBadRequestError("One or more knowledge entries not found")
	}
	for _, k := range knowledgeList {
		if k.KnowledgeBaseID != kbID {
			return errors.NewBadRequestError(
				fmt.Sprintf("Knowledge %s does not belong to knowledge base %s",
					secutils.SanitizeForLog(k.ID), secutils.SanitizeForLog(kbID)))
		}
	}
	return nil
}

// DeleteKnowledge godoc
// @Summary      删除知识
// @Description  根据ID异步删除知识条目。请求会被入队到与批量删除相同的异步管道（asynq）；
// @Description  接口返回 200 仅表示任务已提交（响应 data.task_id 为任务 ID），实际删除由后台 worker 完成。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "知识ID"
// @Success      200  {object}  map[string]interface{}  "任务已提交，返回 task_id"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge/{id} [delete]
func (h *KnowledgeHandler) DeleteKnowledge(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start deleting knowledge")

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Knowledge ID is empty")
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	_, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.OrgRoleEditor)
	if err != nil {
		c.Error(err)
		return
	}

	// Reuse the batch async pipeline so single-item delete shares the same
	// hardening (asynq retries, business-aware queue routing, marking-as-deleting
	// inside the worker) as BatchDeleteKnowledge / ClearKnowledgeBaseContents.
	effectiveTenantID, _ := effCtx.Value(types.TenantIDContextKey).(uint64)
	if effectiveTenantID == 0 {
		logger.Error(ctx, "Effective tenant ID missing after access validation")
		c.Error(errors.NewInternalServerError("workspace context unavailable"))
		return
	}

	logger.Infof(ctx, "Enqueuing knowledge delete, ID: %s", secutils.SanitizeForLog(id))
	taskID, err := h.enqueueKnowledgeListDelete(effCtx, effectiveTenantID, []string{id})
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue knowledge delete task: %v", err)
		c.Error(errors.NewInternalServerError("Failed to enqueue delete task"))
		return
	}

	logger.Infof(ctx, "Knowledge delete task enqueued: %s, knowledge_id: %s", taskID, secutils.SanitizeForLog(id))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Delete task submitted",
		"data": gin.H{
			"task_id": taskID,
		},
	})
}

// BatchDeleteKnowledgeRequest is the body schema for POST /knowledge/batch-delete.
type BatchDeleteKnowledgeRequest struct {
	KBID string   `json:"kb_id" binding:"required"`
	IDs  []string `json:"ids"  binding:"required"`
}

// BatchDeleteKnowledge godoc
// @Summary      批量删除知识
// @Description  按 ID 列表批量删除单个知识库下的多个知识条目
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        request  body      BatchDeleteKnowledgeRequest  true  "批量删除请求"
// @Success      200      {object}  map[string]interface{}       "删除成功"
// @Failure      400      {object}  errors.AppError              "请求参数错误"
// @Failure      403      {object}  errors.AppError              "权限不足"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge/batch-delete [post]
func (h *KnowledgeHandler) BatchDeleteKnowledge(c *gin.Context) {
	ctx := c.Request.Context()

	var req BatchDeleteKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("Invalid request parameters: " + err.Error()))
		return
	}

	ids := dedupeKnowledgeIDs(req.IDs)
	if len(ids) == 0 {
		c.Error(errors.NewBadRequestError("ids cannot be empty"))
		return
	}
	const maxBatch = 200
	if len(ids) > maxBatch {
		c.Error(errors.NewBadRequestError(fmt.Sprintf("too many ids (max %d per batch)", maxBatch)))
		return
	}

	// Validate KB access (editor or admin) using the kb_id from body.
	_, kbID, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, req.KBID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to delete knowledge"))
		return
	}
	if err := h.requireKBOwnershipOrAdmin(c, kbID); err != nil {
		c.Error(err)
		return
	}
	ctx = context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	// Single batch fetch to validate that every id exists and belongs to the
	// requested KB. The service-layer DeleteKnowledgeList only enforces tenant
	// scope, not KB scope, so the handler must guard against cross-KB deletion.
	knowledgeList, err := h.kgService.GetKnowledgeBatch(ctx, effectiveTenantID, ids)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	if len(knowledgeList) != len(ids) {
		c.Error(errors.NewBadRequestError("One or more knowledge entries not found"))
		return
	}
	for _, k := range knowledgeList {
		if k.KnowledgeBaseID != kbID {
			c.Error(errors.NewBadRequestError(
				fmt.Sprintf("Knowledge %s does not belong to knowledge base %s",
					secutils.SanitizeForLog(k.ID), secutils.SanitizeForLog(kbID))))
			return
		}
	}

	taskID, err := h.enqueueKnowledgeListDelete(ctx, effectiveTenantID, ids)
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue batch knowledge delete task: %v", err)
		c.Error(errors.NewInternalServerError("Failed to enqueue batch delete task"))
		return
	}

	logger.Infof(ctx, "Batch knowledge delete task enqueued: %s, kb_id: %s, count: %d",
		taskID, secutils.SanitizeForLog(kbID), len(ids))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Batch delete task submitted",
		"data": gin.H{
			"task_id":       taskID,
			"deleted_count": len(ids),
		},
	})
}

// ClearKnowledgeBaseContents godoc
// @Summary      清空知识库内容
// @Description  删除知识库下的所有知识条目（异步任务）。知识库本身保留，仅清空其中的内容
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "知识库ID"
// @Success      200  {object}  map[string]interface{}  "清空任务已提交"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      403  {object}  errors.AppError         "权限不足"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge [delete]
func (h *KnowledgeHandler) ClearKnowledgeBaseContents(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start clearing knowledge base contents")

	kb, kbID, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccess(c)
	if err != nil {
		c.Error(err)
		return
	}

	// Only owner (admin with matching tenant) can clear knowledge base contents
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if kb.TenantID != tenantID || permission != types.OrgRoleAdmin {
		c.Error(errors.NewForbiddenError("Only knowledge base owner can clear contents"))
		return
	}

	ctx = context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	knowledgeList, err := h.kgService.ListKnowledgeByKnowledgeBaseID(ctx, kbID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to list knowledge entries").WithDetails(err.Error()))
		return
	}

	if len(knowledgeList) == 0 {
		logger.Infof(ctx, "Knowledge base %s is already empty", secutils.SanitizeForLog(kbID))
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Knowledge base is already empty",
			"data":    gin.H{"deleted_count": 0},
		})
		return
	}

	knowledgeIDs := make([]string, 0, len(knowledgeList))
	for _, knowledge := range knowledgeList {
		knowledgeIDs = append(knowledgeIDs, knowledge.ID)
	}

	taskID, err := h.enqueueKnowledgeListDelete(ctx, effectiveTenantID, knowledgeIDs)
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue knowledge list delete task: %v", err)
		c.Error(errors.NewInternalServerError("Failed to enqueue cleanup task"))
		return
	}

	logger.Infof(ctx, "Knowledge base contents clear task enqueued: %s, kb_id: %s, count: %d",
		taskID, secutils.SanitizeForLog(kbID), len(knowledgeIDs))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Knowledge base contents clear task submitted",
		"data":    gin.H{"deleted_count": len(knowledgeIDs)},
	})
}

// DownloadKnowledgeFile godoc
// @Summary      下载知识文件
// @Description  下载知识条目关联的原始文件
// @Tags         知识管理
// @Accept       json
// @Produce      application/octet-stream
// @Param        id   path      string  true  "知识ID"
// @Success      200  {file}    file    "文件内容"
// @Failure      400  {object}  errors.AppError  "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge/{id}/download [get]
func (h *KnowledgeHandler) DownloadKnowledgeFile(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start downloading knowledge file")

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Knowledge ID is empty")
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	// Keep a handler-level Editor check in addition to the route guard. The
	// original file is more sensitive than parsed-content reads and must not
	// be downloadable through a read-only organization share.
	_, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.OrgRoleEditor)
	if err != nil {
		c.Error(err)
		return
	}
	logger.Infof(ctx, "Retrieving knowledge file, ID: %s", secutils.SanitizeForLog(id))

	file, filename, err := h.kgService.GetKnowledgeFile(effCtx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to retrieve file").WithDetails(err.Error()))
		return
	}
	defer file.Close()

	logger.Infof(
		ctx,
		"Knowledge file retrieved successfully, ID: %s, filename: %s",
		secutils.SanitizeForLog(id),
		secutils.SanitizeForLog(filename),
	)

	// Set response headers for file download
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	cd := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
	c.Header("Content-Disposition", cd)
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Expires", "0")
	c.Header("Cache-Control", "must-revalidate")
	c.Header("Pragma", "public")

	// Stream file content to response
	c.Stream(func(w io.Writer) bool {
		if _, err := io.Copy(w, file); err != nil {
			logger.Errorf(ctx, "Failed to send file: %v", err)
			return false
		}
		logger.Debug(ctx, "File sending completed")
		return false
	})
}

// mimeTypeByExt returns the MIME type for a given file extension.
func mimeTypeByExt(filename string) string {
	ct, _ := secutils.SafeContentTypeByFilename(filename)
	return ct
}

// PreviewKnowledgeFile godoc
// @Summary      预览知识文件
// @Description  返回知识条目关联的原始文件，Content-Type 根据文件类型设置，用于浏览器内嵌预览
// @Tags         知识管理
// @Accept       json
// @Produce      application/pdf,image/jpeg,image/png,text/plain
// @Param        id   path      string  true  "知识ID"
// @Success      200  {file}    file    "文件内容"
// @Failure      400  {object}  errors.AppError  "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge/{id}/preview [get]
func (h *KnowledgeHandler) PreviewKnowledgeFile(c *gin.Context) {
	ctx := c.Request.Context()

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	// 删除历史抽屉预览：?include_deleted=1 允许预览被系统自动删除（软删）但仍保留
	// 物理源文件的记录。仅限同租户 owner 校验（共享 KB 的历史预览暂不支持）。
	if c.Query("include_deleted") == "1" {
		tenantID := c.GetUint64(types.TenantIDContextKey.String())
		if tenantID == 0 {
			c.Error(errors.NewUnauthorizedError("Unauthorized"))
			return
		}
		effCtx := context.WithValue(ctx, types.TenantIDContextKey, tenantID)
		knowledge, err := h.kgService.GetDeletedKnowledgeByID(effCtx, id)
		if err != nil {
			c.Error(errors.NewNotFoundError("Knowledge not found"))
			return
		}
		if knowledge.TenantID != tenantID {
			c.Error(errors.NewForbiddenError("Forbidden"))
			return
		}
		file, filename, ferr := h.kgService.GetDeletedKnowledgeFile(effCtx, id)
		if ferr != nil {
			logger.ErrorWithFields(ctx, ferr, nil)
			c.Error(errors.NewInternalServerError("Failed to retrieve file").WithDetails(ferr.Error()))
			return
		}
		defer file.Close()
		contentType, inline := secutils.SafeContentTypeByFilename(filename)
		c.Header("Content-Type", contentType)
		c.Header("X-Content-Type-Options", "nosniff")
		disposition := "inline"
		if !inline {
			disposition = "attachment"
		}
		c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filename}))
		c.Header("Cache-Control", "private, max-age=3600")
		c.Stream(func(w io.Writer) bool {
			if _, err := io.Copy(w, file); err != nil {
				logger.Errorf(ctx, "Failed to stream preview: %v", err)
				return false
			}
			return false
		})
		return
	}

	_, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.OrgRoleViewer)
	if err != nil {
		c.Error(err)
		return
	}

	file, filename, err := h.kgService.GetKnowledgeFile(effCtx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to retrieve file").WithDetails(err.Error()))
		return
	}
	defer file.Close()

	contentType, inline := secutils.SafeContentTypeByFilename(filename)
	c.Header("Content-Type", contentType)
	c.Header("X-Content-Type-Options", "nosniff")
	disposition := "inline"
	if !inline {
		disposition = "attachment"
	}
	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filename}))
	c.Header("Cache-Control", "private, max-age=3600")

	c.Stream(func(w io.Writer) bool {
		if _, err := io.Copy(w, file); err != nil {
			logger.Errorf(ctx, "Failed to stream preview: %v", err)
			return false
		}
		return false
	})
}

// PreviewDeletedKnowledgeFile godoc
// @Summary      预览已删除（删除历史）知识文件的源文件
// @Description  返回被系统自动删除（软删、保留物理源文件）的知识条目关联的原始文件，供删除历史抽屉查看。
//               挂载在 KB 作用域路由下（URL 携带 KB id），避免走 /knowledge/:id 路由的
//               deleted_at IS NULL 知识解析中间件（软删记录无法通过该守卫）。
// @Tags         知识管理
// @Accept       json
// @Produce      application/pdf,image/jpeg,image/png,text/plain
// @Param        id             path  string  true  "知识库ID"
// @Param        knowledgeId    path  string  true  "知识ID"
// @Success      200  {file}    file    "文件内容"
// @Failure      400  {object}  errors.AppError  "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/preview-deleted [get]
func (h *KnowledgeHandler) PreviewDeletedKnowledgeFile(c *gin.Context) {
	ctx := c.Request.Context()

	id := secutils.SanitizeForLog(c.Param("knowledgeId"))
	if id == "" {
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, tenantID)
	knowledge, err := h.kgService.GetDeletedKnowledgeByID(effCtx, id)
	if err != nil {
		c.Error(errors.NewNotFoundError("Knowledge not found"))
		return
	}
	if knowledge.TenantID != tenantID {
		c.Error(errors.NewForbiddenError("Forbidden"))
		return
	}
	file, filename, ferr := h.kgService.GetDeletedKnowledgeFile(effCtx, id)
	if ferr != nil {
		logger.ErrorWithFields(ctx, ferr, nil)
		c.Error(errors.NewInternalServerError("Failed to retrieve file").WithDetails(ferr.Error()))
		return
	}
	defer file.Close()
	contentType, inline := secutils.SafeContentTypeByFilename(filename)
	c.Header("Content-Type", contentType)
	c.Header("X-Content-Type-Options", "nosniff")
	disposition := "inline"
	if !inline {
		disposition = "attachment"
	}
	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filename}))
	c.Header("Cache-Control", "private, max-age=3600")
	c.Stream(func(w io.Writer) bool {
		if _, err := io.Copy(w, file); err != nil {
			logger.Errorf(ctx, "Failed to stream preview: %v", err)
			return false
		}
		return false
	})
}

// GetKnowledgeBatchRequest defines parameters for batch knowledge retrieval
type GetKnowledgeBatchRequest struct {
	IDs                 []string `form:"ids" binding:"required"` // List of knowledge IDs
	KBID                string   `form:"kb_id"`                  // Optional: scope to this KB (validates access and uses effective tenant for shared KB)
	AgentID             string   `form:"agent_id"`               // Optional: when using a shared agent, use agent's tenant for retrieval (validates shared agent access)
	AgentSourceTenantID uint64   `form:"agent_source_tenant_id"` // Optional source selector, verified against the share relation
}

// GetKnowledgeBatch godoc
// @Summary      批量获取知识
// @Description  根据ID列表批量获取知识条目。可选 kb_id：指定时按该知识库校验权限并用于共享知识库的空间解析；可选 agent_id：使用共享智能体时传此参数，后端按智能体所属空间查询（用于刷新后恢复共享知识库下的文件）
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        ids       query     []string  true   "知识ID列表"
// @Param        kb_id     query     string   false  "可选，知识库ID（用于共享知识库时指定范围）"
// @Param        agent_id  query     string   false  "可选，共享智能体ID（用于按智能体空间批量拉取文件详情）"
// @Success      200       {object}  map[string]interface{}  "知识列表"
// @Failure      400       {object}  errors.AppError        "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge/batch [get]
func (h *KnowledgeHandler) GetKnowledgeBatch(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID, ok := c.Get(types.TenantIDContextKey.String())
	if !ok {
		logger.Error(ctx, "Failed to get tenant ID")
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	effectiveTenantID := tenantID.(uint64)

	var req GetKnowledgeBatchRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}
	if _, parseErr := types.ParseAgentSourceTenantID(c.Query(types.AgentSourceTenantIDParam)); parseErr != nil {
		c.Error(errors.NewBadRequestError(parseErr.Error()))
		return
	}

	// agentAllowedKBIDs restricts results to the agent's configured KB scope.
	// nil = no agent restriction; empty slice = agent has no KB access (none mode).
	var agentAllowedKBIDs []string

	// Optional agent_id: when using shared agent, resolve agent and use its tenant for batch retrieval (so shared KB files can be loaded after refresh)
	if agentID := secutils.SanitizeForLog(req.AgentID); agentID != "" && h.agentShareService != nil {
		userIDVal, ok := c.Get(types.UserIDContextKey.String())
		if !ok {
			c.Error(errors.NewUnauthorizedError("Unauthorized"))
			return
		}
		userID, _ := userIDVal.(string)
		currentTenantID := c.GetUint64(types.TenantIDContextKey.String())
		if currentTenantID == 0 {
			c.Error(errors.NewUnauthorizedError("Unauthorized"))
			return
		}
		callerTenantRole := types.TenantRoleFromContext(ctx)
		agent, err := h.agentShareService.GetSharedAgentForTenant(ctx, currentTenantID, callerTenantRole, agentID, req.AgentSourceTenantID)
		if err != nil || agent == nil {
			logger.Warnf(ctx, "GetKnowledgeBatch: invalid or inaccessible shared agent %s: %v", agentID, err)
			c.Error(errors.NewForbiddenError("Invalid or inaccessible shared agent").WithDetails(err.Error()))
			return
		}
		_ = userID
		effectiveTenantID = agent.TenantID
		agentAllowedKBIDs = resolveAgentAllowedKBIDs(agent)

		if agentAllowedKBIDs != nil && len(agentAllowedKBIDs) == 0 {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": []*types.Knowledge{}})
			return
		}
		logger.Infof(ctx, "Batch retrieving knowledge with agent_id, effective tenant ID: %d, IDs count: %d, allowed KBs: %v",
			effectiveTenantID, len(req.IDs), agentAllowedKBIDs)
	}

	var knowledges []*types.Knowledge
	var err error

	// scopeKBID tracks the single KB the results must belong to (set by explicit kb_id).
	var scopeKBID string

	// Optional kb_id: validate KB access and use effective tenant for shared KB
	if kbID := secutils.SanitizeForLog(req.KBID); kbID != "" {
		_, _, effID, _, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
		if err != nil {
			c.Error(err)
			return
		}
		if agentAllowedKBIDs != nil && !sliceContains(agentAllowedKBIDs, kbID) {
			c.Error(errors.NewForbiddenError("Knowledge base not accessible through this agent"))
			return
		}
		scopeKBID = kbID
		effectiveTenantID = effID
		ctx = context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

		logger.Infof(ctx, "Batch retrieving knowledge with kb_id, effective tenant ID: %d, IDs count: %d",
			effectiveTenantID, len(req.IDs))

		knowledges, err = h.kgService.GetKnowledgeBatch(ctx, effectiveTenantID, req.IDs)
	} else {
		// No kb_id: use GetKnowledgeBatchWithSharedAccess (or effectiveTenantID may already be set by agent_id for shared agent)
		logger.Infof(ctx, "Batch retrieving knowledge without kb_id, effective tenant ID: %d, IDs count: %d",
			effectiveTenantID, len(req.IDs))

		knowledges, err = h.kgService.GetKnowledgeBatchWithSharedAccess(ctx, effectiveTenantID, req.IDs)
	}

	// Build the effective allowed-KB set from explicit kb_id, shared agent
	// scope, and per-API-key KB restrictions.
	var allowedKBSet map[string]bool
	if scopeKBID != "" {
		allowedKBSet = map[string]bool{scopeKBID: true}
	} else if agentAllowedKBIDs != nil {
		allowedKBSet = make(map[string]bool, len(agentAllowedKBIDs))
		for _, id := range agentAllowedKBIDs {
			allowedKBSet[id] = true
		}
	}
	if apiKeySet := tenantAPIKeyAllowedKBSet(ctx); apiKeySet != nil {
		allowedKBSet = intersectKBAllowSet(allowedKBSet, apiKeySet)
	}
	if allowedKBSet != nil {
		knowledges = filterKnowledgesByKBAllowSet(knowledges, allowedKBSet)
	}

	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to retrieve knowledge list").WithDetails(err.Error()))
		return
	}

	logger.Infof(ctx, "Batch knowledge retrieval successful, requested count: %d, returned count: %d",
		len(req.IDs), len(knowledges))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    knowledges,
	})
}

// UpdateKnowledgeRequest defines the partial-update body for PUT /knowledge/:id.
// Omitted fields are left unchanged; an explicit empty description clears the summary.
type UpdateKnowledgeRequest struct {
	Title          *string         `json:"title"`
	Description    *string         `json:"description"`
	CustomMetadata json.RawMessage `json:"custom_metadata"`
}

// metadataHasNestedValues reports whether the raw custom_metadata JSON object
// contains any nested object or array values. Nested payloads (e.g. the
// invoice module's "invoices" array) must bypass the generic flat-scalar
// validation applied by UpdateKnowledge.
func metadataHasNestedValues(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var probe map[string]interface{}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	for _, value := range probe {
		switch value.(type) {
		case map[string]interface{}, []interface{}:
			return true
		}
	}
	return false
}

// UpdateKnowledge godoc
// @Summary      更新知识
// @Description  部分更新知识条目（标题/描述/自定义元数据）；未传字段保持不变，显式传空 description 可清空摘要
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id       path      string                   true  "知识ID"
// @Param        request  body      UpdateKnowledgeRequest   true  "更新字段（均可选）"
// @Success      200      {object}  map[string]interface{}  "更新成功"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge/{id} [put]
func (h *KnowledgeHandler) UpdateKnowledge(c *gin.Context) {
	ctx := c.Request.Context()

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Knowledge ID is empty")
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	_, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.OrgRoleEditor)
	if err != nil {
		c.Error(err)
		return
	}

	var req UpdateKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	// The generic UpdateKnowledge path applies a flat-scalar validation to
	// custom_metadata (strings/numbers/booleans only, <=20 keys, <=1000 chars).
	// The invoice module persists nested data (the "invoices" array), so when
	// the payload contains nested objects/arrays we route the metadata write
	// through SaveInvoiceCustomMetadata (which bypasses that check) and still
	// apply title/description updates through the regular path.
	if metadataHasNestedValues(req.CustomMetadata) {
		if err := h.kgService.SaveInvoiceCustomMetadata(effCtx, id, types.JSON(req.CustomMetadata)); err != nil {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError(err.Error()))
			return
		}
		if req.Title != nil || req.Description != nil {
			partial := types.Knowledge{ID: id}
			if req.Title != nil {
				partial.Title = *req.Title
			}
			if req.Description != nil {
				partial.Description = *req.Description
				partial.DescriptionSpecified = true
			}
			if err := h.kgService.UpdateKnowledge(effCtx, &partial); err != nil {
				logger.ErrorWithFields(ctx, err, nil)
				c.Error(errors.NewInternalServerError(err.Error()))
				return
			}
		}
	} else {
		knowledge := types.Knowledge{ID: id, CustomMetadata: types.JSON(req.CustomMetadata)}
		if req.Title != nil {
			knowledge.Title = *req.Title
		}
		if req.Description != nil {
			knowledge.Description = *req.Description
			knowledge.DescriptionSpecified = true
		}
		if err := h.kgService.UpdateKnowledge(effCtx, &knowledge); err != nil {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError(err.Error()))
			return
		}
	}
	updated, getErr := h.kgService.GetKnowledgeByID(effCtx, id)
	if getErr != nil {
		logger.Warnf(ctx, "Knowledge updated but failed to reload status for %s: %v", id, getErr)
	}

	logger.Infof(ctx, "Knowledge updated successfully, knowledge ID: %s", id)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Knowledge updated successfully",
		"data":    updated,
	})
}

// invoiceCustomMetadata is the persisted shape written into a knowledge
// entry's custom_metadata by ExtractInvoice. The frontend reads exactly
// these keys to render the invoice management list.
type invoiceCustomMetadata struct {
	Kind          string                      `json:"kind"`
	Invoices      []service.InvoiceExtractionItem `json:"invoices"`
	ExtractStatus string                      `json:"extract_status"`
	ExtractError  string                      `json:"extract_error"`
}

// contractCustomMetadata is the persisted shape written into a knowledge
// entry's custom_metadata by ExtractContract. The frontend reads exactly
// these keys to render the contract management list.
type contractCustomMetadata struct {
	Kind          string                          `json:"kind"`
	Contracts     []service.ContractExtractionItem `json:"contracts"`
	ExtractStatus string                          `json:"extract_status"`
	ExtractError  string                          `json:"extract_error"`
}

// contractBatchesHaveText reports whether any extraction batch carries usable
// text. A scanned document (no text layer) yields only empty batches; without
// this guard the extract endpoint would judge it not-a-contract and auto-delete
// a legitimate scanned contract.
func contractBatchesHaveText(batches []string) bool {
	for _, b := range batches {
		if strings.TrimSpace(b) != "" {
			return true
		}
	}
	return false
}

// invoiceBatchesHaveText is the invoice-side twin of contractBatchesHaveText.
func invoiceBatchesHaveText(batches []string) bool {
	for _, b := range batches {
		if strings.TrimSpace(b) != "" {
			return true
		}
	}
	return false
}

// autoDeleteCount reads the auto_deleted_count marker from a knowledge row's
// custom_metadata (0 when absent). It powers the restore→re-extract→still
// not-a-document anti-loop guard.
func (h *KnowledgeHandler) autoDeleteCount(ctx context.Context, knowledge *types.Knowledge) int {
	if knowledge == nil || len(knowledge.CustomMetadata) == 0 {
		return 0
	}
	var meta map[string]any
	if err := json.Unmarshal(knowledge.CustomMetadata, &meta); err != nil {
		return 0
	}
	if c, ok := meta["auto_deleted_count"].(float64); ok {
		return int(c)
	}
	return 0
}

// ExtractContract godoc
// @Summary      提取合同字段
// @Description  读取已解析文档文本，调用提取模型（复用知识库 summary_model_id）提取合同字段并写入 custom_metadata。幂等：对同一知识重复调用会覆盖写。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id           path  string  true  "知识库ID"
// @Param        knowledgeId  path  string  true  "知识ID"
// @Success      200  {object}  map[string]interface{}  "提取成功"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      403  {object}  errors.AppError         "无权限"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/extract-contract [post]
func (h *KnowledgeHandler) ExtractContract(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	knowledgeID := secutils.SanitizeForLog(c.Param("knowledgeId"))
	if kbID == "" || knowledgeID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id and knowledge id cannot be empty"))
		return
	}

	kb, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to extract contract fields"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	knowledge, err := h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for contract extraction", err)
		c.Error(errors.NewNotFoundError("Knowledge not found"))
		return
	}
	if knowledge.KnowledgeBaseID != kbID {
		c.Error(errors.NewBadRequestError("Knowledge does not belong to the given knowledge base"))
		return
	}
	if knowledge.ParseStatus != types.ParseStatusCompleted {
		c.Error(errors.NewBadRequestError("document has not been parsed yet, please wait for parsing to finish"))
		return
	}
	// 待补录豁免：manual 记录由用户人工维护，不自动重提取、不参与任何自动删除判定。
	var curContractMeta contractCustomMetadata
	_ = json.Unmarshal(knowledge.CustomMetadata, &curContractMeta)
	if curContractMeta.ExtractStatus == "manual" {
		logger.Infof(ctx, "Contract extraction skipped: knowledge %s is manual intake", knowledgeID)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "待补录记录，无需重复提取",
			"data":    map[string]interface{}{"kind": "contract", "extract_status": "manual", "removed": false},
		})
		return
	}

	modelID := strings.TrimSpace(kb.SummaryModelID)
	if modelID == "" {
		c.Error(errors.NewBadRequestError("no extraction model configured for the knowledge base"))
		return
	}
	chatModel, err := h.modelService.GetChatModel(effCtx, modelID)
	if err != nil {
		logger.Error(ctx, "Failed to load extraction model", err)
		c.Error(errors.NewInternalServerError("get extraction model failed: " + err.Error()))
		return
	}

	chunks, err := h.chunkService.ListChunksByKnowledgeID(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to list chunks for contract extraction", err)
		c.Error(errors.NewInternalServerError("list chunks failed: " + err.Error()))
		return
	}
	// 分批提取：超长多合同文档单次 LLM 调用不可靠，按 chunk 分批独立调用后合并，
	// Normalize 时按文件内合同编号去重。
	batches := service.BuildContractExtractionBatches(knowledge.FileName, knowledge.Description, chunks, 0)
	// 扫描件防御：无文本层（扫描件 PDF / 纯图片）时不要自动删除，保留文件并标记
	// failed 让用户人工处理（配合删除历史，避免"监测检测合同"这类扫描件被误删）。
	if !contractBatchesHaveText(batches) {
		noTextMeta, jerr := json.Marshal(contractCustomMetadata{
			Kind:          "contract",
			ExtractStatus: "failed",
			ExtractError:  "文档无可提取文本（疑似扫描件），已保留文件待人工处理",
		})
		if jerr == nil {
			if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(noTextMeta)); serr != nil {
				logger.Warnf(ctx, "Failed to persist contract no-text state: %v", serr)
			}
		}
		logger.Warnf(ctx, "Contract extraction skipped: no text layer for knowledge %s (scanned document), file kept", knowledgeID)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "document has no extractable text (possible scanned document), file kept",
			"data":    map[string]interface{}{"kind": "contract", "extract_status": "failed", "removed": false},
		})
		return
	}
	merged := &service.ContractExtractionResult{Kind: "contract"}
	var extractErr error
	sawContract := false
	for i, batch := range batches {
		if strings.TrimSpace(batch) == "" {
			continue
		}
		batchRes, berr := service.ExtractContractsFromContent(effCtx, chatModel, batch)
		if berr != nil {
			if i == 0 && len(merged.Contracts) == 0 {
				extractErr = berr
				break
			}
			logger.Warnf(ctx, "Contract extraction batch %d failed (non-fatal): %v", i+1, berr)
			continue
		}
		if batchRes == nil || batchRes.Kind == "not_contract" {
			continue
		}
		sawContract = true
		merged.Contracts = append(merged.Contracts, batchRes.Contracts...)
	}
	if !sawContract {
		merged.Kind = "not_contract"
	}
	if extractErr != nil {
		if failMeta, jerr := json.Marshal(contractCustomMetadata{
			Kind:          "contract",
			ExtractStatus: "failed",
			ExtractError:  "contract extraction failed: " + extractErr.Error(),
		}); jerr == nil {
			if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(failMeta)); serr != nil {
				logger.Warnf(ctx, "Failed to persist contract extraction failed-state: %v", serr)
			}
		}
		logger.Error(ctx, "Contract extraction model call failed", extractErr)
		c.Error(errors.NewInternalServerError("contract extraction failed: " + extractErr.Error()))
		return
	}
	extracted := merged
	// 自动编号去重：无合同编号的合同按当天已有最大序号 +1 顺序递增（跨文件唯一）
	autoSeq, seqErr := h.kgService.MaxAutoContractSeq(effCtx, kbID)
	if seqErr != nil {
		logger.Warnf(ctx, "Failed to compute auto contract seq for %s: %v", kbID, seqErr)
	}
	service.NormalizeContractExtractionResult(extracted, autoSeq)

	meta := contractCustomMetadata{
		Kind:          extracted.Kind,
		Contracts:     extracted.Contracts,
		ExtractStatus: "success",
		ExtractError:  extracted.ExtractError,
	}
	// 自定义识别规则：合同类型归类（用户可配置关键词/正则 → 合同类型），
	// 按模型提取出的合同类型字段匹配，命中则覆盖模型结果。
	if recCfg, rerr := h.kgService.GetRecognitionConfig(effCtx, kbID); rerr == nil {
		for i := range meta.Contracts {
			if t := service.ClassifyTypeFromRules(meta.Contracts[i].ContractType, recCfg); t != "" {
				meta.Contracts[i].ContractType = t
			}
		}
	}
	if extracted.Kind == "not_contract" {
		// 自定义识别规则捞回：模型判非但包含规则命中 → 认定为合同，置 manual 待补录，
		// 不自动删除（解决"该是合同却没入库"的误判）。
		if recCfg, rerr := h.kgService.GetRecognitionConfig(effCtx, kbID); rerr == nil {
			if service.MatchIncludeRules(strings.Join(batches, "\n"), recCfg) {
				meta.Kind = "contract"
				meta.Contracts = []service.ContractExtractionItem{}
				meta.ExtractStatus = "manual"
				meta.ExtractError = "识别规则命中，已认定为合同，请编辑补录字段"
				if raw, merr := json.Marshal(meta); merr == nil {
					if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(raw)); serr != nil {
						logger.Warnf(ctx, "Failed to persist rule-rescued contract meta: %v", serr)
					}
				}
				logger.Infof(ctx, "Contract rule-rescued by include rule, knowledge %s kept as manual", knowledgeID)
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"message": "识别规则命中，已认定为合同，请在列表中编辑补录字段",
					"data":    map[string]interface{}{"kind": "contract", "extract_status": "manual", "removed": false},
				})
				return
			}
		}
		meta.ExtractStatus = "not_contract"
		if meta.ExtractError == "" {
			meta.ExtractError = "document does not look like a contract"
		}
		// 防循环：auto_deleted_count >= 1 说明该文件曾被自动删除后由用户从删除历史
		// 恢复，再次判定非合同不再自动删除，改为 failed 保留（避免 删除→恢复→删除 死循环）。
		if h.autoDeleteCount(effCtx, knowledge) > 0 {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "系统判定非合同文件，但该文件已被人工恢复，已保留待人工处理"
			logger.Warnf(ctx, "Contract re-extraction judged non-contract but row was restored before; keeping file %s", knowledgeID)
		} else {
			// 非合同文件不能存在于合同知识库 → 提取判定后自动删除该文件（保留物理文件，
			// 写入删除历史，可在"删除历史"抽屉中查看/恢复/永久删除）
			if delErr := h.kgService.AutoDeleteKnowledge(effCtx, knowledgeID, "not_contract"); delErr != nil {
				logger.Warnf(ctx, "auto-delete non-contract knowledge failed: %v", delErr)
			} else {
				logger.Infof(ctx, "auto-deleted non-contract knowledge, ID: %s", knowledgeID)
				c.JSON(http.StatusOK, gin.H{
					"success":  true,
					"message":  "非合同文件已移至删除历史，可在删除历史中恢复",
					"data":     map[string]interface{}{"removed": true, "kind": "not_contract"},
				})
				return
			}
		}
	}
	// 空提取守卫：标记为合同但没有可用合同（空数组或全部空字段）→ failed 可重试
	if meta.Kind == "contract" {
		if len(meta.Contracts) == 0 {
			meta.ExtractStatus = "failed"
			if meta.ExtractError == "" {
				meta.ExtractError = "提取未返回任何合同，请重试"
			}
		} else if service.ContractsAllEmpty(meta.Contracts) {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "提取结果字段为空，请重试"
		}
	}

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		c.Error(errors.NewInternalServerError("failed to encode extraction result"))
		return
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(metaJSON)); err != nil {
		logger.Error(ctx, "Failed to persist contract extraction result", err)
		c.Error(errors.NewInternalServerError("failed to save extraction result: " + err.Error()))
		return
	}

	logger.Infof(ctx, "Contract extraction succeeded, knowledge ID: %s, kind: %s, contracts: %d",
		knowledgeID, meta.Kind, len(meta.Contracts))
	if meta.ExtractStatus == "failed" {
		logger.Warnf(ctx, "Contract extraction returned an unusable result, knowledge ID: %s, err: %s", knowledgeID, meta.ExtractError)
		c.Error(errors.NewInternalServerError(meta.ExtractError))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Contract extraction succeeded",
		"data": map[string]interface{}{
			"knowledge_id":    knowledgeID,
			"kind":            meta.Kind,
			"extract_status":  meta.ExtractStatus,
			"contracts":       meta.Contracts,
		},
	})
}

// regulationCustomMetadata is the persisted shape written into a knowledge
// entry's custom_metadata by ExtractRegulation. The frontend reads exactly
// these keys to render the regulation management list.
type regulationCustomMetadata struct {
	Kind          string                              `json:"kind"`
	Regulations   []types.RegulationExtractionItem    `json:"regulations"`
	ExtractStatus string                              `json:"extract_status"`
	ExtractError  string                              `json:"extract_error"`
}

// regulationBatchesHaveText reports whether any extraction batch carries usable
// text. A scanned document (no text layer) yields only empty batches; without
// this guard the extract endpoint would judge it not-a-regulation and
// auto-delete a legitimate scanned regulation.
func regulationBatchesHaveText(batches []string) bool {
	for _, b := range batches {
		if strings.TrimSpace(b) != "" {
			return true
		}
	}
	return false
}

// ExtractRegulation godoc
// @Summary      提取制度字段
// @Description  读取已解析文档文本，调用提取模型（复用知识库 summary_model_id）提取制度字段并写入 custom_metadata。幂等：对同一知识重复调用会覆盖写。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id           path  string  true  "知识库ID"
// @Param        knowledgeId  path  string  true  "知识ID"
// @Success      200  {object}  map[string]interface{}  "提取成功"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      403  {object}  errors.AppError         "无权限"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/extract-regulation [post]
func (h *KnowledgeHandler) ExtractRegulation(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	knowledgeID := secutils.SanitizeForLog(c.Param("knowledgeId"))
	if kbID == "" || knowledgeID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id and knowledge id cannot be empty"))
		return
	}

	kb, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to extract regulation fields"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	knowledge, err := h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for regulation extraction", err)
		c.Error(errors.NewNotFoundError("Knowledge not found"))
		return
	}
	if knowledge.KnowledgeBaseID != kbID {
		c.Error(errors.NewBadRequestError("Knowledge does not belong to the given knowledge base"))
		return
	}
	if knowledge.ParseStatus != types.ParseStatusCompleted {
		c.Error(errors.NewBadRequestError("document has not been parsed yet, please wait for parsing to finish"))
		return
	}
	// 待补录豁免：manual 记录由用户人工维护，不自动重提取、不参与任何自动删除判定。
	var curRegMeta regulationCustomMetadata
	_ = json.Unmarshal(knowledge.CustomMetadata, &curRegMeta)
	if curRegMeta.ExtractStatus == "manual" {
		logger.Infof(ctx, "Regulation extraction skipped: knowledge %s is manual intake", knowledgeID)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "待补录记录，无需重复提取",
			"data":    map[string]interface{}{"kind": "regulation", "extract_status": "manual", "removed": false},
		})
		return
	}

	modelID := strings.TrimSpace(kb.SummaryModelID)
	if modelID == "" {
		c.Error(errors.NewBadRequestError("no extraction model configured for the knowledge base"))
		return
	}
	chatModel, err := h.modelService.GetChatModel(effCtx, modelID)
	if err != nil {
		logger.Error(ctx, "Failed to load extraction model", err)
		c.Error(errors.NewInternalServerError("get extraction model failed: " + err.Error()))
		return
	}

	chunks, err := h.chunkService.ListChunksByKnowledgeID(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to list chunks for regulation extraction", err)
		c.Error(errors.NewInternalServerError("list chunks failed: " + err.Error()))
		return
	}
	batches := service.BuildRegulationExtractionBatches(knowledge.FileName, knowledge.Description, chunks, 0)
	// 扫描件防御：无文本层（扫描件 PDF / 纯图片）时不要自动删除，保留文件并标记
	// failed 让用户人工处理（配合删除历史）。
	if !regulationBatchesHaveText(batches) {
		noTextMeta, jerr := json.Marshal(regulationCustomMetadata{
			Kind:          "regulation",
			ExtractStatus: "failed",
			ExtractError:  "文档无可提取文本（疑似扫描件），已保留文件待人工处理",
		})
		if jerr == nil {
			if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(noTextMeta)); serr != nil {
				logger.Warnf(ctx, "Failed to persist regulation no-text state: %v", serr)
			}
		}
		logger.Warnf(ctx, "Regulation extraction skipped: no text layer for knowledge %s (scanned document), file kept", knowledgeID)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "document has no extractable text (possible scanned document), file kept",
			"data":    map[string]interface{}{"kind": "regulation", "extract_status": "failed", "removed": false},
		})
		return
	}
	merged := &service.RegulationExtractionResult{Kind: "regulation"}
	var extractErr error
	sawRegulation := false
	for i, batch := range batches {
		if strings.TrimSpace(batch) == "" {
			continue
		}
		batchRes, berr := service.ExtractRegulationsFromContent(effCtx, chatModel, batch)
		if berr != nil {
			if i == 0 && len(merged.Regulations) == 0 {
				extractErr = berr
				break
			}
			logger.Warnf(ctx, "Regulation extraction batch %d failed (non-fatal): %v", i+1, berr)
			continue
		}
		if batchRes == nil || batchRes.Kind == "not_regulation" {
			continue
		}
		sawRegulation = true
		merged.Regulations = append(merged.Regulations, batchRes.Regulations...)
	}
	if !sawRegulation {
		merged.Kind = "not_regulation"
	}
	if extractErr != nil {
		if failMeta, jerr := json.Marshal(regulationCustomMetadata{
			Kind:          "regulation",
			ExtractStatus: "failed",
			ExtractError:  "regulation extraction failed: " + extractErr.Error(),
		}); jerr == nil {
			if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(failMeta)); serr != nil {
				logger.Warnf(ctx, "Failed to persist regulation extraction failed-state: %v", serr)
			}
		}
		logger.Error(ctx, "Regulation extraction model call failed", extractErr)
		c.Error(errors.NewInternalServerError("regulation extraction failed: " + extractErr.Error()))
		return
	}
	extracted := merged
	// 自动编号去重：无制度编号的制度按当天已有最大序号 +1 顺序递增（跨文件唯一）
	autoSeq, seqErr := h.kgService.MaxAutoRegulationSeq(effCtx, kbID)
	if seqErr != nil {
		logger.Warnf(ctx, "Failed to compute auto regulation seq for %s: %v", kbID, seqErr)
	}
	service.NormalizeRegulationExtractionResult(extracted, autoSeq)

	meta := regulationCustomMetadata{
		Kind:          extracted.Kind,
		Regulations:   extracted.Regulations,
		ExtractStatus: "success",
		ExtractError:  extracted.ExtractError,
	}
	// 自定义识别规则：制度类型归类（用户可配置关键词/正则 → 制度类型），
	// 按模型提取出的制度类型字段匹配，命中则覆盖模型结果。
	if recCfg, rerr := h.kgService.GetRecognitionConfig(effCtx, kbID); rerr == nil {
		for i := range meta.Regulations {
			if t := service.ClassifyTypeFromRules(meta.Regulations[i].RegType, recCfg); t != "" {
				meta.Regulations[i].RegType = t
			}
		}
	}
	if extracted.Kind == "not_regulation" {
		// 自定义识别规则捞回：模型判非但包含规则命中 → 认定为制度，置 manual 待补录，
		// 不自动删除（解决"该是制度却没入库"的误判）。
		if recCfg, rerr := h.kgService.GetRecognitionConfig(effCtx, kbID); rerr == nil {
			if service.MatchIncludeRules(strings.Join(batches, "\n"), recCfg) {
				meta.Kind = "regulation"
				meta.Regulations = []types.RegulationExtractionItem{}
				meta.ExtractStatus = "manual"
				meta.ExtractError = "识别规则命中，已认定为制度，请编辑补录字段"
				if raw, merr := json.Marshal(meta); merr == nil {
					if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(raw)); serr != nil {
						logger.Warnf(ctx, "Failed to persist rule-rescued regulation meta: %v", serr)
					}
				}
				logger.Infof(ctx, "Regulation rule-rescued by include rule, knowledge %s kept as manual", knowledgeID)
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"message": "识别规则命中，已认定为制度，请在列表中编辑补录字段",
					"data":    map[string]interface{}{"kind": "regulation", "extract_status": "manual", "removed": false},
				})
				return
			}
		}
		meta.ExtractStatus = "not_regulation"
		if meta.ExtractError == "" {
			meta.ExtractError = "document does not look like a regulation"
		}
		// 防循环：auto_deleted_count >= 1 说明该文件曾被自动删除后由用户从删除历史
		// 恢复，再次判定非制度不再自动删除，改为 failed 保留（避免 删除→恢复→删除 死循环）。
		if h.autoDeleteCount(effCtx, knowledge) > 0 {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "系统判定非制度文件，但该文件已被人工恢复，已保留待人工处理"
			logger.Warnf(ctx, "Regulation re-extraction judged non-regulation but row was restored before; keeping file %s", knowledgeID)
		} else {
			// 非制度文件不能存在于制度知识库 → 提取判定后自动删除该文件（保留物理文件，
			// 写入删除历史，可在"删除历史"抽屉中查看/恢复/永久删除）
			if delErr := h.kgService.AutoDeleteKnowledge(effCtx, knowledgeID, "not_regulation"); delErr != nil {
				logger.Warnf(ctx, "auto-delete non-regulation knowledge failed: %v", delErr)
			} else {
				logger.Infof(ctx, "auto-deleted non-regulation knowledge, ID: %s", knowledgeID)
				c.JSON(http.StatusOK, gin.H{
					"success":  true,
					"message":  "非制度文件已移至删除历史，可在删除历史中恢复",
					"data":     map[string]interface{}{"removed": true, "kind": "not_regulation"},
				})
				return
			}
		}
	}
	// 空提取守卫：标记为制度但没有可用制度（空数组或全部空字段）→ failed 可重试
	if meta.Kind == "regulation" {
		if len(meta.Regulations) == 0 {
			meta.ExtractStatus = "failed"
			if meta.ExtractError == "" {
				meta.ExtractError = "提取未返回任何制度，请重试"
			}
		} else if service.RegulationsAllEmpty(meta.Regulations) {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "提取结果字段为空，请重试"
		}
	}

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		c.Error(errors.NewInternalServerError("failed to encode extraction result"))
		return
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(metaJSON)); err != nil {
		logger.Error(ctx, "Failed to persist regulation extraction result", err)
		c.Error(errors.NewInternalServerError("failed to save extraction result: " + err.Error()))
		return
	}

	logger.Infof(ctx, "Regulation extraction succeeded, knowledge ID: %s, kind: %s, regulations: %d",
		knowledgeID, meta.Kind, len(meta.Regulations))
	if meta.ExtractStatus == "failed" {
		logger.Warnf(ctx, "Regulation extraction returned an unusable result, knowledge ID: %s, err: %s", knowledgeID, meta.ExtractError)
		c.Error(errors.NewInternalServerError(meta.ExtractError))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Regulation extraction succeeded",
		"data": map[string]interface{}{
			"knowledge_id":    knowledgeID,
			"kind":            meta.Kind,
			"extract_status":  meta.ExtractStatus,
			"regulations":     meta.Regulations,
		},
	})
}

// GetRecognitionConfig godoc
// @Summary      获取识别规则配置
// @Description  返回知识库级文档识别规则（包含判定规则 + 类型归类规则），发票/合同管理页的设置面板读取。
// @Tags         知识库
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "知识库ID"
// @Success      200  {object}  types.RecognitionConfig  "识别规则配置"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/recognition-config [get]
func (h *KnowledgeBaseHandler) GetRecognitionConfig(c *gin.Context) {
	ctx := c.Request.Context()
	_, kbID, effectiveTenantID, _, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		c.Error(err)
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	cfg, err := h.knowledgeService.GetRecognitionConfig(effCtx, kbID)
	if err != nil {
		logger.Error(ctx, "Failed to get recognition config", err)
		c.Error(errors.NewInternalServerError("get recognition config failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

// ListDeletedKnowledge godoc
// @Summary      删除历史列表
// @Description  返回知识库中所有被系统自动删除（判定非合同/非发票）且保留源文件的记录，供"删除历史"抽屉查看、恢复、永久删除。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id        path  string  true  "知识库ID"
// @Param        page      query int     false "页码"
// @Param        page_size query int     false "每页数量"
// @Param        q         query string  false "按文件名/标题搜索"
// @Success      200  {object}  types.DeletedKnowledgePage  "删除历史列表"
// @Failure      403  {object}  errors.AppError             "无权限"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/deleted-knowledge [get]
func (h *KnowledgeHandler) ListDeletedKnowledge(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	if kbID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id cannot be empty"))
		return
	}
	_, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to list deleted knowledge"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.kgService.ListDeletedKnowledge(effCtx, kbID, page, pageSize, c.Query("q"))
	if err != nil {
		logger.Error(ctx, "Failed to list deleted knowledge", err)
		c.Error(errors.NewInternalServerError("list deleted knowledge failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, result)
}

// RestoreDeletedKnowledge godoc
// @Summary      恢复自动删除的记录
// @Description  把被系统自动删除（非合同/非发票）的记录恢复回知识库并重新解析提取；再次判定非该类文档时不再自动删除（防循环）。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id           path  string  true  "知识库ID"
// @Param        knowledgeId  path  string  true  "知识ID"
// @Success      200  {object}  map[string]interface{}  "恢复成功"
// @Failure      403  {object}  errors.AppError         "无权限"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/deleted-knowledge/{knowledgeId}/restore [post]
func (h *KnowledgeHandler) RestoreDeletedKnowledge(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	knowledgeID := secutils.SanitizeForLog(c.Param("knowledgeId"))
	if kbID == "" || knowledgeID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id and knowledge id cannot be empty"))
		return
	}
	_, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to restore deleted knowledge"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	if _, err := h.kgService.RestoreDeletedKnowledge(effCtx, knowledgeID); err != nil {
		logger.Error(ctx, "Failed to restore deleted knowledge", err)
		c.Error(errors.NewInternalServerError("restore deleted knowledge failed: " + err.Error()))
		return
	}
	logger.Infof(ctx, "Restored auto-deleted knowledge %s", knowledgeID)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "已重新入库，请在列表中编辑补录字段",
		"data":    map[string]interface{}{"knowledge_id": knowledgeID},
	})
}

// PurgeDeletedKnowledge godoc
// @Summary      永久删除历史记录
// @Description  永久删除一条删除历史记录（DB 硬删 + 物理源文件删除），不可恢复。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id           path  string  true  "知识库ID"
// @Param        knowledgeId  path  string  true  "知识ID"
// @Success      200  {object}  map[string]interface{}  "永久删除成功"
// @Failure      403  {object}  errors.AppError         "无权限"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/deleted-knowledge/{knowledgeId}/purge [post]
func (h *KnowledgeHandler) PurgeDeletedKnowledge(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	knowledgeID := secutils.SanitizeForLog(c.Param("knowledgeId"))
	if kbID == "" || knowledgeID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id and knowledge id cannot be empty"))
		return
	}
	_, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to purge deleted knowledge"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	if err := h.kgService.PurgeDeletedKnowledge(effCtx, knowledgeID); err != nil {
		logger.Error(ctx, "Failed to purge deleted knowledge", err)
		c.Error(errors.NewInternalServerError("purge deleted knowledge failed: " + err.Error()))
		return
	}
	logger.Infof(ctx, "Purged auto-deleted knowledge %s permanently", knowledgeID)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Knowledge purged permanently",
		"data":    map[string]interface{}{"knowledge_id": knowledgeID},
	})
}

// ExtractInvoice godoc
// @Summary      提取发票字段
// @Description  读取已解析文档文本，调用提取模型（复用知识库 summary_model_id）提取发票字段并写入 custom_metadata。幂等：对同一知识重复调用会覆盖写。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id           path  string  true  "知识库ID"
// @Param        knowledgeId  path  string  true  "知识ID"
// @Success      200  {object}  map[string]interface{}  "提取成功"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      403  {object}  errors.AppError         "无权限"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/extract-invoice [post]
func (h *KnowledgeHandler) ExtractInvoice(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	knowledgeID := secutils.SanitizeForLog(c.Param("knowledgeId"))
	if kbID == "" || knowledgeID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id and knowledge id cannot be empty"))
		return
	}

	kb, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to extract invoice fields"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	knowledge, err := h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for invoice extraction", err)
		c.Error(errors.NewNotFoundError("Knowledge not found"))
		return
	}
	if knowledge.KnowledgeBaseID != kbID {
		c.Error(errors.NewBadRequestError("Knowledge does not belong to the given knowledge base"))
		return
	}
	if knowledge.ParseStatus != types.ParseStatusCompleted {
		c.Error(errors.NewBadRequestError("document has not been parsed yet, please wait for parsing to finish"))
		return
	}
	// 待补录豁免：manual 记录由用户人工维护，不自动重提取、不参与任何自动删除判定。
	var curInvoiceMeta invoiceCustomMetadata
	_ = json.Unmarshal(knowledge.CustomMetadata, &curInvoiceMeta)
	if curInvoiceMeta.ExtractStatus == "manual" {
		logger.Infof(ctx, "Invoice extraction skipped: knowledge %s is manual intake", knowledgeID)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "待补录记录，无需重复提取",
			"data":    map[string]interface{}{"kind": "invoice", "extract_status": "manual", "removed": false},
		})
		return
	}

	modelID := strings.TrimSpace(kb.SummaryModelID)
	if modelID == "" {
		c.Error(errors.NewBadRequestError("no extraction model configured for the knowledge base"))
		return
	}
	chatModel, err := h.modelService.GetChatModel(effCtx, modelID)
	if err != nil {
		logger.Error(ctx, "Failed to load extraction model", err)
		c.Error(errors.NewInternalServerError("get extraction model failed: " + err.Error()))
		return
	}

	chunks, err := h.chunkService.ListChunksByKnowledgeID(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to list chunks for invoice extraction", err)
		c.Error(errors.NewInternalServerError("list chunks failed: " + err.Error()))
		return
	}
	// 分批提取：超长多发票文档（18/22 张）单次 LLM 调用不可靠（智谱 500 /
	// 输出截断），按 chunk 分批独立调用后合并，Normalize 时按发票号去重。
	batches := service.BuildInvoiceExtractionBatches(knowledge.FileName, knowledge.Description, chunks, 0)
	// 扫描件防御：无文本层（扫描件 PDF / 纯图片）时不自动删除，保留文件并标记
	// failed 让用户人工处理。
	if !invoiceBatchesHaveText(batches) {
		noTextMeta, jerr := json.Marshal(invoiceCustomMetadata{
			Kind:          "invoice",
			ExtractStatus: "failed",
			ExtractError:  "文档无可提取文本（疑似扫描件），已保留文件待人工处理",
		})
		if jerr == nil {
			if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(noTextMeta)); serr != nil {
				logger.Warnf(ctx, "Failed to persist invoice no-text state: %v", serr)
			}
		}
		logger.Warnf(ctx, "Invoice extraction skipped: no text layer for knowledge %s (scanned document), file kept", knowledgeID)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "document has no extractable text (possible scanned document), file kept",
			"data":    map[string]interface{}{"kind": "invoice", "extract_status": "failed", "removed": false},
		})
		return
	}
	merged := &service.InvoiceExtractionResult{Kind: "invoice"}
	var extractErr error
	sawInvoice := false
	for i, batch := range batches {
		if strings.TrimSpace(batch) == "" {
			continue
		}
		batchRes, berr := service.ExtractInvoicesFromContent(effCtx, chatModel, batch)
		if berr != nil {
			// 首批失败且无任何结果 → 整体失败（记录 failed 状态可重试）；
			// 后续批失败仅告警跳过，保留已提取的部分。
			if i == 0 && len(merged.Invoices) == 0 {
				extractErr = berr
				break
			}
			logger.Warnf(ctx, "Invoice extraction batch %d failed (non-fatal): %v", i+1, berr)
			continue
		}
		if batchRes == nil || batchRes.Kind == "not_invoice" {
			continue
		}
		sawInvoice = true
		merged.Invoices = append(merged.Invoices, batchRes.Invoices...)
	}
	if !sawInvoice {
		merged.Kind = "not_invoice"
	}
	if extractErr != nil {
		// Persist a failed state so the UI shows "提取失败" (retriable via the
		// floating toolbar) instead of looping forever as "提取中/待提取".
		if failMeta, jerr := json.Marshal(invoiceCustomMetadata{
			Kind:          "invoice",
			ExtractStatus: "failed",
			ExtractError:  "invoice extraction failed: " + extractErr.Error(),
		}); jerr == nil {
			if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(failMeta)); serr != nil {
				logger.Warnf(ctx, "Failed to persist invoice extraction failed-state: %v", serr)
			}
		}
		logger.Error(ctx, "Invoice extraction model call failed", extractErr)
		c.Error(errors.NewInternalServerError("invoice extraction failed: " + extractErr.Error()))
		return
	}
	extracted := merged
	service.NormalizeInvoiceExtractionResult(extracted)

	meta := invoiceCustomMetadata{
		Kind:          extracted.Kind,
		Invoices:      extracted.Invoices,
		ExtractStatus: "success",
		ExtractError:  extracted.ExtractError,
	}
	// 自定义识别规则：类型归类（用户可配置关键词/正则 → 发票类型），
	// 按模型提取出的发票类型字段匹配，命中则覆盖内置关键字判定。
	if recCfg, rerr := h.kgService.GetRecognitionConfig(effCtx, kbID); rerr == nil {
		fullText := strings.Join(batches, "\n")
		for i := range meta.Invoices {
			if t := service.ClassifyTypeFromRules(meta.Invoices[i].InvoiceType, recCfg); t != "" {
				meta.Invoices[i].InvoiceType = t
			} else if meta.Invoices[i].InvoiceType == "" {
				// 模型未识别出类型时，用全文兜底归类（避免"其它票据"误归类）
				if t := service.ClassifyTypeFromRules(fullText, recCfg); t != "" {
					meta.Invoices[i].InvoiceType = t
				}
			}
		}
	}
	if extracted.Kind == "not_invoice" {
		// 自定义识别规则捞回：模型判非但包含规则命中 → 认定为发票，置 manual 待补录，
		// 不自动删除（避免"该是发票却没入库"）。
		if recCfg, rerr := h.kgService.GetRecognitionConfig(effCtx, kbID); rerr == nil {
			if service.MatchIncludeRules(strings.Join(batches, "\n"), recCfg) {
				meta.Kind = "invoice"
				meta.Invoices = []service.InvoiceExtractionItem{}
				meta.ExtractStatus = "manual"
				meta.ExtractError = "识别规则命中，已认定为发票，请编辑补录字段"
				if raw, merr := json.Marshal(meta); merr == nil {
					if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(raw)); serr != nil {
						logger.Warnf(ctx, "Failed to persist rule-rescued invoice meta: %v", serr)
					}
				}
				logger.Infof(ctx, "Invoice rule-rescued by include rule, knowledge %s kept as manual", knowledgeID)
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"message": "识别规则命中，已认定为发票，请在列表中编辑补录字段",
					"data":    map[string]interface{}{"kind": "invoice", "extract_status": "manual", "removed": false},
				})
				return
			}
		}
		meta.ExtractStatus = "not_invoice"
		if meta.ExtractError == "" {
			meta.ExtractError = "document does not look like an invoice"
		}
		// 防循环：auto_deleted_count >= 1（曾被自动删除后人工恢复）→ 不再自动删除，
		// 改为 failed 保留。
		if h.autoDeleteCount(effCtx, knowledge) > 0 {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "系统判定非发票文件，但该文件已被人工恢复，已保留待人工处理"
			logger.Warnf(ctx, "Invoice re-extraction judged non-invoice but row was restored before; keeping file %s", knowledgeID)
		} else {
			// 需求：非发票文件不能存在于发票知识库 → 提取判定后自动删除该文件
			// （保留物理文件写入删除历史，可在"删除历史"抽屉中查看/恢复/永久删除）。
			if delErr := h.kgService.AutoDeleteKnowledge(effCtx, knowledgeID, "not_invoice"); delErr != nil {
				logger.Warnf(ctx, "auto-delete non-invoice knowledge failed: %v", delErr)
			} else {
				logger.Infof(ctx, "auto-deleted non-invoice knowledge, ID: %s", knowledgeID)
				c.JSON(http.StatusOK, gin.H{
					"success":  true,
					"message":  "非发票文件已移至删除历史，可在删除历史中恢复",
					"data":     map[string]interface{}{"removed": true, "kind": "not_invoice"},
				})
				return
			}
		}
	}
	// Extraction-result guard: a model reply marked as invoice but carrying no
	// usable invoice (empty list, or every invoice blank) is a garbage/truncated
	// reply — persist it as failed (not success) so the UI shows a retriable
	// state instead of an empty "successful" row.
	if meta.Kind == "invoice" {
		if len(meta.Invoices) == 0 {
			meta.ExtractStatus = "failed"
			if meta.ExtractError == "" {
				meta.ExtractError = "提取未返回任何发票，请重试"
			}
		} else if service.InvoicesAllEmpty(meta.Invoices) {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "提取结果字段为空，请重试"
		}
	}

	// Cross-file dedup: mark invoices whose number already exists in another
	// document of the same KB, so the UI can hide them by default.
	if len(meta.Invoices) > 0 {
		h.markInvoiceDuplicates(effCtx, kbID, knowledgeID, &meta)
	}

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		c.Error(errors.NewInternalServerError("failed to encode extraction result"))
		return
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(metaJSON)); err != nil {
		logger.Error(ctx, "Failed to persist invoice extraction result", err)
		c.Error(errors.NewInternalServerError("failed to save extraction result: " + err.Error()))
		return
	}

	// 自动打标签功能已弃用（需求决策：不再自动创建/关联大类标签，
	// 保留手动标签）。category 字段仍随提取结果保存，仅不再自动转标签。

	logger.Infof(ctx, "Invoice extraction succeeded, knowledge ID: %s, kind: %s, invoices: %d",
		knowledgeID, meta.Kind, len(meta.Invoices))
	if meta.ExtractStatus == "failed" {
		logger.Warnf(ctx, "Invoice extraction returned an unusable result, knowledge ID: %s, err: %s", knowledgeID, meta.ExtractError)
		c.Error(errors.NewInternalServerError(meta.ExtractError))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Invoice extraction succeeded",
		"data": map[string]interface{}{
			"knowledge_id":    knowledgeID,
			"kind":            meta.Kind,
			"extract_status":  meta.ExtractStatus,
			"invoices":        meta.Invoices,
		},
	})
}

// ExtractInvoicePage godoc
// @Summary      按页重新提取发票
// @Description  对该知识文档中指定的第 N 张发票（发票按文档内出现顺序编号，即提取结果中的 page）单独重新提取，仅替换该张发票的数据，不影响同文件其它发票。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id           path  string  true  "知识库ID"
// @Param        knowledgeId  path  string  true  "知识ID"
// @Param        body         body  object  true  "{\"page\":1}"
// @Success      200  {object}  map[string]interface{}  "更新后的发票"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/extract-invoice-page [post]
func (h *KnowledgeHandler) ExtractInvoicePage(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	knowledgeID := secutils.SanitizeForLog(c.Param("knowledgeId"))
	if kbID == "" || knowledgeID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id and knowledge id cannot be empty"))
		return
	}
	kb, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to extract invoice fields"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		var req struct {
			Page int `json:"page"`
		}
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			logger.Warn(ctx, "extract-invoice-page: failed to decode body", err)
		} else if req.Page >= 1 {
			page = req.Page
		}
	}
	if page < 1 {
		c.Error(errors.NewBadRequestError("page must be a positive integer"))
		return
	}

	knowledge, err := h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for invoice page extraction", err)
		c.Error(errors.NewNotFoundError("Knowledge not found"))
		return
	}
	if knowledge.KnowledgeBaseID != kbID {
		c.Error(errors.NewBadRequestError("Knowledge does not belong to the given knowledge base"))
		return
	}
	if knowledge.ParseStatus != types.ParseStatusCompleted {
		c.Error(errors.NewBadRequestError("document has not been parsed yet, please wait for parsing to finish"))
		return
	}
	var meta invoiceCustomMetadata
	if err := json.Unmarshal(knowledge.CustomMetadata, &meta); err != nil || meta.Kind != "invoice" || len(meta.Invoices) == 0 {
		c.Error(errors.NewBadRequestError("no invoice extraction data, please run single-file extraction first"))
		return
	}
	if page > len(meta.Invoices) {
		c.Error(errors.NewBadRequestError(fmt.Sprintf("page %d out of range (1-%d)", page, len(meta.Invoices))))
		return
	}

	modelID := strings.TrimSpace(kb.SummaryModelID)
	if modelID == "" {
		c.Error(errors.NewBadRequestError("no extraction model configured for the knowledge base"))
		return
	}
	chatModel, err := h.modelService.GetChatModel(effCtx, modelID)
	if err != nil {
		logger.Error(ctx, "Failed to load extraction model", err)
		c.Error(errors.NewInternalServerError("get extraction model failed: " + err.Error()))
		return
	}

	chunks, err := h.chunkService.ListChunksByKnowledgeID(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to list chunks for invoice page extraction", err)
		c.Error(errors.NewInternalServerError("list chunks failed: " + err.Error()))
		return
	}
	content := service.BuildInvoiceExtractionContent(knowledge.FileName, knowledge.Description, chunks)
	res, err := service.ExtractInvoicePageFromContent(effCtx, chatModel, content, page)
	if err != nil {
		logger.Error(ctx, "Invoice page extraction model call failed", err)
		c.Error(errors.NewInternalServerError("invoice page extraction failed: " + err.Error()))
		return
	}
	service.NormalizeInvoiceExtractionResult(res)
	if res == nil || res.Kind == "not_invoice" || len(res.Invoices) == 0 {
		c.Error(errors.NewBadRequestError("unable to re-extract this invoice, please try single-file extraction"))
		return
	}

	upd := res.Invoices[0]
	upd.Page = page // 保持原页码
	meta.Invoices[page-1] = upd
	metaBytes, jerr := json.Marshal(meta)
	if jerr != nil {
		logger.Error(ctx, "Failed to encode invoice page extraction result", jerr)
		c.Error(errors.NewInternalServerError("failed to encode extraction result"))
		return
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(metaBytes)); err != nil {
		logger.Error(ctx, "Failed to persist invoice page extraction result", err)
		c.Error(errors.NewInternalServerError("failed to save extraction result: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Invoice page extraction succeeded",
		"data":    upd,
	})
}

// DeleteInvoicePage godoc
// @Summary      删除指定发票记录
// @Description  按发票页码从 custom_metadata.invoices 中移除该张发票；若该文档仅剩这一张发票，则整份文档一并删除。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id           path  string  true  "知识库ID"
// @Param        knowledgeId  path  string  true  "知识ID"
// @Param        request      body  object       true  "page：发票页码（文档内出现顺序，从 1 开始）"
// @Success      200  {object}  map[string]interface{}  "deleted_file: 是否整份删除"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      403  {object}  errors.AppError         "无权限"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/delete-invoice-page [post]
func (h *KnowledgeHandler) DeleteInvoicePage(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	knowledgeID := secutils.SanitizeForLog(c.Param("knowledgeId"))
	if kbID == "" || knowledgeID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id and knowledge id cannot be empty"))
		return
	}
	kb, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	_ = kb
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to delete invoice records"))
		return
	}
	if err := h.requireKBOwnershipOrAdmin(c, kbID); err != nil {
		c.Error(err)
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		var req struct {
			Page int `json:"page"`
		}
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			logger.Warn(ctx, "delete-invoice-page: failed to decode body", err)
		} else if req.Page >= 1 {
			page = req.Page
		}
	}
	if page < 1 {
		c.Error(errors.NewBadRequestError("page must be a positive integer"))
		return
	}

	knowledge, err := h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for invoice page delete", err)
		c.Error(errors.NewNotFoundError("Knowledge not found"))
		return
	}
	if knowledge.KnowledgeBaseID != kbID {
		c.Error(errors.NewBadRequestError("Knowledge does not belong to the given knowledge base"))
		return
	}
	var meta invoiceCustomMetadata
	if err := json.Unmarshal(knowledge.CustomMetadata, &meta); err != nil || meta.Kind != "invoice" || len(meta.Invoices) == 0 {
		c.Error(errors.NewBadRequestError("no invoice extraction data to delete"))
		return
	}
	if page > len(meta.Invoices) {
		c.Error(errors.NewBadRequestError(fmt.Sprintf("page %d out of range (1-%d)", page, len(meta.Invoices))))
		return
	}

	// 移除该页发票
	targetIdx := -1
	for i, inv := range meta.Invoices {
		if i == page-1 || inv.Page == page {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		targetIdx = page - 1
	}
	meta.Invoices = append(meta.Invoices[:targetIdx], meta.Invoices[targetIdx+1:]...)
	// 后续发票页码前移，保持页码=文档内出现顺序
	for i := range meta.Invoices {
		meta.Invoices[i].Page = i + 1
	}

	// 删空：整份文档一并删除
	if len(meta.Invoices) == 0 {
		if _, err := h.enqueueKnowledgeListDelete(effCtx, effectiveTenantID, []string{knowledgeID}); err != nil {
			logger.Error(ctx, "Failed to enqueue knowledge delete after removing last invoice", err)
			c.Error(errors.NewInternalServerError("failed to schedule delete: " + err.Error()))
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success":     true,
			"message":     "Invoice removed, source file scheduled for deletion",
			"deleted_file": true,
		})
		return
	}

	metaBytes, jerr := json.Marshal(meta)
	if jerr != nil {
		logger.Error(ctx, "Failed to encode invoice metadata after delete", jerr)
		c.Error(errors.NewInternalServerError("failed to encode metadata"))
		return
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(metaBytes)); err != nil {
		logger.Error(ctx, "Failed to persist invoice metadata after delete", err)
		c.Error(errors.NewInternalServerError("failed to save metadata: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"message":     "Invoice record deleted",
		"deleted_file": false,
		"data":        meta.Invoices,
	})
}

// ExtractContractPage godoc
// @Summary      按页重新提取合同
// @Description  对该知识文档中指定的第 N 份合同（合同按文档内出现顺序编号，即提取结果中的 page）单独重新提取，仅替换该份合同的数据，不影响同文件其它合同。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id           path  string  true  "知识库ID"
// @Param        knowledgeId  path  string  true  "知识ID"
// @Param        body         body  object  true  "{\"page\":1}"
// @Success      200  {object}  map[string]interface{}  "更新后的合同"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/extract-contract-page [post]
func (h *KnowledgeHandler) ExtractContractPage(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	knowledgeID := secutils.SanitizeForLog(c.Param("knowledgeId"))
	if kbID == "" || knowledgeID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id and knowledge id cannot be empty"))
		return
	}
	kb, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to extract contract fields"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		var req struct {
			Page int `json:"page"`
		}
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			logger.Warn(ctx, "extract-contract-page: failed to decode body", err)
		} else if req.Page >= 1 {
			page = req.Page
		}
	}
	if page < 1 {
		c.Error(errors.NewBadRequestError("page must be a positive integer"))
		return
	}

	knowledge, err := h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for contract page extraction", err)
		c.Error(errors.NewNotFoundError("Knowledge not found"))
		return
	}
	if knowledge.KnowledgeBaseID != kbID {
		c.Error(errors.NewBadRequestError("Knowledge does not belong to the given knowledge base"))
		return
	}
	if knowledge.ParseStatus != types.ParseStatusCompleted {
		c.Error(errors.NewBadRequestError("document has not been parsed yet, please wait for parsing to finish"))
		return
	}
	var meta contractCustomMetadata
	if err := json.Unmarshal(knowledge.CustomMetadata, &meta); err != nil || meta.Kind != "contract" || len(meta.Contracts) == 0 {
		c.Error(errors.NewBadRequestError("no contract extraction data, please run single-file extraction first"))
		return
	}
	if page > len(meta.Contracts) {
		c.Error(errors.NewBadRequestError(fmt.Sprintf("page %d out of range (1-%d)", page, len(meta.Contracts))))
		return
	}

	modelID := strings.TrimSpace(kb.SummaryModelID)
	if modelID == "" {
		c.Error(errors.NewBadRequestError("no extraction model configured for the knowledge base"))
		return
	}
	chatModel, err := h.modelService.GetChatModel(effCtx, modelID)
	if err != nil {
		logger.Error(ctx, "Failed to load extraction model", err)
		c.Error(errors.NewInternalServerError("get extraction model failed: " + err.Error()))
		return
	}

	chunks, err := h.chunkService.ListChunksByKnowledgeID(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to list chunks for contract page extraction", err)
		c.Error(errors.NewInternalServerError("list chunks failed: " + err.Error()))
		return
	}
	content := service.BuildContractExtractionContent(knowledge.FileName, knowledge.Description, chunks)
	res, err := service.ExtractContractPageFromContent(effCtx, chatModel, content, page)
	if err != nil {
		logger.Error(ctx, "Contract page extraction model call failed", err)
		c.Error(errors.NewInternalServerError("contract page extraction failed: " + err.Error()))
		return
	}
	prevNo := meta.Contracts[page-1].ContractNo
	service.NormalizeContractExtractionResult(res, 0)
	if res == nil || res.Kind == "not_contract" || len(res.Contracts) == 0 {
		c.Error(errors.NewBadRequestError("unable to re-extract this contract, please try single-file extraction"))
		return
	}

	upd := res.Contracts[0]
	upd.Page = page // 保持原页码
	// 保留原合同编号，避免单页重新提取时自动编号漂移产生新号
	if prevNo != "" {
		upd.ContractNo = prevNo
	}
	meta.Contracts[page-1] = upd
	metaBytes, jerr := json.Marshal(meta)
	if jerr != nil {
		logger.Error(ctx, "Failed to encode contract page extraction result", jerr)
		c.Error(errors.NewInternalServerError("failed to encode extraction result"))
		return
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(metaBytes)); err != nil {
		logger.Error(ctx, "Failed to persist contract page extraction result", err)
		c.Error(errors.NewInternalServerError("failed to save extraction result: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Contract page extraction succeeded",
		"data":    upd,
	})
}

// DeleteContractPage godoc
// @Summary      删除指定合同记录
// @Description  按合同页码从 custom_metadata.contracts 中移除该份合同；若该文档仅剩这一份合同，则整份文档一并删除。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id           path  string  true  "知识库ID"
// @Param        knowledgeId  path  string  true  "知识ID"
// @Param        request      body  object       true  "page：合同页码（文档内出现顺序，从 1 开始）"
// @Success      200  {object}  map[string]interface{}  "deleted_file: 是否整份删除"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      403  {object}  errors.AppError         "无权限"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/delete-contract-page [post]
func (h *KnowledgeHandler) DeleteContractPage(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	knowledgeID := secutils.SanitizeForLog(c.Param("knowledgeId"))
	if kbID == "" || knowledgeID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id and knowledge id cannot be empty"))
		return
	}
	kb, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	_ = kb
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to delete contract records"))
		return
	}
	if err := h.requireKBOwnershipOrAdmin(c, kbID); err != nil {
		c.Error(err)
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		var req struct {
			Page int `json:"page"`
		}
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			logger.Warn(ctx, "delete-contract-page: failed to decode body", err)
		} else if req.Page >= 1 {
			page = req.Page
		}
	}
	if page < 1 {
		c.Error(errors.NewBadRequestError("page must be a positive integer"))
		return
	}

	knowledge, err := h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for contract page delete", err)
		c.Error(errors.NewNotFoundError("Knowledge not found"))
		return
	}
	if knowledge.KnowledgeBaseID != kbID {
		c.Error(errors.NewBadRequestError("Knowledge does not belong to the given knowledge base"))
		return
	}
	var meta contractCustomMetadata
	if err := json.Unmarshal(knowledge.CustomMetadata, &meta); err != nil || meta.Kind != "contract" || len(meta.Contracts) == 0 {
		c.Error(errors.NewBadRequestError("no contract extraction data to delete"))
		return
	}
	if page > len(meta.Contracts) {
		c.Error(errors.NewBadRequestError(fmt.Sprintf("page %d out of range (1-%d)", page, len(meta.Contracts))))
		return
	}

	// 移除该份合同
	targetIdx := -1
	for i, ct := range meta.Contracts {
		if i == page-1 || ct.Page == page {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		targetIdx = page - 1
	}
	meta.Contracts = append(meta.Contracts[:targetIdx], meta.Contracts[targetIdx+1:]...)
	// 后续合同页码前移，保持页码=文档内出现顺序
	for i := range meta.Contracts {
		meta.Contracts[i].Page = i + 1
	}

	// 删空：整份文档一并删除
	if len(meta.Contracts) == 0 {
		if _, err := h.enqueueKnowledgeListDelete(effCtx, effectiveTenantID, []string{knowledgeID}); err != nil {
			logger.Error(ctx, "Failed to enqueue knowledge delete after removing last contract", err)
			c.Error(errors.NewInternalServerError("failed to schedule delete: " + err.Error()))
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success":     true,
			"message":     "Contract removed, source file scheduled for deletion",
			"deleted_file": true,
		})
		return
	}

	metaBytes, jerr := json.Marshal(meta)
	if jerr != nil {
		logger.Error(ctx, "Failed to encode contract metadata after delete", jerr)
		c.Error(errors.NewInternalServerError("failed to encode metadata"))
		return
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(metaBytes)); err != nil {
		logger.Error(ctx, "Failed to persist contract metadata after delete", err)
		c.Error(errors.NewInternalServerError("failed to save metadata: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"message":     "Contract record deleted",
		"deleted_file": false,
		"data":        meta.Contracts,
	})
}

// markInvoiceDuplicates scans the other documents of the same KB and flags
// invoices whose number already appears elsewhere. In-file duplicates are
// already collapsed by the model prompt; this pass only handles cross-file
// repetition.
func (h *KnowledgeHandler) markInvoiceDuplicates(ctx context.Context, kbID, currentKnowledgeID string, meta *invoiceCustomMetadata) {
	if len(meta.Invoices) == 0 {
		return
	}
	// Existing numbers from sibling documents, keyed by invoice_no.
	existing := make(map[string]struct{})
	result, err := h.kgService.ListPagedKnowledgeByKnowledgeBaseID(ctx, kbID, &types.Pagination{Page: 1, PageSize: 1000}, types.KnowledgeListFilter{})
	if err != nil {
		logger.Warnf(ctx, "Invoice dedup scan failed, skipping duplicate marking: %v", err)
		return
	}
	items, _ := result.Data.([]*types.Knowledge)
	for _, k := range items {
		if k == nil || k.ID == currentKnowledgeID || len(k.CustomMetadata) == 0 {
			continue
		}
		var sibling struct {
			Invoices []service.InvoiceExtractionItem `json:"invoices"`
		}
		if err := json.Unmarshal(k.CustomMetadata, &sibling); err != nil {
			continue
		}
		for _, inv := range sibling.Invoices {
			if no := strings.TrimSpace(inv.InvoiceNo); no != "" {
				existing[no] = struct{}{}
			}
		}
	}
	if len(existing) == 0 {
		return
	}
	for i := range meta.Invoices {
		if no := strings.TrimSpace(meta.Invoices[i].InvoiceNo); no != "" {
			if _, dup := existing[no]; dup {
				meta.Invoices[i].Duplicate = true
			}
		}
	}
}

// RegenerateKnowledgeSummary refreshes a stale summary after chunk or metadata edits.
func (h *KnowledgeHandler) RegenerateKnowledgeSummary(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}
	_, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.OrgRoleEditor)
	if err != nil {
		c.Error(err)
		return
	}
	knowledge, err := h.kgService.GetKnowledgeByID(effCtx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	if knowledge.SummaryStatus == "" || knowledge.SummaryStatus == types.SummaryStatusNone {
		knowledge, err = h.kgService.RegenerateKnowledgeSummary(effCtx, id)
	} else {
		err = h.kgService.RequestKnowledgeSummaryRefresh(effCtx, id)
		if err == nil {
			knowledge, err = h.kgService.GetKnowledgeByID(effCtx, id)
		}
	}
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": knowledge})
}

// UpdateManualKnowledge godoc
// @Summary      更新手工知识
// @Description  更新手工录入的Markdown知识内容
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id       path      string                       true  "知识ID"
// @Param        request  body      types.ManualKnowledgePayload true  "手工知识内容"
// @Success      200      {object}  map[string]interface{}       "更新后的知识"
// @Failure      400      {object}  errors.AppError              "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge/manual/{id} [put]
func (h *KnowledgeHandler) UpdateManualKnowledge(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start updating manual knowledge")

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Knowledge ID is empty")
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	_, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.OrgRoleEditor)
	if err != nil {
		c.Error(err)
		return
	}

	var req types.ManualKnowledgePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse manual knowledge update request", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	knowledge, err := h.kgService.UpdateManualKnowledge(effCtx, id, &req)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_id": id,
		})
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Manual knowledge updated successfully, knowledge ID: %s", id)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    knowledge,
	})
}

// ReparseKnowledge godoc
// @Summary      重新解析知识
// @Description  删除知识中现有的文档内容并重新解析，使用异步任务方式处理
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "知识ID"
// @Param        body body      object  false  "可选的处理配置覆盖：{\"process_config\": KnowledgeProcessOverrides}"
// @Success      200  {object}  map[string]interface{}  "重新解析任务已提交"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      403  {object}  errors.AppError         "权限不足"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge/{id}/reparse [post]
func (h *KnowledgeHandler) ReparseKnowledge(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start re-parsing knowledge")

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Knowledge ID is empty")
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	// Validate KB access with editor permission (reparse requires write access)
	_, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.OrgRoleEditor)
	if err != nil {
		c.Error(err)
		return
	}

	// Optional per-reparse parse config override. Empty body keeps the
	// overrides stored at upload time.
	var processOverrides *types.KnowledgeProcessOverrides
	if c.Request.ContentLength != 0 {
		var req struct {
			ProcessConfig *types.KnowledgeProcessOverrides `json:"process_config"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			logger.Error(ctx, "Failed to parse reparse request body", err)
			c.Error(errors.NewBadRequestError("Invalid reparse request body").WithDetails(err.Error()))
			return
		}
		processOverrides = req.ProcessConfig
	}

	// Call service to reparse knowledge
	knowledge, err := h.kgService.ReparseKnowledge(effCtx, id, processOverrides)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_id": id,
		})
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Knowledge reparse task submitted successfully, knowledge ID: %s", id)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Knowledge reparse task submitted",
		"data":    knowledge,
	})
}

// CancelKnowledgeParse godoc
// @Summary      取消知识解析
// @Description  取消进行中的知识解析任务。当前已写入的 chunk / 索引保留，可通过 reparse 接口重新触发解析。已完成 / 已失败 / 删除中的知识不支持取消。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "知识ID"
// @Success      200  {object}  map[string]interface{}  "取消已提交"
// @Failure      400  {object}  errors.AppError         "状态不支持取消"
// @Failure      403  {object}  errors.AppError         "权限不足"
// @Failure      404  {object}  errors.AppError         "知识不存在"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge/{id}/cancel-parse [post]
func (h *KnowledgeHandler) CancelKnowledgeParse(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start cancelling knowledge parse")

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Knowledge ID is empty")
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}

	// Editor permission — same gate as ReparseKnowledge / DeleteKnowledge.
	_, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.OrgRoleEditor)
	if err != nil {
		c.Error(err)
		return
	}

	knowledge, err := h.kgService.CancelKnowledgeParse(effCtx, id)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_id": id,
		})
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Knowledge parse cancelled successfully, knowledge ID: %s", id)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Knowledge parse cancelled",
		"data":    knowledge,
	})
}

type knowledgeTagBatchRequest struct {
	Updates map[string][]string `json:"updates" binding:"required,min=1"`
	KBID    string              `json:"kb_id"` // Optional: scope to this KB (validates editor access and uses effective tenant for shared KB)
}

// UpdateKnowledgeTagBatch godoc
// @Summary      批量更新知识标签
// @Description  批量更新知识条目的标签。可选 kb_id：指定时按该知识库校验编辑权限并用于共享知识库的空间解析
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        request  body      object  true  "标签更新请求（updates 必填，kb_id 可选）"
// @Success      200      {object}  map[string]interface{}  "更新成功"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge/tags [put]
func (h *KnowledgeHandler) UpdateKnowledgeTagBatch(c *gin.Context) {
	ctx := c.Request.Context()

	// Ensure tenant ID is in context (service reads it; may be missing if request context was not set by auth)
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	ctx = context.WithValue(ctx, types.TenantIDContextKey, tenantID)

	var req knowledgeTagBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse knowledge tag batch request", err)
		c.Error(errors.NewBadRequestError("请求参数不合法").WithDetails(err.Error()))
		return
	}
	// Resolve effective tenant and the authorized KB scope.
	var authorizedKBID string
	if kbID := secutils.SanitizeForLog(req.KBID); kbID != "" {
		_, _, effID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
		if err != nil {
			c.Error(err)
			return
		}
		if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
			c.Error(errors.NewForbiddenError("No permission to update knowledge tags"))
			return
		}
		authorizedKBID = kbID
		ctx = context.WithValue(ctx, types.TenantIDContextKey, effID)
	} else if len(req.Updates) > 0 {
		// No kb_id: infer from first knowledge ID so shared-KB updates work without client sending kb_id
		var firstKnowledgeID string
		for id := range req.Updates {
			firstKnowledgeID = id
			break
		}
		if firstKnowledgeID != "" {
			knowledge, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, firstKnowledgeID, types.OrgRoleEditor)
			if err != nil {
				c.Error(err)
				return
			}
			authorizedKBID = knowledge.KnowledgeBaseID
			ctx = effCtx
		}
	}
	if err := h.kgService.UpdateKnowledgeTagBatch(ctx, authorizedKBID, req.Updates); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// UpdateImageInfo godoc
// @Summary      更新图像信息
// @Description  更新知识分块的图像信息
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id        path      string  true  "知识ID"
// @Param        chunk_id  path      string  true  "分块ID"
// @Param        request   body      object{image_info=string}  true  "图像信息"
// @Success      200       {object}  map[string]interface{}     "更新成功"
// @Failure      400       {object}  errors.AppError            "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge/image/{id}/{chunk_id} [put]
func (h *KnowledgeHandler) UpdateImageInfo(c *gin.Context) {
	ctx := c.Request.Context()
	logger.Info(ctx, "Start updating image info")

	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		logger.Error(ctx, "Knowledge ID is empty")
		c.Error(errors.NewBadRequestError("Knowledge ID cannot be empty"))
		return
	}
	chunkID := secutils.SanitizeForLog(c.Param("chunk_id"))
	if chunkID == "" {
		logger.Error(ctx, "Chunk ID is empty")
		c.Error(errors.NewBadRequestError("Chunk ID cannot be empty"))
		return
	}

	_, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, id, types.OrgRoleEditor)
	if err != nil {
		c.Error(err)
		return
	}

	var request struct {
		ImageInfo string `json:"image_info"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	logger.Infof(ctx, "Updating knowledge chunk, knowledge ID: %s, chunk ID: %s", id, chunkID)
	err = h.kgService.UpdateImageInfo(effCtx, id, chunkID, secutils.SanitizeForLog(request.ImageInfo))
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Knowledge chunk updated successfully, knowledge ID: %s, chunk ID: %s", id, chunkID)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Knowledge chunk image updated successfully",
	})
}

// SearchKnowledge godoc
// @Summary      Search knowledge
// @Description  Search knowledge files by keyword. Pass recent=true without a keyword to browse recent files. When agent_id is set (shared agent), scope is the agent's configured knowledge bases.
// @Tags         Knowledge
// @Accept       json
// @Produce      json
// @Param        keyword    query     string  false "Keyword to search"
// @Param        offset     query     int     false "Offset for pagination (minimum 0)" minimum(0)
// @Param        limit      query     int     false "Limit for pagination (default 20, maximum 100)" minimum(1) maximum(100)
// @Param        file_types query     string  false "Comma-separated file extensions to filter (e.g., csv,xlsx)"
// @Param        agent_id   query     string  false "Shared agent ID (search within agent's KB scope)"
// @Param        recent     query     bool    false "Return recent files when keyword is empty"
// @Success      200         {object}  map[string]interface{}     "Search results"
// @Failure      400         {object}  errors.AppError            "Invalid request"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge/search [get]
func (h *KnowledgeHandler) SearchKnowledge(c *gin.Context) {
	ctx := c.Request.Context()
	if userID, ok := c.Get(types.UserIDContextKey.String()); ok {
		ctx = context.WithValue(ctx, types.UserIDContextKey, userID)
	}
	// Accept both ?keyword= (legacy / upstream name) and ?query= (what most
	// MCP / agent integrations send). Empty input is only valid for an explicit
	// recent-file browse request; ordinary callers still get a clear 400 instead
	// of silently receiving the same newest cards for every missing query.
	keyword := c.Query("keyword")
	if keyword == "" {
		keyword = c.Query("query")
	}
	recent, _ := strconv.ParseBool(c.DefaultQuery("recent", "false"))
	if strings.TrimSpace(keyword) == "" && !recent {
		c.Error(errors.NewBadRequestError("missing search keyword: pass ?keyword=... or ?query=..."))
		return
	}
	keyword = strings.TrimSpace(keyword)
	offset, limit, ok := parseOffsetPagination(c)
	if !ok {
		return
	}

	var fileTypes []string
	if fileTypesStr := c.Query("file_types"); fileTypesStr != "" {
		for _, ft := range strings.Split(fileTypesStr, ",") {
			ft = strings.TrimSpace(ft)
			if ft != "" {
				fileTypes = append(fileTypes, ft)
			}
		}
	}

	agentID := c.Query("agent_id")
	if agentID != "" {
		userIDVal, ok := c.Get(types.UserIDContextKey.String())
		if !ok {
			c.Error(errors.NewUnauthorizedError("user ID not found"))
			return
		}
		_ = userIDVal
		currentTenantID := c.GetUint64(types.TenantIDContextKey.String())
		if currentTenantID == 0 {
			c.Error(errors.NewUnauthorizedError("workspace ID not found"))
			return
		}
		callerTenantRole := types.TenantRoleFromContext(ctx)
		requestedSourceTenantID, parseErr := types.ParseAgentSourceTenantID(c.Query(types.AgentSourceTenantIDParam))
		if parseErr != nil {
			c.Error(errors.NewBadRequestError(parseErr.Error()))
			return
		}
		agent, err := h.agentShareService.GetSharedAgentForTenant(ctx, currentTenantID, callerTenantRole, agentID, requestedSourceTenantID)
		if err != nil {
			if goerrors.Is(err, service.ErrAgentShareNotFound) || goerrors.Is(err, service.ErrAgentSharePermission) || goerrors.Is(err, service.ErrAgentNotFoundForShare) {
				c.Error(errors.NewForbiddenError("no permission for this shared agent"))
				return
			}
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to verify shared agent access").WithDetails(err.Error()))
			return
		}
		sourceTenantID := agent.TenantID
		mode := agent.Config.KBSelectionMode
		if mode == "none" {
			c.JSON(http.StatusOK, gin.H{
				"success":  true,
				"data":     []interface{}{},
				"has_more": false,
				"total":    0,
			})
			return
		}
		var scopes []types.KnowledgeSearchScope
		if mode == "selected" && len(agent.Config.KnowledgeBases) > 0 {
			for _, kbID := range agent.Config.KnowledgeBases {
				if kbID != "" {
					scopes = append(scopes, types.KnowledgeSearchScope{TenantID: sourceTenantID, KBID: kbID})
				}
			}
		}
		if len(scopes) == 0 {
			kbs, err := h.kbService.ListKnowledgeBasesByTenantID(ctx, sourceTenantID)
			if err != nil {
				logger.ErrorWithFields(ctx, err, nil)
				c.Error(errors.NewInternalServerError("Failed to list knowledge bases").WithDetails(err.Error()))
				return
			}
			// `all` mode: authoritative server-side capability filter. Mirrors the
			// logic in ListKnowledgeBases so @file search, KB listing, and runtime
			// all agree on what "mode=all" actually means for this agent. The
			// filter is agent-mode aware so quick-answer (RAG-only) skips
			// wiki-only KBs even though it has no `allowed_tools`.
			filter := tools.DeriveKBFilterForAgent(agent.Config.AgentMode, agent.Config.AllowedTools)
			removed := 0
			for _, kb := range kbs {
				if kb == nil || kb.Type != types.KnowledgeBaseTypeDocument {
					continue
				}
				if !filter.IsEmpty() && !tools.KBSatisfiesAgentRequirements(kb.Capabilities(), agent.Config.AgentMode, agent.Config.AllowedTools) {
					removed++
					continue
				}
				scopes = append(scopes, types.KnowledgeSearchScope{TenantID: sourceTenantID, KBID: kb.ID})
			}
			if removed > 0 {
				logger.Infof(ctx,
					"SearchKnowledge(agent=%s, mode=all): capability filter removed %d KBs",
					agentID, removed)
			}
		}
		scopes = filterKnowledgeSearchScopesForAPIKey(ctx, scopes)
		if len(scopes) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success":  true,
				"data":     []interface{}{},
				"has_more": false,
				"total":    0,
			})
			return
		}
		knowledges, hasMore, total, err := h.kgService.SearchKnowledgeForScopes(ctx, scopes, keyword, offset, limit, fileTypes)
		if err != nil {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to search knowledge").WithDetails(err.Error()))
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success":  true,
			"data":     knowledges,
			"has_more": hasMore,
			"total":    total,
		})
		return
	}

	if scopes, restricted := tenantAPIKeySearchScopes(ctx); restricted {
		if len(scopes) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success":  true,
				"data":     []interface{}{},
				"has_more": false,
				"total":    0,
			})
			return
		}
		knowledges, hasMore, total, err := h.kgService.SearchKnowledgeForScopes(ctx, scopes, keyword, offset, limit, fileTypes)
		if err != nil {
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError("Failed to search knowledge").WithDetails(err.Error()))
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success":  true,
			"data":     knowledges,
			"has_more": hasMore,
			"total":    total,
		})
		return
	}

	// Default: own + shared KBs
	knowledges, hasMore, total, err := h.kgService.SearchKnowledge(ctx, keyword, offset, limit, fileTypes)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to search knowledge").WithDetails(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"data":     knowledges,
		"has_more": hasMore,
		"total":    total,
	})
}

// MoveKnowledgeRequest defines the request for moving knowledge items
type MoveKnowledgeRequest struct {
	KnowledgeIDs []string `json:"knowledge_ids" binding:"required,min=1"`
	SourceKBID   string   `json:"source_kb_id"  binding:"required"`
	TargetKBID   string   `json:"target_kb_id"  binding:"required"`
	Mode         string   `json:"mode"          binding:"required,oneof=reuse_vectors reparse"`
}

// MoveKnowledgeResponse defines the response for move knowledge
type MoveKnowledgeResponse struct {
	TaskID         string `json:"task_id"`
	SourceKBID     string `json:"source_kb_id"`
	TargetKBID     string `json:"target_kb_id"`
	KnowledgeCount int    `json:"knowledge_count"`
	Message        string `json:"message"`
}

// MoveKnowledge moves knowledge items from one knowledge base to another (async task).
//
// MoveKnowledge godoc
// @Summary      移动知识到其他知识库
// @Description  将一条或多条知识从源知识库移动到目标知识库（异步），返回任务 ID 用于查询进度
// @Tags         知识
// @Accept       json
// @Produce      json
// @Param        request  body      handler.MoveKnowledgeRequest  true  "{source_kb_id, target_kb_id, knowledge_ids}"
// @Success      200      {object}  handler.MoveKnowledgeResponse  "任务信息"
// @Failure      400      {object}  errors.AppError                "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge/move [post]
func (h *KnowledgeHandler) MoveKnowledge(c *gin.Context) {
	ctx := c.Request.Context()

	var req MoveKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "MoveKnowledge: failed to parse request", err)
		c.Error(errors.NewBadRequestError("Invalid request parameters: " + err.Error()))
		return
	}

	// Validate source != target
	if req.SourceKBID == req.TargetKBID {
		c.Error(errors.NewBadRequestError("Source and target knowledge base cannot be the same"))
		return
	}

	tenantID, exists := c.Get(types.TenantIDContextKey.String())
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	if err := requireTenantAPIKeyKnowledgeBases(ctx, req.SourceKBID, req.TargetKBID); err != nil {
		c.Error(err)
		return
	}

	// Validate source KB
	sourceKB, err := h.kbService.GetKnowledgeBaseByID(ctx, req.SourceKBID)
	if err != nil {
		if goerrors.Is(err, repository.ErrKnowledgeBaseNotFound) {
			c.Error(errors.NewNotFoundError("Source knowledge base not found"))
			return
		}
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	if sourceKB.TenantID != tenantID.(uint64) {
		c.Error(errors.NewForbiddenError("No permission to access source knowledge base"))
		return
	}
	if err := h.requireKBOwnershipOrAdmin(c, req.SourceKBID); err != nil {
		c.Error(err)
		return
	}

	// Validate target KB
	targetKB, err := h.kbService.GetKnowledgeBaseByID(ctx, req.TargetKBID)
	if err != nil {
		if goerrors.Is(err, repository.ErrKnowledgeBaseNotFound) {
			c.Error(errors.NewNotFoundError("Target knowledge base not found"))
			return
		}
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	if targetKB.TenantID != tenantID.(uint64) {
		c.Error(errors.NewForbiddenError("No permission to access target knowledge base"))
		return
	}
	if err := h.requireKBOwnershipOrAdmin(c, req.TargetKBID); err != nil {
		c.Error(err)
		return
	}

	// Validate type match
	if sourceKB.Type != targetKB.Type {
		c.Error(errors.NewBadRequestError("Source and target knowledge bases must be the same type"))
		return
	}

	// Validate embedding model match
	if sourceKB.EmbeddingModelID != targetKB.EmbeddingModelID {
		c.Error(errors.NewBadRequestError("Source and target must use the same embedding model"))
		return
	}

	// reuse_vectors copies index entries directly between KBs, which only works
	// inside the same VectorStore backend. A cross-store reuse_vectors move would
	// route CopyIndices through the SOURCE store and then delete the source
	// indices, corrupting the vector data. Reject it and point the caller at
	// reparse mode, which re-indexes into the target store safely.
	if req.Mode == "reuse_vectors" && !sourceKB.SharesStoreWith(targetKB) {
		c.Error(errors.NewBadRequestError(
			"reuse_vectors move across different vector stores is not supported; " +
				"use reparse mode to move into a different store"))
		return
	}

	// Validate all knowledge IDs belong to source KB and are in completed status
	for _, kID := range req.KnowledgeIDs {
		knowledge, err := h.kgService.GetKnowledgeByID(ctx, kID)
		if err != nil {
			c.Error(errors.NewBadRequestError(fmt.Sprintf("Knowledge item %s not found", kID)))
			return
		}
		if knowledge.KnowledgeBaseID != req.SourceKBID {
			c.Error(errors.NewBadRequestError(fmt.Sprintf("Knowledge item %s does not belong to the source knowledge base", kID)))
			return
		}
		if knowledge.ParseStatus != types.ParseStatusCompleted {
			c.Error(errors.NewBadRequestError(fmt.Sprintf("Knowledge item %s is not in completed status (current: %s)", kID, knowledge.ParseStatus)))
			return
		}
	}

	// Generate task ID
	taskID := utils.GenerateTaskID("kg_move", tenantID.(uint64), req.SourceKBID)

	// Create move payload
	payload := types.KnowledgeMovePayload{
		TenantID:     tenantID.(uint64),
		TaskID:       taskID,
		KnowledgeIDs: req.KnowledgeIDs,
		SourceKBID:   req.SourceKBID,
		TargetKBID:   req.TargetKBID,
		Mode:         req.Mode,
		Initiator:    types.TaskInitiatorFromContext(ctx),
	}
	langfuse.InjectTracing(ctx, &payload)

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Errorf(ctx, "MoveKnowledge: failed to marshal payload: %v", err)
		c.Error(errors.NewInternalServerError("Failed to create task"))
		return
	}

	// Enqueue move task
	task := asynq.NewTask(types.TypeKnowledgeMove, payloadBytes,
		asynq.TaskID(taskID), asynq.Queue(types.QueueMaintenance),
		asynq.MaxRetry(3), asynq.Timeout(2*time.Hour))
	info, err := h.asynqClient.Enqueue(task)
	if err != nil {
		logger.Errorf(ctx, "MoveKnowledge: failed to enqueue task: %v", err)
		c.Error(errors.NewInternalServerError("Failed to enqueue task"))
		return
	}

	logger.Infof(ctx, "MoveKnowledge: task enqueued: %s, asynq_id: %s, source: %s, target: %s, count: %d",
		taskID, info.ID, secutils.SanitizeForLog(req.SourceKBID), secutils.SanitizeForLog(req.TargetKBID), len(req.KnowledgeIDs))

	// Save initial progress
	initialProgress := &types.KnowledgeMoveProgress{
		TaskID:     taskID,
		SourceKBID: req.SourceKBID,
		TargetKBID: req.TargetKBID,
		Status:     types.KBCloneStatusPending,
		Total:      len(req.KnowledgeIDs),
		Progress:   0,
		Message:    "Task queued, waiting to start...",
		CreatedAt:  time.Now().Unix(),
		UpdatedAt:  time.Now().Unix(),
	}
	if err := h.kgService.SaveKnowledgeMoveProgress(ctx, initialProgress); err != nil {
		logger.Warnf(ctx, "MoveKnowledge: failed to save initial progress: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": MoveKnowledgeResponse{
			TaskID:         taskID,
			SourceKBID:     req.SourceKBID,
			TargetKBID:     req.TargetKBID,
			KnowledgeCount: len(req.KnowledgeIDs),
			Message:        "Knowledge move task started",
		},
	})
}

// GetKnowledgeMoveProgress retrieves the progress of a knowledge move task.
//
// GetKnowledgeMoveProgress godoc
// @Summary      获取知识移动进度
// @Description  按任务 ID 查询移动进度
// @Tags         知识
// @Produce      json
// @Param        task_id  path      string                       true  "移动任务 ID"
// @Success      200      {object}  types.KnowledgeMoveProgress  "进度信息"
// @Failure      404      {object}  errors.AppError              "任务不存在"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge/move/progress/{task_id} [get]
func (h *KnowledgeHandler) GetKnowledgeMoveProgress(c *gin.Context) {
	ctx := c.Request.Context()

	taskID := c.Param("task_id")
	if taskID == "" {
		c.Error(errors.NewBadRequestError("Task ID cannot be empty"))
		return
	}
	if err := requireTaskProgressTenant(ctx, taskID); err != nil {
		c.Error(err)
		return
	}

	progress, err := h.kgService.GetKnowledgeMoveProgress(ctx, taskID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    progress,
	})
}

// resolveAgentAllowedKBIDs returns the set of knowledge base IDs that the
// shared agent is allowed to access based on its KBSelectionMode config.
// Returns nil when no restriction applies ("all" mode), or a concrete slice
// (possibly empty for "none" mode) when the results must be filtered.
func resolveAgentAllowedKBIDs(agent *types.CustomAgent) []string {
	switch agent.Config.KBSelectionMode {
	case "all":
		return nil
	case "none":
		return []string{}
	case "selected":
		return agent.Config.KnowledgeBases
	default:
		if len(agent.Config.KnowledgeBases) > 0 {
			return agent.Config.KnowledgeBases
		}
		return nil
	}
}

// parseCommaSeparatedTagIDs splits a comma-separated string of tag IDs and
// filters out empty strings and the "__untagged__" sentinel value.
func parseCommaSeparatedTagIDs(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "__untagged__" {
			continue
		}
		result = append(result, p)
	}
	return result
}

// parseFilterTime parses a query-string timestamp accepted by knowledge list
// filters. It supports RFC3339, RFC3339 with milliseconds, and the date-only
// "2006-01-02" form (interpreted at start of day in the local timezone).
func parseFilterTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"}
	var lastErr error
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}

func sliceContains(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}

type batchReparseKnowledgeRequest struct {
	KBID          string                           `json:"kb_id" binding:"required"`
	IDs           []string                         `json:"ids" binding:"required"`
	ProcessConfig *types.KnowledgeProcessOverrides `json:"process_config,omitempty"`
}

// BatchReparseKnowledge godoc
// @Summary      批量重新解析知识
// @Description  按 ID 列表批量重新解析单个知识库下的多个知识条目
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        request  body      batchReparseKnowledgeRequest  true  "批量重解析请求"
// @Success      200      {object}  map[string]interface{}        "任务已提交"
// @Failure      400      {object}  errors.AppError               "请求参数错误"
// @Failure      403      {object}  errors.AppError               "权限不足"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge/batch-reparse [post]
func (h *KnowledgeHandler) BatchReparseKnowledge(c *gin.Context) {
	ctx := c.Request.Context()
	var req batchReparseKnowledgeRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Errorf(ctx, "failed to parse batch reparse knowledge request: %v", err)
		c.Error(errors.NewBadRequestError("invalid batch reparse knowledge request parameters"))
		return
	}

	seen := make(map[string]struct{}, len(req.IDs))
	ids := make([]string, 0, len(req.IDs))
	for _, raw := range req.IDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		c.Error(errors.NewBadRequestError("no knowledge IDs provided for batch reparse"))
		return
	}
	const maxBatch = 200
	if len(ids) > maxBatch {
		c.Error(errors.NewBadRequestError(fmt.Sprintf("too many ids (max %d per batch)", maxBatch)))
		return
	}

	_, kbID, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, req.KBID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("no permission to reparse knowledge in this kb"))
		return
	}
	ctx = context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	knowledgeList, err := h.kgService.GetKnowledgeBatch(ctx, effectiveTenantID, ids)
	if err != nil {
		logger.Errorf(ctx, "failed to get knowledge batch, kb_id: %s, size: %d, err: %v", kbID, len(ids), err)
		c.Error(errors.NewInternalServerError("failed to get knowledge batch"))
		return
	}
	if len(knowledgeList) != len(ids) {
		c.Error(errors.NewBadRequestError("some knowledge entries were not found"))
		return
	}
	for _, k := range knowledgeList {
		if k.KnowledgeBaseID != kbID {
			c.Error(errors.NewBadRequestError(
				fmt.Sprintf("Knowledge %s does not belong to knowledge base %s",
					secutils.SanitizeForLog(k.ID), secutils.SanitizeForLog(kbID))))
			return
		}
	}

	taskID, err := h.enqueueKnowledgeListReparse(ctx, effectiveTenantID, ids, req.ProcessConfig)
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue batch knowledge reparse task: %v", err)
		c.Error(errors.NewInternalServerError("Failed to enqueue batch reparse task"))
		return
	}

	logger.Infof(ctx, "Batch knowledge reparse task enqueued: %s, kb_id: %s, count: %d",
		taskID, secutils.SanitizeForLog(kbID), len(ids))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Batch reparse task submitted",
		"data": gin.H{
			"task_id":       taskID,
			"reparse_count": len(ids),
		},
	})
}

func requireTenantAPIKeyKnowledgeBase(ctx context.Context, kbID string) error {
	return requireTenantAPIKeyKnowledgeBases(ctx, kbID)
}

func requireTenantAPIKeyKnowledgeBases(ctx context.Context, kbIDs ...string) error {
	return types.AuthorizeTenantAPIKeyKnowledgeBases(ctx, kbIDs...)
}

func tenantAPIKeyAllowedKBSet(ctx context.Context) map[string]bool {
	scope, ok := types.TenantAPIKeyScopeFromContext(ctx)
	if !ok || !scope.IsKnowledgeBaseRestricted() {
		return nil
	}
	allowed := make(map[string]bool, len(scope.KnowledgeBaseIDs))
	for _, id := range scope.KnowledgeBaseIDs {
		allowed[id] = true
	}
	return allowed
}

func intersectKBAllowSet(base, restrict map[string]bool) map[string]bool {
	if restrict == nil {
		return base
	}
	if base == nil {
		return restrict
	}
	out := make(map[string]bool)
	for id := range base {
		if restrict[id] {
			out[id] = true
		}
	}
	return out
}

func filterKnowledgesByKBAllowSet(knowledges []*types.Knowledge, allowed map[string]bool) []*types.Knowledge {
	if allowed == nil {
		return knowledges
	}
	filtered := make([]*types.Knowledge, 0, len(knowledges))
	for _, k := range knowledges {
		if k != nil && allowed[k.KnowledgeBaseID] {
			filtered = append(filtered, k)
		}
	}
	return filtered
}

func filterKnowledgeSearchScopesForAPIKey(ctx context.Context, scopes []types.KnowledgeSearchScope) []types.KnowledgeSearchScope {
	allowed := tenantAPIKeyAllowedKBSet(ctx)
	if allowed == nil {
		return scopes
	}
	filtered := make([]types.KnowledgeSearchScope, 0, len(scopes))
	for _, scope := range scopes {
		if allowed[scope.KBID] {
			filtered = append(filtered, scope)
		}
	}
	return filtered
}

func tenantAPIKeySearchScopes(ctx context.Context) ([]types.KnowledgeSearchScope, bool) {
	scope, ok := types.TenantAPIKeyScopeFromContext(ctx)
	if !ok || !scope.IsKnowledgeBaseRestricted() {
		return nil, false
	}
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		return nil, true
	}
	scopes := make([]types.KnowledgeSearchScope, 0, len(scope.KnowledgeBaseIDs))
	for _, kbID := range scope.KnowledgeBaseIDs {
		scopes = append(scopes, types.KnowledgeSearchScope{TenantID: tenantID, KBID: kbID})
	}
	return scopes, true
}
