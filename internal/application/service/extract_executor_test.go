package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// 提取执行器的重试策略测试（确定性，不调真实模型，CI 可跑）。
//
// 覆盖的是"提取不出来"这条主链路的兜底能力：模型偶发返回空内容/非 JSON 时，
// 执行器应换策略再试，而不是直接失败。用 fake 驱动，断言调用序列与最终结果。

// fakeRetryChat 按预设序列返回内容，并记录每次调用的形态（流式/非流式、是否带 schema）。
type fakeRetryChat struct {
	streamOut     []string
	nonStreamOut  []string
	streamErrs    []error
	nonStreamErrs []error
	streamIdx     int
	nonStreamIdx  int
	calls         []string
	formats       []string
	// lastMessages 记录最近一次调用收到的消息，供断言（如自愈回灌是否截断）
	lastMessages []chat.Message
}

func (f *fakeRetryChat) Chat(_ context.Context, msgs []chat.Message, opts *chat.ChatOptions) (*types.ChatResponse, error) {
	f.lastMessages = msgs
	f.calls = append(f.calls, "nonstream")
	f.formats = append(f.formats, string(opts.Format))
	i := f.nonStreamIdx
	f.nonStreamIdx++
	if i < len(f.nonStreamErrs) && f.nonStreamErrs[i] != nil {
		return nil, f.nonStreamErrs[i]
	}
	if i < len(f.nonStreamOut) {
		return &types.ChatResponse{Content: f.nonStreamOut[i]}, nil
	}
	return &types.ChatResponse{Content: ""}, nil
}

func (f *fakeRetryChat) ChatStream(_ context.Context, msgs []chat.Message, opts *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	f.lastMessages = msgs
	f.calls = append(f.calls, "stream")
	f.formats = append(f.formats, string(opts.Format))
	i := f.streamIdx
	f.streamIdx++
	ch := make(chan types.StreamResponse, 2)
	if i < len(f.streamErrs) && f.streamErrs[i] != nil {
		close(ch)
		return ch, f.streamErrs[i]
	}
	out := ""
	if i < len(f.streamOut) {
		out = f.streamOut[i]
	}
	go func() {
		if out != "" {
			ch <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: out}
		}
		ch <- types.StreamResponse{Done: true}
		close(ch)
	}()
	return ch, nil
}

func (f *fakeRetryChat) GetModelName() string { return "fake-model" }
func (f *fakeRetryChat) GetModelID() string   { return "fake-model-id" }

// parseable 模拟调用方的校验：必须是含 "kind" 的 JSON 对象。
func parseable(raw string) bool {
	raw = strings.TrimSpace(raw)
	return strings.HasPrefix(raw, "{") && strings.Contains(raw, `"kind"`)
}

func TestExtractExecutorStopsOnFirstSuccess(t *testing.T) {
	m := &fakeRetryChat{
		streamOut:    []string{`{"kind":"invoice","invoices":[]}`},
		nonStreamOut: []string{`{"kind":"invoice","invoices":[]}`},
	}
	got, err := RunExtractWithRetry(context.Background(), m,
		[]chat.Message{{Role: "user", Content: "x"}},
		&chat.ChatOptions{}, DefaultExtractAttempts(nil), parseable, nil)
	if err != nil {
		t.Fatalf("首次可解析即应成功: %v", err)
	}
	if !strings.Contains(got, `"kind"`) {
		t.Errorf("返回内容异常: %q", got)
	}
	if len(m.calls) != 1 {
		t.Errorf("首次成功应只调用 1 次，实际 %d 次: %v", len(m.calls), m.calls)
	}
}

func TestExtractExecutorSucceedsOnLaterAttempt(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	// 第 1 次流式返回空、第 2 次非流式返回垃圾、第 3 次（带 schema 非流式）成功
	m := &fakeRetryChat{
		streamOut:    []string{"", ""},
		nonStreamOut: []string{"抱歉，我无法处理该文档", `{"kind":"invoice","invoices":[{"invoice_no":"123"}]}`},
	}
	got, err := RunExtractWithRetry(context.Background(), m,
		[]chat.Message{{Role: "user", Content: "x"}},
		&chat.ChatOptions{}, DefaultExtractAttempts(schema), parseable, nil)
	if err != nil {
		t.Fatalf("第 3 次尝试成功时应返回成功: %v", err)
	}
	if !strings.Contains(got, "123") {
		t.Errorf("应返回第 3 次的内容: %q", got)
	}
	if len(m.calls) != 3 {
		t.Errorf("应恰好调用 3 次，实际 %d: %v", len(m.calls), m.calls)
	}
	// 第 3 次是非流式 + schema 强约束
	if m.calls[2] != "nonstream" {
		t.Errorf("第 3 次应为 nonstream，实际 %s", m.calls[2])
	}
	if m.formats[2] != string(schema) {
		t.Errorf("第 3 次应带 schema，实际 Format=%q", m.formats[2])
	}
	// 前两次不应带 schema
	if m.formats[0] != "" || m.formats[1] != "" {
		t.Errorf("前两次不应带 schema: %v", m.formats[:2])
	}
}

