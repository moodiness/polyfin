// Command polyfin-desktop is Polyfin Desktop: Polyfin without Docker, on
// Windows, macOS and Linux. An icon in the tray, or the menu bar on macOS,
// starts the database and the server in the background, shows whether
// they run, opens the admin app, the web client and the log folder, and
// quits, stopping them. See docs/desktop.md.
//
// With -headless it runs without the icon, the browser and the login
// item, until it is interrupted: for tests and smokes.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/moodiness/polyfin/internal/desktop"
	"github.com/moodiness/polyfin/internal/desktop/app"
	"github.com/moodiness/polyfin/internal/desktop/keystore"
	"github.com/moodiness/polyfin/internal/desktop/loginitem"
	"github.com/moodiness/polyfin/internal/desktop/tools"
	"github.com/moodiness/polyfin/internal/desktop/update"
)

// version is set at build time with -ldflags "-X main.version=…": the
// server's version, which the app ships.
var version = "dev"

// DatabasePortVariable gives the cluster a fixed port.
const DatabasePortVariable = "POLYFIN_DESKTOP_DATABASE_PORT"

// quitWait bounds the stop of the server and the database when the app
// quits.
const quitWait = 3 * time.Minute

func main() {
	headless := flag.Bool("headless", false, "run without the tray icon, the browser and the login item, until interrupted")
	background := flag.Bool(strings.TrimPrefix(loginitem.Arg, "--"), false, "started with the computer: open no browser")
	showVersion := flag.Bool("version", false, "print the version")
	importKey := flag.String("import-secret-key", "", "store the secret key of the file, another server's, in place of this data folder's, then exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *importKey != "" {
		if err := importSecretKey(*importKey); err != nil {
			fmt.Fprintln(os.Stderr, "polyfin-desktop:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(*headless, *background); err != nil {
		fmt.Fprintln(os.Stderr, "polyfin-desktop:", err)
		os.Exit(1)
	}
}

func run(headless, background bool) error {
	folders, err := desktop.FindFolders()
	if err != nil {
		return err
	}
	install, err := desktop.FindInstall()
	if err != nil {
		return err
	}
	if err := folders.Make(); err != nil {
		return err
	}
	app.RotateLogs(folders.Log())
	logFile, err := os.OpenFile(filepath.Join(folders.Log(), "desktop.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	var out io.Writer = logFile
	if headless {
		out = io.MultiWriter(logFile, os.Stderr)
	}
	logger := slog.New(slog.NewTextHandler(out, nil))

	release, err := app.Lock(folders.Lock())
	if errors.Is(err, app.ErrRunning) {
		// Opening the app again opens the admin app.
		logger.Info("Polyfin Desktop already runs: opening the admin app")
		if !headless {
			return desktop.Open(app.AdminURL())
		}
		return err
	}
	if err != nil {
		return err
	}
	defer release()

	manifest, err := toolsManifest()
	if err != nil {
		return err
	}
	port := 0
	if text := os.Getenv(DatabasePortVariable); text != "" {
		if port, err = strconv.Atoi(text); err != nil || port <= 0 || port > 65535 {
			return fmt.Errorf("%s is not a port: %q", DatabasePortVariable, text)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	options := app.Options{
		Version: version, Install: install, Folders: folders, Keys: keystore.System{}, Tools: manifest, DatabasePort: port,
		OpenSetup: !headless && !background, Open: desktop.Open, Executable: exe, Logger: logger,
		Updates: &update.Checker{Version: version, GOARCH: runtime.GOARCH, Kind: update.KindOf(runtime.GOOS, os.Getenv)},
	}
	if !headless {
		if options.Login, err = loginitem.New(exe); err != nil {
			logger.Warn("Start with the computer is unavailable", "error", err)
		}
	}
	logger.Info("Polyfin Desktop starts", "version", version, "data", folders.Data, "install", install.Dir)
	if headless {
		return runHeadless(options, logger)
	}
	relaunch := runTray(options, logger)
	release()
	if relaunch != nil {
		return startAfterQuit(relaunch)
	}
	return nil
}

// importSecretKey stores the key of file in place of the data folder's,
// while the app does not run: the server would keep using the old one.
func importSecretKey(file string) error {
	folders, err := desktop.FindFolders()
	if err != nil {
		return err
	}
	if err := folders.Make(); err != nil {
		return err
	}
	release, err := app.Lock(folders.Lock())
	if err != nil {
		if errors.Is(err, app.ErrRunning) {
			return errors.New("quit Polyfin Desktop first")
		}
		return err
	}
	defer release()
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if err := keystore.Replace(keystore.System{}, keystore.Account(folders.Data), strings.TrimSpace(string(data))); err != nil {
		return err
	}
	fmt.Println("The secret key is stored: Polyfin uses it from its next start.")
	return nil
}

// toolsManifest is the pinned downloads of this system, or those of the
// manifest ManifestVariable names.
func toolsManifest() (tools.Manifest, error) {
	if file := os.Getenv(tools.ManifestVariable); file != "" {
		return tools.Load(file)
	}
	manifest, ok := tools.Pinned(runtime.GOOS, runtime.GOARCH)
	if !ok {
		return tools.Manifest{}, fmt.Errorf("no FFmpeg build is pinned for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return manifest, nil
}

// runHeadless runs until an interrupt, then stops everything.
func runHeadless(options app.Options, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var mu sync.Mutex
	var last app.Phase
	options.Changed = func(s app.Status) {
		mu.Lock()
		defer mu.Unlock()
		if s.Phase != last {
			last = s.Phase
			logger.Info("Polyfin Desktop", "phase", s.Phase, "percent", s.Percent)
		}
	}
	a := app.New(options)
	a.Start()
	<-ctx.Done()
	logger.Info("Quitting")
	quit(a)
	logger.Info("Polyfin Desktop quit")
	return nil
}

// quit stops the server and the database, waiting for them.
func quit(a *app.App) {
	ctx, cancel := context.WithTimeout(context.Background(), quitWait)
	defer cancel()
	a.Stop(ctx)
}

// startAfterQuit starts the new app, or the installer, once this one
// let its data folder go.
func startAfterQuit(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
