package desktop

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
)

// The admin app installs the update the app offers, as the tray's Install
// Polyfin <version> does, through two files of the data folder, which is
// the user's alone, as polyfin-desktop -quit asks the app to quit. While
// the server runs, the app writes what it offers to Folders.Offer, which
// the server reads. To install it, the server writes the running app's
// token, read from Folders.Instance, and the version to
// Folders.InstallRequest, which the app looks for. A request that carries
// another app's token, left by a crash, installs nothing.

// ServerVariable is set to 1 in the environment of the server the app
// starts: the server then knows that the app starts it again when it
// exits, and offers what the app offers.
const ServerVariable = "POLYFIN_DESKTOP"

// ErrNoApp is a data folder no app runs on, as the server finds it.
var ErrNoApp = errors.New("Polyfin Desktop does not run on this data folder")

// Offered is the update the app offers, as the tray's menu shows it.
type Offered struct {
	Version string `json:"version"`
	// Notes is the address of the release notes.
	Notes string `json:"notes,omitempty"`
	// Installing tells that the app is downloading the update, Percent of
	// it done, before it stops Polyfin to install it.
	Installing bool `json:"installing,omitempty"`
	Percent    int  `json:"percent,omitempty"`
	// Failed tells that the last install failed: the app's log tells why.
	Failed bool `json:"failed,omitempty"`
}

// WriteOffer writes what the app offers, removing the file for nil.
func WriteOffer(f Folders, offered *Offered) error {
	if offered == nil {
		err := os.Remove(f.Offer())
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	data, err := json.Marshal(offered)
	if err != nil {
		return err
	}
	temporary := f.Offer() + ".new"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, f.Offer())
}

// ReadOffer reads what the app offers, nil when it offers nothing.
func ReadOffer(f Folders) (*Offered, error) {
	data, err := os.ReadFile(f.Offer())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var offered Offered
	if err := json.Unmarshal(data, &offered); err != nil {
		return nil, err
	}
	if offered.Version == "" {
		return nil, nil
	}
	return &offered, nil
}

// RequestInstall asks the app running on the data folder to install
// version, the update it offers: ErrNoApp when no app runs there. The app
// looks for the request twice a second, and ignores one for another
// version than the one it offers.
func RequestInstall(f Folders, version string) error {
	_, token, ok := ReadInstance(f.Instance())
	if !ok {
		return ErrNoApp
	}
	return os.WriteFile(f.InstallRequest(), []byte(token+" "+version+"\n"), 0o600)
}

// InstallRequested reads the request to install, removing it, and tells
// the version asked when it carries token, the running app's. A request
// that carries another token was left by a crash: it installs nothing.
func InstallRequested(f Folders, token string) (version string, ok bool) {
	data, err := os.ReadFile(f.InstallRequest())
	if err != nil {
		return "", false
	}
	if err := os.Remove(f.InstallRequest()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	fields := strings.Fields(string(data))
	if len(fields) != 2 || fields[0] != token {
		return "", false
	}
	return fields[1], true
}

// ReadInstance reads the record of the running app, Folders.Instance: its
// PID and token.
func ReadInstance(path string) (pid int, token string, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, "", false
	}
	fields := strings.Fields(string(data))
	if len(fields) != 2 {
		return 0, "", false
	}
	pid, err = strconv.Atoi(fields[0])
	if err != nil || pid <= 0 {
		return 0, "", false
	}
	return pid, fields[1], true
}
