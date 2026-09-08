package interfaces

import (
	"context"
	"io"
	"mime/multipart"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/hibiken/asynq"
)

// KnowledgeService defines the interface for knowledge services.
type KnowledgeService interface {
	// CreateKnowledgeFromFile creates knowledge from a file.
	// channel identifies the ingestion channel (e.g. "web", "api", "wechat"); empty defaults to "web".
	CreateKnowledgeFromFile(
		ctx context.Context,
		kbID string,
		file *multipart.FileHeader,
		metadata map[string]string,
		enableMultimodel *bool,
		customFileName string,
		tagIDs []string,
		channel string,
		processOverrides *types.KnowledgeProcessOverrides,
	) (*types.Knowledge, error)
	// CreateKnowledgeFromURL creates knowledge from a URL.
	// When fileName or fileType is provided (or the URL path has a known file extension),
	// the URL is treated as a direct file download instead of a web page crawl.
	// channel identifies the ingestion channel; empty defaults to "web".
	CreateKnowledgeFromURL(
		ctx context.Context,
		kbID string,
		url string,
		fileName string,
		fileType string,
		enableMultimodel *bool,
		title string,
		tagIDs []string,
		channel string,
		processOverrides *types.KnowledgeProcessOverrides,
	) (*types.Knowledge, error)
	// CreateKnowledgeFromPassage creates knowledge from text passages.
	// channel identifies the ingestion channel; empty defaults to "web".
	CreateKnowledgeFromPassage(ctx context.Context, kbID string, passage []string, channel string) (*types.Knowledge, error)
	// CreateKnowledgeFromPassageSync creates knowledge from text passages and waits until chunks are indexed.
	CreateKnowledgeFromPassageSync(ctx context.Context, kbID string, passage []string, channel string) (*types.Knowledge, error)
	// CreateKnowledgeFromManual creates or saves manual Markdown knowledge content.
	// channel identifies the ingestion channel; empty defaults to "web".
	CreateKnowledgeFromManual(
		ctx context.Context,
		kbID string,
		payload *types.ManualKnowledgePayload,
		channel string,
	) (*types.Knowledge, error)
	// GetKnowledgeByID retrieves knowledge by ID (uses tenant from context).
	GetKnowledgeByID(ctx context.Context, id string) (*types.Knowledge, error)
	// GetKnowledgeByIDOnly retrieves knowledge by ID without tenant filter (for permission resolution).
	GetKnowledgeByIDOnly(ctx context.Context, id string) (*types.Knowledge, error)
	// GetOwningKBCreatorID resolves a knowledge ID to the CreatorID of its
	// owning KnowledgeBase, scoped to the caller's tenant. Used by the
	// per-KB ownership lookups in handler/rbac_lookups.go (PR 5, #1303) so
	// chunk and knowledge sub-resource routes can inherit the same
	// "creator-of-the-KB OR Admin+" gate that KB-level routes already use.
	// Returns the underlying repository sentinel errors unchanged so
	// callers can map them to middleware.ErrResourceNotFound.
	GetOwningKBCreatorID(ctx context.Context, knowledgeID string) (string, error)
	// GetKnowledgeBatch retrieves a batch of knowledge by IDs.
	GetKnowledgeBatch(ctx context.Context, tenantID uint64, ids []string) ([]*types.Knowledge, error)
	// GetKnowledgeBatchWithSharedAccess retrieves knowledge by IDs including items from shared KBs the user has access to.
	GetKnowledgeBatchWithSharedAccess(ctx context.Context, tenantID uint64, ids []string) ([]*types.Knowledge, error)
	// ListKnowledgeIDsByTagIDs returns document knowledge IDs carrying any of
	// the specified KB-local tags.
	ListKnowledgeIDsByTagIDs(ctx context.Context, tenantID uint64, kbID string, tagIDs []string) ([]string, error)
	// ListKnowledgeByKnowledgeBaseID lists all knowledge under a knowledge base.
	ListKnowledgeByKnowledgeBaseID(ctx context.Context, kbID string) ([]*types.Knowledge, error)
	// ListPagedKnowledgeByKnowledgeBaseID lists all knowledge under a knowledge base
	// with pagination. The filter struct controls optional dimensions (tag, keyword,
	// file type, parse status, source channel, updated time range); pass a zero
	// struct to disable all filters.
	ListPagedKnowledgeByKnowledgeBaseID(
		ctx context.Context,
		kbID string,
		page *types.Pagination,
		filter types.KnowledgeListFilter,
	) (*types.PageResult, error)
	// ListInvoiceRecords returns the invoice-level aggregated list of a knowledge
	// base: every extracted invoice flattened into its own row, de-duplicated by
	// invoice number (keeps the newest source), filtered/searched/sorted/paginated
	// server-side with amount aggregates.
	ListInvoiceRecords(ctx context.Context, kbID string, filter types.InvoiceListFilter) (*types.InvoiceListResult, error)
	// ListInvoiceTaxRates returns the distinct tax rates that appear across all
	// invoices in a knowledge base (invoice-level rate + all item-level rates),
	// powering the tax-rate filter dropdown.
	ListInvoiceTaxRates(ctx context.Context, kbID string) ([]types.InvoiceTaxRateCount, error)
	// ListContractRecords returns the contract-level aggregated list of a knowledge
	// base: every extracted contract flattened into its own row (de-duplication is
	// handled at the knowledge-file upload layer by file hash), filtered/searched/
	// sorted/paginated server-side with contract-amount aggregates.
	ListContractRecords(ctx context.Context, kbID string, filter types.ContractListFilter) (*types.ContractListResult, error)
	// ListContractTypes returns the distinct contract types that appear across
	// all contracts in a knowledge base, powering the contract-type filter dropdown.
	ListContractTypes(ctx context.Context, kbID string) ([]types.ContractTypeCount, error)
	// MaxAutoContractSeq returns the highest trailing sequence number among
	// auto-generated contract numbers (HT-YYYYMMDD-NNN) for the current date in
	// the knowledge base, so newly generated numbers stay unique across files.
	MaxAutoContractSeq(ctx context.Context, kbID string) (int, error)
	// ListRegulationRecords returns the regulation-level aggregated list of a
	// knowledge base: every extracted regulation flattened into its own row
	// (de-duplication is handled at the knowledge-file upload layer by file
	// hash), filtered/searched/sorted/paginated server-side.
	ListRegulationRecords(ctx context.Context, kbID string, filter types.RegulationListFilter) (*types.RegulationListResult, error)
	// ListRegulationTypes returns the distinct regulation types that appear
	// across all regulations in a knowledge base, powering the regulation-type
	// filter dropdown.
	ListRegulationTypes(ctx context.Context, kbID string) ([]types.RegulationTypeCount, error)
	// MaxAutoRegulationSeq returns the highest trailing sequence number among
	// auto-generated regulation numbers (ZD-YYYYMMDD-NNN) for the current date in
	// the knowledge base, so newly generated numbers stay unique across files.
	MaxAutoRegulationSeq(ctx context.Context, kbID string) (int, error)
	// ListAwardPunishRecords returns the award/punish-level aggregated list of a
	// knowledge base: every extracted record (one per person) flattened into its
	// own row (de-duplication is handled at the knowledge-file upload layer by
	// file hash), filtered/searched/sorted/paginated server-side.
	ListAwardPunishRecords(ctx context.Context, kbID string, filter types.AwardPunishListFilter) (*types.AwardPunishListResult, error)
	// ListAwardPunishTypes returns the distinct award/punish types that appear
	// across all records in a knowledge base, powering the type filter dropdown.
	ListAwardPunishTypes(ctx context.Context, kbID string) ([]types.AwardPunishTypeCount, error)
	// ListUtilityBillRecords returns the utility-bill aggregated list of a
	// knowledge base: one row per extracted bill (de-duplication handled at the
	// knowledge-file upload layer by file hash), filtered/searched/sorted/
	// paginated server-side.
	ListUtilityBillRecords(ctx context.Context, kbID string, filter types.UtilityBillListFilter) (*types.UtilityBillListResult, error)
	// MaxAutoAwardPunishSeq returns the highest trailing sequence number among
	// auto-generated 文号 (JC-YYYYMMDD-NNN) for the current date in the knowledge
	// base, so newly generated numbers stay unique across files.
	MaxAutoAwardPunishSeq(ctx context.Context, kbID string) (int, error)
	// GetRecognitionConfig returns the KB-level document recognition rules
	// (include-judgement + type classification). Invoice KBs fall back to the
	// built-in keyword rules when none are stored yet.
	GetRecognitionConfig(ctx context.Context, kbID string) (*types.RecognitionConfig, error)
	// SaveRecognitionConfig persists the KB-level recognition rules.
	SaveRecognitionConfig(ctx context.Context, kbID string, cfg *types.RecognitionConfig) error
	// ReassessRecognition re-runs include-judgement rules against the retained
	// auto-deleted rows of the KB. Rows hit by an enabled rule are restored as
	// manual (待补录) records so they re-appear in the list for manual editing.
	// Returns the number of rows restored.
	ReassessRecognition(ctx context.Context, kbID string) (int, error)
	// ListKnowledgeFolderTree returns the folder hierarchy derived from the
	// folder_path of every knowledge entry in a knowledge base, with per-folder
	// document counts. It powers the document sidebar tree.
	ListKnowledgeFolderTree(ctx context.Context, kbID string) (*types.KnowledgeFolderTree, error)
	// MoveKnowledgeToFolder re-files knowledge entries under the given folder
	// path (empty means the knowledge base top level). Folders are derived from
	// the stored paths, so a path that does not exist yet is created implicitly.
	MoveKnowledgeToFolder(
		ctx context.Context,
		kbID string,
		ids []string,
		folderPath string,
	) (int64, error)
	// RenameKnowledgeFolder moves a folder and everything below it to a new path.
	RenameKnowledgeFolder(ctx context.Context, kbID string, from string, to string) (int64, error)
	// DeleteKnowledge deletes knowledge by ID.
	DeleteKnowledge(ctx context.Context, id string) error
	// AutoDeleteKnowledge deletes a knowledge row judged not to belong in its
	// knowledge base (not_contract / not_invoice). It keeps the physical file
	// and records auto-delete history markers so the delete-history drawer can
	// list, preview, restore and purge the row.
	AutoDeleteKnowledge(ctx context.Context, id, reason string) error
	// ListDeletedKnowledge returns the auto-deleted rows of a knowledge base.
	ListDeletedKnowledge(
		ctx context.Context,
		kbID string,
		page, pageSize int,
		keyword string,
	) (*types.DeletedKnowledgePage, error)
	// RestoreDeletedKnowledge brings an auto-deleted row back and re-enqueues
	// parsing; front-end polling then triggers extraction automatically.
	RestoreDeletedKnowledge(ctx context.Context, id string) (*types.Knowledge, error)
	// PurgeDeletedKnowledge permanently removes an auto-deleted row and its
	// retained physical file.
	PurgeDeletedKnowledge(ctx context.Context, id string) error
	// DeleteKnowledgeList deletes multiple knowledge entries by IDs.
	DeleteKnowledgeList(ctx context.Context, ids []string) error
	// GetKnowledgeFile retrieves the file associated with the knowledge.
	GetKnowledgeFile(ctx context.Context, id string) (io.ReadCloser, string, error)
	// GetDeletedKnowledgeFile retrieves the retained physical file of a
	// soft-deleted (auto-deleted) row, for the delete-history preview drawer.
	GetDeletedKnowledgeFile(ctx context.Context, id string) (io.ReadCloser, string, error)
	// GetDeletedKnowledgeByID returns one soft-deleted (auto-deleted) row by ID.
	GetDeletedKnowledgeByID(ctx context.Context, id string) (*types.Knowledge, error)
	// UpdateKnowledge updates knowledge information.
	UpdateKnowledge(ctx context.Context, knowledge *types.Knowledge) error
	// SaveInvoiceCustomMetadata persists invoice extraction metadata without the
	// generic flat-scalar custom_metadata validation (used by the invoice module).
	SaveInvoiceCustomMetadata(ctx context.Context, knowledgeID string, meta types.JSON) error
	// RegenerateKnowledgeSummary refreshes the document description and summary retrieval chunk.
	RegenerateKnowledgeSummary(ctx context.Context, knowledgeID string) (*types.Knowledge, error)
	// RequestKnowledgeSummaryRefresh enqueues an async summary refresh.
	RequestKnowledgeSummaryRefresh(ctx context.Context, knowledgeID string) error
	// RegenerateChunkQuestions rebuilds the auxiliary questions for one current chunk revision.
	RegenerateChunkQuestions(ctx context.Context, chunkID string) ([]types.GeneratedQuestion, error)
	// UpdateManualKnowledge updates manual Markdown knowledge content.
	UpdateManualKnowledge(
		ctx context.Context,
		knowledgeID string,
		payload *types.ManualKnowledgePayload,
	) (*types.Knowledge, error)
	// ReparseKnowledge deletes existing document content and re-parses the knowledge asynchronously.
	// When processOverrides is non-nil, it is validated and persisted to the knowledge metadata
	// before re-parsing, letting callers adjust parse config on reparse; nil keeps stored overrides.
	ReparseKnowledge(
		ctx context.Context,
		knowledgeID string,
		processOverrides *types.KnowledgeProcessOverrides,
	) (*types.Knowledge, error)
	// CancelKnowledgeParse marks an in-progress parse as cancelled by the
	// user. The knowledge row and any partially written chunks/index are
	// kept; downstream queued tasks for the same knowledge are best-effort
	// dequeued and active workers are signalled to stop at their next
	// checkpoint. Idempotent — returns the existing row when the knowledge
	// is already cancelled. Returns an error when the knowledge is in a
	// terminal state (completed / failed) or being deleted.
	CancelKnowledgeParse(ctx context.Context, knowledgeID string) (*types.Knowledge, error)
	// CloneKnowledgeBase clones knowledge to another knowledge base.
	CloneKnowledgeBase(ctx context.Context, srcID, dstID string) error
	// UpdateImageInfo updates image information for a knowledge chunk.
	UpdateImageInfo(ctx context.Context, knowledgeID string, chunkID string, imageInfo string) error
	// ListFAQEntries lists FAQ entries under a FAQ knowledge base.
	// When tagUUIDs is non-empty, results are filtered by tag UUID on FAQ chunks (OR semantics).
	// searchField: specifies which field to search in ("standard_question", "similar_questions", "answers", "" for all)
	// sortOrder: "asc" for time ascending (updated_at ASC), default is time descending (updated_at DESC)
	ListFAQEntries(
		ctx context.Context,
		kbID string,
		page *types.Pagination,
		tagUUIDs []string,
		legacyTagSeqID int64,
		keyword string,
		searchField string,
		sortOrder string,
	) (*types.PageResult, error)
	// UpsertFAQEntries imports or appends FAQ entries asynchronously.
	// When DryRun is true, only validates entries without actually importing.
	// Returns task ID (Knowledge ID) for tracking import progress.
	UpsertFAQEntries(ctx context.Context, kbID string, payload *types.FAQBatchUpsertPayload) (string, error)
	// CreateFAQEntry creates a single FAQ entry synchronously.
	CreateFAQEntry(ctx context.Context, kbID string, payload *types.FAQEntryPayload) (*types.FAQEntry, error)
	// GetFAQEntry retrieves a single FAQ entry by seq_id.
	GetFAQEntry(ctx context.Context, kbID string, entrySeqID int64) (*types.FAQEntry, error)
	// UpdateFAQEntry updates a single FAQ entry.
	UpdateFAQEntry(ctx context.Context, kbID string, entrySeqID int64, payload *types.FAQEntryPayload) (*types.FAQEntry, error)
	// AddSimilarQuestions adds similar questions to a FAQ entry.
	AddSimilarQuestions(ctx context.Context, kbID string, entrySeqID int64, questions []string) (*types.FAQEntry, error)
	// UpdateFAQEntryFieldsBatch updates multiple fields for FAQ entries in batch.
	// Supports updating is_enabled, is_recommended, tag_id, and other fields in a single call.
	UpdateFAQEntryFieldsBatch(ctx context.Context, kbID string, req *types.FAQEntryFieldsBatchUpdate) error
	// DeleteFAQEntries deletes FAQ entries in batch by seq_id.
	DeleteFAQEntries(ctx context.Context, kbID string, entrySeqIDs []int64) error
	// SearchFAQEntries searches FAQ entries using hybrid search.
	SearchFAQEntries(ctx context.Context, kbID string, req *types.FAQSearchRequest) ([]*types.FAQEntry, error)
	// ExportFAQEntries exports all FAQ entries for a knowledge base as CSV data.
	ExportFAQEntries(ctx context.Context, kbID string) ([]byte, error)
	// ExportFAQEntriesJSON exports all FAQ entries as a JSON array compatible with FAQEntryPayload.
	ExportFAQEntriesJSON(ctx context.Context, kbID string) ([]byte, error)
	// UpdateKnowledgeTagBatch updates tag for document knowledge items in batch.
	// authorizedKBID restricts all updates to knowledge items belonging to this KB;
	// pass empty string to skip (caller must ensure authorization by other means).
	UpdateKnowledgeTagBatch(ctx context.Context, authorizedKBID string, updates map[string][]string) error
	// SetKnowledgeTags replaces all tags for a single knowledge entry.
	SetKnowledgeTags(ctx context.Context, knowledgeID string, tagIDs []string) error
	// GetKnowledgeTags returns tags for multiple knowledge IDs.
	GetKnowledgeTags(ctx context.Context, knowledgeIDs []string) (map[string][]*types.KnowledgeTag, error)
	// EnsureInvoiceCategoryTags creates (if missing) and attaches the given
	// invoice category tags to a knowledge entry, preserving its existing manual tags.
	EnsureInvoiceCategoryTags(ctx context.Context, kbID, knowledgeID string, categories []string) error
	// UpdateFAQEntryTagBatch updates tag for FAQ entries in batch.
	// Key: entry seq_id, Value: tag seq_id (nil to remove tag)
	UpdateFAQEntryTagBatch(ctx context.Context, kbID string, updates map[int64]*int64) error
	// GetRepository gets the knowledge repository
	GetRepository() KnowledgeRepository
	// ProcessManualUpdate handles Asynq manual knowledge update tasks (cleanup + re-indexing)
	ProcessManualUpdate(ctx context.Context, t *asynq.Task) error
	// ProcessDocument handles Asynq document processing tasks
	ProcessDocument(ctx context.Context, t *asynq.Task) error
	// ProcessFAQImport handles Asynq FAQ import tasks
	ProcessFAQImport(ctx context.Context, t *asynq.Task) error
	// ProcessQuestionGeneration handles Asynq question generation tasks
	ProcessQuestionGeneration(ctx context.Context, t *asynq.Task) error
	// ProcessSummaryGeneration handles Asynq summary generation tasks
	ProcessSummaryGeneration(ctx context.Context, t *asynq.Task) error
	// ProcessKBClone handles Asynq knowledge base clone tasks
	ProcessKBClone(ctx context.Context, t *asynq.Task) error
	// ProcessKnowledgeMove handles Asynq knowledge move tasks
	ProcessKnowledgeMove(ctx context.Context, t *asynq.Task) error
	// ProcessKnowledgeListDelete handles Asynq knowledge list delete tasks
	ProcessKnowledgeListDelete(ctx context.Context, t *asynq.Task) error
	// ProcessKnowledgeListReparse handles Asynq knowledge list reparse tasks
	ProcessKnowledgeListReparse(ctx context.Context, t *asynq.Task) error
	// GetKBCloneProgress retrieves the progress of a knowledge base clone task
	GetKBCloneProgress(ctx context.Context, taskID string) (*types.KBCloneProgress, error)
	// SaveKBCloneProgress saves the progress of a knowledge base clone task
	SaveKBCloneProgress(ctx context.Context, progress *types.KBCloneProgress) error
	// GetKnowledgeMoveProgress retrieves the progress of a knowledge move task
	GetKnowledgeMoveProgress(ctx context.Context, taskID string) (*types.KnowledgeMoveProgress, error)
	// SaveKnowledgeMoveProgress saves the progress of a knowledge move task
	SaveKnowledgeMoveProgress(ctx context.Context, progress *types.KnowledgeMoveProgress) error
	// GetFAQImportProgress retrieves the progress of an FAQ import task
	GetFAQImportProgress(ctx context.Context, taskID string) (*types.FAQImportProgress, error)
	// UpdateLastFAQImportResultDisplayStatus updates the display status of FAQ import result
	UpdateLastFAQImportResultDisplayStatus(ctx context.Context, kbID string, displayStatus string) error
	// SearchKnowledge searches knowledge items by keyword across the tenant.
	// fileTypes: optional list of file extensions to filter by (e.g., ["csv", "xlsx"])
	SearchKnowledge(ctx context.Context, keyword string, offset, limit int, fileTypes []string) ([]*types.Knowledge, bool, int64, error)
	// SearchKnowledgeForScopes searches knowledge within the given (tenant_id, kb_id) scopes (e.g. for shared agent context).
	SearchKnowledgeForScopes(ctx context.Context, scopes []types.KnowledgeSearchScope, keyword string, offset, limit int, fileTypes []string) ([]*types.Knowledge, bool, int64, error)
}

