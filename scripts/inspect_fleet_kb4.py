import os, re
import psycopg2

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
pwd = env.get('DB_PASSWORD') or env.get('POSTGRES_PASSWORD')
db = env.get('DB_NAME') or env.get('POSTGRES_DB')
if env.get('DATABASE_URL'):
    m = re.match(r'postgres(?:ql)?://([^:]+):([^@]+)@([^:/]+):(\d+)/(\S+)', env['DATABASE_URL'])
    if m:
        user, pwd, host, port, db = m.group(1), m.group(2), m.group(3), m.group(4), m.group(5)

conn = psycopg2.connect(host=host, port=port, user=user, password=pwd, dbname=db, connect_timeout=10)
conn.autocommit = True
cur = conn.cursor()

OLD = 'b134e39a-859e-4943-9f89-bbbbb01e2384'  # 日常事务-车队
NEW = 'b4848deb-d284-4978-88b4-c7725d4b0215'  # 车队管理

# 1) 车队管理库的 1 个文件详情 + fleet_records 引用
cur.execute("""
  SELECT id, title, file_name, file_hash, parse_status, created_at
  FROM knowledges WHERE knowledge_base_id=%s AND deleted_at IS NULL
""", (NEW,))
print('=== NEW kb active files ===')
files_new = cur.fetchall()
for r in files_new:
    print('kid=%s | %s | %s | %s | %s' % (r[0], r[1], r[2], r[4], r[5]))

# 该文件在 fleet_records 的引用
for r in files_new:
    cur.execute("SELECT COUNT(*) FROM fleet_records WHERE doc_knowledge_id=%s AND deleted_at IS NULL", (r[0],))
    print('fleet_records refs for %s: %s' % (r[0][:8], cur.fetchone()[0]))

# 2) knowledge_extract_configs 两个库的配置
cur.execute("""SELECT column_name FROM information_schema.columns WHERE table_name='knowledge_extract_configs' ORDER BY ordinal_position""")
print('\nextract configs cols:', [c[0] for c in cur.fetchall()])
cur.execute("""
  SELECT kb.name, COUNT(*)
  FROM knowledge_extract_configs c JOIN knowledge_bases kb ON kb.id = c.knowledge_base_id
  WHERE kb.name IN ('车队管理','日常事务-车队')
  GROUP BY kb.name
""")
print('extract configs by kb:')
for r in cur.fetchall():
    print('  %s: %s' % (r[0], r[1]))

# 3) 全局搜其它表对这两个 kb 的引用
cur.execute("""
  SELECT table_name, column_name FROM information_schema.columns
  WHERE table_schema='public'
    AND (column_name IN ('knowledge_base_id','kb_id') OR column_name LIKE '%kb_id%')
  ORDER BY table_name
""")
refs = cur.fetchall()
print('\n=== tables with kb id columns ===')
for t, c in refs:
    if t in ('knowledges','knowledge_bases'):
        continue
    try:
        cur.execute("""
          SELECT COUNT(*) FROM %s t JOIN knowledge_bases kb
          ON kb.id = t.%s WHERE kb.name IN ('车队管理','日常事务-车队')
        """ % (t, c))
        n = cur.fetchone()[0]
        if n:
            print('  %s.%s -> %s rows reference these KBs' % (t, c, n))
    except Exception as e:
        pass

cur.close(); conn.close()
print('\ndone')
