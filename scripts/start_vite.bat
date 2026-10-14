@echo off
set "NODE_DIR=C:\Users\koujiang\AppData\Local\Doubao\User Data\sandbox_runtime\bases\d0ace100fdff5b2d247847aca1e3d83b\node"
set "PATH=%NODE_DIR%;%PATH%"
set NODE_OPTIONS=--max-old-space-size=6144
set VITE_DEV_PROXY_TARGET=http://localhost:8080
cd /d D:\Downloads\WeKnora-main\frontend
call npm run dev