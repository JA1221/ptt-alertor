# Ptt-Alertor

<img align="right" src="https://raw.githubusercontent.com/Ptt-Alertor/ptt-alertor/master/logo.jpg">

[![Build Status](https://github.com/Ptt-Alertor/ptt-alertor/actions/workflows/main.yml/badge.svg)](https://github.com/Ptt-Alertor/ptt-alertor/actions/workflows/main.yml)
[![codecov](https://codecov.io/gh/Ptt-Alertor/ptt-alertor/branch/master/graph/badge.svg)](https://codecov.io/gh/Ptt-Alertor/ptt-alertor)
[![Go Report Card](https://goreportcard.com/badge/github.com/Ptt-Alertor/ptt-alertor)](https://goreportcard.com/report/github.com/Ptt-Alertor/ptt-alertor)
[![Code Climate](https://api.codeclimate.com/v1/badges/f7047295fce56a0465dc/maintainability)](https://codeclimate.com/github/Ptt-Alertor/ptt-alertor/maintainability)
[![StackShare](https://img.shields.io/badge/tech-stack-0690fa.svg?style=flat)](https://stackshare.io/ptt-alertor/ptt-alertor)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

## 本機執行

需求：Docker Desktop（含 docker compose）

```sh
cp .env.example .env   # 至少設定 AUTH_USER / AUTH_PW，要用 Telegram 再填 TELEGRAM_TOKEN
docker compose up -d --build
```

* 網站：<http://localhost:9090>
* 會一併啟動 Redis 與 DynamoDB Local，資料存在 docker volume（`docker compose down -v` 會清空）
* 首次啟動會自動建立 DynamoDB Local 的 `boards`、`articles` 資料表
* `APP_HOST` 不是 `https://` 開頭時，Telegram bot 改用 long polling，不需要公開網址；
  向 [@BotFather](https://t.me/BotFather) 申請 token 填入 `TELEGRAM_TOKEN` 即可在 Telegram 上使用
* 未設定的 LINE / Telegram 頻道會停用並在 log 顯示警告，不會讓程式中止
* 看 log：`docker compose logs -f app`

執行測試：

```sh
docker run --rm -v "$PWD:/src" -w /src golang:1.15 go test -race -tags test ./...
```

## API

### Board

* GET /boards

* GET /boards/[board name]/articles

* GET /boards/[board name]/articles/[article code]

### Keyword

* GET /keyword/boards

### Author

* GET /author/boards

### PushSum

* GET /pushsum/boards

### Articles

* GET /articles

### User (Auth)

* GET /users

* GET /users/[account]

* POST /users

```json
{
    "profile":{
        "account": "sample",
        "email":"sample@mail.com"
    },
    "subscribes":[
        {
            "board":"gossiping",
            "keywords":["問卦","爆卦","公告"]
        },
        {
            "board":"lol",
            "keywords":["閒聊"]
        }
    ]
}
```

* PUT /users/[account]

```json
{
    "profile":{
        "account": "sample",
        "email":"sample@mail.com"
    },
    "subscribes":[]
}
```

## Credits

### Real Life

Rose Li, Aries Huang, Scott Kao, Amy Li

### Ptt

DMM, oas, bestpika, Zero0910, lucky0509, wbreeze, chang0206, lindo0130, hungys, gyman7788, tooilxui, myamyakoko, whkuo, papago89, timeline, Kamikiri

### Facebook

Mr.clu, Woqeker