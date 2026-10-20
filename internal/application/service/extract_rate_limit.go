package service

import (
	"context"
	"encoding/json"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
)

// 本文件是**提取链路的速率治理层**：并发削峰（源头） + 限流退避（兜底） +
// 错误信息中文化（展示）。三者配套才能根治批量上传时的 429/1302 失败。
//
// 背景（2026-10-20 实测）：连续上传 20 份通行费汇总单时，前端上传并发 × 单文件
// 内部批次并发（invoiceBatchConcurrency=4）叠加，瞬间向大模型发起数十个并发请求，
// 触发供应商限流 `status 429: {"error":{"code":"1302","message":"您的账户已达到速率限制"}}`，
// 7 份文件提取失败并生成「提取失败」占位行污染发票列表。
//
// 治理分两层，缺一不可：
//   - 削峰：全局信号量限制同时在飞的 LLM 提取请求数（默认 3），多文件排队串行化；
//   - 兜底：仍命中限流时按指数退避 + 抖动重试，且不消耗 Prompt 降级次数。

const (
	// defaultExtractLLMConcurrency 全局同时在飞的 LLM 提取请求上限。
	// 供应商侧限流通常按「并发请求数」而非 QPS 计，取 3 兼顾吞吐与安全边界。
	defaultExtractLLMConcurrency = 3
	maxExtractLLMConcurrency     = 16
	// maxExtractRateLimitRetries 单次提取允许的限流退避次数（不占用 Prompt 降级次数）。
	// 退避序列 3/6/12/24/45/45s，总等待 ≈135s，足以覆盖批量上传的排队窗口。
	maxExtractRateLimitRetries = 6
)

// extractLLMSem 全局提取并发信号量（进程级）。
var extractLLMSem = make(chan struct{}, ExtractLLMConcurrency())

// ExtractLLMConcurrency 返回当前生效的全局并发上限，可用
// WEKNORA_EXTRACT_LLM_CONCURRENCY 覆盖（非法值回落默认，永不返回 0）。
func ExtractLLMConcurrency() int {
	raw := strings.TrimSpace(os.Getenv("WEKNORA_EXTRACT_LLM_CONCURRENCY"))
	if raw == "" {
		return defaultExtractLLMConcurrency
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultExtractLLMConcurrency
	}
	if n > maxExtractLLMConcurrency {
		return maxExtractLLMConcurrency
	}
	return n
}

// maxInvoiceBatchConcurrency 单文件内部批次并发的绝对上限（与 handler 侧历史值一致）。
const maxInvoiceBatchConcurrency = 4

// InvoiceEffectiveBatchConcurrency 单文件内部批次并发度：由全局上限派生，
// 保证「多文件排队时单文件不吃满全局额度」（留 1 个槽给其它文件）。
//
// 例：全局 3 → 单文件 2；全局 1 → 单文件 1（严格串行）。
func InvoiceEffectiveBatchConcurrency() int {
	n := ExtractLLMConcurrency()
	if n <= 1 {
		return 1
	}
	if n-1 < maxInvoiceBatchConcurrency {
		return n - 1
	}
	return maxInvoiceBatchConcurrency
}

// acquireExtractLLMSlot 获取一个全局提取槽位；返回的 release 必须被调用一次。
// ctx 取消时立即返回错误（不占槽、不泄漏 goroutine）。
func acquireExtractLLMSlot(ctx context.Context) (func(), error) {
	select {
	case extractLLMSem <- struct{}{}:
		released := false
		return func() {
			if released {
				return
			}
			released = true
			<-extractLLMSem
		}, nil
	case <-ctx.Done():
		return func() {}, ctx.Err()
	}
}

// IsExtractRateLimitError 判定错误是否为「供应商速率限制」。
// 覆盖各家常见表述：HTTP 429、厂商码 1302、rate limit / 速率限制 / 请求频率。
func IsExtractRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, hint := range []string{
		"429", "1302",
		"rate limit", "rate_limit", "ratelimit", "rate-limit",
		"too many requests",
		"速率限制", "请求频率", "限流",
	} {
		if strings.Contains(msg, hint) {
			return true
		}
	}
	return false
}

// extractBackoffBase 退避基准序列（秒）：3 → 6 → 12 → 24 → 45（封顶）。
func extractBackoffBase(attempt int) time.Duration {
	const ceiling = 45 * time.Second
	base := 3 * time.Second
	if attempt < 0 {
		attempt = 0
	}
	d := base
	for i := 0; i < attempt; i++ {
		d *= 2
	}
	if d > ceiling {
		d = ceiling
	}
	return d
}

// extractJitterFloat 抖动源（测试可替换为固定值以获得确定性结果）。
var extractJitterFloat = rand.Float64

