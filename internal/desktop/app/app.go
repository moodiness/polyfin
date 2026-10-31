// Package app runs Polyfin in the background for Polyfin Desktop: the
// cluster, then the very polyfin serve of the Docker image as a child
// process, given only the environment variables the server reads; it
// watches them, starts the server again when it ends by itself, and stops
// the server, then the cluster, when asked. The tray of cmd/polyfin-desktop
// shows its Status and calls its methods.
package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/desktop"
	"github.com/moodiness/polyfin/internal/desktop/cluster"
	"github.com/moodiness/polyfin/internal/desktop/keystore"
	"github.com/moodiness/polyfin/internal/desktop/loginitem"
	"github.com/moodiness/polyfin/internal/desktop/tools"
	"github.com/moodiness/polyfin/internal/desktop/update"
)

// Phase is what the app is doing.
type Phase string

const (
	Starting    Phase = "starting"
	Downloading Phase = "downloading"
	// Rebuilding: the database rebuilds its indexes as it starts, after an
	// update changed its collation version.
	Rebuilding Phase = "rebuilding"
	Running    Phase = "running"
	Updating   Phase = "updating"
	Stopping   Phase = "stopping"
	Stopped    Phase = "stopped"
	Failed     Phase = "failed"
)

// Problem is something the tray tells about while Polyfin runs, or why
// it stopped.
type Problem string

const (
	// NoKeyStore: the system's key store could not be read, and the
	// server runs without its key: the secrets it stores meanwhile stay
	// unencrypted, and are encrypted once the key is back.
	NoKeyStore Problem = "key-store"
	// NoTools: FFmpeg and ffprobe could not be downloaded; the next start
	// tries again.
	NoTools Problem = "tools"
	// UpdateFailed: the new release could not be installed.
	UpdateFailed Problem = "update"
	// StartFailed: the database or the server could not start.
	StartFailed Problem = "start"
)

// Status is what the tray shows.
type Status struct {
	Phase Phase
	// Percent is the progress of a download, while Downloading or
	// Updating.
	Percent int
	// Problems are what went wrong, with Detail the last error, for the
	// log folder to explain.
	Problems []Problem
	Detail   string
	// SetupRequired tells that no administrator exists yet, and
	// SetupCode is the code the admin app's setup asks for.
	SetupRequired bool
	SetupCode     string
	// Offer is a new release to install.
	Offer *update.Offer
	// StartWithComputer tells whether the system starts the app.
	StartWithComputer bool
}

func (s Status) Has(p Problem) bool {
	for _, have := range s.Problems {
		if have == p {
			return true
		}
	}
	return false
}

// Options are what the app runs with.
type Options struct {
	// Version is the app's version, which is the server's.
	Version string
	Install desktop.Install
	Folders desktop.Folders
	Keys    keystore.Store
	Tools   tools.Manifest
	// DatabasePort is the cluster's port; 0 chooses a free one.
	DatabasePort int
	// Login is the login item; nil leaves it alone, as in tests.
	Login loginitem.Item
	// OpenSetup opens the admin app's setup in the browser while no
	// administrator exists: off when the system started the app.
	OpenSetup bool
	// Open opens an address in the browser.
	Open func(string) error
	// Updates finds the release to offer; nil offers none.
	Updates *update.Checker
	// Executable is the app's, which an update replaces.
	Executable string
	Logger     *slog.Logger
	// Changed is told every change of the status.
	Changed func(Status)
}

// App runs Polyfin.
type App struct {
	o      Options
	logger *slog.Logger
	client *http.Client

	mu     sync.Mutex
	status Status
	cancel context.CancelFunc
	done   chan struct{}
	// opened tells that the setup was opened in the browser already.
	opened bool

	// offerMu orders the writes of the offer, and offered is the last one
	// written.
	offerMu sync.Mutex
	offered *desktop.Offered

	// pending is the update this version was installed by, until its
	// server answers /healthz: nil once it did, or without one. Under mu.
	pending *update.Pending
	// rolledBack carries what starts the previous version, once the app
	// went back to it (see update.Pending).
	rolledBack chan *exec.Cmd
}

