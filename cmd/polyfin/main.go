// Command polyfin runs the Polyfin server.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/admin"
	"github.com/moodiness/polyfin/internal/collections"
	"github.com/moodiness/polyfin/internal/config"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/logs"
	"github.com/moodiness/polyfin/internal/mediasegments"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/playlists"
	"github.com/moodiness/polyfin/internal/preferences"
	"github.com/moodiness/polyfin/internal/quickconnect"
	"github.com/moodiness/polyfin/internal/recordings"
	"github.com/moodiness/polyfin/internal/server"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tasks"
	"github.com/moodiness/polyfin/internal/throttle"
	"github.com/moodiness/polyfin/internal/thumbnails"
	"github.com/moodiness/polyfin/internal/userdata"
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
  POLYFIN_FFPROBE       ffprobe executable (default ffprobe, from PATH)
  POLYFIN_FFMPEG        FFmpeg executable (default ffmpeg, from PATH)
  POLYFIN_CACHE_DIR     where sources being played and their remuxes are kept (default: a polyfin directory in the system's temporary directory)
  POLYFIN_CACHE_SIZE    space the source cache may use, such as 20GB (default 10GB)
  POLYFIN_HWACCEL       GPU video is converted on: auto, nvenc, vaapi or none (default auto)
  POLYFIN_VAAPI_DEVICE  render node VAAPI opens (default: each in turn)
  POLYFIN_SEGMENTS      databases skip buttons come from, preferred first: theintrodb, introdb or none (default theintrodb,introdb)
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
	started := time.Now()
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	// The level follows the settings' detailed log once they are loaded.
	// The recent lines are also kept, redacted, for administrators to read
	// from Jellyfin apps.
	level := new(slog.LevelVar)
	level.Set(cfg.LogLevel)
	recent := logs.NewRing(logs.Capacity)
	logger := slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, recent), &slog.HandlerOptions{Level: level}))
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
	store.FollowLogLevel(level, cfg.LogLevel)
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
	addonClient := stremio.NewClient(version)
	addonStore := addons.New(pool, addonClient)
	secret, err := database.Secret(ctx, pool)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath(cfg.FFprobe); err != nil {
		logger.Warn("ffprobe was not found: nothing can be played until it is installed", "ffprobe", cfg.FFprobe)
	}
	if _, err := exec.LookPath(cfg.FFmpeg); err != nil {
		logger.Warn("FFmpeg was not found: apps that cannot play a file as it is cannot play it", "ffmpeg", cfg.FFmpeg)
	}
	sources, err := source.New(filepath.Join(cfg.CacheDir, "sources"), cfg.CacheSize, addonClient, logger)
	if err != nil {
		return fmt.Errorf("prepare the source cache: %w", err)
	}
	defer sources.Close()
	segments, err := hls.NewManager(cfg.FFmpeg, filepath.Join(cfg.CacheDir, "segments"), logger)
	if err != nil {
		return fmt.Errorf("prepare the segment directory: %w", err)
	}
	defer segments.Close()
	switch hw, ok := segments.DetectHardware(cfg.Acceleration, cfg.VAAPIDevice); {
	case ok:
		logger.Info("Video is converted on the GPU", "method", hw.Method, "device", hw.Device, "encoders", hw.Encoders, "tone_mapping", hw.ToneMapping)
	case cfg.Acceleration == "auto":
		logger.Info("Video is converted in software: no GPU encodes")
	case cfg.Acceleration != "none":
		logger.Warn("Video is converted in software: the GPU asked for does not encode", "hwaccel", cfg.Acceleration)
	}
	lib := library.New(pool, addonStore, addonClient, logger, store.Settings)
	channels := iptv.New(pool, addonStore, addonClient, logger, store.Settings)
	lib.UseIPTV(channels)
	if err := lib.SpoolGuidesIn(filepath.Join(cfg.CacheDir, "guides")); err != nil {
		return fmt.Errorf("prepare the guide directory: %w", err)
	}
	activityLog := activity.New(pool, store.Settings, logger)
	registry := tasks.New(logger)
	registerTasks(registry, store, activityLog, lib, channels, logger)
	player, err := playback.New(pool, addonClient, cfg.FFprobe, playback.NewSigner(secret), sources, segments, lib.Renew, logger, store.Settings)
	if err != nil {
		return err
	}
	defer player.Close()
	images := thumbnails.New(thumbnails.Options{
		DB:          pool,
		FFmpeg:      cfg.FFmpeg,
		Hardware:    segments.Hardware(),
		ToneMapping: segments.HasFilters("zscale", "tonemap"),
		Settings:    store.Settings,
		Open:        func(v library.Version) thumbnails.Source { return player.OpenSource(v).Once() },
		Analyzed:    player.Analyzed,
		Logger:      logger,
	})
	defer images.Close()
	recorder := recordings.New(recordings.Config{DB: pool, Dir: cfg.RecordingsDir, Guide: lib, Recorder: player, Users: store, Logger: logger})
	if recorder.Available() {
		logger.Info("Live TV recording is on", "folder", cfg.RecordingsDir)
		registerRecordingTasks(registry, recorder)
	}
	webClient := server.WebClientFiles(cfg.WebDir)
	switch {
	case webClient != nil:
		logger.Info("The web client is served at /web/", "folder", cfg.WebDir)
	case cfg.WebDir != config.DefaultWebDir:
		logger.Warn("No web client: POLYFIN_WEB_DIR holds no index.html", "folder", cfg.WebDir)
	}
	jellyfinAPI := jellyfin.New(jellyfin.Options{
		ServerID:      serverID,
		Accounts:      store,
		QuickConnect:  quickConnect,
		SignIns:       signIns,
		WebSocketPort: listener.Addr().(*net.TCPAddr).Port,
		Library:       lib,
		Stremio:       addonClient,
		Playback:      player,
		Preferences:   preferences.New(pool),
		UserData:      userdata.New(pool),
		Segments:      mediasegments.New(pool, mediasegments.Sources(cfg.Segments), version, logger),
		Playlists:     playlists.New(pool),
		Collections:   collections.New(pool),
		Thumbnails:    images,
		Logger:        logger,
		Activity:      activityLog,
		Tasks:         registry,
		Logs:          recent,
		CacheDir:      cfg.CacheDir,
		RecordingsDir: cfg.RecordingsDir,
		FontsDir:      cfg.FontsDir,
		Recordings:    recorder,
	})
	httpServer := &http.Server{
		Handler: server.New(server.Options{
			Database: pool,
			Admin:    adminApp,
			AdminAPI: admin.New(admin.Options{
				Version:       version,
				ServerID:      serverID,
				Database:      pool,
				Accounts:      store,
				Addons:        addonStore,
				QuickConnect:  quickConnect,
				SignIns:       signIns,
				SetupCode:     setupCode,
				Logger:        logger,
				Guides:        lib,
				IPTV:          channels,
				Activity:      activityLog,
				RecordingsDir: cfg.RecordingsDir,
				WebClient:     webClient != nil,
				Sessions:      jellyfinAPI,
				Tasks:         registry,
				Logs:          recent,
				Recordings:    recorder,
				Library:       lib,
				Health: admin.HealthSources{
					Addons:       addonClient,
					Cache:        sources,
					CacheDir:     cfg.CacheDir,
					Encoder:      segments,
					Thumbnails:   images,
					DatabaseSize: func(ctx context.Context) (int64, error) { return database.Size(ctx, pool) },
					Started:      started,
				},
				Variables: config.Variables(os.Environ(), cfg),
			}),
			Jellyfin:      jellyfinAPI,
			Web:           webClient,
			SetupRequired: store.SetupRequired,
			Logger:        logger,
		}),
		ReadHeaderTimeout: readHeaderTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	// Jellyfin's handler, created above, closes the sockets of the devices
	// the device sweep signs out. The guides due are fetched at start too.
	registry.Start(ctx)
	// Recordings in progress when ctx ends are finished, as partial, when
	// Polyfin starts again.
	go recorder.Run(ctx)
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

// registerTasks registers Polyfin's own periodic jobs, which Jellyfin apps
// show as scheduled tasks.
func registerTasks(registry *tasks.Registry, store *accounts.Store, activityLog *activity.Store, lib *library.Service, channels *iptv.Service,
	logger *slog.Logger) {
	registry.Register(tasks.Task{
		Key:      "SignOutInactiveDevices",
		Category: tasks.CategoryMaintenance,
		Text: map[string]tasks.Text{
			"en": {Name: "Sign out unused devices", Description: "Signs out the Jellyfin apps left unused for the number of days the settings choose."},
			"fr": {Name: "Déconnecter les appareils inutilisés", Description: "Déconnecte les applis Jellyfin restées inutilisées pendant le nombre de jours choisi dans les paramètres."},
		},
		Interval: accounts.DeviceSweepInterval,
		AtStart:  true,
		Run:      func(ctx context.Context) error { return store.SweepInactiveDevices(ctx, logger) },
	})
	registry.Register(tasks.Task{
		Key:      "CleanActivityLog",
		Category: tasks.CategoryMaintenance,
		Text: map[string]tasks.Text{
			"en": {Name: "Clean the activity log", Description: "Deletes the activity log entries older than 30 days."},
			"fr": {Name: "Nettoyer le journal d’activité", Description: "Supprime les entrées du journal d’activité de plus de 30 jours."},
		},
		Interval: activity.SweepInterval,
		AtStart:  true,
		Run: func(ctx context.Context) error {
			deleted, err := activityLog.Sweep(ctx)
			if deleted > 0 {
				logger.Info("Deleted old activity log entries", "entries", deleted)
			}
			return err
		},
	})
	registry.Register(tasks.Task{
		Key:      "RefreshLiveTvGuides",
		Category: tasks.CategoryLiveTV,
		Text: map[string]tasks.Text{
			"en": {Name: "Refresh Live TV guides", Description: "Fetches the IPTV channel lists, then the XMLTV guides of live TV catalogs, not fetched for the hours set under Settings › Live TV; run by hand, fetches every list and guide now."},
			"fr": {Name: "Actualiser les guides TV", Description: "Télécharge les listes de chaînes IPTV, puis les guides XMLTV des catalogues de TV en direct, qui ne l’ont pas été depuis le nombre d’heures choisi dans Paramètres › TV en direct ; lancé à la main, télécharge toutes les listes et tous les guides tout de suite."},
		},
		Interval: library.GuideCheck,
		AtStart:  true,
		Run: func(ctx context.Context) error {
			// The guides are matched with the channels of the lists.
			if err := channels.RefreshDue(ctx, tasks.ByHand(ctx)); err != nil {
				return err
			}
			return lib.RefreshGuides(ctx, tasks.ByHand(ctx))
		},
	})
	// Asking for ratings costs requests to addons: this runs only when an
	// administrator starts it.
	registry.Register(tasks.Task{
		Key:      "RefreshRatings",
		Category: tasks.CategoryLibrary,
		Text: map[string]tasks.Text{
			"en": {Name: "Refresh ratings", Description: "Asks the server's addons again for the ratings and genres of titles last asked long ago, which parental control and blocked genres use."},
			"fr": {Name: "Actualiser les classifications", Description: "Redemande aux addons du serveur les classifications et les genres des titres demandés il y a longtemps, qui servent au contrôle parental et aux genres bloqués."},
		},
		Run: func(ctx context.Context) error {
			asked, err := lib.RefreshRatings(ctx)
			logger.Info("Ratings refreshed", "titles", asked)
			return err
		},
	})
}

// registerRecordingTasks registers the periodic jobs of Live TV recording,
// when it is on: the scheduler itself runs on its own (see
// recordings.Service.Run).
func registerRecordingTasks(registry *tasks.Registry, recorder *recordings.Service) {
	registry.Register(tasks.Task{
		Key:      "DeleteOldRecordings",
		Category: tasks.CategoryLiveTV,
		Text: map[string]tasks.Text{
			"en": {Name: "Delete old recordings", Description: "Deletes the Live TV recordings older than the number of days the settings keep them."},
			"fr": {Name: "Supprimer les anciens enregistrements", Description: "Supprime les enregistrements de TV en direct plus anciens que le nombre de jours choisi dans les paramètres."},
		},
		Interval: recordings.SweepInterval,
		AtStart:  true,
		Run:      recorder.SweepRecordings,
	})
	registry.Register(tasks.Task{
		Key:      "ScheduleSeriesRecordings",
		Category: tasks.CategoryLiveTV,
		Text: map[string]tasks.Text{
			"en": {Name: "Schedule series recordings", Description: "Looks in the guide for the new programmes of each series recording and schedules them."},
			"fr": {Name: "Programmer les enregistrements de séries", Description: "Cherche dans le guide les nouvelles émissions de chaque série enregistrée et les programme."},
		},
		Interval: recordings.SeriesRefreshInterval,
		AtStart:  true,
		Run:      recorder.RefreshSeriesTimers,
	})
}