// ExtractBackoffDelay 计算第 attempt 次（0-based）限流重试的等待时长：
// 基准指数增长（3/6/12/24/45s 封顶）叠加 ±20% 抖动，打散多请求同时重试的尖峰。
func ExtractBackoffDelay(attempt int) time.Duration {
	base := extractBackoffBase(attempt)
	// 抖动区间 [0.8, 1.2)
	jitter := 0.8 + extractJitterFloat()*0.4
	return time.Duration(float64(base) * jitter)
}

// extractBackoffSleep 退避睡眠（测试可替换为记录型桩函数）。
var extractBackoffSleep = func(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// sleepRateLimitBackoff 执行一次限流退避睡眠；返回 false 表示 ctx 已取消（应放弃重试）。
func sleepRateLimitBackoff(ctx context.Context, attempt int) bool {
	d := ExtractBackoffDelay(attempt)
	logger.Warnf(ctx, "extract rate limited, backing off %s (attempt %d/%d)", d, attempt+1, maxExtractRateLimitRetries)
	if err := extractBackoffSleep(ctx, d); err != nil {
		logger.Warnf(ctx, "extract backoff aborted: %v", err)
		return false
	}
	return true
}

// ---- 错误信息中文化（与前端 formatExtractErrorMessage 同规则） ----

// techPrefixRE 匹配英文技术栈前缀片段（用于逐段剥离，露出核心中文原因）。
var techPrefixRE = regexp.MustCompile(`(?i)^(invoice extraction failed|extract invoice fields|extract invoice|create chat completion|chat completion|api request failed( with status \d+)?|request failed|http error|llm call failed|extract failed|failed to [a-z ]+?|error|json unmarshal|cannot unmarshal)$`)

var cjkRE = regexp.MustCompile(`[\p{Han}]`)

// FormatExtractErrorMessage 把生硬的英文调用栈 + 裸 JSON 错误信息转译为可展示的中文短句。
// 幂等：已经是中文的结果再次传入不会被破坏。
func FormatExtractErrorMessage(raw string) string {
	msg := strings.TrimSpace(raw)
	if msg == "" {
		return ""
	}
	lower := strings.ToLower(msg)

	// ① 裸 JSON：优先取出其中的 message 字段
	msg = extractJSONMessage(msg)
	lower = strings.ToLower(msg)

	// ② 分类转译
	switch {
	case strings.Contains(lower, "429"), strings.Contains(lower, "1302"),
		strings.Contains(lower, "rate limit"), strings.Contains(lower, "速率限制"),
		strings.Contains(lower, "请求频率"), strings.Contains(lower, "too many requests"):
		return "请求频率超限（已触发大模型限流保护），请稍后重试"
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "deadline exceeded"),
		strings.Contains(lower, "context deadline"), strings.Contains(lower, "i/o timeout"),
		strings.Contains(lower, "超时"):
		return "提取超时，文档较大或模型响应缓慢"
	case strings.Contains(lower, "no_text"), strings.Contains(lower, "no text content"),
		strings.Contains(lower, "empty content"), strings.Contains(lower, "empty extraction"),
		strings.Contains(lower, "未识别到"):
		return "未识别到有效票面文本，请检查源文件是否清晰"
	}

	// ③ 剥离英文技术栈前缀，露出核心原因
	if out := stripExtractTechPrefix(msg); out != "" {
		return out
	}
	return msg
}

// extractJSONMessage 从 `{"error":{"code":"1302","message":"..."}}` 之类的裸 JSON
// 中取出 message；非 JSON 或取不到则原样返回。
func extractJSONMessage(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		return s
	}
	// 错误信息里可能夹带前后缀（如 "...: {"error": {...}}"），截取首个 JSON 对象
	start := strings.Index(s, "{")
	if start < 0 {
		return s
	}
	depth := 0
	end := -1
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
			}
		}
		if end > 0 {
			break
		}
	}
	if end <= 0 {
		return s
	}
	var payload struct {
		Message string `json:"message"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(s[start:end]), &payload); err != nil {
		return s
	}
	if payload.Error.Message != "" {
		return payload.Error.Message
	}
	if payload.Message != "" {
		return payload.Message
	}
	return s
}

// stripExtractTechPrefix 按 ":" 分段丢弃纯英文技术栈前缀，直到命中含中文的片段；
// 全部为英文时返回最后一段（保留最有信息量的尾部）。
func stripExtractTechPrefix(s string) string {
	parts := strings.Split(s, ":")
	for i, p := range parts {
		seg := strings.TrimSpace(p)
		if seg == "" {
			continue
		}
		if cjkRE.MatchString(seg) {
			return strings.TrimSpace(strings.Join(parts[i:], "："))
		}
		if !techPrefixRE.MatchString(strings.TrimRight(seg, " ")) {
			// 命中未知英文片段：保留从它开始的剩余内容（可能是真实原因）
			return strings.TrimSpace(strings.Join(parts[i:], "："))
		}
	}
	// 全英文：返回最后一段
	for i := len(parts) - 1; i >= 0; i-- {
		if strings.TrimSpace(parts[i]) != "" {
			return strings.TrimSpace(parts[i])
		}
	}
	return ""
}