// New returns an app, stopped.
func New(o Options) *App {
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	if o.Open == nil {
		o.Open = func(string) error { return nil }
	}
	a := &App{o: o, logger: o.Logger, client: &http.Client{Timeout: 5 * time.Second}, rolledBack: make(chan *exec.Cmd, 1)}
	on := a.readState().StartWithComputer
	a.status = Status{Phase: Stopped, StartWithComputer: on != nil && *on}
	return a
}

// RolledBack gives what starts the previous version once the app went
// back to it, as the new version's server started update.MaxStarts times
// without answering: the app is to quit, then start it.
func (a *App) RolledBack() <-chan *exec.Cmd { return a.rolledBack }

// Status is the app's status now.
func (a *App) Status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

func (a *App) update(change func(*Status)) {
	a.mu.Lock()
	change(&a.status)
	status := a.status
	a.mu.Unlock()
	a.publishOffer(status)
	if a.o.Changed != nil {
		a.o.Changed(status)
	}
}

// publishOffer writes the update the app offers, while the server runs,
// to Folders.Offer, for the admin app to offer it too, when it changed.
func (a *App) publishOffer(s Status) {
	var offered *desktop.Offered
	if s.Offer != nil && (s.Phase == Running || s.Phase == Updating) {
		offered = &desktop.Offered{Version: s.Offer.Version, Notes: s.Offer.Notes, Installing: s.Phase == Updating,
			Failed: s.Has(UpdateFailed)}
		if offered.Installing {
			offered.Percent = s.Percent
		}
	}
	a.offerMu.Lock()
	defer a.offerMu.Unlock()
	if offered == a.offered || offered != nil && a.offered != nil && *offered == *a.offered {
		return
	}
	if err := desktop.WriteOffer(a.o.Folders, offered); err != nil {
		a.logger.Warn("The update offered could not be told to the admin app", "error", err)
		return
	}
	a.offered = offered
}

func (a *App) problem(p Problem, err error) {
	a.update(func(s *Status) {
		if !s.Has(p) {
			s.Problems = append(s.Problems, p)
		}
		if err != nil {
			s.Detail = err.Error()
		}
	})
}

