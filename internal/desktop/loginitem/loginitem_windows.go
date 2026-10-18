package loginitem

import (
	"errors"
	"os"

	"golang.org/x/sys/windows/registry"
)

// runKey is the key of the programs Windows starts at the user's sign-in.
const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// runItem is the value of the Run key that starts the app.
type runItem struct {
	value string
}

// New is the value of the Run key that starts exe at sign-in.
func New(exe string) (Item, error) {
	return runItem{value: RunValue(command("windows", exe, os.Getenv))}, nil
}

func (r runItem) Enabled() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer key.Close()
	_, _, err = key.GetStringValue(Label)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (r runItem) Enable() error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(Label, r.value)
}

func (r runItem) Disable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.DeleteValue(Label); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}
