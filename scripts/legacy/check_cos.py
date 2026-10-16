# -*- coding: utf-8 -*-
"""检查 9d3f1b3a 的 PDF 是否存在于 COS 桶（腾讯云 COS v5 签名 HEAD 探测）"""
import hashlib, hmac, time, urllib.request, urllib.parse

env = {}
with open(r"D:\Downloads\WeKnora-main\.env", encoding="utf-8", errors="ignore") as f:
    for line in f:
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        k, v = line.split("=", 1)
        env[k.strip()] = v.strip()

secret_id = env.get("COS_SECRET_ID", "")
secret_key = env.get("COS_SECRET_KEY", "")
region = env.get("COS_REGION", "ap-seoul")
bucket = env.get("COS_BUCKET_NAME", "")
prefix = env.get("COS_PATH_PREFIX", "weknora")
custom = env.get("COS_CUSTOM_DOMAIN", "").strip()

print("region:", region, "| bucket:", bucket, "| prefix:", prefix, "| custom:", custom)

def build_signed_url(method, path):
    host = custom if custom else f"{bucket}.cos.{region}.myqcloud.com"
    uri = "/" + path.lstrip("/")
    start = int(time.time()) - 60
    end = start + 900
    key_time = f"{start};{end}"
    sign_key = hmac.new(secret_key.encode(), key_time.encode(), hashlib.sha1).hexdigest()

    # HttpString: method\nuri\nparams\nheaders\n （headers 按 key 排序，& 连接）
    http_string = f"{method.lower()}\n{uri}\n\nhost={host}\n"
    string_to_sign = hashlib.sha1(http_string.encode()).hexdigest()
    signature = hmac.new(sign_key.encode(), string_to_sign.encode(), hashlib.sha1).hexdigest()

    q = urllib.parse.urlencode({
        "q-sign-algorithm": "sha1",
        "q-ak": secret_id,
        "q-sign-time": key_time,
        "q-key-time": key_time,
        "q-header-list": "host",
        "q-url-param-list": "",
        "q-signature": signature,
    })
    return f"https://{host}{uri}?{q}"

def probe(method, path):
    url = build_signed_url(method, path)
    req = urllib.request.Request(url, method=method)
    try:
        resp = urllib.request.urlopen(req, timeout=20)
        return resp.status, dict(resp.headers), ""
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="replace")[:500]
        return e.code, dict(e.headers), body
    except Exception as e:
        return None, {}, str(e)

print("secret_id_len:", len(secret_id), "| secret_key_len:", len(secret_key))

keys = [
    f"{prefix}/10000/9d3f1b3a-f622-4e2c-add5-47687afa91db/1791770536475642554.pdf",
    f"{prefix}/10000/0c883e41-ad4c-47c9-a094-515f5532811d/1791778971549316600.pdf",
]
for k in keys:
    status, headers, body = probe("GET", k)
    print("-" * 60)
    print("KEY:", k)
    print("STATUS:", status)
    print("BODY:", body[:300])
    if isinstance(headers, dict):
        print("content-length:", headers.get("Content-Length"), "| etag:", headers.get("ETag"), "| ct:", headers.get("Content-Type"))
