#!/bin/sh
# 啟動或更新 Ptt Alertor（macOS / Linux）
set -e
cd "$(dirname "$0")"

if ! command -v docker >/dev/null 2>&1; then
  echo "找不到 Docker，請先安裝：https://docs.docker.com/get-docker/"
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "正在啟動 Docker..."
  if [ "$(uname)" = "Darwin" ]; then open -a Docker; fi
  i=0
  until docker info >/dev/null 2>&1; do
    i=$((i + 1))
    if [ $i -gt 60 ]; then
      echo "Docker 沒有啟動，請手動啟動後再執行一次。"
      exit 1
    fi
    sleep 3
  done
fi

echo "下載最新版本..."
docker compose pull --ignore-pull-failures --quiet || true
echo "啟動中（沒有現成映像檔時會自動編譯，約需幾分鐘）..."
docker compose up -d --remove-orphans

echo "等待服務就緒..."
i=0
until curl -fs http://localhost:9090/healthz >/dev/null 2>&1 || [ $i -ge 30 ]; do
  i=$((i + 1))
  sleep 2
done

url=http://localhost:9090
if command -v open >/dev/null 2>&1; then open "$url"; elif command -v xdg-open >/dev/null 2>&1; then xdg-open "$url" >/dev/null 2>&1 || true; fi
echo
echo "完成！打開 $url 設定或查看狀態（第一次會是設定頁）。"
echo "更新到最新版：再執行一次 ./start.sh"
