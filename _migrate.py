import psycopg2
conn = psycopg2.connect(host="db.kunsjmpkvfjjzcotdgrg.supabase.co", port=5432,
    user="postgres", password="7aPBAINUbuMmTD", dbname="postgres", sslmode="require", connect_timeout=15)
cur = conn.cursor()

sql = """
UPDATE fleet_records r SET record_type = c.target
FROM (
  SELECT name,
    CASE scope WHEN 'driver' THEN 'driver-archive'
               WHEN 'maintain' THEN 'maintain-archive'
               ELSE 'vehicle-archive' END AS target
  FROM fleet_categories WHERE deleted_at IS NULL
) c
WHERE r.doc_type = c.name AND r.deleted_at IS NULL AND r.record_type <> c.target
"""
cur.execute(sql)
print("rows fixed:", cur.rowcount)
conn.commit()

print("=== after: records per record_type / doc_type ===")
cur.execute("""
  SELECT record_type, doc_type, count(*) FROM fleet_records
  WHERE deleted_at IS NULL GROUP BY 1,2 ORDER BY 1,2
""")
for r in cur.fetchall():
    print(r)
conn.close()
