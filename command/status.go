package command

import (
	"fmt"
	"strings"

	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/models/access"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
	"github.com/Ptt-Alertor/ptt-alertor/notify"
)

// status also shows the main keyboard, users of older versions get new buttons
func (h *Handler) status(account string) (notify.Message, error) {
	snap := h.Health.Snapshot()
	admin, err := access.IsAdmin(account)
	if err != nil {
		return notify.Message{}, err
	}
	if admin {
		return notify.Message{Text: StatusText(snap, nil), Keyboard: true}, nil
	}
	u, err := user.Find(account)
	if err != nil {
		return notify.Message{}, err
	}
	mine := make(map[string]bool)
	for _, b := range u.Subscribes.KeywordBoards() {
		mine[strings.ToLower(b)] = true
	}
	return notify.Message{Text: StatusText(snap, mine), Keyboard: true}, nil
}

// StatusText describes health snapshot, boards are limited to only when it is not nil
func StatusText(s health.Snapshot, only map[string]bool) string {
	var boards []health.BoardStatus
	failing := 0
	for _, b := range s.Boards {
		if only != nil && !only[b.Name] {
			continue
		}
		boards = append(boards, b)
		if !b.OK() {
			failing++
		}
	}

	var lines []string
	switch {
	case only == nil && !s.Healthy():
		lines = append(lines, fmt.Sprintf("⚠️ <b>發現 %d 個問題</b>", len(s.Problems)))
		for _, p := range s.Problems {
			lines = append(lines, "・"+notify.E(p))
		}
	case only != nil && (s.PTTError != "" || failing > 0):
		lines = append(lines, "⚠️ <b>部分功能異常</b>")
	default:
		lines = append(lines, "✅ <b>全部正常</b>")
	}
	lines = append(lines, "")

	ptt := "正常"
	if s.PTTError != "" {
		ptt = "❌ " + s.PTTError
	}
	lines = append(lines, fmt.Sprintf("PTT：%s（%s檢查）", notify.E(ptt), health.Ago(s.Now, s.PTTLastCheck)))
	if only == nil {
		telegram := "最後送出 " + health.Ago(s.Now, s.TelegramLastSent)
		if s.TelegramSendError != "" {
			telegram = "❌ 送出失敗：" + s.TelegramSendError
		}
		lines = append(lines, "Telegram："+notify.E(telegram))
	}

	lines = append(lines, "", fmt.Sprintf("<b>看板</b>（%d）", len(boards)))
	for _, b := range boards {
		if b.OK() {
			lines = append(lines, fmt.Sprintf("✅ %s：%s更新", notify.E(b.Name), health.Ago(s.Now, b.LastSuccess)))
		} else {
			lines = append(lines, fmt.Sprintf("❌ %s：已失敗 %s，%s", notify.E(b.Name), health.Duration(s.Now.Sub(b.FailingSince)), notify.E(b.LastError)))
		}
	}
	if len(boards) == 0 {
		lines = append(lines, "（沒有巡邏中的看板）")
	}

	if only == nil {
		comments := fmt.Sprintf("推文追蹤：%d 篇，%s檢查", s.CommentsTracked, health.Ago(s.Now, s.CommentsLastCheck))
		if s.CommentsError != "" {
			comments += "，最近錯誤：" + s.CommentsError
		}
		lines = append(lines, "", notify.E(comments), "已執行 "+health.Duration(s.Now.Sub(s.Started)))
	}
	return strings.Join(lines, "\n")
}
