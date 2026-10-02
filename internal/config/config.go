// Package config reads Polyfin's runtime settings from the environment.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultListen is the port Jellyfin clients try first when no port is given.
const DefaultListen = ":8096"

// Config holds the settings needed to start the server.
type Config struct {
	// DatabaseURL is a PostgreSQL connection string. It contains credentials:
	// never log it.
	DatabaseURL string
	// Listen is the TCP address of the HTTP server, as host:port.
	Listen   string
	LogLevel slog.Level
	// FFprobe and FFmpeg are the executables, paths or names looked up in
	// PATH.
	FFprobe string
	FFmpeg  string
	// CacheDir keeps the blocks of the sources being played, and CacheSize
	// bounds the space they take, in bytes.
	CacheDir  string
	CacheSize int64
}

// defaultCacheSize is 10 GB: a few films, read again for seeks and
// restarts without downloading them again.
const defaultCacheSize = 10_000_000_000

// minCacheSize leaves room for the blocks of a few sources read at once.
const minCacheSize = 256 << 20

// Load reads the configuration through getenv, normally os.Getenv.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		DatabaseURL: strings.TrimSpace(getenv("POLYFIN_DATABASE_URL")),
		Listen:      strings.TrimSpace(getenv("POLYFIN_LISTEN")),
		LogLevel:    slog.LevelInfo,
		FFprobe:     strings.TrimSpace(getenv("POLYFIN_FFPROBE")),
		FFmpeg:      strings.TrimSpace(getenv("POLYFIN_FFMPEG")),
		CacheDir:    strings.TrimSpace(getenv("POLYFIN_CACHE_DIR")),
		CacheSize:   defaultCacheSize,
	}
	if cfg.FFprobe == "" {
		cfg.FFprobe = "ffprobe"
	}
	if cfg.FFmpeg == "" {
		cfg.FFmpeg = "ffmpeg"
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = filepath.Join(os.TempDir(), "polyfin")
	}
	var errs []error
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("POLYFIN_DATABASE_URL is required"))
	}
	if cfg.Listen == "" {
		cfg.Listen = DefaultListen
	} else if err := validateListen(cfg.Listen); err != nil {
		errs = append(errs, fmt.Errorf("POLYFIN_LISTEN: %w", err))
	}
	if level := strings.TrimSpace(getenv("POLYFIN_LOG_LEVEL")); level != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(level)); err != nil {
			errs = append(errs, fmt.Errorf("POLYFIN_LOG_LEVEL: %q is not one of debug, info, warn, error", level))
		}
	}
	if raw := strings.TrimSpace(getenv("POLYFIN_CACHE_SIZE")); raw != "" {
		size, err := parseSize(raw)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("POLYFIN_CACHE_SIZE: %w", err))
		case size < minCacheSize:
			errs = append(errs, fmt.Errorf("POLYFIN_CACHE_SIZE: %q is below the minimum of 256 MiB", raw))
		default:
			cfg.CacheSize = size
		}
	}
	return cfg, errors.Join(errs...)
}

// units are the size suffixes POLYFIN_CACHE_SIZE accepts, decimal and
// binary.
var units = map[string]float64{
	"": 1, "b": 1,
	"kb": 1e3, "mb": 1e6, "gb": 1e9, "tb": 1e12,
	"k": 1 << 10, "m": 1 << 20, "g": 1 << 30, "t": 1 << 40,
	"kib": 1 << 10, "mib": 1 << 20, "gib": 1 << 30, "tib": 1 << 40,
}

// parseSize reads a size such as "10GB", "500 MiB" or "20000000000".
func parseSize(text string) (int64, error) {
	number := strings.TrimRightFunc(strings.ToLower(text), func(r rune) bool { return r >= 'a' && r <= 'z' })
	unit := strings.ToLower(text)[len(number):]
	factor, known := units[unit]
	value, err := strconv.ParseFloat(strings.TrimSpace(number), 64)
	if !known || err != nil || value < 0 || value*factor > math.MaxInt64 {
		return 0, fmt.Errorf("%q is not a size such as 10GB", text)
	}
	return int64(value * factor), nil
}

func validateListen(address string) error {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%q is not a host:port address", address)
	}
	number, err := strconv.ParseUint(port, 10, 16)
	if err != nil || number == 0 {
		return fmt.Errorf("%q has no valid port (1-65535)", address)
	}
	return nil
}
