// Package loginitem starts Polyfin Desktop with the computer, as each
// system does it for the user: a launch agent on macOS, a value of the
// Run key of the user's registry on Windows, an autostart entry on Linux.
// The app starts then with Arg, in the background.
package loginitem

import (
	"bytes"
	"encoding/xml"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Arg is the argument the system starts the app with: no browser opens.
const Arg = "--background"

// Label names the item: the launch agent's label on macOS, the Run value
// on Windows, the autostart file on Linux.
const Label = "io.github.moodiness.polyfin-desktop"

// Item is the login item of the app.
type Item interface {
	// Enabled tells whether the system starts the app with the computer.
	Enabled() (bool, error)
	Enable() error
	Disable() error
}

// command is how the system starts the executable exe: on macOS, a
// bundle is opened as an app, as the Finder does, and an AppImage's own
// file is run rather than the folder it mounts.
func command(goos, exe string, env func(string) string) []string {
	if goos == "darwin" {
		if bundle := bundleOf(exe); bundle != "" {
			return []string{"/usr/bin/open", "-a", bundle, "--args", Arg}
		}
	}
	if goos == "linux" {
		if image := env("APPIMAGE"); image != "" {
			exe = image
		}
	}
	return []string{exe, Arg}
}

// bundleOf is the macOS bundle of exe, a path with slashes.
func bundleOf(exe string) string {
	macos := path.Dir(exe)
	contents := path.Dir(macos)
	if path.Base(macos) == "MacOS" && path.Base(contents) == "Contents" && strings.HasSuffix(path.Dir(contents), ".app") {
		return path.Dir(contents)
	}
	return ""
}

// LaunchAgent is the launch agent macOS starts the app with at login,
// once: RunAtLoad without KeepAlive, so that quitting the app stops it.
func LaunchAgent(args []string) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + escape(Label) + `</string>
	<key>ProgramArguments</key>
	<array>
`)
	for _, arg := range args {
		b.WriteString("\t\t<string>" + escape(arg) + "</string>\n")
	}
	b.WriteString(`	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`)
	return b.Bytes()
}

func escape(text string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(text))
	return b.String()
}

// DesktopEntry is the autostart entry of the XDG specification Linux
// desktops start the app with at login.
func DesktopEntry(args []string) []byte {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = quoteExec(arg)
	}
	return []byte(`[Desktop Entry]
Type=Application
Name=Polyfin
Comment=Start Polyfin with the computer
Exec=` + strings.Join(quoted, " ") + `
Terminal=false
X-GNOME-Autostart-enabled=true
`)
}

// quoteExec quotes an argument of an Exec key: in double quotes when it
// holds a reserved character, with ", `, $ and \ escaped, and % doubled.
func quoteExec(arg string) string {
	arg = strings.ReplaceAll(arg, "%", "%%")
	if !strings.ContainsAny(arg, " \t\n\"'\\><~|&;$*?#()`") {
		return arg
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range arg {
		if strings.ContainsRune("\"`$\\", r) {
			// The value of a desktop entry unescapes \\ once more.
			b.WriteString(`\\`)
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

// RunValue is the command line of the value Windows starts the app with
// at sign-in, its executable quoted.
func RunValue(args []string) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		if i == 0 || strings.ContainsAny(arg, " \t") {
			arg = `"` + arg + `"`
		}
		parts[i] = arg
	}
	return strings.Join(parts, " ")
}

// LaunchAgentPath is where macOS reads the launch agent from. macOS and
// Linux paths are built with slashes whatever the host, as tests of every
// system run everywhere.
func LaunchAgentPath(home string) string {
	return path.Join(home, "Library", "LaunchAgents", Label+".plist")
}

// AutostartPath is where Linux desktops read the autostart entry from.
func AutostartPath(home string, env func(string) string) string {
	config := env("XDG_CONFIG_HOME")
	if config == "" || !path.IsAbs(config) {
		config = path.Join(home, ".config")
	}
	return path.Join(config, "autostart", "polyfin-desktop.desktop")
}

// fileItem is a login item that is a file: macOS's and Linux's.
type fileItem struct {
	path    string
	content []byte
}

func (f fileItem) Enabled() (bool, error) {
	_, err := os.Stat(f.path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func (f fileItem) Enable() error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	temporary := f.path + ".new"
	if err := os.WriteFile(temporary, f.content, 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, f.path)
}

func (f fileItem) Disable() error {
	err := os.Remove(f.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
