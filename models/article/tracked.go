package article

import (
	"encoding/json"
	"strings"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/gomodule/redigo/redis"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
)

// tracked articles (comment subscriptions) are stored in redis:
// article:<code>:detail hash of board and content json, article:<code>:subs set of accounts
const (
	prefix       = "article:"
	subsSuffix   = ":subs"
	detailSuffix = ":detail"
)

// Find gets tracked article, ok is false when not found
func Find(code string) (a Article, ok bool) {
	conn := connections.Redis()
	defer conn.Close()

	m, err := redis.StringMap(conn.Do("HGETALL", prefix+code+detailSuffix))
	if err != nil {
		log.WithError(err).WithField("code", code).Error("Find Article Failed")
		return a, false
	}
	if m["content"] == "" {
		return a, false
	}
	if err := json.Unmarshal([]byte(m["content"]), &a); err != nil {
		log.WithError(err).WithField("code", code).Error("Article Content Decode Failed")
		return a, false
	}
	a.Board = m["board"]
	return a, a.Board != "" && a.Code != ""
}

// Save saves tracked article
func (a Article) Save() error {
	content, err := json.Marshal(a)
	if err != nil {
		return err
	}
	conn := connections.Redis()
	defer conn.Close()
	_, err = conn.Do("HSET", prefix+a.Code+detailSuffix, "board", a.Board, "content", content)
	return err
}

// Track adds account as subscriber of article
func Track(code, account string) error {
	conn := connections.Redis()
	defer conn.Close()
	_, err := conn.Do("SADD", prefix+code+subsSuffix, account)
	return err
}

// Untrack removes subscriber, the article is destroyed when nobody tracks it
func Untrack(code, account string) error {
	conn := connections.Redis()
	defer conn.Close()
	if _, err := conn.Do("SREM", prefix+code+subsSuffix, account); err != nil {
		return err
	}
	n, err := redis.Int(conn.Do("SCARD", prefix+code+subsSuffix))
	if err != nil {
		return err
	}
	if n == 0 {
		return Destroy(code)
	}
	return nil
}

// Subscribers lists accounts tracking article
func Subscribers(code string) ([]string, error) {
	conn := connections.Redis()
	defer conn.Close()
	return redis.Strings(conn.Do("SMEMBERS", prefix+code+subsSuffix))
}

// TrackedCodes lists codes of tracked articles
func TrackedCodes() ([]string, error) {
	keys, err := connections.Scan(prefix + "*" + subsSuffix)
	if err != nil {
		return nil, err
	}
	codes := make([]string, 0, len(keys))
	for _, key := range keys {
		codes = append(codes, strings.TrimSuffix(strings.TrimPrefix(key, prefix), subsSuffix))
	}
	return codes, nil
}

// Destroy deletes tracked article and its subscribers
func Destroy(code string) error {
	conn := connections.Redis()
	defer conn.Close()
	_, err := conn.Do("DEL", prefix+code+detailSuffix, prefix+code+subsSuffix)
	return err
}
