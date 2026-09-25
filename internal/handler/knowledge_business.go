package handler

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// BusinessExtractHandler 承载 WeKnora 二次开发中「日常事务」模块的业务提取与记录管理。
type BusinessExtractHandler struct {
	*KnowledgeHandler
	db           *gorm.DB
	modelService interfaces.ModelService
	businessSvc  *service.BusinessExtractService
}

// NewBusinessExtractHandler 构造业务提取 Handler。
func NewBusinessExtractHandler(kh *KnowledgeHandler, db *gorm.DB, ms interfaces.ModelService, bsvc *service.BusinessExtractService) *BusinessExtractHandler {
	return &BusinessExtractHandler{KnowledgeHandler: kh, db: db, modelService: ms, businessSvc: bsvc}
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