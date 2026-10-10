package jobs

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/models/access"
	"github.com/Ptt-Alertor/ptt-alertor/notify"
	pttHttp "github.com/Ptt-Alertor/ptt-alertor/ptt/http"
)

const pttURL = "https://www.ptt.cc/bbs/index.html"

// Monitor checks ptt.cc and redis are reachable every minute
type Monitor struct {
	Health *health.Tracker
	Client *http.Client
}

// Run checks until ctx is done
func (m *Monitor) Run(ctx context.Context) {
	loop(ctx, time.Minute, func(ctx context.Context) {
		err := m.checkPTT(ctx)
		if err != nil {
			log.WithError(err).Warn("PTT Unreachable")
		}
		m.Health.PTTChecked(err)

		err = connections.PingRedis()
		if err != nil {
			log.WithError(err).Error("Redis Unreachable")
		}
		m.Health.RedisChecked(err)
	})
}

func (m *Monitor) checkPTT(ctx context.Context) error {
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

// Alerter sends alerts and recoveries of health problems to admins
type Alerter struct {
	Health   *health.Tracker
	Notifier notify.Notifier
	// Interval is time between evaluations
	Interval time.Duration
	// admins is last known admins, alerts still reach them when redis is down
	admins []string
}

// Run evaluates health until ctx is done
func (a *Alerter) Run(ctx context.Context) {
	loop(ctx, a.Interval, func(ctx context.Context) {
		if admins, err := access.Admins(); err == nil {
			a.admins = admins
		}
		msgs := a.Health.Evaluate()
		for _, msg := range msgs {
			log.WithField("message", msg).Warn("Health Alert")
			for _, admin := range a.admins {
				chatID, err := strconv.ParseInt(admin, 10, 64)
				if err != nil {
					continue
				}
				notify.Deliver(a.Notifier, chatID, notify.Text(msg), health.Notification{
					Account: admin,
					Kind:    health.KindAlert,
					Summary: msg,
				})
			}
		}
	})
}
