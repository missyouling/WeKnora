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

OLD = 'b134e39a-859e-4943-9f89-bbbbb01e2384'  # 日常事务-车队（保留）
NEW = 'b4848deb-d284-4978-88b4-c7725d4b0215'  # 车队管理（删除）

def q(sql, args=None):
    cur.execute(sql, args)
    return cur.fetchall()

# ---- 0) 前置检查：NEW 库现状 ----
print('== pre-check NEW kb ==')
print('knowledges(NEW):', q("SELECT COUNT(*) FROM knowledges WHERE knowledge_base_id=%s", (NEW,))[0][0])
print('chunks(NEW):', q("SELECT COUNT(*) FROM chunks WHERE knowledge_base_id=%s", (NEW,))[0][0])
print('embeddings(NEW):', q("SELECT COUNT(*) FROM embeddings WHERE knowledge_base_id=%s", (NEW,))[0][0])
print('chunk_images(NEW):', q("SELECT COUNT(*) FROM chunk_images WHERE knowledge_base_id=%s", (NEW,))[0][0])
print('extract_cfgs(NEW):', q("SELECT COUNT(*) FROM knowledge_extract_configs WHERE knowledge_base_id=%s AND deleted_at IS NULL", (NEW,))[0][0])

# ---- 1) 迁移文件 ----
kid_new = [r[0] for r in q("SELECT id FROM knowledges WHERE knowledge_base_id=%s AND deleted_at IS NULL", (NEW,))]
print('\n== migrate files ==')
for kid in kid_new:
    cur.execute("UPDATE knowledges SET knowledge_base_id=%s WHERE id=%s", (OLD, kid))
    print('  moved knowledge %s -> OLD' % kid[:8])

# ---- 2) 同步派生数据 ----
cur.execute("UPDATE chunks SET knowledge_base_id=%s WHERE knowledge_base_id=%s", (OLD, NEW))
print('chunks moved:', cur.rowcount)
cur.execute("UPDATE embeddings SET knowledge_base_id=%s WHERE knowledge_base_id=%s", (OLD, NEW))
print('embeddings moved:', cur.rowcount)
cur.execute("UPDATE chunk_images SET knowledge_base_id=%s WHERE knowledge_base_id=%s", (OLD, NEW))
print('chunk_images moved:', cur.rowcount)

# ---- 3) 删除 NEW 库的重复提取配置（OLD 已有同 scope+cert_type） ----
cur.execute("DELETE FROM knowledge_extract_configs WHERE knowledge_base_id=%s", (NEW,))
print('\nextract configs deleted:', cur.rowcount)

# ---- 4) 删除 NEW 库 ----
cur.execute("DELETE FROM knowledge_bases WHERE id=%s", (NEW,))
print('knowledge_base deleted:', cur.rowcount)

# ---- 5) 验证 ----
print('\n== verify ==')
print('remaining kb named 车队管理/日常事务-车队:')
for r in q("SELECT id, name FROM knowledge_bases WHERE name IN ('车队管理','日常事务-车队')"):
    print('  ', r[0][:8], r[1])
print('knowledges still on NEW:', q("SELECT COUNT(*) FROM knowledges WHERE knowledge_base_id=%s", (NEW,))[0][0])
print('chunks still on NEW:', q("SELECT COUNT(*) FROM chunks WHERE knowledge_base_id=%s", (NEW,))[0][0])
print('OLD active files:', q("SELECT COUNT(*) FROM knowledges WHERE knowledge_base_id=%s AND deleted_at IS NULL", (OLD,))[0][0])
print('fleet_records on OLD:', q("SELECT COUNT(*) FROM fleet_records WHERE doc_knowledge_id IN (SELECT id FROM knowledges WHERE knowledge_base_id=%s) AND deleted_at IS NULL", (OLD,))[0][0])

cur.close(); conn.close()
print('\ndone')
