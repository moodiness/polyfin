package desktop

import (
	"encoding/json"
	"errors"
	"os"
	"time"
)

// RolledBack tells that the app went back to its previous version, as the
// new one's server stopped before it answered: the server shows it under
// System › Health, and the app no longer offers that version. The file is
// removed once a new version answers.
type RolledBack struct {
	// Version is the version that did not start, Previous the one put
	// back.
	Version  string `json:"version"`
	Previous string `json:"previous"`
	// Reason is why, as the app's log tells it.
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

// WriteRolledBack keeps r in the data folder.
func WriteRolledBack(f Folders, r RolledBack) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	temporary := f.RolledBack() + ".new"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, f.RolledBack())
}

// ReadRolledBack reads what WriteRolledBack kept, nil when nothing is.
func ReadRolledBack(f Folders) (*RolledBack, error) {
	data, err := os.ReadFile(f.RolledBack())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r RolledBack
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	if r.Version == "" {
		return nil, nil
	}
	return &r, nil
}

// RemoveRolledBack removes what WriteRolledBack kept.
func RemoveRolledBack(f Folders) error {
	err := os.Remove(f.RolledBack())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
