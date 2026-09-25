package handler

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// BusinessExtractHandler 承载 WeKnora 二次开发中「日常事务」模块的业务提取与记录管理
// 接口（合同/发票/制度/奖惩/电费/光伏/车队证照等）。
type BusinessExtractHandler struct {
	*KnowledgeHandler
	db           *gorm.DB
	modelService interfaces.ModelService
}

// NewBusinessExtractHandler 构造业务提取 Handler。
func NewBusinessExtractHandler(kh *KnowledgeHandler, db *gorm.DB, ms interfaces.ModelService) *BusinessExtractHandler {
	return &BusinessExtractHandler{KnowledgeHandler: kh, db: db, modelService: ms}
}

