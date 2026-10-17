package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
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

// SingleAttemptExtractAttempts 只尝试一次（非流式）。
//
// 专供分批回退路径：主调用（Guided）失败时通常已吃满一次长超时，若回退再叠加
// 默认 4 次尝试，单批最坏耗时 = 5 + 4×5 = 25 分钟，批次串行后总时长失控
// （2026-10-17 实测：16 页 5 批，用户等待 10+ 分钟无结果）。
func SingleAttemptExtractAttempts() []ExtractAttempt {
	return []ExtractAttempt{{Label: "fallback-once"}}
}

// forcedReasoningModelRE 识别"思考态无法关闭"的模型：对它们发送 thinking:false
// 要么被供应商拒绝（400），要么被忽略，都会白白浪费一次调用。
// 只认明确命名特征，避免误判普通模型（如 glm-4-flash 不应命中）。
var forcedReasoningModelRE = regexp.MustCompile(`(?i)(reasoner|reasoning|thinking|qwq|qwen3|(^|[-_.])r1([-_.]|$)|(^|[-_.])z1([-_.]|$)|(^|[-_.])o[1-4]([-_.]|$))`)

// modelForcesThinking 报告该模型是否强制思考（此时不应下發任何 thinking 开关）。
func modelForcesThinking(modelName string) bool {
	name := strings.TrimSpace(modelName)
	if name == "" {
		return false
	}
	// 去掉常见的厂商前缀（如 "deepseek/deepseek-reasoner"）不影响匹配，直接整体匹配。
	return forcedReasoningModelRE.MatchString(name)
}

// thinkingParamRejected 判断错误是否为"供应商不接受 thinking/reasoning 参数"。
// 判定要保守：必须同时出现思考相关词与参数非法类词，避免把网络错误当成参数问题。
func thinkingParamRejected(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	mentionsThinking := strings.Contains(msg, "thinking") ||
		strings.Contains(msg, "reasoning") ||
		strings.Contains(msg, "推理")
	if !mentionsThinking {
		return false
	}
	for _, hint := range []string{
		"not support", "unsupported", "invalid", "unknown", "unrecognized",
		"unexpected", "not allowed", "illegal", "不支持", "无效",
	} {
		if strings.Contains(msg, hint) {
			return true
		}
	}
	return false
}

// RunExtractWithRetry 按尝试序列执行模型调用，每次产出交给 parse 校验。
//   - parse 返回 true → 该原始文本可用，立即返回，不再尝试后续策略；
//   - 全部失败 → 返回最后一次错误，并把最后一份原始输出交给 onFailure（可为 nil）做诊断落盘。
//
// baseOpts 会被逐次浅拷贝后覆盖 Format / Thinking，调用方传入的 baseOpts 不会被修改。
//
// 思考参数自适应（跨模型兼容）：
//   - 模型本身强制思考（reasoner/r1/z1/qwen3 等）→ 不发送 thinking 字段，
//     让供应商走默认；发送 false 会被拒或无效。
//   - 供应商明确回报"不接受该参数"→ **在同一次尝试内**去掉该参数重发一次，
//     不浪费一次完整策略（4 次尝试很贵，且每次都有硬超时）。
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
	forceThinking := modelForcesThinking(modelName)
	if forceThinking {
		logger.Infof(ctx, "extract: model %q forces thinking, omitting thinking switch", modelName)
	}

	for i, a := range attempts {
		start := time.Now()
		var (
			raw     string
			err     error
			parseOK bool
		)

		// 最多 2 次：第 2 次仅在"供应商拒绝 thinking 参数"时触发。
		for try := 0; try < 2; try++ {
			opts := *baseOpts
			if forceThinking || try > 0 {
				opts.Thinking = nil
				opts.ReasoningEffort = ""
			}
			if len(a.Schema) > 0 {
				opts.Format = a.Schema
			}

			raw, err = "", nil
			// 每次尝试都带硬超时：供应商偶发挂起连接（无响应、无错误）会让调用方
			// 无限等待，任务永久停在 processing。超时转为错误后走重试/失败路径。
			callCtx, cancelCall := withExtractCallTimeout(ctx)
			if a.Stream {
				raw, err = chatStreamCollect(callCtx, model, messages, &opts)
			} else {
				resp, cerr := model.Chat(callCtx, messages, &opts)
				if cerr != nil {
					err = cerr
				} else if resp != nil {
					raw = resp.Content
				}
			}
			cancelCall()

			if err != nil && try == 0 && !forceThinking && thinkingParamRejected(err) {
				logger.Warnf(ctx, "extract attempt %d (%s) rejected thinking parameter, retrying without it: %v", i+1, a.Label, err)
				continue
			}
			if err == nil {
				parseOK = parse(raw)
			}
			break
		}
		elapsed := time.Since(start)

		if err != nil {
			lastErr = err
			logger.Warnf(ctx, "extract attempt %d (%s) error: %v (elapsed=%s)", i+1, a.Label, err, elapsed)
			continue
		}
		if parseOK {
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
