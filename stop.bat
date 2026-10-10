@echo off
chcp 65001 >nul
cd /d "%~dp0"
docker compose stop
echo 已停止。資料都還在，執行 start.bat 就能再啟動。
pause
