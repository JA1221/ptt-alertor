package health

import (
	"encoding/json"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/gomodule/redigo/redis"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
)

const (
	historyKey  = "notify:recent"
	historySize = 20
)

// Notification kinds
const (
	KindKeyword = "keyword"
	KindComment = "comment"
	KindAlert   = "alert"
)

// Notification is a sent message record
type Notification struct {
	Time    time.Time `json:"time"`
	Account string    `json:"account"`
	Kind    string    `json:"kind"`
	Board   string    `json:"board"`
	Summary string    `json:"summary"`
}

// Record saves a sent notification, keeps latest ones only
func Record(n Notification) {
	b, err := json.Marshal(n)
	if err != nil {
		log.WithError(err).Error("Notification Encode Failed")
		return
	}
	conn := connections.Redis()
	defer conn.Close()
	conn.Send("MULTI")
	conn.Send("LPUSH", historyKey, b)
	conn.Send("LTRIM", historyKey, 0, historySize-1)
	if _, err := conn.Do("EXEC"); err != nil {
		log.WithError(err).Error("Notification Record Failed")
	}
}

// Recent lists latest notifications, newest first
func Recent() []Notification {
	conn := connections.Redis()
	defer conn.Close()
	items, err := redis.ByteSlices(conn.Do("LRANGE", historyKey, 0, historySize-1))
	if err != nil {
		log.WithError(err).Error("Notification List Failed")
		return nil
	}
	ns := make([]Notification, 0, len(items))
	for _, item := range items {
		var n Notification
		if err := json.Unmarshal(item, &n); err == nil {
			ns = append(ns, n)
		}
	}
	return ns
}
