//go:build !darwin && !windows

package loginitem

import "os"

// New is the autostart entry that starts exe at login.
func New(exe string) (Item, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return fileItem{path: AutostartPath(home, os.Getenv), content: DesktopEntry(command("linux", exe, os.Getenv))}, nil
}
