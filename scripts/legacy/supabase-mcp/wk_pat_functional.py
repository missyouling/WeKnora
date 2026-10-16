import json
import os
import re
import urllib.error
import urllib.request

MCP_URL = ("https://mcp.supabase.com/mcp?project_ref=sjdjsulwsqgavwcupaby"
           "&features=docs%2Caccount%2Cdatabase%2Cdebugging%2Cdevelopment%2Cfunctions%2Cbranching")
UA = ("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36")
pat = re.search(r"\b(sbp_[A-Za-z0-9]{20,})\b",
                open(r"D:\Downloads\WeKnora-main\supabase.txt", encoding="utf-8").read()).group(1)

H = {"Authorization": "Bearer " + pat, "Content-Type": "application/json",
     "Accept": "application/json, text/event-stream", "User-Agent": UA}
sid = [None]


def rpc(payload):
    h = dict(H)
    if sid[0]:
        h["mcp-session-id"] = sid[0]
    req = urllib.request.Request(MCP_URL, data=json.dumps(payload).encode(),
                                 method="POST", headers=h)
    try:
        with urllib.request.urlopen(req, timeout=90) as resp:
            if resp.headers.get("mcp-session-id"):
                sid[0] = resp.headers.get("mcp-session-id")
            return resp.status, resp.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", "replace")[:400]
    except Exception as e:
        return None, "%s: %s" % (type(e).__name__, e)


def parse(raw):
    for line in raw.splitlines():
        if line.startswith("data:"):
            try:
                return json.loads(line[5:].strip())
            except Exception:
                pass
    try:
        return json.loads(raw)
    except Exception:
        return None


_id = [0]


def call_tool(name, args):
    _id[0] += 1
    st, body = rpc({"jsonrpc": "2.0", "id": _id[0], "method": "tools/call",
                    "params": {"name": name, "arguments": args}})
    msg = parse(body)
    if st != 200 or not msg:
        return "HTTP %s / %s" % (st, body[:300])
    if "error" in msg:
        return "RPC error: %s" % json.dumps(msg["error"])[:300]
    out = []
    for c in msg.get("result", {}).get("content", []):
        if c.get("type") == "text":
            out.append(c["text"])
    return "\n".join(out)


rpc({"jsonrpc": "2.0", "id": 1, "method": "initialize",
     "params": {"protocolVersion": "2025-03-26", "capabilities": {},
                "clientInfo": {"name": "workbuddy", "version": "1.0"}}})
rpc({"jsonrpc": "2.0", "method": "notifications/initialized"})

print("=== execute_sql: 迁移后新库元数据 ===")
print(call_tool("execute_sql", {"query":
    "SELECT current_database() AS db, current_user AS usr, "
    "inet_server_addr()::text AS server_addr, "
    "(SELECT count(*) FROM information_schema.tables "
    " WHERE table_schema='public') AS public_tables, "
    "(SELECT count(*) FROM tenants) AS tenants, "
    "(SELECT count(*) FROM knowledges) AS knowledges, "
    "(SELECT count(*) FROM chunks) AS chunks, "
    "(SELECT count(*) FROM embeddings) AS embeddings"}))

print("\n=== execute_sql: 已装扩展 ===")
print(call_tool("execute_sql", {"query":
    "SELECT extname, extversion FROM pg_extension ORDER BY 1"}))

print("\n=== get_project_url ===")
print(call_tool("get_project_url", {}))
