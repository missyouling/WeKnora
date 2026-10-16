#!/bin/bash
echo "=== 1) GitHub SSH 认证测试 ==="
timeout 15 ssh -o StrictHostKeyChecking=no -o BatchMode=yes -T git@github.com 2>&1 | head -3
echo "exit=$?"
echo
echo "=== 2) VPS SSH 直连测试 ==="
timeout 15 ssh -o StrictHostKeyChecking=no -o BatchMode=yes -p 5522 root@43.155.185.216 'hostname; echo VPS_OK' 2>&1 | head -3
echo
echo "=== 3) VPS 上关键状态速查 ==="
timeout 20 ssh -o StrictHostKeyChecking=no -o BatchMode=yes -p 5522 root@43.155.185.216 'cd /home/weknora-daily && docker compose ps --format "{{.Name}} {{.Status}}" 2>/dev/null | head -6; curl -s -m 5 http://localhost:8080/health' 2>&1 | head -10
echo "=== DONE ==="
