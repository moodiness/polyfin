package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The folders the settings choose hold, from the start that reads them,
// the segments, the files being read and the cache, each in its
// subfolders; without one, the cache in the data folder holds them as
// before.
func TestChosenFoldersAreUsed(t *testing.T) {
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
