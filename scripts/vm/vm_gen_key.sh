#!/bin/bash
echo "=== 1) 生成 ed25519 key（无口令）==="
if [ ! -f ~/.ssh/id_ed25519 ]; then
  ssh-keygen -t ed25519 -C "weknora-dev-vm@10.10.10.239" -N "" -f ~/.ssh/id_ed25519
  echo "已生成"
else
  echo "已存在，复用"
fi
echo "=== 2) 公钥内容 ==="
cat ~/.ssh/id_ed25519.pub
echo "=== 3) 测试 GitHub 连通（未添加 key 会失败，预期）==="
timeout 12 ssh -o StrictHostKeyChecking=no -o BatchMode=yes -T git@github.com 2>&1 | head -2 || true
echo "=== 4) 测试 VPS 连通（未添加 key 会失败，预期）==="
timeout 12 ssh -o StrictHostKeyChecking=no -o BatchMode=yes -p 5522 root@43.155.185.216 'echo VPS_OK' 2>&1 | head -2 || true
echo "=== DONE ==="
