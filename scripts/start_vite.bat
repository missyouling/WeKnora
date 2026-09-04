@echo off
set "NODE_DIR=C:\Users\koujiang\AppData\Local\Doubao\User Data\sandbox_runtime\bases\c98c5042338ed152c6f10ecd8591889f\node"
set "PATH=%NODE_DIR%;%PATH%"
set NODE_OPTIONS=--max-old-space-size=6144
set VITE_DEV_PROXY_TARGET=http://localhost:8088
cd /d D:\Downloads\WeKnora-main\frontend
call npm run dev
