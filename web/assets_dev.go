//go:build !production

// Package webui provides the built admin application.
package webui

import (
	"io/fs"
	"os"
)

// Assets returns the admin application built on disk by `make web`, so
// development builds pick up a rebuilt frontend without recompiling Go.
func Assets() (fs.FS, error) {
	return os.DirFS("web/dist"), nil
}
