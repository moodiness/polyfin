// Package config reads Polyfin's runtime settings from the environment.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
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
	// FFprobe is the ffprobe executable, a path or a name looked up in PATH.
	FFprobe string
}

// Load reads the configuration through getenv, normally os.Getenv.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		DatabaseURL: strings.TrimSpace(getenv("POLYFIN_DATABASE_URL")),
		Listen:      strings.TrimSpace(getenv("POLYFIN_LISTEN")),
		LogLevel:    slog.LevelInfo,
		FFprobe:     strings.TrimSpace(getenv("POLYFIN_FFPROBE")),
	}
	if cfg.FFprobe == "" {
		cfg.FFprobe = "ffprobe"
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
	return cfg, errors.Join(errs...)
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
