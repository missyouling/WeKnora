#!/bin/bash
set -e
export PATH="$HOME/.local/bin:$PATH"
cd /opt/weknora-dev/docreader
echo "=== 移除旧 Windows venv（若存在）==="
rm -rf .venv
echo "=== uv venv 重建 ==="
uv venv --python 3.11 .venv 2>&1 | tail -3
echo "=== uv sync（按 uv.lock）==="
uv sync --frozen 2>&1 | tail -8
echo "=== 验证关键依赖 ==="
.venv/bin/python -c "import grpc, pypdf, pydantic, markitdown, opendataloader; print('IMPORTS_OK')" 2>&1 | tail -3
echo "=== DONE ==="
