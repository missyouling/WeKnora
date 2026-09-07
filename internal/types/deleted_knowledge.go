package types

// DeletedKnowledgeItem is one auto-deleted (not_contract / not_invoice)
// knowledge row surfaced in the "删除历史" drawer. The physical file is kept
// so the row can be previewed and restored without re-uploading.
type DeletedKnowledgeItem struct {
	ID              string `json:"id"`
	KnowledgeBaseID string `json:"knowledge_base_id"`
	Title           string `json:"title"`
	FileName        string `json:"file_name"`
	FileType        string `json:"file_type"`
	FileSize        int64  `json:"file_size"`
	FileHash        string `json:"file_hash"`
	FilePath        string `json:"file_path"`
	// Reason is the auto-delete cause recorded at delete time:
	// "not_contract" | "not_invoice".
	Reason string `json:"reason"`
	// DeleteCount is how many times this row has been auto-deleted (0 on
	// first auto-delete, 1 after one restore+re-delete cycle, ...). It powers
	// the anti-loop guard: restoring a row does not reset the count, so a
	// re-extraction that judges the document not-a-contract again keeps the
	// row instead of auto-deleting it once more.
	DeleteCount int    `json:"delete_count"`
	DeletedAt   string `json:"deleted_at"`
	CreatedAt   string `json:"created_at"`
}

// DeletedKnowledgePage is the paginated result of ListDeletedKnowledge.
type DeletedKnowledgePage struct {
	Data     []DeletedKnowledgeItem `json:"data"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
	Success  bool                   `json:"success"`
}
