import os, re, sys, json
import psycopg2

# 读取 .env（值不回显）
def load_env(path):
    env = {}
    with open(path, 'r', encoding='utf-8', errors='ignore') as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith('#') or '=' not in line:
                continue
            k, v = line.split('=', 1)
            env[k.strip()] = v.strip().strip('"').strip("'")
    return env

env = {}
for p in [r'D:\Downloads\WeKnora-main\.env', r'D:\Downloads\WeKnora-main\deploy\.env']:
    if os.path.exists(p):
        env.update(load_env(p))

host = env.get('DB_HOST') or env.get('POSTGRES_HOST')
port = env.get('DB_PORT') or '5432'
user = env.get('DB_USER') or env.get('POSTGRES_USER')
pwd = env.get('DB_PASSWORD') or env.get('POSTGRES_PASSWORD') or env.get('DATABASE_PASSWORD')
db = env.get('DB_NAME') or env.get('POSTGRES_DB') or env.get('DATABASE_NAME')
if env.get('DATABASE_URL'):
    # postgres://user:pwd@host:port/db
    m = re.match(r'postgres(?:ql)?://([^:]+):([^@]+)@([^:/]+):(\d+)/(\S+)', env['DATABASE_URL'])
    if m:
        user, pwd, host, port, db = m.group(1), m.group(2), m.group(3), m.group(4), m.group(5)

print('conn info: host=%s port=%s db=%s user=%s (pwd hidden)' % (host, port, db, user))

conn = psycopg2.connect(host=host, port=port, user=user, password=pwd, dbname=db, connect_timeout=10)
conn.autocommit = True
cur = conn.cursor()

# 1) 两个知识库
cur.execute("""
  SELECT id, name, COALESCE(description,'') , created_at
  FROM knowledge_bases
  WHERE name IN ('车队管理','日常事务-车队')
  ORDER BY created_at
""")
rows = cur.fetchall()
print('\n=== knowledge_bases ===')
for r in rows:
    print('id=%s name=%s created=%s' % (r[0], r[1], r[3]))
    print('   desc=%s' % r[2][:80])

# 2) 每个库的文件数（按 not deleted 统计）
ids = [r[0] for r in rows]
if ids:
    cur.execute("""
      SELECT kb_id, COUNT(*) FILTER (WHERE state IS DISTINCT FROM 'deleted'), COUNT(*)
      FROM knowledges WHERE kb_id = ANY(%s) GROUP BY kb_id
    """, (ids,))
    print('\n=== knowledges count ===')
    for r in cur.fetchall():
        print('kb_id=%s active=%s total=%s' % (r[0], r[1], r[2]))

    # 3) 关联表探查：找引用 knowledge_id 的业务表
    cur.execute("""
      SELECT table_name FROM information_schema.tables
      WHERE table_schema='public' AND table_name NOT IN ('schema_migrations','_prisma_migrations')
        AND (table_name ~ 'invoice|fleet|contract|record|cert|regulat|utility|award')
      ORDER BY table_name
    """)
    tables = [r[0] for r in cur.fetchall()]
    print('\n=== candidate business tables ===')
    print(tables)

    # 4) 业务表里按知识库归属统计（若存在 knowledge_id 列）
    for t in tables:
        try:
            cur.execute("""SELECT column_name FROM information_schema.columns WHERE table_name=%s""", (t,))
            cols = [c[0] for c in cur.fetchall()]
            kid_cols = [c for c in cols if c in ('knowledge_id','knowledgeIds','kb_id','knowledge_base_id')]
            if not kid_cols:
                continue
            for kc in kid_cols:
                cur.execute("""
                  SELECT kb.name, COUNT(*)
                  FROM %s t JOIN knowledges k ON k.id = t.%s::uuid
                  JOIN knowledge_bases kb ON kb.id = k.kb_id
                  WHERE kb.name IN ('车队管理','日常事务-车队')
                  GROUP BY kb.name
                """ % (t, kc))
                res = cur.fetchall()
                if res:
                    print('\n  %s.%s:' % (t, kc))
                    for r in res:
                        print('    %s -> %s rows' % (r[0], r[1]))
        except Exception as e:
            print('  skip %s: %s' % (t, str(e)[:120]))

cur.close(); conn.close()
print('\ndone')
