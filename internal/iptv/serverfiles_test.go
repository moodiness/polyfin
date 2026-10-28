package iptv

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/serverfile"
)

// writeList writes an M3U list to path, modified at modified.
func writeList(t *testing.T, path, body string, modified time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
}

// An administrator's M3U list may be a file of the server, by its path or
// a file:// address: it is read as a list from an address is, again when
// it changes before its refresh is due, and a list that went missing is
// told at once.
func TestListsAreReadFromFilesOfTheServer(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	start := *e.now.Load()
	path := filepath.Join(t.TempDir(), "custom.m3u")
	writeList(t, path, groupsPlaylist, start.Add(-time.Hour))
	addon, err := e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "Files", Account: Account{Kind: addons.KindM3U, URL: path}}, false)
	if err != nil {
		t.Fatal(err)
	}
	channels, err := e.service.Channels(t.Context(), addon.ID)
	if err != nil || !slices.Equal(names(channels), []string{"Zeb One", "Orbe Junior", "Quill Toons", "Lone Channel"}) {
		t.Fatalf("channels: %q %v", names(channels), err)
	}
	if addon.ManifestURL != path || Redact(addon.ManifestURL) != path {
		t.Errorf("stored as %q, shown as %q", addon.ManifestURL, Redact(addon.ManifestURL))
	}
	read := func() *time.Time {
		t.Helper()
		source, err := e.service.Source(t.Context(), addons.Shared(), addon.ID)
		if err != nil {
			t.Fatal(err)
		}
		return source.CheckedAt
	}

	// Unchanged, it waits for its turn.
	e.later(time.Minute)
	if err := e.service.RefreshDue(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if at := read(); at == nil || !at.Equal(start) {
		t.Errorf("an unchanged file was read again: %v", at)
	}
	// Changed, it is read at the next look, its turn or not.
	writeList(t, path, "#EXTM3U\n#EXTINF:-1,Zeb Two\nhttps://live.example/2.ts\n", start.Add(30*time.Second))
	if err := e.service.RefreshDue(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if channels, _ := e.service.Channels(t.Context(), addon.ID); !slices.Equal(names(channels), []string{"Zeb Two"}) {
		t.Errorf("after the file changed: %q", names(channels))
	}
	// Gone, it is told at once, its channels kept, and only once.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	e.later(time.Minute)
	if err := e.service.RefreshDue(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	source, _ := e.service.Source(t.Context(), addons.Shared(), addon.ID)
	if source.Error != errorFileUnreadable || source.Channels != 1 {
		t.Errorf("a list gone: %+v", source)
	}
	failed := read()
	e.later(time.Minute)
	if err := e.service.RefreshDue(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if at := read(); !at.Equal(*failed) {
		t.Errorf("a list still gone was read again before its retry: %v", at)
	}

	// A file:// address names the same file.
	other := filepath.Join(t.TempDir(), "other list.m3u")
	writeList(t, other, groupsPlaylist, start)
	address := (&url.URL{Scheme: "file", Path: other}).String()
	added, err := e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "By address", Account: Account{Kind: addons.KindM3U, URL: address}}, false)
	if err != nil || added.ManifestURL != other {
		t.Errorf("%s: %q %v", address, added.ManifestURL, err)
	}
}

// A path that holds no list is refused when adding, so is a relative path,
// and a member's source never reads a file of the server, whether it
// exists or not.
func TestListFilesAreChecked(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	dir := t.TempDir()
	list := filepath.Join(dir, "custom.m3u")
	writeList(t, list, groupsPlaylist, time.Now())
	member := addons.Personal(accounts.ID{9})
	for _, tc := range []struct {
		name     string
		address  string
		guide    string
		confined bool
		want     error
	}{
		{"a missing file", filepath.Join(dir, "missing.m3u"), "", false, serverfile.ErrMissing},
		{"a folder", dir, "", false, serverfile.ErrUnreadable},
		{"a relative path", "custom.m3u", "", false, ErrInvalidAddress},
		{"a missing guide", list, filepath.Join(dir, "missing.xml"), false, serverfile.ErrMissing},
		{"a member's file", list, "", true, ErrLocalFile},
		{"a member's missing file", filepath.Join(dir, "missing.m3u"), "", true, ErrLocalFile},
		{"a member's guide file", "https://lists.example/tv.m3u", list, true, ErrLocalFile},
	} {
		scope := addons.Shared()
		if tc.confined {
			scope = member
		}
		_, err := e.service.Add(t.Context(), scope, NewSource{Name: "TV", Account: Account{Kind: addons.KindM3U, URL: tc.address}, Guide: tc.guide}, tc.confined)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, err, tc.want)
		}
	}
	if _, _, err := e.service.PreviewAccount(t.Context(), Account{Kind: addons.KindM3U, URL: list}, "group", "", true); !errors.Is(err, ErrLocalFile) {
		t.Errorf("a member's preview of a file: %v", err)
	}
}