// Start starts the cluster and the server in the background, unless they
// run already.
func (a *App) Start() {
	a.mu.Lock()
	if a.done != nil {
		a.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel, a.done = cancel, make(chan struct{})
	done := a.done
	a.status.Phase, a.status.Problems, a.status.Detail, a.status.Percent = Starting, nil, "", 0
	a.mu.Unlock()
	a.update(func(*Status) {})
	go func() {
		defer close(done)
		a.run(ctx)
	}()
}

// Stop stops the server, then the cluster, and returns once both have
// stopped, or when ctx ends.
func (a *App) Stop(ctx context.Context) {
	a.mu.Lock()
	cancel, done := a.cancel, a.done
	a.mu.Unlock()
	if done == nil {
		return
	}
	cancel()
	select {
	case <-done:
	case <-ctx.Done():
		return
	}
	a.mu.Lock()
	if a.done == done {
		a.cancel, a.done = nil, nil
	}
	a.mu.Unlock()
}

// Listen is the server's address, from POLYFIN_LISTEN as the server reads
// it: the address to ask it at, and the one to open in the browser.
func Listen() (ask, open string) {
	listen := os.Getenv("POLYFIN_LISTEN")
	if listen == "" {
		listen = ":8096"
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		host, port = "", "8096"
	}
	switch host {
	case "", "0.0.0.0", "::":
		return net.JoinHostPort("127.0.0.1", port), net.JoinHostPort("localhost", port)
	}
	return net.JoinHostPort(host, port), net.JoinHostPort(host, port)
}

// AdminURL is the admin app's address, WebURL the web client's.
func AdminURL() string { _, open := Listen(); return "http://" + open + "/admin/" }
func WebURL() string   { _, open := Listen(); return "http://" + open + "/web/" }

// run starts everything, watches it until ctx ends, then stops it.
func (a *App) run(ctx context.Context) {
	db := &cluster.Cluster{Bin: a.o.Install.PostgresBin(), Dir: a.o.Folders.Cluster(),
		Log: filepath.Join(a.o.Folders.Log(), "postgresql.log"), Port: a.o.DatabasePort, Logger: a.logger,
		Rebuilding: a.rebuilding}
	server, err := a.prepare(ctx, db)
	var rollBack bool
	defer func() {
		a.update(func(s *Status) { s.Phase = Stopping })
		// Stopping must finish, whatever ended the run.
		stop := context.Background()
		if server != nil {
			server.stop()
		}
		if err := db.Stop(stop); err != nil {
			a.logger.Error("The database could not be stopped", "error", err)
		}
		if rollBack {
			a.goBack(server.lastExit())
		}
		a.update(func(s *Status) {
			if s.Phase != Failed {
				s.Phase = Stopped
			}
			s.SetupCode = ""
		})
	}()
	if err != nil {
		if ctx.Err() == nil {
			a.logger.Error("Polyfin could not start", "error", err)
			a.problem(StartFailed, err)
			a.update(func(s *Status) { s.Phase = Failed })
		}
		return
	}
	ctx, stopRun := context.WithCancel(ctx)
	defer stopRun()
	var wg sync.WaitGroup
	wg.Go(func() {
		// A new version that does not start ends the run, for the
		// previous one to be put back once everything stopped.
		rollBack = server.supervise(ctx)
		stopRun()
	})
	wg.Go(func() { a.monitor(ctx, db) })
	wg.Wait()
}

// reviewPending reads the update this version may have been installed
// by: while its server has not answered yet, the previous app and the
// copy of the database are kept. A record of another version, as left by
// an installer that was cancelled, is dropped with what it kept.
func (a *App) reviewPending() {
	if a.o.Updates != nil {
		if err := update.RemoveLeftovers(a.o.Updates.Kind, a.o.Executable); err != nil {
			a.logger.Warn("What the return to the previous version left could not be removed", "error", err)
		}
	}
	pending, err := update.ReadPending(a.o.Folders)
	if err != nil {
		a.logger.Warn("The record of the last update could not be read", "error", err)
		return
	}
	if pending == nil {
		return
	}
	if pending.To != a.o.Version {
		a.logger.Info("The update to another version was not installed: what it kept is removed", "version", pending.To)
		if err := pending.Confirm(a.o.Folders, a.o.Executable); err != nil {
			a.logger.Warn("What the update kept could not be removed", "error", err)
		}
		return
	}
	a.logger.Info("The new version runs: the previous one is kept until its server answers", "previous", pending.From,
		"starts", pending.Starts)
	a.mu.Lock()
	a.pending = pending
	a.mu.Unlock()
}

// countStart counts a start of the new version's server, while it has not
// answered yet: false once it started update.MaxStarts times, for the app
// to go back to the previous version.
func (a *App) countStart() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pending == nil {
		return true
	}
	if a.pending.Starts >= update.MaxStarts {
		return false
	}
	a.pending.Starts++
	if err := a.pending.Write(a.o.Folders); err != nil {
		a.logger.Warn("The record of the last update could not be written", "error", err)
	}
	return true
}

// mayStartAgain tells whether the server may start again: false once a
// new version's server started update.MaxStarts times without answering.
func (a *App) mayStartAgain() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pending == nil || a.pending.Starts < update.MaxStarts
}

// probation tells whether the new version's server has not answered yet.
func (a *App) probation() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pending != nil
}

// confirm removes the previous app and the copy of the database, once the
// new version's server answered /healthz.
func (a *App) confirm() {
	a.mu.Lock()
	pending := a.pending
	a.pending = nil
	a.mu.Unlock()
	if pending == nil {
		return
	}
	a.logger.Info("The new version answers: the previous one is removed", "version", pending.To, "previous", pending.From)
	if err := pending.Confirm(a.o.Folders, a.o.Executable); err != nil {
		a.logger.Warn("The previous version could not be removed", "error", err)
	}
	if err := desktop.RemoveRolledBack(a.o.Folders); err != nil {
		a.logger.Warn("The record of an earlier return to the previous version could not be removed", "error", err)
	}
}

