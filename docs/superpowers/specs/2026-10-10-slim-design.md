# Ptt-Alertor 精簡版設計

日期：2026-10-10　分支：`slim`

## 目標

把 Ptt-Alertor 改成**只給自己用、在本機以 docker compose 執行**的 PTT 關鍵字通知 bot。
移除不能用或非核心的功能，修正 bug，並加上「能確認功能正常」的監控手段。

成功標準：

1. `docker compose up -d --build` 只啟動 app + Redis 兩個容器即可完整運作
2. Telegram 上能新增/刪除關鍵字與推文追蹤，新文章與新推文會通知
3. 能從 Telegram（`狀態`、`測試`）與瀏覽器狀態頁確認各元件正常；異常時主動通知
4. 所有測試通過，現有使用者訂閱資料不需重設

## 範圍

### 保留

| 功能 | 說明 |
|---|---|
| 關鍵字通知 | 支援 `&`（同時含）、`!`（排除）、`regexp:`；多看板、多關鍵字 |
| 推文追蹤 | `新增推文 網址`，有新推文就傳推文內容 |
| Telegram bot | 只用 long polling；白名單 `TELEGRAM_ALLOWED_IDS` |
| 指令 | `指令`、`清單`、`新增/刪除 看板 關鍵字`、`新增推文/刪除推文 網址`、`推文清單`、`清理推文`、`狀態`、`測試 看板` |

### 移除

- 通知管道：LINE、Messenger、Email（Mailgun）、短網址、廣播
- 功能：作者通知、推文數通知、排行榜、`add/del -k` 英文指令、使用者管理 API、Telegram webhook、`BOARD_HIGH`
- 儲存：DynamoDB（含 DynamoDB Local 容器），全部改用 Redis
- 部署：`.aws/`、`ecs-deploy`、ECS 部署 workflow
- 網站：原宣傳網站（含原作者 Google Analytics、Facebook SDK、LINE QR code、S3 圖片）
- 沒用到的程式：`session`、檔案儲存 driver、一次性搬資料 job、`testing/*.ahk`、未使用的 myutil 函式

### 技術升級

- Go 1.15 → 1.23；`garyburd/redigo` → `gomodule/redigo`；`miniredis` → `miniredis/v2`
- 移除 aws-sdk、line-bot-sdk、mailgun、gops、cron 等不再使用的套件
- 網頁樣板以 `go:embed` 打包進執行檔

## 架構

```
main.go               設定、組裝、HTTP server、優雅關閉
config/               讀取環境變數
connections/          Redis 連線池
ptt/{http,rss,web}    PTT 抓取與解析（保留原實作）
models/
  article/            文章、推文；追蹤文章的 Redis 儲存
  board/              看板快照、巡邏清單、RSS→HTML 備援抓取、板名建議
  keyword/            看板 → 訂閱者索引
  subscription/       使用者訂閱（看板、關鍵字、追蹤文章）
  user/               使用者（帳號 = Telegram ID）
command/              指令解析與處理
notify/               Notifier 介面（送訊息）
channels/telegram/    bot：白名單、依序處理更新、送訊息重試
health/               健康狀態紀錄、異常判斷、最近通知
jobs/                 看板檢查、推文追蹤檢查、PTT 監控、異常通知
web/                  狀態頁、/health
```

資料流：`jobs` 透過 `models` 抓取與比對 → 命中時呼叫 `notify.Notifier` →
`channels/telegram` 送出；每次抓取、送出結果都回報給 `health`；
`command`、`web` 從 `health` 與 `models` 讀取狀態。

## 監控

`health` 在記憶體中記錄：

- 每個看板：最後成功時間、最後錯誤、連續失敗起始時間
- 推文追蹤：追蹤篇數、最後檢查時間
- PTT 連線狀態、Telegram 狀態（最後送出成功／錯誤）、啟動時間

最近 20 則通知存在 Redis（`notify:recent`），重啟後保留。

異常通知規則（每次事件只通知一次，恢復時再通知一次，送給所有白名單使用者）：

- 看板連續失敗 ≥ 10 分鐘 → `⚠️ <看板> 已連續 10 分鐘抓取失敗：<原因>`
- PTT 監控連續 3 次（每分鐘一次）失敗 → `⚠️ PTT 無法連線`；期間不發看板個別警告
- 恢復 → `✅ ... 已恢復`

`測試 看板`：即時抓取該看板最新文章（不影響比對基準），以使用者在該看板的關鍵字比對，回覆命中結果。

狀態頁 `http://localhost:9090`：元件狀態、看板、推文追蹤、訂閱清單、最近通知，每 30 秒自動更新。
`/health`：全部正常回 200，有異常回 503（JSON）。只綁定 127.0.0.1。

## Bug 修正

1. 巡邏清單由訂閱決定：最後一個訂閱移除時刪除看板與快照；啟動時依訂閱重建清單
2. 推文追蹤改以推文數量判斷新推文（PTT 推文時間只到分鐘，原本會漏）；文章刪除時停止追蹤並通知
3. 推文追蹤加上重疊檢查防護
4. 多看板指令先驗證全部看板，再一次寫入
5. Telegram 更新依序處理，避免同一使用者資料互相覆蓋
6. Telegram 送出失敗重試 3 次；429 依 `retry_after` 等待
7. 比對時忽略 ID 為 0（已刪除）的文章
8. `指令` 說明固定順序，移除失效網址與 LINE/粉絲團文字
9. 背景工作改成「檢查器 → Notifier」的簡單介面，取代單例加多層 channel

## 相容性

- 使用者資料（`user:<id>`）JSON 格式維持相容；多餘欄位（line、email 等）讀取時忽略
- `keyword:<board>:subs`、`article:<code>:subs`、`article:<code>:detail`、`board:<name>` key 不變
- DynamoDB 中的看板快照不搬移，第一輪重新建立基準

## 測試

- 單元測試（miniredis、假 Notifier）：關鍵字比對、指令解析、推文差異、巡邏清單同步、異常通知觸發／恢復
- 保留原本的爬蟲與工具測試
- 實機：docker compose 執行，檢查 log、狀態頁、`/health`，Telegram 傳 `狀態`、`測試 stock`
