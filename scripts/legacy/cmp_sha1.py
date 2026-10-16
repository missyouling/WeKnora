# -*- coding: utf-8 -*-
import hashlib

env = {}
with open(r"D:\Downloads\WeKnora-main\.env", encoding="utf-8", errors="ignore") as f:
    for line in f:
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        k, v = line.split("=", 1)
        env[k.strip()] = v.strip()

secret_key = env.get("COS_SECRET_KEY", "")
host = env.get("COS_CUSTOM_DOMAIN", "").strip()
path = "/weknora/10000/9d3f1b3a-f622-4e2c-add5-47687afa91db/1791770536475642554.pdf"

http_string = "get\n" + path + "\n\nhost=" + host + "\n"
print("http_string repr:", repr(http_string))
print("local sha1:", hashlib.sha1(http_string.encode()).hexdigest())
print("expected  : 55ac554cfebe7dd90e53478136936ad882b9c342")
