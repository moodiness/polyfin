package app

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/desktop"
)

// The quit command, polyfin-desktop -quit, asks the app running on a data
// folder to quit through two files of the folder, which is the user's
// alone. The app, once it holds the folder's lock, writes its PID and a
// random token to Folders.Instance, readable by the user only; the
// command writes that token to Folders.QuitRequest, which the app looks
// for. A request the app does not answer, left by a crash, carries
// another app's token, and quits no later one.

// ErrNotRunning is a data folder no app runs on.
var ErrNotRunning = errors.New("Polyfin Desktop does not run on this data folder")

// ErrStillRunning is an app that did not quit in the time given.
var ErrStillRunning = errors.New("Polyfin Desktop still runs on this data folder")

// quitPoll is how often the app looks for a request to quit, and the
// command for the app's end.
const quitPoll = 500 * time.Millisecond

// removeWait bounds how long the app tries again to remove its record,
// or a request it answered: Windows refuses to remove a file another
// process holds open, as the quit command does each time it reads the
// record, and as a file scanner may.
const removeWait = 2 * time.Second

// ListenQuit, called once the app holds the data folder's lock, records
// the app in Folders.Instance, forgets a request a crash left, and
// returns a channel closed when a request to quit comes. stop ends the
// listening and removes the record, before the lock is released.
func ListenQuit(folders desktop.Folders) (requested <-chan struct{}, stop func(), err error) {
	token := rand.Text()
	// A request left behind carries another app's token: removing it
	// only tidies up.
	_ = os.Remove(folders.QuitRequest())
	record := strconv.Itoa(os.Getpid()) + " " + token + "\n"
	if err := os.WriteFile(folders.Instance(), []byte(record), 0o600); err != nil {
		return nil, nil, err
	}
	quit := make(chan struct{})
	done := make(chan struct{})
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		ticker := time.NewTicker(quitPoll)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if quitRequested(folders, token) {
					close(quit)
					return
				}
			}
		}
	}()
	var once sync.Once
	return quit, func() {
		once.Do(func() {
			close(done)
			<-ended
			removeFile(folders.Instance())
		})
	}, nil
}

// quitRequested tells whether the request names the app of token, and
// removes it then. Any other request is ignored: the command writes the
// running app's token in its place.
func quitRequested(folders desktop.Folders, token string) bool {
	data, err := os.ReadFile(folders.QuitRequest())
	if err != nil || strings.TrimSpace(string(data)) != token {
		return false
	}
	removeFile(folders.QuitRequest())
	return true
}

// removeFile removes path, trying again for removeWait while it cannot,
// so that the app's files do not outlive it.
func removeFile(path string) {
	deadline := time.Now().Add(removeWait)
	for {
		err := os.Remove(path)
		if err == nil || errors.Is(err, os.ErrNotExist) || time.Now().After(deadline) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// RequestQuit asks the app running on the data folder to quit, as its
// Quit item does, and waits until its process has ended, or ctx is done:
// ErrNotRunning when no app runs there, ErrStillRunning when it still
// runs at the end.
func RequestQuit(ctx context.Context, folders desktop.Folders) error {
	free, err := lockFree(folders.Lock())
	if err != nil {
		return err
	}
	if free {
		return ErrNotRunning
	}
	pid, sent := 0, ""
	ticker := time.NewTicker(quitPoll)
	defer ticker.Stop()
	for {
		// The record is read again each time: an app that was starting
		// writes its own after taking the lock.
		if p, token, ok := desktop.ReadInstance(folders.Instance()); ok && token != sent {
			if err := os.WriteFile(folders.QuitRequest(), []byte(token+"\n"), 0o600); err != nil {
				return err
			}
			pid, sent = p, token
		}
		free, err := lockFree(folders.Lock())
		if err != nil {
			return err
		}
		if free {
			// The app releases the lock once the server and the database
			// stopped, and ends right after.
			for pid != 0 && running(pid) {
				select {
				case <-ctx.Done():
					return ErrStillRunning
				case <-ticker.C:
				}
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ErrStillRunning
		case <-ticker.C:
		}
	}
}

// ListenInstall, called once ListenQuit recorded the app, forgets a
// request to install that a crash left, and returns a channel that gets
// the version each request of the server asks to install (see
// desktop.RequestInstall). stop ends the listening.
func ListenInstall(folders desktop.Folders) (requested <-chan string, stop func(), err error) {
	_, token, ok := desktop.ReadInstance(folders.Instance())
	if !ok {
		return nil, nil, errors.New("the app's record cannot be read")
	}
	_ = os.Remove(folders.InstallRequest())
	install := make(chan string, 1)
	done := make(chan struct{})
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		ticker := time.NewTicker(quitPoll)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if version, ok := desktop.InstallRequested(folders, token); ok {
					// A request coming while the last one waits is the same.
					select {
					case install <- version:
					default:
					}
				}
			}
		}
	}()
	var once sync.Once
	return install, func() {
		once.Do(func() {
			close(done)
			<-ended
		})
	}, nil
}

// lockFree tells whether no app holds the lock of path, taking it for a
// moment. A missing file, or data folder, is held by none.
func lockFree(path string) (bool, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()
	err = lockFile(file)
	if errors.Is(err, ErrRunning) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// At once, for an app starting meanwhile: Windows may release the
	// lock of a closed file later.
	return true, unlockFile(file)
}