// goBack puts the previous app and the copy of the database back, once
// the server and the database stopped, records why for System › Health,
// and gives what starts the previous app to RolledBack. A failure is told
// as the start's.
func (a *App) goBack(reason error) {
	a.mu.Lock()
	pending := a.pending
	a.mu.Unlock()
	why := fmt.Sprintf("its server stopped %d times before it answered", pending.Starts)
	if reason != nil {
		why += ": " + reason.Error()
	}
	a.logger.Error("The new version does not start: going back to the previous one", "version", pending.To, "previous", pending.From,
		"reason", why)
	cmd, err := pending.Restore(a.o.Folders, a.o.Folders.Cluster(), a.o.Executable)
	if err != nil {
		a.logger.Error("The previous version could not be put back", "error", err)
		a.problem(StartFailed, err)
		a.update(func(s *Status) { s.Phase = Failed })
		return
	}
	a.mu.Lock()
	a.pending = nil
	a.mu.Unlock()
	record := desktop.RolledBack{Version: pending.To, Previous: pending.From, Reason: why, At: time.Now().UTC()}
	if err := desktop.WriteRolledBack(a.o.Folders, record); err != nil {
		a.logger.Warn("The return to the previous version could not be recorded", "error", err)
	}
	select {
	case a.rolledBack <- cmd:
	default:
	}
}

// prepare stops a server a crash left running, reads the key, starts the
// cluster and makes sure the tools are there.
func (a *App) prepare(ctx context.Context, db *cluster.Cluster) (*serverProcess, error) {
	if err := a.o.Install.Check(); err != nil {
		return nil, err
	}
	if err := a.o.Folders.Make(); err != nil {
		return nil, err
	}
	stopLeftover(a.o.Folders.ServerPID(), a.logger)
	a.reviewPending()
	key := ""
	if a.o.Keys != nil {
		var created bool
		var err error
		key, created, err = keystore.SecretKey(a.o.Keys, keystore.Account(a.o.Folders.Data))
		switch {
		case err != nil:
			a.logger.Warn("The secret key could not be read from the system's key store: Polyfin runs without it", "error", err)
			a.problem(NoKeyStore, err)
		case created:
			a.logger.Info("A secret key was made and kept in the system's key store")
		}
	}
	if _, err := db.Prepare(ctx); err != nil {
		return nil, err
	}
	if err := db.Start(ctx); err != nil {
		return nil, err
	}
	paths := a.ensureTools(ctx)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	a.applyLoginItem()
	return &serverProcess{app: a, env: serverEnvironment(os.Environ(), a.o, db.URL(), key, paths)}, nil
}

// ensureTools downloads FFmpeg and ffprobe when they are missing, with
// progress. A failure leaves the server without them, and is told.
func (a *App) ensureTools(ctx context.Context) tools.Paths {
	t := tools.Tools{Dir: a.o.Folders.Tools(), Manifest: a.o.Tools}
	if paths, ok := t.Ready(); ok {
		return paths
	}
	a.logger.Info("Downloading FFmpeg and ffprobe", "version", a.o.Tools.Version)
	a.update(func(s *Status) { s.Phase, s.Percent = Downloading, 0 })
	last := -1
	paths, err := t.Ensure(ctx, func(done, total int64) {
		percent := 0
		if total > 0 {
			percent = int(done * 100 / total)
		}
		if percent != last {
			last = percent
			a.update(func(s *Status) { s.Percent = percent })
		}
	})
	a.update(func(s *Status) { s.Phase, s.Percent = Starting, 0 })
	if err != nil {
		if ctx.Err() == nil {
			a.logger.Error("FFmpeg and ffprobe could not be downloaded: the next start tries again", "error", err)
			a.problem(NoTools, err)
		}
		return tools.Paths{}
	}
	a.logger.Info("FFmpeg and ffprobe are ready", "ffmpeg", paths.FFmpeg)
	return paths
}

// rebuilding shows the database's rebuild of its indexes while it starts,
// which may take a while on a large library.
func (a *App) rebuilding(on bool) {
	a.update(func(s *Status) {
		switch {
		case on && s.Phase == Starting:
			s.Phase = Rebuilding
		case !on && s.Phase == Rebuilding:
			s.Phase = Starting
		}
	})
}

