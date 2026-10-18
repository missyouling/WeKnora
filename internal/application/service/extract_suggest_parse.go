package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/infrastructure/chunker"
	"github.com/Tencent/WeKnora/internal/infrastructure/docparser"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// 本文件是「发票设置 → 智能字段生成」的**样票取文本**入口。
//
// 为什么不直接复用 TestExtractConfig（沙箱测试规则）：
//   - 沙箱只吃「知识库里已解析好的文档」，没有上传入口；
//   - 沙箱跑的是提取任务（问"能提出什么值"），这里要跑的是字段建模
//     （问"票面上有哪些字段"）。
//
// 为什么不走 temporaryDocumentService.Create：那条路会落 temporary_documents 表、
// 绑会话、投递 asynq 任务，是一个**有状态、有生命周期**的聊天附件流程；
// 而智能配置只需要一段纯文本。所以这里抽出同包内可复用的解析核心，
// 复用 parse() 的解析器选择与 VLM OCR 兜底，但不落库、不排队。
//
// 与 (*temporaryDocumentService).parse 的关系：解析逻辑**同源**，但入口不同——
// 本函数直接吃内存字节（HTTP 上传的样票），不经过 fileService 取流。

const (
	// extractSuggestMaxTextRunes 票面文本上限：与 temporaryDocumentPromptBudget
	// 同量级（12000），保证一次建议调用的 Prompt 不超出模型上下文。
	extractSuggestMaxTextRunes = 12000
)

// ExtractSuggestParser 样票解析所需的最小依赖集合。
//
// 刻意**不包含** repository / fileService / taskEnqueuer：
// 从类型层面保证「智能建议」链路不可能写库或投递任务（纯建议计算接口）。
type ExtractSuggestParser struct {
	// Documents 远端文档解析器（docreader）：PDF/Office 走它出Markdown。
	// 为 nil 时仅纯文本/音频判定可用，非纯文本会明确报错而不是静默返回空。
	Documents interfaces.DocumentReader
	// Model 供 VLM OCR 兜底使用（扫描件/图片样票全靠它）；为 nil 时跳过 OCR。
	Model interfaces.ModelService
	// Tenant 提供租户级解析器引擎配置（ParseChatParserEngine / ToOverridesMap）。
	Tenant interfaces.TenantService
}

// ExtractSuggestInput 一次样票解析的输入。
type ExtractSuggestInput struct {
	FileName string // 上传时的原始文件名（仅用于解析器判定与日志）
	Ext      string // 扩展名，带点(.pdf)或不带(pdf)均可
	Data     []byte // 文件字节
	// ParserEngine 显式指定解析引擎；空或 auto 时按租户配置自动选择。
	ParserEngine string
	// VLMModelID 扫描件/图片的 OCR 兜底模型；空则不做 OCR。
	VLMModelID string
	// OCRMaxPages 送入 VLM 的页图上限；<=0 用包内默认值。
	OCRMaxPages int
	// ImageUnderstanding 扫描 PDF 是否走页图 OCR（纯图片文件恒走，不受此开关影响）。
	ImageUnderstanding bool
}

// ExtractSuggestResult 样票解析结果。
type ExtractSuggestResult struct {
	Text      string `json:"text"`      // 解析后的票面纯文本（已截断）
	Truncated bool   `json:"truncated"` // Text 是否因超长被截断
	Parser    string `json:"parser"`    // 实际生效的解析器标识：plain_text / asr / docreader / vlm / 引擎名
}

// normalizedExt 归一化扩展名：去点、小写（docparser 与 temporaryDocument 的判定口径一致）。
func (in ExtractSuggestInput) normalizedExt() string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(in.Ext), "."))
}

