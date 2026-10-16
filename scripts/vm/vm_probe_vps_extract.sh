#!/bin/bash
KID=4d4d9d6d-ba50-4416-b58a-7da4b04af339
echo "=== 1) asynq 队列任务统计 ==="
redis-cli -a 'redis123!@#' --no-auth-warning 2>/dev/null keys 'asynq:*' | head -20 || \
redis-cli -a "$(grep REDIS_PASSWORD /home/weknora-daily/.env | cut -d= -f2)" --no-auth-warning keys 'asynq:*' 2>/dev/null | head -20
echo
echo "=== 2) VPS 后端日志：该 knowledge 最近 100 行 extract 相关 ==="
docker logs weknora-daily --since 6h 2>&1 | grep -E "4d4d9d6d|Business extract task|extract invoice" | tail -40
echo "=== DONE ==="
