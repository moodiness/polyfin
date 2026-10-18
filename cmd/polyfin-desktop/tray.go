//go:build !darwin || cgo

package main

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"fyne.io/systray"

	"github.com/moodiness/polyfin/internal/desktop"
	"github.com/moodiness/polyfin/internal/desktop/app"
)

// iconSize is the icon's width in pixels: macOS's menu bar is 22 points
// high, 44 pixels on a Retina display.
const iconSize = 44

// runTray shows the icon and its menu until Quit Polyfin, starting
// Polyfin at once. It returns what to start once the app has quit, after
// an update.
func runTray(options app.Options, logger *slog.Logger) *exec.Cmd {
	text := textsFor(desktop.Language())
	changes := make(chan app.Status, 1)
	options.Changed = func(s app.Status) {
		// Only the latest status matters: an older one waiting is dropped.
		select {
		case <-changes:
		default:
		}
		changes <- s
	}
	a := app.New(options)
	var relaunchMu sync.Mutex
	var relaunch *exec.Cmd
	var quitOnce sync.Once
	quitting := make(chan struct{})
	onReady := func() {
		setIcon(false)
		systray.SetTooltip("Polyfin")
		status := systray.AddMenuItem(text.starting, "")
		status.Disable()
		problems := systray.AddMenuItem("", "")
		problems.Disable()
		problems.Hide()
		setup := systray.AddMenuItem("", "")
		setup.Hide()
		systray.AddSeparator()
		openAdmin := systray.AddMenuItem(text.openAdmin, "")
		openWeb := systray.AddMenuItem(text.openWeb, "")
		openLogs := systray.AddMenuItem(text.openLogs, "")
		copyKey := systray.AddMenuItem(text.copyKey, text.keyHint)
		if options.Keys == nil {
			copyKey.Disable()
		}
		systray.AddSeparator()
		startStop := systray.AddMenuItem(text.stop, "")
		startWith := systray.AddMenuItemCheckbox(text.startWithComputer, "", a.Status().StartWithComputer)
		if options.Login == nil {
			startWith.Disable()
		}
		install := systray.AddMenuItem("", "")
		install.Hide()
		systray.AddSeparator()
		quitItem := systray.AddMenuItem(text.quit, "")

		var current app.Status
		show := func(s app.Status) {
			current = s
			status.SetTitle(text.phase(s))
			setIcon(s.Phase != app.Running)
			if lines := text.problems(s); len(lines) > 0 {
				problems.SetTitle(lines[len(lines)-1])
				problems.Show()
			} else {
				problems.Hide()
			}
			if s.SetupCode != "" && s.SetupRequired {
				setup.SetTitle(formatted(text.setupCode, s.SetupCode))
				setup.Show()
			} else {
				setup.Hide()
			}
			switch s.Phase {
			case app.Stopped, app.Failed:
				startStop.SetTitle(text.start)
				startStop.Enable()
			case app.Stopping, app.Updating:
				startStop.Disable()
			default:
				startStop.SetTitle(text.stop)
				startStop.Enable()
			}
			if s.StartWithComputer {
				startWith.Check()
			} else {
				startWith.Uncheck()
			}
			if s.Offer != nil && s.Phase == app.Running {
				install.SetTitle(formatted(text.install, s.Offer.Version))
				install.Show()
			} else {
				install.Hide()
			}
		}
		show(a.Status())
		a.Start()

		quitApp := func() { quitOnce.Do(func() { close(quitting); systray.Quit() }) }
		go func() {
			signals := make(chan os.Signal, 1)
			signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
			<-signals
			quitApp()
		}()
		go func() {
			for {
				select {
				case <-quitting:
					return
				case s := <-changes:
					show(s)
				case <-status.ClickedCh:
				case <-problems.ClickedCh:
					open(logger, options.Folders.Log())
				case <-setup.ClickedCh:
					if err := desktop.Copy(current.SetupCode); err != nil {
						logger.Warn("The setup code could not be copied", "error", err)
					}
					open(logger, app.AdminURL())
				case <-openAdmin.ClickedCh:
					open(logger, app.AdminURL())
				case <-openWeb.ClickedCh:
					open(logger, app.WebURL())
				case <-openLogs.ClickedCh:
					open(logger, options.Folders.Log())
				case <-copyKey.ClickedCh:
					copySecretKey(a, copyKey, text, logger)
				case <-startStop.ClickedCh:
					switch current.Phase {
					case app.Stopped, app.Failed:
						a.Start()
					default:
						startStop.Disable()
						go quit(a)
					}
				case <-startWith.ClickedCh:
					if err := a.SetStartWithComputer(!startWith.Checked()); err != nil {
						logger.Warn("Start with the computer could not be changed", "error", err)
					}
				case <-install.ClickedCh:
					install.Hide()
					go func() {
						cmd, err := a.InstallUpdate(context.Background())
						if err != nil {
							return
						}
						relaunchMu.Lock()
						relaunch = cmd
						relaunchMu.Unlock()
						quitApp()
					}()
				case <-quitItem.ClickedCh:
					status.SetTitle(text.stopping)
					quitApp()
				}
			}
		}()
	}
	// macOS calls onExit as it terminates the app, at logout: Polyfin
	// stops before it returns. Quit Polyfin ends Run without it.
	onExit := func() {
		quit(a)
	}
	systray.Run(onReady, onExit)
	quit(a)
	relaunchMu.Lock()
	defer relaunchMu.Unlock()
	return relaunch
}

// keyNoticeTime is how long the menu item says that the key was copied.
const keyNoticeTime = 10 * time.Second

// copySecretKey copies the key from the key store to the clipboard, the
// item saying for a while to keep it with the backups.
func copySecretKey(a *app.App, item *systray.MenuItem, text texts, logger *slog.Logger) {
	key, err := a.SecretKey()
	if err == nil {
		err = desktop.Copy(key)
	}
	if err != nil {
		logger.Warn("The secret key could not be copied", "error", err)
		return
	}
	logger.Info("The secret key was copied to the clipboard")
	item.SetTitle(text.keyCopied)
	time.AfterFunc(keyNoticeTime, func() { item.SetTitle(text.copyKey) })
}

// setIcon shows the logo, faded while Polyfin does not run.
func setIcon(faded bool) {
	if runtime.GOOS == "darwin" {
		data := icon(iconSize, true, faded)
		systray.SetTemplateIcon(data, data)
		return
	}
	data := icon(64, false, faded)
	if runtime.GOOS == "windows" {
		data = ico(data, 64)
	}
	systray.SetIcon(data)
}

func open(logger *slog.Logger, target string) {
	if err := desktop.Open(target); err != nil {
		logger.Warn("Could not open", "target", target, "error", err)
	}
}
