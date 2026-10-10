// Package keyword keeps board -> subscriber accounts index in redis (keyword:<board>:subs),
// and the checking board list follows it
package keyword

import (
	"sort"
	"strings"

	"github.com/gomodule/redigo/redis"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
)

const (
	prefix = "keyword:"
	suffix = ":subs"
)

// Subscribers lists accounts having keywords of board
func Subscribers(boardName string) ([]string, error) {
	conn := connections.Redis()
	defer conn.Close()
	return redis.Strings(conn.Do("SMEMBERS", prefix+strings.ToLower(boardName)+suffix))
}

// Sync rebuilds index from subscriptions of accounts, and makes checking boards equal to indexed boards
func Sync(accounts []string) error {
	index := make(map[string][]string)
	for _, account := range accounts {
		u, err := user.Find(account)
		if err == user.ErrNotFound {
			continue
		}
		if err != nil {
			return err
		}
		for _, b := range u.Subscribes.KeywordBoards() {
			b = strings.ToLower(b)
			index[b] = append(index[b], account)
		}
	}

	conn := connections.Redis()
	defer conn.Close()
	keys, err := redis.Strings(conn.Do("KEYS", prefix+"*"+suffix))
	if err != nil {
		return err
	}
	conn.Send("MULTI")
	for _, key := range keys {
		conn.Send("DEL", key)
	}
	boards := make([]string, 0, len(index))
	for b, subs := range index {
		boards = append(boards, b)
		conn.Send("SADD", redis.Args{prefix + b + suffix}.AddFlat(subs)...)
	}
	if _, err := conn.Do("EXEC"); err != nil {
		return err
	}
	sort.Strings(boards)
	return board.Sync(boards)
}
