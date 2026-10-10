@echo off
chcp 65001 >nul
rem 雙擊：備份資料到 backups 資料夾
rem 把備份檔拖曳到這個檔案上：用它還原
cd /d "%~dp0"
title Ptt Alertor 備份

docker info >nul 2>nul
if errorlevel 1 (
  echo Docker 沒有在執行，請先執行 start.bat。
  pause
  exit /b 1
)

if not exist backups mkdir backups
for /f %%t in ('powershell -NoProfile -Command "Get-Date -Format yyyyMMdd-HHmmss"') do set TS=%%t
if not "%~1"=="" goto restore

set FILE=backups\ptt-alertor-%TS%.json
call :backup "%FILE%" || goto failed
echo.
echo 已備份到 %FILE%
echo 檔案裡可能有 bot token，請不要分享給別人。
echo 還原：把備份檔拖曳到 backup.bat 上。
pause
exit /b 0

:restore
echo 要用 %~nx1 還原嗎？目前的資料會被覆蓋（會先自動備份一份）。
choice /c YN /m "確定還原"
if errorlevel 2 exit /b 0
call :backup "backups\before-restore-%TS%.json" || goto failed
docker compose run --rm -T app restore < "%~1" || goto failed
docker compose restart app
echo.
echo 還原完成。原本的資料備份在 backups\before-restore-%TS%.json
pause
exit /b 0

:backup
docker compose run --rm -T app backup > %1
if errorlevel 1 (
  del %1 2>nul
  exit /b 1
)
exit /b 0

:failed
echo.
echo 失敗了，請把上面的訊息截圖給架設者。
pause
exit /b 1
