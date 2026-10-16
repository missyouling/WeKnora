#!/bin/bash
echo "=== 1) 后端 8080 health ==="
curl -s -m 5 http://localhost:8080/health; echo
echo "=== 2) 前端 5173 ==="
curl -s -m 5 -o /dev/null -w 'HTTP %{http_code}\n' http://localhost:5173/ || echo VITE_UNREACHABLE
echo "=== 3) docreader 50051 归属 ==="
ss -tlnp | grep 50051
echo "=== docreader 进程 ==="
ps aux | grep -E "docreader" | grep -v grep | head -3
echo "=== docreader 日志尾部 ==="
tail -5 /opt/weknora-dev/data/docreader_vm.log 2>/dev/null || echo NO_LOG
echo "=== vite 日志尾部 ==="
tail -5 /opt/weknora-dev/data/vite_vm.log 2>/dev/null || echo NO_LOG
echo "=== 端口汇总 ==="
ss -tlnp | grep -E ':(8080|5173|50051)\b'
echo "=== DONE ==="
