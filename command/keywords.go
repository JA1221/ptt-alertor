package command

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/keyword"
	"github.com/Ptt-Alertor/ptt-alertor/models/subscription"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
	"github.com/Ptt-Alertor/ptt-alertor/notify"
)

const (
	// maxListButtons limits delete buttons of list, telegram allows about 100
	maxListButtons = 60
	// testShowLimit limits articles shown per keyword by 測試
	testShowLimit = 5
)

// errLimit is shown to user as is
type errLimit string

func (e errLimit) Error() string { return string(e) }

func (h *Handler) list(account string, edit bool) (notify.Message, error) {
	u, err := user.Find(account)
	if err != nil {
		return notify.Message{}, err
	}
	msg := notify.Message{Edit: edit}
	subs := sortedSubs(u.Subscribes)
	lines := []string{"📋 <b>你的關鍵字</b>"}
	var buttons []notify.Button
	for _, s := range subs {
		if len(s.Keywords) == 0 {
			continue
		}
		lines = append(lines, "<b>"+notify.E(s.Board)+"</b>："+notify.E(strings.Join(s.Keywords, "、")))
		for _, kw := range s.Keywords {
			data := cbDeleteKeyword + s.Board + "|" + kw
			if len(data) <= callbackLimit && len(buttons) < maxListButtons {
				buttons = append(buttons, button("🗑 "+s.Board+"｜"+truncate(kw, 16), data))
			}
		}
	}
	if len(lines) == 1 {
		lines = append(lines, "（還沒有關鍵字）", "", "新增範例：<code>新增 stock 台積電</code>")
	} else {
		lines = append(lines, "", "點下方按鈕可刪除關鍵字。")
	}
	lines = append(lines, "", fmt.Sprintf("💬 推文追蹤 %d 篇，輸入「推文清單」查看。", u.Subscribes.ArticleCount()))
	msg.Text = strings.Join(lines, "\n")
	for i := 0; i < len(buttons); i += 2 {
		msg.Buttons = append(msg.Buttons, buttons[i:min(i+2, len(buttons))])
	}
	return msg, nil
}

func sortedSubs(ss subscription.Subscriptions) subscription.Subscriptions {
	sorted := make(subscription.Subscriptions, len(ss))
	copy(sorted, ss)
	sort.Slice(sorted, func(i, j int) bool { return strings.ToLower(sorted[i].Board) < strings.ToLower(sorted[j].Board) })
	return sorted
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

func (h *Handler) keywords(account, action, boardStr, keywordStr string) (notify.Message, error) {
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
	if err == nil {
		err = keyword.Update(account, true)
	}
	if err != nil {
		return notify.Message{}, err
	}
	list, err := h.list(account, false)
	list.Text = "✅ " + action + "成功\n\n" + list.Text
	return list, err
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
	articles, err := h.FetchBoard(boardName)
	if err != nil {
		return notify.Text(fmt.Sprintf("❌ %s 抓取失敗：%s", boardName, err)), nil
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
		return notify.Message{Text: strings.Join(lines, "\n")}, nil
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
	return notify.Message{Text: strings.Join(lines, "\n")}, nil
}
