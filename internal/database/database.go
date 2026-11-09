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

// maxConns bounds the pool when the URL sets no pool_max_conns. pgx would
// allow one connection for each CPU, 32 on a large NAS, more than a light
// PostgreSQL's max_connections of 20 holds. Polyfin's load fits in fewer:
// about 7 at once on a busy server. With the connection that listens to
// the changes of addons, and pg_dump's during a backup, Polyfin uses 12
// at most, which leaves room for an administrator's session and the
// connections PostgreSQL keeps for superusers.
const maxConns = 10

// Connect opens a connection pool and waits until PostgreSQL accepts it.
//
// A database container often starts after Polyfin (Unraid starts containers
// independently), so unreachable servers are retried with backoff until ctx
// ends. Rejected credentials and missing databases are configuration errors
// that waiting cannot fix: they are returned immediately.
func Connect(ctx context.Context, url string, logger *slog.Logger) (*pgxpool.Pool, error) {
	config, err := poolConfig(url)
	if err != nil {
		return nil, err
	}
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

// poolConfig reads the database URL into the pool's configuration, maxConns
// connections at most unless the URL's pool_max_conns sets another bound.
func poolConfig(url string) (*pgxpool.Config, error) {
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		// pgconn redacts the password from parse errors.
		return nil, fmt.Errorf("invalid database URL: %w", err)
	}
	config.ConnConfig.RuntimeParams["application_name"] = "polyfin"
	// pgxpool takes pool_max_conns out of the settings it parses, and
	// gives the same bound whether the URL set it or not: the URL is
	// read again to tell.
	connConfig, err := pgconn.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("invalid database URL: %w", err)
	}
	if _, set := connConfig.RuntimeParams["pool_max_conns"]; !set {
		config.MaxConns = maxConns
	}
	return config, nil
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

// Secret returns the server's signing secret, created by a migration. It
// signs what Polyfin hands to players, so it never leaves the server.
func Secret(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) ([]byte, error) {
	var secret []byte
	if err := db.QueryRow(ctx, "SELECT secret FROM server_identity").Scan(&secret); err != nil {
		return nil, fmt.Errorf("read server secret: %w", err)
	}
	return secret, nil
}

// PlexClient returns the client identifier Polyfin names itself by to
// plex.tv and to Plex Media Servers, created by a migration, so that every
// sign-in with Plex reuses one entry among the account's devices.
func PlexClient(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (string, error) {
	var id string
	if err := db.QueryRow(ctx, "SELECT plex_client::text FROM server_identity").Scan(&id); err != nil {
		return "", fmt.Errorf("read Plex client identifier: %w", err)
	}
	return id, nil
}

// Size returns the bytes the database takes on disk.
func Size(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (int64, error) {
	var size int64
	if err := db.QueryRow(ctx, "SELECT pg_database_size(current_database())").Scan(&size); err != nil {
		return 0, fmt.Errorf("read database size: %w", err)
	}
	return size, nil
}
