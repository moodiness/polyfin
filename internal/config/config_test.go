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
		"POLYFIN_LISTEN":     "8096",
		"POLYFIN_LOG_LEVEL":  "verbose",
		"POLYFIN_CACHE_SIZE": "lots",
	}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"POLYFIN_DATABASE_URL", "POLYFIN_LISTEN", "POLYFIN_LOG_LEVEL", "POLYFIN_CACHE_SIZE"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestCacheSizesReadAsPeopleWriteThem(t *testing.T) {
	for text, want := range map[string]int64{
		"":            defaultCacheSize,
		"20GB":        20_000_000_000,
		"500 mb":      500_000_000,
		"1.5TB":       1_500_000_000_000,
		"8GiB":        8 << 30,
		"10g":         10 << 30,
		"21474836480": 21474836480,
	} {
		cfg, err := Load(env(map[string]string{"POLYFIN_DATABASE_URL": "postgresql://polyfin@db/polyfin", "POLYFIN_CACHE_SIZE": text}))
		if err != nil || cfg.CacheSize != want {
			t.Errorf("%q: %d %v, want %d", text, cfg.CacheSize, err, want)
		}
	}
	for _, text := range []string{"10XB", "-1GB", "100MB", "GB"} {
		if _, err := Load(env(map[string]string{"POLYFIN_DATABASE_URL": "postgresql://polyfin@db/polyfin", "POLYFIN_CACHE_SIZE": text})); err == nil {
			t.Errorf("%q was accepted", text)
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
