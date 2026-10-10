package config

import (
	"reflect"
	"testing"
)

func TestParseIDs(t *testing.T) {
	ids, err := ParseIDs(" 123, 456 ,,")
	if err != nil || !reflect.DeepEqual(ids, []int64{123, 456}) {
		t.Errorf("ParseIDs() = %v, %v", ids, err)
	}
	if _, err := ParseIDs("123,abc"); err == nil {
		t.Error("invalid id should fail")
	}
}

func TestLoad(t *testing.T) {
	t.Setenv("TELEGRAM_TOKEN", "")
	if _, err := Load(); err == nil {
		t.Error("token is required")
	}
	t.Setenv("TELEGRAM_TOKEN", " token ")
	t.Setenv("TELEGRAM_ALLOWED_IDS", "1")
	t.Setenv("HTTP_ADDR", "")
	cfg, err := Load()
	if err != nil || cfg.TelegramToken != "token" || cfg.HTTPAddr != ":9090" || !reflect.DeepEqual(cfg.AllowedIDs, []int64{1}) {
		t.Errorf("Load() = %+v, %v", cfg, err)
	}
}
