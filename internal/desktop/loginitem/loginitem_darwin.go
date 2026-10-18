package loginitem

import "os"

// New is the launch agent that starts exe at login.
func New(exe string) (Item, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return fileItem{path: LaunchAgentPath(home), content: LaunchAgent(command("darwin", exe, os.Getenv))}, nil
}
