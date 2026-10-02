//go:build production

// Package webui provides the built admin application.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var assets embed.FS

// Assets returns the admin application embedded in the production binary.
func Assets() (fs.FS, error) {
	return fs.Sub(assets, "dist")
}
