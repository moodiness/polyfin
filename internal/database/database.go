// Package database owns Polyfin's PostgreSQL connection and schema.
package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	pingTimeout   = 5 * time.Second
	firstRetry    = time.Second
	maxRetryDelay = 30 * time.Second
)

// Connect opens a connection pool and waits until PostgreSQL accepts it.
//
// A database container often starts after Polyfin (Unraid starts containers
// independently), so unreachable servers are retried with backoff until ctx
// ends. Rejected credentials and missing databases are configuration errors
// that waiting cannot fix: they are returned immediately.
func Connect(ctx context.Context, url string, logger *slog.Logger) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		// pgconn redacts the password from parse errors.
		return nil, fmt.Errorf("invalid database URL: %w", err)
	}
	config.ConnConfig.RuntimeParams["application_name"] = "polyfin"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	delay := firstRetry
	for {
		pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
		err := pool.Ping(pingCtx)
		cancel()
		if err == nil {
			return pool, nil
		}
		if ctx.Err() != nil {
			pool.Close()
			return nil, ctx.Err()
		}
		if permanent(err) {
			pool.Close()
			return nil, err
		}
		logger.Warn("PostgreSQL is not reachable yet", "error", err, "retry_in", delay)
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		delay = min(delay*2, maxRetryDelay)
	}
}

// permanent reports server answers that will not change by retrying.
func permanent(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	// Class 28: invalid authorization; 3D000: the database does not exist.
	return strings.HasPrefix(pgErr.Code, "28") || pgErr.Code == "3D000"
}

// ServerID returns the server identity created by the first migration, as
// the 32 lowercase hexadecimal characters Jellyfin clients expect.
func ServerID(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (string, error) {
	var id string
	err := db.QueryRow(ctx, "SELECT replace(id::text, '-', '') FROM server_identity").Scan(&id)
	if err != nil {
		return "", fmt.Errorf("read server identity: %w", err)
	}
	return id, nil
}
