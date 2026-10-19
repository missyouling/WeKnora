package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/filetransport"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/tracing/langfuse"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/hibiken/asynq"
)

// BusinessExtractHandler 承载 WeKnora 二次开发中「日常事务」模块的业务提取与记录管理。
type BusinessExtractHandler struct {
	*KnowledgeHandler
	db              *gorm.DB
	modelService    interfaces.ModelService
	businessSvc     *service.BusinessExtractService
	storageResolver interfaces.StorageBackendResolver
	resourceCatalog interfaces.ResourceCatalog
	// asynqClient 异步任务投递器：C 方案（extract-business 异步化）下 HTTP handler
	// 投递任务即返 200，worker 通过 ProcessBusinessExtract 执行原同步提取核心，
	// 避免 6 分钟级模型调用撞网关读超时（524/500）。
	asynqClient interfaces.TaskEnqueuer
	// documentReader / tenantService 供「智能字段生成」样票解析使用
	//（service.ParseFileForExtractSuggest 需要解析器与租户引擎配置）。
	// 二者都是可选依赖：未装配时接口 A 仍可用 text 直传路径。
	documentReader interfaces.DocumentReader
	tenantService  interfaces.TenantService
	// extractMu 按 knowledgeID 防重入：同一文档并发提取会重复烧 token 且后写覆盖。
	// 单实例内存锁（多实例部署时需换分布式锁）。
	extractMu sync.Map // map[string]struct{}
}

// NewBusinessExtractHandler 构造业务提取 Handler。
func NewBusinessExtractHandler(kh *KnowledgeHandler, db *gorm.DB, ms interfaces.ModelService, bsvc *service.BusinessExtractService,
	storageResolver interfaces.StorageBackendResolver, resourceCatalog interfaces.ResourceCatalog,
	asynqClient interfaces.TaskEnqueuer,
	documentReader interfaces.DocumentReader, tenantService interfaces.TenantService,
) *BusinessExtractHandler {
	return &BusinessExtractHandler{
		KnowledgeHandler: kh, db: db, modelService: ms, businessSvc: bsvc,
		storageResolver: storageResolver, resourceCatalog: resourceCatalog,
		asynqClient:    asynqClient,
		documentReader: documentReader, tenantService: tenantService,
	}
}

// extractionTimeout 提取任务总超时：脱离 HTTP 请求生命周期后仍需硬上限，
// 避免无人值守的提取任务永久运行烧 token。
const extractionTimeout = 30 * time.Minute

// invoiceBatchBudget 单批发票提取的时间预算（Guided 主调用 + 至多一次回退）。
//
// 为什么需要它：Guided 调用可能吃满一次长超时（实测 5 分钟，供应商对某些批次
// 直接不返回），而回退路径若使用默认 4 次尝试序列，单批最坏耗时 = 5 + 4×5 = 25 分钟；
// 16 页文档切 5 批串行后总时长可达 1~2 小时，用户侧表现为"进度 100% 却迟迟不出结果"。
// 有了每批预算，最坏单批 8 分钟；某批失败只丢失该批，其余批次照常提取并落库。
const invoiceBatchBudget = 8 * time.Minute

// invoiceBatchConcurrency 分批提取的最大并发批次数。
//
// 批次串行时总时长 = 批次数 × 单批耗时（实测 2~3 分钟/批），16 页文档 4 批约 12 分钟，
// 是"慢"的主因。取 2：总时长约减半，同时避免高并发触发供应商限流（429）。
// 并发下**必须**用预分配切片按批次索引写结果，禁止无序 append（会导致页码错位）。
const invoiceBatchConcurrency = 4

// newExtractionContext 返回脱离 HTTP 请求生命周期的提取上下文。
// 场景：前端超时/切页面/网络抖动导致 gin request ctx 被取消时，耗时的大模型
// 调用不会被连带掐断（context canceled），提取继续在后台跑完并落库，
// 前端已有轮询会刷出最终状态。30 分钟硬上限防止任务永久运行。
func newExtractionContext(effectiveTenantID uint64) (context.Context, context.CancelFunc) {
	base := context.WithValue(context.Background(), types.TenantIDContextKey, effectiveTenantID)
	return context.WithTimeout(base, extractionTimeout)
}

// appendUniqueStrings 追加字符串并去重（保持首次出现顺序）。
// 用于合并多批结果的正则补洞字段列表——同一字段可能被多批各补一次。
func appendUniqueStrings(dst []string, add ...string) []string {
	for _, v := range add {
		if v == "" {
			continue
		}
		found := false
		for _, existing := range dst {
			if existing == v {
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, v)
		}
	}
	return dst
}

// mergeFailedStateMeta constructs the custom_metadata written when an extraction
// fails at the model call (e.g. 429 rate limit) or on a recovered panic. It
// preserves the previously extracted records (contracts/regulations/invoices/
// award_punish) and file-level tags instead of overwriting them with a null
// array, so a transient model/panic failure neither wipes existing data nor
// drops the record from the list; only the status/error fields are updated.
func mergeFailedStateMeta(prev []byte, kind, extractErr string) types.JSON {
	merged := map[string]interface{}{}
	if len(prev) > 0 {
		if uerr := json.Unmarshal(prev, &merged); uerr != nil {
			merged = map[string]interface{}{}
		}
	}
	merged["kind"] = kind
	merged["extract_status"] = "failed"
	merged["extract_error"] = extractErr
	raw, err := json.Marshal(merged)
	if err != nil {
		return types.JSON(prev)
	}
	return types.JSON(raw)
}

// parseSandboxPagination reads page/page_size from query with safe defaults.
func parseSandboxPagination(c *gin.Context) (page, pageSize int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	pageSize, _ = strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	return page, pageSize
}

// extractInProgressReason 是「同一文档已有提取任务在途」的机器可读标识。
// 409 是同一 HTTP 状态但语义分两类：本值代表并发重复提交（数据仍会由持锁
// 请求完成），另一类是业务性冲突（单号重复）。前端据此把前者判为「已在提取
// 中」并转轮询跟随，而不是当成提取失败。
const extractInProgressReason = "extract_in_progress"

// extractInProgressDetails 构造 409 响应体 details：附带 knowledge_id/scope
// 便于前端与日志定位是哪份文档的哪条通道被拒。
func extractInProgressDetails(knowledgeID, scope string) gin.H {
	return gin.H{"reason": extractInProgressReason, "knowledge_id": knowledgeID, "scope": scope}
}

// businessExtractionInProgress 判断业务提取（发票/合同/制度/奖惩）是否因文档
// 仍处于解析流水线中而必须拦截。仅拦截 pending/processing/finalizing/deleting；
// completed 直接放行；failed/cancelled 也放行——docreader 短暂不可用会留下 failed，
// 但文本 chunk 可能已完整落库，而业务重提只依赖 chunk 文本（行规则/模型），不应
// 被失败状态挡死；无文本时由下游批次检查（invoiceBatchesHaveText 等）给出明确提示。
func businessExtractionInProgress(status string) bool {
	switch status {
	case types.ParseStatusPending, types.ParseStatusProcessing,
		types.ParseStatusFinalizing, types.ParseStatusDeleting:
		return true
	default:
		return false
	}
}

// noExtractableTextNotice 在业务重提找不到可提取文本时，区分两种成因给出准确提示，
// 避免把"解析失败/被中断导致文本 chunk 缺失"误报成"扫描件无文本层"：
//   - parse_status=failed：docreader 解析失败或被中断（如 gRPC 50051 不可用），
//     旧 chunk 可能已被软删而新 chunk 未生成，应引导用户先重新解析；
//   - 其它（completed 等）：解析成功却无文本，才是真正的扫描件/纯图片。
//
// 返回值依次为写入 custom_metadata 的中文 ExtractError 与返回前端的英文 message。
func noExtractableTextNotice(parseStatus string) (string, string) {
	if parseStatus == types.ParseStatusFailed {
		return "文档解析失败或被中断，暂无可提取文本，请先重新解析后再提取",
			"document parsing previously failed or was interrupted; please re-parse the document before extracting"
	}
	return "文档无可提取文本（疑似扫描件），已保留文件待人工处理",
		"document has no extractable text (possible scanned document), file kept"
}

// autoDeleteCount reads the auto_deleted_count marker from a knowledge row's
// custom_metadata (0 when absent).
func (h *BusinessExtractHandler) autoDeleteCount(ctx context.Context, knowledge *types.Knowledge) int {
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

func (h *BusinessExtractHandler) ExtractContract(c *gin.Context, certType string) {
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
		c.Error(errors.NewForbiddenError("No permission to extract contract fields"))
		return
	}
	// 快速失败提示（真正防重入由 worker 端 extractMu 保证）：同一文档已有任务在跑时
	// 直接 409，避免连点重复投递。
	if _, loaded := h.extractMu.Load(knowledgeID); loaded {
		c.Error(errors.NewConflictError("extraction already in progress for this document").
			WithDetails(extractInProgressDetails(knowledgeID, "contract")))
		return
	}
	taskID, err := h.enqueueBusinessExtract(ctx, effectiveTenantID, kbID, knowledgeID, "contract", certType)
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue contract extraction for %s: %v", knowledgeID, err)
		c.Error(errors.NewInternalServerError("enqueue contract extraction failed: " + err.Error()))
		return
	}
	logger.Infof(ctx, "Contract extraction enqueued: knowledge=%s task=%s", knowledgeID, taskID)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Contract extraction submitted",
		"data": map[string]interface{}{
			"knowledge_id":   knowledgeID,
			"kind":           "contract",
			"extract_status": "processing",
			"task_id":        taskID,
		},
	})
}

