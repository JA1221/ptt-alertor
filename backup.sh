#!/bin/sh
# 備份 Ptt Alertor 資料（macOS / Linux）
#   ./backup.sh          備份到 backups 資料夾
#   ./backup.sh 備份檔   用備份檔還原
set -e

restore_file=""
if [ -n "$1" ]; then
  restore_file=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
fi
cd "$(dirname "$0")"
mkdir -p backups
ts=$(date +%Y%m%d-%H%M%S)

backup() {
  if ! docker compose run --rm -T app backup > "$1"; then
    rm -f "$1"
    echo "備份失敗"
    exit 1
  fi
}

if [ -z "$restore_file" ]; then
  file=backups/ptt-alertor-$ts.json
  backup "$file"
  echo "已備份到 $file"
  echo "檔案裡可能有 bot token，請不要分享給別人。"
  echo "還原：./backup.sh $file"
  exit 0
fi

printf "要用 %s 還原嗎？目前的資料會被覆蓋（會先自動備份一份）。[y/N] " "$(basename "$restore_file")"
read -r answer
case "$answer" in y | Y) ;; *) exit 0 ;; esac
backup "backups/before-restore-$ts.json"
docker compose run --rm -T app restore < "$restore_file"
docker compose restart app
echo "還原完成。原本的資料備份在 backups/before-restore-$ts.json"
