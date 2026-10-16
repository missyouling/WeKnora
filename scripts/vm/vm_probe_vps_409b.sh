#!/bin/bash
echo "=== A) 日志中所有 POST 请求（近12h，前 50 条）==="
docker logs weknora-daily --since 12h 2>&1 | grep -oE "method=POST path=[^ ]+" | sort | uniq -c | sort -rn | head -50
echo
echo "=== B) 日志中 extract-business 路径（任何状态）==="
docker logs weknora-daily --since 12h 2>&1 | grep -c "extract-business" || true
echo
echo "=== C) Redis asynq 队列 key 统计 ==="
docker exec weknora-redis sh -c 'redis-cli -a "$REDIS_PASSWORD" --no-auth-warning keys "asynq:*" 2>/dev/null | head -30; echo ---; redis-cli -a "$REDIS_PASSWORD" --no-auth-warning zcount "asynq:default:queues" 0 +inf 2>/dev/null' 2>/dev/null || echo "redis exec failed, try direct"
echo
echo "=== D) asynq 任务类型分布（LRANGE 各队列前 5）==="
for q in asynq:default:pending asynq:default:active asynq:default:retry asynq:default:scheduled; do
  echo "-- $q --"
  docker exec weknora-redis sh -c "redis-cli -a \"\$REDIS_PASSWORD\" --no-auth-warning lrange $q 0 4 2>/dev/null | head -c 600" 2>/dev/null
  echo
done
echo "=== E) 04:57-05:04 提取相关完整日志 ==="
docker logs weknora-daily --since 12h 2>&1 | grep -E "04:5[0-9]|05:0[0-4]" | grep -iE "extract|enqueue|Business" | head -60
echo "=== DONE ==="
