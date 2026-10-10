// Package notify sends messages to users and records sent notifications
package notify

import (
	"time"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/health"
)

// Notifier sends text to a chat
type Notifier interface {
	Send(chatID int64, text string) error
}

// Deliver sends text, and records it in recent notifications when sent
func Deliver(n Notifier, chatID int64, text string, record health.Notification) error {
	if err := n.Send(chatID, text); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"account": record.Account,
			"kind":    record.Kind,
			"board":   record.Board,
		}).Error("Notification Not Sent")
		return err
	}
	record.Time = time.Now()
	health.Record(record)
	log.WithFields(log.Fields{
		"account": record.Account,
		"kind":    record.Kind,
		"board":   record.Board,
	}).Info("Notification Sent")
	return nil
}
