@echo off
cd /d D:\Downloads\WeKnora-main
for /f "usebackq eol=# tokens=1,* delims==" %%A in (".env") do (
    if not "%%A"=="" set "%%A=%%B"
)
D:\Downloads\WeKnora-main\server.exe >> D:\Downloads\WeKnora-main\data\server2.out.log 2>&1
