package config

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The names of the folders the settings choose for the files Polyfin
// works on, as System › Health and the metrics name them.
const (
	ConversionFolder = "conversion"
	ReadingFolder    = "reading"
	CacheFolder      = "cache"
)

// Folders are the folders Polyfin keeps the files it works on in for a
// run, as the settings chose them at its start: Conversion holds the
// segments of conversions and repackagings, Reading the files being read,
// and Cache the relayed images, the guides and the picture subtitles being
// read, each in subfolders of its own (see Segments and the others).
type Folders struct {
	Conversion string
	Reading    string
	Cache      string
	// Unwritable are the folders the settings chose that could not be
	// written into at start: the cache in the data folder takes their
	// place meanwhile.
	Unwritable []UnwritableFolder
}

// UnwritableFolder is a folder the settings chose that Polyfin could not
// write into at start: Name is ConversionFolder, ReadingFolder or
// CacheFolder, and Err tells why.
type UnwritableFolder struct {
	Name   string
	Folder string
	Err    error
}

// ChooseFolders returns the folders of a run on a server keeping its files
// in dataDir: those settings chose, each checked as the recordings and
// backup folders are (see PrepareFolder), and the cache in the data folder
// for those it leaves empty or Polyfin cannot write into. A folder chosen
// is never created: it is where a disk or a memory folder is mounted.
func ChooseFolders(dataDir string, settings accounts.Settings) Folders {
	place := accounts.DefaultCacheFolder(dataDir)
	folders := Folders{Conversion: place, Reading: place, Cache: place}
	for _, f := range []struct {
		name, chosen string
		into         *string
	}{
		{ConversionFolder, settings.ConversionFolder, &folders.Conversion},
		{ReadingFolder, settings.ReadingFolder, &folders.Reading},
		{CacheFolder, settings.CacheFolder, &folders.Cache},
	} {
		if f.chosen == "" {
			continue
		}
		if err := PrepareFolder(f.chosen, false); err != nil {
			folders.Unwritable = append(folders.Unwritable, UnwritableFolder{Name: f.name, Folder: f.chosen, Err: err})
			continue
		}
		*f.into = f.chosen
	}
	return folders
}

// Segments is the folder of the segments of conversions and
// repackagings, emptied at start.
func (f Folders) Segments() string { return filepath.Join(f.Conversion, "segments") }

// Sources is the folder of the blocks of the addons' files being read,
// emptied at start.
func (f Folders) Sources() string { return filepath.Join(f.Reading, "sources") }

// Torrents is the folder of the torrents' pieces, emptied at start.
func (f Folders) Torrents() string { return filepath.Join(f.Reading, "torrents") }

// Images is the folder of the relayed images, kept across starts.
func (f Folders) Images() string { return filepath.Join(f.Cache, "images") }

// Guides is the folder of the guides being read, emptied at start.
func (f Folders) Guides() string { return filepath.Join(f.Cache, "guides") }

// OCR is the folder of the picture subtitles being read into text,
// emptied at start.
func (f Folders) OCR() string { return filepath.Join(f.Cache, "ocr") }

// subfolders are the folders of the run, each in one of f's.
func (f Folders) subfolders() []string {
	return []string{f.Segments(), f.Sources(), f.Torrents(), f.Images(), f.Guides(), f.OCR()}
}

// ClearUnused removes the folders of the cache in the data folder dataDir
// that the run leaves to folders chosen elsewhere: a folder used before
// is emptied only by the runs that use it, and the relayed images are
// kept across starts, so what a former run left there would stay. One
// that a folder of the run is within, or that is within one the run
// writes to, is kept. The folders the settings chose before are left as
// they are: they are the administrator's.
func (f Folders) ClearUnused(dataDir string) error {
	written := f.subfolders()
	place := accounts.DefaultCacheFolder(dataDir)
	var errs []error
	for _, unused := range (Folders{Conversion: place, Reading: place, Cache: place}).subfolders() {
		if slices.ContainsFunc(written, func(dir string) bool { return within(dir, unused) || within(unused, dir) }) ||
			slices.ContainsFunc([]string{f.Conversion, f.Reading, f.Cache}, func(dir string) bool { return within(dir, unused) }) {
			continue
		}
		errs = append(errs, os.RemoveAll(unused))
	}
	return errors.Join(errs...)
}

// Disks are the folders whose disks System › Health and the metrics
// measure, by name: the cache, then the conversion folder and the folder
// for files being read when each is another folder than those before it.
func (f Folders) Disks() []NamedFolder {
	disks := []NamedFolder{{CacheFolder, f.Cache}}
	for _, folder := range []NamedFolder{{ConversionFolder, f.Conversion}, {ReadingFolder, f.Reading}} {
		if !slices.ContainsFunc(disks, func(d NamedFolder) bool { return filepath.Clean(d.Path) == filepath.Clean(folder.Path) }) {
			disks = append(disks, folder)
		}
	}
	return disks
}

// NamedFolder is one of the Folders, by its name.
type NamedFolder struct {
	Name string
	Path string
}

// within reports whether dir is parent or one of its subfolders.
func within(dir, parent string) bool {
	rel, err := filepath.Rel(parent, dir)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
