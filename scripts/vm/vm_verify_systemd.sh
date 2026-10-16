#!/bin/bash
echo "=== 等待后端初始化 ==="
sleep 25
echo "=== 端口 ==="
ss -tlnp | grep -E ':(8080|5173|50051)\b'
echo "=== 服务状态 ==="
systemctl is-active weknora-backend.service weknora-vite.service
echo "=== health ==="
curl -s -m 5 http://localhost:8080/health; echo
echo "=== 前端 200 ==="
curl -s -m 5 -o /dev/null -w 'HTTP %{http_code}\n' http://localhost:5173/
echo "=== 后端日志尾部(asynq) ==="
strings /opt/weknora-dev/data/server_vm.log | grep -iE 'asynq .*starting|sandbox resolver|Starting processing' | tail -5
echo "=== 重启持久性验证（重启 backend 一次）==="
systemctl restart weknora-backend.service
sleep 20
systemctl is-active weknora-backend.service
curl -s -m 5 http://localhost:8080/health; echo
echo "=== DONE ==="
