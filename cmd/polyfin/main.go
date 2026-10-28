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
	"sync"
	"syscall"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/admin"
	"github.com/moodiness/polyfin/internal/anime"
	"github.com/moodiness/polyfin/internal/appimport"
	"github.com/moodiness/polyfin/internal/appimport/nuvio"
	stremioimport "github.com/moodiness/polyfin/internal/appimport/stremio"
	"github.com/moodiness/polyfin/internal/audiosegments"
	"github.com/moodiness/polyfin/internal/backup"
	"github.com/moodiness/polyfin/internal/collections"
	"github.com/moodiness/polyfin/internal/commercials"
	"github.com/moodiness/polyfin/internal/config"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/discovery"
	"github.com/moodiness/polyfin/internal/dlna"
	"github.com/moodiness/polyfin/internal/forwarded"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/jellyfinimport"
	"github.com/moodiness/polyfin/internal/letterboxd"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/logs"
	"github.com/moodiness/polyfin/internal/loudness"
	"github.com/moodiness/polyfin/internal/lyrics"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/mediasegments"
	"github.com/moodiness/polyfin/internal/metrics"
	"github.com/moodiness/polyfin/internal/notifications"
	"github.com/moodiness/polyfin/internal/ocr"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/playlists"
	"github.com/moodiness/polyfin/internal/plays"
	"github.com/moodiness/polyfin/internal/preferences"
	"github.com/moodiness/polyfin/internal/process"
	"github.com/moodiness/polyfin/internal/quickconnect"
	"github.com/moodiness/polyfin/internal/ratings"
	"github.com/moodiness/polyfin/internal/recordings"
	"github.com/moodiness/polyfin/internal/reminders"
	"github.com/moodiness/polyfin/internal/remuxdb"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/server"
	"github.com/moodiness/polyfin/internal/silo"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tasks"
	"github.com/moodiness/polyfin/internal/themecopy"
	"github.com/moodiness/polyfin/internal/throttle"
	"github.com/moodiness/polyfin/internal/thumbnails"
	"github.com/moodiness/polyfin/internal/tmdb"
	"github.com/moodiness/polyfin/internal/torrent"
	"github.com/moodiness/polyfin/internal/trackers"
	"github.com/moodiness/polyfin/internal/updates"
	"github.com/moodiness/polyfin/internal/updatetool"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
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
  serve        Start the server (default)
  healthcheck  Exit 0 when the server at POLYFIN_LISTEN answers /healthz within 3 seconds, 1 otherwise
  version      Print the version
  help         Show this help

