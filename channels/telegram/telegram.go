// Package telegram is the telegram bot: receives updates by long polling,
// updates of the same user are handled in order and different users in parallel,
// and sends HTML messages with retries
package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"

	"github.com/Ptt-Alertor/ptt-alertor/command"
	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/myutil"
	"github.com/Ptt-Alertor/ptt-alertor/notify"
)

const (
	maxCharacters = 4096
	sendRetries   = 3
	pollTimeout   = 50
	workers       = 8
)

// HandleFunc handles input and returns reply
type HandleFunc func(in command.Input) notify.Message

// Bot is telegram bot
type Bot struct {
	api    *tgbotapi.BotAPI
	token  string
	handle HandleFunc
	health *health.Tracker
}

// New connects telegram with token
func New(token string, handle HandleFunc, tracker *health.Tracker) (*Bot, error) {
	client := &http.Client{Timeout: (pollTimeout + 30) * time.Second}
	b := &Bot{token: token, handle: handle, health: tracker}
	api, err := tgbotapi.NewBotAPIWithClient(token, client)
	if err != nil {
		return nil, b.sanitize(err)
	}
	b.api = api
	return b, nil
}

// Verify checks token and returns bot user name
func Verify(token string) (string, error) {
	b, err := New(token, nil, nil)
	if err != nil {
		return "", err
	}
	return b.UserName(), nil
}

// UserName is bot's telegram user name
func (b *Bot) UserName() string {
	return b.api.Self.UserName
}

// SetHandler sets handler of updates, it must be set before Run
func (b *Bot) SetHandler(handle HandleFunc) {
	b.handle = handle
}

// sanitize removes token from error, request urls contain token
func (b *Bot) sanitize(err error) error {
	if err == nil || b.token == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), b.token, "<token>"))
}

var menu = []struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}{
	{"add", "新增關鍵字"},
	{"list", "我的關鍵字"},
	{"articles", "推文追蹤清單"},
	{"status", "運作狀態"},
	{"help", "指令說明"},
}

func (b *Bot) setMenu() {
	commands, _ := json.Marshal(menu)
	if _, err := b.api.MakeRequest("setMyCommands", url.Values{"commands": {string(commands)}}); err != nil {
		log.WithError(b.sanitize(err)).Warn("Telegram Set Commands Failed")
	}
}

// descriptions are shown to people opening the bot before they start a chat
var descriptions = []struct{ get, set, field, value string }{
	{"getMyDescription", "setMyDescription", "description",
		"PTT 關鍵字通知：看板有新文章的標題符合你的關鍵字就通知你，也能追蹤文章推文。\n\n按「開始」後照著指示使用，不用記指令。"},
	{"getMyShortDescription", "setMyShortDescription", "short_description",
		"PTT 新文章關鍵字通知、推文追蹤"},
}

// setDescriptions fills empty descriptions, ones set by BotFather are kept
func (b *Bot) setDescriptions() {
	for _, d := range descriptions {
		resp, err := b.api.MakeRequest(d.get, url.Values{})
		if err != nil {
			log.WithError(b.sanitize(err)).Warn("Telegram Get Description Failed")
			continue
		}
		var current map[string]string
		if json.Unmarshal(resp.Result, &current) != nil || current[d.field] != "" {
			continue
		}
		if _, err := b.api.MakeRequest(d.set, url.Values{d.field: {d.value}}); err != nil {
			log.WithError(b.sanitize(err)).Warn("Telegram Set Description Failed")
		}
	}
}

// Run receives updates until ctx is done
func (b *Bot) Run(ctx context.Context) {
	if _, err := b.api.RemoveWebhook(); err != nil {
		log.WithError(b.sanitize(err)).Warn("Telegram Remove Webhook Failed")
	}
	b.setMenu()
	b.setDescriptions()

	// updates of a user always go to the same worker, so they are handled in order
	queues := make([]chan tgbotapi.Update, workers)
	var wg sync.WaitGroup
	for i := range queues {
		queues[i] = make(chan tgbotapi.Update, 100)
		wg.Add(1)
		go func(q chan tgbotapi.Update) {
			defer wg.Done()
			for update := range q {
				b.handleUpdate(update)
			}
		}(queues[i])
	}
	defer func() {
		for _, q := range queues {
			close(q)
		}
		wg.Wait()
	}()

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
			if from := sender(update); from != nil {
				queues[from.ID%workers] <- update
			}
		}
	}
}

func sender(update tgbotapi.Update) *tgbotapi.User {
	switch {
	case update.CallbackQuery != nil:
		return update.CallbackQuery.From
	case update.Message != nil:
		return update.Message.From
	}
	return nil
}

func name(u *tgbotapi.User) string {
	n := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if n == "" {
		n = u.UserName
	}
	return n
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

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	text := strings.TrimSpace(msg.Text)
	// only private chats, the bot is personal to each user
	if text == "" || msg.From == nil || !msg.Chat.IsPrivate() {
		return
	}
	if msg.IsCommand() {
		// "/list@bot args" -> "/list args"
		text = strings.TrimSpace("/" + msg.Command() + " " + msg.CommandArguments())
	}
	reply := b.handle(command.Input{
		Account: strconv.Itoa(msg.From.ID),
		ChatID:  msg.Chat.ID,
		Name:    name(msg.From),
		Text:    text,
	})
	b.reply(msg.Chat.ID, reply)
}

