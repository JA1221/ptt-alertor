// Package telegram is the telegram bot: receives updates by long polling one by one,
// only allowed users can use it, and sends messages with retries
package telegram

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"

	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/myutil"
)

const (
	maxCharacters = 4096
	sendRetries   = 3
	pollTimeout   = 50
	cancelData    = "CANCEL"
)

// HandleFunc handles text from account in chat and returns reply
type HandleFunc func(account string, chatID int64, text string) string

// Bot is telegram bot
type Bot struct {
	api     *tgbotapi.BotAPI
	token   string
	allowed map[int64]bool
	handle  HandleFunc
	health  *health.Tracker
}

// New connects telegram with token
func New(token string, allowed []int64, handle HandleFunc, tracker *health.Tracker) (*Bot, error) {
	client := &http.Client{Timeout: (pollTimeout + 30) * time.Second}
	api, err := tgbotapi.NewBotAPIWithClient(token, client)
	b := &Bot{
		api:     api,
		token:   token,
		allowed: make(map[int64]bool),
		handle:  handle,
		health:  tracker,
	}
	if err != nil {
		return nil, b.sanitize(err)
	}
	for _, id := range allowed {
		b.allowed[id] = true
	}
	return b, nil
}

// UserName is bot's telegram user name
func (b *Bot) UserName() string {
	return b.api.Self.UserName
}

// sanitize removes token from error, request urls contain token
func (b *Bot) sanitize(err error) error {
	if err == nil || b.token == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), b.token, "<token>"))
}

// Run receives updates until ctx is done, updates are handled in order
// so commands of the same user do not overwrite each other
func (b *Bot) Run(ctx context.Context) {
	if _, err := b.api.RemoveWebhook(); err != nil {
		log.WithError(b.sanitize(err)).Warn("Telegram Remove Webhook Failed")
	}
	log.Info("Telegram Bot Starts Long Polling")
	cfg := tgbotapi.NewUpdate(0)
	cfg.Timeout = pollTimeout
	for ctx.Err() == nil {
		updates, err := b.api.GetUpdates(cfg)
		err = b.sanitize(err)
		b.health.TelegramPolled(err)
		if err != nil {
			log.WithError(err).Warn("Telegram Get Updates Failed")
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
			}
			continue
		}
		for _, update := range updates {
			if update.UpdateID >= cfg.Offset {
				cfg.Offset = update.UpdateID + 1
			}
			if ctx.Err() != nil {
				return
			}
			b.handleUpdate(update)
		}
	}
}

func (b *Bot) handleUpdate(update tgbotapi.Update) {
	defer func() {
		if r := recover(); r != nil {
			log.WithField("panic", r).Error("Telegram Handle Update Panic")
		}
	}()
	switch {
	case update.CallbackQuery != nil:
		b.handleCallback(update.CallbackQuery)
	case update.Message != nil:
		b.handleMessage(update.Message)
	}
}

func (b *Bot) permitted(from *tgbotapi.User, chatID int64) bool {
	if from == nil {
		return false
	}
	if b.allowed[int64(from.ID)] {
		return true
	}
	log.WithField("id", from.ID).Warn("Telegram User Not Allowed")
	b.reply(chatID, fmt.Sprintf("這是私人 bot。\n你的 Telegram ID：%d\n要使用請將它加入 TELEGRAM_ALLOWED_IDS 後重新啟動。", from.ID))
	return false
}

var deleteAllRe = regexp.MustCompile(`^刪除\s+\S+\s+\*$`)

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	text := strings.TrimSpace(msg.Text)
	if text == "" || !b.permitted(msg.From, msg.Chat.ID) {
		return
	}
	if msg.IsCommand() {
		// "/list@bot args" -> "/list args"
		text = strings.TrimSpace("/" + msg.Command() + " " + msg.CommandArguments())
	}
	if deleteAllRe.MatchString(text) {
		b.confirm(msg.Chat.ID, text)
		return
	}
	account := strconv.Itoa(msg.From.ID)
	b.reply(msg.Chat.ID, b.handle(account, msg.Chat.ID, text))
}

func (b *Bot) handleCallback(q *tgbotapi.CallbackQuery) {
	if _, err := b.api.AnswerCallbackQuery(tgbotapi.NewCallback(q.ID, "")); err != nil {
		log.WithError(b.sanitize(err)).Warn("Telegram Answer Callback Failed")
	}
	if q.Message == nil || !b.permitted(q.From, q.Message.Chat.ID) {
		return
	}
	chatID := q.Message.Chat.ID
	if q.Data == cancelData {
		b.reply(chatID, "已取消。")
		return
	}
	b.reply(chatID, b.handle(strconv.Itoa(q.From.ID), chatID, q.Data))
}

func (b *Bot) confirm(chatID int64, cmd string) {
	msg := tgbotapi.NewMessage(chatID, "確定"+cmd+"？")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("是", cmd),
			tgbotapi.NewInlineKeyboardButtonData("否", cancelData),
		))
	if err := b.send(msg); err != nil {
		log.WithError(err).Error("Telegram Send Confirmation Failed")
	}
}

func (b *Bot) reply(chatID int64, text string) {
	if err := b.Send(chatID, text); err != nil {
		log.WithError(err).WithField("chat", chatID).Error("Telegram Reply Failed")
	}
}

// Send sends text to chat, long text is split by line breaks
func (b *Bot) Send(chatID int64, text string) error {
	for _, part := range myutil.SplitTextByLineBreak(text, maxCharacters) {
		msg := tgbotapi.NewMessage(chatID, part)
		msg.DisableWebPagePreview = true
		if err := b.send(msg); err != nil {
			return err
		}
	}
	return nil
}

// send retries on network errors and rate limits (waits retry_after),
// other errors from telegram (e.g. bot blocked) are not retried
func (b *Bot) send(msg tgbotapi.Chattable) error {
	var err error
	for attempt := 0; attempt <= sendRetries; attempt++ {
		if _, err = b.api.Send(msg); err == nil {
			b.health.TelegramSent(nil)
			return nil
		}
		wait := time.Duration(1<<attempt) * time.Second
		if apiErr, ok := err.(tgbotapi.Error); ok {
			if apiErr.RetryAfter <= 0 {
				break
			}
			wait = time.Duration(apiErr.RetryAfter) * time.Second
		}
		if attempt < sendRetries {
			log.WithError(b.sanitize(err)).WithField("wait", wait).Warn("Telegram Send Failed, Retry")
			time.Sleep(wait)
		}
	}
	err = b.sanitize(err)
	b.health.TelegramSent(err)
	return err
}
