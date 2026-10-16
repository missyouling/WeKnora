#!/bin/bash
set -e
echo "=== apt 安装 python 工具链 ==="
apt-get install -y -qq python3-pip python3-venv python3-dev >/dev/null 2>&1 || apt-get install -y python3-pip python3-venv python3-dev
echo "=== pip 安装 uv ==="
pip3 install -q --break-system-packages uv 2>&1 | tail -1 || true
export PATH="$HOME/.local/bin:$PATH"
export PATH="$HOME/.cargo/bin:$PATH"
echo "=== uv 版本 ==="
command -v uv && uv --version || echo "uv still missing, 试 pipx 或官方脚本"
if ! command -v uv >/dev/null 2>&1; then
  echo "=== 官方脚本重装 uv ==="
  curl -LsSf https://astral.sh/uv/install.sh -o /tmp/uv_install.sh
  sh /tmp/uv_install.sh || true
  export PATH="$HOME/.local/bin:$PATH"
  command -v uv && uv --version
fi
echo "=== DONE ==="
