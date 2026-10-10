@echo off
chcp 65001 >nul
cd /d "%~dp0"
title Ptt Alertor

where docker >nul 2>nul
if not errorlevel 1 goto docker_installed
echo 這台電腦還沒有安裝 Docker Desktop。
where winget >nul 2>nul
if errorlevel 1 goto install_manually
choice /c YN /m "要自動下載安裝嗎（約 600MB，安裝時會跳出系統權限確認）"
if errorlevel 2 goto install_manually
winget install -e --id Docker.DockerDesktop --accept-package-agreements --accept-source-agreements
if errorlevel 1 goto install_manually
echo.
echo 安裝完成！請重新開機，開機後打開 Docker Desktop 按同意，再執行一次 start.bat。
pause
exit /b 0

:install_manually
echo 請到開啟的網頁下載安裝 Docker Desktop，裝好後再執行一次 start.bat。
start "" "https://www.docker.com/products/docker-desktop/"
pause
exit /b 1

:docker_installed

docker info >nul 2>nul
if not errorlevel 1 goto docker_ready
echo 正在啟動 Docker Desktop，第一次可能要等一兩分鐘...
if exist "%ProgramFiles%\Docker\Docker\Docker Desktop.exe" start "" "%ProgramFiles%\Docker\Docker\Docker Desktop.exe"
for /l %%i in (1,1,60) do (
  timeout /t 3 /nobreak >nul
  docker info >nul 2>nul && goto docker_ready
)
echo Docker Desktop 沒有啟動，請手動打開 Docker Desktop 後再執行一次。
pause
exit /b 1

:docker_ready
echo 下載最新版本...
docker compose pull --ignore-pull-failures --quiet
echo 啟動中（沒有現成映像檔時會自動編譯，約需幾分鐘）...
docker compose up -d --remove-orphans
if errorlevel 1 (
  echo.
  echo 啟動失敗，請把上面的訊息截圖給架設者。
  pause
  exit /b 1
)

echo 等待服務就緒...
rem PORT in .env changes the port
set URL=http://localhost:9090
for /f "tokens=2 delims=:" %%p in ('docker compose port app 9090 2^>nul') do set URL=http://localhost:%%p
for /l %%i in (1,1,30) do (
  curl -fs %URL%/healthz >nul 2>nul && goto ready
  timeout /t 2 /nobreak >nul
)
echo.
echo 服務還沒準備好，可能還在啟動中。請等一分鐘後用瀏覽器打開 %URL%
echo 一直打不開的話，執行 docker compose logs app 查看原因。
call :autostart
pause
exit /b 1

:ready
start "" %URL%
echo.
echo 完成！瀏覽器會打開 %URL% （第一次會是設定頁）
echo 這個視窗可以關掉，bot 會在背景持續執行。
echo 更新到最新版：再執行一次這個檔案。
call :autostart
pause
exit /b 0

rem asks once to start Docker Desktop at sign in, the bot then starts by itself
:autostart
set RUNKEY=HKCU\Software\Microsoft\Windows\CurrentVersion\Run
set DOCKER_EXE=%ProgramFiles%\Docker\Docker\Docker Desktop.exe
reg query "%RUNKEY%" /v "Ptt Alertor" >nul 2>nul && exit /b 0
findstr /r /c:"\"AutoStart\": *true" "%APPDATA%\Docker\settings-store.json" >nul 2>nul && exit /b 0
if exist ".no-autostart" exit /b 0
if not exist "%DOCKER_EXE%" exit /b 0
echo.
choice /c YN /m "要讓電腦開機後自動執行 bot 嗎"
if errorlevel 2 (
  type nul > .no-autostart
  echo 不設定。之後想要的話，刪掉 .no-autostart 檔案再執行一次 start.bat。
  exit /b 0
)
reg add "%RUNKEY%" /v "Ptt Alertor" /t REG_SZ /d "\"%DOCKER_EXE%\"" /f >nul
echo 已設定，開機登入後會自動執行。取消：工作管理員 ^> 啟動應用程式 ^> 停用 Ptt Alertor。
exit /b 0
