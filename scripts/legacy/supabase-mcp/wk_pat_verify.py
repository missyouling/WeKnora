import json
import re
import sys
import urllib.error
import urllib.request

UA = ("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36")
SRC = r"D:\Downloads\WeKnora-main\supabase.txt"

raw = open(SRC, encoding="utf-8").read()

# 提取 sbp_ 前缀的 Personal Access Token
m = re.search(r"\b(sbp_[A-Za-z0-9]{20,})\b", raw)
if not m:
    sys.exit("未在 supabase.txt 中找到 sbp_ 前缀的 PAT")
pat = m.group(1)
print("PAT 提取成功，长度=%d，前缀=%s..." % (len(pat), pat[:8]))

# 安全提醒：该文件还含数据库连接串（明文密码）
if "postgresql://" in raw:
    print("[注意] supabase.txt 同时包含数据库连接串（明文密码），属敏感文件；已被 .gitignore 忽略")


def api(path):
    req = urllib.request.Request("https://api.supabase.com" + path,
                                 headers={"Authorization": "Bearer " + pat,
                                          "Accept": "application/json",
                                          "User-Agent": UA})
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            return r.status, json.loads(r.read().decode())
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()[:300]
    except Exception as e:
        return None, "%s: %s" % (type(e).__name__, e)


print("\n=== 1. 验证 PAT：GET /v1/projects ===")
st, body = api("/v1/projects")
print("  http_status:", st)
if st == 200:
    for p in body:
        print("    ref=%-26s name=%-12s region=%-16s status=%s"
              % (p.get("ref"), p.get("name"), p.get("region"), p.get("status")))
else:
    print("  ", body)
    sys.exit("PAT 验证失败")

print("\n=== 2. 组织信息（验证 organizations:read scope） ===")
st, body = api("/v1/organizations")
print("  http_status:", st)
if st == 200:
    for o in body:
        print("    id=%-24s name=%s" % (o.get("id"), o.get("name")))
else:
    print("  ", body)

print("\n=== 3. 目标项目详情（weknora-da） ===")
st, body = api("/v1/projects/sjdjsulwsqgavwcupaby")
print("  http_status:", st)
if st == 200:
    for k in ("id", "name", "region", "status", "created_at"):
        print("    %-12s %s" % (k, body.get(k)))
else:
    print("  ", body)

print("\nRESULT: PAT_VALID" if st == 200 else "\nRESULT: PAT_INVALID")
