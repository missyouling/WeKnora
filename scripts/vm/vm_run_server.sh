#!/bin/bash
cd /opt/weknora-dev
# 加载 .env（兼容注释行、空行、带 # 值）
set -a
while IFS='=' read -r k v; do
  case "$k" in
    ''|'#'*) continue ;;
  esac
  if [ -n "$k" ]; then
    export "$k"="$v"
  fi
done < /opt/weknora-dev/.env
set +a

# 清理旧进程
pkill -f server_linux.exe 2>/dev/null || true
sleep 1

nohup ./server_linux.exe > /opt/weknora-dev/data/server_vm.log 2>&1 &
echo "SERVER_PID: $!"
sleep 6
if ss -tlnp | grep -q ':8080'; then
  echo SERVER_UP
else
  echo SERVER_DOWN
  echo "--- 日志尾部 ---"
  strings /opt/weknora-dev/data/server_vm.log | tail -20
fi
