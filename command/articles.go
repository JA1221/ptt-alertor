package command

import (
	"fmt"
	"strings"

	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/subscription"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
	"github.com/Ptt-Alertor/ptt-alertor/notify"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/web"
)

func (h *Handler) addArticle(account, boardName, code string) (notify.Message, error) {
	u, err := user.Find(account)
	if err != nil {
		return notify.Message{}, err
	}
	if u.Subscribes.HasArticle(code) {
		return notify.Text("已在追蹤這篇文章。"), nil
	}
	if u.Subscribes.ArticleCount() >= MaxArticles {
		return notify.Text(fmt.Sprintf("推文追蹤最多 %d 篇，輸入「推文清單」整理追蹤列表。", MaxArticles)), nil
	}

	a, tracked := article.Find(code)
	if !tracked {
		online, err := h.FetchArticle(boardName, code)
		if _, ok := err.(web.URLNotFoundError); ok {
			return notify.Text("文章不存在。"), nil
		}
		if err != nil {
			return notify.Text("抓取文章失敗：" + err.Error()), nil
		}
		// comments so far are the base, only later ones are sent
		a = article.Article{
			ID:           online.ID,
			Code:         code,
			Title:        online.Title,
			Link:         online.Link,
			Board:        online.Board,
			CommentCount: len(online.Comments),
		}
		if err := a.Save(); err != nil {
			return notify.Message{}, err
		}
	}
	if err := article.Track(code, account); err != nil {
		return notify.Message{}, err
	}
	err = user.Update(account, func(u *user.User) error {
		u.Subscribes.AddArticle(a.Board, code)
		return nil
	})
	if err != nil {
		return notify.Message{}, err
	}
	return notify.Message{Text: "✅ 開始追蹤推文\n" + notify.Link(a.Title, a.Link)}, nil
}

func (h *Handler) removeArticle(account, code string) (notify.Message, error) {
	var removed bool
	err := user.Update(account, func(u *user.User) error {
		removed = u.Subscribes.RemoveArticle(code)
		return nil
	})
	if err == nil {
		err = article.Untrack(code, account)
	}
	if err != nil {
		return notify.Message{}, err
	}
	if !removed {
		return notify.Text("沒有追蹤這篇文章。"), nil
	}
	return notify.Text("✅ 已停止追蹤推文。"), nil
}

func (h *Handler) articleList(account string, edit bool) (notify.Message, error) {
	u, err := user.Find(account)
	if err != nil {
		return notify.Message{}, err
	}
	msg := notify.Message{Edit: edit}
	if u.Subscribes.ArticleCount() == 0 {
		msg.Text = "💬 還沒有追蹤任何文章。\n直接貼上 PTT 文章網址就能追蹤推文。"
		return msg, nil
	}
	lines := []string{fmt.Sprintf("💬 <b>推文追蹤</b>（%d/%d）", u.Subscribes.ArticleCount(), MaxArticles)}
	for _, s := range u.Subscribes.Sorted() {
		for _, code := range s.Articles {
			title := code
			link := subscription.ArticleURL(s.Board, code)
			if a, ok := article.Find(code); ok {
				title, link = a.Title, a.Link
			}
			lines = append(lines, "<b>"+notify.E(s.Board)+"</b>｜"+notify.Link(title, link))
			if len(msg.Buttons) < maxListButtons {
				msg.Buttons = append(msg.Buttons, buttonRow(button("🗑 "+truncate(title, 24), cbDeleteArticle+code))...)
			}
		}
	}
	lines = append(lines, "", "點下方按鈕可停止追蹤；輸入「清理推文」可移除失效的追蹤。")
	msg.Text = strings.Join(lines, "\n")
	return msg, nil
}

// cleanArticles removes tracking of articles which are no longer checked (e.g. data lost)
func (h *Handler) cleanArticles(account string) (notify.Message, error) {
	var cleaned int
	err := user.Update(account, func(u *user.User) error {
		var codes []string
		for _, s := range u.Subscribes {
			codes = append(codes, s.Articles...)
		}
		for _, code := range codes {
			if _, ok := article.Find(code); ok {
				continue
			}
			if err := article.Untrack(code, account); err != nil {
				return err
			}
			u.Subscribes.RemoveArticle(code)
			cleaned++
		}
		return nil
	})
	if err != nil {
		return notify.Message{}, err
	}
	return notify.Text(fmt.Sprintf("清理 %d 則推文追蹤。", cleaned)), nil
}