// runContractCore 是不依赖 gin 的合同提取核心执行器（C 方案：HTTP handler 投递即返，
// asynq worker 调用本方法执行原同步提取逻辑）。响应改为返回值/error，由调用方记录；
// 失败状态照旧经 persistCtx 落库，模型调用失败不再因 HTTP 超时被连带掐断。
func (h *BusinessExtractHandler) runContractCore(ctx context.Context, kbID, knowledgeID, certType string, effectiveTenantID uint64) (gin.H, error) {
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	// persistCtx 脱离任务/请求生命周期：任务超时/取消后仍可落库提取结果或
	// failed 状态（否则 ctx 被取消，failed 状态写不进去，记录永久停在"提取中"）。
	persistCtx := context.WithValue(context.Background(), types.TenantIDContextKey, effectiveTenantID)
	// extractCtx 30 分钟硬上限：耗时的大模型调用不被任务取消连带掐断。
	extractCtx, extractCancel := newExtractionContext(effectiveTenantID)
	defer extractCancel()

	var knowledge *types.Knowledge
	// panic 兜底：提取协程任何 panic 都强制落 failed 状态（用独立 ctx），
	// 保证状态闭环，不允许永久转圈。
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf(ctx, "Contract extraction panic recovered: %v\n%s", r, debug.Stack())
			var panicPrev []byte
			if knowledge != nil {
				panicPrev = knowledge.CustomMetadata
			}
			failMeta := mergeFailedStateMeta(panicPrev, "contract", fmt.Sprintf("contract extraction panic: %v", r))
			if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, failMeta); serr != nil {
				logger.Warnf(ctx, "Failed to persist panic failed-state: %v", serr)
			}
		}
	}()

	var err error
	knowledge, err = h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for contract extraction", err)
		return nil, errors.NewNotFoundError("Knowledge not found")
	}
	if knowledge.KnowledgeBaseID != kbID {
		return nil, errors.NewBadRequestError("Knowledge does not belong to the given knowledge base")
	}
	if businessExtractionInProgress(knowledge.ParseStatus) {
		return nil, errors.NewBadRequestError("document has not been parsed yet, please wait for parsing to finish")
	}
	// 待补录豁免：manual 记录由用户人工维护，不自动重提取、不参与任何自动删除判定。
	var curContractMeta contractCustomMetadata
	_ = json.Unmarshal(knowledge.CustomMetadata, &curContractMeta)
	if curContractMeta.ExtractStatus == "manual" {
		logger.Infof(ctx, "Contract extraction skipped: knowledge %s is manual intake", knowledgeID)
		return gin.H{
			"success": true,
			"message": "待补录记录，无需重复提取",
			"data":    map[string]interface{}{"kind": "contract", "extract_status": "manual", "removed": false},
		}, nil
	}

	// worker 上下文没有 gin 侧的 kb 绑定：重新按 kbID 取知识库配置（仅需 summary_model_id）。
	kb, kberr := h.kbService.GetKnowledgeBaseByIDOnly(effCtx, kbID)
	if kberr != nil || kb == nil {
		return nil, errors.NewNotFoundError("Knowledge base not found")
	}
	modelID := strings.TrimSpace(kb.SummaryModelID)
	if modelID == "" {
		return nil, errors.NewBadRequestError("no extraction model configured for the knowledge base")
	}
	chatModel, err := h.modelService.GetChatModel(extractCtx, modelID)
	if err != nil {
		logger.Error(ctx, "Failed to load extraction model", err)
		return nil, errors.NewInternalServerError("get extraction model failed: " + err.Error())
	}

	chunks, err := h.chunkService.ListChunksByKnowledgeID(extractCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to list chunks for contract extraction", err)
		return nil, errors.NewInternalServerError("list chunks failed: " + err.Error())
	}
	// 分批提取：超长多合同文档单次 LLM 调用不可靠，按 chunk 分批独立调用后合并，
	// Normalize 时按文件内合同编号去重。
	batches := service.BuildContractExtractionBatches(knowledge.FileName, knowledge.Description, chunks, 0)
	// 扫描件防御：无文本层（扫描件 PDF / 纯图片）时不要自动删除，保留文件并标记
	// failed 让用户人工处理（配合删除历史，避免"监测检测合同"这类扫描件被误删）。
	if !contractBatchesHaveText(batches) {
		zhNotice, enNotice := noExtractableTextNotice(knowledge.ParseStatus)
		noTextMeta := mergeFailedStateMeta(knowledge.CustomMetadata, "contract", zhNotice)
		if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, noTextMeta); serr != nil {
			logger.Warnf(ctx, "Failed to persist contract no-text state: %v", serr)
		}
		logger.Warnf(ctx, "Contract extraction skipped: no text layer for knowledge %s (parse_status=%s), file kept", knowledgeID, knowledge.ParseStatus)
		return gin.H{
			"success": true,
			"message": enNotice,
			"data":    map[string]interface{}{"kind": "contract", "extract_status": "failed", "removed": false},
		}, nil
	}
	merged := &service.ContractExtractionResult{Kind: "contract"}
	var extractErr error
	sawContract := false
	// 提取进行中标记：先落 processing 中间态，让前端轮询/状态条与表格状态列实时显示
	//「正在提取」（避免点重新提取后状态列短暂停留在旧值/failed）。合并保留既有字段
	//（fleet_cert_type、旧 contracts 等），提取成功/失败路径会用终态整体覆盖。
	procMeta := map[string]interface{}{"kind": "contract", "extract_status": "processing"}
	if len(knowledge.CustomMetadata) > 0 {
		var prev map[string]interface{}
		if jerr := json.Unmarshal(knowledge.CustomMetadata, &prev); jerr == nil {
			for k, v := range prev {
				if _, exists := procMeta[k]; !exists {
					procMeta[k] = v
				}
			}
		}
	}
	delete(procMeta, "extract_error")
	if pmeta, jerr := json.Marshal(procMeta); jerr == nil {
		if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, types.JSON(pmeta)); serr != nil {
			logger.Warnf(ctx, "Failed to persist contract processing-state: %v", serr)
		}
	}
	// 规则解析：优先请求携带的归档类型（上传弹窗 body.cert_type），未携带时回退文件级
	// 打标 fleet_cert_type，再按 resolveContractRuleCfg 回退整体规则。
	ruleCertType := certType
	if ruleCertType == "" && len(knowledge.CustomMetadata) > 0 {
		var mdMeta struct {
			FleetCertType string `json:"fleet_cert_type"`
		}
		if jerr := json.Unmarshal(knowledge.CustomMetadata, &mdMeta); jerr == nil && mdMeta.FleetCertType != "" {
			ruleCertType = mdMeta.FleetCertType
		}
	}
	// 用户配置的提取规则（scope=contract）：按归档分类匹配，未配置时回退整体规则
	contractRuleCfg := h.resolveContractRuleCfg(extractCtx, kbID, ruleCertType)
	for i, batch := range batches {
		if strings.TrimSpace(batch) == "" {
			continue
		}
		batchRes, berr := service.ExtractContractsFromContentWithRules(extractCtx, chatModel, batch, contractRuleCfg)
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
		failMeta := mergeFailedStateMeta(knowledge.CustomMetadata, "contract", "contract extraction failed: "+extractErr.Error())
		if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, failMeta); serr != nil {
			logger.Warnf(ctx, "Failed to persist contract extraction failed-state: %v", serr)
		}
		logger.Error(ctx, "Contract extraction model call failed", extractErr)
		return nil, errors.NewInternalServerError("contract extraction failed: " + extractErr.Error())
	}
	extracted := merged
	// 自动编号去重：无合同编号的合同按当天已有最大序号 +1 顺序递增（跨文件唯一）
	autoSeq, seqErr := h.businessSvc.MaxAutoContractSeq(extractCtx, kbID)
	if seqErr != nil {
		logger.Warnf(ctx, "Failed to compute auto contract seq for %s: %v", kbID, seqErr)
	}
	service.NormalizeContractExtractionResult(extracted, autoSeq)
	// 通用质量闭环（非发票业务口径）：标准日期字段归一化为 YYYY-MM-DD，
	// 用户配置为必填（Enabled && Required）的字段先用该字段的 Regex 补洞，
	// 补不到才记 error。发票专属规则（号码/代码二选一、金额交叉校验）不在此列。
	contractIssues, contractPatches := service.ValidateGenericExtraction(
		service.NewGenericRecords(extracted.Contracts), strings.Join(batches, "\n"), contractRuleCfg)
	if len(contractIssues) > 0 || len(contractPatches) > 0 {
		logger.Infof(ctx, "Contract extraction quality: issues=%d patched=%v knowledge=%s",
			len(contractIssues), service.PatchedFieldNames(contractPatches), knowledgeID)
	}

	meta := contractCustomMetadata{
		Kind:           extracted.Kind,
		Contracts:      extracted.Contracts,
		ExtractStatus:  "success",
		ExtractError:   extracted.ExtractError,
		ExtractIssues:  contractIssues,
		PatchedByRegex: service.PatchedFieldNames(contractPatches),
		RegexPatches:   contractPatches,
	}
	// 自定义识别规则：合同类型归类（用户可配置关键词/正则 → 合同类型），
	// 按模型提取出的合同类型字段匹配，命中则覆盖模型结果。
	if recCfg, rerr := h.kgService.GetRecognitionConfig(extractCtx, kbID); rerr == nil {
		for i := range meta.Contracts {
			if t := service.ClassifyTypeFromRules(meta.Contracts[i].ContractType, recCfg); t != "" {
				meta.Contracts[i].ContractType = t
			}
		}
	}
	// 用户归档分类绝对优先（对齐发票）：ruleCertType（上传选择/文件级 fleet_cert_type
	// 打标）非空时，同文件全部合同（含多页）的 ContractType 强制为该分类，覆盖模型与
	// 自定义识别规则；ruleCertType 为空时校验类型落在已启用合同分类集合，否则降级"其它合同"。
	h.applyContractUserCategoryPrecedence(extractCtx, &meta, ruleCertType)
	if extracted.Kind == "not_contract" {
		// 自定义识别规则捞回：模型判非但包含规则命中 → 认定为合同，置 manual 待补录，
		// 不自动删除（解决"该是合同却没入库"的误判）。
		if recCfg, rerr := h.kgService.GetRecognitionConfig(extractCtx, kbID); rerr == nil {
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
				return gin.H{
					"success": true,
					"message": "识别规则命中，已认定为合同，请在列表中编辑补录字段",
					"data":    map[string]interface{}{"kind": "contract", "extract_status": "manual", "removed": false},
				}, nil
			}
		}
		meta.ExtractStatus = "not_contract"
		if meta.ExtractError == "" {
			meta.ExtractError = "document does not look like a contract"
		}
		// 防循环：auto_deleted_count >= 1 说明该文件曾被自动删除后由用户从删除历史
		// 恢复，再次判定非合同不再自动删除，改为 failed 保留（避免 删除→恢复→删除 死循环）。
		if h.autoDeleteCount(extractCtx, knowledge) > 0 {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "系统判定非合同文件，但该文件已被人工恢复，已保留待人工处理"
			logger.Warnf(ctx, "Contract re-extraction judged non-contract but row was restored before; keeping file %s", knowledgeID)
		} else {
			// 非合同文件不能存在于合同知识库 → 提取判定后自动删除该文件（保留物理文件，
			// 写入删除历史，可在"删除历史"抽屉中查看/恢复/永久删除）
			if delErr := h.kgService.AutoDeleteKnowledge(extractCtx, knowledgeID, "not_contract"); delErr != nil {
				logger.Warnf(ctx, "auto-delete non-contract knowledge failed: %v", delErr)
			} else {
				logger.Infof(ctx, "auto-deleted non-contract knowledge, ID: %s", knowledgeID)
				return gin.H{
					"success": true,
					"message": "非合同文件已移至删除历史，可在删除历史中恢复",
					"data":    map[string]interface{}{"removed": true, "kind": "not_contract"},
				}, nil
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
		return nil, errors.NewInternalServerError("failed to encode extraction result")
	}
	// 合并而非覆盖：保留 custom_metadata 中提取流程不管理的字段（如
	// auto_deleted_count），避免整 JSON 覆盖丢数据（与发票最终保存口径一致）。
	mergedJSON := metaJSON
	if len(knowledge.CustomMetadata) > 0 {
		var existing map[string]interface{}
		var fresh map[string]interface{}
		if jerr1 := json.Unmarshal(knowledge.CustomMetadata, &existing); jerr1 == nil {
			if jerr2 := json.Unmarshal(metaJSON, &fresh); jerr2 == nil {
				for k, v := range fresh {
					existing[k] = v
				}
				if mj, merr := json.Marshal(existing); merr == nil {
					mergedJSON = mj
				}
			}
		}
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, types.JSON(mergedJSON)); err != nil {
		logger.Error(ctx, "Failed to persist contract extraction result", err)
		return nil, errors.NewInternalServerError("failed to save extraction result: " + err.Error())
	}

	logger.Infof(ctx, "Contract extraction succeeded, knowledge ID: %s, kind: %s, contracts: %d",
		knowledgeID, meta.Kind, len(meta.Contracts))
	if meta.ExtractStatus == "failed" {
		logger.Warnf(ctx, "Contract extraction returned an unusable result, knowledge ID: %s, err: %s", knowledgeID, meta.ExtractError)
		return nil, errors.NewInternalServerError(meta.ExtractError)
	}
	return gin.H{
		"success": true,
		"message": "Contract extraction succeeded",
		"data": map[string]interface{}{
			"knowledge_id":   knowledgeID,
			"kind":           meta.Kind,
			"extract_status": meta.ExtractStatus,
			"contracts":      meta.Contracts,
		},
	}, nil
}

// regulationCustomMetadata is the persisted shape written into a knowledge
// entry's custom_metadata by ExtractRegulation. The frontend reads exactly
// these keys to render the regulation management list.
type regulationCustomMetadata struct {
	Kind          string                           `json:"kind"`
	Regulations   []types.RegulationExtractionItem `json:"regulations"`
	ExtractStatus string                           `json:"extract_status"`
	ExtractError  string                           `json:"extract_error"`
	FleetCertType string                           `json:"fleet_cert_type,omitempty"` // 归档分类打标（上传弹窗选择），供重提取选规则/绝对优先
	// 通用质量闭环元数据（与发票同口径）：字段级问题、正则补洞溯源。
	ExtractIssues  []service.ExtractFieldIssue `json:"extract_issues,omitempty"`
	PatchedByRegex []string                    `json:"patched_by_regex,omitempty"`
	RegexPatches   []service.RegexFieldPatch   `json:"regex_patches,omitempty"`
}

// awardPunishCustomMetadata is the persisted shape written into a knowledge
// entry's custom_metadata by ExtractAwardPunish. The frontend reads exactly
// these keys to render the notice/announcement management list.
type awardPunishCustomMetadata struct {
	Kind          string                            `json:"kind"`
	Records       []types.AwardPunishExtractionItem `json:"records"`
	ExtractStatus string                            `json:"extract_status"`
	ExtractError  string                            `json:"extract_error"`
	FleetCertType string                            `json:"fleet_cert_type,omitempty"` // 归档分类打标（上传弹窗选择），供重提取选规则/绝对优先
	// 通用质量闭环元数据（与发票同口径）：字段级问题、正则补洞溯源。
	ExtractIssues  []service.ExtractFieldIssue `json:"extract_issues,omitempty"`
	PatchedByRegex []string                    `json:"patched_by_regex,omitempty"`
	RegexPatches   []service.RegexFieldPatch   `json:"regex_patches,omitempty"`
}

