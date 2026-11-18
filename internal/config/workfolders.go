package config

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/workfolder"
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
	// Occupied are the folders the settings chose that held at start,
	// under the name of a folder Polyfin empties, files it did not make:
	// they are left as they are, and the cache in the data folder takes
	// their place meanwhile.
	Occupied []OccupiedFolder
}

// UnwritableFolder is a folder the settings chose that Polyfin could not
// write into at start: Name is ConversionFolder, ReadingFolder or
// CacheFolder, and Err tells why.
type UnwritableFolder struct {
	Name   string
	Folder string
	Err    error
}

// OccupiedFolder is a folder the settings chose where Polyfin found at
// start, under the name of a folder it empties, files it did not make:
// Name is ConversionFolder, ReadingFolder or CacheFolder, and Holds the
// name of that folder within Folder, as "torrents".
type OccupiedFolder struct {
	Name   string
	Folder string
	Holds  string
}

// emptied are the folders of a run that are emptied at its start, each
// in the folder of one setting, with what tells it from a folder Polyfin
// did not make.
var emptied = []struct {
	kind    workfolder.Kind
	setting string
	dir     func(Folders) string
}{
	{workfolder.Segments, ConversionFolder, Folders.Segments},
	{workfolder.Torrents, ReadingFolder, Folders.Torrents},
	{workfolder.Guides, CacheFolder, Folders.Guides},
	{workfolder.OCR, CacheFolder, Folders.OCR},
}

// ChooseFolders returns the folders of a run on a server keeping its files
// in dataDir: those settings chose, each checked as the recordings and
// backup folders are (see PrepareFolder), and the cache in the data folder
// for those it leaves empty or Polyfin cannot write into. A folder chosen
// is never created: it is where a disk or a memory folder is mounted. Nor
// is one emptied of what Polyfin did not make: where a folder emptied at
// start would take the place of an administrator's files (see
// workfolder.Kind.Check), the folder chosen is left as it is, and the
// cache in the data folder is used for it too.
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
		if held := occupied(f.name, f.chosen, place); held != nil {
			folders.Occupied = append(folders.Occupied, held...)
			continue
		}
		*f.into = f.chosen
	}
	return folders
}

// occupied returns, for the folder chosen for a setting, each folder
// emptied at start that would be one Polyfin did not make. The cache in
// the data folder, place, is Polyfin's own even when a setting names it
// (see MarkOwn).
func occupied(setting, chosen, place string) []OccupiedFolder {
	if filepath.Clean(chosen) == filepath.Clean(place) {
		return nil
	}
	there := Folders{Conversion: chosen, Reading: chosen, Cache: chosen}
	var held []OccupiedFolder
	for _, e := range emptied {
		if dir := e.dir(there); e.setting == setting && errors.Is(e.kind.Check(dir), workfolder.ErrOccupied) {
			held = append(held, OccupiedFolder{Name: setting, Folder: chosen, Holds: filepath.Base(dir)})
		}
	}
	return held
}

// Segments is the folder of the segments of conversions and
// repackagings, emptied at start.
func (f Folders) Segments() string { return filepath.Join(f.Conversion, "segments") }

// Sources is the folder of the blocks of the addons' files being read,
// kept across starts as the source cache's policy says (see
// source.Policy.Keep).
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

// MarkOwn marks as made by Polyfin the folders of the cache in the data
// folder dataDir that the run empties at its start: the data folder is
// Polyfin's own, so whatever one of them holds, a run before left it
// there, of whichever version, and it is emptied as it always was. In a
// folder the settings chose, such a folder is told from an
// administrator's by what it holds alone (see ChooseFolders).
func (f Folders) MarkOwn(dataDir string) error {
	place := filepath.Clean(accounts.DefaultCacheFolder(dataDir))
	var errs []error
	for _, e := range emptied {
		if dir := e.dir(f); filepath.Dir(dir) == place {
			errs = append(errs, workfolder.Mark(dir))
		}
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
