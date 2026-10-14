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

OLD = 'b134e39a-859e-4943-9f89-bbbbb01e2384'  # 日常事务-车队 (旧)
NEW = 'b4848deb-d284-4978-88b4-c7725d4b0215'  # 车队管理 (新)

# 1) 两库文件数（活跃/全部/各类型）
cur.execute("""
  SELECT knowledge_base_id,
         COUNT(*) AS total,
         COUNT(*) FILTER (WHERE deleted_at IS NULL) AS active,
         COUNT(*) FILTER (WHERE deleted_at IS NULL AND parse_status='success') AS parsed
  FROM knowledges
  WHERE knowledge_base_id IN (%s,%s)
  GROUP BY knowledge_base_id
""", (OLD, NEW))
print('=== knowledges per kb ===')
for r in cur.fetchall():
    print('kb=%s total=%s active=%s parsed=%s' % (r[0][:8], r[1], r[2], r[3]))

# 2) 两库文件明细（标题/文件名/状态）
cur.execute("""
  SELECT knowledge_base_id, title, file_name, parse_status, COALESCE(metadata->>'cert_type',''), COALESCE(metadata->>'scope',''), deleted_at IS NOT NULL AS del
  FROM knowledges
  WHERE knowledge_base_id IN (%s,%s)
  ORDER BY knowledge_base_id, created_at
""", (OLD, NEW))
print('\n=== files detail ===')
for r in cur.fetchall():
    print('kb=%s | %s | %s | %s | %s %s | del=%s' % (r[0][:8], (r[1] or '')[:30], (r[2] or '')[:30], r[3], r[4][:12], r[5][:16], r[6]))

# 3) fleet_records 结构 + 归属（knowledge_id -> kb）
cur.execute("""SELECT column_name FROM information_schema.columns WHERE table_name='fleet_records' ORDER BY ordinal_position""")
cols = [c[0] for c in cur.fetchall()]
print('\n=== fleet_records columns ===')
print(cols)

kid_cols = [c for c in cols if 'knowledge' in c.lower() or 'kb' in c.lower() or c in ('id','document_id','file_id')]
for c in kid_cols:
    if c in ('id','document_id','file_id'):
        continue
    try:
        cur.execute("""
          SELECT kb.name, COUNT(*)
          FROM fleet_records t JOIN knowledges k ON k.id = t.%s
          JOIN knowledge_bases kb ON kb.id = k.knowledge_base_id
          WHERE kb.name IN ('车队管理','日常事务-车队')
          GROUP BY kb.name
        """ % (c,))
        res = cur.fetchall()
        if res:
            print('fleet_records.%s ->' % c)
            for r in res:
                print('   %s: %s' % (r[0], r[1]))
    except Exception as e:
        print('fleet_records.%s err: %s' % (c, str(e)[:100]))

# 4) 其它业务表引用这两个 kb 的情况（invoice/contract/regul/utility 等可能用 knowledge 或直接存 kb_id）
tables = ['billing_records','billing_tenant_items','utility_meter_records','utility_meter_items','fleet_vehicles','fleet_drivers','fleet_etc_cards','fleet_fuel_cards','fleet_suppliers']
for t in tables:
    try:
        cur.execute("""SELECT column_name FROM information_schema.columns WHERE table_name=%s""", (t,))
        tcols = [c[0] for c in cur.fetchall()]
        tkids = [c for c in tcols if 'knowledge' in c.lower() or c == 'kb_id']
        if not tkids:
            continue
        for c in tkids:
            try:
                cur.execute("""
                  SELECT kb.name, COUNT(*)
                  FROM %s t JOIN knowledges k ON k.id = t.%s
                  JOIN knowledge_bases kb ON kb.id = k.knowledge_base_id
                  WHERE kb.name IN ('车队管理','日常事务-车队')
                  GROUP BY kb.name
                """ % (t, c))
                res = cur.fetchall()
                if res:
                    print('%s.%s ->' % (t, c))
                    for r in res:
                        print('   %s: %s' % (r[0], r[1]))
            except Exception as e:
                pass
    except Exception:
        pass

# 5) 直接引用 kb id 的表（可能存 kb_id 列）
for t in ['fleet_records','billing_records','knowledge_extract_configs','fleet_categories','fleet_cert_groups']:
    try:
        cur.execute("""SELECT column_name FROM information_schema.columns WHERE table_name=%s""", (t,))
        tcols = [c[0] for c in cur.fetchall()]
        if 'kb_id' in tcols:
            cur.execute("SELECT kb.name, COUNT(*) FROM %s t JOIN knowledge_bases kb ON kb.id=t.kb_id WHERE kb.name IN ('车队管理','日常事务-车队') GROUP BY kb.name" % t)
            res = cur.fetchall()
            if res:
                print('%s.kb_id ->' % t)
                for r in res:
                    print('   %s: %s' % (r[0], r[1]))
    except Exception as e:
        pass

cur.close(); conn.close()
print('\ndone')