// awardPunishBatchesHaveText reports whether any extraction batch carries usable
// text. A scanned document (no text layer) yields only empty batches; without
// this guard the extract endpoint would judge it not-award/punish and
// auto-delete a legitimate scanned notice.
func awardPunishBatchesHaveText(batches []string) bool {
	for _, b := range batches {
		if strings.TrimSpace(b) != "" {
			return true
		}
	}
	return false
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

// markInvoiceDuplicates scans the other documents of the same KB and flags
// invoices whose number already appears elsewhere. In-file duplicates are
// already collapsed by the model prompt; this pass only handles cross-file
// repetition.

// contractCustomMetadata mirrors the per-knowledge custom_metadata shape for contracts.
type contractCustomMetadata struct {
	Kind          string                           `json:"kind"`
	Contracts     []service.ContractExtractionItem `json:"contracts"`
	ExtractStatus string                           `json:"extract_status"`
	ExtractError  string                           `json:"extract_error"`
	FleetCertType string                           `json:"fleet_cert_type,omitempty"` // 归档分类打标（上传弹窗选择），供重提取选规则/绝对优先
	// 通用质量闭环元数据（与发票同口径）：字段级问题、正则补洞溯源。
	ExtractIssues  []service.ExtractFieldIssue `json:"extract_issues,omitempty"`
	PatchedByRegex []string                    `json:"patched_by_regex,omitempty"`
	RegexPatches   []service.RegexFieldPatch   `json:"regex_patches,omitempty"`
}

// contractBatchesHaveText reports whether any extraction batch carries usable text.
func contractBatchesHaveText(batches []string) bool {
	for _, b := range batches {
		if strings.TrimSpace(b) != "" {
			return true
		}
	}
	return false
}

// enabledInvoiceCategoryNames 返回 invoice scope 当前已启用的分类名列表
// （tenant_id=0 全局配置，enabled=true，排除软删，按 sort_order 升序），用于两处：
//  1. 提取引导（guide）注入类型枚举约束——已停用分类不得出现在发给 LLM 的 Schema 选项中；
//  2. 写库前的「停用分类隔离」——OCR/规则结果命中已停用分类（如普通/专用被禁用后
//     的存量值）或当前分类库不存在的类型时，一律降级「其它票据」。
func (h *BusinessExtractHandler) enabledInvoiceCategoryNames(ctx context.Context) []string {
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("tenant_id = 0 AND scope = ? AND enabled = TRUE AND deleted_at IS NULL", types.FleetCategoryScopeInvoice).
		Order("sort_order ASC, created_at ASC").Find(&cats).Error; err != nil {
		logger.Warnf(ctx, "load enabled invoice categories failed: %v", err)
		return nil
	}
	names := make([]string, 0, len(cats))
	for _, c := range cats {
		if n := strings.TrimSpace(c.Name); n != "" {
			names = append(names, n)
		}
	}
	return names
}

// enabledContractCategoryNames 返回当前已启用的合同分类名（scope=contract，
// tenant_id=0 内置分类），按 sort_order 升序。与发票版口径一致。
func (h *BusinessExtractHandler) enabledContractCategoryNames(ctx context.Context) []string {
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("tenant_id = 0 AND scope = ? AND enabled = TRUE AND deleted_at IS NULL", "contract").
		Order("sort_order ASC, created_at ASC").Find(&cats).Error; err != nil {
		logger.Warnf(ctx, "load enabled contract categories failed: %v", err)
		return nil
	}
	names := make([]string, 0, len(cats))
	for _, c := range cats {
		if n := strings.TrimSpace(c.Name); n != "" {
			names = append(names, n)
		}
	}
	return names
}

// applyContractUserCategoryPrecedence 在合同写库前执行「用户指定分类绝对优先 +
// 停用分类隔离」，与发票 applyInvoiceUserCategoryPrecedence 口径对齐：
//   - certType 非空（上传弹窗选择/文件级 fleet_cert_type 打标）：无视模型与自定义识别规则
//     提议的类型，同文件下全部合同（含多页）的 ContractType 统一赋值为用户指定分类；
//   - certType 为空（未打标的直连重提取）：提取类型必须落在当前已启用合同分类集合内，
//     命中停用分类或无法匹配的类型降级「其它合同」；空类型保持为空（前端显示 --）。
func (h *BusinessExtractHandler) applyContractUserCategoryPrecedence(ctx context.Context, meta *contractCustomMetadata, certType string) {
	if meta == nil {
		return
	}
	activeTypes := make(map[string]struct{})
	for _, n := range h.enabledContractCategoryNames(ctx) {
		activeTypes[n] = struct{}{}
	}
	if certType = strings.TrimSpace(certType); certType != "" {
		for i := range meta.Contracts {
			meta.Contracts[i].ContractType = certType
		}
		meta.FleetCertType = certType
		return
	}
	for i := range meta.Contracts {
		t := strings.TrimSpace(meta.Contracts[i].ContractType)
		if t == "" {
			continue
		}
		if _, ok := activeTypes[t]; ok {
			continue
		}
		meta.Contracts[i].ContractType = "其它合同"
	}
	// certType 缺失（历史遗留文件无 fleet_cert_type）但同文件所有合同最终都归属同一个
	// 启用分类时，补回文件级打标，避免这类文件每次直连重提都被错误降级。
	if meta.FleetCertType == "" && len(meta.Contracts) > 0 {
		first := strings.TrimSpace(meta.Contracts[0].ContractType)
		if first != "" && first != "其它合同" {
			allSame := true
			for _, c := range meta.Contracts {
				if strings.TrimSpace(c.ContractType) != first {
					allSame = false
					break
				}
			}
			if allSame {
				if _, ok := activeTypes[first]; ok {
					meta.FleetCertType = first
				}
			}
		}
	}
}

// enabledRegulationCategoryNames 返回当前已启用的制度分类名（scope=regulation，
// tenant_id=0 内置分类），按 sort_order 升序。与发票/合同口径一致。
func (h *BusinessExtractHandler) enabledRegulationCategoryNames(ctx context.Context) []string {
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("tenant_id = 0 AND scope = ? AND enabled = TRUE AND deleted_at IS NULL", "regulation").
		Order("sort_order ASC, created_at ASC").Find(&cats).Error; err != nil {
		logger.Warnf(ctx, "load enabled regulation categories failed: %v", err)
		return nil
	}
	names := make([]string, 0, len(cats))
	for _, c := range cats {
		if n := strings.TrimSpace(c.Name); n != "" {
			names = append(names, n)
		}
	}
	return names
}

// applyRegulationUserCategoryPrecedence 在制度写库前执行「用户指定分类绝对优先 +
// 停用分类隔离」，与发票/合同 applyUserCategoryPrecedence 口径对齐：
//   - certType 非空（上传弹窗选择）：无视模型与自定义识别规则提议的类型，同文件下全部
//     制度的 RegType 统一赋值为用户指定分类；
//   - certType 为空（直连重提取）：提取类型必须落在当前已启用制度分类集合内，命中停用
//     分类或无法匹配的类型降级「其它制度」；空类型保持为空（前端显示 --）。
func (h *BusinessExtractHandler) applyRegulationUserCategoryPrecedence(ctx context.Context, meta *regulationCustomMetadata, certType string) {
	if meta == nil {
		return
	}
	activeTypes := make(map[string]struct{})
	for _, n := range h.enabledRegulationCategoryNames(ctx) {
		activeTypes[n] = struct{}{}
	}
	if certType = strings.TrimSpace(certType); certType != "" {
		for i := range meta.Regulations {
			meta.Regulations[i].RegType = certType
		}
		meta.FleetCertType = certType
		return
	}
	for i := range meta.Regulations {
		t := strings.TrimSpace(meta.Regulations[i].RegType)
		if t == "" {
			continue
		}
		if _, ok := activeTypes[t]; ok {
			continue
		}
		meta.Regulations[i].RegType = "其它制度"
	}
	// certType 缺失（历史遗留文件无 fleet_cert_type）但同文件所有制度最终都归属同一个
	// 启用分类时，补回文件级打标，避免这类文件每次直连重提都被错误降级。
	if meta.FleetCertType == "" && len(meta.Regulations) > 0 {
		first := strings.TrimSpace(meta.Regulations[0].RegType)
		if first != "" && first != "其它制度" {
			allSame := true
			for _, r := range meta.Regulations {
				if strings.TrimSpace(r.RegType) != first {
					allSame = false
					break
				}
			}
			if allSame {
				if _, ok := activeTypes[first]; ok {
					meta.FleetCertType = first
				}
			}
		}
	}
}

// enabledNoticeCategoryNames 返回当前已启用的通知公告分类名（scope=award_punish，
// tenant_id=0 内置分类），按 sort_order 升序。与发票/合同/制度口径一致。
func (h *BusinessExtractHandler) enabledNoticeCategoryNames(ctx context.Context) []string {
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("tenant_id = 0 AND scope = ? AND enabled = TRUE AND deleted_at IS NULL", "award_punish").
		Order("sort_order ASC, created_at ASC").Find(&cats).Error; err != nil {
		logger.Warnf(ctx, "load enabled notice categories failed: %v", err)
		return nil
	}
	names := make([]string, 0, len(cats))
	for _, c := range cats {
		if n := strings.TrimSpace(c.Name); n != "" {
			names = append(names, n)
		}
	}
	return names
}

// applyNoticeUserCategoryPrecedence 在通知公告写库前执行「用户指定分类绝对优先 +
// 停用分类隔离」，与发票/合同/制度 applyUserCategoryPrecedence 口径对齐：
//   - certType 非空（上传弹窗选择）：无视模型与自定义识别规则提议的类型，同文件下全部
//     公告记录的 ApType 统一赋值为用户指定分类；
//   - certType 为空（直连重提取）：提取类型必须落在当前已启用通知公告分类集合内，命中停用
//     分类或无法匹配的类型降级「其它通知」；空类型保持为空（前端显示 --）。
func (h *BusinessExtractHandler) applyNoticeUserCategoryPrecedence(ctx context.Context, meta *awardPunishCustomMetadata, certType string) {
	if meta == nil {
		return
	}
	activeTypes := make(map[string]struct{})
	for _, n := range h.enabledNoticeCategoryNames(ctx) {
		activeTypes[n] = struct{}{}
	}
	if certType = strings.TrimSpace(certType); certType != "" {
		for i := range meta.Records {
			meta.Records[i].ApType = certType
		}
		meta.FleetCertType = certType
		return
	}
	for i := range meta.Records {
		t := strings.TrimSpace(meta.Records[i].ApType)
		if t == "" {
			continue
		}
		if _, ok := activeTypes[t]; ok {
			continue
		}
		meta.Records[i].ApType = "其它通知"
	}
	// certType 缺失（历史遗留文件无 fleet_cert_type）但同文件所有公告最终都归属同一个
	// 启用分类时，补回文件级打标，避免这类文件每次直连重提都被错误降级。
	if meta.FleetCertType == "" && len(meta.Records) > 0 {
		first := strings.TrimSpace(meta.Records[0].ApType)
		if first != "" && first != "其它通知" {
			allSame := true
			for _, r := range meta.Records {
				if strings.TrimSpace(r.ApType) != first {
					allSame = false
					break
				}
			}
			if allSame {
				if _, ok := activeTypes[first]; ok {
					meta.FleetCertType = first
				}
			}
		}
	}
}

// applyInvoiceUserCategoryPrecedence 在写库前对提取结果执行「用户指定分类绝对优先 +
// 停用分类隔离」：
//   - certType 非空（上传弹窗选择/文件级 fleet_cert_type 打标）：无视 OCR、正则、
//     自定义识别规则提议的类型，同文件下全部发票（含多页）的 InvoiceType 与 Category
//     统一赋值为用户指定分类；
//   - certType 为空（未打标的直连重提取）：OCR/规则结果必须落在当前已启用分类集合内，
//     命中已停用分类或无法匹配的类型降级「其它票据」；空类型保持为空（前端显示 --）。
func (h *BusinessExtractHandler) applyInvoiceUserCategoryPrecedence(ctx context.Context, meta *service.InvoiceCustomMetadata, certType string) {
	if meta == nil {
		return
	}
	activeTypes := make(map[string]struct{})
	for _, n := range h.enabledInvoiceCategoryNames(ctx) {
		activeTypes[n] = struct{}{}
	}
	if certType = strings.TrimSpace(certType); certType != "" {
		for i := range meta.Invoices {
			meta.Invoices[i].InvoiceType = certType
			meta.Invoices[i].Category = certType
		}
		meta.FleetCertType = certType
		return
	}
	for i := range meta.Invoices {
		t := strings.TrimSpace(meta.Invoices[i].InvoiceType)
		if t == "" {
			continue
		}
		if _, ok := activeTypes[t]; ok {
			continue
		}
		// 票面大类（如「普通发票」）已被停用，但行规则/模型给出了当前仍启用的
		// 细分分类（Category，如「通行费发票」）时，归入该细分分类，而不是笼统
		// 降级「其它票据」——细分记录本就应挂在启用的细分分类下。
		if cat := strings.TrimSpace(meta.Invoices[i].Category); cat != "" {
			if _, ok := activeTypes[cat]; ok {
				meta.Invoices[i].InvoiceType = cat
				continue
			}
		}
		meta.Invoices[i].InvoiceType = "其它票据"
	}
	// certType 缺失（历史遗留文件没有 fleet_cert_type）但同文件所有发票最终都归属
	// 同一个启用分类时，补回文件级打标，避免这类文件每次直连重提都被错误降级。
	if meta.FleetCertType == "" && len(meta.Invoices) > 0 {
		first := strings.TrimSpace(meta.Invoices[0].InvoiceType)
		if first != "" && first != "其它票据" {
			allSame := true
			for _, inv := range meta.Invoices {
				if strings.TrimSpace(inv.InvoiceType) != first {
					allSame = false
					break
				}
			}
			if allSame {
				if _, ok := activeTypes[first]; ok {
					meta.FleetCertType = first
				}
			}
		}
	}
}

func (h *BusinessExtractHandler) ExtractInvoice(c *gin.Context, certType string) {
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
		c.Error(errors.NewForbiddenError("No permission to extract invoice fields"))
		return
	}
	// 快速失败提示（真正防重入由 worker 端 extractMu 保证）：同一文档已有任务在跑时
	// 直接 409，避免连点重复投递。
	if _, loaded := h.extractMu.Load(knowledgeID); loaded {
		c.Error(errors.NewConflictError("extraction already in progress for this document").
			WithDetails(extractInProgressDetails(knowledgeID, "invoice")))
		return
	}
	taskID, err := h.enqueueBusinessExtract(ctx, effectiveTenantID, kbID, knowledgeID, "invoice", certType)
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue invoice extraction for %s: %v", knowledgeID, err)
		c.Error(errors.NewInternalServerError("enqueue invoice extraction failed: " + err.Error()))
		return
	}
	logger.Infof(ctx, "Invoice extraction enqueued: knowledge=%s task=%s", knowledgeID, taskID)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Invoice extraction submitted",
		"data": map[string]interface{}{
			"knowledge_id":   knowledgeID,
			"kind":           "invoice",
			"extract_status": "processing",
			"task_id":        taskID,
		},
	})
}

