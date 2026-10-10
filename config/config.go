package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is app settings from environment variables
type Config struct {
	TelegramToken string
	// AllowedIDs are telegram user ids allowed to use the bot
	AllowedIDs []int64
	HTTPAddr   string
}

// Load reads config from environment variables
func Load() (Config, error) {
	cfg := Config{
		TelegramToken: strings.TrimSpace(os.Getenv("TELEGRAM_TOKEN")),
		HTTPAddr:      os.Getenv("HTTP_ADDR"),
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":9090"
	}
	if cfg.TelegramToken == "" {
		return cfg, errors.New("TELEGRAM_TOKEN is required")
	}
	ids, err := ParseIDs(os.Getenv("TELEGRAM_ALLOWED_IDS"))
	if err != nil {
		return cfg, err
	}
	cfg.AllowedIDs = ids
	return cfg, nil
}

// ParseIDs parses comma separated telegram ids
func ParseIDs(s string) ([]int64, error) {
	var ids []int64
	for _, field := range strings.Split(s, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		id, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("TELEGRAM_ALLOWED_IDS has invalid id %q", field)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
