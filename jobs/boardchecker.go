package jobs

import (
	"context"
	"fmt"
	"time"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/keyword"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
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

// Check fetches board, saves articles and notifies subscribers of new articles,
// the first fetch only saves articles as base of comparison
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
	if err := board.SaveArticles(name, online); err != nil {
		return err
	}
	if saved == nil {
		log.WithField("board", name).Info("Board Articles Created")
		return nil
	}
	news := board.NewArticles(saved, online)
	if len(news) == 0 {
		return nil
	}
	log.WithFields(log.Fields{"board": name, "count": len(news)}).Info("New Articles")
	c.notify(name, news)
	return nil
}

func (c *BoardChecker) notify(name string, news article.Articles) {
	accounts, err := keyword.Subscribers(name)
	if err != nil {
		log.WithError(err).WithField("board", name).Error("Get Keyword Subscribers Failed")
		return
	}
	for _, account := range accounts {
		u, err := user.Find(account)
		if err != nil {
			log.WithError(err).WithField("account", account).Error("Find User Failed")
			continue
		}
		sub, _ := u.Subscribes.Find(name)
		for _, kw := range sub.Keywords {
			matched := news.Match(kw)
			if len(matched) == 0 {
				continue
			}
			text := fmt.Sprintf("「%s」@%s\n\n%s", kw, sub.Board, matched.String())
			notify.Deliver(c.Notifier, u.ChatID(), text, health.Notification{
				Account: account,
				Kind:    health.KindKeyword,
				Board:   sub.Board,
				Summary: kw + "：" + matched[0].Title,
			})
		}
	}
}
