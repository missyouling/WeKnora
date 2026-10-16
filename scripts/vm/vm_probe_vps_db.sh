#!/bin/bash
echo "=== 1) weknora-daily 容器状态与启动时间 ==="
docker ps --filter name=weknora-daily --format '{{.Names}} started={{.RunningFor}} status={{.Status}}'
docker inspect weknora-daily --format 'StartedAt={{.State.StartedAt}} image={{.Config.Image}}'
echo
echo "=== 2) 当前镜像 digest ==="
docker inspect weknora-daily --format '{{.Image}}'
echo
echo "=== 3) 数据库该条 knowledge 当前状态 ==="
DBHOST=$(grep '^DB_HOST=' /home/weknora-daily/.env | cut -d= -f2)
DBUSER=$(grep '^DB_USER=' /home/weknora-daily/.env | cut -d= -f2)
DBPASS=$(grep '^DB_PASSWORD=' /home/weknora-daily/.env | cut -d= -f2)
DBNAME=$(grep '^DB_NAME=' /home/weknora-daily/.env | cut -d= -f2)
if [ -z "$DBUSER" ]; then DBUSER=$(grep '^POSTGRES_USER=' /home/weknora-daily/.env | cut -d= -f2); fi
if [ -z "$DBPASS" ]; then DBPASS=$(grep '^POSTGRES_PASSWORD=' /home/weknora-daily/.env | cut -d= -f2); fi
if [ -z "$DBNAME" ]; then DBNAME=$(grep '^POSTGRES_DB=' /home/weknora-daily/.env | cut -d= -f2); fi
echo "host=$DBHOST user=$DBUSER db=$DBNAME"
PGPASSWORD="$DBPASS" psql -h "$DBHOST" -U "$DBUSER" -d "$DBNAME" -t -c "SELECT id, file_name, file_type, parse_status, custom_metadata->>'extract_status' AS extract_status, custom_metadata->>'extract_error' AS extract_error, updated_at FROM knowledges WHERE id='4d4d9d6d-ba50-4416-b58a-7da4b04af339';" 2>&1 | head -5
echo "=== DONE ==="