// ParseFileForExtractSuggest 把一张样票解析成纯文本，供智能字段生成使用。
// 复用 (*temporaryDocumentService).parse 的三条能力：
//  1. 解析器引擎选择（租户配置 → 显式 ParserEngine）；
//  2. 纯文本/音频短路（文本直接返回，音频走 ASR —— 但发票场景音频无意义，明确拒绝）；
//  3. 扫描件/图片的 VLM OCR 兜底（applyImageUnderstanding 同逻辑）。
//
// 明确不做的事：写 temporary_documents 表、绑会话、投递 asynq 任务、
// 触碰 knowledge_base / chunks —— 整个函数是纯计算。
func ParseFileForExtractSuggest(ctx context.Context, deps ExtractSuggestParser, in ExtractSuggestInput) (ExtractSuggestResult, error) {
	ext := in.normalizedExt()
	if len(in.Data) == 0 {
		return ExtractSuggestResult{}, fmt.Errorf("sample file is empty")
	}
	if strings.TrimSpace(in.FileName) == "" {
		return ExtractSuggestResult{}, fmt.Errorf("sample file name is required")
	}

	// 1) 纯文本短路：与 parse() 同口径（temporaryTextExtensions 以点开头，这里两种形态都试）
	if _, ok := temporaryTextExtensions["."+ext]; ok {
		return truncateSuggestText(string(in.Data), "plain_text"), nil
	}

	// 2) 音频：聊天附件支持 ASR，但票面样票不可能是音频，直接拒绝而不是
	// 悄悄返回空文本（空文本会让上层误判为"票面无字段"）。
	if docparser.IsAudioFormat(ext) {
		return ExtractSuggestResult{}, fmt.Errorf("audio sample is not supported: %s", ext)
	}

	// 3) 引擎选择：显式指定优先，否则按租户配置为该扩展名挑引擎
	parserEngine := strings.TrimSpace(in.ParserEngine)
	if parserEngine == "" || parserEngine == "auto" {
		parserEngine = suggestParserEngine(ctx, deps, ext)
	}
	if deps.Documents == nil {
		return ExtractSuggestResult{}, fmt.Errorf(
			"no document reader available to parse sample: %s", ext,
		)
	}

	request := &types.ReadRequest{
		FileContent: in.Data,
		FileName:    in.FileName,
		FileType:    ext,
	}
	if parserEngine != "" {
		request.ParserEngine = parserEngine
	}
	if deps.Tenant != nil {
		if tenant, ok := ctx.Value(types.TenantInfoContextKey).(*types.Tenant); ok &&
			tenant != nil && tenant.ParserEngineConfig != nil {
			request.ParserEngineOverrides = tenant.ParserEngineConfig.ToOverridesMap()
		}
	}
	reader, err := docparser.NewReader(ctx, parserEngine, ext, false, docparser.ReaderDeps{
		Overrides: request.ParserEngineOverrides,
		Remote:    deps.Documents,
	})
	if err != nil {
		return ExtractSuggestResult{}, fmt.Errorf("parse sample: %w", err)
	}
	result, err := reader.Read(ctx, request)
	if err != nil {
		return ExtractSuggestResult{}, fmt.Errorf("parse sample: %w", err)
	}
	content := ""
	if result != nil {
		content = result.MarkdownContent
	}
	if content != "" {
		content = normalizeSuggestText(content)
	}
	parser := "docreader"
	if request.ParserEngine != "" {
		parser = request.ParserEngine
	}

	// 4) 扫描件/图片 VLM OCR 兜底：票面样票（扫描 PDF、手机拍的 JPG）绝大多数走这里。
	if deps.Model != nil && strings.TrimSpace(in.VLMModelID) != "" {
		pageImages := collectImageBytes(imageRefsOf(result), suggestOCRMaxPages(in.OCRMaxPages))
		if extra := applySuggestImageUnderstanding(ctx, deps.Model, in.VLMModelID, ext, in.Data, pageImages, content); extra != "" {
			content = extra
			parser = "vlm"
		}
	}
	return truncateSuggestText(content, parser), nil
}

// suggestParserEngine 按租户配置为该扩展名选择解析引擎（auto 语义的具体化）。
func suggestParserEngine(ctx context.Context, deps ExtractSuggestParser, ext string) string {
	if deps.Tenant == nil {
		return ""
	}
	if tenant, ok := ctx.Value(types.TenantInfoContextKey).(*types.Tenant); ok && tenant != nil {
		return strings.TrimSpace(tenant.ParserEngineConfig.ResolveChatParserEngine(ext))
	}
	return ""
}

func suggestOCRMaxPages(override int) int {
	if override > 0 {
		return override
	}
	return temporaryDocumentImageOCRMaxPages()
}

func imageRefsOf(result *types.ReadResult) []types.ImageRef {
	if result == nil {
		return nil
	}
	return result.ImageRefs
}

// applySuggestImageUnderstanding 与 (*temporaryDocumentService).applyImageUnderstanding
// 保持同一判定：图片文件恒做 OCR 兜底（文本够多时跳过，省一次 VLM 调用）；
// 扫描 PDF 需显式开启 ImageUnderstanding 且页面文本过少才触发。
func applySuggestImageUnderstanding(
	ctx context.Context,
	modelService interfaces.ModelService,
	vlmModelID, ext string,
	fileData []byte,
	pageImages [][]byte,
	content string,
) string {
	lowText := approxTextContentRunes(content) < temporaryDocumentLowTextRunes
	var extracted string
	switch {
	case docparser.IsImageFormat(ext):
		if !lowText || len(fileData) == 0 {
			return ""
		}
		extracted = understandImagesWithVLMSvc(ctx, modelService, vlmModelID, [][]byte{fileData}, false, true)
	case lowText && len(pageImages) > 0:
		extracted = understandImagesWithVLMSvc(ctx, modelService, vlmModelID, pageImages, true, false)
	default:
		return ""
	}
	extracted = strings.TrimSpace(extracted)
	if extracted == "" {
		return ""
	}
	if strings.TrimSpace(content) == "" {
		return extracted
	}
	return content + "\n\n" + extracted
}

// normalizeSuggestText 行尾归一 + HTML 表格归一（与 parse() 同一层清洗）。
func normalizeSuggestText(md string) string {
	if md == "" {
		return ""
	}
	md = chunker.NormalizeLineEndings(md)
	return docparser.NormalizeHTMLTables(md)
}

// truncateSuggestText 按上限截断并置位 Truncated；空白内容原样返回，
// 由调用方决定是报"无法识别"还是当作空票面。
func truncateSuggestText(text, parser string) ExtractSuggestResult {
	runes := []rune(text)
	if len(runes) <= extractSuggestMaxTextRunes {
		return ExtractSuggestResult{Text: text, Truncated: false, Parser: parser}
	}
	logger.Debugf(context.Background(), "extract suggest: sample text truncated from %d to %d runes",
		len(runes), extractSuggestMaxTextRunes)
	return ExtractSuggestResult{
		Text:      string(runes[:extractSuggestMaxTextRunes]),
		Truncated: true,
		Parser:    parser,
	}
}

// ExtractSuggestMaxTextRunes 暴露票面文本上限，供 handler 层做同口径截断
// （JSON 直传路径不走 ParseFileForExtractSuggest，但同样不能超过这个预算）。
func ExtractSuggestMaxTextRunes() int {
	return extractSuggestMaxTextRunes
}
