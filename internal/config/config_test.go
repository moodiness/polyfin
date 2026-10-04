package config

import (
	"log/slog"
	"slices"
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
		"POLYFIN_HWACCEL":      " VAAPI ",
		"POLYFIN_VAAPI_DEVICE": "/dev/dri/renderD129",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "[::1]:9000" || cfg.LogLevel != slog.LevelDebug || cfg.Acceleration != "vaapi" || cfg.VAAPIDevice != "/dev/dri/renderD129" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	// Without a choice, the first GPU that works converts video.
	if cfg, err := Load(env(map[string]string{"POLYFIN_DATABASE_URL": "postgresql://polyfin@db/polyfin"})); err != nil || cfg.Acceleration != "auto" {
		t.Errorf("default acceleration %q, %v", cfg.Acceleration, err)
	}
}

func TestFontsComeFromTheSystemFolderUnlessSet(t *testing.T) {
	for value, want := range map[string]string{"": "/usr/share/fonts", " /fonts ": "/fonts"} {
		cfg, err := Load(env(map[string]string{"POLYFIN_DATABASE_URL": "postgresql://polyfin@db/polyfin", "POLYFIN_FONTS_DIR": value}))
		if err != nil || cfg.FontsDir != want {
			t.Errorf("POLYFIN_FONTS_DIR %q: %q, %v", value, cfg.FontsDir, err)
		}
	}
}

func TestLoadReportsEveryInvalidSetting(t *testing.T) {
	_, err := Load(env(map[string]string{
		"POLYFIN_LISTEN":     "8096",
		"POLYFIN_LOG_LEVEL":  "verbose",
		"POLYFIN_CACHE_SIZE": "lots",
		"POLYFIN_HWACCEL":    "qsv",
		"POLYFIN_SEGMENTS":   "theintrodb,other",
	}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"POLYFIN_DATABASE_URL", "POLYFIN_LISTEN", "POLYFIN_LOG_LEVEL", "POLYFIN_CACHE_SIZE", "POLYFIN_HWACCEL", "POLYFIN_SEGMENTS"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestSegmentDatabasesKeepTheOrderOfPreference(t *testing.T) {
	for text, want := range map[string][]string{
		"":                       {"theintrodb", "introdb"},
		" IntroDB , theintrodb ": {"introdb", "theintrodb"},
		"introdb,introdb":        {"introdb"},
		"none":                   nil,
	} {
		cfg, err := Load(env(map[string]string{"POLYFIN_DATABASE_URL": "postgresql://polyfin@db/polyfin", "POLYFIN_SEGMENTS": text}))
		if err != nil || !slices.Equal(cfg.Segments, want) {
			t.Errorf("%q: %v, %v; want %v", text, cfg.Segments, err, want)
		}
	}
	// Turning the databases off cannot be mixed with naming one.
	if _, err := Load(env(map[string]string{"POLYFIN_DATABASE_URL": "postgresql://polyfin@db/polyfin", "POLYFIN_SEGMENTS": "none,introdb"})); err == nil {
		t.Error("none with a database was accepted")
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