// serverEnvironment is the app's environment, without what would point
// the server elsewhere, with the variables the server reads: its cluster,
// the data folder, the tools, jellyfin-web, the fonts, the key, that the
// app runs it (desktop.ServerVariable), and PATH starting with
// PostgreSQL's programs, where backups find pg_dump and pg_restore.
func serverEnvironment(base []string, o Options, databaseURL, key string, paths tools.Paths) []string {
	set := map[string]string{
		"POLYFIN_DATABASE_URL": databaseURL,
		"POLYFIN_DATA_DIR":     o.Folders.Data,
		"POLYFIN_WEB_DIR":      o.Install.WebDir(),
		"POLYFIN_FONTS_DIR":    o.Install.FontsDir(),
		desktop.ServerVariable: "1",
	}
	if paths.FFmpeg != "" {
		set["POLYFIN_FFMPEG"], set["POLYFIN_FFPROBE"] = paths.FFmpeg, paths.FFprobe
	}
	if key != "" {
		set["POLYFIN_SECRET_KEY"] = key
	}
	path := o.Install.PostgresBin()
	env := make([]string, 0, len(base)+len(set)+1)
	for _, entry := range base {
		name, value, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(name)
		switch {
		case upper == "PATH":
			if value != "" {
				path += string(os.PathListSeparator) + value
			}
			continue
		case upper == "POLYFIN_SECRET_KEY", upper == "POLYFIN_FFMPEG", upper == "POLYFIN_FFPROBE", strings.HasPrefix(upper, "PG"):
			continue
		}
		if _, ours := set[upper]; ours {
			continue
		}
		env = append(env, entry)
	}
	for name, value := range set {
		env = append(env, name+"="+value)
	}
	return append(env, "PATH="+path)
}

// adminStatus is what the admin app's status answers.
type adminStatus struct {
	Database      string `json:"database"`
	SetupRequired bool   `json:"setupRequired"`
}

// monitor asks the server how it is, every 2 seconds until it answers,
// then every 10; starts the cluster again when it stopped by itself;
// opens the setup once; turns Start with the computer on once set up; and
// looks for an update every hour.
func (a *App) monitor(ctx context.Context, db *cluster.Cluster) {
	ask, _ := Listen()
	var nextUpdate time.Time
	wait := 2 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if a.probation() && a.healthy(ctx, "http://"+ask+"/healthz") {
			a.confirm()
		}
		status, err := a.ask(ctx, "http://"+ask+"/admin/api/status")
		if err != nil || status.Database != "ready" {
			wait = 2 * time.Second
			a.update(func(s *Status) {
				if s.Phase == Running {
					s.Phase = Starting
				}
			})
			if err == nil && !db.Running(ctx) && ctx.Err() == nil {
				a.logger.Warn("The database stopped by itself: starting it again")
				if err := db.Start(ctx); err != nil && ctx.Err() == nil {
					a.logger.Error("The database could not start again", "error", err)
				}
			}
			continue
		}
		wait = 10 * time.Second
		a.update(func(s *Status) {
			if s.Phase == Starting {
				s.Phase = Running
			}
			s.SetupRequired = status.SetupRequired
			if !status.SetupRequired {
				s.SetupCode = ""
			}
		})
		if status.SetupRequired {
			a.openSetup()
		} else {
			a.setUp()
		}
		if a.o.Updates != nil && time.Now().After(nextUpdate) {
			nextUpdate = time.Now().Add(time.Hour)
			a.checkUpdate(ctx, db.URL())
		}
	}
}

// healthy tells whether the server answers its /healthz at address.
func (a *App) healthy(ctx context.Context, address string) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return false
	}
	response, err := a.client.Do(request)
	if err != nil {
		return false
	}
	response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func (a *App) ask(ctx context.Context, address string) (adminStatus, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return adminStatus{}, err
	}
	response, err := a.client.Do(request)
	if err != nil {
		return adminStatus{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return adminStatus{}, fmt.Errorf("status %d", response.StatusCode)
	}
	var status adminStatus
	err = json.NewDecoder(io.LimitReader(response.Body, 1<<16)).Decode(&status)
	return status, err
}

// openSetup opens the admin app's setup in the browser, once per run of
// the app, unless the system started it.
func (a *App) openSetup() {
	a.mu.Lock()
	open := a.o.OpenSetup && !a.opened
	a.opened = true
	a.mu.Unlock()
	if !open {
		return
	}
	a.logger.Info("No administrator yet: opening the admin app's setup", "address", AdminURL())
	if err := a.o.Open(AdminURL()); err != nil {
		a.logger.Warn("The browser could not be opened", "error", err)
	}
}

