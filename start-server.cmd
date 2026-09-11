@echo off
set SERVER_PORT=8080
set "DB_PASSWORD=7aPBAINUbuMmTD"
set "SYSTEM_AES_KEY=0123456789abcdef0123456789abcdef"
set "DB_HOST=db.kunsjmpkvfjjzcotdgrg.supabase.co"
set "STORAGE_TYPE=local"
set "JWT_SECRET=weknora-local-dev-jwt-secret-2026"
set "DB_NAME=postgres"
set "DB_DRIVER=postgres"
set "LOCAL_STORAGE_BASE_DIR=D:\Downloads\WeKnora-main\data\files"
set "RETRIEVE_DRIVER=postgres"
set "DOCREADER_ADDR=127.0.0.1:50051"
set "DOCREADER_TRANSPORT=grpc"
set "AUTO_MIGRATE=true"
set "DB_PORT=5432"
set "DB_USER=postgres"
set "DB_SSLMODE=require"
set "GIN_MODE=release"
cd /d D:\Downloads\WeKnora-main
start /b weknora-server.exe >> server-run.log 2>&1
