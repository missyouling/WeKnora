# -*- coding: utf-8 -*-
"""备份 9d3f1b3a 的 custom_metadata 与 chunks（重跑提取前防破坏）。"""
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
cur.execute("select table_schema, table_name from information_schema.tables where table_name like '%knowledge%' order by table_schema, table_name limit 40")
for r in cur.fetchall():
    print("table:", r)
kid = "9d3f1b3a-f622-4e2c-add5-47687afa91db"
cur.execute("select id, custom_metadata, created_at from knowledges where id=%s", (kid,))
row = cur.fetchone()
cur.execute("select id, content from chunks where knowledge_id=%s order by chunk_index", (kid,))
chunks = cur.fetchall()
cur.execute("select id, created_at from knowledge_bases where id=%s", ("3919e8d5-190e-4e1f-adca-77af7f117cfc",))
print("kb row:", cur.fetchone())
meta = row[1] or {}
invs = meta.get("invoices", [])
out = {
    "knowledge": {"id": row[0], "custom_metadata": meta, "created_at": str(row[2])},
    "chunks": [{"id": c[0], "content": c[1]} for c in chunks],
}
with open(r"D:\Downloads\WeKnora-main\backup_9d3f1b3a.json", "w", encoding="utf-8") as f:
    json.dump(out, f, ensure_ascii=False, default=str)
print("invoices:", len(invs))
print("issuer sample:", [str(i.get("issuer") or "-")[:8] for i in invs[:6]])
print("buyer:", invs[0].get("buyer_name") if invs else "", "| seller:", invs[0].get("seller_name") if invs else "")
print("chunks:", len(chunks))
print("backup written")
