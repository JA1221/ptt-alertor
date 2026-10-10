// Package notify defines messages sent to users and records sent notifications
package notify

import (
	"errors"
	"html"
	"sync"
	"time"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/health"
)

// ErrUnreachable means the chat can not receive messages, e.g. user blocked the bot
var ErrUnreachable = errors.New("chat unreachable")

// Button is an inline button, it runs Data as callback or opens URL
type Button struct {
	Text string
	Data string
	URL  string
}

// Message is a HTML message, dynamic content must be escaped by E
type Message struct {
	Text    string
	Buttons [][]Button
	// Keyboard shows main reply keyboard, ignored when Buttons exist
	Keyboard bool
	// Edit replaces the message whose button was pressed
	Edit bool
}

// Text makes plain text message
func Text(s string) Message {
	return Message{Text: E(s)}
}

// E escapes text for HTML messages
func E(s string) string {
	return html.EscapeString(s)
}

// Link makes HTML link
func Link(text, url string) string {
	if url == "" {
		return E(text)
	}
	return `<a href="` + E(url) + `">` + E(text) + `</a>`
}

// Notifier sends message to a chat
type Notifier interface {
	Send(chatID int64, msg Message) error
}

// Relay sends by a notifier set later, e.g. after telegram is connected
type Relay struct {
	mu sync.RWMutex
	n  Notifier
}

// Set sets notifier to send by
func (r *Relay) Set(n Notifier) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n = n
}

// Send sends by the set notifier, fails before it is set
func (r *Relay) Send(chatID int64, msg Message) error {
	r.mu.RLock()
	n := r.n
	r.mu.RUnlock()
	if n == nil {
		return errors.New("telegram not connected")
	}
	return n.Send(chatID, msg)
}

// Deliver sends message, and records it in recent notifications when sent
func Deliver(n Notifier, chatID int64, msg Message, record health.Notification) error {
	fields := log.Fields{
		"account": record.Account,
		"kind":    record.Kind,
		"board":   record.Board,
	}
	if err := n.Send(chatID, msg); err != nil {
		log.WithError(err).WithFields(fields).Error("Notification Not Sent")
		return err
	}
	record.Time = time.Now()
	health.Record(record)
	log.WithFields(fields).Info("Notification Sent")
	return nil
}