// runInvoiceCore 是不依赖 gin 的发票提取核心执行器（C 方案：HTTP handler 投递即返，
// asynq worker 调用本方法执行原同步提取逻辑）。响应改为返回值/error，由调用方记录；
// 失败状态照旧经 persistCtx 落库，模型调用失败不再因 HTTP 超时被连带掐断。
func (h *BusinessExtractHandler) runInvoiceCore(ctx context.Context, kbID, knowledgeID, certType string, effectiveTenantID uint64) (gin.H, error) {
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	// persistCtx 脱离任务/请求生命周期：任务超时/取消后仍可落库提取结果或
	// failed 状态（否则 ctx 被取消，failed 状态写不进去，记录永久停在"提取中"）。
	persistCtx := context.WithValue(context.Background(), types.TenantIDContextKey, effectiveTenantID)
	// extractCtx 30 分钟硬上限：耗时的大模型调用不被任务取消连带掐断。
	extractCtx, extractCancel := newExtractionContext(effectiveTenantID)
	defer extractCancel()

	var knowledge *types.Knowledge
	// panic 兜底：提取协程任何 panic 都强制落 failed 状态（用独立 ctx），
	// 保证状态闭环，不允许永久转圈。
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf(ctx, "Invoice extraction panic recovered: %v\n%s", r, debug.Stack())
			var panicPrev []byte
			if knowledge != nil {
				panicPrev = knowledge.CustomMetadata
			}
			failMeta := mergeFailedStateMeta(panicPrev, "invoice", fmt.Sprintf("invoice extraction panic: %v", r))
			if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, failMeta); serr != nil {
				logger.Warnf(ctx, "Failed to persist panic failed-state: %v", serr)
			}
		}
	}()

	var err error
	knowledge, err = h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for invoice extraction", err)
		return nil, errors.NewNotFoundError("Knowledge not found")
	}
	if knowledge.KnowledgeBaseID != kbID {
		return nil, errors.NewBadRequestError("Knowledge does not belong to the given knowledge base")
	}
	if businessExtractionInProgress(knowledge.ParseStatus) {
		return nil, errors.NewBadRequestError("document has not been parsed yet, please wait for parsing to finish")
	}
	// 注：不再对 manual（待补录）状态短路重提——前端自动提取链路（上传任务/列表兜底）
	// 不会对已 manual 文件发起请求，此处短路只会挡死"用户配置好规则后手动重提"的合法路径。
	// 重提会按所选归档类型规则重新提取并覆写 kind/invoices（manual 文件 invoices 恒为空，
	// 无手工补录数据可丢失）。

	// worker 上下文没有 gin 侧的 kb 绑定：重新按 kbID 取知识库配置（仅需 summary_model_id）。
	kb, kberr := h.kbService.GetKnowledgeBaseByIDOnly(effCtx, kbID)
	if kberr != nil || kb == nil {
		return nil, errors.NewNotFoundError("Knowledge base not found")
	}
	modelID := strings.TrimSpace(kb.SummaryModelID)
	if modelID == "" {
		return nil, errors.NewBadRequestError("no extraction model configured for the knowledge base")
	}
	chatModel, err := h.modelService.GetChatModel(extractCtx, modelID)
	if err != nil {
		logger.Error(ctx, "Failed to load extraction model", err)
		return nil, errors.NewInternalServerError("get extraction model failed: " + err.Error())
	}

	chunks, err := h.chunkService.ListChunksByKnowledgeID(extractCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to list chunks for invoice extraction", err)
		return nil, errors.NewInternalServerError("list chunks failed: " + err.Error())
	}
	// 分批提取：超长多发票文档（18/22 张）单次 LLM 调用不可靠（智谱 500 /
	// 输出截断），按 chunk 分批独立调用后合并，Normalize 时按发票号去重。
	batches := service.BuildInvoiceExtractionBatches(knowledge.FileName, knowledge.Description, chunks, 0)
	// 扫描件防御：无文本层（扫描件 PDF / 纯图片）时不自动删除，保留文件并标记
	// failed 让用户人工处理。
	if !invoiceBatchesHaveText(batches) {
		zhNotice, enNotice := noExtractableTextNotice(knowledge.ParseStatus)
		noTextMeta := mergeFailedStateMeta(knowledge.CustomMetadata, "invoice", zhNotice)
		if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, noTextMeta); serr != nil {
			logger.Warnf(ctx, "Failed to persist invoice no-text state: %v", serr)
		}
		logger.Warnf(ctx, "Invoice extraction skipped: no text layer for knowledge %s (parse_status=%s), file kept", knowledgeID, knowledge.ParseStatus)
		return gin.H{
			"success": true,
			"message": enNotice,
			"data":    map[string]interface{}{"kind": "invoice", "extract_status": "failed", "removed": false},
		}, nil
	}
	merged := &service.InvoiceExtractionResult{Kind: "invoice"}
	var extractErr error
	sawInvoice := false
	// 提取进行中标记：与合同/车队提取一致，先落 processing 中间态，让前端轮询
	// 状态条实时显示「正在提取『文件名』进度 N%」。保留既有字段（fleet_cert_type、
	// 旧 invoices 等），提取成功/失败路径会用终态整体覆盖。
	procMeta := map[string]interface{}{"kind": "invoice", "extract_status": "processing"}
	if len(knowledge.CustomMetadata) > 0 {
		var prev map[string]interface{}
		if jerr := json.Unmarshal(knowledge.CustomMetadata, &prev); jerr == nil {
			for k, v := range prev {
				if _, exists := procMeta[k]; !exists {
					procMeta[k] = v
				}
			}
		}
	}
	delete(procMeta, "extract_error")
	if pmeta, jerr := json.Marshal(procMeta); jerr == nil {
		if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, types.JSON(pmeta)); serr != nil {
			logger.Warnf(ctx, "Failed to persist invoice processing-state: %v", serr)
		}
	}
	// 沙盒提取规则：优先按请求携带的归档类型（上传弹窗 body.cert_type，如"三联收据"）
	// 精确匹配分类规则；请求未携带时回退文件级打标 fleet_cert_type；再按
	// resolveInvoiceRuleCfg 内建回退链（分类→普通发票→整体规则→内置模板）。
	ruleCertType := certType
	if ruleCertType == "" && len(knowledge.CustomMetadata) > 0 {
		var mdMeta struct {
			FleetCertType string `json:"fleet_cert_type"`
		}
		if jerr := json.Unmarshal(knowledge.CustomMetadata, &mdMeta); jerr == nil && mdMeta.FleetCertType != "" {
			ruleCertType = mdMeta.FleetCertType
		}
	}
	invoiceRuleCfg := h.resolveInvoiceRuleCfg(ctx, kbID, ruleCertType)
	// 明细行子列口径注入：从分类配置实时读取 items 字段的 SubFields（通行费/电费等
	// 分类的票面明细结构不同），让规则与 LLM 两条提取路径都按分类真实结构输出明细。
	h.injectInvoiceItemSubFields(ctx, ruleCertType, invoiceRuleCfg)
	// 规则提取优先：系统生成的规整电子票据（通行费汇总单等）文本格式固定，
	// 按「号码行+金额行」正则可 100% 全量命中（模型在高模板噪音长文本中会漏提，
	// 实测 18 张通行费发票只提取出 1~8 张）；未命中该固定格式时才回退分批模型。
	docHint := knowledgeID
	if len(docHint) > 8 {
		docHint = docHint[:8]
	}
	// 规则提取阶段此前完全无日志：一旦卡在这里（大文档正则/字符串处理慢），
	// 日志里只有 "task started" 然后长时间静默，无法判断卡在规则还是模型调用。
	logger.Infof(ctx, "Invoice extraction: start rule extraction (%d chunks, %d batches) knowledge=%s",
		len(chunks), len(batches), knowledgeID)
	ruleRes, ruleOK := service.ExtractInvoicesByLineRulesFromChunks(chunks)
	logger.Infof(ctx, "Invoice extraction: line rules done, hit=%v knowledge=%s", ruleOK, knowledgeID)
	if ruleOK {
		logger.Infof(ctx, "Invoice extraction by line rules: %d invoices from knowledge %s", len(ruleRes.Invoices), knowledgeID)
	}
	if !ruleOK {
		// 新版 docreader（invoice2.html）输出 Markdown 表格，旧纯文本「号码行+金额行」
		// 规则不命中；改用 Markdown 表格规则确定性提取（税号/车牌/日期/金额/税率/税额
		// 均为 ASCII），避免回退 LLM 在乱码长文本上耗时 3 分钟超时。
		if mdRes, mdOK := service.ExtractInvoicesByMarkdownTableRulesFromChunks(chunks, docHint); mdOK {
			ruleRes = mdRes
			ruleOK = true
			logger.Infof(ctx, "Invoice extraction by markdown-table rules: %d invoices from knowledge %s", len(mdRes.Invoices), knowledgeID)
		}
	}
	if !ruleOK {
		// 第三套：扁平布局规则（docreader 实际输出格式）——发票代码/号码/开票日期
		// 独立成行，明细行字段顺序固定（项目 车牌 车型 通行起 通行止 金额 税率 税额），
		// 可确定性解析，毫秒级完成且不会把『机器编号』误当发票号。
		// 为什么排在 LLM 之前：实测 16 页票据即使缩到 8 chunk/批，LLM 仍会因
		// "多张票 × 8 个子列"的输出量触发 5 分钟超时，稳定性远不如规则。
		if flatRes, flatOK := service.ExtractInvoicesByFlatLayoutFromChunks(chunks); flatOK {
			ruleRes = flatRes
			ruleOK = true
			logger.Infof(ctx, "Invoice extraction by flat-layout rules: %d invoices from knowledge %s", len(flatRes.Invoices), knowledgeID)
		}
	}
	if ruleOK {
		merged = ruleRes
		sawInvoice = len(ruleRes.Invoices) > 0
	} else {
		// 已启用发票类型列表在批次间不变，提至循环外避免每批重复查库
		enabledCategoryNames := strings.Join(h.enabledInvoiceCategoryNames(extractCtx), "、")
		failedBatches := 0
		logger.Infof(ctx, "Invoice extraction: rule miss, entering LLM batching (%d batches, concurrency=%d) knowledge=%s",
			len(batches), invoiceBatchConcurrency, knowledgeID)

		// 受控并发：批次串行时总时长 = 批次数 × 单批耗时（实测 2~3 分钟/批），
		// 是"慢"的主因；并发 2 可把 4 批从 12 分钟压到 6~7 分钟，上限 2 兼顾
		// 供应商限流风险（更高并发容易触发 429）。
		//
		// 防"张冠李戴"红线：结果写入**预分配切片按批次索引**（results[idx]），
		// 绝不 append —— 并发 append 顺序不定，会让发票与页码错位。
		// 合并阶段回到主 goroutine 顺序遍历，保证 Page 归位与文档顺序一致。
		results := make([]*service.InvoiceExtractionResult, len(batches))
		errs := make([]error, len(batches))
		sem := make(chan struct{}, invoiceBatchConcurrency)
		var wg sync.WaitGroup
		for i, batch := range batches {
			content := strings.TrimSpace(batch.Content)
			if content == "" {
				continue
			}
			wg.Add(1)
			go func(idx int, content string, batch service.InvoiceExtractionBatch) {
				defer wg.Done()
				// goroutine 内未捕获的 panic 会终止整个进程，必须逐批兜住。
				defer func() {
					if r := recover(); r != nil {
						errs[idx] = fmt.Errorf("batch %d panic: %v", idx+1, r)
						logger.Errorf(ctx, "Invoice extraction batch %d panic: %v\n%s", idx+1, r, debug.Stack())
					}
				}()
				sem <- struct{}{}
				defer func() { <-sem }()

				// 分批提取多发票合集时，模型常因单批文本碎片化（模板噪音占比高、发票被
				// 跨 chunk 拆开）而漏提本批中的部分发票（典型：18 张只提取出 8 张）。
				// 主调用用"发票合集 + 第 N/M 批"引导，要求提取本批全部可见发票；
				// 失败时再回退普通调用兜底一次。
				guide := fmt.Sprintf(
					"重要提示：文档《%s》是包含多张电子发票的合集（文档描述：%s）。以下文本是该文档的第 %d/%d 批内容，本批文本中包含若干张发票。"+
						"请逐张提取本批文本中出现的全部发票（特征：发票号码、金额、税率/征收率、税额、价税合计、购销方信息、开票人、发票类型等），一张都不可遗漏。"+
						"文本可能因文档分页/拆分而不完整，只要出现发票号码或金额信息即应作为一张发票提取；禁止因文本不完整而将 kind 设为 not_invoice。"+
						"类型约束：invoice_type 只能从当前已启用的发票类型中选择：%s；禁止输出该列表之外的任何类型。",
					knowledge.FileName, knowledge.Description, idx+1, len(batches), enabledCategoryNames)

				// 每批独立时间预算：Guided 可能吃满一次长超时（实测 5 分钟），
				// 回退若再叠加多次重试，单批最坏耗时可达 25 分钟。
				logger.Infof(ctx, "Invoice extraction batch %d/%d start: chunks=%d-%d runes=%d",
					idx+1, len(batches), batch.StartIndex, batch.EndIndex, len([]rune(content)))
				batchCtx, cancelBatch := context.WithTimeout(extractCtx, invoiceBatchBudget)
				defer cancelBatch()
				res, berr := service.ExtractInvoicesFromContentWithRulesGuided(batchCtx, chatModel, content, invoiceRuleCfg, guide)
				if berr != nil {
					// 回退只再试一次：主调用失败时通常已吃满一次超时。
					logger.Warnf(ctx, "Invoice extraction batch %d guided failed, fallback to plain (single attempt): %v", idx+1, berr)
					res, berr = service.ExtractInvoicesWithRulesOnce(batchCtx, chatModel, content, invoiceRuleCfg)
				}
				results[idx], errs[idx] = res, berr
			}(i, content, batch)
		}
		wg.Wait()

		// 顺序合并 + 页码归位（并发期间各批只写自己的 results[idx]，此处单线程合并）
		for i := range batches {
			if errs[i] != nil {
				failedBatches++
				logger.Warnf(ctx, "Invoice extraction batch %d/%d failed (non-fatal): %v", i+1, len(batches), errs[i])
				continue
			}
			batchRes := results[i]
			if batchRes == nil || batchRes.Kind == "not_invoice" {
				continue
			}
			// 页码归位（防"张冠李戴"红线）：批次在原文档中的 chunk 区间 + 批内顺序，
			// 覆盖服务层按结果序号赋的 Page。
			for j := range batchRes.Invoices {
				batchRes.Invoices[j].Page = batches[i].StartIndex + 1 + j
			}
			sawInvoice = true
			// 观测性透传：把每批的校验问题与正则补洞溯源合并到整份结果。
			// 批内下标是**批局部**的，合并时要偏移成整份结果的下标，否则前端
			// 按 index 定位会指到别的发票（Index=-1 是结果级问题，保持原样）。
			base := len(merged.Invoices)
			for _, issue := range batchRes.ExtractIssues {
				if issue.Index >= 0 {
					issue.Index += base
				}
				merged.ExtractIssues = append(merged.ExtractIssues, issue)
			}
			for _, patch := range batchRes.RegexPatches {
				if patch.Index >= 0 {
					patch.Index += base
				}
				merged.RegexPatches = append(merged.RegexPatches, patch)
			}
			merged.PatchedByRegex = appendUniqueStrings(merged.PatchedByRegex, batchRes.PatchedByRegex...)
			merged.Invoices = append(merged.Invoices, batchRes.Invoices...)
		}
		// 全部批次失败且无任何产出 → 整体失败（记 failed、可重试）；
		// 只要有一批成功就保留部分结果，不因个别批次丢掉整份成果。
		if failedBatches == len(batches) && len(merged.Invoices) == 0 {
			if errs[0] != nil {
				extractErr = errs[0]
			} else {
				extractErr = fmt.Errorf("all %d extraction batches failed", len(batches))
			}
		}
		// 部分完成提示：有批次失败但仍拿到数据时，把"结果可能不完整"写入 ExtractError
		// （状态仍为 success——已有可用数据，不该因个别批次丢掉整份成果）。
		// 用户据此知道需要复核哪些页码，而不是以为全部提取成功。
		if failedBatches > 0 && len(merged.Invoices) > 0 {
			merged.ExtractError = fmt.Sprintf(
				"部分批次未完成（%d/%d 批失败，可能因模型超时或返回异常），已保留其余批次结果，建议复核",
				failedBatches, len(batches))
		}
	}
	if !sawInvoice {
		merged.Kind = "not_invoice"
	}
	if extractErr != nil {
		// Persist a failed state so the UI shows "提取失败" (retriable via the
		// floating toolbar) instead of looping forever as "提取中/待提取". Preserve
		// previously extracted invoices rather than overwriting them on a transient
		// model failure.
		failMeta := mergeFailedStateMeta(knowledge.CustomMetadata, "invoice", "invoice extraction failed: "+extractErr.Error())
		if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, failMeta); serr != nil {
			logger.Warnf(ctx, "Failed to persist invoice extraction failed-state: %v", serr)
		}
		logger.Error(ctx, "Invoice extraction model call failed", extractErr)
		return nil, errors.NewInternalServerError("invoice extraction failed: " + extractErr.Error())
	}
	extracted := merged
	service.NormalizeInvoiceExtractionResult(extracted)
	// 确定性覆盖层：用票面明确标签纠正 LLM 易错字段——发票号码（实测曾误取
	// 机器编号 499098302394 而非『发票号码 64869484』）与明细子列（车牌/车型/
	// 通行日期起止）。张数不一致时该函数不改动任何字段（防按序错位）。
	if n := service.OverlayDeterministicInvoiceFields(extracted, chunks); n > 0 {
		logger.Infof(ctx, "Invoice extraction: deterministic overlay applied (%d invoices) knowledge=%s", n, knowledgeID)
	}
	// 规则路径（Line / MarkdownTable / FlatLayout）此前**完全没有质量元数据**：
	// 走规则就永远看不到 extract_issues，日期也保持票面原样（"2026年7月2日" 与
	// "2026-07-02" 混在同一个列表里，排序/筛选全错）。这里补上与 LLM 路径一致的
	// 确定性收尾——日期归一化 + 配置级必填校验 + 后置正则补洞，零 token 成本，
	// 且**不触发 LLM Repair**（规则路径存在的意义就是省掉模型调用）。
	// 放在 overlay 之后：overlay 会用票面标签纠正号码/明细，先校验会报"已被修正"的假问题。
	if ruleOK && extracted.Kind == "invoice" {
		issues := service.FinalizeInvoiceRuleResult(
			extractCtx, extracted, invoiceBatchesFullText(batches), invoiceRuleCfg)
		logger.Infof(ctx, "Invoice extraction: rule path finalized, issues=%d patched=%v knowledge=%s",
			len(issues), extracted.PatchedByRegex, knowledgeID)
	}

	meta := service.InvoiceCustomMetadata{
		Kind:           extracted.Kind,
		Invoices:       extracted.Invoices,
		ExtractStatus:  "success",
		ExtractError:   extracted.ExtractError,
		ExtractIssues:  extracted.ExtractIssues,
		PatchedByRegex: extracted.PatchedByRegex,
		RegexPatches:   extracted.RegexPatches,
	}
	// 自定义识别规则：类型归类（用户可配置关键词/正则 → 发票类型），
	// 按模型提取出的发票类型字段匹配，命中则覆盖内置关键字判定。
	if recCfg, rerr := h.kgService.GetRecognitionConfig(extractCtx, kbID); rerr == nil {
		fullText := invoiceBatchesFullText(batches)
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
	// 归档细分分类：上传弹窗选择的票据类型（cert_type，如"通行费发票"）写入每条
	// 记录 category，供列表「细分分类」筛选（普通/专用为票面大类，通行费/电费等
	// 为细分分类，细分记录仍统计进所属大类的卡片）。cert_type 为空（单文件重提取
	// 直连路由）时从文件级 fleet_cert_type 元数据兜底。
	if certType == "" && len(knowledge.CustomMetadata) > 0 {
		var certMeta struct {
			FleetCertType string `json:"fleet_cert_type"`
		}
		if jerr := json.Unmarshal(knowledge.CustomMetadata, &certMeta); jerr == nil {
			certType = certMeta.FleetCertType
		}
	}
	// 用户指定分类绝对优先 + 停用分类隔离（任务 1）：
	// certType 非空 → 同文件全部发票的 InvoiceType 与 Category 统一赋值为用户指定分类
	// （无视 OCR/规则/自定义识别规则）；certType 为空 → 类型必须落在已启用分类集合内，
	// 命中停用分类或无法匹配的类型降级「其它票据」。
	// 用 ruleCertType 而不是 certType：自动提取链路（前端轮询）不带 cert_type，
	// 此时 certType 为空会导致「用户选择的分类」失效、回退成票面推断的类型，
	// 推断结果不在启用列表时就被降级成「其它票据」（用户反馈 2026-10-17）。
	// ruleCertType 已做 certType → 文件级 fleet_cert_type 的回退。
	h.applyInvoiceUserCategoryPrecedence(extractCtx, &meta, ruleCertType)
	if extracted.Kind == "not_invoice" {
		// 自定义识别规则捞回：模型判非但包含规则命中 → 认定为发票，置 manual 待补录，
		// 不自动删除（避免"该是发票却没入库"）。
		if recCfg, rerr := h.kgService.GetRecognitionConfig(extractCtx, kbID); rerr == nil {
			if service.MatchIncludeRules(invoiceBatchesFullText(batches), recCfg) {
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
				return gin.H{
					"success": true,
					"message": "识别规则命中，已认定为发票，请在列表中编辑补录字段",
					"data":    map[string]interface{}{"kind": "invoice", "extract_status": "manual", "removed": false},
				}, nil
			}
		}
		meta.ExtractStatus = "not_invoice"
		if meta.ExtractError == "" {
			meta.ExtractError = "document does not look like an invoice"
		}
		// 防循环：auto_deleted_count >= 1（曾被自动删除后人工恢复）→ 不再自动删除，
		// 改为 failed 保留。
		// 需求变更（发票删除历史功能弃用）：非发票文件不再自动删除，保留在库并标记
		// failed，用户可通过「待复核」卡/「已上传文件」抽屉人工处理。
		meta.ExtractStatus = "failed"
		if h.autoDeleteCount(extractCtx, knowledge) > 0 {
			meta.ExtractError = "系统判定非发票文件，该文件曾被人工恢复，已保留待人工处理"
		} else {
			meta.ExtractError = "系统判定非发票文件，已保留待人工复核"
		}
		logger.Infof(ctx, "non-invoice knowledge kept for manual review, ID: %s", knowledgeID)
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
		h.businessSvc.MarkInvoiceDuplicates(extractCtx, kbID, knowledgeID, &meta)
	}

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, errors.NewInternalServerError("failed to encode extraction result")
	}
	// 合并而非覆盖：保留 custom_metadata 中提取流程不管理的字段
	//（如 auto_deleted_count、历史 fleet_cert_type），避免整 JSON 覆盖丢数据。
	// 三结构体收敛：以 InvoiceCustomMetadata 为唯一写入形状，其它字段透传保留。
	mergedJSON := metaJSON
	if len(knowledge.CustomMetadata) > 0 {
		var existing map[string]interface{}
		var fresh map[string]interface{}
		if jerr1 := json.Unmarshal(knowledge.CustomMetadata, &existing); jerr1 == nil {
			if jerr2 := json.Unmarshal(metaJSON, &fresh); jerr2 == nil {
				for k, v := range fresh {
					existing[k] = v
				}
				if mj, merr := json.Marshal(existing); merr == nil {
					mergedJSON = mj
				}
			}
		}
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, types.JSON(mergedJSON)); err != nil {
		logger.Error(ctx, "Failed to persist invoice extraction result", err)
		return nil, errors.NewInternalServerError("failed to save extraction result: " + err.Error())
	}

	// 自动打标签功能已弃用（需求决策：不再自动创建/关联大类标签，
	// 保留手动标签）。category 字段仍随提取结果保存，仅不再自动转标签。

	logger.Infof(ctx, "Invoice extraction succeeded, knowledge ID: %s, kind: %s, invoices: %d",
		knowledgeID, meta.Kind, len(meta.Invoices))
	if meta.ExtractStatus == "failed" {
		logger.Warnf(ctx, "Invoice extraction returned an unusable result, knowledge ID: %s, err: %s", knowledgeID, meta.ExtractError)
		return nil, errors.NewInternalServerError(meta.ExtractError)
	}
	return gin.H{
		"success": true,
		"message": "Invoice extraction succeeded",
		"data": map[string]interface{}{
			"knowledge_id":   knowledgeID,
			"kind":           meta.Kind,
			"extract_status": meta.ExtractStatus,
			"invoices":       meta.Invoices,
		},
	}, nil
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

