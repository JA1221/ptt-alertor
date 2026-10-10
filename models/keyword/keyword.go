// Package keyword keeps board -> subscriber accounts index in redis (keyword:<board>:subs),
// and the checking board list follows it
package keyword

import (
	"sort"
	"strings"
	"sync"

	"github.com/gomodule/redigo/redis"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
)

const (
	prefix = "keyword:"
	suffix = ":subs"
)

// mu serializes index changes, a board must not be dropped while another user adds it
var mu sync.Mutex

func key(boardName string) string {
	return prefix + strings.ToLower(boardName) + suffix
}

// Subscribers lists accounts having keywords of board
func Subscribers(boardName string) ([]string, error) {
	conn := connections.Redis()
	defer conn.Close()
	return redis.Strings(conn.Do("SMEMBERS", key(boardName)))
}

// boardsOf lists boards having keywords of enabled user, none when user is not found
func boardsOf(account string) ([]string, error) {
	u, err := user.Find(account)
	if err == user.ErrNotFound {
		return nil, nil
	}
	if err != nil || !u.Enable {
		return nil, err
	}
	var boards []string
	for _, b := range u.Subscribes.KeywordBoards() {
		boards = append(boards, strings.ToLower(b))
	}
	return boards, nil
}

// Rebuild rebuilds index from subscriptions of accounts, and makes checking boards equal to indexed boards
func Rebuild(accounts []string) error {
	mu.Lock()
	defer mu.Unlock()
	index := make(map[string][]string)
	for _, account := range accounts {
		boards, err := boardsOf(account)
		if err != nil {
			return err
		}
		for _, b := range boards {
			index[b] = append(index[b], account)
		}
	}

	keys, err := connections.Scan(prefix + "*" + suffix)
	if err != nil {
		return err
	}
	conn := connections.Redis()
	defer conn.Close()
	conn.Send("MULTI")
	for _, k := range keys {
		conn.Send("DEL", k)
	}
	boards := make([]string, 0, len(index))
	for b, subs := range index {
		boards = append(boards, b)
		conn.Send("SADD", redis.Args{key(b)}.AddFlat(subs)...)
	}
	if _, err := conn.Do("EXEC"); err != nil {
		return err
	}
	sort.Strings(boards)
	return board.Sync(boards)
}

// Update updates index of one account after its subscriptions or access changed,
// member is false when account can no longer use the bot
func Update(account string, member bool) error {
	mu.Lock()
	defer mu.Unlock()
	var want []string
	if member {
		var err error
		if want, err = boardsOf(account); err != nil {
			return err
		}
	}
	wanted := make(map[string]bool, len(want))
	for _, b := range want {
		wanted[b] = true
	}

	conn := connections.Redis()
	defer conn.Close()
	for _, b := range board.List() {
		if wanted[b] {
			continue
		}
		removed, err := redis.Int(conn.Do("SREM", key(b), account))
		if err != nil {
			return err
		}
		if removed == 0 {
			continue
		}
		n, err := redis.Int(conn.Do("SCARD", key(b)))
		if err != nil {
			return err
		}
		if n == 0 {
			if err := board.Remove(b); err != nil {
				return err
			}
		}
	}
	for _, b := range want {
		if _, err := conn.Do("SADD", key(b), account); err != nil {
			return err
		}
		if err := board.Add(b); err != nil {
			return err
		}
	}
	return nil
}
