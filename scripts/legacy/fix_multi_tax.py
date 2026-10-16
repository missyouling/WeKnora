# -*- coding: utf-8 -*-
"""确定性回填：1347567f 电信发票 items 错位修复（展示层已闭环，数据层回填）"""
import psycopg2, json

env = {}
with open(r"D:\Downloads\WeKnora-main\.env", encoding="utf-8", errors="ignore") as f:
    for line in f:
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        k, v = line.split("=", 1)
        env[k.strip()] = v.strip()

conn = psycopg2.connect(host=env["DB_HOST"], port=env.get("DB_PORT", "5432"), user=env["DB_USER"],
                        password=env["DB_PASSWORD"], dbname=env.get("DB_NAME", "postgres"), connect_timeout=20)
cur = conn.cursor()

TID = "1347567f-75b1-43f3-820c-c9f365c8bf82"
cur.execute("SELECT custom_metadata FROM knowledges WHERE id = %s", (TID,))
row = cur.fetchone()
assert row and row[0], "record not found"
meta = row[0] if isinstance(row[0], dict) else json.loads(row[0])
print("=== BEFORE ===")
print(json.dumps(meta.get("invoices", [{}])[0], ensure_ascii=False, indent=1)[:2000])

inv = meta["invoices"][0]
inv["items"] = [
    {"name": "通信线路服务", "qty": 1, "price": 4.21, "amount": 4.21, "tax_rate": 0.13, "tax_amount": 0.55},
    {"name": "基础电信服务", "qty": 1, "price": 502.89, "amount": 502.89, "tax_rate": 0.09, "tax_amount": 45.26},
    {"name": "增值电信服务", "qty": 1, "price": 320.84, "amount": 320.84, "tax_rate": 0.06, "tax_amount": 19.25},
]
inv["amount"] = 827.94
inv["tax"] = 65.06
inv["total_amount"] = 893.0
inv["tax_rate"] = 0.09  # 金额最大明细行(502.89)税率 9%

cur.execute("UPDATE knowledges SET custom_metadata = %s::jsonb WHERE id = %s",
            (json.dumps(meta, ensure_ascii=False), TID))
conn.commit()
print("\n=== AFTER ===")
print(json.dumps(meta.get("invoices", [{}])[0], ensure_ascii=False, indent=1))
print("\nUPDATED OK rows:", cur.rowcount)
conn.close()
