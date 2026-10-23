// Package webtheme holds the web player's default theme, LumaaGlaass,
// bundled with Polyfin (see third_party/lumaaglaass): the web client serves
// its files from Polyfin itself, and the default Custom CSS and JavaScript
// load them, so no browser loads the theme from elsewhere. Its files are
// refreshed from a commit of the theme's repository, with Polyfin's
// patches, by scripts/lumaaglaass.sh.
package webtheme

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
)

// Path is where the web client serves the theme, under /web/: a file at
// Path + Version + "/" + its name, such as assets/lumaaglaass.css.
const Path = "polyfin/themes/lumaaglaass/"

//go:embed lumaaglaass
var embedded embed.FS

// Files are the theme's files, by their path in its repository.
var Files = func() fs.FS {
	files, err := fs.Sub(embedded, "lumaaglaass")
	if err != nil {
		panic(err)
	}
	return files
}()

// Version names the content of Files: it changes with any of them, so
// that an address with it can be cached for good.
var Version = func() string {
	hash := sha256.New()
	err := fs.WalkDir(Files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := fs.ReadFile(Files, name)
		if err != nil {
			return err
		}
		hash.Write([]byte(name))
		hash.Write([]byte{0})
		hash.Write(data)
		hash.Write([]byte{0})
		return nil
	})
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(hash.Sum(nil)[:8])
}()

// Address is where the web page loads the theme's file name from: relative
// to the web client's address, as jellyfin-web's page is, /web/ or under
// the base path of Jellyfin.
func Address(name string) string {
	return Path + Version + "/" + name
}