func TestExtractExecutorAllAttemptsFail(t *testing.T) {
	m := &fakeRetryChat{
		streamOut:    []string{"", ""},
		nonStreamOut: []string{"not json", "also not json"},
	}
	var failedRaw string
	var failedErr error
	_, err := RunExtractWithRetry(context.Background(), m,
		[]chat.Message{{Role: "user", Content: "x"}},
		&chat.ChatOptions{}, DefaultExtractAttempts(nil), parseable,
		func(raw string, e error) { failedRaw, failedErr = raw, e })
	if err == nil {
		t.Fatal("全部失败时应返回错误")
	}
	if len(m.calls) != 4 {
		t.Errorf("应跑满 4 次尝试，实际 %d: %v", len(m.calls), m.calls)
	}
	// onFailure 必须拿到"信息量最大"的那份输出，供诊断落盘。
	// 第 4 次是流式空 content（供应商典型表现），若只留最后一次就拿不到任何线索。
	if failedRaw != "also not json" {
		t.Errorf("onFailure 应收到最长的非空原始输出，实际 %q", failedRaw)
	}
	if failedErr == nil {
		t.Error("onFailure 应收到错误")
	}
}

func TestExtractExecutorErrorContinuesToNextAttempt(t *testing.T) {
	// 第 1 次流式报错，第 2 次非流式成功 → 不应因单次错误中断
	m := &fakeRetryChat{
		streamErrs:   []error{errors.New("stream unavailable")},
		streamOut:    []string{""},
		nonStreamOut: []string{`{"kind":"invoice","invoices":[]}`},
	}
	got, err := RunExtractWithRetry(context.Background(), m,
		[]chat.Message{{Role: "user", Content: "x"}},
		&chat.ChatOptions{}, DefaultExtractAttempts(nil), parseable, nil)
	if err != nil {
		t.Fatalf("单次调用错误应继续尝试而非失败: %v", err)
	}
	if !strings.Contains(got, `"kind"`) {
		t.Errorf("返回内容异常: %q", got)
	}
	if len(m.calls) != 2 {
		t.Errorf("应调用 2 次（第 1 次报错后重试），实际 %d: %v", len(m.calls), m.calls)
	}
}

func TestExtractExecutorSingleAttemptStopsAfterOne(t *testing.T) {
	// 分批回退路径专用：Guided 已可能吃满一次长超时，回退再叠加 4 次尝试会让
	// 单批最坏耗时 = 5 + 4×5 = 25 分钟。这里锁死"只尝试一次"。
	m := &fakeRetryChat{nonStreamOut: []string{"not json"}}
	_, err := RunExtractWithRetry(context.Background(), m,
		[]chat.Message{{Role: "user", Content: "x"}},
		&chat.ChatOptions{}, SingleAttemptExtractAttempts(), parseable, nil)
	if err == nil {
		t.Fatal("不可解析时应返回错误")
	}
	if len(m.calls) != 1 {
		t.Errorf("单次尝试序列应只调用 1 次，实际 %d: %v", len(m.calls), m.calls)
	}
}

func TestExtractExecutorDoesNotMutateBaseOpts(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	base := &chat.ChatOptions{}
	m := &fakeRetryChat{
		streamOut:    []string{"", ""},
		nonStreamOut: []string{"bad", "bad"},
	}
	_, _ = RunExtractWithRetry(context.Background(), m,
		[]chat.Message{{Role: "user", Content: "x"}},
		base, DefaultExtractAttempts(schema), parseable, nil)
	if base.Format != nil {
		t.Errorf("baseOpts 不应被写入 Format，实际 %q", string(base.Format))
	}
}
