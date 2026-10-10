package jobs

import (
	"context"
	"sort"
	"strings"
	"time"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/command"
	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/keyword"
	"github.com/Ptt-Alertor/ptt-alertor/notify"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/rss"
)

// BoardChecker checks new articles of checking boards one by one, and notifies matched keywords
type BoardChecker struct {
	Fetch    func(board string) (article.Articles, error)
	Notifier notify.Notifier
	Health   *health.Tracker
	// Interval is pause between boards, avoids being blocked by ptt.cc
	Interval time.Duration
	// Cycle is minimum time of checking all boards
	Cycle time.Duration
}

// Run checks boards until ctx is done
func (c *BoardChecker) Run(ctx context.Context) {
	log.Info("Board Checker Start")
	loop(ctx, c.Cycle, c.round)
	log.Info("Board Checker Stop")
}

func (c *BoardChecker) round(ctx context.Context) {
	boards := board.List()
	c.Health.SetBoards(boards)
	for i, name := range boards {
		if i > 0 && !sleep(ctx, c.Interval) {
			return
		}
		err := c.Check(name)
		c.Health.BoardChecked(name, err)
		if err == nil {
			continue
		}
		log.WithError(err).WithField("board", name).Warn("Board Check Failed")
		if err == rss.ErrTooManyRequests && !sleep(ctx, time.Minute) {
			return
		}
	}
}

// Check fetches board and notifies subscribers of new articles,
// articles are saved only when changed, the first fetch only saves them as base of comparison
func (c *BoardChecker) Check(name string) error {
	online, err := c.Fetch(name)
	if err != nil {
		return err
	}
	if len(online) == 0 {
		return nil
	}
	saved, err := board.SavedArticles(name)
	if err != nil {
		return err
	}
	if saved == nil {
		log.WithField("board", name).Info("Board Articles Created")
		return board.SaveArticles(name, online)
	}
	news := board.NewArticles(saved, online)
	if len(news) == 0 {
		return nil
	}
	if err := board.SaveArticles(name, online); err != nil {
		return err
	}
	log.WithFields(log.Fields{"board": name, "count": len(news)}).Info("New Articles")
	c.notify(name, news)
	return nil
}

func (c *BoardChecker) notify(name string, news article.Articles) {
	sort.SliceStable(news, func(i, j int) bool { return news[i].ID < news[j].ID })
	accounts, err := keyword.Subscribers(name)
	if err != nil {
		log.WithError(err).WithField("board", name).Error("Get Keyword Subscribers Failed")
		return
	}
	for _, account := range accounts {
		u, ok := enabledUser(account)
		if !ok {
			continue
		}
		sub, _ := u.Subscribes.Find(name)
		keywords, matched := match(sub.Keywords, news)
		if len(matched) == 0 {
			continue
		}
		deliver(c.Notifier, u, ArticlesMessage(name, keywords, matched), health.Notification{
			Account: account,
			Kind:    health.KindKeyword,
			Board:   name,
			Summary: strings.Join(keywords, "、") + "：" + matched[0].Title,
			Link:    matched[0].Link,
		})
	}
}

// match returns keywords having matches and articles matching any keyword, each article once
func match(keywords []string, articles article.Articles) (hit []string, matched article.Articles) {
	hitKeyword := make(map[string]bool)
	for _, a := range articles {
		var ok bool
		for _, kw := range keywords {
			if a.MatchKeyword(kw) {
				ok = true
				hitKeyword[kw] = true
			}
		}
		if ok {
			matched = append(matched, a)
		}
	}
	for _, kw := range keywords {
		if hitKeyword[kw] {
			hit = append(hit, kw)
		}
	}
	return hit, matched
}

// ArticlesMessage formats new articles of board matching keywords
func ArticlesMessage(boardName string, keywords []string, articles article.Articles) notify.Message {
	lines := []string{"🔔 <b>" + notify.E(boardName) + "</b>｜" + notify.E(strings.Join(keywords, "、")), ""}
	for _, a := range articles {
		lines = append(lines, notify.Link(a.Title, a.Link))
	}
	return notify.Message{Text: strings.Join(lines, "\n"), Buttons: command.ArticlesButtons(boardName)}
}
