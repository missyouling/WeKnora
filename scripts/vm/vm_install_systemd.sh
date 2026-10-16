#!/bin/bash
set -e
DEV=/opt/weknora-dev
ENVF=$DEV/.env

echo "=== 0) 前置校验与自愈（防止重装后踩历史坑） ==="

# 0.1 二进制必须存在且可执行。
# 历史坑：server_linux.exe 权限变为 -rw-r--r-- 时，systemctl 报 203/EXEC
#         "Permission denied" 并 5s 间隔无限 crash-loop；旧进程不重启时毫无症状。
if [ ! -f "$DEV/server_linux.exe" ]; then
  echo "  [FATAL] 缺少 $DEV/server_linux.exe，请先在 VM 上执行："
  echo "          cd $DEV && CGO_ENABLED=1 go build -o server_linux.exe ./cmd/server"
  exit 1
fi
chmod +x "$DEV/server_linux.exe"
echo "  [OK] 二进制可执行: $(ls -l "$DEV/server_linux.exe" | cut -d' ' -f1)"

# 0.2 .env 必须存在（systemd 的 EnvironmentFile 指向它，缺了服务起不来）。
if [ ! -f "$ENVF" ]; then
  echo "  [FATAL] 缺少 $ENVF（systemd EnvironmentFile 依赖它）"
  exit 1
fi

# 0.3 JIEBA_DICT_DIR 必须显式落 .env。
# 历史坑：该变量为空时 internal/types/evaluation.go 的 newJieba() 回退到编译期
#         Go module cache 路径（/root/go/pkg/mod/.../jieba.dict.utf8），VM 上该
#         cache 被清理后启动即 panic: Dictionary file does not exist，进程 status=2 秒退。
# 教训：进程级配置一律落 .env，不能依赖 shell 环境或 vm_run_server.sh——systemd 不继承。
if grep -q '^JIEBA_DICT_DIR=' "$ENVF"; then
  sed -i 's|^JIEBA_DICT_DIR=.*|JIEBA_DICT_DIR=/opt/weknora-dev/jieba_dict|' "$ENVF"
  echo "  [OK] JIEBA_DICT_DIR 已修正"
else
  printf 'JIEBA_DICT_DIR=/opt/weknora-dev/jieba_dict\n' >> "$ENVF"
  echo "  [OK] JIEBA_DICT_DIR 已追加（原 .env 缺失）"
fi

# 0.4 gojieba 5 个词典必须齐备（缺任一同样会启动 panic）。
JIEBA_DIR=/opt/weknora-dev/jieba_dict
missing=0
for f in jieba.dict.utf8 hmm_model.utf8 user.dict.utf8 idf.utf8 stop_words.utf8; do
  if [ ! -f "$JIEBA_DIR/$f" ]; then
    echo "  [WARN] 缺词典: $JIEBA_DIR/$f"
    missing=1
  fi
done
if [ "$missing" = "1" ]; then
  echo "  [WARN] 词典不齐，后端会 panic。可从 VPS 拷贝："
  echo "         scp -P 5522 -r root@43.155.185.216:/home/weknora-daily/jieba_dict $DEV/"
fi
[ "$missing" = "0" ] && echo "  [OK] gojieba 5 个词典齐备"

# 0.5 数据库连接方式提醒（免费版 Supabase：VM 无 IPv6，只能用共享池 Session 模式）。
echo "  [INFO] 当前 DB 配置:"
grep -E '^DB_(HOST|USER|PORT|SSLMODE)=' "$ENVF" | sed 's/^/         /' || true
if grep -q '^DB_HOST=db\.' "$ENVF"; then
  echo "  [WARN] DB_HOST 是直连域名(db.*)，但 VM 无 IPv6 出站、内网 DNS 劫持到 198.18.x.x，"
  echo "         直连必失败。请改用共享池 Session 模式："
  echo "           DB_HOST=aws-0-<REGION>.pooler.supabase.com"
  echo "           DB_USER=postgres.<PROJECT_REF>   # 后缀即 Supavisor 租户标识，不可省"
