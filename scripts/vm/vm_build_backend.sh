#!/bin/bash
set -e
cd /opt/weknora-dev
echo "=== 环境检查 ==="
gcc --version | head -1
ls /usr/include/sqlite3.h && echo SQLITE3_H_OK
echo "=== go env ==="
go version
echo "CGO_ENABLED=$(go env CGO_ENABLED)"
echo "=== 检查 anydoc 库（编译期 cgo 特性）==="
ls third_party/anydoc-go/lib/ 2>/dev/null || echo "no anydoc lib dir"
echo "=== go mod download ==="
export GOMODCACHE=/root/go/pkg/mod
go mod download 2>&1 | tail -5 || echo "mod download warning"
echo "=== go build（anydoc tag 探测）==="
if ls third_party/anydoc-go/lib/linux_amd64_gnu/libanydoc_go.a >/dev/null 2>&1; then
  echo "有 anydoc 静态库 → 带 anydoc tag 构建"
  CGO_ENABLED=1 go build -tags anydoc -o server_linux.exe ./cmd/server
else
  echo "无 anydoc 库 → 普通构建"
  CGO_ENABLED=1 go build -o server_linux.exe ./cmd/server
fi
echo "BUILD_EXIT=$?"
ls -lh server_linux.exe 2>/dev/null || echo "BUILD_FAILED"
echo "=== DONE ==="
