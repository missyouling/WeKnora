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

# knowledges 表结构（找 kb 外键列）
cur.execute("""SELECT column_name FROM information_schema.columns WHERE table_name='knowledges' ORDER BY ordinal_position""")
print('=== knowledges columns ===')
print([c[0] for c in cur.fetchall()])

# knowledge_bases 列
cur.execute("""SELECT column_name FROM information_schema.columns WHERE table_name='knowledge_bases' ORDER BY ordinal_position""")
print('\n=== knowledge_bases columns ===')
print([c[0] for c in cur.fetchall()])

# 业务表清单
cur.execute("""
  SELECT table_name FROM information_schema.tables
  WHERE table_schema='public' AND table_name NOT IN ('schema_migrations','_prisma_migrations','knowledges','knowledge_bases','knowledge_chunks','knowledge_tags','resources','storage_backends','knowledge_tags_bindings','knowledge_bindings','embedding_records','wikipedia_entities','wiki_edges','topic_nodes','topic_graph_edges','tenant_settings','users','invitations','audit_logs','system_settings','session_records','conversation_records','model_configs','document_records','system_agents','builtin_agents','agent_type_presets','agents','chats','prompt_templates','knowledge_relationships','graph_embeddings','vector_records','keyword_records','dataset_records')
  ORDER BY table_name
""")
print('\n=== other tables ===')
print([r[0] for r in cur.fetchall()])

cur.close(); conn.close()
