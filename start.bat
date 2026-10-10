@echo off
chcp 65001 >nul
cd /d "%~dp0"
title Ptt Alertor

where docker >nul 2>nul
if errorlevel 1 (
  echo 找不到 Docker，請先安裝 Docker Desktop，裝好後再執行一次這個檔案。
  start "" "https://www.docker.com/products/docker-desktop/"
  pause
  exit /b 1
)

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
for /l %%i in (1,1,30) do (
  curl -fs http://localhost:9090/healthz >nul 2>nul && goto ready
  timeout /t 2 /nobreak >nul
)
:ready
start "" http://localhost:9090
echo.
echo 完成！瀏覽器會打開 http://localhost:9090 （第一次會是設定頁）
echo 這個視窗可以關掉，bot 會在背景持續執行。
echo 想要開機自動執行：Docker Desktop 設定 ^> General ^> 勾選 Start Docker Desktop when you sign in
echo 更新到最新版：再執行一次這個檔案。
pause