func (h *BusinessExtractHandler) ExtractInvoicePage(c *gin.Context) {
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
	// 防重入：与整份提取共用同一文档锁
	if _, loaded := h.extractMu.LoadOrStore(knowledgeID, struct{}{}); loaded {
		c.Error(errors.NewConflictError("extraction already in progress for this document"))
		return
	}
	defer h.extractMu.Delete(knowledgeID)

	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	page, _ := parseSandboxPagination(c)
	// query 未显式传 page 时，允许从 POST body 取（parseSandboxPagination 恒返回 >=1，
	// 原 if page < 1 判断永假，body 分支为死代码）
	if c.Query("page") == "" {
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
	if businessExtractionInProgress(knowledge.ParseStatus) {
		c.Error(errors.NewBadRequestError("document has not been parsed yet, please wait for parsing to finish"))
		return
	}
	var meta service.InvoiceCustomMetadata
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
	invoiceRuleCfg := h.resolveInvoiceRuleCfg(ctx, kbID, "")
	if kdMeta := knowledge.CustomMetadata; len(kdMeta) > 0 {
		var mdMeta struct {
			FleetCertType string `json:"fleet_cert_type"`
		}
		if jerr := json.Unmarshal(kdMeta, &mdMeta); jerr == nil && mdMeta.FleetCertType != "" {
			invoiceRuleCfg = h.resolveInvoiceRuleCfg(ctx, kbID, mdMeta.FleetCertType)
		}
	}
	res, err := service.ExtractInvoicePageFromContentWithRules(effCtx, chatModel, content, page, invoiceRuleCfg)
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
	// 重提取不丢归档细分分类：文件级 fleet_cert_type（上传弹窗选择）覆盖新提取记录的
	// InvoiceType 与 Category（与主提取链路 applyInvoiceUserCategoryPrecedence 同口径）；
	// 未打标时按已启用分类集合过滤，命中停用分类降级「其它票据」。
	pageCert := ""
	if len(knowledge.CustomMetadata) > 0 {
		var certMeta struct {
			FleetCertType string `json:"fleet_cert_type"`
		}
		if jerr := json.Unmarshal(knowledge.CustomMetadata, &certMeta); jerr == nil {
			pageCert = certMeta.FleetCertType
		}
	}
	tmp := &service.InvoiceCustomMetadata{Invoices: []service.InvoiceExtractionItem{upd}}
	h.applyInvoiceUserCategoryPrecedence(effCtx, tmp, pageCert)
	if len(tmp.Invoices) > 0 {
		upd = tmp.Invoices[0]
		upd.Page = page
	}
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

func (h *BusinessExtractHandler) DeleteInvoicePage(c *gin.Context) {
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

	page, _ := parseSandboxPagination(c)
	// query 未显式传 page 时，允许从 POST body 取（parseSandboxPagination 恒返回 >=1，
	// 原 if page < 1 判断永假，body 分支为死代码）
	if c.Query("page") == "" {
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
	var meta service.InvoiceCustomMetadata
	if err := json.Unmarshal(knowledge.CustomMetadata, &meta); err != nil || meta.Kind != "invoice" || len(meta.Invoices) == 0 {
		c.Error(errors.NewBadRequestError("no invoice extraction data to delete"))
		return
	}
	if page > len(meta.Invoices) {
		c.Error(errors.NewBadRequestError(fmt.Sprintf("page %d out of range (1-%d)", page, len(meta.Invoices))))
		return
	}

	// 移除该页发票 + 删空时级联删除源文件（service 层实现，见 knowledge_invoice_cascade_delete.go）。
	// 级联顺序：先落 invoices=[] + pending_file_delete + parse_status=deleting（一次两列），
	// 再 repo.DeleteKnowledge —— 保证即便异步清理失败也不会留下空壳孤儿挡住用户重传。
	deletedFile, remaining, rerr := h.businessSvc.RemoveInvoicePage(effCtx, kbID, knowledgeID, page)
	if rerr != nil {
		logger.Error(ctx, "Failed to remove invoice page", rerr)
		msg := rerr.Error()
		if isInvalidInvoicePageDelete(msg) {
			c.Error(errors.NewBadRequestError(msg))
			return
		}
		c.Error(errors.NewInternalServerError("failed to remove invoice page: " + msg))
		return
	}
	if deletedFile {
		// 源文件已删（或已置 deleting）。入队清理 chunks / 向量 / 存储对象；
		// 入队失败不影响上一步的删除结果，仅记录告警由巡检兜底。
		if _, qerr := h.enqueueKnowledgeListDelete(effCtx, effectiveTenantID, kbID, []string{knowledgeID}); qerr != nil {
			logger.Warn(ctx, "last invoice removed but enqueue knowledge delete failed (will be swept)", qerr)
		}
		c.JSON(http.StatusOK, gin.H{
			"success":      true,
			"message":      "Invoice removed, source file deleted",
			"deleted_file": true,
		})
		return
	}
	// 仍有剩余发票：service 已持久化剩余 invoices（页码已重排）。
	// ⚠️ 此处**不得**再用函数开头解析出的 meta 回写 —— 那是删除前的全量快照，
	// 会把刚删掉的发票写回去（联级删除与逐张删除都会静默失效）。
	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"message":      "Invoice record deleted",
		"deleted_file": false,
		"data":         remaining,
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

// isInvalidInvoicePageDelete 判断删除失败是否属于「调用方参数/状态错误」，
// 这类错误应回 400 而不是 500（避免用户看到"系统错误"而反复重试无效操作）。
func isInvalidInvoicePageDelete(msg string) bool {
	return strings.Contains(msg, "out of range") ||
		strings.Contains(msg, "positive integer") ||
		strings.Contains(msg, "does not belong") ||
		strings.Contains(msg, "no invoice extraction data") ||
		strings.Contains(msg, "not found")
}

type invoiceCustomMetadata struct {
	Kind          string                          `json:"kind"`
	Invoices      []service.InvoiceExtractionItem `json:"invoices"`
	ExtractStatus string                          `json:"extract_status"`
	ExtractError  string                          `json:"extract_error"`
}

func invoiceBatchesHaveText(batches []service.InvoiceExtractionBatch) bool {
	for _, b := range batches {
		if strings.TrimSpace(b.Content) != "" {
			return true
		}
	}
	return false
}

// invoiceBatchesFullText 拼接全部批次文本（自定义识别规则在整篇文档上匹配）。
func invoiceBatchesFullText(batches []service.InvoiceExtractionBatch) string {
	parts := make([]string, 0, len(batches))
	for _, b := range batches {
		parts = append(parts, b.Content)
	}
	return strings.Join(parts, "\n")
}

func (h *BusinessExtractHandler) ExtractRegulation(c *gin.Context, certType string) {
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
		c.Error(errors.NewForbiddenError("No permission to extract regulation fields"))
		return
	}
	// 快速失败提示（真正防重入由 worker 端 extractMu 保证）：同一文档已有任务在跑时
	// 直接 409，避免连点重复投递。
	if _, loaded := h.extractMu.Load(knowledgeID); loaded {
		c.Error(errors.NewConflictError("extraction already in progress for this document").
			WithDetails(extractInProgressDetails(knowledgeID, "regulation")))
		return
	}
	taskID, err := h.enqueueBusinessExtract(ctx, effectiveTenantID, kbID, knowledgeID, "regulation", certType)
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue regulation extraction for %s: %v", knowledgeID, err)
		c.Error(errors.NewInternalServerError("enqueue regulation extraction failed: " + err.Error()))
		return
	}
	logger.Infof(ctx, "Regulation extraction enqueued: knowledge=%s task=%s", knowledgeID, taskID)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Regulation extraction submitted",
		"data": map[string]interface{}{
			"knowledge_id":   knowledgeID,
			"kind":           "regulation",
			"extract_status": "processing",
			"task_id":        taskID,
		},
	})
}

