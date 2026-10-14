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

OLD = 'b134e39a-859e-4943-9f89-bbbbb01e2384'
NEW = 'b4848deb-d284-4978-88b4-c7725d4b0215'

# NEW 库提取配置
cur.execute("""
  SELECT id, scope, cert_type, enabled, deleted_at IS NOT NULL AS del, created_at
  FROM knowledge_extract_configs WHERE knowledge_base_id=%s ORDER BY scope, cert_type
""", (NEW,))
print('=== NEW kb extract configs ===')
new_cfgs = cur.fetchall()
for r in new_cfgs:
    print('id=%s scope=%s cert=%s enabled=%s del=%s created=%s' % (r[0][:8], r[1], r[2], r[3], r[4], r[5]))

# OLD 库提取配置
cur.execute("""
  SELECT id, scope, cert_type, enabled, deleted_at IS NOT NULL AS del, created_at
  FROM knowledge_extract_configs WHERE knowledge_base_id=%s ORDER BY scope, cert_type
""", (OLD,))
print('\n=== OLD kb extract configs ===')
old_cfgs = cur.fetchall()
for r in old_cfgs:
    print('id=%s scope=%s cert=%s enabled=%s del=%s created=%s' % (r[0][:8], r[1], r[2], r[3], r[4], r[5]))

# 冲突判断
old_keys = {(r[1], r[2]) for r in old_cfgs}
print('\n=== merge plan ===')
for r in new_cfgs:
    if (r[1], r[2]) in old_keys:
        print('CONFLICT skip: scope=%s cert=%s (exists in OLD)' % (r[1], r[2]))
    else:
        print('MIGRATE: scope=%s cert=%s (new to OLD)' % (r[1], r[2]))

# FK 约束：谁引用 knowledge_bases
cur.execute("""
  SELECT tc.table_name, kcu.column_name, rc.delete_rule
  FROM information_schema.table_constraints tc
  JOIN information_schema.key_column_usage kcu ON tc.constraint_name = kcu.constraint_name AND tc.table_schema=kcu.table_schema
  JOIN information_schema.referential_constraints rc ON tc.constraint_name = rc.constraint_name AND tc.table_schema=rc.constraint_schema
  WHERE tc.constraint_type='FOREIGN KEY' AND rc.unique_constraint_schema='public'
    AND EXISTS (SELECT 1 FROM information_schema.key_column_usage k2
                JOIN information_schema.table_constraints tc2 ON tc2.constraint_name=k2.constraint_name
                WHERE tc2.constraint_type IN ('PRIMARY KEY','UNIQUE') AND k2.table_name='knowledge_bases' AND k2.column_name=kcu.column_name)
""")
print('\n=== FK refs to knowledge_bases ===')
for r in cur.fetchall():
    print('  %s.%s delete_rule=%s' % (r[0], r[1], r[2]))

cur.close(); conn.close()
print('\ndone')