// setUp turns Start with the computer on the first time an administrator
// exists, unless it was chosen before.
func (a *App) setUp() {
	if a.o.Login == nil {
		return
	}
	state := a.readState()
	if state.StartWithComputer != nil {
		return
	}
	a.logger.Info("Polyfin is set up: starting it with the computer")
	if err := a.SetStartWithComputer(true); err != nil {
		a.logger.Warn("Start with the computer could not be turned on", "error", err)
	}
}

// applyLoginItem writes the login item again at each start while it is
// on, so that it starts the app where it is now, and removes it while it
// is off.
func (a *App) applyLoginItem() {
	if a.o.Login == nil {
		return
	}
	state := a.readState()
	if state.StartWithComputer == nil {
		return
	}
	var err error
	if *state.StartWithComputer {
		err = a.o.Login.Enable()
	} else {
		err = a.o.Login.Disable()
	}
	if err != nil {
		a.logger.Warn("The login item could not be updated", "error", err)
	}
}

// SecretKey is the key of the data folder, from the system's key store,
// for the tray to copy: the server never sends it.
func (a *App) SecretKey() (string, error) {
	if a.o.Keys == nil {
		return "", errors.New("this run of the app has no key store")
	}
	return keystore.Read(a.o.Keys, keystore.Account(a.o.Folders.Data))
}

// SetStartWithComputer turns the login item on or off, and keeps the
// choice.
func (a *App) SetStartWithComputer(on bool) error {
	if a.o.Login == nil {
		return errors.New("this run of the app leaves the login item alone")
	}
	var err error
	if on {
		err = a.o.Login.Enable()
	} else {
		err = a.o.Login.Disable()
	}
	if err != nil {
		return err
	}
	state := a.readState()
	state.StartWithComputer = &on
	if err := a.writeState(state); err != nil {
		return err
	}
	a.update(func(s *Status) { s.StartWithComputer = on })
	return nil
}

// checkUpdate reads what the server's check found, and offers its
// package.
func (a *App) checkUpdate(ctx context.Context, databaseURL string) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return
	}
	found, err := update.ReadFound(ctx, conn)
	_ = conn.Close(ctx)
	if err != nil {
		a.logger.Debug("What the check for a new version found could not be read", "error", err)
		return
	}
	current := a.Status().Offer
	if current != nil && current.Version == found.Latest && found.Enabled {
		return
	}
	// The version the app went back from is not offered again.
	skip := ""
	if back, err := desktop.ReadRolledBack(a.o.Folders); err == nil && back != nil {
		skip = back.Version
	}
	offer, err := a.o.Updates.Offer(ctx, found, skip)
	if err != nil {
		a.logger.Warn("The new version's package could not be found, or is refused", "version", found.Latest, "error", err)
		return
	}
	if offer != nil && (current == nil || current.Version != offer.Version) {
		a.logger.Info("A new version of Polyfin Desktop can be installed", "version", offer.Version)
	}
	a.update(func(s *Status) { s.Offer = offer })
}

