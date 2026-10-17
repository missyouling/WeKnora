package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 版本戳注入链路的回归保护：
// /api/v1/system/info 的 commit_id / build_time 由 Dockerfile.da 的
// -ldflags -X 写入本包变量。这条链路断掉（包改名、变量改名、CI 漏传
// build-args）时不会报错，只会静默退回 "unknown"——线上就没法把运行中的
// 实例对应到具体提交。这里用源码断言把断点变成红灯。

// repoFile 读取仓库根下的文件（测试运行在 internal/handler，根为 ../..）。
func repoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatalf("读取仓库文件 %s 失败: %v（测试需以 internal/handler 为工作目录运行）", rel, err)
	}
	return string(b)
}

// modulePath 从 go.mod 取模块路径：-X 的目标包路径必须由它拼出，
// 这样模块改名时断言会跟着走，而不是写死一份可能漂移的字符串。
func modulePath(t *testing.T) string {
	t.Helper()
	for _, line := range strings.Split(repoFile(t, "go.mod"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	t.Fatal("go.mod 中找不到 module 声明")
	return ""
}

// TestDailyAffairsVersionStampLdflags 校验 Dockerfile 的 -X 注入目标与
// 本包变量一致，且 CI 真的把 Git SHA 传了进去。
func TestDailyAffairsVersionStampLdflags(t *testing.T) {
	target := modulePath(t) + "/internal/handler"
	dockerfile := repoFile(t, "Dockerfile.da")

	if !strings.Contains(dockerfile, "ARG GIT_COMMIT") {
		t.Fatal("Dockerfile.da 缺少 `ARG GIT_COMMIT`：CI 无法注入 Git SHA，commit_id 恒为 unknown")
	}
	for _, want := range []string{
		"-X " + target + ".CommitID",
		"-X " + target + ".BuildTime",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Fatalf("Dockerfile.da 的 go build 缺少 %s（期待 -ldflags 注入 %s 包变量）", want, target)
		}
	}

	wf := repoFile(t, ".github/workflows/da-ci-cd.yml")
	if !strings.Contains(wf, "GIT_COMMIT=${{ github.sha }}") {
		t.Fatal("da-ci-cd.yml 的 build-push 未传 GIT_COMMIT=${{ github.sha }}：镜像拿不到真实提交号")
	}
}

// TestDailyAffairsVersionStampVars 保证注入目标是真实存在的包变量，
// 且有非空默认值（变量被改名/删除时本文件编译不过）。
func TestDailyAffairsVersionStampVars(t *testing.T) {
	for name, v := range map[string]string{
		"Version":   Version,
		"CommitID":  CommitID,
		"BuildTime": BuildTime,
	} {
		if strings.TrimSpace(v) == "" {
			t.Fatalf("handler.%s 默认值为空：未注入 -ldflags 时 /system/info 会直接省掉该字段", name)
		}
	}
}
