#!/bin/sh
# 存量发票无损重提（第 24 轮任务二）。
#
# 目标：用新的 HTML 表格规则 + 字段补齐重跑一遍，补齐车牌 / 开票人 / 金额四段。
#
# 安全性设计：
#   1. **绝不传 cert_type** —— 后端 applyInvoiceUserCategoryPrecedence 的分类优先级是
#      「请求 cert > invoice_category > fleet_cert_type」，不传就等于沿用库里已存的
#      分类，重提不会改动任何发票的分类（这是本任务的红线）。
#   2. 规则命中即短路 LLM（`if ruleOK && !ruleShort`），所以批量重提基本不烧 token；
#      只有规则完全没命中的文件才会走模型。
#   3. 逐个串行、每个之间留间隔 —— 提取有按知识 ID 的内存防重入锁，并发会被跳过。
#   4. 先备份 custom_metadata（调用方负责），脚本只做重提。
#
# 用法：batch-reextract-invoices.sh [文件筛选 SQL 条件] [每批数量]
set -e
cd /opt/weknora-dev
. ./.env

FILTER="${1:-custom_metadata->>'kind' = 'invoice'}"
LIMIT="${2:-200}"

TOK=$(cat /tmp/pw/token.txt)
KB="${KB_ID:-3919e8d5-190e-4e1f-adca-77af7f117cfc}"

echo "[reextract] 筛选: $FILTER  上限: $LIMIT  知识库: $KB"

# 取出待重提的知识 ID（按更新时间倒序，先补最近被看到的）
IDS=$(env PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -A \
  -c "SELECT id FROM knowledges
      WHERE deleted_at IS NULL AND knowledge_base_id = '$KB'
        AND ($FILTER)
      ORDER BY updated_at DESC LIMIT $LIMIT;")

total=0
ok=0
fail=0
for kid in $IDS; do
  total=$((total + 1))
  code=$(curl -s -o /tmp/reextract_one.json -w '%{http_code}' -X POST \
    -H "Authorization: Bearer $TOK" -H "Content-Type: application/json" \
    -d '{}' \
    "http://127.0.0.1:8080/api/v1/knowledge-bases/$KB/knowledge/$kid/extract-invoice")
  if [ "$code" = "200" ]; then
    ok=$((ok + 1))
  else
    fail=$((fail + 1))
    echo "  [warn] $kid -> HTTP $code"
  fi
  # 提取有按 ID 的内存锁 + 规则短路，间隔 1 秒足够避免互相踩
  sleep 1
done

echo "[reextract] 完成：提交 $total 份，成功 $ok，失败 $fail"
