package command

import (
	"errors"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/keyword"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
	"github.com/Ptt-Alertor/ptt-alertor/notify"
)

const (
	// maxListButtons limits buttons of a list, telegram allows about 100
	maxListButtons = 60
	// testShowLimit limits articles shown per keyword by 測試
	testShowLimit = 5
)

// errLimit is shown to user as is
type errLimit string

func (e errLimit) Error() string { return string(e) }

// list shows keywords and a button for each board
func (h *Handler) list(account string, edit bool) (notify.Message, error) {
	u, err := user.Find(account)
	if err != nil {
		return notify.Message{}, err
	}
	msg := notify.Message{Edit: edit}
	lines := []string{"📋 <b>你的關鍵字</b>"}
	var buttons []notify.Button
	for _, s := range u.Subscribes.Sorted() {
		if len(s.Keywords) == 0 {
			continue
		}
		lines = append(lines, "<b>"+notify.E(s.Board)+"</b>："+notify.E(strings.Join(s.Keywords, "、")))
		buttons = append(buttons, button(fmt.Sprintf("%s（%d）", s.Board, len(s.Keywords)), cbBoardView+s.Board))
	}
	if len(lines) == 1 {
		lines = append(lines, "（還沒有關鍵字）")
	} else {
		lines = append(lines, "", "點看板可刪除或新增關鍵字。")
	}
	lines = append(lines, "", fmt.Sprintf("💬 推文追蹤 %d 篇，輸入「推文清單」查看。", u.Subscribes.ArticleCount()))
	msg.Text = strings.Join(lines, "\n")
	msg.Buttons = append(rows(buttons, 2), buttonRow(button("➕ 新增關鍵字", "新增"))...)
	return msg, nil
}

// boardView shows keywords of a board with buttons to delete them
func (h *Handler) boardView(account, boardName string, edit bool) (notify.Message, error) {
	u, err := user.Find(account)
	if err != nil {
		return notify.Message{}, err
	}
	sub, _ := u.Subscribes.Find(boardName)
	if len(sub.Keywords) == 0 {
		return h.list(account, edit)
	}
	msg := notify.Message{
		Text: strings.Join([]string{
			fmt.Sprintf("📋 <b>%s</b> 的關鍵字（%d/%d）", notify.E(sub.Board), len(sub.Keywords), MaxKeywords),
			notify.E(strings.Join(sub.Keywords, "、")),
			"",
			"點關鍵字可刪除。",
		}, "\n"),
		Edit: edit,
	}
	var buttons []notify.Button
	for _, kw := range sub.Keywords {
		if data := cbDeleteKeyword + sub.Board + "|" + kw; len(data) <= callbackLimit {
			buttons = append(buttons, button("🗑 "+truncate(kw, 20), data))
		}
	}
	msg.Buttons = append(rows(buttons, 2),
		[]notify.Button{button("➕ 新增", cbPickBoard+sub.Board), button("🗑 全部刪除", cbDeleteBoard+sub.Board)},
		[]notify.Button{button("🧪 測試抓取", "測試 "+sub.Board), button("⬅️ 返回清單", "清單")},
	)
	return msg, nil
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

// splitParams splits comma separated params (half or full width), empty ones are dropped
func splitParams(s string) []string {
	var params []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '，' }) {
		if p = strings.TrimSpace(p); p != "" {
			params = append(params, p)
		}
	}
	return params
}

// keywords adds or removes keywords and shows the list
func (h *Handler) keywords(account, action, boardStr, keywordStr string) (notify.Message, error) {
	problem, err := h.editKeywords(account, action, boardStr, keywordStr)
	if err != nil || problem.Text != "" {
		return problem, err
	}
	return h.added(account, action, nil)
}

// added shows the list after keywords changed
func (h *Handler) added(account, action string, err error) (notify.Message, error) {
	if err != nil {
		return notify.Message{}, err
	}
	list, err := h.list(account, false)
	list.Text = "✅ " + action + "成功\n\n" + list.Text
	return list, err
}

// EditKeywords adds (action 新增) or removes (刪除) comma separated keywords of comma separated boards,
// problem is a reason shown to user when nothing changed
func (h *Handler) EditKeywords(account, action, boardStr, keywordStr string) (problem string, err error) {
	// e.g. admins set by env who never talked to the bot, private chat id equals user id
	if _, err := user.Find(account); err == user.ErrNotFound {
		chatID, _ := strconv.ParseInt(account, 10, 64)
		if _, err := user.Register(account, chatID, ""); err != nil {
			return "", err
		}
	}
	msg, err := h.editKeywords(account, action, boardStr, keywordStr)
	return html.UnescapeString(msg.Text), err
}

