package handler

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// TestNewExtractionContext_DetachedFromRequest 回归测试：模拟前端超时/切页面导致
// HTTP 请求上下文被取消，提取上下文必须仍然有效（大模型调用不被连带掐断）。
func TestNewExtractionContext_DetachedFromRequest(t *testing.T) {
	reqCtx, cancel := context.WithCancel(context.Background())
	cancel() // 模拟：前端断开，请求上下文已取消

	extractCtx, extractCancel := newExtractionContext(10000)
	defer extractCancel()

	select {
	case <-reqCtx.Done():
	default:
		t.Fatal("前置条件失效：请求上下文应已取消")
	}
	select {
	case <-extractCtx.Done():
		t.Fatal("提取上下文不应随请求取消而取消")
	default:
	}
}

// TestNewExtractionContext_CarriesTenantID 提取上下文必须携带租户 ID，
// 否则后续的 service/仓储调用会因缺租户而失败。
func TestNewExtractionContext_CarriesTenantID(t *testing.T) {
	const tenantID uint64 = 10000
	extractCtx, extractCancel := newExtractionContext(tenantID)
	defer extractCancel()

	got, ok := extractCtx.Value(types.TenantIDContextKey).(uint64)
	if !ok || got != tenantID {
		t.Fatalf("租户 ID 丢失：got=%v ok=%v want=%d", got, ok, tenantID)
	}
}

// TestNewExtractionContext_HasTimeout 脱离请求后必须有硬上限，
// 避免无人值守的提取任务永久运行烧 token。
func TestNewExtractionContext_HasTimeout(t *testing.T) {
	extractCtx, extractCancel := newExtractionContext(10000)
	defer extractCancel()

	deadline, ok := extractCtx.Deadline()
	if !ok {
		t.Fatal("提取上下文必须带超时 deadline")
	}
	remain := time.Until(deadline)
	if remain <= 0 || remain > extractionTimeout {
		t.Fatalf("超时不合理：remain=%v extractionTimeout=%v", remain, extractionTimeout)
	}
	if extractionTimeout != 30*time.Minute {
		t.Fatalf("提取总超时被意外修改：%v", extractionTimeout)
	}
}
