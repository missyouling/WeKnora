#!/bin/bash
cd /opt/weknora-dev
echo "=== 分支 ==="
git branch --show-current
echo "=== remote ==="
git remote -v
echo "=== git 身份 ==="
git config user.name; git config user.email
git config --global user.name 2>/dev/null; git config --global user.email 2>/dev/null
echo "=== HEAD ==="
git log -1 --oneline
echo "=== 工作区状态 ==="
git status --short | head -10
echo "=== ~/.ssh ==="
ls -la ~/.ssh/ 2>/dev/null || echo "无 ~/.ssh"
echo "=== DONE ==="
