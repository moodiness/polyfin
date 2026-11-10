package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/cluster"
)

// watchInterval is how often polyfin serve checks that the database it
// runs itself is still running.
const watchInterval = 10 * time.Second

// embeddedDatabase is the PostgreSQL cluster polyfin serve runs itself
// when POLYFIN_DATABASE is embedded: in the folder postgresql of the data
// folder, logging to log/postgresql.log, as Polyfin Desktop's. It is
// started before the server and stopped after it, and after a restore. A
// cluster that stops by itself meanwhile is started again.
type embeddedDatabase struct {
	cluster *cluster.Cluster
	logger  *slog.Logger
	// interval is watchInterval, shorter in tests.
	interval time.Duration

	stopWatch context.CancelFunc
	watching  sync.WaitGroup
}

// startEmbedded makes the cluster at first start, then starts it, with
// the server programs of the first folder of path, a list as PATH is,
// that has pg_ctl.
func startEmbedded(ctx context.Context, path, dataDir string, logger *slog.Logger) (*embeddedDatabase, error) {
	bin, err := serverPrograms(path)
	if err != nil {
		return nil, err
	}
	db := newEmbedded(bin, dataDir, logger)
	if err := db.start(ctx); err != nil {
		return nil, err
	}
	return db, nil
}

// newEmbedded is the cluster of dataDir, run with the programs of bin.
func newEmbedded(bin, dataDir string, logger *slog.Logger) *embeddedDatabase {
	return &embeddedDatabase{
		cluster: &cluster.Cluster{
			Bin:    bin,
			Dir:    filepath.Join(dataDir, "postgresql"),
			Log:    filepath.Join(dataDir, "log", "postgresql.log"),
			Logger: logger,
		},
		logger:   logger,
		interval: watchInterval,
	}
}

// start prepares and starts the cluster, then watches it until stop.
func (db *embeddedDatabase) start(ctx context.Context) error {
	if _, err := db.cluster.Prepare(ctx); err != nil {
		return fmt.Errorf("prepare the database: %w", err)
	}
	if err := db.cluster.Start(ctx); err != nil {
		return err
	}
	db.logger.Info("The database started", "folder", db.cluster.Dir)
	// The watch outlives a stop of serve: the restore that may follow
	// needs the database.
	watchCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	db.stopWatch = cancel
	db.watching.Add(1)
	go db.watch(watchCtx)
	return nil
}

// watch starts the cluster again whenever it is found stopped, until ctx
// ends.
func (db *embeddedDatabase) watch(ctx context.Context) {
	defer db.watching.Done()
	ticker := time.NewTicker(db.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if db.cluster.Running(ctx) || ctx.Err() != nil {
			continue
		}
		db.logger.Warn("The database stopped by itself: starting it again", "log", db.cluster.Log)
		if err := db.cluster.Start(ctx); err != nil && ctx.Err() == nil {
			db.logger.Error("The database cannot start again", "error", err)
		}
	}
}

// url is the server's database URL, with the password: never logged.
func (db *embeddedDatabase) url() string { return db.cluster.URL() }

// stop stops watching, then stops the cluster, once the server and any
// restore no longer use it.
func (db *embeddedDatabase) stop() {
	db.stopWatch()
	db.watching.Wait()
	if err := db.cluster.Stop(context.Background()); err != nil {
		db.logger.Error("The database did not stop cleanly: it recovers at its next start", "error", err)
		return
	}
	db.logger.Info("The database stopped")
}

// serverPrograms is the first folder of path that has PostgreSQL's
// pg_ctl.
func serverPrograms(path string) (string, error) {
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		name := "pg_ctl"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		info, err := os.Stat(filepath.Join(dir, name))
		if err == nil && !info.IsDir() {
			return dir, nil
		}
	}
	return "", errors.New("POLYFIN_DATABASE is embedded, but no folder of PATH has PostgreSQL's server programs (pg_ctl)")
}
