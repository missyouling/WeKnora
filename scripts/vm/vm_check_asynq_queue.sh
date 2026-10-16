#!/bin/bash
cd /home/weknora-daily || exit 1
set -a; . ./.env; set +a
PW="$REDIS_PASSWORD"
Q='asynq:{default}'
echo '=== dead (619d6578) ==='
docker exec weknora-redis redis-cli -a "$PW" --no-auth-warning ZRANGE "${Q}:dead" 0 -1 WITHSCORES | grep -B1 -A1 '619d6578' | head -12
echo '=== retry (619d6578) ==='
docker exec weknora-redis redis-cli -a "$PW" --no-auth-warning ZRANGE "${Q}:retry" 0 -1 WITHSCORES | grep -B1 -A1 '619d6578' | head -12
echo '=== failed hash (619d6578) ==='
docker exec weknora-redis redis-cli -a "$PW" --no-auth-warning HGETALL "${Q}:failed" | grep -B1 -A1 '619d6578' | head -8
echo '=== completed contains count ==='
docker exec weknora-redis redis-cli -a "$PW" --no-auth-warning ZRANGE "${Q}:completed" 0 -1 | grep -c '619d6578'
echo '=== dead total / retry total / completed total ==='
docker exec weknora-redis redis-cli -a "$PW" --no-auth-warning ZCARD "${Q}:dead"
docker exec weknora-redis redis-cli -a "$PW" --no-auth-warning ZCARD "${Q}:retry"
docker exec weknora-redis redis-cli -a "$PW" --no-auth-warning ZCARD "${Q}:completed"
echo '=== all business:extract in dead ==='
docker exec weknora-redis redis-cli -a "$PW" --no-auth-warning ZRANGE "${Q}:dead" 0 -1 | grep -c 'business:extract'
echo '=== all business:extract in retry ==='
docker exec weknora-redis redis-cli -a "$PW" --no-auth-warning ZRANGE "${Q}:retry" 0 -1 | grep -c 'business:extract'
