package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
)

// 本文件是提取中台的**通用执行器**：把「结构化输出约束 + 多次重试」固化为可复用能力。
//
// 背景：这套模式最早在 solar（knowledge_solar_extract.go）里跑通——4 次尝试
// （流式 → 非流式 → 非流式+JSON Schema 强制 → 流式重试），每次产出先校验再决定
// 是否继续。它是六个业务里唯一有重试的实现。发票体积最大（1472 行）却只有单次调用，
// 解析失败就直接抛错，是"提取不出来"的主要来源。
//
// 现在把这段能力抽出来，供所有提取业务复用，避免每个业务各写一份（或干脆不写）。

// ExtractAttempt 一次提取尝试的策略。
type ExtractAttempt struct {
	Label  string          // 诊断标签（仅日志用，如 "stream-free"）
	Stream bool            // 是否走流式收集
	Schema json.RawMessage // 非空时强制结构化输出（写入 chat.ChatOptions.Format）
}

// DefaultExtractAttempts 默认尝试序列，源自 solar 的成熟实践。
// 顺序：流式 → 非流式 → 非流式 + schema 强约束 → 流式重试。
// 每一步产出后由调用方的 parse 校验，成功即返回，不跑多余的尝试（省 token）。
func DefaultExtractAttempts(schema json.RawMessage) []ExtractAttempt {
	return []ExtractAttempt{
		{Label: "stream-free", Stream: true},
		{Label: "nonstream-free"},
		{Label: "nonstream-jsonobject", Schema: schema},
		{Label: "stream-free-2", Stream: true},
	}
}

// RunExtractWithRetry 按尝试序列执行模型调用，每次产出交给 parse 校验。
//   - parse 返回 true → 该原始文本可用，立即返回，不再尝试后续策略；
//   - 全部失败 → 返回最后一次错误，并把最后一份原始输出交给 onFailure（可为 nil）做诊断落盘。
//
// baseOpts 会被逐次浅拷贝后覆盖 Format，调用方传入的 baseOpts 不会被修改。
func RunExtractWithRetry(
	ctx context.Context,
	model chat.Chat,
	messages []chat.Message,
	baseOpts *chat.ChatOptions,
	attempts []ExtractAttempt,
	parse func(raw string) bool,
	onFailure func(raw string, err error),
) (string, error) {
	var lastErr error
	var lastRaw string

	for i, a := range attempts {
		opts := *baseOpts
		if len(a.Schema) > 0 {
			opts.Format = a.Schema
		}

		var raw string
		var err error
		if a.Stream {
			raw, err = chatStreamCollect(ctx, model, messages, &opts)
		} else {
			resp, cerr := model.Chat(ctx, messages, &opts)
			if cerr != nil {
				err = cerr
			} else {
				raw = resp.Content
			}
		}

		if err != nil {
			lastErr = err
			logger.Warnf(ctx, "extract attempt %d (%s) error: %v", i+1, a.Label, err)
			continue
		}
		if parse(raw) {
			return raw, nil
		}
		lastRaw = raw
		lastErr = fmt.Errorf("unparseable response (len=%d)", len(raw))
		logger.Warnf(ctx, "extract attempt %d (%s) unparseable (len=%d)", i+1, a.Label, len(raw))
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no usable response")
	}
	if onFailure != nil {
		onFailure(lastRaw, lastErr)
	}
	return "", lastErr
}