// InstallUpdate downloads the offered release, stops Polyfin, and replaces
// the app with it. It returns what to start once the app has quit, nil
// when nothing is. On a failure, Polyfin runs on as it was, and the menu
// tells that the update could not be installed.
func (a *App) InstallUpdate(ctx context.Context) (*exec.Cmd, error) {
	offer := a.Status().Offer
	if offer == nil || a.o.Updates == nil {
		return nil, errors.New("no update is offered")
	}
	if err := update.CanInstall(a.o.Updates.Kind, a.o.Executable); err != nil {
		a.failUpdate(err)
		return nil, err
	}
	if err := a.o.Updates.Current(*offer); err != nil {
		a.failUpdate(err)
		return nil, err
	}
	a.logger.Info("Downloading the new version", "version", offer.Version)
	a.update(func(s *Status) { s.Phase, s.Percent = Updating, 0 })
	file, err := update.Download(ctx, a.o.Updates.Client, *offer, a.o.Folders.Updates(), func(done, total int64) {
		if total > 0 {
			percent := int(done * 100 / total)
			a.update(func(s *Status) { s.Percent = percent })
		}
	})
	if err != nil {
		a.failUpdate(err)
		return nil, err
	}
	a.Stop(ctx)
	// The database as this version leaves it is kept until the new one
	// answers: once migrated, it would not start this one again. A .deb,
	// which the system's package installer replaces, keeps no previous
	// app to go back to.
	kind := a.o.Updates.Kind
	database := ""
	if kind != update.Deb {
		database = a.o.Folders.PreviousDatabase()
		if err := update.CopyTree(a.o.Folders.Cluster(), database); err != nil {
			err = fmt.Errorf("keep a copy of the database: %w", err)
			a.Start()
			a.failUpdate(err)
			return nil, err
		}
	}
	installed, err := update.Install(ctx, file, kind, a.o.Executable)
	if err != nil {
		if database != "" {
			_ = os.RemoveAll(database)
		}
		// Starting clears the problems: the failure is told after it.
		a.Start()
		a.failUpdate(err)
		return nil, err
	}
	if installed.Previous != "" {
		pending := update.Pending{From: a.o.Version, To: offer.Version, Kind: kind, App: installed.App, Previous: installed.Previous,
			Database: database}
		if err := pending.Write(a.o.Folders); err != nil {
			a.logger.Warn("The previous version is not kept: the new one cannot go back to it", "error", err)
			_ = pending.Confirm(a.o.Folders, a.o.Executable)
		}
	}
	a.logger.Info("The new version is installed: starting it", "version", offer.Version)
	return installed.Start, nil
}

func (a *App) failUpdate(err error) {
	a.logger.Error("The new version could not be installed", "error", err)
	a.problem(UpdateFailed, err)
	a.update(func(s *Status) {
		if s.Phase == Updating {
			s.Phase, s.Percent = Running, 0
		}
	})
}

// state is the app's own choices, in the data folder.
type state struct {
	// StartWithComputer is nil until set up or chosen.
	StartWithComputer *bool `json:"startWithComputer,omitempty"`
}

func (a *App) readState() state {
	var s state
	if data, err := os.ReadFile(a.o.Folders.State()); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	return s
}

func (a *App) writeState(s state) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	temporary := a.o.Folders.State() + ".new"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, a.o.Folders.State())
}

// serverProcess is polyfin serve, started again when it ends by itself.
type serverProcess struct {
	app *App
	env []string

	mu      sync.Mutex
	process *os.Process
	exited  chan struct{}
	// exit is why the server last ended, or could not start.
	exit error
}

// lastExit is why the server last ended, or could not start.
func (p *serverProcess) lastExit() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exit
}

// Restarts wait from minDelay, doubling up to maxDelay; a server that ran
// for stableRun starts again at minDelay.
const (
	minDelay  = 2 * time.Second
	maxDelay  = time.Minute
	stableRun = time.Minute
	// stopWait is how long the server has to stop: its own shutdown
	// takes up to 15 seconds.
	stopWait = 30 * time.Second
	// outputWait bounds the wait for the server's last output once it has
	// ended.
	outputWait = 5 * time.Second
)

// supervise starts the server, and again each time it ends by itself,
// until ctx ends. It returns true, without starting it, once a new
// version's server started update.MaxStarts times without answering: the
// app goes back to the previous version.
func (p *serverProcess) supervise(ctx context.Context) bool {
	delay := minDelay
	for ctx.Err() == nil {
		if !p.app.countStart() {
			return true
		}
		started := time.Now()
		exited, err := p.start()
		if err != nil {
			p.app.logger.Error("The server could not start", "error", err)
			p.app.problem(StartFailed, err)
			p.mu.Lock()
			p.exit = err
			p.mu.Unlock()
		} else {
			select {
			case <-ctx.Done():
				return false
			case <-exited:
			}
			p.app.logger.Warn("The server stopped by itself: starting it again", "after", delay)
			p.app.update(func(s *Status) {
				if s.Phase == Running {
					s.Phase = Starting
				}
			})
		}
		if !p.app.mayStartAgain() {
			return true
		}
		if time.Since(started) > stableRun {
			delay = minDelay
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(delay):
		}
		delay = min(delay*2, maxDelay)
	}
	return false
}

// setupCode finds the code in the line the server logs while no
// administrator exists.
var setupCode = regexp.MustCompile(`\bsetup_code=([0-9A-Z]{4}-[0-9A-Z]{4})\b`)