// runRegulationCore 是不依赖 gin 的制度提取核心执行器（C 方案：HTTP handler 投递即返，
// asynq worker 调用本方法执行原同步提取逻辑）。响应改为返回值/error，由调用方记录；
// 失败状态照旧经 persistCtx 落库，模型调用失败不再因 HTTP 超时被连带掐断。
func (h *BusinessExtractHandler) runRegulationCore(ctx context.Context, kbID, knowledgeID, certType string, effectiveTenantID uint64) (gin.H, error) {
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	// persistCtx 脱离任务/请求生命周期：任务超时/取消后仍可落库提取结果或
	// failed 状态（否则 ctx 被取消，failed 状态写不进去，记录永久停在"提取中"）。
	persistCtx := context.WithValue(context.Background(), types.TenantIDContextKey, effectiveTenantID)
	// extractCtx 30 分钟硬上限：耗时的大模型调用不被任务取消连带掐断。
	extractCtx, extractCancel := newExtractionContext(effectiveTenantID)
	defer extractCancel()

	var knowledge *types.Knowledge
	// panic 兜底：提取协程任何 panic 都强制落 failed 状态（用独立 ctx），
	// 保证状态闭环，不允许永久转圈。
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf(ctx, "Regulation extraction panic recovered: %v\n%s", r, debug.Stack())
			var panicPrev []byte
			if knowledge != nil {
				panicPrev = knowledge.CustomMetadata
			}
			failMeta := mergeFailedStateMeta(panicPrev, "regulation", fmt.Sprintf("regulation extraction panic: %v", r))
			if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, failMeta); serr != nil {
				logger.Warnf(ctx, "Failed to persist panic failed-state: %v", serr)
			}
		}
	}()

	var err error
	knowledge, err = h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for regulation extraction", err)
		return nil, errors.NewNotFoundError("Knowledge not found")
	}
	if knowledge.KnowledgeBaseID != kbID {
		return nil, errors.NewBadRequestError("Knowledge does not belong to the given knowledge base")
	}
	if businessExtractionInProgress(knowledge.ParseStatus) {
		return nil, errors.NewBadRequestError("document has not been parsed yet, please wait for parsing to finish")
	}
	// 待补录豁免：manual 记录由用户人工维护，不自动重提取、不参与任何自动删除判定。
	var curRegMeta regulationCustomMetadata
	_ = json.Unmarshal(knowledge.CustomMetadata, &curRegMeta)
	if curRegMeta.ExtractStatus == "manual" {
		logger.Infof(ctx, "Regulation extraction skipped: knowledge %s is manual intake", knowledgeID)
		return gin.H{
			"success": true,
			"message": "待补录记录，无需重复提取",
			"data":    map[string]interface{}{"kind": "regulation", "extract_status": "manual", "removed": false},
		}, nil
	}

	// worker 上下文没有 gin 侧的 kb 绑定：重新按 kbID 取知识库配置（仅需 summary_model_id）。
	kb, kberr := h.kbService.GetKnowledgeBaseByIDOnly(effCtx, kbID)
	if kberr != nil || kb == nil {
		return nil, errors.NewNotFoundError("Knowledge base not found")
	}
	modelID := strings.TrimSpace(kb.SummaryModelID)
	if modelID == "" {
		return nil, errors.NewBadRequestError("no extraction model configured for the knowledge base")
	}
	chatModel, err := h.modelService.GetChatModel(extractCtx, modelID)
	if err != nil {
		logger.Error(ctx, "Failed to load extraction model", err)
		return nil, errors.NewInternalServerError("get extraction model failed: " + err.Error())
	}

	chunks, err := h.chunkService.ListChunksByKnowledgeID(extractCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to list chunks for regulation extraction", err)
		return nil, errors.NewInternalServerError("list chunks failed: " + err.Error())
	}
	batches := service.BuildRegulationExtractionBatches(knowledge.FileName, knowledge.Description, chunks, 0)
	// 扫描件防御：无文本层（扫描件 PDF / 纯图片）时不要自动删除，保留文件并标记
	// failed 让用户人工处理（配合删除历史）。
	if !regulationBatchesHaveText(batches) {
		zhNotice, enNotice := noExtractableTextNotice(knowledge.ParseStatus)
		noTextMeta := mergeFailedStateMeta(knowledge.CustomMetadata, "regulation", zhNotice)
		if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, noTextMeta); serr != nil {
			logger.Warnf(ctx, "Failed to persist regulation no-text state: %v", serr)
		}
		logger.Warnf(ctx, "Regulation extraction skipped: no text layer for knowledge %s (parse_status=%s), file kept", knowledgeID, knowledge.ParseStatus)
		return gin.H{
			"success": true,
			"message": enNotice,
			"data":    map[string]interface{}{"kind": "regulation", "extract_status": "failed", "removed": false},
		}, nil
	}
	merged := &service.RegulationExtractionResult{Kind: "regulation"}
	var extractErr error
	sawRegulation := false
	// 提取进行中标记：先落 processing 中间态，让前端轮询/状态条与表格状态列实时显示
	//「正在提取」（避免点重新提取后状态列短暂停留在旧值/failed）。合并保留既有字段
	//（fleet_cert_type、旧 regulations 等），提取成功/失败路径会用终态整体覆盖。
	procMeta := map[string]interface{}{"kind": "regulation", "extract_status": "processing"}
	if len(knowledge.CustomMetadata) > 0 {
		var prev map[string]interface{}
		if jerr := json.Unmarshal(knowledge.CustomMetadata, &prev); jerr == nil {
			for k, v := range prev {
				if _, exists := procMeta[k]; !exists {
					procMeta[k] = v
				}
			}
		}
	}
	delete(procMeta, "extract_error")
	if pmeta, jerr := json.Marshal(procMeta); jerr == nil {
		if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, types.JSON(pmeta)); serr != nil {
			logger.Warnf(ctx, "Failed to persist regulation processing-state: %v", serr)
		}
	}
	// 规则解析：优先请求携带的归档类型（上传弹窗 body.cert_type），未携带时回退文件级
	// 打标 fleet_cert_type，再按 resolveRegulationRuleCfg 回退整体规则。
	ruleCertType := certType
	if ruleCertType == "" && len(knowledge.CustomMetadata) > 0 {
		var mdMeta struct {
			FleetCertType string `json:"fleet_cert_type"`
		}
		if jerr := json.Unmarshal(knowledge.CustomMetadata, &mdMeta); jerr == nil && mdMeta.FleetCertType != "" {
			ruleCertType = mdMeta.FleetCertType
		}
	}
	// 用户配置的提取规则（scope=regulation）：按归档分类匹配，未配置时回退整体规则
	regulationRuleCfg := h.resolveRegulationRuleCfg(extractCtx, kbID, ruleCertType)
	for i, batch := range batches {
		if strings.TrimSpace(batch) == "" {
			continue
		}
		batchRes, berr := service.ExtractRegulationsFromContentWithRules(extractCtx, chatModel, batch, regulationRuleCfg)
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
		failMeta := mergeFailedStateMeta(knowledge.CustomMetadata, "regulation", "regulation extraction failed: "+extractErr.Error())
		if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, failMeta); serr != nil {
			logger.Warnf(ctx, "Failed to persist regulation extraction failed-state: %v", serr)
		}
		logger.Error(ctx, "Regulation extraction model call failed", extractErr)
		return nil, errors.NewInternalServerError("regulation extraction failed: " + extractErr.Error())
	}
	extracted := merged
	// 自动编号去重：无制度编号的制度按当天已有最大序号 +1 顺序递增（跨文件唯一）
	autoSeq, seqErr := h.businessSvc.MaxAutoRegulationSeq(extractCtx, kbID)
	if seqErr != nil {
		logger.Warnf(ctx, "Failed to compute auto regulation seq for %s: %v", kbID, seqErr)
	}
	service.NormalizeRegulationExtractionResult(extracted, autoSeq)
	// 通用质量闭环（非发票业务口径）：日期归一化 + 配置级必填补洞/告警。
	regulationIssues, regulationPatches := service.ValidateGenericExtraction(
		service.NewGenericRecords(extracted.Regulations), strings.Join(batches, "\n"), regulationRuleCfg)
	if len(regulationIssues) > 0 || len(regulationPatches) > 0 {
		logger.Infof(ctx, "Regulation extraction quality: issues=%d patched=%v knowledge=%s",
			len(regulationIssues), service.PatchedFieldNames(regulationPatches), knowledgeID)
	}

	meta := regulationCustomMetadata{
		Kind:           extracted.Kind,
		Regulations:    extracted.Regulations,
		ExtractStatus:  "success",
		ExtractError:   extracted.ExtractError,
		ExtractIssues:  regulationIssues,
		PatchedByRegex: service.PatchedFieldNames(regulationPatches),
		RegexPatches:   regulationPatches,
	}
	// 自定义识别规则：制度类型归类（用户可配置关键词/正则 → 制度类型），
	// 按模型提取出的制度类型字段匹配，命中则覆盖模型结果。
	if recCfg, rerr := h.kgService.GetRecognitionConfig(extractCtx, kbID); rerr == nil {
		for i := range meta.Regulations {
			if t := service.ClassifyTypeFromRules(meta.Regulations[i].RegType, recCfg); t != "" {
				meta.Regulations[i].RegType = t
			}
		}
	}
	// 用户归档分类绝对优先（六同步核心）：上传弹窗选择的分类绝对优先于模型/识别规则结果，
	// 同文件所有制度都归到该分类；certType 为空时按 enabled 分类做隔离/降级
	//（与发票/合同 applyUserCategoryPrecedence 口径一致）。
	h.applyRegulationUserCategoryPrecedence(extractCtx, &meta, certType)
	if extracted.Kind == "not_regulation" {
		// 自定义识别规则捞回：模型判非但包含规则命中 → 认定为制度，置 manual 待补录，
		// 不自动删除（解决"该是制度却没入库"的误判）。
		if recCfg, rerr := h.kgService.GetRecognitionConfig(extractCtx, kbID); rerr == nil {
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
				return gin.H{
					"success": true,
					"message": "识别规则命中，已认定为制度，请在列表中编辑补录字段",
					"data":    map[string]interface{}{"kind": "regulation", "extract_status": "manual", "removed": false},
				}, nil
			}
		}
		meta.ExtractStatus = "not_regulation"
		if meta.ExtractError == "" {
			meta.ExtractError = "document does not look like a regulation"
		}
		// 防循环：auto_deleted_count >= 1 说明该文件曾被自动删除后由用户从删除历史
		// 恢复，再次判定非制度不再自动删除，改为 failed 保留（避免 删除→恢复→删除 死循环）。
		if h.autoDeleteCount(extractCtx, knowledge) > 0 {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "系统判定非制度文件，但该文件已被人工恢复，已保留待人工处理"
			logger.Warnf(ctx, "Regulation re-extraction judged non-regulation but row was restored before; keeping file %s", knowledgeID)
		} else {
			// 非制度文件不能存在于制度知识库 → 提取判定后自动删除该文件（保留物理文件，
			// 写入删除历史，可在"删除历史"抽屉中查看/恢复/永久删除）
			if delErr := h.kgService.AutoDeleteKnowledge(extractCtx, knowledgeID, "not_regulation"); delErr != nil {
				logger.Warnf(ctx, "auto-delete non-regulation knowledge failed: %v", delErr)
			} else {
				logger.Infof(ctx, "auto-deleted non-regulation knowledge, ID: %s", knowledgeID)
				return gin.H{
					"success": true,
					"message": "非制度文件已移至删除历史，可在删除历史中恢复",
					"data":    map[string]interface{}{"removed": true, "kind": "not_regulation"},
				}, nil
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
		return nil, errors.NewInternalServerError("failed to encode extraction result")
	}
	// 最终保存合并而非覆盖：序列化本次新 meta 后与 knowledge.CustomMetadata 旧值合并
	//（保留 auto_deleted_count 等本次新 meta 不含的字段），再用 persistCtx 落库；
	// 前端超时/取消请求后仍可写入（与发票/合同最终保存口径一致）。
	if len(knowledge.CustomMetadata) > 0 {
		var existing map[string]interface{}
		if uerr := json.Unmarshal(knowledge.CustomMetadata, &existing); uerr == nil {
			var fresh map[string]interface{}
			if uerr := json.Unmarshal(metaJSON, &fresh); uerr == nil {
				for k, v := range fresh {
					existing[k] = v
				}
				if merged, merr := json.Marshal(existing); merr == nil {
					metaJSON = merged
				}
			}
		}
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, types.JSON(metaJSON)); err != nil {
		logger.Error(ctx, "Failed to persist regulation extraction result", err)
		return nil, errors.NewInternalServerError("failed to save extraction result: " + err.Error())
	}

	logger.Infof(ctx, "Regulation extraction succeeded, knowledge ID: %s, kind: %s, regulations: %d",
		knowledgeID, meta.Kind, len(meta.Regulations))
	if meta.ExtractStatus == "failed" {
		logger.Warnf(ctx, "Regulation extraction returned an unusable result, knowledge ID: %s, err: %s", knowledgeID, meta.ExtractError)
		return nil, errors.NewInternalServerError(meta.ExtractError)
	}
	return gin.H{
		"success": true,
		"message": "Regulation extraction succeeded",
		"data": map[string]interface{}{
			"knowledge_id":   knowledgeID,
			"kind":           meta.Kind,
			"extract_status": meta.ExtractStatus,
			"regulations":    meta.Regulations,
		},
	}, nil
}

// ExtractAwardPunish godoc
// @Summary      提取奖惩字段
// @Description  读取已解析文档文本，调用提取模型（复用知识库 summary_model_id）提取奖惩字段（按当事人拆分多条记录）并写入 custom_metadata。幂等：对同一知识重复调用会覆盖写。
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
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/extract-award-punish [post]

func (h *BusinessExtractHandler) ExtractAwardPunish(c *gin.Context, certType string) {
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
		c.Error(errors.NewForbiddenError("No permission to extract notice fields"))
		return
	}
	// 快速失败提示（真正防重入由 worker 端 extractMu 保证）：同一文档已有任务在跑时
	// 直接 409，避免连点重复投递。
	if _, loaded := h.extractMu.Load(knowledgeID); loaded {
		c.Error(errors.NewConflictError("extraction already in progress for this document").
			WithDetails(extractInProgressDetails(knowledgeID, "award_punish")))
		return
	}
	taskID, err := h.enqueueBusinessExtract(ctx, effectiveTenantID, kbID, knowledgeID, "award_punish", certType)
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue notice extraction for %s: %v", knowledgeID, err)
		c.Error(errors.NewInternalServerError("enqueue notice extraction failed: " + err.Error()))
		return
	}
	logger.Infof(ctx, "Notice extraction enqueued: knowledge=%s task=%s", knowledgeID, taskID)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Notice extraction submitted",
		"data": map[string]interface{}{
			"knowledge_id":   knowledgeID,
			"kind":           "award_punish",
			"extract_status": "processing",
			"task_id":        taskID,
		},
	})
}

