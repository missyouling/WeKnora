import psycopg2
conn = psycopg2.connect(host="db.kunsjmpkvfjjzcotdgrg.supabase.co", port=5432,
    user="postgres", password="7aPBAINUbuMmTD", dbname="postgres", sslmode="require", connect_timeout=15)
cur = conn.cursor()
cur.execute("select table_name from information_schema.tables where table_schema='public' and table_name like '%fleet%'")
print("=== fleet tables ===")
for r in cur.fetchall(): print(r[0])
print("=== categories columns ===")
cur.execute("select column_name,data_type from information_schema.columns where table_name='fleet_categories' order by ordinal_position")
try:
    for r in cur.fetchall(): print(r)
except Exception as e:
    print("err", e)
print("=== fleet_categories rows ===")
try:
    cur.execute("select * from fleet_categories limit 60")
    cols=[d[0] for d in cur.description]
    print(cols)
    for r in cur.fetchall(): print(r)
except Exception as e:
    print("err", e)
conn.close()