fi

echo "=== 1) 停掉现有 nohup 进程 ==="
pkill -f server_linux.exe 2>/dev/null || true
pkill -f "sh -c vite" 2>/dev/null || true
pkill -f "node .*/vite" 2>/dev/null || true
sleep 2

echo "=== 2) 禁用 apt redis-server（避免开机自启冲突）==="
systemctl disable redis-server.service 2>/dev/null || true
systemctl stop redis-server.service 2>/dev/null || true
echo "redis-server disabled: $(systemctl is-enabled redis-server.service 2>/dev/null)"

echo "=== 3) 写 systemd unit ==="
cat > /etc/systemd/system/weknora-backend.service <<'EOF'
[Unit]
Description=WeKnora Backend (dev VM)
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/weknora-dev
EnvironmentFile=/opt/weknora-dev/.env
ExecStart=/opt/weknora-dev/server_linux.exe
Restart=always
RestartSec=5
StandardOutput=append:/opt/weknora-dev/data/server_vm.log
StandardError=append:/opt/weknora-dev/data/server_vm.log

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/weknora-vite.service <<'EOF'
[Unit]
Description=WeKnora Frontend Vite Dev Server (dev VM)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/weknora-dev/frontend
Environment=PATH=/usr/local/node22/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ExecStart=/usr/local/node22/bin/npm run dev
Restart=always
RestartSec=5
StandardOutput=append:/opt/weknora-dev/data/vite_vm.log
StandardError=append:/opt/weknora-dev/data/vite_vm.log

[Install]
WantedBy=multi-user.target
EOF

echo "=== 4) 加载并启用 ==="
systemctl daemon-reload
systemctl enable weknora-backend.service weknora-vite.service

echo "=== 5) 启动服务 ==="
systemctl start weknora-backend.service
systemctl start weknora-vite.service

echo "=== 6) 等待就绪（后端为 300MB+ 二进制，初始化需 10~60s） ==="
ok=""
for i in $(seq 1 20); do
  st=$(systemctl is-active weknora-backend.service 2>/dev/null || true)
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 http://127.0.0.1:8080/health 2>/dev/null || true)
  echo "  [$i] active=$st health=$code"
  if [ "$st" = "active" ] && [ "$code" = "200" ]; then ok=1; break; fi
  # 早停：若服务已 failed/反复重启，不要白等
  if [ "$st" = "failed" ]; then echo "  [ERROR] 服务进入 failed 状态，提前终止等待"; break; fi
  sleep 6
done

echo "=== 7) 状态 ==="
systemctl is-active weknora-backend.service weknora-vite.service
systemctl status weknora-backend.service --no-pager -l | head -8
echo "--- vite ---"
systemctl status weknora-vite.service --no-pager -l | head -8

echo "=== 8) 端口验证 ==="
ss -tlnp | grep -E ':(8080|5173|50051)\b' || echo "  [WARN] 未听到 8080/5173/50051"

echo "=== 9) 结果判定与诊断指引 ==="
if [ "$ok" = "1" ]; then
  echo "  [OK] 后端就绪，/health 返回 200"
else
  echo "  [FAIL] 后端未就绪。按以下顺序排查："
  echo "   1) 权限问题（203/EXEC Permission denied）:"
  echo "        chmod +x $DEV/server_linux.exe && systemctl reset-failed weknora-backend && systemctl restart weknora-backend"
  echo "   2) jieba 词典 panic（Dictionary file does not exist）:"
  echo "        确认 .env 有 JIEBA_DICT_DIR=/opt/weknora-dev/jieba_dict 且 5 个词典齐备"
  echo "   3) 数据库连不上:"
  echo "        VM 无 IPv6，必须用共享池 Session 模式（DB_USER=postgres.<PROJECT_REF>）"
  echo "   4) 看日志:"
  echo "        journalctl -u weknora-backend -n 50 --no-pager"
  echo "        tail -50 $DEV/data/server_vm.log"
  exit 1
fi
echo "=== DONE ==="
