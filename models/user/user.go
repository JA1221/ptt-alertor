// Package user stores users in redis as user:<account> json, account is telegram user id
package user

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/models/subscription"
)

const prefix = "user:"

// ErrNotFound means user is not registered
var ErrNotFound = errors.New("user not found")

// User is a bot user, json is compatible with old versions (unused fields are dropped)
type User struct {
	Enable     bool      `json:"enable"`
	CreateTime time.Time `json:"createTime"`
	UpdateTime time.Time `json:"updateTime"`
	Profile    Profile   `json:"Profile"`
	Subscribes subscription.Subscriptions
}

// Profile is user's account and telegram chat
type Profile struct {
	Account      string `json:"account"`
	Telegram     string `json:"telegram"`
	TelegramChat int64  `json:"telegramChat"`
}

// ChatID is telegram chat to send messages, private chat id equals user id
func (u User) ChatID() int64 {
	if u.Profile.TelegramChat != 0 {
		return u.Profile.TelegramChat
	}
	id, _ := strconv.ParseInt(u.Profile.Account, 10, 64)
	return id
}

// Find gets user, err is ErrNotFound when not registered
func Find(account string) (u User, err error) {
	conn := connections.Redis()
	defer conn.Close()
	b, err := redis.Bytes(conn.Do("GET", prefix+account))
	if err == redis.ErrNil {
		return u, ErrNotFound
	}
	if err != nil {
		return u, err
	}
	err = json.Unmarshal(b, &u)
	return u, err
}

// Accounts lists registered accounts
func Accounts() ([]string, error) {
	conn := connections.Redis()
	defer conn.Close()
	keys, err := redis.Strings(conn.Do("KEYS", prefix+"*"))
	if err != nil {
		return nil, err
	}
	accounts := make([]string, 0, len(keys))
	for _, key := range keys {
		accounts = append(accounts, strings.TrimPrefix(key, prefix))
	}
	return accounts, nil
}

func save(u User) error {
	u.UpdateTime = time.Now()
	b, err := json.Marshal(u)
	if err != nil {
		return err
	}
	conn := connections.Redis()
	defer conn.Close()
	_, err = conn.Do("SET", prefix+u.Profile.Account, b)
	return err
}

// mu serializes read-modify-write of users from commands and background jobs
var mu sync.Mutex

// Update changes registered user by fn and saves it when fn succeeds
func Update(account string, fn func(u *User) error) error {
	mu.Lock()
	defer mu.Unlock()
	u, err := Find(account)
	if err != nil {
		return err
	}
	if err := fn(&u); err != nil {
		return err
	}
	return save(u)
}

// Register creates user or updates its telegram chat
func Register(account string, chatID int64) error {
	mu.Lock()
	defer mu.Unlock()
	u, err := Find(account)
	if err == ErrNotFound {
		u = User{
			CreateTime: time.Now(),
			Profile:    Profile{Account: account, Telegram: account},
		}
	} else if err != nil {
		return err
	}
	u.Enable = true
	u.Profile.TelegramChat = chatID
	return save(u)
}
