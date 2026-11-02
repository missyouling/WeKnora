package handler

import (
	"testing"
	"time"
)

// 提取防重入锁的**超时自愈**（第 24 轮任务二）。
//
// 真机事故：某份文件（22d87e28 通行费汇总单）在上一轮重提时 worker 中断，
// 内存锁没被 defer 释放，此后每次重提都 409 `extract_in_progress`，
// 而库里 `custom_metadata.extract_status` 也停在 `processing`。
// 用户看到的现象是「这份文件一直在提取中」，实际早就没有任务在跑，
// 而且**没有任何自愈路径** —— 只能重启服务清锁。
//
// 修复：锁里存加锁时刻，超过 extractLockTTL 视为失效并顺手清理。
// 这组测试锁住三条契约：
//  1. 正常持锁 → 仍然拒绝（防重入不能被削弱）；
//  2. 超时持锁 → 放行并清理（自愈）；
//  3. 从未加锁 → 放行（首次提交不受影响）。

func newTestHandler() *BusinessExtractHandler {
	return &BusinessExtractHandler{}
}

func TestExtractLockedRejectsFreshLock(t *testing.T) {
	h := newTestHandler()
	if !h.tryLockExtract("kid-1") {
		t.Fatal("首次加锁必须成功")
	}
	if !h.extractLocked("kid-1") {
		t.Fatal("刚加的锁必须判定为「在途」，否则防重入形同虚设")
	}
	// 重复加锁必须失败（并发重复提交要挡住）
	if h.tryLockExtract("kid-1") {
		t.Fatal("已有未超时锁时不得重复加锁")
	}
}

func TestExtractLockedReleasesExpiredLock(t *testing.T) {
	h := newTestHandler()
	// 直接写一个「很久以前」的时间戳，模拟 worker 死亡后残留的锁
	h.extractMu.Store("kid-2", time.Now().Add(-2*extractLockTTL))
	if h.extractLocked("kid-2") {
		t.Fatal("超过 TTL 的锁必须判定为已失效，否则该文档永久卡死")
	}
	// 必须顺手清理（否则下次仍会命中同一条残留记录）
	if _, loaded := h.extractMu.Load("kid-2"); loaded {
		t.Fatal("失效锁必须被删除，不能留在 map 里")
	}
	// 清理后应能重新加锁
	if !h.tryLockExtract("kid-2") {
		t.Fatal("失效锁清理后必须可以重新加锁")
	}
}

func TestExtractLockedNoLock(t *testing.T) {
	h := newTestHandler()
	if h.extractLocked("kid-3") {
		t.Fatal("从未加锁时必须放行")
	}
}

func TestExtractLockedWrongValueTypeTreatedAsExpired(t *testing.T) {
	// 防御性：万一有人把锁写成别的类型（历史版本写的是 struct{}{}），
	// 不能因此把文档永久锁死 —— 当作已失效并清理。
	h := newTestHandler()
	h.extractMu.Store("kid-4", struct{}{})
	if h.extractLocked("kid-4") {
		t.Fatal("类型不符时不得判定为在途")
	}
	if _, loaded := h.extractMu.Load("kid-4"); loaded {
		t.Fatal("类型不符的脏值必须被清理")
	}
}

func TestExtractLockTTLIsReasonable(t *testing.T) {
	// TTL 太短会把正在跑的长任务误判为失效（重复提取、白烧 token）；
	// 太长则自愈太慢。实测单份最长约 3 分钟，取 15 分钟留 5 倍余量。
	if extractLockTTL < 10*time.Minute || extractLockTTL > 30*time.Minute {
		t.Fatalf("TTL 应在 10~30 分钟区间，实际 %s", extractLockTTL)
	}
}
