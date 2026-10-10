package command

import (
	"errors"
	"strings"

	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
	"github.com/Ptt-Alertor/ptt-alertor/notify"
)

// PopularBoards are suggested when adding keywords, after boards of the user
var PopularBoards = []string{
	"Gossiping", "Stock", "Lifeismoney", "e-shopping", "MobileComm", "HardwareSale",
	"C_Chat", "Baseball", "NBA", "movie", "Tech_Job", "home-sale",
}

const maxMenuBoards = 12

// addMenu asks which board to add keywords to
func (h *Handler) addMenu(account string) (notify.Message, error) {
	u, err := user.Find(account)
	if err != nil {
		return notify.Message{}, err
	}
	var names []string
	seen := make(map[string]bool)
	add := func(name string) {
		if !seen[strings.ToLower(name)] && len(names) < maxMenuBoards {
			seen[strings.ToLower(name)] = true
			names = append(names, name)
		}
	}
	for _, s := range u.Subscribes.Sorted() {
		if len(s.Keywords) > 0 {
			add(s.Board)
		}
	}
	for _, name := range PopularBoards {
		add(name)
	}

	var buttons []notify.Button
	for _, name := range names {
		buttons = append(buttons, button(name, cbPickBoard+name))
	}
	msg := notify.Message{Text: "➕ <b>新增關鍵字</b>\n選擇看板，或直接輸入「看板 關鍵字」，例如 <code>stock 台積電</code>"}
	msg.Buttons = rows(buttons, 3)
	msg.Buttons = append(msg.Buttons, []notify.Button{button("✏️ 其他看板", cbOtherBoard), button("取消", cbCancel)})
	return msg, nil
}

// askBoard waits for a board name
func (h *Handler) askBoard(account string) notify.Message {
	h.prompts.set(account, "")
	msg := notify.Message{Text: "輸入看板名稱，例如 <code>Lifeismoney</code>", Edit: true}
	msg.Buttons = buttonRow(button("取消", cbCancel))
	return msg
}

// askKeywords waits for keywords of board
func (h *Handler) askKeywords(account, boardName string) notify.Message {
	h.prompts.set(account, strings.ToLower(boardName))
	msg := notify.Message{
		Text: strings.Join([]string{
			"輸入要在 <b>" + notify.E(boardName) + "</b> 通知的關鍵字，多個用逗號分隔：",
			"例：<code>台積電,聯發科</code>",
			"同時包含：<code>台積電&amp;法說</code>　排除：<code>台積電&amp;!公告</code>",
		}, "\n"),
		Edit: true,
	}
	msg.Buttons = buttonRow(button("取消", cbCancel))
	return msg
}

// answerPrompt handles text the bot was waiting for
func (h *Handler) answerPrompt(account string, p prompt, text string) (notify.Message, error) {
	if p.board != "" {
		problem, err := h.editKeywords(account, "新增", p.board, text)
		if err != nil || problem.Text == "" {
			return h.added(account, "新增", err)
		}
		if len(problem.Buttons) == 0 {
			problem.Buttons = buttonRow(button("✏️ 重新輸入", cbPickBoard+p.board))
		}
		return problem, nil
	}

	name := strings.ToLower(strings.TrimSpace(text))
	retry := buttonRow(button("✏️ 重新輸入", cbOtherBoard), button("取消", cbCancel))
	if !boardNameRe.MatchString(name) {
		msg := notify.Text("板名只能有英文、數字和 _ . -")
		msg.Buttons = retry
		return msg, nil
	}
	if err := h.CheckBoard(name); err != nil {
		msg := notify.Text("找不到看板 " + name + "。")
		if !errors.Is(err, board.ErrNotExist) {
			msg = notify.Text("檢查看板失敗：" + err.Error())
		} else if s := board.Suggest(name); s != "" && !strings.EqualFold(s, name) {
			msg = notify.Text("找不到看板 " + name + "，是不是 " + s + "？")
			retry = append([][]notify.Button{{button("👉 "+s, cbPickBoard+s)}}, retry...)
		}
		msg.Buttons = retry
		return msg, nil
	}
	msg := h.askKeywords(account, name)
	msg.Edit = false
	return msg, nil
}

// rows lays out buttons n per row
func rows(buttons []notify.Button, n int) [][]notify.Button {
	var rs [][]notify.Button
	for i := 0; i < len(buttons); i += n {
		rs = append(rs, buttons[i:min(i+n, len(buttons))])
	}
	return rs
}
