package jobs

import (
	"context"
	"strings"
	"time"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
	"github.com/Ptt-Alertor/ptt-alertor/notify"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/web"
)

// CommentChecker checks new comments of tracked articles one by one,
// rounds never overlap so the same comments are not sent twice
type CommentChecker struct {
	FetchArticle func(board, code string) (article.Article, error)
	Notifier     notify.Notifier
	Health       *health.Tracker
	// Interval is pause between articles
	Interval time.Duration
	// Cycle is minimum time of checking all articles
	Cycle time.Duration
}

// Run checks tracked articles until ctx is done
func (c *CommentChecker) Run(ctx context.Context) {
	log.Info("Comment Checker Start")
	loop(ctx, c.Cycle, c.round)
	log.Info("Comment Checker Stop")
}

func (c *CommentChecker) round(ctx context.Context) {
	codes, err := article.TrackedCodes()
	if err != nil {
		c.Health.CommentsChecked(0, err)
		return
	}
	var lastErr error
	for i, code := range codes {
		if i > 0 && !sleep(ctx, c.Interval) {
			return
		}
		if err := c.Check(code); err != nil {
			log.WithError(err).WithField("code", code).Warn("Comment Check Failed")
			lastErr = err
		}
	}
	c.Health.CommentsChecked(len(codes), lastErr)
}

// legacy reports article saved by old versions, which kept comments of last update instead of count
func legacy(a article.Article) bool {
	return a.CommentCount == 0 && (len(a.Comments) > 0 || !a.LastPushDateTime.IsZero())
}

// Check fetches article and notifies subscribers of new comments,
// subscribers are told and tracking stops when the article is deleted
func (c *CommentChecker) Check(code string) error {
	saved, ok := article.Find(code)
	if !ok {
		return nil
	}
	online, err := c.FetchArticle(saved.Board, code)
	if _, gone := err.(web.URLNotFoundError); gone {
		return c.stop(saved)
	}
	if err != nil {
		return err
	}

	var news article.Comments
	if !legacy(saved) {
		news = article.NewComments(saved, online)
	}
	// count only grows, a broken page with fewer comments must not cause sending old comments again
	if count := len(online.Comments); count > saved.CommentCount || legacy(saved) {
		saved.CommentCount = count
		saved.Comments = nil
		saved.LastPushDateTime = time.Time{}
		if saved.Title == "" {
			saved.Title = online.Title
		}
		if saved.Link == "" {
			saved.Link = online.Link
		}
		if err := saved.Save(); err != nil {
			return err
		}
	}
	if len(news) == 0 {
		return nil
	}
	log.WithFields(log.Fields{"board": saved.Board, "code": code, "count": len(news)}).Info("New Comments")
	c.send(saved, CommentsMessage(saved, news))
	return nil
}

// CommentsMessage formats new comments of article
func CommentsMessage(a article.Article, comments article.Comments) notify.Message {
	lines := []string{"💬 <b>" + notify.E(a.Board) + "</b>｜" + notify.Link(a.Title, a.Link), ""}
	for _, c := range comments {
		lines = append(lines, notify.E(c.String()))
	}
	return notify.Message{Text: strings.Join(lines, "\n")}
}

func (c *CommentChecker) send(a article.Article, msg notify.Message) {
	accounts, err := article.Subscribers(a.Code)
	if err != nil {
		log.WithError(err).WithField("code", a.Code).Error("Get Article Subscribers Failed")
		return
	}
	for _, account := range accounts {
		u, err := user.Find(account)
		if err != nil {
			log.WithError(err).WithField("account", account).Error("Find User Failed")
			continue
		}
		if !u.Enable {
			continue
		}
		deliver(c.Notifier, u, msg, health.Notification{
			Account: account,
			Kind:    health.KindComment,
			Board:   a.Board,
			Summary: a.Title,
		})
	}
}

func (c *CommentChecker) stop(a article.Article) error {
	log.WithFields(log.Fields{"board": a.Board, "code": a.Code}).Info("Article Deleted, Stop Tracking")
	c.send(a, notify.Message{Text: "🗑 文章已刪除，停止追蹤推文\n<b>" + notify.E(a.Board) + "</b>｜" + notify.E(a.Title)})
	accounts, err := article.Subscribers(a.Code)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		err := user.Update(account, func(u *user.User) error {
			u.Subscribes.RemoveArticle(a.Code)
			return nil
		})
		if err != nil && err != user.ErrNotFound {
			return err
		}
	}
	return article.Destroy(a.Code)
}
