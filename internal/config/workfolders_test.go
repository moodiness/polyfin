package config

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/workfolder"
)

// The folders the settings choose hold, from the start that reads them,
// the segments, the files being read and the cache, each in its
// subfolders; without one, the cache in the data folder holds them as
// before.
func TestChosenFoldersAreUsed(t *testing.T) {
	t.Parallel()
	dataDir, conversion, reading, cache := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	place := filepath.Join(dataDir, "cache")
	folders := ChooseFolders(dataDir, accounts.Settings{})
	if folders.Conversion != place || folders.Reading != place || folders.Cache != place || folders.Unwritable != nil {
		t.Errorf("none chosen: %+v", folders)
	}
	folders = ChooseFolders(dataDir, accounts.Settings{ConversionFolder: conversion, ReadingFolder: reading, CacheFolder: cache})
	if folders.Unwritable != nil {
		t.Fatalf("unwritable: %+v", folders.Unwritable)
	}
	for got, want := range map[string]string{
		folders.Segments(): filepath.Join(conversion, "segments"),
		folders.Sources():  filepath.Join(reading, "sources"),
		folders.Torrents(): filepath.Join(reading, "torrents"),
		folders.Images():   filepath.Join(cache, "images"),
		folders.Guides():   filepath.Join(cache, "guides"),
		folders.OCR():      filepath.Join(cache, "ocr"),
	} {
		if got != want {
			t.Errorf("%s, want %s", got, want)
		}
	}
	if got, want := folders.Disks(), []NamedFolder{{"cache", cache}, {"conversion", conversion}, {"reading", reading}}; !slices.Equal(got, want) {
		t.Errorf("disks %+v, want %+v", got, want)
	}
	// One disk is measured once, under the first name it has.
	folders = ChooseFolders(dataDir, accounts.Settings{ConversionFolder: conversion})
	if got, want := folders.Disks(), []NamedFolder{{"cache", place}, {"conversion", conversion}}; !slices.Equal(got, want) {
		t.Errorf("disks %+v, want %+v", got, want)
	}
}

// A folder the settings chose that cannot be written into at start, as
// one that went missing or a file, is told, and the cache in the data
// folder takes its place; it is not created.
func TestUnwritableFolderFallsBack(t *testing.T) {
	t.Parallel()
	dataDir, cache := t.TempDir(), t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing")
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	folders := ChooseFolders(dataDir, accounts.Settings{ConversionFolder: missing, ReadingFolder: file, CacheFolder: cache})
	place := filepath.Join(dataDir, "cache")
	if folders.Conversion != place || folders.Reading != place || folders.Cache != cache {
		t.Errorf("folders: %+v", folders)
	}
	if len(folders.Unwritable) != 2 || folders.Unwritable[0].Name != ConversionFolder || folders.Unwritable[0].Folder != missing ||
		folders.Unwritable[0].Err == nil || folders.Unwritable[1].Name != ReadingFolder || folders.Unwritable[1].Folder != file {
		t.Errorf("unwritable: %+v", folders.Unwritable)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("the missing folder was made: %v", err)
	}
}

