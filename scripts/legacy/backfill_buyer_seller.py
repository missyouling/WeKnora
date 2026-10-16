# -*- coding: utf-8 -*-
"""回填 9d3f1b3a 购/销方名称（依据同 KB 同来源 LLM 提取文件 1f8cee13）；
并打印 1f8cee13 的购/销方确认。"""
import json, sys
sys.path.insert(0, r"D:\Downloads\WeKnora-main")
env = {}
with open(r"D:\Downloads\WeKnora-main\.env", encoding="utf-8") as f:
    for line in f:
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        k, v = line.split("=", 1)
        env[k.strip()] = v.strip()
import psycopg2
conn = psycopg2.connect(host=env["DB_HOST"], port=env.get("DB_PORT", "5432"),
                        user=env["DB_USER"], password=env["DB_PASSWORD"],
                        dbname=env["DB_NAME"], sslmode=env.get("DB_SSLMODE", "require"))
cur = conn.cursor()

# 1) 旁证文件购/销方
cur.execute("select custom_metadata from knowledges where id=%s", ("1f8cee13-2311-4488-a75c-e9b5f78f5f16",))
row = cur.fetchone()
meta1 = (row[0] or {}) if row else {}
inv1 = (meta1.get("invoices") or [{}])[0]
print("1f8cee13 buyer:", inv1.get("buyer_name"), "| seller:", inv1.get("seller_name"),
      "| buyer_tax:", inv1.get("buyer_tax_no"), "| seller_tax:", inv1.get("seller_tax_no"))

# 2) 目标文件当前购/销方
kid = "9d3f1b3a-f622-4e2c-add5-47687afa91db"
cur.execute("select custom_metadata from knowledges where id=%s", (kid,))
row = cur.fetchone()
meta = (row[0] or {}) if row else {}
invs = meta.get("invoices") or []
print("9d3f1b3a invoices:", len(invs), "| buyer_tax0:", invs[0].get("buyer_tax_no") if invs else "")
print("9d3f1b3a seller_tax0:", invs[0].get("seller_tax_no") if invs else "")

# 3) 回填购买方名称：全部 20 张 buyer_tax_no == 91500227MA5U54AJ0A 与旁证一致 → 重庆星达铜业有限公司。
#    销售方每张税号不同（多主体高速运营公司）且名称文本层乱码，不做推断回填。
buyer_name = "重庆星达铜业有限公司"
changed = 0
for inv in invs:
    if inv.get("buyer_tax_no") == "91500227MA5U54AJ0A" and not inv.get("buyer_name"):
        inv["buyer_name"] = buyer_name
        changed += 1
if changed:
    meta["invoices"] = invs
    cur.execute("update knowledges set custom_metadata=%s where id=%s", (json.dumps(meta, ensure_ascii=False, default=str), kid))
    conn.commit()
    print("backfilled buyer_name rows:", changed)
else:
    print("no rows to backfill")

