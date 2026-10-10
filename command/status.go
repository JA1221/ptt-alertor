package command

import (
	"fmt"
	"strings"

	"github.com/Ptt-Alertor/ptt-alertor/health"
)

// StatusText describes health snapshot for chat
func StatusText(s health.Snapshot) string {
	var lines []string
	if s.Healthy() {
		lines = append(lines, "✅ 全部正常")
	} else {
		lines = append(lines, fmt.Sprintf("⚠️ 發現 %d 個問題：", len(s.Problems)))
		for _, p := range s.Problems {
			lines = append(lines, "・"+p)
		}
	}
	lines = append(lines, "")

	ptt := "正常"
	if s.PTTError != "" {
		ptt = "❌ " + s.PTTError
	}
	lines = append(lines, fmt.Sprintf("PTT：%s（%s檢查）", ptt, health.Ago(s.Now, s.PTTLastCheck)))
	telegram := "最後送出 " + health.Ago(s.Now, s.TelegramLastSent)
	if s.TelegramSendError != "" {
		telegram = "❌ 送出失敗：" + s.TelegramSendError
	}
	lines = append(lines, "Telegram："+telegram)

	lines = append(lines, "", fmt.Sprintf("看板（%d）：", len(s.Boards)))
	for _, b := range s.Boards {
		if b.OK() {
			lines = append(lines, fmt.Sprintf("✅ %s：%s更新", b.Name, health.Ago(s.Now, b.LastSuccess)))
		} else {
			lines = append(lines, fmt.Sprintf("❌ %s：已失敗 %s，%s", b.Name, health.Duration(s.Now.Sub(b.FailingSince)), b.LastError))
		}
	}

	comments := fmt.Sprintf("推文追蹤：%d 篇，%s檢查", s.CommentsTracked, health.Ago(s.Now, s.CommentsLastCheck))
	if s.CommentsError != "" {
		comments += "，最近錯誤：" + s.CommentsError
	}
	lines = append(lines, "", comments)
	lines = append(lines, "已執行 "+health.Duration(s.Now.Sub(s.Started)))
	return strings.Join(lines, "\n")
}
