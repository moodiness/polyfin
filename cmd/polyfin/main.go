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
	"github.com/moodiness/polyfin/internal/backup"
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
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/server"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/streamyfin"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tasks"
	"github.com/moodiness/polyfin/internal/throttle"
	"github.com/moodiness/polyfin/internal/thumbnails"
	"github.com/moodiness/polyfin/internal/trackers"
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
  POLYFIN_HWACCEL       GPU video is converted on, copied into the settings at the first start only: auto, nvenc, vaapi or none (default auto)
  POLYFIN_VAAPI_DEVICE  render node VAAPI opens (default: each in turn)
  POLYFIN_SEGMENTS      databases skip buttons come from, preferred first, copied into the settings at the first start only: theintrodb, introdb, publicmetadb or none (default theintrodb,introdb,publicmetadb)
  POLYFIN_SECRET_KEY    key the stored keys and tokens are encrypted with: 32 bytes in base64, from openssl rand -base64 32 (default: none, stored unencrypted)
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
	box, err := secrets.New(cfg.SecretKey)
	if err != nil {
		return fmt.Errorf("POLYFIN_SECRET_KEY: %w", err)
	}
	if err := box.Prepare(ctx, pool, logger); err != nil {
		return fmt.Errorf("seal the stored secrets: %w", err)
	}
	store, err := accounts.Open(ctx, pool, accounts.Sealing(box))
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	// POLYFIN_HWACCEL and POLYFIN_SEGMENTS are copied into the settings
	// once; the settings decide from then on.
	if err := store.AdoptEnvironment(ctx, cfg.Acceleration, cfg.Segments); err != nil {
		return fmt.Errorf("copy POLYFIN_HWACCEL and POLYFIN_SEGMENTS into the settings: %w", err)
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
	// Addons and libraries are answered from memory while their changes
	// are followed.
	go addonStore.Watch(ctx, logger)
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
	segments.SelectHardware(store.Settings().HardwareAcceleration, cfg.VAAPIDevice)
	lib := library.New(pool, addonStore, addonClient, logger, store.Settings)
	channels := iptv.New(pool, addonStore, addonClient, logger, store.Settings)
	lib.UseIPTV(channels)
	channels.OnChange(lib.LineupChanged)
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
	// Channel streams of IPTV sources share their account's connections,
	// and how they answer orders them.
	player.LiveSources(channels.Connections, func(ctx context.Context, v library.Version, failure string) {
		if err := channels.ReportStream(ctx, v.Origin.Addon, v.Origin.ID, v.URL, failure); err != nil {
			logger.Warn("A channel's stream health could not be saved", "error", err)
		}
	})
	images := thumbnails.New(thumbnails.Options{
		DB:          pool,
		FFmpeg:      cfg.FFmpeg,
		Hardware:    player.DecodingHardware,
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
	backups := backup.New(backup.Config{Dir: cfg.BackupDir, DatabaseURL: cfg.DatabaseURL, DB: pool, Settings: store.Settings, Logger: logger})
	if backups.Available() {
		logger.Info("Database backups are on", "folder", cfg.BackupDir)
		if _, err := exec.LookPath(backups.PgDump()); err != nil {
			logger.Warn("pg_dump was not found: database backups fail until it is installed", "pg_dump", backups.PgDump())
		}
		registerBackupTask(registry, backups)
	}
	webClient := server.WebClientFiles(cfg.WebDir)
	switch {
	case webClient != nil:
		logger.Info("The web client is served at /web/", "folder", cfg.WebDir)
	case cfg.WebDir != config.DefaultWebDir:
		logger.Warn("No web client: POLYFIN_WEB_DIR holds no index.html", "folder", cfg.WebDir)
	}
	skipSegments := mediasegments.New(pool, mediasegments.Sources(accounts.SegmentSources), version, logger, store.Settings)
	userData := userdata.New(pool)
	// Imported watch histories find their titles in the library and add to
	// the users' data.
	tracking := trackers.New(trackers.Options{DB: pool, Settings: store.Settings, Secrets: box, Version: version, Logger: logger,
		Titles: lib, UserData: userData})
	defer tracking.Close()
	homeRows := streamyfin.New(pool)
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
		UserData:      userData,
		Segments:      skipSegments,
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
		Trackers:      tracking,
		Streamyfin:    homeRows,
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
				Segments:      skipSegments,
				Activity:      activityLog,
				RecordingsDir: cfg.RecordingsDir,
				VAAPIDevice:   cfg.VAAPIDevice,
				WebClient:     webClient != nil,
				Sessions:      jellyfinAPI,
				Tasks:         registry,
				Logs:          recent,
				Recordings:    recorder,
				Library:       lib,
				LibraryImages: lib,
				Health: admin.HealthSources{
					Addons:       addonClient,
					Cache:        sources,
					CacheDir:     cfg.CacheDir,
					Encoder:      segments,
					Thumbnails:   images,
					DatabaseSize: func(ctx context.Context) (int64, error) { return database.Size(ctx, pool) },
					Started:      started,
					SecretKey:    box.Enabled(),
					Secrets:      func(ctx context.Context) (secrets.Report, error) { return box.Inspect(ctx, pool) },
				},
				Variables: config.Variables(os.Environ(), cfg),
				Trackers:  tracking,
				Backups:   backups,
				// Streamyfin's rows show the server's libraries and
				// their collections.
				Streamyfin:      homeRows,
				StreamyfinItems: lib,
			}),
			Jellyfin:      jellyfinAPI,
			Web:           webClient,
			SetupRequired: store.SetupRequired,
			CustomJs:      func() string { return store.Settings().CustomJs },
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
	// What was left to send to tracking services is sent again.
	go tracking.Run(ctx)
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
	// Home screens then show the libraries' first pages at once, after a
	// restart too.
	registry.Register(tasks.Task{
		Key:      "WarmLibraries",
		Category: tasks.CategoryLibrary,
		Text: map[string]tasks.Text{
			"en": {Name: "Read the libraries' first pages", Description: "Reads the first page of each library from the addons when it is older than Refresh catalogs after (minutes), so that home screens never wait for them, and forgets the catalog pages and descriptions no one read for long."},
			"fr": {Name: "Lire la première page des médiathèques", Description: "Relit auprès des addons la première page de chaque médiathèque plus ancienne que Rafraîchir les catalogues après (minutes), pour que les écrans d’accueil ne les attendent jamais, et oublie les pages de catalogue et les descriptions que personne n’a lues depuis longtemps."},
		},
		Interval: library.WarmInterval,
		AtStart:  true,
		Run:      lib.Warm,
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

// registerBackupTask registers the daily database backup, when backups
// are on.
func registerBackupTask(registry *tasks.Registry, backups *backup.Service) {
	registry.Register(tasks.Task{
		Key:      "BackUpDatabase",
		Category: tasks.CategoryMaintenance,
		Text: map[string]tasks.Text{
			"en": {Name: "Back up the database", Description: "Copies the database into POLYFIN_BACKUP_DIR every day at the hour set under Settings › Backups, and deletes the oldest copies past the number kept."},
			"fr": {Name: "Sauvegarder la base de données", Description: "Copie la base de données dans POLYFIN_BACKUP_DIR chaque jour à l’heure choisie dans Paramètres › Sauvegardes, et supprime les plus anciennes copies au-delà du nombre conservé."},
		},
		Daily: backups.Hour,
		Run:   backups.Run,
	})
}
