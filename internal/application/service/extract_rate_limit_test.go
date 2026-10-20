package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/models/chat"
)

// 速率治理层测试（确定性：不调真实模型、不真睡，CI 可跑）。
//
// 锁死三件事：
//  1. 命中 429/1302 时限流重试**不推进** Prompt 降级序列（降级要重新烧 token）；
//  2. 退避序列 3/6/12/24/45s 且带抖动，超过重试上限才失败；
//  3. 全局信号量真的把并发压在上限内（批量上传削峰的根）。

// stubBackoff 把退避睡眠替换为记录型桩函数，返回记录的时长切片。
func stubBackoff(t *testing.T) *[]time.Duration {
	t.Helper()
	recorded := &[]time.Duration{}
	orig := extractBackoffSleep
	extractBackoffSleep = func(_ context.Context, d time.Duration) error {
		*recorded = append(*recorded, d)
		return nil
	}
	t.Cleanup(func() { extractBackoffSleep = orig })
	return recorded
}

// stubJitter 把抖动源固定为 0.5（抖动区间 [0.8,1.2) 的中点 → 延迟 = 基准值）。
func stubJitter(t *testing.T) {
	t.Helper()
	orig := extractJitterFloat
	extractJitterFloat = func() float64 { return 0.5 }
	t.Cleanup(func() { extractJitterFloat = orig })
}

func rateLimitErr() error {
	return errors.New("API request failed with status 429: {\"error\":{\"code\":\"1302\",\"message\":\"您的账户已达到速率限制，请您控制请求频率\"}}")
}

// TestExtractRateLimitRetriesThenSucceeds：连续两次限流后第三次成功 ——
// 必须自动排队重试成功，且不消耗 Prompt 降级次数（只用了第 1 个策略）。
func TestExtractRateLimitRetriesThenSucceeds(t *testing.T) {
	sleeps := stubBackoff(t)
	stubJitter(t)

	f := &fakeRetryChat{
		nonStreamErrs: []error{rateLimitErr(), rateLimitErr(), nil},
		nonStreamOut:  []string{"", "", "OK"},
	}
	// 两个策略：若限流错误推进了降级序列，第二次调用会带上 schema（formats[1] != ""）
	schema := []byte(`{"type":"object"}`)
	attempts := []ExtractAttempt{{Label: "a1"}, {Label: "a2", Schema: schema}}

	got, err := RunExtractWithRetry(context.Background(), f, nil, &chat.ChatOptions{}, attempts,
		func(raw string) bool { return raw == "OK" }, nil)
	if err != nil {
		t.Fatalf("限流后应自动重试成功，实际 err=%v", err)
	}
	if got != "OK" {
		t.Fatalf("got %q, want OK", got)
	}
	if len(f.calls) != 3 {
		t.Fatalf("调用次数 = %d, want 3（两次限流重试 + 一次成功）", len(f.calls))
	}
	if len(*sleeps) != 2 {
		t.Fatalf("退避次数 = %d, want 2", len(*sleeps))
	}
	if (*sleeps)[0] != 3*time.Second || (*sleeps)[1] != 6*time.Second {
		t.Fatalf("退避序列 = %v, want [3s 6s]", *sleeps)
	}
	for i, fm := range f.formats {
		if fm != "" {
			t.Fatalf("第 %d 次调用带上了 schema %q —— 限流重试不得推进 Prompt 降级序列", i+1, fm)
		}
	}
}

// TestExtractRateLimitGivesUpAfterMaxRetries：持续限流 → 退避到上限后返回错误。
func TestExtractRateLimitGivesUpAfterMaxRetries(t *testing.T) {
	sleeps := stubBackoff(t)
	stubJitter(t)

	errs := make([]error, maxExtractRateLimitRetries+2)
	for i := range errs {
		errs[i] = rateLimitErr()
	}
	f := &fakeRetryChat{nonStreamErrs: errs}
	_, err := RunExtractWithRetry(context.Background(), f, nil, &chat.ChatOptions{},
		SingleAttemptExtractAttempts(), func(string) bool { return true }, nil)
	if err == nil {
		t.Fatal("持续限流应在退避上限后失败")
	}
	if !IsExtractRateLimitError(err) {
		t.Fatalf("返回错误应仍可被判定为限流错误: %v", err)
	}
	if len(*sleeps) != maxExtractRateLimitRetries {
		t.Fatalf("退避次数 = %d, want %d", len(*sleeps), maxExtractRateLimitRetries)
	}
}

// TestExtractRateLimitBackoffDelaySequence：退避基准 3/6/12/24/45 封顶 + 抖动范围。
func TestExtractRateLimitBackoffDelaySequence(t *testing.T) {
	stubJitter(t)
	want := []time.Duration{3, 6, 12, 24, 45, 45}
	for i, w := range want {
		if got := ExtractBackoffDelay(i); got != w*time.Second {
			t.Errorf("ExtractBackoffDelay(%d) = %v, want %v", i, got, w*time.Second)
		}
	}
	// 抖动边界：±20%
	orig := extractJitterFloat
	extractJitterFloat = func() float64 { return 0 }
	lo := ExtractBackoffDelay(0)
	extractJitterFloat = func() float64 { return 1 }
	hi := ExtractBackoffDelay(0)
	extractJitterFloat = orig
	if lo != time.Duration(2.4*float64(time.Second)) || hi != time.Duration(3.6*float64(time.Second)) {
		t.Errorf("抖动区间 = [%v, %v], want [2.4s, 3.6s]", lo, hi)
	}
}