// maxToast is max characters of a callback answer
const maxToast = 200

func (b *Bot) handleCallback(q *tgbotapi.CallbackQuery) {
	if q.Message == nil || q.From == nil {
		b.answer(q.ID, "")
		return
	}
	chatID := q.Message.Chat.ID
	reply := b.handle(command.Input{
		Account:  strconv.Itoa(q.From.ID),
		ChatID:   chatID,
		Name:     name(q.From),
		Text:     q.Data,
		Callback: true,
	})
	// the button keeps loading until answered
	b.answer(q.ID, reply.Toast)
	if reply.Text == "" {
		return
	}
	if reply.Edit && b.edit(chatID, q.Message.MessageID, reply) == nil {
		return
	}
	b.reply(chatID, reply)
}

func (b *Bot) answer(id, toast string) {
	if r := []rune(toast); len(r) > maxToast {
		toast = string(r[:maxToast-1]) + "…"
	}
	if _, err := b.api.AnswerCallbackQuery(tgbotapi.NewCallback(id, toast)); err != nil {
		log.WithError(b.sanitize(err)).Warn("Telegram Answer Callback Failed")
	}
}

func (b *Bot) reply(chatID int64, msg notify.Message) {
	if err := b.Send(chatID, msg); err != nil {
		log.WithError(err).WithField("chat", chatID).Error("Telegram Reply Failed")
	}
}

func inlineMarkup(rows [][]notify.Button) *tgbotapi.InlineKeyboardMarkup {
	if len(rows) == 0 {
		return nil
	}
	markup := tgbotapi.InlineKeyboardMarkup{}
	for _, row := range rows {
		var buttons []tgbotapi.InlineKeyboardButton
		for _, btn := range row {
			if btn.URL != "" {
				buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonURL(btn.Text, btn.URL))
			} else {
				buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(btn.Text, btn.Data))
			}
		}
		markup.InlineKeyboard = append(markup.InlineKeyboard, buttons)
	}
	return &markup
}

func replyKeyboard() tgbotapi.ReplyKeyboardMarkup {
	var rows [][]tgbotapi.KeyboardButton
	for _, row := range command.KeyboardRows {
		var buttons []tgbotapi.KeyboardButton
		for _, text := range row {
			buttons = append(buttons, tgbotapi.NewKeyboardButton(text))
		}
		rows = append(rows, buttons)
	}
	keyboard := tgbotapi.NewReplyKeyboard(rows...)
	keyboard.ResizeKeyboard = true
	return keyboard
}

func (b *Bot) edit(chatID int64, messageID int, msg notify.Message) error {
	if len([]rune(msg.Text)) > maxCharacters {
		return errors.New("message too long to edit")
	}
	edit := tgbotapi.NewEditMessageText(chatID, messageID, msg.Text)
	edit.ParseMode = tgbotapi.ModeHTML
	edit.DisableWebPagePreview = true
	edit.ReplyMarkup = inlineMarkup(msg.Buttons)
	_, err := b.api.Send(edit)
	if err != nil && strings.Contains(err.Error(), "message is not modified") {
		return nil
	}
	return b.sanitize(err)
}

// Send sends message to chat, long text is split by line breaks and buttons go with the last part
func (b *Bot) Send(chatID int64, msg notify.Message) error {
	parts := myutil.SplitTextByLineBreak(msg.Text, maxCharacters)
	for i, part := range parts {
		m := tgbotapi.NewMessage(chatID, part)
		m.ParseMode = tgbotapi.ModeHTML
		m.DisableWebPagePreview = true
		if i == len(parts)-1 {
			if markup := inlineMarkup(msg.Buttons); markup != nil {
				m.ReplyMarkup = markup
			} else if msg.Keyboard {
				m.ReplyMarkup = replyKeyboard()
			}
		}
		if err := b.send(m); err != nil {
			return err
		}
	}
	return nil
}

// send retries on network errors and rate limits (waits retry_after),
// unreachable chats (e.g. bot blocked) return notify.ErrUnreachable,
// broken HTML falls back to plain text
func (b *Bot) send(m tgbotapi.MessageConfig) error {
	var err error
	for attempt := 0; attempt <= sendRetries; attempt++ {
		if _, err = b.api.Send(m); err == nil {
			b.health.TelegramSent(nil)
			return nil
		}
		wait := time.Duration(1<<attempt) * time.Second
		if apiErr, ok := err.(tgbotapi.Error); ok {
			switch {
			case unreachable(apiErr.Message):
				return fmt.Errorf("%w: %s", notify.ErrUnreachable, apiErr.Message)
			case strings.Contains(apiErr.Message, "can't parse entities") && m.ParseMode != "":
				log.WithField("error", apiErr.Message).Warn("Telegram HTML Invalid, Send Plain Text")
				m.ParseMode = ""
				m.Text = notify.Plain(m.Text)
				attempt--
				continue
			case apiErr.RetryAfter <= 0:
				attempt = sendRetries
				continue
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

func unreachable(msg string) bool {
	for _, s := range []string{"bot was blocked", "user is deactivated", "chat not found", "bot can't initiate"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}