// At start, what the data folder's cache kept for a folder now chosen
// elsewhere is removed, the relayed images that starts keep included; the
// places still in use, and the folders chosen before, are kept.
func TestUnusedPlacesAreCleared(t *testing.T) {
	t.Parallel()
	dataDir, former, cache := t.TempDir(), t.TempDir(), t.TempDir()
	keep := func(path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("kept"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	place := filepath.Join(dataDir, "cache")
	for _, name := range []string{"images", "guides", "segments", "sources"} {
		keep(filepath.Join(place, name, "file"))
	}
	keep(filepath.Join(former, "images", "file"))
	folders := ChooseFolders(dataDir, accounts.Settings{CacheFolder: cache})
	if err := folders.ClearUnused(dataDir); err != nil {
		t.Fatal(err)
	}
	for path, kept := range map[string]bool{
		filepath.Join(place, "images", "file"):   false,
		filepath.Join(place, "guides", "file"):   false,
		filepath.Join(place, "segments", "file"): true,
		filepath.Join(place, "sources", "file"):  true,
		filepath.Join(former, "images", "file"):  true,
	} {
		if _, err := os.Stat(path); (err == nil) != kept {
			t.Errorf("%s: kept %v, want %v", path, err == nil, kept)
		}
	}

	// A folder chosen within a place of the data folder's cache keeps it.
	within := filepath.Join(place, "segments", "chosen")
	if err := os.MkdirAll(within, 0o700); err != nil {
		t.Fatal(err)
	}
	folders = ChooseFolders(dataDir, accounts.Settings{ConversionFolder: within, CacheFolder: cache})
	if err := folders.ClearUnused(dataDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(place, "segments", "file")); err != nil {
		t.Errorf("the place holding the folder chosen was removed: %v", err)
	}
}

// emptiedPlaces are the folders emptied at start, each with the setting
// whose folder holds it and a file a version before the marker wrote
// there.
var emptiedPlaces = []struct {
	name, setting string
	choose        func(folder string) accounts.Settings
	used          func(Folders) string
	earlier       string
}{
	{"segments", ConversionFolder, func(folder string) accounts.Settings { return accounts.Settings{ConversionFolder: folder} },
		func(f Folders) string { return f.Conversion }, "encoding-123456/0.ts"},
	{"torrents", ReadingFolder, func(folder string) accounts.Settings { return accounts.Settings{ReadingFolder: folder} },
		func(f Folders) string { return f.Reading }, "0123456789abcdef0123456789abcdef01234567/0"},
	{"guides", CacheFolder, func(folder string) accounts.Settings { return accounts.Settings{CacheFolder: folder} },
		func(f Folders) string { return f.Cache }, "guide-123456.zip"},
	{"ocr", CacheFolder, func(folder string) accounts.Settings { return accounts.Settings{CacheFolder: folder} },
		func(f Folders) string { return f.Cache }, "polyfin-ocr-123456/1.png"},
}

// keepFile writes a file at path, with its folders.
func keepFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// start empties the folders of a run as cmd/polyfin's serve does when
// Polyfin starts: the segments' through the manager of conversions, and
// each of them as the torrents' store, the guides' reader and serve
// itself do.
func start(t *testing.T, dataDir string, folders Folders) {
	t.Helper()
	if err := folders.MarkOwn(dataDir); err != nil {
		t.Fatal(err)
	}
	manager, err := hls.NewManager("ffmpeg-not-installed", folders.Segments(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	manager.Close()
	for _, e := range emptied {
		if err := e.kind.Empty(e.dir(folders)); err != nil {
			t.Fatal(err)
		}
	}
}

// holds lists what dir holds.
func holds(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// A folder the settings choose may already hold, under the name of a
// folder Polyfin empties at start, files that are the administrator's:
// they are left as they are, the setting is told, and the cache in the
// data folder is used for it meanwhile.
func TestAChosenFolderKeepsWhatPolyfinDidNotMake(t *testing.T) {
	t.Parallel()
	for _, p := range emptiedPlaces {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			dataDir, chosen := t.TempDir(), t.TempDir()
			place := filepath.Join(dataDir, "cache")
			kept := filepath.Join(chosen, p.name, "holidays", "2019.mkv")
			keepFile(t, kept)
			// Saving the setting, then the start: cmd/polyfin's serve.
			if err := PrepareFolder(chosen, false); err != nil {
				t.Fatal(err)
			}
			folders := ChooseFolders(dataDir, p.choose(chosen))
			if len(folders.Occupied) != 1 || folders.Occupied[0] != (OccupiedFolder{Name: p.setting, Folder: chosen, Holds: p.name}) || folders.Unwritable != nil {
				t.Errorf("told: %+v", folders)
			}
			if p.used(folders) != place {
				t.Errorf("the folder used is %s, want %s", p.used(folders), place)
			}
			start(t, dataDir, folders)
			if _, err := os.Stat(kept); err != nil {
				t.Fatalf("after the start, the administrator's file is gone: %v", err)
			}
			if got := holds(t, filepath.Join(place, p.name)); len(got) != 1 || got[0] != workfolder.Marker {
				t.Errorf("the data folder's %s holds %v", p.name, got)
			}
			// Given the folder chosen all the same, a start refuses it.
			if p.name == "segments" {
				if _, err := hls.NewManager("ffmpeg-not-installed", filepath.Join(chosen, p.name), slog.New(slog.DiscardHandler)); !errors.Is(err, workfolder.ErrOccupied) {
					t.Errorf("a manager of conversions given the folder: %v", err)
				}
			}
			for _, e := range emptied {
				if err := e.kind.Empty(filepath.Join(chosen, p.name)); !errors.Is(err, workfolder.ErrOccupied) {
					t.Errorf("emptying the folder: %v", err)
				}
			}
			if _, err := os.Stat(kept); err != nil {
				t.Errorf("the administrator's file is gone: %v", err)
			}
			if got := holds(t, filepath.Join(chosen, p.name)); len(got) != 1 || got[0] != "holidays" {
				t.Errorf("the folder chosen holds %v under %s", got, p.name)
			}
		})
	}
}

// A folder Polyfin made in the folder chosen is emptied at each start as
// before: it gets its marker when it is made, and what a run left there
// is removed by the next.
func TestAChosenFolderPolyfinMadeIsStillEmptied(t *testing.T) {
	t.Parallel()
	for _, p := range emptiedPlaces {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			dataDir, chosen := t.TempDir(), t.TempDir()
			folders := ChooseFolders(dataDir, p.choose(chosen))
			if folders.Occupied != nil || p.used(folders) != chosen {
				t.Fatalf("a folder chosen that holds nothing: %+v", folders)
			}
			start(t, dataDir, folders)
			made := filepath.Join(chosen, p.name)
			if got := holds(t, made); len(got) != 1 || got[0] != workfolder.Marker {
				t.Fatalf("the folder made holds %v", got)
			}
			// A run leaves files of any name; the next start removes them.
			keepFile(t, filepath.Join(made, "of the last run", "file"))
			keepFile(t, filepath.Join(made, "notes.txt"))
			folders = ChooseFolders(dataDir, p.choose(chosen))
			if folders.Occupied != nil || p.used(folders) != chosen {
				t.Fatalf("at the next start: %+v", folders)
			}
			start(t, dataDir, folders)
			if got := holds(t, made); len(got) != 1 || got[0] != workfolder.Marker {
				t.Errorf("after the next start it holds %v", got)
			}
		})
	}
}

// At the first start after an upgrade, the folders an earlier version
// made in the folder chosen have no marker. One that holds nothing but
// what that version wrote there is marked and emptied; one that holds
// another file beside is left as it is, and told.
func TestAChosenFolderOfAnEarlierVersionIsMarkedAtTheUpgrade(t *testing.T) {
	t.Parallel()
	for _, p := range emptiedPlaces {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			dataDir, chosen := t.TempDir(), t.TempDir()
			made := filepath.Join(chosen, p.name)
			keepFile(t, filepath.Join(made, filepath.FromSlash(p.earlier)))
			folders := ChooseFolders(dataDir, p.choose(chosen))
			if folders.Occupied != nil || p.used(folders) != chosen {
				t.Fatalf("a folder of an earlier version: %+v", folders)
			}
			start(t, dataDir, folders)
			if got := holds(t, made); len(got) != 1 || got[0] != workfolder.Marker {
				t.Errorf("after the upgrade it holds %v", got)
			}

			other := t.TempDir()
			left := filepath.Join(other, p.name, filepath.FromSlash(p.earlier))
			mine := filepath.Join(other, p.name, "notes.txt")
			keepFile(t, left)
			keepFile(t, mine)
			folders = ChooseFolders(dataDir, p.choose(other))
			if len(folders.Occupied) != 1 || folders.Occupied[0] != (OccupiedFolder{Name: p.setting, Folder: other, Holds: p.name}) {
				t.Errorf("with another file, told: %+v", folders.Occupied)
			}
			if p.used(folders) != filepath.Join(dataDir, "cache") {
				t.Errorf("with another file, the folder used is %s", p.used(folders))
			}
			start(t, dataDir, folders)
			for _, path := range []string{left, mine} {
				if _, err := os.Stat(path); err != nil {
					t.Errorf("with another file: %v", err)
				}
			}
			if _, err := os.Stat(filepath.Join(other, p.name, workfolder.Marker)); !os.IsNotExist(err) {
				t.Errorf("with another file, the folder was marked: %v", err)
			}
		})
	}
}

