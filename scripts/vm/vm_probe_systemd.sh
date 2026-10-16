#!/bin/bash
echo "=== 当前三服务进程 ==="
ps aux | grep -E "server_linux|docreader.main|vite" | grep -v grep
echo "=== 端口监听 ==="
ss -tlnp | grep -E ':(8080|5173|50051)\b'
echo "=== 容器 restart policy ==="
docker inspect --format '{{.Name}} restart={{.HostConfig.RestartPolicy.Name}}' $(docker ps -q) 2>/dev/null
echo "=== Redis 连通性 ==="
redis-cli -a 'redis123!@#' -h 127.0.0.1 -p 6379 ping 2>/dev/null | tail -1
echo "=== 现有 systemd 服务 ==="
systemctl list-unit-files | grep -iE 'weknora|docreader|redis' || echo "无 weknora 相关 systemd"
echo "=== DONE ==="
