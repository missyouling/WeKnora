# -*- coding: utf-8 -*-
import os

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
kid = "9d3f1b3a-f622-4e2c-add5-47687afa91db"
kb = "3919e8d5-190e-4e1f-adca-77af7f117cfc"

# 1. knowledge 恢复（幂等）
cur.execute("UPDATE knowledges SET deleted_at=NULL, parse_status='completed' WHERE id=%s AND deleted_at IS NOT NULL", (kid,))
print("knowledge restored rows:", cur.rowcount)

# 2. chunks 恢复（幂等）
cur.execute("UPDATE chunks SET deleted_at=NULL WHERE knowledge_id=%s AND deleted_at IS NOT NULL", (kid,))
print("chunks restored rows:", cur.rowcount)

# 3. tag 关系重建：找标签「通行费」
cur.execute("SELECT id FROM knowledge_tags WHERE tenant_id=10000 AND knowledge_base_id=%s AND name='通行费' LIMIT 1", (kb,))
tag = cur.fetchone()
print("tag:", tag)
if tag:
    cur.execute("SELECT count(*) FROM knowledge_tag_relations WHERE knowledge_id=%s AND tag_id=%s", (kid, tag[0]))
    if cur.fetchone()[0] == 0:
        cur.execute("""
            INSERT INTO knowledge_tag_relations (knowledge_id, tag_id, created_at)
            VALUES (%s, %s, now())
        """, (kid, tag[0]))
        print("tag relation inserted")
    else:
        print("tag relation exists")
else:
    print("tag 通行费 not found, skip")

conn.commit()

# 4. 复查
cur.execute("SELECT id, parse_status, deleted_at FROM knowledges WHERE id=%s", (kid,))
print("knowledge:", cur.fetchone())
cur.execute("SELECT count(*) FROM chunks WHERE knowledge_id=%s AND deleted_at IS NULL", (kid,))
print("alive chunks:", cur.fetchone()[0])
cur.execute("SELECT count(*) FROM knowledge_tag_relations WHERE knowledge_id=%s", (kid,))
print("tag relations:", cur.fetchone()[0])
cur.execute("SELECT jsonb_array_length(COALESCE(custom_metadata->'invoices','[]'::jsonb)), custom_metadata->>'extract_status' FROM knowledges WHERE id=%s", (kid,))
print("invoices:", cur.fetchone())
cur.close(); conn.close()
print("DONE")
