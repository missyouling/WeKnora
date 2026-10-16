import json
import os
import re
import sys
import urllib.error
import urllib.request

MCP_JSON = r"C:\Users\koujiang\.workbuddy\mcp.json"
SRC = r"D:\Downloads\WeKnora-main\supabase.txt"
MCP_URL = ("https://mcp.supabase.com/mcp?project_ref=sjdjsulwsqgavwcupaby"
           "&features=docs%2Caccount%2Cdatabase%2Cdebugging%2Cdevelopment%2Cfunctions%2Cbranching")
UA = ("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36")

raw = open(SRC, encoding="utf-8").read()
pat = re.search(r"\b(sbp_[A-Za-z0-9]{20,})\b", raw).group(1)

# 写配置（保留其它 server 条目）
with open(MCP_JSON, encoding="utf-8") as f:
    cfg = json.load(f)
cfg.setdefault("mcpServers", {})["supabase"] = {
    "type": "http",
    "url": MCP_URL,
    "headers": {"Authorization": "Bearer " + pat},
}
tmp = MCP_JSON + ".tmp"
with open(tmp, "w", encoding="utf-8") as f:
    json.dump(cfg, f, indent=2, ensure_ascii=False)
os.replace(tmp, MCP_JSON)
print("MCP_JSON_UPDATED")
entry = cfg["mcpServers"]["supabase"]
print("  servers:", list(cfg["mcpServers"].keys()))
print("  type   :", entry["type"])
print("  auth   :", entry["headers"]["Authorization"][:14] + "...(已脱敏)")


def rpc(payload, sid=None):
    req = urllib.request.Request(
        MCP_URL, data=json.dumps(payload).encode("utf-8"), method="POST",
        headers={"Authorization": "Bearer " + pat,
                 "Content-Type": "application/json",
                 "Accept": "application/json, text/event-stream",
                 "User-Agent": UA,
                 **({"mcp-session-id": sid} if sid else {})})
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            return resp.status, resp.headers.get("mcp-session-id"), resp.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        return e.code, None, e.read().decode("utf-8", "replace")[:400]
    except Exception as e:
        return None, None, "%s: %s" % (type(e).__name__, e)


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


print("\n=== MCP initialize ===")
st, sid, body = rpc({"jsonrpc": "2.0", "id": 1, "method": "initialize",
                     "params": {"protocolVersion": "2025-03-26", "capabilities": {},
                                "clientInfo": {"name": "workbuddy", "version": "1.0"}}})
print("  http_status:", st)
print("  session_id :", (sid[:24] + "...") if sid else None)
msg = parse(body)
if st == 200 and msg and "result" in msg:
    r = msg["result"]
    print("  serverInfo :", r.get("serverInfo"))
    print("  protocol   :", r.get("protocolVersion"))
else:
    print("  raw:", body[:400])
    sys.exit("initialize 失败")

print("\n=== notifications/initialized ===")
st2, _, _ = rpc({"jsonrpc": "2.0", "method": "notifications/initialized"}, sid)
print("  http_status:", st2)

print("\n=== tools/list ===")
st, _, body = rpc({"jsonrpc": "2.0", "id": 2, "method": "tools/list"}, sid)
msg = parse(body)
if st == 200 and msg and "result" in msg:
    tools = msg["result"].get("tools", [])
    print("  http_status:", st, "| 工具总数:", len(tools))
    for t in tools:
        print("    -", t.get("name"))
else:
    print("  raw:", body[:400])
    sys.exit("tools/list 失败")

print("\nRESULT: MCP_OK")