// editKeywords changes keywords, the message is empty when changed,
// otherwise it tells user why nothing changed
func (h *Handler) editKeywords(account, action, boardStr, keywordStr string) (notify.Message, error) {
	boards := splitParams(boardStr)
	for i, b := range boards {
		if !boardNameRe.MatchString(b) {
			return notify.Text("板名格式錯誤：" + b), nil
		}
		boards[i] = strings.ToLower(b)
	}
	if len(boards) == 0 {
		return notify.Text("請輸入看板。"), nil
	}

	var keywords []string
	keywordStr = strings.TrimSpace(keywordStr)
	if strings.HasPrefix(keywordStr, "regexp:") {
		if _, err := regexp.Compile(strings.TrimPrefix(keywordStr, "regexp:")); err != nil {
			return notify.Text("正規表示式錯誤：" + err.Error()), nil
		}
		keywords = []string{keywordStr}
	} else {
		keywords = splitParams(keywordStr)
	}
	if len(keywords) == 0 {
		return notify.Text("請輸入關鍵字。"), nil
	}

	// all boards are checked before saving, one wrong board saves nothing
	if action == "新增" {
		if msg, ok := h.checkBoards(boards, keywordStr); !ok {
			return msg, nil
		}
	}
	err := user.Update(account, func(u *user.User) error {
		for _, b := range boards {
			if action == "新增" {
				u.Subscribes.AddKeywords(b, keywords)
			} else {
				u.Subscribes.RemoveKeywords(b, keywords)
			}
		}
		if len(u.Subscribes.KeywordBoards()) > MaxBoards {
			return errLimit(fmt.Sprintf("看板最多 %d 個，請先刪除不需要的看板。", MaxBoards))
		}
		for _, s := range u.Subscribes {
			if len(s.Keywords) > MaxKeywords {
				return errLimit(fmt.Sprintf("每個看板最多 %d 個關鍵字（%s）。", MaxKeywords, s.Board))
			}
		}
		return nil
	})
	var limit errLimit
	if errors.As(err, &limit) {
		return notify.Text(limit.Error()), nil
	}
	if err != nil {
		return notify.Message{}, err
	}
	return notify.Message{}, keyword.Update(account, true)
}

// checkBoards reports all boards exist, otherwise message tells wrong ones
// and offers retrying with suggested names
func (h *Handler) checkBoards(boards []string, keywordStr string) (notify.Message, bool) {
	var wrong []string
	fixed := make([]string, len(boards))
	copy(fixed, boards)
	fixable := true
	for i, b := range boards {
		err := h.CheckBoard(b)
		if err == nil {
			continue
		}
		line := b
		suggestion := ""
		if errors.Is(err, board.ErrNotExist) {
			suggestion = board.Suggest(b)
		} else {
			line += "（" + err.Error() + "）"
		}
		if suggestion != "" && !strings.EqualFold(suggestion, b) {
			line += "（是不是 " + suggestion + "？）"
			fixed[i] = suggestion
		} else {
			fixable = false
		}
		wrong = append(wrong, line)
	}
	if len(wrong) == 0 {
		return notify.Message{}, true
	}
	msg := notify.Text("❌ 板名錯誤，未做任何變更：\n" + strings.Join(wrong, "\n"))
	if fixable {
		data := cbAddKeyword + strings.Join(fixed, ",") + "|" + keywordStr
		if len(data) <= callbackLimit {
			msg.Buttons = buttonRow(button("➕ 改用 "+strings.Join(fixed, ",")+" 新增", data))
		}
	}
	return msg, false
}

// test fetches latest articles of board now and matches keywords of account,
// saved articles are untouched so checking is not affected
func (h *Handler) test(account, boardName string) (notify.Message, error) {
	boardName = strings.ToLower(boardName)
	back := buttonRow(button("⬅️ 返回 "+boardName, cbBoardView+boardName))
	articles, err := h.FetchBoard(boardName)
	if err != nil {
		msg := notify.Text(fmt.Sprintf("❌ %s 抓取失敗：%s", boardName, err))
		msg.Buttons = back
		return msg, nil
	}
	latest := make(article.Articles, len(articles))
	copy(latest, articles)
	sort.SliceStable(latest, func(i, j int) bool { return latest[i].ID > latest[j].ID })

	lines := []string{fmt.Sprintf("✅ <b>%s</b> 抓取成功，共 %d 篇。", notify.E(boardName), len(articles))}
	if len(latest) > 0 {
		lines = append(lines, "最新："+notify.Link(latest[0].Title, latest[0].Link))
	}

	u, err := user.Find(account)
	if err != nil {
		return notify.Message{}, err
	}
	sub, _ := u.Subscribes.Find(boardName)
	if len(sub.Keywords) == 0 {
		lines = append(lines, "", "你在 "+notify.E(boardName)+" 沒有設定關鍵字。")
		return notify.Message{Text: strings.Join(lines, "\n"), Buttons: back}, nil
	}
	for _, kw := range sub.Keywords {
		matched := latest.Match(kw)
		lines = append(lines, "", fmt.Sprintf("「%s」命中 %d 篇", notify.E(kw), len(matched)))
		if len(matched) > testShowLimit {
			matched = matched[:testShowLimit]
		}
		for _, a := range matched {
			lines = append(lines, notify.Link(a.Title, a.Link))
		}
	}
	return notify.Message{Text: strings.Join(lines, "\n"), Buttons: back}, nil
}