// TestExtractRateLimitDetection：限流识别覆盖各家表述，且不误判普通错误。
func TestExtractRateLimitDetection(t *testing.T) {
	yes := []string{
		"API request failed with status 429",
		`{"error":{"code":"1302","message":"您的账户已达到速率限制，请您控制请求频率"}}`,
		"rate limit exceeded",
		"too many requests",
		"您的账户已达到速率限制",
	}
	for _, s := range yes {
		if !IsExtractRateLimitError(errors.New(s)) {
			t.Errorf("应判定为限流: %s", s)
		}
	}
	no := []string{
		"unparseable response (len=0)",
		"context deadline exceeded",
		"连接被拒绝",
	}
	for _, s := range no {
		if IsExtractRateLimitError(errors.New(s)) {
			t.Errorf("不应判定为限流: %s", s)
		}
	}
	if IsExtractRateLimitError(nil) {
		t.Error("nil 错误不得判定为限流")
	}
}

// TestExtractRateLimitSemaphoreCapsConcurrency：全局信号量把并发压在上限内。
func TestExtractRateLimitSemaphoreCapsConcurrency(t *testing.T) {
	cap := ExtractLLMConcurrency()
	var cur, peak int32
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < cap*3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			release, err := acquireExtractLLMSlot(context.Background())
			if err != nil {
				return
			}
			n := atomic.AddInt32(&cur, 1)
			mu.Lock()
			if n > peak {
				peak = n
			}
			mu.Unlock()
			time.Sleep(20 * time.Millisecond)
			atomic.AddInt32(&cur, -1)
			release()
		}()
	}
	close(start)
	wg.Wait()
	mu.Lock()
	got := peak
	mu.Unlock()
	if int(got) > cap {
		t.Fatalf("观测到的最大并发 = %d, 超过全局上限 %d", got, cap)
	}
}

// TestExtractRateLimitSlotRespectsContextCancel：ctx 取消时立即放弃排队。
func TestExtractRateLimitSlotRespectsContextCancel(t *testing.T) {
	hold := make([]func(), 0, ExtractLLMConcurrency())
	for i := 0; i < ExtractLLMConcurrency(); i++ {
		rel, err := acquireExtractLLMSlot(context.Background())
		if err != nil {
			t.Fatalf("占满槽位失败: %v", err)
		}
		hold = append(hold, rel)
	}
	defer func() {
		for _, r := range hold {
			r()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if _, err := acquireExtractLLMSlot(ctx); err == nil {
		t.Fatal("槽位占满时应阻塞，ctx 超时后返回错误")
	}
}

// TestExtractInvoiceBatchConcurrencyDerivedFromGlobalLimit：单文件批次并发
// 必须由全局上限派生（留一个槽给其它文件），否则批量上传会独占额度。
func TestExtractInvoiceBatchConcurrencyDerivedFromGlobalLimit(t *testing.T) {
	n := ExtractLLMConcurrency()
	got := InvoiceEffectiveBatchConcurrency()
	if n <= 1 {
		if got != 1 {
			t.Fatalf("全局上限 %d 时单文件并发应为 1, got %d", n, got)
		}
		return
	}
	if got != n-1 && got != maxInvoiceBatchConcurrency {
		t.Fatalf("单文件并发 = %d, want min(%d-1, %d)", got, n, maxInvoiceBatchConcurrency)
	}
	if got > n {
		t.Fatalf("单文件并发 %d 不得超过全局上限 %d", got, n)
	}
}

// TestExtractFormatExtractErrorMessageChinese：错误信息中文化（与前端同规则）。
func TestExtractFormatExtractErrorMessageChinese(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			name: "429 裸 JSON",
			in:   `invoice extraction failed: extract invoice fields: create chat completion: API request failed with status 429: {"error":{"code":"1302","message":"您的账户已达到速率限制，请您控制请求频率"}}`,
			want: "请求频率超限（已触发大模型限流保护），请稍后重试",
		},
		{
			name: "1302 中文",
			in:   `API request failed with status 429: {"error":{"code":"1302","message":"您的账户已达到速率限制"}}`,
			want: "请求频率超限（已触发大模型限流保护），请稍后重试",
		},
		{
			name: "超时",
			in:   "extract invoice fields: context deadline exceeded",
			want: "提取超时，文档较大或模型响应缓慢",
		},
		{
			name: "无文本",
			in:   "no text content to extract",
			want: "未识别到有效票面文本，请检查源文件是否清晰",
		},
		{
			name: "剥离英文技术栈前缀",
			in:   "invoice extraction failed: extract invoice fields: 票面缺少发票号码",
			want: "票面缺少发票号码",
		},
		{
			name: "纯中文原样返回",
			in:   "票面金额与明细不符",
			want: "票面金额与明细不符",
		},
		{
			name: "空串",
			in:   "   ",
			want: "",
		},
	}
	for _, c := range cases {
		if got := FormatExtractErrorMessage(c.in); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	// 幂等：转译结果再次转译不变
	once := FormatExtractErrorMessage(`API request failed with status 429: {"error":{"code":"1302","message":"您的账户已达到速率限制"}}`)
	if twice := FormatExtractErrorMessage(once); twice != once {
		t.Errorf("转译必须幂等: %q -> %q", once, twice)
	}
}

// TestExtractFormatExtractErrorMessageNoRawJSONLeak：转译后不得残留裸 JSON/英文栈。
func TestExtractFormatExtractErrorMessageNoRawJSONLeak(t *testing.T) {
	out := FormatExtractErrorMessage(`invoice extraction failed: extract invoice fields: create chat completion: API request failed with status 429: {"error":{"code":"1302","message":"您的账户已达到速率限制，请您控制请求频率"}}`)
	for _, leak := range []string{"{", "}", "invoice extraction failed", "create chat completion", "status 429"} {
		if strings.Contains(out, leak) {
			t.Errorf("转译结果仍残留 %q: %s", leak, out)
		}
	}
}
