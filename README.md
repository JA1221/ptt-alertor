# Ptt-Alertor（精簡版）

<img align="right" src="https://raw.githubusercontent.com/Ptt-Alertor/ptt-alertor/master/logo.jpg">

自己用的 PTT 關鍵字通知 Telegram bot，在本機以 docker compose 執行（app + Redis）。

* 關鍵字通知：看板有新文章標題符合關鍵字時通知，支援 `&`（同時包含）、`!`（排除）、`regexp:`（正規表示式）
* 推文追蹤：追蹤的文章有新推文時傳推文內容，文章刪除時通知並停止追蹤
* 監控：Telegram `狀態`、`測試 看板` 指令、瀏覽器狀態頁、`/health`；看板持續抓取失敗或 PTT 無法連線時主動通知

## 執行

需求：Docker Desktop（含 docker compose）

1. 向 [@BotFather](https://t.me/BotFather) 申請 bot token
2. 建立設定檔並填入 `TELEGRAM_TOKEN`

   ```sh
   cp .env.example .env
   ```

3. 啟動

   ```sh
   docker compose up -d --build
   ```

4. 在 Telegram 對 bot 傳任意訊息，bot 會回覆你的 Telegram ID。
   把它填入 `.env` 的 `TELEGRAM_ALLOWED_IDS`（多人以逗號分隔），再執行一次 `docker compose up -d`

| 環境變數 | 說明 |
|---|---|
| `TELEGRAM_TOKEN` | bot token（必填） |
| `TELEGRAM_ALLOWED_IDS` | 可使用 bot 的 Telegram ID，逗號分隔；其他人傳訊息只會收到自己的 ID |
| `TZ` | 狀態頁時區，預設 `Asia/Taipei` |

* 狀態頁：<http://localhost:9090>（只綁定本機，每 30 秒自動更新）
* 健康檢查：<http://localhost:9090/health>，全部正常回 200，有異常回 503
* 看 log：`docker compose logs -f app`
* 資料存在 docker volume `redis-data`（`docker compose down -v` 會清空）

## 指令

| 指令 | 說明 |
|---|---|
| `指令` | 指令清單 |
| `清單` | 設定的看板與關鍵字 |
| `新增 看板 關鍵字` | 例：`新增 gossiping,movie 金城武,結衣`；任一看板不存在時不會做任何變更 |
| `刪除 看板 關鍵字` | 關鍵字為 `*` 時刪除該看板全部關鍵字（會先確認） |
| `新增推文 網址` / `刪除推文 網址` | 追蹤文章推文，最多 50 篇 |
| `推文清單` / `清理推文` | 列出追蹤文章 / 移除已失效的追蹤 |
| `狀態` | PTT、Telegram、各看板、推文追蹤的狀態 |
| `測試 看板` | 立即抓取看板最新文章，以你的關鍵字比對並回覆結果（不影響通知） |

關鍵字範例：`台積電&法說`（同時包含）、`台積電&!公告`（包含台積電但不含公告）、`regexp:^\[新聞\]`。

## 異常通知

送給所有 `TELEGRAM_ALLOWED_IDS`，每次事件只通知一次，恢復時再通知一次：

* 看板連續抓取失敗 10 分鐘
* PTT 連續 3 次（每分鐘檢查一次）無法連線，期間不另發看板警告

## 開發

```sh
docker run --rm -v "$PWD:/src" -w /src golang:1.23-alpine go test ./...
```

`ptt/rss`、`ptt/web` 部分測試會連線 ptt.cc。

```
main.go               設定、組裝、HTTP server、優雅關閉
config/               環境變數
connections/          Redis 連線池
ptt/{http,rss,web}    PTT 抓取與解析
models/               文章、看板、關鍵字索引、訂閱、使用者（Redis）
command/              指令解析與處理
notify/               Notifier 介面
channels/telegram/    bot：白名單、依序處理更新、送出重試
health/               健康狀態、異常判斷、最近通知
jobs/                 看板檢查、推文追蹤、PTT 監控、異常通知
web/                  狀態頁、/health
```

## Credits

原專案：[Ptt-Alertor/ptt-alertor](https://github.com/Ptt-Alertor/ptt-alertor)

### Real Life

Rose Li, Aries Huang, Scott Kao, Amy Li

### Ptt

DMM, oas, bestpika, Zero0910, lucky0509, wbreeze, chang0206, lindo0130, hungys, gyman7788, tooilxui, myamyakoko, whkuo, papago89, timeline, Kamikiri

### Facebook

Mr.clu, Woqeker
