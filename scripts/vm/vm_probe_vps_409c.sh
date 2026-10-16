#!/bin/bash
echo "=== 1) 当前时间 (VPS) ==="
date -u '+%Y-%m-%d %H:%M:%S UTC'
echo
echo "=== 2) Redis asynq 队列滞留（用 .env 密码）==="
RDPASS=$(grep -E '^REDIS_PASSWORD=' /home/weknora-daily/.env | cut -d= -f2-)
echo "redis pass found: ${RDPASS:+yes}"
docker exec weknora-redis redis-cli -a "$RDPASS" --no-auth-warning keys 'asynq:*' 2>&1 | head -30
echo "--- active/pending/retry 各队列长度 ---"
for q in 'asynq:default:active' 'asynq:default:pending' 'asynq:default:retry' 'asynq:default:scheduled'; do
  echo -n "$q = "
  docker exec weknora-redis redis-cli -a "$RDPASS" --no-auth-warning llen "$q" 2>&1
done
echo
echo "=== 3) 日志中 status_code=409 或 Conflict ==="
docker logs weknora-daily --since 12h 2>&1 | grep -E 'status_code=409|Conflict|already in progress|extraction already' | head -20
echo "---(end 409 grep)---"
echo
echo "=== 4) 日志中 4d4d9d6d 的 processing 写入与提取执行（完整上下文）==="
docker logs weknora-daily --since 12h 2>&1 | grep -nE '4d4d9d6d' | grep -iE 'processing|extract|SaveInvoice|Business|enqueue|CustomMetadata' | head -40
echo
echo "=== 5) 日志中 method=POST（任何路径，近12h）==="
docker logs weknora-daily --since 12h 2>&1 | grep -oE 'method=[A-Z]+' | sort | uniq -c | sort -rn | head
echo "=== DONE ==="
