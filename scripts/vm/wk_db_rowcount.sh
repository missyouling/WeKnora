#!/usr/bin/env bash
# P2-2 旧库下线「观察期」：每日记录新库关键表行数并输出增量。
#
# 用途：旧库（kunsjmpkvfjjzcotdgrg，新加坡）下线前需观察 2 周，确认新库写入正常。
#      本脚本每天跑一次，把关键表行数追加到 TSV，便于看趋势与增量。
#
# 在 VM 上运行： bash /opt/weknora-dev/scripts/vm/wk_db_rowcount.sh
# cron（每天 09:00）： 0 9 * * * bash /opt/weknora-dev/scripts/vm/wk_db_rowcount.sh >> /var/log/weknora-rowcount.log 2>&1
#
# 注意：只连新库（配置取自 VM .env）。旧库凭据不在仓库也不在 VM，
#      旧库「零应用连接」的结论由 Supabase Dashboard / MCP 单独确认。

set -euo pipefail

APP=${APP_DIR:-/opt/weknora-dev}
ENVF=$APP/.env
LOG=${ROWCOUNT_LOG:-$APP/.workbuddy/db-rowcount.tsv}
TABLES="tenants knowledges chunks embeddings messages"

if [ ! -f "$ENVF" ]; then
  echo "ERROR: 缺少 $ENVF" >&2
  exit 1
fi

set -a
# shellcheck disable=SC1090
. "$ENVF"
set +a
export PGPASSWORD="$DB_PASSWORD"

mkdir -p "$(dirname "$LOG")"
if [ ! -f "$LOG" ]; then
  printf 'date\ttime\t%s\n' "$(echo $TABLES | tr ' ' '\t')" >"$LOG"
fi

vals=()
for t in $TABLES; do
  n=$(psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" \
    -tAc "select count(*) from public.$t" 2>/dev/null | tr -d '[:space:]')
  vals+=("${n:-ERR}")
done

now_date=$(date +%F)
now_time=$(date +%H:%M)
printf '%s\t%s\t%s\n' "$now_date" "$now_time" "$(IFS=$'\t'; echo "${vals[*]}")" >>"$LOG"

echo "=== 新库关键表行数 $now_date $now_time ==="
paste <(printf '%s\n' $TABLES) <(printf '%s\n' "${vals[@]}") | column -t 2>/dev/null ||
  paste <(printf '%s\n' $TABLES) <(printf '%s\n' "${vals[@]}")

# 与上一次记录对比增量（必须跳过表头行，否则首次运行时会拿表头当历史）
mapfile -t data_lines < <(grep -v '^date\s' "$LOG")
if [ "${#data_lines[@]}" -ge 2 ]; then
  prev_line=${data_lines[-2]}
  curr_line=${data_lines[-1]}
  echo ""
  echo "=== 较上次增量 ==="
  paste <(printf '%s\n' $TABLES) \
    <(printf '%s\n' "$(echo "$prev_line" | cut -f3- | tr '\t' '\n')") \
    <(printf '%s\n' "$(echo "$curr_line" | cut -f3- | tr '\t' '\n')") |
    awk '{ delta = ($3 ~ /^[0-9]+$/ && $2 ~ /^[0-9]+$/) ? $3 - $2 : "n/a"
           printf "%-14s 上次 %-10s 本次 %-10s 增量 %s\n", $1, $2, $3, delta }'
else
  echo ""
  echo "(首次记录，无历史可对比)"
fi
echo ""
echo "日志: $LOG"
