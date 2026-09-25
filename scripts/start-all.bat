@echo off
rem One-click launcher for WeKnora dev services.
rem Usage: start-all.bat            -> start missing services
rem        start-all.bat stop       -> stop services
rem        start-all.bat status     -> show status
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0start-all.ps1" %*
