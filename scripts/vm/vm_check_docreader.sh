#!/bin/bash
cd /opt/weknora-dev/docreader
echo "=== pip list 关键包 ==="
.venv/bin/pip list 2>/dev/null | grep -iE "opendataloader|markitdown|textract|grpcio|pypdf|trafilatura|playwright|pypdfium"
echo "=== import 测试 ==="
.venv/bin/python - <<'PY'
mods = ["grpc", "pypdf", "pydantic", "markitdown", "trafilatura", "pypdfium2", "bs4", "lxml", "requests"]
ok, fail = [], []
for m in mods:
    try:
        __import__(m)
        ok.append(m)
    except Exception as e:
        fail.append((m, str(e)[:80]))
print("OK:", ok)
print("FAIL:", fail)
PY
echo "=== DONE ==="
