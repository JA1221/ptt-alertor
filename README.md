# Ptt Alertor

<img align="right" width="120" src="logo.jpg">

PTT 關鍵字通知 Telegram bot。自己架、分享給朋友用，不用寫程式。

- 🔔 看板有新文章標題符合關鍵字就通知，支援「同時包含」「排除」「正規表示式」
- 💬 追蹤文章推文，有新推文就傳內容
- 👥 朋友傳訊息給 bot → 你按「允許」就能用
- 🖱️ Telegram 按鈕選單操作，網頁也能管理成員與關鍵字
- 📊 狀態頁、異常主動通知，壞了會知道

## 三步驟架設

**1. 安裝 Docker Desktop**

到 [docker.com](https://www.docker.com/products/docker-desktop/) 下載安裝（Windows / macOS）。Linux 安裝 Docker Engine 即可。

**2. 下載並啟動**

到 [Releases](https://github.com/JA1221/ptt-alertor/releases/latest) 下載最新版的 **Source code (zip)**，解壓縮後：

- Windows：雙擊 `start.bat`
- macOS / Linux：在資料夾開終端機執行 `./start.sh`

**3. 照網頁指示設定**

瀏覽器會自動打開 <http://localhost:9090>：

1. 在 Telegram 找 [@BotFather](https://t.me/BotFather)，傳 `/newbot` 建立 bot，把拿到的 token 貼到網頁
2. 點「在 Telegram 開啟」→ 按「開始」，你就是管理員了

完成！在 Telegram 按下方「➕ 新增」，選看板、輸入關鍵字試試看。

> 第一次執行 `start.bat` 時會問要不要開機自動執行，選 Y 就好。之後想取消：工作管理員 → 啟動應用程式 → 停用「Ptt Alertor」。macOS：Docker Desktop → Settings → General → 勾選「Start Docker Desktop when you sign in」。

## 分享給朋友

把 bot 連結 `https://t.me/你的bot帳號` 傳給朋友（狀態頁上也有）。朋友傳訊息給 bot 時，你會在 Telegram 收到「✅ 允許／❌ 拒絕」按鈕，狀態頁上也能按。

狀態頁 <http://localhost:9090> 可以直接：允許／拒絕申請、移除成員、切換註冊模式、幫任何成員新增或刪除關鍵字、下載備份與還原。

Telegram 裡也可以用指令：

| 管理員指令 | 說明 |
|---|---|
| `使用者` | 成員與待審核清單，可按鈕移除或核准 |
| `註冊 審核` | 陌生人要經過你同意（預設） |
| `註冊 開放` | 任何人都能直接使用 |
| `註冊 關閉` | 不接受新使用者 |

每人上限：20 個看板、每個看板 20 個關鍵字、50 篇推文追蹤。

## 使用方式

不用記指令，按下方鍵盤就好：

- **➕ 新增**：選看板（自己的看板和熱門看板）→ 輸入關鍵字，完成
- **📋 清單**：點看板 → 點關鍵字刪除，或在該看板再新增
- **💬 推文清單**：追蹤中的文章，點按鈕停止追蹤
- 通知下方的按鈕：「⚙️ 管理關鍵字」調整太吵的關鍵字、推文通知的「🛑 停止追蹤」
- 直接貼上 PTT 或 pttweb 文章網址 → 一鍵追蹤推文；輸入 `stock 台積電` → 一鍵新增

習慣打字也可以用指令：

| 指令 | 說明 |
|---|---|
| `新增 看板 關鍵字` | 例：`新增 gossiping,movie 金城武,結衣`（逗號分隔多個） |
| `刪除 看板 關鍵字` | 關鍵字填 `*` 刪除該看板全部 |
| 貼上文章網址 | 一鍵追蹤推文 |
| `推文清單`、`清理推文` | 管理追蹤中的文章 |
| `狀態` | PTT、看板、推文追蹤是否正常 |
| `測試 看板` | 立刻抓最新文章並用你的關鍵字比對（不影響通知） |

關鍵字進階：`台積電&法說`（同時包含）、`台積電&!公告`（排除公告）、`regexp:^\[新聞\]`（正規表示式）。

## 常見問題

**更新到最新版？** 再執行一次 `start.bat` / `start.sh`，會自動下載最新的 1.x 版（不會跨到可能不相容的 2.x）。目前版本顯示在狀態頁最下方，改了什麼看 [Releases](https://github.com/JA1221/ptt-alertor/releases)。

**備份與還原？** 狀態頁最下方可以下載備份、上傳還原。也可以雙擊 `backup.bat`（macOS / Linux：`./backup.sh`），資料會存到 `backups` 資料夾。還原時把備份檔拖曳到 `backup.bat` 上（`./backup.sh 備份檔`），還原前會先自動備份目前的資料。備份檔裡可能有 bot token，不要分享給別人。

**停止？** 執行 `stop.bat`，或 `docker compose stop`。資料會保留。

**資料存在哪？** Docker volume `ptt-alertor_redis-data`。`docker compose down -v` 會全部清除，換電腦或重灌前記得先備份。

**換 bot token？** 在 `.env` 設定 `TELEGRAM_TOKEN=新token` 後重新執行 start；或在 BotFather 撤銷舊 token，程式會自動回到設定頁。

**9090 port 被占用？** 建立 `.env` 寫入 `PORT=9091`，網址改成 <http://localhost:9091>。

**出問題了？**
- 狀態頁 <http://localhost:9090> 會列出問題；看板連續失敗 10 分鐘或 PTT 連不上時，bot 會通知管理員
- 看紀錄：`docker compose logs -f app`

## 進階設定

複製 `.env.example` 為 `.env`，全部都可以不填：

| 變數 | 說明 |
|---|---|
| `TELEGRAM_TOKEN` | bot token，填了就不用在網頁設定 |
| `TELEGRAM_ADMIN_IDS` | 管理員 Telegram ID，逗號分隔，填了就不用在網頁認領 |
| `PORT` | 網頁 port，預設 9090 |
| `TZ` | 時區，預設 `Asia/Taipei` |

狀態頁只開放本機連線。架在遠端伺服器時，用 SSH tunnel 開啟：`ssh -L 9090:localhost:9090 你的伺服器`。

## 開發

```sh
docker compose up -d --build          # 用本機程式碼建置並啟動
docker run --rm -v "$PWD:/src" -w /src golang:1.23 go test -race -short ./...
```

推送到 `master` 時，GitHub Actions 會跑測試並建置 amd64/arm64 映像檔到 `ghcr.io/<owner>/<repo>:latest`。推送 `v1.2.3` 這類 tag 會發布 `1.2.3`、`1.2`、`1` 版本，`docker-compose.yml` 使用 `1`。

```
main.go               設定、組裝、HTTP server、優雅關閉、healthcheck/backup/restore 子指令
config/               環境變數
backup/               全部資料匯出成 JSON 與還原
connections/          Redis 連線池
ptt/{http,rss,web}    PTT 抓取與解析
models/               文章、看板、關鍵字索引、訂閱、使用者、權限（Redis）
command/              指令、按鈕選單、對話狀態
notify/               訊息格式與 Notifier 介面
channels/telegram/    bot：依使用者分流、送出重試、封鎖偵測
health/               健康狀態、異常判斷、最近通知
jobs/                 看板檢查、推文追蹤、PTT/Redis 監控、異常通知
web/                  設定精靈、狀態頁（管理成員、關鍵字、備份）、/health
```

## Credits

改自 [Ptt-Alertor/ptt-alertor](https://github.com/Ptt-Alertor/ptt-alertor)（Apache-2.0）。

原專案感謝名單 — Real Life：Rose Li, Aries Huang, Scott Kao, Amy Li；Ptt：DMM, oas, bestpika, Zero0910, lucky0509, wbreeze, chang0206, lindo0130, hungys, gyman7788, tooilxui, myamyakoko, whkuo, papago89, timeline, Kamikiri；Facebook：Mr.clu, Woqeker
