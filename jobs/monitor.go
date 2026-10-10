package jobs

import (
	"context"
	"fmt"
	"net/http"
	"time"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/notify"
	pttHttp "github.com/Ptt-Alertor/ptt-alertor/ptt/http"
)

const pttURL = "https://www.ptt.cc/bbs/index.html"

// PTTMonitor checks ptt.cc is reachable every minute
type PTTMonitor struct {
	Health *health.Tracker
	Client *http.Client
}

// Run checks PTT until ctx is done
func (m *PTTMonitor) Run(ctx context.Context) {
	loop(ctx, time.Minute, func(ctx context.Context) {
		err := m.check(ctx)
		if err != nil {
			log.WithError(err).Warn("PTT Unreachable")
		}
		m.Health.PTTChecked(err)
	})
}

func (m *PTTMonitor) check(ctx context.Context) error {
	req, err := pttHttp.HttpRequest(pttURL)
	if err != nil {
		return err
	}
	resp, err := m.Client.Do(req.WithContext(ctx))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	return nil
}

// Alerter sends alerts and recoveries of health problems to owners
type Alerter struct {
	Health   *health.Tracker
	Notifier notify.Notifier
	// Owners are telegram chats receiving alerts
	Owners []int64
	// Interval is time between evaluations
	Interval time.Duration
}

// Run evaluates health until ctx is done
func (a *Alerter) Run(ctx context.Context) {
	loop(ctx, a.Interval, func(ctx context.Context) {
		for _, msg := range a.Health.Evaluate() {
			log.WithField("message", msg).Warn("Health Alert")
			for _, owner := range a.Owners {
				notify.Deliver(a.Notifier, owner, msg, health.Notification{
					Account: fmt.Sprint(owner),
					Kind:    health.KindAlert,
					Summary: msg,
				})
			}
		}
	})
}
