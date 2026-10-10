// Package access keeps who can use the bot and app settings in redis:
// access:admins, access:members, access:pending sets and settings hash
package access

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gomodule/redigo/redis"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
)

const (
	adminsKey   = "access:admins"
	membersKey  = "access:members"
	pendingKey  = "access:pending"
	settingsKey = "settings"

	tokenField        = "telegram_token"
	registrationField = "registration"
	claimField        = "claim_code"
)

// Registration modes decide how strangers become members
const (
	ModeApproval = "approval"
	ModeOpen     = "open"
	ModeClosed   = "closed"
)

// ModeNames are names of registration modes shown to users
var ModeNames = map[string]string{
	ModeApproval: "審核",
	ModeOpen:     "開放",
	ModeClosed:   "關閉",
}

func do(cmd string, args ...interface{}) (interface{}, error) {
	conn := connections.Redis()
	defer conn.Close()
	return conn.Do(cmd, args...)
}

// IsAdmin reports account is admin
func IsAdmin(account string) (bool, error) {
	return redis.Bool(do("SISMEMBER", adminsKey, account))
}

// IsMember reports account can use the bot, admins are members
func IsMember(account string) (bool, error) {
	return redis.Bool(do("SISMEMBER", membersKey, account))
}

// Admins lists admin accounts
func Admins() ([]string, error) {
	return redis.Strings(do("SMEMBERS", adminsKey))
}

// Members lists member accounts
func Members() ([]string, error) {
	return redis.Strings(do("SMEMBERS", membersKey))
}

// Pending lists accounts waiting for approval
func Pending() ([]string, error) {
	return redis.Strings(do("SMEMBERS", pendingKey))
}

// AddAdmin makes account admin and member
func AddAdmin(account string) error {
	conn := connections.Redis()
	defer conn.Close()
	conn.Send("MULTI")
	conn.Send("SADD", adminsKey, account)
	conn.Send("SADD", membersKey, account)
	conn.Send("SREM", pendingKey, account)
	_, err := conn.Do("EXEC")
	return err
}

// Approve makes account member
func Approve(account string) error {
	conn := connections.Redis()
	defer conn.Close()
	conn.Send("MULTI")
	conn.Send("SADD", membersKey, account)
	conn.Send("SREM", pendingKey, account)
	_, err := conn.Do("EXEC")
	return err
}

// Request adds account to pending, added is false when it was pending already
func Request(account string) (added bool, err error) {
	return redis.Bool(do("SADD", pendingKey, account))
}

// Reject removes account from pending
func Reject(account string) error {
	_, err := do("SREM", pendingKey, account)
	return err
}

// Remove removes account from members and admins
func Remove(account string) error {
	conn := connections.Redis()
	defer conn.Close()
	conn.Send("MULTI")
	conn.Send("SREM", membersKey, account)
	conn.Send("SREM", adminsKey, account)
	conn.Send("SREM", pendingKey, account)
	_, err := conn.Do("EXEC")
	return err
}

// Migrate makes existing enabled users members when upgrading from versions without access control
func Migrate() error {
	exists, err := redis.Bool(do("EXISTS", membersKey))
	if err != nil || exists {
		return err
	}
	accounts, err := user.Accounts()
	if err != nil {
		return err
	}
	for _, account := range accounts {
		u, err := user.Find(account)
		if err != nil {
			continue
		}
		if u.Enable {
			if _, err := do("SADD", membersKey, account); err != nil {
				return err
			}
		}
	}
	return nil
}

func setting(field string) (string, error) {
	s, err := redis.String(do("HGET", settingsKey, field))
	if err == redis.ErrNil {
		return "", nil
	}
	return s, err
}

func setSetting(field, value string) error {
	_, err := do("HSET", settingsKey, field, value)
	return err
}

// Token gets telegram token saved by setup page
func Token() (string, error) {
	return setting(tokenField)
}

// SetToken saves telegram token
func SetToken(token string) error {
	return setSetting(tokenField, token)
}

// Registration gets registration mode, default approval
func Registration() (string, error) {
	mode, err := setting(registrationField)
	if mode == "" {
		mode = ModeApproval
	}
	return mode, err
}

// SetRegistration sets registration mode
func SetRegistration(mode string) error {
	return setSetting(registrationField, mode)
}

// ClaimCode gets code for claiming admin, it is created when missing
func ClaimCode() (string, error) {
	code, err := setting(claimField)
	if err != nil || code != "" {
		return code, err
	}
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	code = hex.EncodeToString(b)
	// another caller may create it first
	if _, err := do("HSETNX", settingsKey, claimField, code); err != nil {
		return "", err
	}
	return setting(claimField)
}

// Claim makes account admin when code matches, the code is used once
func Claim(account, code string) (bool, error) {
	if code == "" {
		return false, nil
	}
	saved, err := setting(claimField)
	if err != nil || saved == "" || saved != code {
		return false, err
	}
	if _, err := do("HDEL", settingsKey, claimField); err != nil {
		return false, err
	}
	return true, AddAdmin(account)
}
