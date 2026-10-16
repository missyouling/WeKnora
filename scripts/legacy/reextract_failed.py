# -*- coding: utf-8 -*-
"""对失败记录 669be4c4 触发重新提取（HTTP 端到端），并等待轮询结果。"""
import os, json, time, urllib.request, urllib.error, hmac, hashlib, base64, datetime

env = {}
with open(".env", "r", encoding="utf-8-sig") as f:
    for line in f:
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            k, v = line.split("=", 1)
            env[k.strip()] = v.strip()

JWT_SECRET = env.get("JWT_SECRET", "")
BASE = "http://localhost:8080"
KB = "3919e8d5-190e-4e1f-adca-77af7f117cfc"
KID = "669be4c4-1563-4a98-a423-f2664f7eca75"
TENANT = env.get("TENANT_ID", "10000")

def b64url(b: bytes) -> str:
    return base64.urlsafe_b64encode(b).rstrip(b"=").decode()

def make_jwt(uid: str, tenant: str) -> str:
    header = b64url(json.dumps({"alg": "HS256", "typ": "JWT"}).encode())
    now = int(time.time())
    payload = b64url(json.dumps({"user_id": uid, "tenant_id": tenant, "exp": now + 3600, "iat": now}).encode())
    signing = f"{header}.{payload}".encode()
    sig = b64url(hmac.new(JWT_SECRET.encode(), signing, hashlib.sha256).digest())
    return f"{header}.{payload}.{sig}"

token = make_jwt("1", TENANT)
print("token_len:", len(token))

def call(method, path, body=None):
    req = urllib.request.Request(f"{BASE}{path}", method=method)
    req.add_header("Authorization", f"Bearer {token}")
    req.add_header("Content-Type", "application/json")
    data = json.dumps(body).encode() if body is not None else None
    try:
        with urllib.request.urlopen(req, data=data, timeout=20) as r:
            return r.status, r.read().decode()[:500]
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()[:500]

# 触发重新提取（参考前端 extract-business 调用）
status, resp = call("POST", f"/api/v1/knowledge-bases/{KB}/knowledge/{KID}/extract-business", {})
print("EXTRACT_STATUS:", status)
print("RESP:", resp)
