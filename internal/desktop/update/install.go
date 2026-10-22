package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/desktop"
)

// ErrNotPackaged is an app that runs from no package, such as a
// development build: it cannot replace itself.
var ErrNotPackaged = errors.New("this app was not installed from a package")

// CanInstall tells whether the app whose executable is exe can replace
// itself with a package of kind: ErrNotPackaged for a development build,
// which runs from no bundle nor AppImage.
func CanInstall(kind Kind, exe string) error {
	switch {
	case kind == DMG && desktop.Bundle(exe) == "":
		return ErrNotPackaged
	case kind == AppImage && os.Getenv("APPIMAGE") == "":
		return ErrNotPackaged
	}
	return nil
}

// Install installs the downloaded package pkg in place of the app whose
// executable is exe, once the server and the database have stopped. It
// returns what to start once the app has quit: the new app, or the
// system's package installer. The data folder is never touched.
//
//   - macOS: the bundle of the disk image replaces the app's bundle.
//   - Windows: the installer starts at once, its window shown as from a
//     download, and nothing is left to start: its wizard waits for a
//     click, and asks to quit Polyfin first if it still runs, so a
//     failure to start it is told while the app still runs.
//   - Linux: the new AppImage replaces the one the app runs from; a .deb
//     opens in the system's package installer.
func Install(ctx context.Context, pkg string, kind Kind, exe string) (*exec.Cmd, error) {
	switch kind {
	case DMG:
		bundle := desktop.Bundle(exe)
		if bundle == "" {
			return nil, ErrNotPackaged
		}
		if err := installDMG(ctx, pkg, bundle); err != nil {
			return nil, err
		}
		return desktop.WindowCommand(context.Background(), "/usr/bin/open", "-n", bundle), nil
	case Installer:
		cmd := desktop.WindowCommand(context.Background(), pkg)
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		_ = cmd.Process.Release()
		return nil, nil
	case AppImage:
		image := os.Getenv("APPIMAGE")
		if image == "" {
			return nil, ErrNotPackaged
		}
		if err := replaceFile(pkg, image); err != nil {
			return nil, err
		}
		return desktop.WindowCommand(context.Background(), image), nil
	default:
		return desktop.WindowCommand(context.Background(), "xdg-open", pkg), nil
	}
}

// installDMG copies the bundle of the disk image beside bundle, then
// swaps them.
func installDMG(ctx context.Context, pkg, bundle string) error {
	mount, err := os.MkdirTemp("", "polyfin-update-")
	if err != nil {
		return err
	}
	defer os.Remove(mount)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "hdiutil", "attach", "-nobrowse", "-readonly", "-noautoopen", "-mountpoint", mount, pkg).CombinedOutput(); err != nil {
		return fmt.Errorf("open the disk image: %w: %s", err, strings.TrimSpace(string(out)))
	}
	defer func() { _ = exec.Command("hdiutil", "detach", "-force", mount).Run() }()
	found, err := filepath.Glob(filepath.Join(mount, "*.app"))
	if err != nil || len(found) != 1 {
		return fmt.Errorf("the disk image holds %d apps, not one", len(found))
	}
	staged := bundle + ".new"
	if err := os.RemoveAll(staged); err != nil {
		return err
	}
	// ditto keeps the bundle's links, modes and extended attributes.
	if out, err := exec.CommandContext(ctx, "ditto", found[0], staged).CombinedOutput(); err != nil {
		os.RemoveAll(staged)
		return fmt.Errorf("copy the new app: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return swap(staged, bundle)
}

// swap puts staged in place of current, which is removed: renamed aside
// first, and put back when the new one cannot take its place.
func swap(staged, current string) error {
	aside := current + ".old"
	if err := os.RemoveAll(aside); err != nil {
		return err
	}
	if err := os.Rename(current, aside); err != nil {
		return err
	}
	if err := os.Rename(staged, current); err != nil {
		_ = os.Rename(aside, current)
		return err
	}
	return os.RemoveAll(aside)
}

// replaceFile puts the executable pkg in place of current, as a copy
// beside current renamed over it, as they may be on different disks.
func replaceFile(pkg, current string) error {
	staged := current + ".new"
	in, err := os.Open(pkg)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(staged, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		in.Close()
		return err
	}
	_, err = io.Copy(out, in)
	// Both are closed before the download is removed: Windows refuses to
	// remove an open file.
	in.Close()
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(staged)
		return err
	}
	if err := os.Chmod(staged, 0o755); err != nil {
		return err
	}
	if err := os.Rename(staged, current); err != nil {
		os.Remove(staged)
		return err
	}
	return os.Remove(pkg)
}
