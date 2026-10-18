package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/moodiness/polyfin/internal/backup"
)

// pendingRestore is a backup Polyfin stopped to restore: plan, nil when
// it stopped for another reason, restored by backups, while the address
// Polyfin listened on answers 503.
type pendingRestore struct {
	plan    *backup.Plan
	backups *backup.Service
	listen  string
	logger  *slog.Logger
}

// restoreBackup restores r's backup (see backup.Service.Restore), every
// part of Polyfin being stopped. Meanwhile, apps get 503 at Polyfin's
// address, and /healthz answers that the process serves. A failure is
// logged, and recorded in the database, which is left as it was.
func restoreBackup(ctx context.Context, r pendingRestore) {
	server := &http.Server{Handler: restoringHandler(), ReadHeaderTimeout: readHeaderTimeout,
		ErrorLog: slog.NewLogLogger(r.logger.Handler(), slog.LevelWarn)}
	if listener, err := net.Listen("tcp", r.listen); err != nil {
		r.logger.Warn("Apps cannot be told that a backup is being restored", "error", err)
	} else {
		go func() { _ = server.Serve(listener) }()
		defer server.Close()
	}
	_ = r.backups.Restore(ctx, *r.plan)
}

// restoringHandler answers while a backup is restored: 200 to /healthz,
// as the process serves, and 503 to everything else, which apps and the
// admin app try again later.
func restoringHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = io.WriteString(w, "ok\n")
			return
		}
		w.Header().Set("Retry-After", "10")
		if strings.HasPrefix(r.URL.Path, "/admin/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":"restoring"}`)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "Polyfin is restoring a backup of its database: try again in a moment.\n")
	})
}