// The cache in the data folder is Polyfin's own: its folders are emptied
// at start whatever they hold, as every version did, also when a setting
// names that cache. Only its folders are marked so.
func TestTheDataFoldersCacheIsEmptiedWhateverItHolds(t *testing.T) {
	t.Parallel()
	dataDir, chosen := t.TempDir(), t.TempDir()
	place := filepath.Join(dataDir, "cache")
	for _, p := range emptiedPlaces {
		keepFile(t, filepath.Join(place, p.name, "of a version long gone", "file"))
	}
	for _, settings := range []accounts.Settings{{}, {ConversionFolder: place, ReadingFolder: place, CacheFolder: place}} {
		if folders := ChooseFolders(dataDir, settings); folders.Occupied != nil || folders.Unwritable != nil || folders.Conversion != place {
			t.Errorf("%+v: %+v", settings, folders)
		}
	}
	folders := ChooseFolders(dataDir, accounts.Settings{})
	start(t, dataDir, folders)
	for _, p := range emptiedPlaces {
		if got := holds(t, filepath.Join(place, p.name)); len(got) != 1 || got[0] != workfolder.Marker {
			t.Errorf("the data folder's %s holds %v", p.name, got)
		}
	}

	kept := filepath.Join(chosen, "segments", "holidays", "2019.mkv")
	keepFile(t, kept)
	if err := (Folders{Conversion: chosen, Reading: place, Cache: place}).MarkOwn(dataDir); err != nil {
		t.Fatal(err)
	}
	if got := holds(t, filepath.Join(chosen, "segments")); len(got) != 1 || got[0] != "holidays" {
		t.Errorf("the folder chosen holds %v", got)
	}
}
