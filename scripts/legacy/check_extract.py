# -*- coding: utf-8 -*-
import os, json

def load_env(path):
    d = {}
    with open(path, "r", encoding="utf-8", errors="replace") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            k, v = line.split("=", 1)
            d[k.strip()] = v.strip()
    return d

env = load_env(r"D:\Downloads\WeKnora-main\.env")
host = env.get("DB_HOST", "")
port = env.get("DB_PORT", "5432")
user = env.get("DB_USER", "")
pwd = env.get("DB_PASSWORD", "")
db = env.get("DB_NAME", "postgres")

import psycopg2
conn = psycopg2.connect(host=host, port=port, user=user, password=pwd, dbname=db, connect_timeout=20)
cur = conn.cursor()

kid = "9d3f1b3a-f622-4e2c-add5-47687afa91db"
cur.execute("""
    SELECT k.id, k.title, k.parse_status, k.summary_status,
           jsonb_array_length(COALESCE(k.custom_metadata->'invoices','[]'::jsonb)) AS n,
           k.custom_metadata->>'extract_status' AS extract_status,
           k.updated_at
    FROM knowledges k
    WHERE k.id = %s
""", (kid,))
row = cur.fetchone()
print("ROW:", row)

cur.execute("""
    SELECT k.custom_metadata
    FROM knowledges k
    WHERE k.id = %s
""", (kid,))
meta = cur.fetchone()[0]
if isinstance(meta, str):
    meta = json.loads(meta)
inv = meta.get("invoices") or []
print("invoices:", len(inv))
plates = set()
rates = {}
for i, it in enumerate(inv[:25], 1):
    flds = it.get("fields") or {}
    plate = flds.get("车牌号", "")
    plates.add(plate)
    rates[str(it.get("tax_rate"))] = rates.get(str(it.get("tax_rate")), 0) + 1
    print(f"#{i:02d} no={it.get('invoice_no')} date={it.get('invoice_date')} cat={it.get('category')} "
          f"plate={plate} amount={it.get('amount')} rate={it.get('tax_rate')} tax={it.get('tax')} total={it.get('total_amount')}")
print("plates:", plates)
print("rates:", rates)

# 对照已成功记录 d1e97f39
cur.execute("""
    SELECT k.id, jsonb_array_length(COALESCE(k.custom_metadata->'invoices','[]'::jsonb)) AS n,
           k.custom_metadata->>'extract_status' AS extract_status
    FROM knowledges k
    WHERE k.id LIKE 'd1e97f39%%' OR k.title LIKE '%%电话费%%'
    ORDER BY k.created_at DESC LIMIT 5
""")
print("对照记录:", cur.fetchall())

cur.close()
conn.close()
