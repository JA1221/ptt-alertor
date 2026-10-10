package telegram

import (
	"errors"
	"testing"

	"github.com/Ptt-Alertor/ptt-alertor/notify"
)

func TestSanitize(t *testing.T) {
	b := &Bot{token: "123:secret"}
	err := b.sanitize(errors.New(`Post "https://api.telegram.org/bot123:secret/getMe": timeout`))
	if err.Error() != `Post "https://api.telegram.org/bot<token>/getMe": timeout` {
		t.Errorf("sanitize() = %v", err)
	}
	if b.sanitize(nil) != nil {
		t.Error("nil stays nil")
	}
}

func TestUnreachable(t *testing.T) {
	for msg, want := range map[string]bool{
		"Forbidden: bot was blocked by the user": true,
		"Forbidden: user is deactivated":         true,
		"Bad Request: chat not found":            true,
		"Too Many Requests: retry after 5":       false,
	} {
		if got := unreachable(msg); got != want {
			t.Errorf("unreachable(%q) = %v", msg, got)
		}
	}
}

func TestInlineMarkup(t *testing.T) {
	if inlineMarkup(nil) != nil {
		t.Error("no buttons, no markup")
	}
	m := inlineMarkup([][]notify.Button{{{Text: "a", Data: "x"}, {Text: "b", URL: "https://t.me"}}})
	row := m.InlineKeyboard[0]
	if len(row) != 2 || *row[0].CallbackData != "x" || *row[1].URL != "https://t.me" {
		t.Errorf("markup = %+v", m)
	}
}

func TestReplyKeyboard(t *testing.T) {
	k := replyKeyboard()
	if !k.ResizeKeyboard || len(k.Keyboard) != 2 || k.Keyboard[0][0].Text != "➕ 新增" {
		t.Errorf("keyboard = %+v", k)
	}
}
