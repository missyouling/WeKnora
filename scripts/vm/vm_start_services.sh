#!/bin/bash
set -e
export PATH="$HOME/.local/bin:$PATH"
cd /opt/weknora-dev

echo "=== 1) 启动 docreader（gRPC 50051）==="
pkill -f "docreader.main" 2>/dev/null || true
sleep 1
nohup docreader/.venv/bin/python -m docreader.main > /opt/weknora-dev/data/docreader_vm.log 2>&1 &
echo "docreader PID: $!"
sleep 3
ss -tlnp | grep 50051 && echo DOCREADER_UP || echo DOCREADER_WAIT

echo "=== 2) 启动后端（8080）==="
pkill -f server_linux.exe 2>/dev/null || true
sleep 1
nohup ./server_linux.exe > /opt/weknora-dev/data/server_vm.log 2>&1 &
echo "server PID: $!"
sleep 5
ss -tlnp | grep 8080 && echo SERVER_UP || echo SERVER_WAIT

echo "=== 3) 启动前端 dev（5173）==="
pkill -f "vite" 2>/dev/null || true
sleep 1
cd frontend
nohup npm run dev > /opt/weknora-dev/data/vite_vm.log 2>&1 &
echo "vite PID: $!"
sleep 8
ss -tlnp | grep 5173 && echo VITE_UP || echo VITE_WAIT

echo "=== 汇总 ==="
ss -tlnp | grep -E ':(8080|5173|50051)\b' || echo NONE_LISTENING
echo "=== 后端日志尾部 ==="
tail -5 /opt/weknora-dev/data/server_vm.log 2>/dev/null || true
echo "=== DONE ==="
