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
import psycopg2
conn = psycopg2.connect(host=env.get("DB_HOST",""), port=env.get("DB_PORT","5432"),
                        user=env.get("DB_USER",""), password=env.get("DB_PASSWORD",""),
                        dbname=env.get("DB_NAME","postgres"), connect_timeout=20)
cur = conn.cursor()
cur.execute("""
    SELECT k.id, k.title, k.parse_status, k.summary_status,
           k.deleted_at, k.created_at, k.updated_at,
           k.custom_metadata->>'extract_status' AS extract_status,
           k.custom_metadata->>'extract_error' AS extract_error
    FROM knowledges k WHERE k.id = %s
""", ("9d3f1b3a-f622-4e2c-add5-47687afa91db",))
print("9d3f1b3a:", cur.fetchone())

# 查所有正在 deleting 的记录（看是否用户手动删）
cur.execute("""
    SELECT k.id, k.title, k.parse_status, k.deleted_at, k.updated_at
    FROM knowledges k
    WHERE k.tenant_id = 10000 AND k.parse_status = 'deleting'
    ORDER BY k.updated_at DESC LIMIT 10
""")
print("deleting rows:")
for r in cur.fetchall():
    print("  ", r)
cur.close(); conn.close()