Environment:
  POLYFIN_DATABASE_URL  PostgreSQL URL (required)
  POLYFIN_SECRET_KEY    key the stored keys and tokens are encrypted with: 32 bytes in base64, from openssl rand -base64 32 (default: none, stored unencrypted)
  POLYFIN_DATA_DIR      absolute path of the folder Polyfin keeps its files in: the cache, and the default recordings and backups folders (default: a polyfin folder in the system's temporary folder)
  POLYFIN_LISTEN        HTTP address (default :8096)
  POLYFIN_FFPROBE       ffprobe executable (default ffprobe, from PATH)
  POLYFIN_FFMPEG        FFmpeg executable (default ffmpeg, from PATH)
  POLYFIN_TESSERACT     Tesseract executable picture subtitles are read into text with (default tesseract, from PATH)
  POLYFIN_TESSDATA_DIR  folder of Tesseract .traineddata files for languages Tesseract's own data lacks (default: none)
  POLYFIN_FONTS_DIR     fallback fonts for subtitles and generated images (default /usr/share/fonts)
  POLYFIN_WEB_DIR       jellyfin-web, served at /web/ (default /usr/share/polyfin/jellyfin-web)

Every other option is a setting of the admin app.
`

// exitRestart is the exit code of a restart asked in the admin app,
// EX_TEMPFAIL: not 0, so that a restart policy that restarts only what
// failed starts Polyfin again.
const exitRestart = 75

// errRestart is serve's end when an administrator restarted Polyfin.
var errRestart = errors.New("restart")

func main() {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	switch command {
	case "serve":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		var restore pendingRestore
		err := serve(ctx, &restore)
		// A restart from the admin app exits once Polyfin has stopped, as
		// on a signal, with exitRestart: Docker's restart policies, on-failure
		// included, systemd's Restart=on-failure and Polyfin Desktop start a
		// process that failed again, where 0 would leave on-failure down.
		if errors.Is(err, errRestart) {
			stop()
			fmt.Fprintln(os.Stderr, "polyfin: stopped to restart, as an administrator asked: exit code", exitRestart)
			os.Exit(exitRestart)
		}
		// A stop requested while waiting for PostgreSQL is not a failure.
		interrupted := ctx.Err() != nil && errors.Is(err, context.Canceled)
		// Polyfin stopped to restore a backup: it starts again afterwards,
		// unless asked to stop meanwhile.
		restart := err == nil && restore.plan != nil && ctx.Err() == nil
		if restart {
			restoreBackup(ctx, restore)
			restart = ctx.Err() == nil
		}
		stop()
		if err != nil && !interrupted {
			fmt.Fprintln(os.Stderr, "polyfin:", err)
			os.Exit(1)
		}
		if restart {
			err := startAgain()
			fmt.Fprintln(os.Stderr, "polyfin: start again after the restore:", err)
			os.Exit(1)
		}
	case "healthcheck":
		if err := healthcheck(os.Getenv, healthcheckTimeout); err != nil {
			fmt.Fprintln(os.Stderr, "polyfin: healthcheck:", err)
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

// serve runs Polyfin until ctx ends, until an administrator restores a
// backup, or restarts Polyfin: restore then holds what Restore needs, once
// every part of Polyfin has stopped, and errRestart tells a restart.
func serve(ctx context.Context, restore *pendingRestore) error {
	started := time.Now()
	// Everything Polyfin runs in the background ends with ctx, or when it
	// stops to restore a backup.
	ctx, stopAll := context.WithCancel(ctx)
	defer stopAll()
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	// The level is info, and follows the settings' detailed log once they
	// are loaded. The recent lines are also kept, redacted, for
	// administrators to read from Jellyfin apps.
	level := new(slog.LevelVar)
	recent := logs.NewRing(logs.Capacity)
	logger := slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, recent), &slog.HandlerOptions{Level: level}))
	for _, warning := range cfg.Warnings {
		logger.Warn("A variable copied into the settings has a value that is not valid: it is not copied", "error", warning)
	}
	for _, retired := range cfg.Retired {
		logger.Info(retired.Name+" is no longer read: its value is a setting, under "+retired.Section, "variable", retired.Name)
	}
	// The programs Polyfin runs, FFmpeg above all, end with it even when it
	// is killed (see process.KillChildrenOnExit).
	if err := process.KillChildrenOnExit(); err != nil {
		logger.Warn("The programs Polyfin runs may outlive it if it is killed", "error", err)
	}
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
	// The variables that were options are copied into the settings once;
	// the settings decide from then on.
	if err := store.AdoptEnvironment(ctx, cfg.Environment); err != nil {
		return fmt.Errorf("copy the environment variables into the settings: %w", err)
	}
	store.FollowLogLevel(level, slog.LevelInfo)
	prepareFolders(store.Settings(), cfg.DataDir, logger)
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
	// The client of a request is read behind the known proxies, for every
	// route; System › Health names the proxies missing from them. Their
	// host names are resolved now, at each change of the settings, every
	// minute, and when an unknown address forwards requests.
	proxies := forwarded.New(store.KnownProxies, nil)
	proxies.Watch(ctx, store.KnownProxyNames, store.SettingsChanged)
	// Jellyfin apps that look for servers on the local network find this
	// one, while the settings turn it on; a port another server uses is
	// told in System › Health, and never stops the start.
	localDiscovery := discovery.Start(ctx, discovery.Options{
		Port:     cfg.DiscoveryPort,
		HTTP:     listener.Addr().(*net.TCPAddr),
		ServerID: serverID,
		Settings: func() discovery.Settings {
			settings := store.Settings()
			return discovery.Settings{Enabled: settings.LocalDiscovery, Name: settings.ServerName, Address: settings.LocalAddress}
		},
		Changed:     store.SettingsChanged,
		InContainer: discovery.InContainer(),
		Logger:      logger,
	})
	// HTTPS is served next to HTTP while the settings turn it on, on a
	// port of its own, from the start of the HTTP server on; a port it
	// cannot listen on, certificate files it cannot read and renewals that
	// fail are told in System › Health, and HTTP keeps working whatever
	// happens to them.
	secure := newHTTPS(cfg, store, pool, box, logger)
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
	// The segments of conversions, the files being read and the cache go
	// in the folders the settings chose, read once, now: a folder chosen
	// applies at the next start. One Polyfin cannot write into is told in
	// System › Health, and the cache in the data folder takes its place
	// meanwhile. The folders of that cache this run leaves unused are
	// removed, those chosen before left as they are.
	work := config.ChooseFolders(cfg.DataDir, store.Settings())
	for _, unwritable := range work.Unwritable {
		logger.Warn("A folder the settings choose cannot be written into: the data folder's cache is used instead until the next start",
			"folder", unwritable.Name, "path", unwritable.Folder, "error", unwritable.Err)
	}
	if err := work.ClearUnused(cfg.DataDir); err != nil {
		logger.Warn("The folders of the data folder's cache left unused could not be removed", "error", err)
	}
	cacheLimit := func() int64 { return int64(store.Settings().CacheSizeGB) * 1e9 }
	sources, err := source.New(work.Sources(), cacheLimit, addonClient, logger)
	if err != nil {
		return fmt.Errorf("prepare the source cache: %w", err)
	}
	defer sources.Close()
	segments, err := hls.NewManager(cfg.FFmpeg, work.Segments(), logger)
	if err != nil {
		return fmt.Errorf("prepare the segment directory: %w", err)
	}
	defer segments.Close()
	settings := store.Settings()
	segments.SelectHardware(settings.HardwareAcceleration, settings.VAAPIDevice)
	lib := library.New(pool, addonStore, addonClient, logger, store.Settings)
	channels := iptv.New(pool, addonStore, addonClient, logger, store.Settings)
	lib.UseIPTV(channels)
	channels.OnChange(lib.LineupChanged)
	// Find tuners asks the local networks for HDHomeRun tuners on their
	// port, or only the addresses POLYFIN_HDHOMERUN_ADDRESSES gives.
	channels.UseTunerDiscovery(cfg.HDHomeRunPort, cfg.HDHomeRunAddresses...)
	// Movies and series show the ratings MDBList gives them while the
	// settings hold the server's key, looked up in the background.
	titleRatings := ratings.New(pool, ratings.MDBList, version, logger, store.Settings)
	defer titleRatings.Close()
	lib.UseRatings(titleRatings)
	// Edit images searches TMDB's images, and people are looked up on TMDB,
	// while the settings hold the server's TMDB key.
	imageSearch := tmdb.New(tmdb.API, tmdb.Images, version, store.Settings)
	lib.UseTMDB(imageSearch)
	// Local folders match their files through the library's searches of
	// the metadata addons, and TMDB's when every addon's search fails.
	folders := localfiles.New(pool, addonStore, lib, logger, store.Settings, box)
	defer folders.Close()
	lib.UseLocal(folders)
	folders.UseTMDB(imageSearch)
	folders.UseTools(cfg.FFprobe, cfg.FFmpeg)
	// The loudness of songs without a gain is measured two at a time, those
	// of local folders and those of music addons together.
	meter := loudness.NewMeter(cfg.FFmpeg)
	folders.UseMeter(meter)
	folders.UseToneMapping(segments.HasFilters("zscale", "tonemap"))
	folders.Watch()
	if err := lib.SpoolGuidesIn(work.Guides()); err != nil {
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
	// The files of local folders are read through the loopback interface,
	// as the addons' files are; FFmpeg reads their discs and disc images
	// itself.
	folders.UseFiles(player.LocalFiles(folders.Open), player.LocalInputs(folders))
	player.LocalSources(folders.Source)
	// Torrent streams are read from their swarms while they play, over the
	// route the settings set alone, their pieces within a quarter of the
	// source cache's space, which counts them. DHT lookups start from the
	// bootstrap nodes clients use, over a route that carries UDP.
	torrents, err := torrent.New(torrent.Config{Settings: func() torrent.Settings { return playback.TorrentSettings(store.Settings()) },
		Dir: work.Torrents(), Budget: func() int64 { return cacheLimit() / 4 },
		Bootstrap: torrent.BootstrapNodes, Logger: logger})
	if err != nil {
		return fmt.Errorf("prepare the torrent directory: %w", err)
	}
	defer torrents.Close()
	sources.Share(torrents.Usage)
	player.Torrents(torrents)
	// A disc image whose UDF file system is laid out in a way Polyfin does
	// not read, as those of rewritable and write-once media are, is read
	// through FFmpeg to tell a Blu-ray disc's.
	folders.UseProbe(player.ProbesInput)
	// A folder whose address, credentials or host key changed has its
	// versions that could not be read tried again at once.
	folders.UseRetry(player.Retry)
	// Picture subtitles are read into text with Tesseract, as the settings
	// ask: those of the versions played, and of the videos scans find. The
	// files written meanwhile go in the cache folder, emptied at start, as
	// the system's temporary folder is read-only in the image's container.
	ocrDir := work.OCR()
	if err := errors.Join(os.RemoveAll(ocrDir), os.MkdirAll(ocrDir, 0o700)); err != nil {
		return fmt.Errorf("prepare the OCR directory: %w", err)
	}
	player.UseTesseract(cfg.FFmpeg, ocrDir, &ocr.Tesseract{Path: cfg.Tesseract, DataDir: cfg.TessdataDir, TempDir: ocrDir})
	folders.UseScanned(player.QueueScanned)
	if _, err := exec.LookPath(cfg.Tesseract); err != nil && store.Settings().SubtitleOCR != "off" {
		logger.Warn("Tesseract was not found: picture subtitles are not read into text", "tesseract", cfg.Tesseract)
	}
	registerLocalFoldersTask(registry, folders)
	// Channel streams of IPTV sources share their account's connections,
	// and how they answer orders them.
	player.LiveSources(channels.Connections, func(ctx context.Context, v library.Version, failure string) {
		if err := channels.ReportStream(ctx, v.Origin.Addon, v.Origin.ID, v.URL, failure); err != nil {
			logger.Warn("A channel's stream health could not be saved", "error", err)
		}
	})
	// A channel whose streams an administrator asks to try again has those
	// that failed to play tried again at once.
	channels.UseRetry(player.RetryItem)
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
	// Recording and backups follow the settings, which turn them on and
	// choose their folders. Recordings that end are told to the
	// notifications, made below, before any recording starts, and have
	// their commercial breaks looked for in the background. Finished
	// recordings are converted to HEVC as the settings ask, on the GPU
	// conversions use, a recording being played replaced once the
	// playbacks followed below and its encodings stop.
	var notifier *notifications.Service
	var breaks *commercials.Service
	var playbacks *plays.Tracker
	recorder := recordings.New(recordings.Config{DB: pool, Folder: func() string { return store.Settings().RecordingsDir(cfg.DataDir) },
		Guide: lib, Recorder: player, Users: store, Logger: logger,
		Ended: func(ctx context.Context, ended recordings.Ended) {
			notifier.RecordingEnded(ctx, ended)
			breaks.Ended(ctx, ended)
		},
		Converter: segments, Prober: media.Prober{Path: cfg.FFprobe, Timeout: time.Minute},
		InUse: func(id accounts.ID) bool { return player.FileInUse(id) || playbacks.Playing(id) }})
	if recorder.Available() {
		logger.Info("Live TV recording is on", "folder", recorder.Dir())
	}
	breaks = commercials.New(commercials.Options{DB: pool, FFmpeg: cfg.FFmpeg, FFprobe: cfg.FFprobe, Recordings: recorder,
		Settings: store.Settings, Logger: logger})
	defer breaks.Close()
	registerRecordingTasks(registry, recorder, breaks)
	// The destinations' passwords and keys are sealed as the other secrets.
	backups := backup.New(backup.Config{Folder: func() string { return store.Settings().BackupDir(cfg.DataDir) }, DatabaseURL: cfg.DatabaseURL,
		DB: pool, Secrets: box, Settings: store.Settings, Logger: logger})
	if backups.Available() {
		logger.Info("Database backups are on", "folder", backups.Dir())
		if _, err := exec.LookPath(backups.PgDump()); err != nil {
			logger.Warn("pg_dump was not found: database backups fail until it is installed", "pg_dump", backups.PgDump())
		}
		if _, err := exec.LookPath(backups.PgRestore()); err != nil {
			logger.Warn("pg_restore was not found: backups cannot be restored from the admin app until it is installed", "pg_restore",
				backups.PgRestore())
		}
		// Backups downloaded or sent to be restored, and never restored, go.
		backups.RemoveRestoreLeftovers()
	}
	registerBackupTask(registry, backups)
	webClient := server.WebClientFiles(cfg.WebDir)
	switch {
	case webClient != nil:
		logger.Info("The web client is served at /web/", "folder", cfg.WebDir)
	case cfg.WebDir != config.DefaultWebDir:
		logger.Warn("No web client: POLYFIN_WEB_DIR holds no index.html", "folder", cfg.WebDir)
	}
	skipSegments := mediasegments.New(pool, mediasegments.Sources(accounts.SegmentSources), version, logger, store.Settings)
	// Local episodes played without an intro or credits have them looked
	// for in their season's audio, a season at a time, read through the
	// loopback interface.
	audioSegments := audiosegments.New(audiosegments.Options{DB: pool, FFmpeg: cfg.FFmpeg, FFprobe: cfg.FFprobe, Files: folders,
		Settings: store.Settings, Logger: logger})
	defer audioSegments.Close()
	userData := userdata.New(pool)
	// Anime named by AniDB, Kitsu or MyAnimeList, as Simkl's history and
	// anime addons name them, are mapped to IMDb and TVDB from two lists
	// kept in the data folder.
	animeMapping := anime.New(anime.Options{Dir: filepath.Join(cfg.DataDir, "anime"), Version: version, Logger: logger})
	defer animeMapping.Close()
	// Imported watch histories find their titles in the library and add to
	// the users' data. A connection a service stops is told to the
	// notifications, made below, as recordings are.
	tracking := trackers.New(trackers.Options{DB: pool, Settings: store.Settings, Secrets: box, Version: version, Logger: logger,
		Titles: lib, UserData: userData, Anime: animeMapping,
		Stopped: func(user accounts.ID, service string) {
			notifier.ConnectionStopped(user, notifications.ConnectionTracking, service)
		}})
	defer tracking.Close()
	// Users and their watch data imported from a Jellyfin server find
	// their titles and merge the same way.
	fromJellyfin := jellyfinimport.New(jellyfinimport.Options{Settings: store.Settings, Titles: lib, UserData: userData, Version: version,
		Logger: logger})
	defer fromJellyfin.Close()
	// The watch data of accounts on apps built on addons, Nuvio's and
	// Stremio's, finds its titles and merges the same way; the accounts'
	// addons become their users' own, and their libraries go on their
	// watchlists. A series of Stremio's tells its episodes watched by
	// their places in the list its metadata addons give, which the
	// server's addons give again. The users who stay connected keep what
	// reads their accounts again, sealed; a connection its app stops is
	// told to the notifications.
	fromNuvio := nuvio.New(nuvio.Options{Anime: animeMapping, Version: version})
	fromStremio := stremioimport.New(stremioimport.Options{Anime: animeMapping, Metas: lib, Version: version})
	fromApps := appimport.New(appimport.Options{Settings: store.Settings, Titles: lib, UserData: userData, Addons: addonStore,
		Activity: activityLog, Logger: logger, DB: pool, Secrets: box,
		Sources:   map[string]appimport.Source{nuvio.App: fromNuvio, stremioimport.App: fromStremio},
		Stopped:   func(user accounts.ID, app string) { notifier.ConnectionStopped(user, notifications.ConnectionApp, app) },
		Watchlist: watchlist.New(pool)})
	defer fromApps.Close()
	// Letterboxd exports name their movies by name and year only: they are
	// searched in the server's metadata addons, then merge the same way,
	// their watchlist going on the users' watchlists.
	fromLetterboxd := letterboxd.New(letterboxd.Options{Titles: lib, UserData: userData, Watchlist: watchlist.New(pool), Logger: logger})
	defer fromLetterboxd.Close()
	// Deleting a user signs out, in the background, the sessions Polyfin
	// opened for them at apps and tracking services, and stops their
	// Letterboxd import.
	store.OnDelete(tracking.Leaving)
	store.OnDelete(fromApps.Leaving)
	store.OnDelete(fromLetterboxd.Leaving)
	// The playbacks apps report are followed: the history keeps the videos
	// that stop, and the notifications, made below, tell every change.
	history := plays.NewHistory(pool, store.Settings, logger)
	registerPlaybackHistoryTask(registry, history, logger)
	playbacks = plays.NewTracker(func(c plays.Change) {
		history.Record(context.Background(), c)
		notifier.Playback(c)
	})
	// The daily check for a new version tells administrators through the
	// notifications, made below.
	updateCheck := updates.New(updates.Options{DB: pool, Version: version, Logger: logger,
		Enabled: func() bool { return store.Settings().UpdateCheck },
		Notify:  func(release updates.Release) { notifier.NewVersion(release.Version, release.URL) }})
	// The songs of music addons are measured after their first play, read
	// as playback reads them; their measures end before playback closes.
	songLoudness := loudness.New(pool, meter, logger, store.Settings)
	defer songLoudness.Close()
	homeRows := homerows.New(pool)
	jellyfinAPI := jellyfin.New(jellyfin.Options{
		ServerID:     serverID,
		Accounts:     store,
		QuickConnect: quickConnect,
		SignIns:      signIns,
		// Blocked accounts and sign-ins on new devices are told through the
		// notifications, made below.
		AccountBlocked: func(user accounts.User, attempt accounts.DeviceInfo) { notifier.AccountBlocked(user, attempt) },
		NewDevice:      func(user accounts.User, device accounts.Device) { notifier.NewDevice(user, device) },
		WebSocketPort:  listener.Addr().(*net.TCPAddr).Port,
		Library:        lib,
		IPTV:           channels,
		Stremio:        addonClient,
		Playback:       player,
		Preferences:    preferences.New(pool),
		UserData:       userData,
		Segments:       skipSegments,
		AudioSegments:  audioSegments,
		RemuxDB:        remuxdb.New(serverID, version, logger, store.Settings),
		Lyrics:         lyrics.New(pool, lyrics.LRCLIB, version, logger, store.Settings),
		TMDB:           imageSearch,
		Loudness:       songLoudness,
		Playlists:      playlists.New(pool),
		Collections:    collections.New(pool),
		HomeRows:       homeRows,
		Thumbnails:     images,
		Logger:         logger,
		Activity:       activityLog,
		Tasks:          registry,
		Updates:        updateCheck,
		Logs:           recent,
		CacheDir:       work.Cache,
		SegmentsDir:    work.Segments(),
		FontsDir:       cfg.FontsDir,
		Recordings:     recorder,
		Commercials:    breaks,
		Backups:        backups,
		Trackers:       tracking,
		Plays:          playbacks,
		// Guests playing through share links are told through the
		// notifications, made below.
		SharePlayed: func(link accounts.ShareLink) { notifier.SharePlayed(link) },
	})
	// DLNA devices on the local network browse and play the libraries of
	// the user chosen for each, while the settings share them; a port
	// another server uses is told in System › Health, and never stops the
	// start.
	sharing := dlna.Start(ctx, dlna.Options{
		Port:     cfg.DLNAPort,
		HTTP:     listener.Addr().(*net.TCPAddr),
		ServerID: serverID,
		DB:       pool,
		Settings: store.Settings,
		Changed:  store.SettingsChanged,
		User:     store.User,
		Signer:   playback.NewSigner(secret),
		Library:  lib,
		Player:   jellyfinAPI,
		Version:  version,
		Logger:   logger,
	})
	// The copy of the files the web player's Custom texts load from
	// elsewhere, kept in the data folder's themes.
	themesDir := filepath.Join(cfg.DataDir, "themes")
	themeCopier, err := themecopy.New(ctx, themecopy.Options{DB: pool, Accounts: store, Dir: themesDir, UserAgent: "Polyfin/" + version, Logger: logger})
	if err != nil {
		return fmt.Errorf("theme copy: %w", err)
	}
	restartRequested := make(chan struct{})
	var restartOnce sync.Once
	adminOptions := admin.Options{
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
		Folders:       folders,
		Segments:      skipSegments,
		Ratings:       titleRatings,
		TMDB:          imageSearch,
		Activity:      activityLog,
		DataDir:       cfg.DataDir,
		WebClient:     webClient != nil,
		ThemeCopier:   themeCopier,
		InContainer:   discovery.InContainer(),
		Sessions:      jellyfinAPI,
		Watching:      jellyfinAPI,
		Tasks:         registry,
		Logs:          recent,
		Recordings:    recorder,
		Library:       lib,
		LibraryImages: lib,
		// Refreshing a library's descriptions, paced.
		LibraryRefreshes: lib,
		SmartLists:       smartlists.New(pool),
		SmartPreviews:    lib,
		// The rows of the web player's home page, and the collections they
		// may list as the Jellyfin API tells them.
		HomeRows:       homeRows,
		HomeRowChoices: jellyfinAPI,
		// Reset my preferences forgets what Jellyfin apps saved, with what
		// the Jellyfin API remembers of it.
		Preferences: jellyfinAPI,
		// Apply this order to every user clears the orders of libraries
		// users saved, with what the Jellyfin API remembers of them.
		LibraryOrders: jellyfinAPI,
		Sharing:       lib,
		Health: admin.HealthSources{
			Addons:           addonClient,
			SearchesLeaveOut: lib.SearchesLeaveOut,
			Cache:            sources,
			Folders:          work,
			Encoder:          segments,
			NVIDIADriver:     segments.NVIDIADriver,
			Thumbnails:       images,
			DatabaseSize:     func(ctx context.Context) (int64, error) { return database.Size(ctx, pool) },
			Started:          started,
			SecretKey:        box.Enabled(),
			Secrets:          func(ctx context.Context) (secrets.Report, error) { return box.Inspect(ctx, pool) },
			Updates:          updateCheck,
			Proxies:          proxies,
			Discovery:        localDiscovery.Status,
			DLNA:             sharing.Status,
			HTTPS:            secure.Status,
			MissingLanguages: player.MissingLanguages,
			Torrents:         torrents,
			Images:           jellyfinAPI,
		},
		Variables:      config.Variables(os.Environ(), cfg),
		Trackers:       tracking,
		Backups:        backups,
		JellyfinImport: fromJellyfin,
		AppImport:      fromApps,
		Letterboxd:     fromLetterboxd,
		Nuvio:          fromNuvio,
		Stremio:        fromStremio,
		Plays:          history,
		DLNA:           sharing,
		// A restart asked in the admin app stops Polyfin as a signal does,
		// below, for what runs it to start it again.
		Restart:    func() { restartOnce.Do(func() { close(restartRequested) }) },
		Desktop:    cfg.Desktop,
		UpdateTool: updatetool.New("Polyfin/" + version),
	}
	// Notifications tell of new episodes, ended recordings, the problems
	// System › Health finds, and once a week, a month and a year what
	// happened in it.
	notifier = notifications.New(notifications.Options{DB: pool, Accounts: store, Library: lib, UserData: userData, Secrets: box,
		Version: version, ServerID: serverID, WebClient: webClient != nil, Problems: admin.HealthProblems(adminOptions), History: history,
		LocalFiles: folders, Logger: logger})
	defer notifier.Close()
	adminOptions.Notifications = notifier
	registerSummaryTasks(registry, notifier)
	// Programme reminders tell users, on their own targets and on their
	// apps when they choose it, that a programme they chose starts soon.
	// They are sent once the notifications read their targets.
	programmeReminders := reminders.New(reminders.Config{DB: pool, Guide: lib, Users: store, Teller: notifier, Apps: jellyfinAPI,
		Reaches: func(user accounts.ID) bool { return notifier.Receives(user, notifications.ProgrammeReminder) },
		Ready:   notifier.Loaded(), Logger: logger})
	jellyfinAPI.Reminders = programmeReminders
	adminOptions.Reminders = programmeReminders
	// Playbacks under way when Polyfin stops are kept in the history, as
	// ended at their last report.
	defer playbacks.Close()
	registerMetrics(addonStore, jellyfinAPI, segments, pool, metricFolders(work, recorder.Dir, backups.Dir))
	httpServer := &http.Server{
		Handler: server.New(server.Options{
			Database:      pool,
			Admin:         adminApp,
			AdminAPI:      admin.New(adminOptions),
			Jellyfin:      jellyfinAPI,
			Share:         jellyfinAPI.Share(),
			DLNA:          sharing.Handler(),
			Web:           webClient,
			SetupRequired: store.SetupRequired,
			CustomJs:      func() string { return store.Settings().ServedJs() },
			SavedThemes:   themesDir,
			// A file of the copy kept that is missing goes to where it
			// came from.
			SavedThemeOriginal: themeCopier.Original,
			Proxies:            proxies,
			HTTPS:              secure.Handler,
			Metrics: metrics.Handler(func() (bool, string) {
				settings := store.Settings()
				return settings.Metrics, settings.MetricsToken
			}),
			Logger: logger,
			// Silo's apps, while the settings allow them, through the
			// Jellyfin API's answers.
			Silo: silo.New(silo.Options{ServerID: serverID, Version: version, Accounts: store, Jellyfin: jellyfinAPI,
				DB: pool, QuickConnect: quickConnect, Logger: logger}),
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
	go recorder.RunConversions(ctx)
	// What was left to send to tracking services is sent again.
	go tracking.Run(ctx)
	// The app connections due are imported, then as often as the settings
	// say.
	go fromApps.Run(ctx)
	go notifier.Run(ctx)
	go programmeReminders.Run(ctx)
	go updateCheck.Run(ctx)
	go playbacks.Run(ctx)
	// Playbacks whose apps stopped reporting are stopped.
	go jellyfinAPI.Run(ctx)
	served := make(chan error, 1)
	go func() { served <- httpServer.Serve(listener) }()
	secure.Start(ctx, httpServer.Handler)
	logger.Info("Polyfin started", "version", version, "address", listener.Addr().String(), "server_id", serverID)

	restarting := false
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
		logger.Info("Polyfin is stopping")
	case <-restartRequested:
		// The same stop as on a signal, but Polyfin then exits with
		// exitRestart, for what runs it to start it again.
		logger.Info("Polyfin is stopping to restart")
		restarting = true
		stopAll()
	case plan := <-backups.Requested():
		// Playbacks, conversions, recordings and tasks stop with the rest:
		// the backup is restored once nothing uses the database.
		logger.Info("Polyfin is stopping to restore a backup")
		*restore = pendingRestore{plan: &plan, backups: backups, listen: cfg.Listen, logger: logger}
		stopAll()
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	// HTTPS stops with ctx, which ended either way.
	if err := secure.Shutdown(shutdownCtx); err != nil {
		logger.Warn("HTTPS did not stop before the shutdown timeout", "error", err)
	}
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Warn("Closing connections still open after the shutdown timeout", "error", err)
		if err := httpServer.Close(); err != nil {
			return err
		}
	}
	if restarting {
		return errRestart
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
			"en":    {Name: "Sign out unused devices", Description: "Signs out the Jellyfin apps left unused for the number of days the settings choose."},
			"fr":    {Name: "Déconnecter les appareils inutilisés", Description: "Déconnecte les applis Jellyfin restées inutilisées pendant le nombre de jours choisi dans les paramètres."},
			"de":    {Name: "Ungenutzte Geräte abmelden", Description: "Meldet die Jellyfin-Apps ab, die so viele Tage ungenutzt blieben, wie in den Einstellungen festgelegt."},
			"es":    {Name: "Cerrar la sesión de los dispositivos sin uso", Description: "Cierra la sesión de las aplicaciones de Jellyfin que no se han usado durante el número de días elegido en los ajustes."},
			"it":    {Name: "Disconnetti i dispositivi inutilizzati", Description: "Disconnette le applicazioni Jellyfin rimaste inutilizzate per il numero di giorni scelto nelle impostazioni."},
			"pt-BR": {Name: "Desconectar dispositivos sem uso", Description: "Desconecta os aplicativos Jellyfin que ficaram sem uso pelo número de dias escolhido nas configurações."},
		},
		Interval: accounts.DeviceSweepInterval,
		AtStart:  true,
		Run:      func(ctx context.Context) error { return store.SweepInactiveDevices(ctx, logger) },
	})
	registry.Register(tasks.Task{
		Key:      "CleanActivityLog",
		Category: tasks.CategoryMaintenance,
		Text: map[string]tasks.Text{
			"en":    {Name: "Clean the activity log", Description: "Deletes the activity log entries older than 30 days."},
			"fr":    {Name: "Nettoyer le journal d’activité", Description: "Supprime les entrées du journal d’activité de plus de 30 jours."},
			"de":    {Name: "Aktivitätsprotokoll bereinigen", Description: "Löscht die Einträge des Aktivitätsprotokolls, die älter als 30 Tage sind."},
			"es":    {Name: "Limpiar el registro de actividad", Description: "Borra las entradas del registro de actividad de más de 30 días."},
			"it":    {Name: "Pulisci il registro attività", Description: "Elimina le voci del registro attività più vecchie di 30 giorni."},
			"pt-BR": {Name: "Limpar o registro de atividades", Description: "Exclui as entradas do registro de atividades com mais de 30 dias."},
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
			"en":    {Name: "Refresh Live TV guides", Description: "Fetches the IPTV channel lists, then the XMLTV guides of live TV catalogs, not fetched for the hours set under Settings › Live TV; run by hand, fetches every list and guide now."},
			"fr":    {Name: "Actualiser les guides TV", Description: "Télécharge les listes de chaînes IPTV, puis les guides XMLTV des catalogues de TV en direct, qui ne l’ont pas été depuis le nombre d’heures choisi dans Paramètres › TV en direct ; lancé à la main, télécharge toutes les listes et tous les guides tout de suite."},
			"de":    {Name: "Live-TV-Programme aktualisieren", Description: "Lädt die IPTV-Kanallisten und dann die XMLTV-Programme der Live-TV-Kataloge herunter, die seit der unter Einstellungen › Live-TV festgelegten Anzahl Stunden nicht geladen wurden; von Hand gestartet, lädt sie sofort alle Listen und Programme herunter."},
			"es":    {Name: "Actualizar las guías de televisión en vivo", Description: "Descarga las listas de canales IPTV y luego las guías XMLTV de los catálogos de televisión en vivo que no se han descargado en las horas elegidas en Ajustes › Televisión en vivo; iniciada a mano, descarga ahora todas las listas y guías."},
			"it":    {Name: "Aggiorna le guide della Diretta TV", Description: "Scarica gli elenchi canali IPTV, poi le guide XMLTV dei cataloghi di Diretta TV non scaricate da quante ore scelte in Impostazioni › Diretta TV; avviata a mano, scarica subito tutti gli elenchi e tutte le guide."},
			"pt-BR": {Name: "Atualizar os guias de TV ao Vivo", Description: "Baixa as listas de canais IPTV e depois os guias XMLTV dos catálogos de TV ao Vivo que não foram baixados nas horas definidas em Configurações › TV ao Vivo; iniciada manualmente, baixa agora todas as listas e guias."},
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
			"en":    {Name: "Read the libraries' first pages", Description: "Reads the first page of each library from the addons when it is older than Refresh catalogs after (minutes), so that home screens never wait for them, then downloads their posters and resizes them for the apps' cards, and forgets the catalog pages and descriptions no one read for long."},
			"fr":    {Name: "Lire la première page des médiathèques", Description: "Relit auprès des addons la première page de chaque médiathèque plus ancienne que Rafraîchir les catalogues après (minutes), pour que les écrans d’accueil ne les attendent jamais, puis télécharge leurs affiches et les redimensionne pour les vignettes des applications, et oublie les pages de catalogue et les descriptions que personne n’a lues depuis longtemps."},
			"de":    {Name: "Erste Seiten der Bibliotheken lesen", Description: "Liest bei den Add-ons die erste Seite jeder Bibliothek neu, die älter ist als „Kataloge aktualisieren nach (Minuten)“, damit Startbildschirme nie darauf warten, lädt dann ihre Poster herunter und skaliert sie für die Kacheln der Apps, und vergisst die Katalogseiten und Beschreibungen, die lange niemand gelesen hat."},
			"es":    {Name: "Leer la primera página de las bibliotecas", Description: "Vuelve a pedir a las extensiones la primera página de cada biblioteca más antigua que «Actualizar los catálogos después de (minutos)», para que las pantallas de inicio nunca la esperen, luego descarga sus pósteres y los redimensiona para las tarjetas de las aplicaciones, y olvida las páginas de catálogo y las descripciones que nadie ha leído en mucho tiempo."},
			"it":    {Name: "Leggi la prima pagina delle librerie", Description: "Rilegge dagli add-on la prima pagina di ogni libreria più vecchia di «Aggiorna i cataloghi dopo (minuti)», così le schermate iniziali non la aspettano mai, poi scarica le loro locandine e le ridimensiona per le schede delle app, e dimentica le pagine di catalogo e le descrizioni che nessuno legge da tempo."},
			"pt-BR": {Name: "Ler a primeira página das bibliotecas", Description: "Lê de novo nos complementos a primeira página de cada biblioteca mais antiga que “Atualizar catálogos após (minutos)”, para que as telas iniciais nunca esperem por ela, depois baixa os pôsteres delas e os redimensiona para os cartões dos aplicativos, e esquece as páginas de catálogo e as descrições que ninguém lê há muito tempo."},
		},
		Interval: library.WarmInterval,
		AtStart:  true,
		Run:      lib.Warm,
	})
	// Reading every collection asks the addons for many pages: it runs at
	// the hour an administrator sets, never by default.
	registry.Register(tasks.Task{
		Key:      "ReadCollections",
		Category: tasks.CategoryLibrary,
		Text: map[string]tasks.Text{
			"en":    {Name: "Read every collection", Description: "Reads every collection of the server's collection libraries whole, one after the other, then the catalogs of the server's smart lists, so that each opens at once with all its titles: every day at the hour set under Settings › Catalogs, never by default."},
			"fr":    {Name: "Lire toutes les collections", Description: "Lit en entier toutes les collections des médiathèques de collections du serveur, l’une après l’autre, puis les catalogues des listes intelligentes du serveur, pour que chacune s’ouvre tout de suite avec tous ses titres : chaque jour à l’heure choisie dans Paramètres › Catalogues, jamais par défaut."},
			"de":    {Name: "Alle Sammlungen lesen", Description: "Liest alle Sammlungen der Sammlungsbibliotheken des Servers nacheinander vollständig, dann die Kataloge der intelligenten Listen des Servers, damit sich jede sofort mit allen Titeln öffnet: jeden Tag zu der unter Einstellungen › Kataloge festgelegten Stunde, standardmäßig nie."},
			"es":    {Name: "Leer todas las colecciones", Description: "Lee enteras, una tras otra, todas las colecciones de las bibliotecas de colecciones del servidor y luego los catálogos de las listas inteligentes del servidor, para que cada una se abra al instante con todos sus títulos: cada día a la hora elegida en Ajustes › Catálogos, nunca por defecto."},
			"it":    {Name: "Leggi tutte le collezioni", Description: "Legge per intero, una dopo l’altra, tutte le collezioni delle librerie di collezioni del server, poi i cataloghi delle liste intelligenti del server, così ognuna si apre subito con tutti i suoi titoli: ogni giorno all’ora scelta in Impostazioni › Cataloghi, mai in modo predefinito."},
			"pt-BR": {Name: "Ler todas as coleções", Description: "Lê por inteiro, uma após a outra, todas as coleções das bibliotecas de coleções do servidor e depois os catálogos das listas inteligentes do servidor, para que cada uma abra na hora com todos os seus títulos: todos os dias na hora definida em Configurações › Catálogos, nunca por padrão."},
		},
		Daily: func() int { return store.Settings().CollectionReadHour },
		Run:   lib.ReadCollections,
	})
	// Asking for ratings costs requests to addons: this runs only when an
	// administrator starts it.
	registry.Register(tasks.Task{
		Key:      "RefreshRatings",
		Category: tasks.CategoryLibrary,
		Text: map[string]tasks.Text{
			"en":    {Name: "Refresh ratings", Description: "Asks the server's addons again for the ratings and genres of titles last asked long ago, which parental control and blocked genres use."},
			"fr":    {Name: "Actualiser les classifications", Description: "Redemande aux addons du serveur les classifications et les genres des titres demandés il y a longtemps, qui servent au contrôle parental et aux genres bloqués."},
			"de":    {Name: "Altersfreigaben aktualisieren", Description: "Fragt die Add-ons des Servers erneut nach den Altersfreigaben und Genres der Titel, die zuletzt vor langer Zeit abgefragt wurden und die Kindersicherung und gesperrte Genres verwenden."},
			"es":    {Name: "Actualizar las clasificaciones", Description: "Vuelve a pedir a las extensiones del servidor las clasificaciones y los géneros de los títulos pedidos hace tiempo, que usan el control parental y los géneros bloqueados."},
			"it":    {Name: "Aggiorna le classificazioni", Description: "Richiede agli add-on del server le classificazioni e i generi dei titoli richiesti molto tempo fa, che usano il controllo parentale e i generi bloccati."},
			"pt-BR": {Name: "Atualizar as classificações", Description: "Pede de novo aos complementos do servidor as classificações etárias e os gêneros dos títulos pedidos há muito tempo, que o controle parental e os gêneros bloqueados usam."},
		},
		Run: func(ctx context.Context) error {
			asked, err := lib.RefreshRatings(ctx)
			logger.Info("Ratings refreshed", "titles", asked)
			return err
		},
	})
	// An account is refused from its end date on its own: this only writes
	// the end to the activity log.
	registry.Register(tasks.Task{
		Key:      "RecordEndedAccounts",
		Category: tasks.CategoryMaintenance,
		Text: map[string]tasks.Text{
			"en":    {Name: "Record ended accounts", Description: "Writes to the activity log, once each, the accounts whose end date has come."},
			"fr":    {Name: "Noter les comptes arrivés à leur fin", Description: "Inscrit au journal d’activité, une fois chacun, les comptes dont la date de fin est arrivée."},
			"de":    {Name: "Abgelaufene Konten protokollieren", Description: "Trägt die Konten, deren Enddatum erreicht ist, einmal je Konto ins Aktivitätsprotokoll ein."},
			"es":    {Name: "Anotar las cuentas finalizadas", Description: "Anota en el registro de actividad, una vez cada una, las cuentas cuya fecha de fin ha llegado."},
			"it":    {Name: "Annota gli account scaduti", Description: "Scrive nel registro attività, una volta ciascuno, gli account la cui data di fine è arrivata."},
			"pt-BR": {Name: "Registrar as contas encerradas", Description: "Registra no registro de atividades, uma vez cada, as contas cuja data de encerramento chegou."},
		},
		Interval: accounts.EndCheckInterval,
		AtStart:  true,
		Run: func(ctx context.Context) error {
			ended, err := store.EndedAccounts(ctx)
			for _, user := range ended {
				activityLog.AccountEnded(ctx, user)
			}
			return err
		},
	})
	// A share link stops at its expiry on its own: this only writes the
	// expiry to the activity log, and forgets the links ended long ago.
	registry.Register(tasks.Task{
		Key:      "RecordExpiredShareLinks",
		Category: tasks.CategoryMaintenance,
		Text: map[string]tasks.Text{
			"en":    {Name: "Record expired share links", Description: "Writes to the activity log, once each, the share links that expired, and deletes those that ended more than 30 days ago."},
			"fr":    {Name: "Noter les liens de partage expirés", Description: "Inscrit au journal d’activité, une fois chacun, les liens de partage qui ont expiré, et supprime ceux qui ont pris fin il y a plus de 30 jours."},
			"de":    {Name: "Abgelaufene Freigabelinks protokollieren", Description: "Trägt die abgelaufenen Freigabelinks einmal je Link ins Aktivitätsprotokoll ein und löscht die, die vor mehr als 30 Tagen endeten."},
			"es":    {Name: "Anotar los enlaces para compartir caducados", Description: "Anota en el registro de actividad, una vez cada uno, los enlaces para compartir que han caducado, y borra los que terminaron hace más de 30 días."},
			"it":    {Name: "Annota i link di condivisione scaduti", Description: "Scrive nel registro attività, una volta ciascuno, i link di condivisione scaduti ed elimina quelli terminati da più di 30 giorni."},
			"pt-BR": {Name: "Registrar os links de compartilhamento expirados", Description: "Registra no registro de atividades, uma vez cada, os links de compartilhamento que expiraram, e exclui os que terminaram há mais de 30 dias."},
		},
		Interval: accounts.ShareLinkCheckInterval,
		AtStart:  true,
		Run: func(ctx context.Context) error {
			expired, err := store.ExpiredShareLinks(ctx)
			for _, link := range expired {
				activityLog.ShareLinkExpiredAt(ctx, link)
			}
			return err
		},
	})
}

// registerLocalFoldersTask registers the scans of local folders: each
// folder at startup, then every LocalScanHours, all of them when run by
// hand.
func registerLocalFoldersTask(registry *tasks.Registry, folders *localfiles.Service) {
	registry.Register(tasks.Task{
		Key:      "ScanLocalFolders",
		Category: tasks.CategoryLibrary,
		Text: map[string]tasks.Text{
			"en":    {Name: "Scan local folders", Description: "Looks in the local folders for the files added, changed or removed since their last scan, and matches the new ones to titles: each folder at startup, then every number of hours set under Settings › Catalogs; run by hand, scans every folder now."},
			"fr":    {Name: "Analyser les dossiers locaux", Description: "Cherche dans les dossiers locaux les fichiers ajoutés, modifiés ou supprimés depuis leur dernière analyse, et associe les nouveaux à des titres : chaque dossier au démarrage, puis toutes les heures choisies dans Paramètres › Catalogues ; lancée à la main, analyse tous les dossiers tout de suite."},
			"de":    {Name: "Lokale Ordner scannen", Description: "Sucht in den lokalen Ordnern die seit dem letzten Scan hinzugefügten, geänderten oder entfernten Dateien und ordnet die neuen Titeln zu: jeden Ordner beim Start, dann alle unter Einstellungen › Kataloge festgelegten Stunden; von Hand gestartet, scannt sie sofort alle Ordner."},
			"es":    {Name: "Escanear las carpetas locales", Description: "Busca en las carpetas locales los archivos añadidos, modificados o eliminados desde su último escaneo, y asocia los nuevos a títulos: cada carpeta al iniciarse, luego cada número de horas elegido en Ajustes › Catálogos; iniciada a mano, escanea ahora todas las carpetas."},
			"it":    {Name: "Scansiona le cartelle locali", Description: "Cerca nelle cartelle locali i file aggiunti, modificati o rimossi dall’ultima scansione e associa i nuovi ai titoli: ogni cartella all’avvio, poi ogni numero di ore scelto in Impostazioni › Cataloghi; avviata a mano, scansiona subito tutte le cartelle."},
			"pt-BR": {Name: "Rastrear as pastas locais", Description: "Procura nas pastas locais os arquivos adicionados, alterados ou removidos desde o último rastreamento e associa os novos a títulos: cada pasta na inicialização, depois a cada número de horas definido em Configurações › Catálogos; iniciada manualmente, rastreia agora todas as pastas."},
		},
		Interval: localfiles.ScanCheck,
		AtStart:  true,
		Run:      func(ctx context.Context) error { return folders.ScanDue(ctx, tasks.ByHand(ctx)) },
	})
}

// registerRecordingTasks registers the periodic jobs of Live TV recording,
// which do nothing while it is off, and the reading of the recordings not
// read yet for their commercials, which does nothing while that is off:
// the scheduler itself runs on its own (see recordings.Service.Run).
func registerRecordingTasks(registry *tasks.Registry, recorder *recordings.Service, breaks *commercials.Service) {
	registry.Register(tasks.Task{
		Key:      "DeleteOldRecordings",
		Category: tasks.CategoryLiveTV,
		Text: map[string]tasks.Text{
			"en":    {Name: "Delete old recordings", Description: "Deletes the Live TV recordings older than the number of days the settings keep them."},
			"fr":    {Name: "Supprimer les anciens enregistrements", Description: "Supprime les enregistrements de TV en direct plus anciens que le nombre de jours choisi dans les paramètres."},
			"de":    {Name: "Alte Aufnahmen löschen", Description: "Löscht die Live-TV-Aufnahmen, die älter sind als die in den Einstellungen festgelegte Anzahl Tage."},
			"es":    {Name: "Borrar las grabaciones antiguas", Description: "Borra las grabaciones de televisión en vivo más antiguas que el número de días elegido en los ajustes."},
			"it":    {Name: "Elimina le vecchie registrazioni", Description: "Elimina le registrazioni della Diretta TV più vecchie del numero di giorni scelto nelle impostazioni."},
			"pt-BR": {Name: "Excluir gravações antigas", Description: "Exclui as gravações de TV ao Vivo mais antigas que o número de dias definido nas configurações."},
		},
		Interval: recordings.SweepInterval,
		AtStart:  true,
		Run:      recorder.SweepRecordings,
	})
	registry.Register(tasks.Task{
		Key:      "ScheduleSeriesRecordings",
		Category: tasks.CategoryLiveTV,
		Text: map[string]tasks.Text{
			"en":    {Name: "Schedule series recordings", Description: "Looks in the guide for the new programmes of each series recording and schedules them."},
			"fr":    {Name: "Programmer les enregistrements de séries", Description: "Cherche dans le guide les nouvelles émissions de chaque série enregistrée et les programme."},
			"de":    {Name: "Serienaufnahmen planen", Description: "Sucht im Fernsehprogramm die neuen Sendungen jeder Serienaufnahme und plant sie ein."},
			"es":    {Name: "Programar las grabaciones de series", Description: "Busca en la guía los nuevos programas de cada grabación de serie y los programa."},
			"it":    {Name: "Pianifica le registrazioni delle serie TV", Description: "Cerca nella guida i nuovi programmi di ogni registrazione di serie TV e li pianifica."},
			"pt-BR": {Name: "Agendar gravações de séries", Description: "Procura no guia os novos programas de cada gravação de série e os agenda."},
		},
		Interval: recordings.SeriesRefreshInterval,
		AtStart:  true,
		Run:      recorder.RefreshSeriesTimers,
	})
	registry.Register(tasks.Task{
		Key:      "FindCommercials",
		Category: tasks.CategoryLiveTV,
		Text: map[string]tasks.Text{
			"en":    {Name: "Find commercials in recordings", Description: "Reads the finished recordings not read yet for their commercial breaks, one at a time, while Find commercials in recordings is on under Settings › Recordings. Each recording is otherwise read once it ends."},
			"fr":    {Name: "Trouver les publicités des enregistrements", Description: "Lit les enregistrements terminés pas encore lus pour trouver leurs coupures publicitaires, un à la fois, quand Trouver les publicités des enregistrements est activé dans Paramètres › Enregistrements. Chaque enregistrement est sinon lu dès qu’il se termine."},
			"de":    {Name: "Werbung in Aufnahmen erkennen", Description: "Liest die beendeten, noch nicht gelesenen Aufnahmen nacheinander auf Werbeunterbrechungen, solange „Werbung in Aufnahmen erkennen“ unter Einstellungen › Aufnahmen aktiviert ist. Sonst wird jede Aufnahme gelesen, sobald sie endet."},
			"es":    {Name: "Detectar anuncios en las grabaciones", Description: "Lee, de una en una, las grabaciones terminadas aún no leídas para encontrar sus pausas publicitarias, mientras «Detectar anuncios en las grabaciones» está activado en Ajustes › Grabaciones. Si no, cada grabación se lee en cuanto termina."},
			"it":    {Name: "Trova la pubblicità nelle registrazioni", Description: "Legge, una alla volta, le registrazioni terminate non ancora lette per trovarne le interruzioni pubblicitarie, finché «Trova la pubblicità nelle registrazioni» è attivo in Impostazioni › Registrazioni. Altrimenti ogni registrazione viene letta appena termina."},
			"pt-BR": {Name: "Encontrar comerciais nas gravações", Description: "Lê, uma de cada vez, as gravações terminadas ainda não lidas para encontrar seus intervalos comerciais, enquanto “Encontrar comerciais nas gravações” estiver ativado em Configurações › Gravações. Caso contrário, cada gravação é lida assim que termina."},
		},
		Interval: commercials.RunInterval,
		Run:      breaks.Run,
	})
}

// registerBackupTask registers the daily database backup: it runs every
// day while backups are on, and by hand only, failing, while they are off.
func registerBackupTask(registry *tasks.Registry, backups *backup.Service) {
	registry.Register(tasks.Task{
		Key:      "BackUpDatabase",
		Category: tasks.CategoryMaintenance,
		Text: map[string]tasks.Text{
			"en":    {Name: "Back up the database", Description: "Copies the database into the backup folder every day at the hour set under Settings › Backups, while backups are on, sends the copies to the backup destinations, and deletes the oldest copies past the number each place keeps."},
			"fr":    {Name: "Sauvegarder la base de données", Description: "Copie la base de données dans le dossier des sauvegardes chaque jour à l’heure choisie dans Paramètres › Sauvegardes, quand elles sont activées, envoie les copies aux destinations des sauvegardes, et supprime les plus anciennes copies au-delà du nombre que chaque endroit conserve."},
			"de":    {Name: "Datenbank sichern", Description: "Kopiert die Datenbank jeden Tag zu der unter Einstellungen › Backups festgelegten Stunde in den Backup-Ordner, solange Backups aktiviert sind, sendet die Kopien an die Backup-Ziele und löscht die ältesten Kopien über die Anzahl hinaus, die jeder Ort behält."},
			"es":    {Name: "Hacer una copia de seguridad de la base de datos", Description: "Copia la base de datos en la carpeta de las copias de seguridad cada día a la hora elegida en Ajustes › Copias de seguridad, mientras estén activadas, envía las copias a los destinos de las copias de seguridad y borra las copias más antiguas que superen el número que conserva cada lugar."},
			"it":    {Name: "Esegui il backup del database", Description: "Copia il database nella cartella dei backup ogni giorno all’ora scelta in Impostazioni › Backup, finché i backup sono attivi, invia le copie alle destinazioni dei backup ed elimina le copie più vecchie oltre il numero che ogni posizione conserva."},
			"pt-BR": {Name: "Fazer backup do banco de dados", Description: "Copia o banco de dados para a pasta de backups todos os dias na hora definida em Configurações › Backups, enquanto os backups estiverem ativados, envia as cópias para os destinos de backup e exclui as cópias mais antigas além do número que cada local mantém."},
		},
		Daily: backups.Hour,
		Run:   backups.Run,
	})
}

// registerPlaybackHistoryTask registers the daily cleanup of the playback
// history: it deletes the playbacks older than the days the settings keep.
func registerPlaybackHistoryTask(registry *tasks.Registry, history *plays.History, logger *slog.Logger) {
	registry.Register(tasks.Task{
		Key:      "CleanPlaybackHistory",
		Category: tasks.CategoryMaintenance,
		Text: map[string]tasks.Text{
			"en":    {Name: "Clean the playback history", Description: "Deletes the playbacks older than the days to keep set under Settings › Playback."},
			"fr":    {Name: "Nettoyer l’historique de lecture", Description: "Supprime les lectures plus anciennes que le nombre de jours à conserver choisi dans Paramètres › Lecture."},
			"de":    {Name: "Wiedergabeverlauf bereinigen", Description: "Löscht die Wiedergaben, die älter sind als die unter Einstellungen › Wiedergabe festgelegten Tage."},
			"es":    {Name: "Limpiar el historial de reproducción", Description: "Borra las reproducciones más antiguas que los días que se conservan, elegidos en Ajustes › Reproducción."},
			"it":    {Name: "Pulisci la cronologia di riproduzione", Description: "Elimina le riproduzioni più vecchie dei giorni da conservare scelti in Impostazioni › Riproduzione."},
			"pt-BR": {Name: "Limpar o histórico de reprodução", Description: "Exclui as reproduções mais antigas que os dias a manter definidos em Configurações › Reprodução."},
		},
		Interval: plays.SweepInterval,
		AtStart:  true,
		Run: func(ctx context.Context) error {
			deleted, err := history.Sweep(ctx)
			if deleted > 0 {
				logger.Info("Deleted old playbacks from the history", "playbacks", deleted)
			}
			return err
		},
	})
}

// registerSummaryTasks registers the weekly summary and the month and year
// in review, by hand only: their schedule is the day and hour of Settings ›
// Notifications, which the notifications follow themselves (see
// notifications.Service.CheckSummary).
func registerSummaryTasks(registry *tasks.Registry, notifier *notifications.Service) {
	send := func(event string) func(context.Context) error {
		return func(ctx context.Context) error { return notifier.SendSummary(ctx, event) }
	}
	registry.Register(tasks.Task{
		Key:      "SendWeeklySummary",
		Category: tasks.CategoryMaintenance,
		Text: map[string]tasks.Text{
			"en":    {Name: "Send the weekly summary", Description: "Sends the summary of the last 7 days now to every notification target that chose it. Every week, it is sent on its own at the day and hour set under Settings › Notifications."},
			"fr":    {Name: "Envoyer le résumé de la semaine", Description: "Envoie tout de suite le résumé des 7 derniers jours à toutes les cibles de notification qui l’ont choisi. Chaque semaine, il est envoyé de lui-même au jour et à l’heure choisis dans Paramètres › Notifications."},
			"de":    {Name: "Wochenzusammenfassung senden", Description: "Sendet die Zusammenfassung der letzten 7 Tage sofort an jedes Benachrichtigungsziel, das sie gewählt hat. Jede Woche wird sie von selbst an dem Tag und zu der Stunde gesendet, die unter Einstellungen › Benachrichtigungen festgelegt sind."},
			"es":    {Name: "Enviar el resumen semanal", Description: "Envía ahora el resumen de los últimos 7 días a cada destino de notificaciones que lo eligió. Cada semana se envía solo el día y a la hora elegidos en Ajustes › Notificaciones."},
			"it":    {Name: "Invia il riepilogo settimanale", Description: "Invia subito il riepilogo degli ultimi 7 giorni a ogni destinazione delle notifiche che l’ha scelto. Ogni settimana viene inviato da solo nel giorno e all’ora scelti in Impostazioni › Notifiche."},
			"pt-BR": {Name: "Enviar o resumo semanal", Description: "Envia agora o resumo dos últimos 7 dias para cada destino de notificação que o escolheu. Toda semana, ele é enviado sozinho no dia e na hora definidos em Configurações › Notificações."},
		},
		Run: send(notifications.WeeklySummary),
	})
	registry.Register(tasks.Task{
		Key:      "SendMonthlySummary",
		Category: tasks.CategoryMaintenance,
		Text: map[string]tasks.Text{
			"en":    {Name: "Send the month in review", Description: "Sends the review of the last month now to every notification target that chose it. Every month, it is sent on its own on the 1st, at the hour of the weekly summary."},
			"fr":    {Name: "Envoyer le bilan du mois", Description: "Envoie tout de suite le bilan du mois dernier à toutes les cibles de notification qui l’ont choisi. Chaque mois, il est envoyé de lui-même le 1er, à l’heure du résumé de la semaine."},
			"de":    {Name: "Monatsrückblick senden", Description: "Sendet den Rückblick auf den letzten Monat sofort an jedes Benachrichtigungsziel, das ihn gewählt hat. Jeden Monat wird er von selbst am 1. gesendet, zur Stunde der Wochenzusammenfassung."},
			"es":    {Name: "Enviar el resumen del mes", Description: "Envía ahora el resumen del último mes a cada destino de notificaciones que lo eligió. Cada mes se envía solo el día 1, a la hora del resumen semanal."},
			"it":    {Name: "Invia il mese in sintesi", Description: "Invia subito la sintesi dell’ultimo mese a ogni destinazione delle notifiche che l’ha scelta. Ogni mese viene inviata da sola il giorno 1, all’ora del riepilogo settimanale."},
			"pt-BR": {Name: "Enviar a retrospectiva do mês", Description: "Envia agora a retrospectiva do último mês para cada destino de notificação que a escolheu. Todo mês, ela é enviada sozinha no dia 1º, na hora do resumo semanal."},
		},
		Run: send(notifications.MonthlySummary),
	})
	registry.Register(tasks.Task{
		Key:      "SendYearlySummary",
		Category: tasks.CategoryMaintenance,
		Text: map[string]tasks.Text{
			"en":    {Name: "Send the year in review", Description: "Sends the review of the last year now to every notification target that chose it. Every year, it is sent on its own on January 1st, at the hour of the weekly summary."},
			"fr":    {Name: "Envoyer le bilan de l’année", Description: "Envoie tout de suite le bilan de l’année dernière à toutes les cibles de notification qui l’ont choisi. Chaque année, il est envoyé de lui-même le 1er janvier, à l’heure du résumé de la semaine."},
			"de":    {Name: "Jahresrückblick senden", Description: "Sendet den Rückblick auf das letzte Jahr sofort an jedes Benachrichtigungsziel, das ihn gewählt hat. Jedes Jahr wird er von selbst am 1. Januar gesendet, zur Stunde der Wochenzusammenfassung."},
			"es":    {Name: "Enviar el resumen del año", Description: "Envía ahora el resumen del último año a cada destino de notificaciones que lo eligió. Cada año se envía solo el 1 de enero, a la hora del resumen semanal."},
			"it":    {Name: "Invia l’anno in sintesi", Description: "Invia subito la sintesi dell’ultimo anno a ogni destinazione delle notifiche che l’ha scelta. Ogni anno viene inviata da sola il 1º gennaio, all’ora del riepilogo settimanale."},
			"pt-BR": {Name: "Enviar a retrospectiva do ano", Description: "Envia agora a retrospectiva do último ano para cada destino de notificação que a escolheu. Todo ano, ela é enviada sozinha em 1º de janeiro, na hora do resumo semanal."},
		},
		Run: send(notifications.YearlySummary),
	})
}

// prepareFolders makes the default recordings or backups folder when the
// settings turn that feature on, and warns about a folder Polyfin cannot
// write into: the feature fails until it is fixed under Settings.
func prepareFolders(settings accounts.Settings, dataDir string, logger *slog.Logger) {
	for _, f := range []struct{ feature, dir, defaultDir string }{
		{"recording", settings.RecordingsDir(dataDir), accounts.DefaultRecordingsFolder(dataDir)},
		{"backups", settings.BackupDir(dataDir), accounts.DefaultBackupFolder(dataDir)},
	} {
		if f.dir == "" {
			continue
		}
		if err := config.PrepareFolder(f.dir, f.dir == f.defaultDir); err != nil {
			logger.Warn("A folder the settings choose cannot be written into", "feature", f.feature, "folder", f.dir, "error", err)
		}
	}
}
