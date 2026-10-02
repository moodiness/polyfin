package config

import (
	"log/slog"
	"strings"
	"testing"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadAcceptsExplicitSettings(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"POLYFIN_DATABASE_URL": "postgresql://polyfin@db/polyfin",
		"POLYFIN_LISTEN":       "[::1]:9000",
		"POLYFIN_LOG_LEVEL":    "DEBUG",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "[::1]:9000" || cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadReportsEveryInvalidSetting(t *testing.T) {
	_, err := Load(env(map[string]string{
		"POLYFIN_LISTEN":    "8096",
		"POLYFIN_LOG_LEVEL": "verbose",
	}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"POLYFIN_DATABASE_URL", "POLYFIN_LISTEN", "POLYFIN_LOG_LEVEL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestLoadRejectsUnusablePorts(t *testing.T) {
	for _, listen := range []string{":0", ":65536", ":http", "host:"} {
		_, err := Load(env(map[string]string{
			"POLYFIN_DATABASE_URL": "postgresql://polyfin@db/polyfin",
			"POLYFIN_LISTEN":       listen,
		}))
		if err == nil {
			t.Errorf("POLYFIN_LISTEN=%q was accepted", listen)
		}
	}
}
