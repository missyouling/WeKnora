#!/bin/bash
echo "=== node/npm 路径 ==="
command -v node
command -v npm
echo "=== apt redis-server 状态 ==="
systemctl is-enabled redis-server.service 2>/dev/null || echo "redis-server 未启用"
systemctl is-active redis-server.service 2>/dev/null || echo "redis-server 未运行"
ps aux | grep -E "redis-server" | grep -v grep || echo "无 redis-server 进程"
echo "=== 6379 归属 ==="
ss -tlnp | grep 6379
echo "=== DONE ==="
