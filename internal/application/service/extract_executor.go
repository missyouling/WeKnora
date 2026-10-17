package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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

	// 诊断上下文：提取卡住时最先要回答的就是"用的哪个模型、每次花了多久、拿到多少内容"。
	modelName := ""
	if model != nil {
		modelName = model.GetModelName()
	}

	for i, a := range attempts {
		start := time.Now()
		opts := *baseOpts
		if len(a.Schema) > 0 {
			opts.Format = a.Schema
		}

		var raw string
		var err error
		// 每次尝试都带硬超时：供应商偶发挂起连接（无响应、无错误）会让调用方
		// 无限等待，任务永久停在 processing。超时转为错误后走重试/失败路径。
		callCtx, cancelCall := withExtractCallTimeout(ctx)
		if a.Stream {
			raw, err = chatStreamCollect(callCtx, model, messages, &opts)
		} else {
			resp, cerr := model.Chat(callCtx, messages, &opts)
			if cerr != nil {
				err = cerr
			} else {
				raw = resp.Content
			}
		}
		cancelCall()
		elapsed := time.Since(start)

		if err != nil {
			lastErr = err
			logger.Warnf(ctx, "extract attempt %d (%s) error: %v (elapsed=%s)", i+1, a.Label, err, elapsed)
			continue
		}
		if parse(raw) {
			logger.Infof(ctx, "extract attempt %d (%s) succeeded (len=%d elapsed=%s)", i+1, a.Label, len(raw), elapsed)
			return raw, nil
		}
		// 空 content 单列一条告警：这是供应商侧问题（智谱等对"长输入 + 大输出"
		// 偶发返回空），与解析逻辑无关。不单独提示时极易被误判成解析写错。
		if strings.TrimSpace(raw) == "" {
			logger.Warnf(ctx, "extract attempt %d (%s) returned EMPTY content: model=%s elapsed=%s — "+
				"常见原因是显式传入 max_tokens（部分供应商在长输出时返回空），请检查 ChatOptions.MaxTokens 是否为 0",
				i+1, a.Label, modelName, elapsed)
		}
		// 保留"信息量最大"的那份原始输出供诊断，而不是最后一次：
		// 末次尝试常是流式空 content（智谱等供应商的典型表现），若只留最后一次，
		// onFailure 拿到的就是空串，线上无法定位模型到底漂移到哪。
		if len(raw) > len(lastRaw) {
			lastRaw = raw
		}
		lastErr = fmt.Errorf("unparseable response (len=%d)", len(raw))
		logger.Warnf(ctx, "extract attempt %d (%s) unparseable: len=%d elapsed=%s model=%s", i+1, a.Label, len(raw), elapsed, modelName)
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no usable response")
	}
	if onFailure != nil {
		onFailure(lastRaw, lastErr)
	}
	return "", lastErr
}
