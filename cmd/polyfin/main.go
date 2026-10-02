// Command polyfin runs the Polyfin server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/admin"
	"github.com/moodiness/polyfin/internal/config"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/quickconnect"
	"github.com/moodiness/polyfin/internal/server"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/throttle"
	webui "github.com/moodiness/polyfin/web"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

const (
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 15 * time.Second
	// Failed password and setup code attempts allowed per client address.
	signInFailures = 10
	signInWindow   = 15 * time.Minute
)

const usage = `Usage: polyfin [command]

Commands:
  serve    Start the server (default)
  version  Print the version
  help     Show this help

Environment:
  POLYFIN_DATABASE_URL  PostgreSQL URL (required)
  POLYFIN_LISTEN        HTTP address (default :8096)
  POLYFIN_LOG_LEVEL     debug, info, warn or error (default info)
`

func main() {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	switch command {
	case "serve":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		err := serve(ctx)
		// A stop requested while waiting for PostgreSQL is not a failure.
		interrupted := ctx.Err() != nil && errors.Is(err, context.Canceled)
		stop()
		if err != nil && !interrupted {
			fmt.Fprintln(os.Stderr, "polyfin:", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println(version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "polyfin: unknown command %q\n\n%s", command, usage)
		os.Exit(2)
	}
}

func serve(ctx context.Context) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	adminApp, err := webui.Assets()
	if err != nil {
		return err
	}

	pool, err := database.Connect(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("migrate the database: %w", err)
	}
	serverID, err := database.ServerID(ctx, pool)
	if err != nil {
		return err
	}
	store, err := accounts.Open(ctx, pool)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	var setupCode string
	if required, err := store.SetupRequired(ctx); err != nil {
		return err
	} else if required {
		setupCode = admin.NewSetupCode()
		logger.Warn("No administrator yet: open /admin/ and create one with this setup code", "setup_code", setupCode)
	}

	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	quickConnect := quickconnect.New()
	signIns := throttle.New(signInFailures, signInWindow)
	httpServer := &http.Server{
		Handler: server.New(server.Options{
			Database: pool,
			Admin:    adminApp,
			AdminAPI: admin.New(admin.Options{
				Version:      version,
				ServerID:     serverID,
				Database:     pool,
				Accounts:     store,
				Addons:       addons.New(pool, stremio.NewClient(version)),
				QuickConnect: quickConnect,
				SignIns:      signIns,
				SetupCode:    setupCode,
				Logger:       logger,
			}),
			Jellyfin: jellyfin.New(jellyfin.Options{
				ServerID:      serverID,
				Accounts:      store,
				QuickConnect:  quickConnect,
				SignIns:       signIns,
				WebSocketPort: listener.Addr().(*net.TCPAddr).Port,
				Logger:        logger,
			}),
			Logger: logger,
		}),
		ReadHeaderTimeout: readHeaderTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	served := make(chan error, 1)
	go func() { served <- httpServer.Serve(listener) }()
	logger.Info("Polyfin started", "version", version, "address", listener.Addr().String(), "server_id", serverID)

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	logger.Info("Polyfin is stopping")
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Warn("Closing connections still open after the shutdown timeout", "error", err)
		return httpServer.Close()
	}
	return nil
}
