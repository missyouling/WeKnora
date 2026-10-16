#!/bin/bash
echo "=== 1) 409/Conflict 记录（近 12h）==="
docker logs weknora-daily --since 12h 2>&1 | grep -iE "conflict|409|already in progress" | tail -30
echo
echo "=== 2) 4d4d9d6d 全部 extract 相关日志（近 12h）==="
docker logs weknora-daily --since 12h 2>&1 | grep -E "4d4d9d6d" | grep -iE "extract|enqueue|task" | tail -40
echo
echo "=== 3) 4d4d9d6d 的文件信息（解析/提取状态）==="
grep -o '"id":"4d4d9d6d[^}]*' /tmp/../dev/null 2>/dev/null
docker logs weknora-daily --since 12h 2>&1 | grep -E "4d4d9d6d" | grep -E "custom_metadata|SaveInvoice|file_name|parse" | tail -20
echo "=== DONE ==="