// start starts the server, its output going to the log folder.
func (p *serverProcess) start() (<-chan struct{}, error) {
	o := p.app.o
	logFile, err := os.OpenFile(filepath.Join(o.Folders.Log(), "polyfin.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		logFile.Close()
		return nil, err
	}
	cmd := desktop.Command(context.Background(), o.Install.Server(), "serve")
	cmd.Env, cmd.Dir, cmd.Stdout, cmd.Stderr = p.env, o.Folders.Data, writer, writer
	prepareServer(cmd)
	err = cmd.Start()
	writer.Close()
	if err != nil {
		reader.Close()
		logFile.Close()
		return nil, err
	}
	if err := adopt(cmd.Process); err != nil {
		p.app.logger.Warn("The server could not be tied to the app", "error", err)
	}
	writePID(o.Folders.ServerPID(), cmd.Process.Pid)
	p.app.logger.Info("The server started", "pid", cmd.Process.Pid)
	copied := make(chan struct{})
	go func() {
		defer close(copied)
		defer logFile.Close()
		defer reader.Close()
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 64<<10), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			_, _ = io.WriteString(logFile, line+"\n")
			if m := setupCode.FindStringSubmatch(line); m != nil {
				code := m[1]
				p.app.update(func(s *Status) { s.SetupCode, s.SetupRequired = code, true })
			}
		}
	}()
	exited := make(chan struct{})
	p.mu.Lock()
	p.process, p.exited = cmd.Process, exited
	p.mu.Unlock()
	go func() {
		err := cmd.Wait()
		// The rest of the server's output, unless a process it left
		// behind holds the pipe: the copy then goes on alone.
		select {
		case <-copied:
		case <-time.After(outputWait):
			p.app.logger.Warn("A process the server started still holds its output")
		}
		p.app.logger.Info("The server ended", "status", err)
		p.mu.Lock()
		p.exit = err
		p.mu.Unlock()
		close(exited)
	}()
	return exited, nil
}

// stop asks the server to stop, waits, and kills it when it does not stop
// in time.
func (p *serverProcess) stop() {
	p.mu.Lock()
	process, exited := p.process, p.exited
	p.mu.Unlock()
	if process == nil {
		return
	}
	select {
	case <-exited:
	default:
		p.app.logger.Info("Stopping the server", "pid", process.Pid)
		if err := interrupt(process); err != nil {
			p.app.logger.Warn("The server could not be asked to stop: stopping it at once", "error", err)
			_ = kill(process)
		}
		select {
		case <-exited:
		case <-time.After(stopWait):
			p.app.logger.Warn("The server did not stop in time: stopping it at once")
			_ = kill(process)
			<-exited
		}
	}
	os.Remove(p.app.o.Folders.ServerPID())
}

// writePID records the server's process and when it started, which tells
// it from a process that took its number later.
func writePID(path string, pid int) {
	start, ok := startTime(pid)
	if !ok {
		return
	}
	_ = os.WriteFile(path, []byte(strconv.Itoa(pid)+" "+strconv.FormatInt(start, 10)+"\n"), 0o600)
}

// stopLeftover stops the server a crash of the app left running: the
// process of the PID file, while it started when the file says.
func stopLeftover(path string, logger *slog.Logger) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	defer os.Remove(path)
	fields := strings.Fields(string(data))
	if len(fields) != 2 {
		return
	}
	pid, err1 := strconv.Atoi(fields[0])
	recorded, err2 := strconv.ParseInt(fields[1], 10, 64)
	if err1 != nil || err2 != nil {
		return
	}
	if start, ok := startTime(pid); !ok || start != recorded {
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	logger.Warn("The server was left running: stopping it", "pid", pid)
	_ = interrupt(process)
	deadline := time.Now().Add(stopWait)
	for alive(pid) && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
	if alive(pid) {
		_ = kill(process)
	}
}

// RotateLogs keeps each log of the log folder under maxLog: a larger one
// is renamed with .1, replacing the one before. It runs at start.
func RotateLogs(dir string) {
	const maxLog = 10 << 20
	for _, name := range []string{"desktop.log", "polyfin.log", "postgresql.log"} {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && info.Size() > maxLog {
			_ = os.Rename(path, path+".1")
		}
	}
}