// runAwardPunishCore 是不依赖 gin 的通知公告提取核心执行器（C 方案：HTTP handler 投递即返，
// asynq worker 调用本方法执行原同步提取逻辑）。响应改为返回值/error，由调用方记录；
// 失败状态照旧经 persistCtx 落库，模型调用失败不再因 HTTP 超时被连带掐断。
func (h *BusinessExtractHandler) runAwardPunishCore(ctx context.Context, kbID, knowledgeID, certType string, effectiveTenantID uint64) (gin.H, error) {
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	// persistCtx 脱离任务/请求生命周期：任务超时/取消后仍可落库提取结果或
	// failed 状态（否则 ctx 被取消，failed 状态写不进去，记录永久停在"提取中"）。
	persistCtx := context.WithValue(context.Background(), types.TenantIDContextKey, effectiveTenantID)
	// extractCtx 30 分钟硬上限：耗时的大模型调用不被任务取消连带掐断。
	extractCtx, extractCancel := newExtractionContext(effectiveTenantID)
	defer extractCancel()

	var knowledge *types.Knowledge
	// panic 兜底：提取协程任何 panic 都强制落 failed 状态（用独立 ctx），
	// 保证状态闭环，不允许永久转圈。
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf(ctx, "Notice extraction panic recovered: %v\n%s", r, debug.Stack())
			var panicPrev []byte
			if knowledge != nil {
				panicPrev = knowledge.CustomMetadata
			}
			failMeta := mergeFailedStateMeta(panicPrev, "award_punish", fmt.Sprintf("notice extraction panic: %v", r))
			if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, failMeta); serr != nil {
				logger.Warnf(ctx, "Failed to persist panic failed-state: %v", serr)
			}
		}
	}()

	var err error
	knowledge, err = h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for notice extraction", err)
		return nil, errors.NewNotFoundError("Knowledge not found")
	}
	if knowledge.KnowledgeBaseID != kbID {
		return nil, errors.NewBadRequestError("Knowledge does not belong to the given knowledge base")
	}
	if businessExtractionInProgress(knowledge.ParseStatus) {
		return nil, errors.NewBadRequestError("document has not been parsed yet, please wait for parsing to finish")
	}
	// 待补录豁免：manual 记录由用户人工维护，不自动重提取、不参与任何自动删除判定。
	var curAPMeta awardPunishCustomMetadata
	_ = json.Unmarshal(knowledge.CustomMetadata, &curAPMeta)
	if curAPMeta.ExtractStatus == "manual" {
		logger.Infof(ctx, "Notice extraction skipped: knowledge %s is manual intake", knowledgeID)
		return gin.H{
			"success": true,
			"message": "待补录记录，无需重复提取",
			"data":    map[string]interface{}{"kind": "award_punish", "extract_status": "manual", "removed": false},
		}, nil
	}

	// worker 上下文没有 gin 侧的 kb 绑定：重新按 kbID 取知识库配置（仅需 summary_model_id）。
	kb, kberr := h.kbService.GetKnowledgeBaseByIDOnly(effCtx, kbID)
	if kberr != nil || kb == nil {
		return nil, errors.NewNotFoundError("Knowledge base not found")
	}
	modelID := strings.TrimSpace(kb.SummaryModelID)
	if modelID == "" {
		return nil, errors.NewBadRequestError("no extraction model configured for the knowledge base")
	}
	chatModel, err := h.modelService.GetChatModel(extractCtx, modelID)
	if err != nil {
		logger.Error(ctx, "Failed to load extraction model", err)
		return nil, errors.NewInternalServerError("get extraction model failed: " + err.Error())
	}

	chunks, err := h.chunkService.ListChunksByKnowledgeID(extractCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to list chunks for notice extraction", err)
		return nil, errors.NewInternalServerError("list chunks failed: " + err.Error())
	}
	batches := service.BuildAwardPunishExtractionBatches(knowledge.FileName, knowledge.Description, chunks, 0)
	// 扫描件防御：无文本层（扫描件 PDF / 纯图片）时不要自动删除，保留文件并标记
	// failed 让用户人工处理（配合删除历史）。
	if !awardPunishBatchesHaveText(batches) {
		zhNotice, enNotice := noExtractableTextNotice(knowledge.ParseStatus)
		noTextMeta := mergeFailedStateMeta(knowledge.CustomMetadata, "award_punish", zhNotice)
		if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, noTextMeta); serr != nil {
			logger.Warnf(ctx, "Failed to persist notice no-text state: %v", serr)
		}
		logger.Warnf(ctx, "Notice extraction skipped: no text layer for knowledge %s (parse_status=%s), file kept", knowledgeID, knowledge.ParseStatus)
		return gin.H{
			"success": true,
			"message": enNotice,
			"data":    map[string]interface{}{"kind": "award_punish", "extract_status": "failed", "removed": false},
		}, nil
	}
	merged := &service.AwardPunishExtractionResult{Kind: "award_punish"}
	var extractErr error
	sawRecord := false
	// 提取进行中标记：先落 processing 中间态，让前端轮询/状态条与表格状态列实时显示
	//「正在提取」（避免点重新提取后状态列短暂停留在旧值/failed）。合并保留既有字段
	//（fleet_cert_type、旧 records 等），提取成功/失败路径会用终态整体覆盖。
	procMeta := map[string]interface{}{"kind": "award_punish", "extract_status": "processing"}
	if len(knowledge.CustomMetadata) > 0 {
		var prev map[string]interface{}
		if jerr := json.Unmarshal(knowledge.CustomMetadata, &prev); jerr == nil {
			for k, v := range prev {
				if _, exists := procMeta[k]; !exists {
					procMeta[k] = v
				}
			}
		}
	}
	delete(procMeta, "extract_error")
	if pmeta, jerr := json.Marshal(procMeta); jerr == nil {
		if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, types.JSON(pmeta)); serr != nil {
			logger.Warnf(ctx, "Failed to persist notice processing-state: %v", serr)
		}
	}
	// 规则解析：优先请求携带的归档类型（上传弹窗 body.cert_type），未携带时回退文件级
	// 打标 fleet_cert_type，再按 resolveNoticeRuleCfg 回退整体规则。
	ruleCertType := certType
	if ruleCertType == "" && len(knowledge.CustomMetadata) > 0 {
		var mdMeta struct {
			FleetCertType string `json:"fleet_cert_type"`
		}
		if jerr := json.Unmarshal(knowledge.CustomMetadata, &mdMeta); jerr == nil && mdMeta.FleetCertType != "" {
			ruleCertType = mdMeta.FleetCertType
		}
	}
	// 用户配置的提取规则（scope=award_punish）：按归档分类匹配，未配置时回退整体规则
	noticeRuleCfg := h.resolveNoticeRuleCfg(extractCtx, kbID, ruleCertType)
	for i, batch := range batches {
		if strings.TrimSpace(batch) == "" {
			continue
		}
		// measures 传 nil：通知公告的「处理决定」为自由文本，不再约束到固定措施库
		batchRes, berr := service.ExtractAwardPunishesFromContentWithRules(extractCtx, chatModel, batch, nil, noticeRuleCfg)
		if berr != nil {
			if i == 0 && len(merged.Records) == 0 {
				extractErr = berr
				break
			}
			logger.Warnf(ctx, "Notice extraction batch %d failed (non-fatal): %v", i+1, berr)
			continue
		}
		if batchRes == nil || batchRes.Kind == "not_award_punish" {
			continue
		}
		sawRecord = true
		merged.Records = append(merged.Records, batchRes.Records...)
	}
	if !sawRecord {
		merged.Kind = "not_award_punish"
	}
	if extractErr != nil {
		failMeta := mergeFailedStateMeta(knowledge.CustomMetadata, "award_punish", "notice extraction failed: "+extractErr.Error())
		if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, failMeta); serr != nil {
			logger.Warnf(ctx, "Failed to persist notice extraction failed-state: %v", serr)
		}
		logger.Error(ctx, "Notice extraction model call failed", extractErr)
		return nil, errors.NewInternalServerError("notice extraction failed: " + extractErr.Error())
	}
	extracted := merged
	// 自动编号去重：无文号的通知按当天已有最大序号 +1 顺序递增（跨文件唯一）
	autoSeq, seqErr := h.businessSvc.MaxAutoAwardPunishSeq(extractCtx, kbID)
	if seqErr != nil {
		logger.Warnf(ctx, "Failed to compute auto notice seq for %s: %v", kbID, seqErr)
	}
	service.NormalizeAwardPunishExtractionResult(extracted, autoSeq)
	// 通用质量闭环（非发票业务口径）：日期归一化（签发/生效日）+ 配置级必填补洞/告警。
	noticeIssues, noticePatches := service.ValidateGenericExtraction(
		service.NewGenericRecords(extracted.Records), strings.Join(batches, "\n"), noticeRuleCfg)
	if len(noticeIssues) > 0 || len(noticePatches) > 0 {
		logger.Infof(ctx, "Notice extraction quality: issues=%d patched=%v knowledge=%s",
			len(noticeIssues), service.PatchedFieldNames(noticePatches), knowledgeID)
	}

	meta := awardPunishCustomMetadata{
		Kind:           extracted.Kind,
		Records:        extracted.Records,
		ExtractStatus:  "success",
		ExtractError:   extracted.ExtractError,
		ExtractIssues:  noticeIssues,
		PatchedByRegex: service.PatchedFieldNames(noticePatches),
		RegexPatches:   noticePatches,
	}
	// 自定义识别规则：通知类型归类（用户可配置关键词/正则 → 通知类型），
	// 按模型提取出的通知类型字段匹配，命中则覆盖模型结果。
	if recCfg, rerr := h.kgService.GetRecognitionConfig(extractCtx, kbID); rerr == nil {
		for i := range meta.Records {
			if t := service.ClassifyTypeFromRules(meta.Records[i].ApType, recCfg); t != "" {
				meta.Records[i].ApType = t
			}
		}
	}
	// 用户归档分类绝对优先（六同步核心）：上传弹窗选择的分类绝对优先于模型/识别规则结果，
	// 同文件所有公告都归到该分类；certType 为空时按 enabled 分类做隔离/降级
	//（与发票/合同/制度 applyUserCategoryPrecedence 口径一致）。
	h.applyNoticeUserCategoryPrecedence(extractCtx, &meta, certType)
	if extracted.Kind == "not_award_punish" {
		// 自定义识别规则捞回：模型判非但包含规则命中 → 认定为通知公告，置 manual 待补录，
		// 不自动删除（解决"该是通知公告却没入库"的误判）。
		if recCfg, rerr := h.kgService.GetRecognitionConfig(extractCtx, kbID); rerr == nil {
			if service.MatchIncludeRules(strings.Join(batches, "\n"), recCfg) {
				meta.Kind = "award_punish"
				meta.Records = []types.AwardPunishExtractionItem{}
				meta.ExtractStatus = "manual"
				meta.ExtractError = "识别规则命中，已认定为通知公告，请编辑补录字段"
				if raw, merr := json.Marshal(meta); merr == nil {
					if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, types.JSON(raw)); serr != nil {
						logger.Warnf(ctx, "Failed to persist rule-rescued notice meta: %v", serr)
					}
				}
				logger.Infof(ctx, "Notice rule-rescued by include rule, knowledge %s kept as manual", knowledgeID)
				return gin.H{
					"success": true,
					"message": "识别规则命中，已认定为通知公告，请在列表中编辑补录字段",
					"data":    map[string]interface{}{"kind": "award_punish", "extract_status": "manual", "removed": false},
				}, nil
			}
		}
		meta.ExtractStatus = "not_award_punish"
		if meta.ExtractError == "" {
			meta.ExtractError = "document does not look like a notice/announcement"
		}
		// 防循环：auto_deleted_count >= 1 说明该文件曾被自动删除后由用户从删除历史
		// 恢复，再次判定非通知公告不再自动删除，改为 failed 保留（避免 删除→恢复→删除 死循环）。
		if h.autoDeleteCount(extractCtx, knowledge) > 0 {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "系统判定非通知公告文件，但该文件已被人工恢复，已保留待人工处理"
			logger.Warnf(ctx, "Notice re-extraction judged non-notice but row was restored before; keeping file %s", knowledgeID)
		} else {
			// 非通知公告文件不能存在于通知公告知识库 → 提取判定后自动删除该文件（保留物理文件，
			// 写入删除历史，可在"删除历史"抽屉中查看/恢复/永久删除）
			if delErr := h.kgService.AutoDeleteKnowledge(extractCtx, knowledgeID, "not_award_punish"); delErr != nil {
				logger.Warnf(ctx, "auto-delete non-notice knowledge failed: %v", delErr)
			} else {
				logger.Infof(ctx, "auto-deleted non-notice knowledge, ID: %s", knowledgeID)
				return gin.H{
					"success": true,
					"message": "非通知公告文件已移至删除历史，可在删除历史中恢复",
					"data":    map[string]interface{}{"removed": true, "kind": "not_award_punish"},
				}, nil
			}
		}
	}
	// 空提取守卫：标记为通知公告但没有可用记录（空数组或全部空字段）→ failed 可重试
	if meta.Kind == "award_punish" {
		if len(meta.Records) == 0 {
			meta.ExtractStatus = "failed"
			if meta.ExtractError == "" {
				meta.ExtractError = "提取未返回任何通知公告记录，请重试"
			}
		} else if service.AwardPunishRecordsAllEmpty(meta.Records) {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "提取结果字段为空，请重试"
		}
	}

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, errors.NewInternalServerError("failed to encode extraction result")
	}
	// 最终保存合并而非覆盖：序列化本次新 meta 后与 knowledge.CustomMetadata 旧值合并
	//（保留 auto_deleted_count 等本次新 meta 不含的字段），再用 persistCtx 落库；
	// 前端超时/取消请求后仍可写入（与发票/合同/制度最终保存口径一致）。
	if len(knowledge.CustomMetadata) > 0 {
		var existing map[string]interface{}
		if uerr := json.Unmarshal(knowledge.CustomMetadata, &existing); uerr == nil {
			var fresh map[string]interface{}
			if uerr := json.Unmarshal(metaJSON, &fresh); uerr == nil {
				for k, v := range fresh {
					existing[k] = v
				}
				if merged, merr := json.Marshal(existing); merr == nil {
					metaJSON = merged
				}
			}
		}
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, types.JSON(metaJSON)); err != nil {
		logger.Error(ctx, "Failed to persist notice extraction result", err)
		return nil, errors.NewInternalServerError("failed to save extraction result: " + err.Error())
	}

	logger.Infof(ctx, "Notice extraction succeeded, knowledge ID: %s, kind: %s, records: %d",
		knowledgeID, meta.Kind, len(meta.Records))
	if meta.ExtractStatus == "failed" {
		logger.Warnf(ctx, "Notice extraction returned an unusable result, knowledge ID: %s, err: %s", knowledgeID, meta.ExtractError)
		return nil, errors.NewInternalServerError(meta.ExtractError)
	}
	return gin.H{
		"success": true,
		"message": "Notice extraction succeeded",
		"data": map[string]interface{}{
			"knowledge_id":   knowledgeID,
			"kind":           meta.Kind,
			"extract_status": meta.ExtractStatus,
			"records":        meta.Records,
		},
	}, nil
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

