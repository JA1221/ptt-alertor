// Package jobs runs background checks: new articles of boards, new comments of tracked articles,
// PTT and redis availability and alerts of failures
package jobs

import (
	"context"
	"errors"
	"runtime/debug"
	"time"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/models/keyword"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
	"github.com/Ptt-Alertor/ptt-alertor/notify"
)

// sleep waits d, returns false when ctx is done first
func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// loop runs round repeatedly, each round takes cycle at least,
// a panic in a round is logged and the next round still runs
func loop(ctx context.Context, cycle time.Duration, round func(ctx context.Context)) {
	for ctx.Err() == nil {
		start := time.Now()
		safely(ctx, round)
		if !sleep(ctx, cycle-time.Since(start)) {
			return
		}
	}
}

func safely(ctx context.Context, round func(ctx context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			log.WithField("panic", r).WithField("stack", string(debug.Stack())).Error("Job Panic")
		}
	}()
	round(ctx)
}

// enabledUser finds user to notify, false when not found or disabled
func enabledUser(account string) (user.User, bool) {
	u, err := user.Find(account)
	if err != nil {
		log.WithError(err).WithField("account", account).Error("Find User Failed")
		return u, false
	}
	return u, u.Enable
}

// deliver sends message to user, users who blocked the bot are disabled
// until they talk to the bot again
func deliver(n notify.Notifier, u user.User, msg notify.Message, record health.Notification) {
	err := notify.Deliver(n, u.ChatID(), msg, record)
	if !errors.Is(err, notify.ErrUnreachable) {
		return
	}
	account := u.Profile.Account
	log.WithField("account", account).Warn("User Unreachable, Disabled")
	if err := user.Disable(account); err != nil {
		log.WithError(err).WithField("account", account).Error("Disable User Failed")
		return
	}
	if err := keyword.Update(account, false); err != nil {
		log.WithError(err).WithField("account", account).Error("Update Keyword Index Failed")
	}
}