// KnowledgeRepository defines the interface for knowledge repositories.
type KnowledgeRepository interface {
	CreateKnowledge(ctx context.Context, knowledge *types.Knowledge) error
	GetKnowledgeByID(ctx context.Context, tenantID uint64, id string) (*types.Knowledge, error)
	// GetKnowledgeByIDOnly returns knowledge by ID without tenant filter (for permission resolution).
	GetKnowledgeByIDOnly(ctx context.Context, id string) (*types.Knowledge, error)
	ListKnowledgeByKnowledgeBaseID(ctx context.Context, tenantID uint64, kbID string) ([]*types.Knowledge, error)
	// ListPagedKnowledgeByKnowledgeBaseID lists all knowledge in a knowledge base
	// with pagination. The filter struct controls optional dimensions (tag, keyword,
	// file type, parse status, source channel, updated time range); pass a zero
	// struct to disable all filters.
	ListPagedKnowledgeByKnowledgeBaseID(ctx context.Context,
		tenantID uint64, kbID string, page *types.Pagination, filter types.KnowledgeListFilter,
	) ([]*types.Knowledge, int64, error)
	UpdateKnowledge(ctx context.Context, knowledge *types.Knowledge) error
	// UpdateKnowledgeBatch updates knowledge items in batch
	UpdateKnowledgeBatch(ctx context.Context, knowledgeList []*types.Knowledge) error
	DeleteKnowledge(ctx context.Context, tenantID uint64, id string) error
	DeleteKnowledgeList(ctx context.Context, tenantID uint64, ids []string) error
	GetKnowledgeBatch(ctx context.Context, tenantID uint64, ids []string) ([]*types.Knowledge, error)
	// CheckKnowledgeExists checks if knowledge already exists.
	// For file types, check by fileHash or (fileName+fileSize).
	// For URL types, check by URL.
	// Returns whether it exists, the existing knowledge object (if any), and possible error.
	CheckKnowledgeExists(
		ctx context.Context,
		tenantID uint64,
		kbID string,
		params *types.KnowledgeCheckParams,
	) (bool, *types.Knowledge, error)
	// ListKnowledgeFolderCounts aggregates the number of knowledge entries
	// stored directly in each folder_path of a knowledge base.
	ListKnowledgeFolderCounts(
		ctx context.Context,
		tenantID uint64,
		kbID string,
	) ([]*types.KnowledgeFolderCount, error)
	// UpdateKnowledgeFolderPath files the given entries under folderPath.
	UpdateKnowledgeFolderPath(
		ctx context.Context,
		tenantID uint64,
		kbID string,
		ids []string,
		folderPath string,
	) (int64, error)
	// RenameKnowledgeFolderPath rewrites folder_path for a folder and all of its
	// descendants. Renaming onto an existing path merges the folders.
	RenameKnowledgeFolderPath(
		ctx context.Context,
		tenantID uint64,
		kbID string,
		from string,
		to string,
	) (int64, error)
	// AminusB returns the IDs of knowledge in A that have no counterpart in B,
	// comparing file_hash as a multiset (so duplicate-count differences and
	// NULL/empty hashes are handled correctly, letting a clone converge).
	AminusB(ctx context.Context, Atenant uint64, A string, Btenant uint64, B string) ([]string, error)
	UpdateKnowledgeColumn(ctx context.Context, id string, column string, value interface{}) error
	// UpdateKnowledgeColumns updates multiple columns of a knowledge row in a single
	// statement so callers that flip several related fields (e.g. parse_status +
	// error_message) cannot leave the row in a half-updated state.
	UpdateKnowledgeColumns(ctx context.Context, id string, values map[string]interface{}) error
	// UpdateActiveDeletingKnowledgeColumns updates an active, non-deleted knowledge row
	// only when it is still in the transient deleting state.
	UpdateActiveDeletingKnowledgeColumns(ctx context.Context, id string, values map[string]interface{}) (bool, error)
	// FinalizeSubtask atomically decrements pending_subtasks_count for the
	// given knowledge and promotes parse_status from "finalizing" to
	// "completed" when the count reaches zero. Returns the post-decrement
	// count, whether this caller's UPDATE was the one that promoted the
	// row, and any error.
	FinalizeSubtask(ctx context.Context, id string) (int, bool, error)
	// SetFinalizing atomically transitions a row from "processing" to
	// "finalizing" and writes the initial pending_subtasks_count. Returns
	// whether the transition took place (false when the row's parse_status
	// was no longer "processing", e.g. user cancelled / deleted in flight).
	SetFinalizing(ctx context.Context, id string, expectedSubtasks int) (bool, error)
	// CountKnowledgeByKnowledgeBaseID counts the number of knowledge items in a knowledge base.
	CountKnowledgeByKnowledgeBaseID(ctx context.Context, tenantID uint64, kbID string) (int64, error)
	// CountKnowledgeByStatus counts the number of knowledge items with the specified parse status.
	CountKnowledgeByStatus(ctx context.Context, tenantID uint64, kbID string, parseStatuses []string) (int64, error)
	// SearchKnowledge searches knowledge items by keyword across the tenant.
	// fileTypes: optional list of file extensions to filter by (e.g., ["csv", "xlsx"])
	SearchKnowledge(ctx context.Context, tenantID uint64, keyword string, offset, limit int, fileTypes []string) ([]*types.Knowledge, bool, error)

	// FindByMetadataKey finds a knowledge item by a key-value pair in the metadata JSON column.
	// Used by data source sync to locate existing items by external_id.
	FindByMetadataKey(ctx context.Context, tenantID uint64, kbID string, key string, value string) (*types.Knowledge, error)
	// FindByMetadataKeyPrefix finds knowledge items whose metadata[key] starts
	// with the given prefix. Used to sweep an external node's attachment sub-items.
	FindByMetadataKeyPrefix(ctx context.Context, tenantID uint64, kbID string, key string, prefix string) ([]*types.Knowledge, error)
	// FindByDataSourceExternalID finds a knowledge item owned by one data source
	// and identified by the source's external item ID.
	FindByDataSourceExternalID(
		ctx context.Context, tenantID uint64, kbID, dataSourceID, externalID string,
	) (*types.Knowledge, error)
	// HardDeleteKnowledge physically removes a row after DeleteKnowledge's soft-delete
	// cascade. Sync-internal deletions use this so rows never become tombstones.
	HardDeleteKnowledge(ctx context.Context, tenantID uint64, id string) error
	// HardDeleteKnowledgeList is the batch counterpart of HardDeleteKnowledge.
	HardDeleteKnowledgeList(ctx context.Context, tenantID uint64, ids []string) error
	// GetDeletedKnowledgeByID returns a soft-deleted knowledge row (Unscoped).
	GetDeletedKnowledgeByID(ctx context.Context, tenantID uint64, id string) (*types.Knowledge, error)
	// ListDeletedKnowledge lists soft-deleted rows carrying the auto-delete marker.
	ListDeletedKnowledge(
		ctx context.Context,
		tenantID uint64,
		kbID string,
		page, pageSize int,
		keyword string,
	) ([]*types.Knowledge, int64, error)
	// RestoreDeletedKnowledgeRow clears the soft-delete tombstone and resets the
	// row to pending so it can be re-parsed.
	RestoreDeletedKnowledgeRow(ctx context.Context, tenantID uint64, id string) error
	// SearchKnowledgeInScopes searches knowledge items by keyword within the given (tenant_id, kb_id) scopes (own + shared).
	SearchKnowledgeInScopes(ctx context.Context, scopes []types.KnowledgeSearchScope, keyword string, offset, limit int, fileTypes []string) ([]*types.Knowledge, bool, int64, error)
	// ListIDsByTagIDs returns all knowledge IDs that have any of the specified tag IDs (OR semantics).
	ListIDsByTagIDs(ctx context.Context, tenantID uint64, kbID string, tagIDs []string) ([]string, error)
	// SetKnowledgeTags replaces all tags for a single knowledge entry (deletes old, inserts new).
	SetKnowledgeTags(ctx context.Context, knowledgeID string, tagIDs []string) error
	// AddKnowledgeTagRelations attaches tags without removing existing ones,
	// after validating that the knowledge and every tag belong to the given
	// tenant and knowledge base. Duplicate deliveries are idempotent.
	AddKnowledgeTagRelations(
		ctx context.Context,
		tenantID uint64,
		kbID, knowledgeID string,
		tagIDs []string,
	) error
	// GetKnowledgeTags returns tags for multiple knowledge IDs.
	GetKnowledgeTags(ctx context.Context, knowledgeIDs []string) (map[string][]*types.KnowledgeTag, error)
	// DeleteKnowledgeTagRelations deletes all tag relations for a knowledge entry.
	DeleteKnowledgeTagRelations(ctx context.Context, knowledgeID string) error
}