func (h *BusinessExtractHandler) ListDeletedKnowledge(c *gin.Context) {
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
	page, pageSize := parseSandboxPagination(c)
	result, _, err := h.businessSvc.ListDeletedKnowledge(effCtx, effectiveTenantID, kbID, page, pageSize, c.Query("q"))
	if err != nil {
		logger.Error(ctx, "Failed to list deleted knowledge", err)
		c.Error(errors.NewInternalServerError("list deleted knowledge failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
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

func (h *BusinessExtractHandler) RestoreDeletedKnowledge(c *gin.Context) {
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
	if err := h.businessSvc.RestoreDeletedKnowledge(effCtx, effectiveTenantID, knowledgeID); err != nil {
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

func (h *BusinessExtractHandler) PurgeDeletedKnowledge(c *gin.Context) {
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
	if err := h.businessSvc.PurgeDeletedKnowledge(effCtx, effectiveTenantID, knowledgeID); err != nil {
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

// PreviewDeletedKnowledgeFile 预览已删除（删除历史）知识文件的源文件。
// 返回被系统自动删除（软删、保留物理源文件）的知识条目关联的原始文件，供删除历史抽屉查看/下载。
func (h *BusinessExtractHandler) PreviewDeletedKnowledgeFile(c *gin.Context) {
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
		c.Error(errors.NewForbiddenError("No permission to preview deleted knowledge"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	file, filename, ferr := h.kgService.GetDeletedKnowledgeFile(effCtx, knowledgeID)
	if ferr != nil {
		logger.Error(ctx, "Failed to retrieve deleted knowledge file", ferr)
		c.Error(errors.NewInternalServerError("Failed to retrieve file").WithDetails(ferr.Error()))
		return
	}
	if file == nil {
		c.Error(errors.NewNotFoundError("deleted knowledge file not found"))
		return
	}
	defer file.Close()
	if err := filetransport.Serve(c.Writer, c.Request, file, filetransport.Options{
		Filename: filename, CacheControl: "private, max-age=3600",
	}); err != nil {
		logger.Errorf(ctx, "Failed to stream deleted knowledge preview: %v", err)
	}
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

// checkDuplicateNo 校验本次保存的业务元数据中的单号（发票号码/合同编号）是否已在
// 同一知识库的其它记录中存在。force=true 时跳过（前端二次确认后强制保存）。
// 返回命中的重复单号、单号语义标签（用于冲突码与提示）与查询错误；无重复时 dupNo 为空。
func (h *BusinessExtractHandler) checkDuplicateNo(ctx context.Context, kbID, knowledgeID string, meta map[string]interface{}, force bool) (dupNo, label string, err error) {
	if force {
		return "", "", nil
	}
	tenantID, _ := ctx.Value(types.TenantIDContextKey).(uint64)
	var field, arrKey string
	if _, ok := meta["invoices"].([]interface{}); ok {
		field = "invoice_no"
		arrKey = "invoices"
		label = "发票号码"
	} else if _, ok := meta["contracts"].([]interface{}); ok {
		field = "contract_no"
		arrKey = "contracts"
		label = "合同编号"
	} else if _, ok := meta["regulations"].([]interface{}); ok {
		field = "reg_no"
		arrKey = "regulations"
		label = "制度编号"
	} else if _, ok := meta["records"].([]interface{}); ok {
		field = "ap_no"
		arrKey = "records"
		label = "奖惩文号"
	} else {
		return "", "", nil
	}
	// 提取本次保存的单号集合（非空）
	nos := map[string]bool{}
	for _, it := range mustArray(meta[arrKey]) {
		if m, ok := it.(map[string]interface{}); ok {
			if v, ok := m[field].(string); ok && strings.TrimSpace(v) != "" {
				nos[strings.TrimSpace(v)] = true
			}
		}
	}
	if len(nos) == 0 {
		return "", "", nil
	}
	// 查询同知识库下其它记录的 custom_metadata，逐个匹配单号
	var rows []struct {
		ID             string
		CustomMetadata types.JSON
	}
	if err := h.db.WithContext(ctx).Table("knowledges").
		Select("id, custom_metadata").
		Where("tenant_id = ? AND knowledge_base_id = ? AND id <> ? AND deleted_at IS NULL AND custom_metadata IS NOT NULL", tenantID, kbID, knowledgeID).
		Find(&rows).Error; err != nil {
		return "", "", err
	}
	for _, row := range rows {
		var m map[string]interface{}
		if err := json.Unmarshal(row.CustomMetadata, &m); err != nil {
			continue
		}
		for _, it := range mustArray(m[arrKey]) {
			if mm, ok := it.(map[string]interface{}); ok {
				if v, ok := mm[field].(string); ok && nos[strings.TrimSpace(v)] {
					return strings.TrimSpace(v), label, nil
				}
			}
		}
	}
	return "", "", nil
}

// mustArray 将 interface{} 安全归一为 []interface{}（nil/非数组返回空切片）。
func mustArray(v interface{}) []interface{} {
	if arr, ok := v.([]interface{}); ok {
		return arr
	}
	return nil
}

// UpdateInvoiceMetadata godoc
// @Summary      保存发票提取元数据
// @Description  直接持久化发票 custom_metadata（含 invoices 数组），绕过原生
// custom_metadata 标量校验，供发票详情抽屉手动保存使用。调用方需自带完整元数据。
// @Tags         日常事务
// @Accept       json
// @Produce      json
// @Param        id            path      string                 true  "知识库ID"
// @Param        knowledgeId   path      string                 true  "知识ID"
// @Param        request       body      map[string]interface{} true  "custom_metadata"
// @Success      200           {object}  map[string]interface{} "保存成功"
// @Failure      400           {object}  errors.AppError       "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/invoice-metadata [put]
func (h *BusinessExtractHandler) UpdateInvoiceMetadata(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	knowledgeID := secutils.SanitizeForLog(c.Param("knowledgeId"))
	if kbID == "" || knowledgeID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id and knowledge id cannot be empty"))
		return
	}
	var body struct {
		CustomMetadata json.RawMessage `json:"custom_metadata"`
		Force          bool            `json:"force"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	if len(body.CustomMetadata) == 0 || string(body.CustomMetadata) == "null" {
		c.Error(errors.NewBadRequestError("custom_metadata cannot be empty"))
		return
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(body.CustomMetadata, &meta); err != nil {
		c.Error(errors.NewBadRequestError("invalid custom_metadata: " + err.Error()))
		return
	}
	// 单号查重（发票号码/合同编号/制度编号/奖惩文号）：命中重复且未携带 force 时返回 409，
	// 前端据此弹「是否强制保存」二次确认后再以 force=true 重试。
	if dupNo, label, derr := h.checkDuplicateNo(ctx, kbID, knowledgeID, meta, body.Force); derr != nil {
		logger.Error(ctx, "Failed to check duplicate business number", derr)
		c.Error(errors.NewInternalServerError("failed to check duplicate business number: " + derr.Error()))
		return
	} else if dupNo != "" {
		code := "DUPLICATE_INVOICE_NO"
		switch label {
		case "合同编号":
			code = "DUPLICATE_CONTRACT_NO"
		case "制度编号":
			code = "DUPLICATE_REGULATION_NO"
		case "奖惩文号":
			code = "DUPLICATE_AWARDPUNISH_NO"
		}
		c.Error(errors.NewConflictError(code + ":已存在相同的" + label + " " + dupNo))
		return
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(ctx, knowledgeID, types.JSON(body.CustomMetadata)); err != nil {
		logger.Error(ctx, "Failed to persist invoice metadata", err)
		c.Error(errors.NewInternalServerError("failed to save invoice metadata: " + err.Error()))
		return
	}
	logger.Infof(ctx, "Invoice custom metadata updated, knowledge ID: %s", knowledgeID)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"id": knowledgeID}})
}

// ExtractBusinessDocument 统一业务文档提取入口：按 body.scope 分发到具体提取 Handler。
func (h *BusinessExtractHandler) ExtractBusinessDocument(c *gin.Context) {
	var body struct {
		Scope    string `json:"scope"`
		CertType string `json:"cert_type"`
	}
	_ = c.ShouldBindJSON(&body)
	switch body.Scope {
	case "contract":
		h.ExtractContract(c, body.CertType)
	case "invoice":
		h.ExtractInvoice(c, body.CertType)
	case "regulation":
		h.ExtractRegulation(c, body.CertType)
	case "award_punish":
		h.ExtractAwardPunish(c, body.CertType)
	default:
		c.Error(errors.NewBadRequestError("unknown business scope: " + body.Scope))
	}
}

// enqueueBusinessExtract 投递业务文档异步提取任务（asynq）。原同步链路模型调用可达
// 6 分钟（批次多发票合集 + MaxTokens 截断 + 上游排队），网关读超时直接 524 / 500；
// 投递后立即返回，worker 执行 core 提取逻辑并照旧落库，前端轮询 extract_status。
func (h *BusinessExtractHandler) enqueueBusinessExtract(
	ctx context.Context, tenantID uint64, kbID, knowledgeID, scope, certType string,
) (string, error) {
	payload := types.BusinessExtractPayload{
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
		KnowledgeID:     knowledgeID,
		Scope:           scope,
		CertType:        certType,
	}
	langfuse.InjectTracing(ctx, &payload)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	task := asynq.NewTask(types.TypeBusinessExtract, payloadBytes,
		asynq.Queue(types.QueueDefault), asynq.MaxRetry(2), asynq.Timeout(extractionTimeout))
	info, err := h.asynqClient.Enqueue(task)
	if err != nil {
		return "", fmt.Errorf("enqueue task: %w", err)
	}
	return info.ID, nil
}

// ProcessBusinessExtract 是 asynq worker 入口：从 bare context 解析 payload 携带的
// 租户/知识库/知识/scope/certType，执行不依赖 gin 的提取核心（响应改为返回值，
// 由调用方记录而非序列化到 HTTP），失败状态照旧落库。
func (h *BusinessExtractHandler) ProcessBusinessExtract(ctx context.Context, t *asynq.Task) error {
	var p types.BusinessExtractPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		logger.Errorf(ctx, "failed to unmarshal business extract payload: %v", err)
		return err
	}
	ctx = context.WithValue(ctx, types.TenantIDContextKey, p.TenantID)
	logger.Infof(ctx, "Business extract task started: scope=%s knowledge=%s kb=%s", p.Scope, p.KnowledgeID, p.KnowledgeBaseID)
	// 防重入：同一文档已有任务在跑（含 HTTP 投递后尚未完成）时跳过，避免重复烧 token。
	if _, loaded := h.extractMu.LoadOrStore(p.KnowledgeID, struct{}{}); loaded {
		logger.Infof(ctx, "Business extract skipped: extraction already in progress for %s", p.KnowledgeID)
		return nil
	}
	defer h.extractMu.Delete(p.KnowledgeID)

	var resp gin.H
	var runErr error
	switch p.Scope {
	case "contract":
		resp, runErr = h.runContractCore(ctx, p.KnowledgeBaseID, p.KnowledgeID, p.CertType, p.TenantID)
	case "invoice":
		resp, runErr = h.runInvoiceCore(ctx, p.KnowledgeBaseID, p.KnowledgeID, p.CertType, p.TenantID)
	case "regulation":
		resp, runErr = h.runRegulationCore(ctx, p.KnowledgeBaseID, p.KnowledgeID, p.CertType, p.TenantID)
	case "award_punish":
		resp, runErr = h.runAwardPunishCore(ctx, p.KnowledgeBaseID, p.KnowledgeID, p.CertType, p.TenantID)
	default:
		runErr = errors.NewInternalServerError("unknown business scope: " + p.Scope)
	}
	if runErr != nil {
		// 失败状态已由 core 内 mergeFailedStateMeta 落库（failed），保证 UI 不再长时间
		// 停在「提取中」。瞬时模型故障（HTTP 5xx / 超时 / 网络瞬断等）返回 error 交给
		// asynq 按投递时的 MaxRetry(2) 自动重试（重试成功会把状态覆盖为 success）；
		// 业务性失败（无文本 / 未识别为发票 / 解析错误 / panic）返回 nil 不重试烧 token。
		logger.Errorf(ctx, "Business extract task failed: scope=%s knowledge=%s err=%v", p.Scope, p.KnowledgeID, runErr)
		if shouldRetryBusinessExtract(runErr) && retryBudgetLeft(ctx) {
			retried, _ := asynq.GetRetryCount(ctx)
			logger.Warnf(ctx, "Business extract task will be retried by asynq: scope=%s knowledge=%s retried=%d", p.Scope, p.KnowledgeID, retried)
			return runErr
		}
		return nil
	}
	logger.Infof(ctx, "Business extract task succeeded: scope=%s knowledge=%s result=%v", p.Scope, p.KnowledgeID, resp)
	return nil
}

// shouldRetryBusinessExtract 判断提取失败是否属于「可自动重试的瞬时错误」：
// 模型 API HTTP 5xx、请求超时、网络瞬断等重试有实际成功概率；业务性失败
// （无文本、未识别为发票、解析错误、panic）不重试烧 token。
func shouldRetryBusinessExtract(runErr error) bool {
	if runErr == nil {
		return false
	}
	msg := runErr.Error()
	lowered := strings.ToLower(msg)
	if strings.Contains(msg, "status 5") || strings.Contains(lowered, "http 5") {
		return true
	}
	if strings.Contains(lowered, "api request failed") ||
		strings.Contains(lowered, "context deadline exceeded") ||
		strings.Contains(lowered, "client.timeout") ||
		strings.Contains(lowered, "connection refused") ||
		strings.Contains(lowered, "read tcp") ||
		strings.Contains(lowered, "write tcp") ||
		strings.Contains(lowered, "temporary failure in name resolution") {
		return true
	}
	return false
}

// retryBudgetLeft 返回 asynq 是否仍有余量自动重试（已重试次数 < MaxRetry）。
func retryBudgetLeft(ctx context.Context) bool {
	retried, ok := asynq.GetRetryCount(ctx)
	if !ok {
		return false
	}
	max, ok2 := asynq.GetMaxRetry(ctx)
	if !ok2 {
		return false
	}
	return retried < max
}
