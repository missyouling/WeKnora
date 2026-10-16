# -*- coding: utf-8 -*-
"""调试 COS v5 签名：对比腾讯云期望的 StringToSign"""
import hashlib, hmac, time, urllib.parse, urllib.request

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

host = custom if custom else f"{bucket}.cos.{region}.myqcloud.com"
path = f"/{prefix}/10000/9d3f1b3a-f622-4e2c-add5-47687afa91db/1791770536475642554.pdf"
method = "get"

start = int(time.time()) - 60
end = start + 900
key_time = f"{start};{end}"
sign_key = hmac.new(secret_key.encode(), key_time.encode(), hashlib.sha1).hexdigest()

# 尝试两种 HttpString 拼法
http_string_a = f"{method}\n{path}\n\nhost={host}\n"
http_string_b = f"{method}\n{path}\n\nhost={urllib.parse.quote(host)}\n"

print("host:", host)
print("sign_key:", sign_key)
print("http_string_a:", repr(http_string_a))
print("sha1(a):", hashlib.sha1(http_string_a.encode()).hexdigest())
print("sha1(b):", hashlib.sha1(http_string_b.encode()).hexdigest())
print("(腾讯云期望 StringToSign 见响应体第二行)")

# 用 A 拼法请求
signature_a = hmac.new(sign_key.encode(), hashlib.sha1(http_string_a.encode()).hexdigest().encode(), hashlib.sha1).hexdigest()
q = urllib.parse.urlencode({
    "q-sign-algorithm": "sha1",
    "q-ak": secret_id,
    "q-sign-time": key_time,
    "q-key-time": key_time,
    "q-header-list": "host",
    "q-url-param-list": "",
    "q-signature": signature_a,
})
url = f"https://{host}{path}?{q}"
req = urllib.request.Request(url, method="GET")
try:
    resp = urllib.request.urlopen(req, timeout=20)
    print("STATUS:", resp.status)
    print("LEN:", resp.headers.get("Content-Length"), "CT:", resp.headers.get("Content-Type"))
except urllib.error.HTTPError as e:
    body = e.read().decode("utf-8", errors="replace")
    print("STATUS:", e.code)
    print("BODY:", body[:800])
