package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is app settings from environment variables, all optional
type Config struct {
	// TelegramToken overrides token saved by setup page
	TelegramToken string
	// AdminIDs are telegram user ids made admins on start
	AdminIDs []int64
	HTTPAddr string
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
	// TELEGRAM_ALLOWED_IDS is the name used by older versions
	ids := os.Getenv("TELEGRAM_ADMIN_IDS")
	if ids == "" {
		ids = os.Getenv("TELEGRAM_ALLOWED_IDS")
	}
	admins, err := ParseIDs(ids)
	if err != nil {
		return cfg, err
	}
	cfg.AdminIDs = admins
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
			return nil, fmt.Errorf("invalid telegram id %q", field)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
