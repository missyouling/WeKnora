# AGENTS.md

## 注意事项

- 每次改动完成后，都必须创建一个对应的 Git commit，以便后续追踪和回滚。
- 每次改动后，都必须编写或更新相关测试，并在交付给用户前，确保所有测试和验证全部通过。

## 后端构建（Windows）

- **必须使用 UCRT 版 WinLibs 工具链**，MSVCRT 版无法链接 duckdb 静态库（duckdb 是 MSVC 编译的，MSVCRT 工具链会报 `__stdio_common_vsnprintf_s` 等未定义符号）。
  - 工具链目录：`C:\Users\koujiang\AppData\Local\Microsoft\WinGet\Packages\BrechtSanders.WinLibs.POSIX.UCRT_Microsoft.Winget.Source_8wekyb3d8bbwe\mingw64\bin`
- 编译命令（在项目根目录执行）：
  ```powershell
  $ucrt = "C:\Users\koujiang\AppData\Local\Microsoft\WinGet\Packages\BrechtSanders.WinLibs.POSIX.UCRT_Microsoft.Winget.Source_8wekyb3d8bbwe\mingw64\bin"
  $env:PATH = "$ucrt;$env:PATH"
  $env:CC = "$ucrt\gcc.exe"
  $env:CXX = "$ucrt\g++.exe"
  $env:CGO_ENABLED = "1"
  $env:CGO_CFLAGS = "-O2 -g -ID:\Downloads\WeKnora-main\third_party\sqlite3"  # sqlite-vec 编译需要 sqlite3.h
  go build -o server.exe ./cmd/server
  ```
- 构建产物必须输出到项目根目录 `server.exe`（计划任务 `WeKnoraServer` 执行 `scripts\start_server.bat` 运行的就是它）。

## 服务启动（Windows 计划任务）

- 已有计划任务：`WeKnoraServer`（后端）、`WeKnoraDocreader`（解析服务）、`WeKnoraVite`（前端）。
- 启动后端：`Start-ScheduledTask -TaskName WeKnoraServer`
- 端口约定：后端 8088、前端 5173、docreader gRPC 50051。
- docreader 可手动启动（当前 python 解释器版本不符时用项目虚拟环境）：
  ```powershell
  $env:PYTHONPATH = "D:\Downloads\WeKnora-main"
  D:\Downloads\WeKnora-main\docreader\.venv\Scripts\python.exe -m docreader.main
  ```
