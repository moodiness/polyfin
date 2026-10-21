package lyrics

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestParseUploadReadsLyricsFiles(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		synced     bool
		want       []Line
	}{
		{"song.lrc", "\xef\xbb\xbf[ti:Song]\r\n[00:01.00]One\r\n[00:02.50]Two\r\n", true, []Line{{"One", time.Second}, {"Two", 2500 * time.Millisecond}}},
		{"song.ELRC", "[00:03.00] <00:03.00>Every <00:03.50>word", true, []Line{{"Every word", 3 * time.Second}}},
		{`C:\lyrics\song.txt`, "Plain one\n\nPlain two\n", false, []Line{{Text: "Plain one"}, {Text: ""}, {Text: "Plain two"}}},
		{"song.lrc", "[ar:Someone]\nUntimed\tline", false, []Line{{Text: "[ar:Someone]"}, {Text: "Untimed\tline"}}},
	} {
		text, found, err := ParseUpload(tc.name, []byte(tc.body))
		if err != nil || found.Synced != tc.synced || !slices.Equal(found.Lines, tc.want) || strings.HasPrefix(text, "\xef\xbb\xbf") {
			t.Errorf("%s %q: %v %+v %q", tc.name, tc.body, err, found, text)
		}
	}
	for name, body := range map[string]string{
		"song.mp3":   "[00:01.00]One",
		"song.lrc.x": "[00:01.00]One",
		"blank.lrc":  " \n\t\n",
		"binary.lrc": "[00:01.00]One\x00\x01",
		"latin1.txt": "caf\xe9",
		"escape.txt": "\x1b[31mred",
		"huge.txt":   strings.Repeat("la ", MaxUpload/3+1),
	} {
		if _, _, err := ParseUpload(name, []byte(body)); !errors.Is(err, ErrNotLyrics) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, name := range []string{"song", "song.", "dir.d/song"} {
		if _, _, err := ParseUpload(name, []byte("words")); !errors.Is(err, ErrNoExtension) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestUploadedLyricsStayWhateverTheSetting(t *testing.T) {
	f := newFakeLRCLIB(t)
	s, on, c := newService(t, f)
	track := song("Unknown Song")
	if _, ok := s.Lyrics(t.Context(), track, 5*time.Second); ok {
		t.Fatal("lyrics of an unknown song")
	}

	// An upload is the track's lyrics, and a month later, when LRCLIB
	// would be asked again, it stays.
	synced, err := s.Upload(t.Context(), track.ID, "song.lrc", []byte("[00:01.00]Uploaded\n[00:02.00]Words"))
	if err != nil || !synced.Synced || len(synced.Lines) != 2 {
		t.Fatalf("upload: %v %+v", err, synced)
	}
	c.advance(missingFor + time.Hour)
	if got, ok := s.Lyrics(t.Context(), track, 5*time.Second); !ok || !slices.Equal(got.Lines, synced.Lines) || f.count("Unknown Song") != 1 {
		t.Errorf("a month later: %v %+v, asked %v", ok, got, f.asked)
	}

	// Turned off, uploaded lyrics are served and known, and uploads work;
	// lyrics chosen among LRCLIB's results are not served.
	on.Store(false)
	for _, read := range []func() (Lyrics, bool){
		func() (Lyrics, bool) { return s.Lyrics(t.Context(), track, 5*time.Second) },
		func() (Lyrics, bool) { return s.Chosen(t.Context(), track.ID) },
	} {
		if got, ok := read(); !ok || got.Lines[0].Text != "Uploaded" {
			t.Errorf("uploaded lyrics turned off: %v %+v", ok, got)
		}
	}
	other := song("Plain Song")
	if _, err := s.Upload(t.Context(), other.ID, "other.txt", []byte("Typed by hand")); err != nil {
		t.Fatal(err)
	}
	if known := s.Known(t.Context(), []accounts.ID{track.ID, other.ID}); !known[track.ID] || !known[other.ID] {
		t.Errorf("known turned off: %v", known)
	}
	if got, ok := s.Lyrics(t.Context(), other, 0); !ok || got.Synced || got.Lines[0].Text != "Typed by hand" || f.count("Plain Song") != 0 {
		t.Errorf("plain upload turned off: %v %+v, asked %v", ok, got, f.asked)
	}
	on.Store(true)

	// A choice replaces an upload, and is not served turned off; an upload
	// replaces a choice; a removal removes it.
	if _, err := s.Choose(t.Context(), track.ID, resultID(12, false)); err != nil {
		t.Fatal(err)
	}
	on.Store(false)
	if _, ok := s.Lyrics(t.Context(), track, 0); ok {
		t.Error("a choice from LRCLIB served turned off")
	}
	if _, err := s.Upload(t.Context(), track.ID, "again.txt", []byte("Second upload")); err != nil {
		t.Fatal(err)
	}
	if got, ok := s.Lyrics(t.Context(), track, 0); !ok || got.Lines[0].Text != "Second upload" {
		t.Errorf("an upload over a choice: %v %+v", ok, got)
	}
	if err := s.Remove(t.Context(), track.ID); err != nil {
		t.Fatal(err)
	}
	on.Store(true)
	if _, ok := s.Lyrics(t.Context(), track, 5*time.Second); ok || f.count("Unknown Song") != 1 {
		t.Errorf("removed upload served, asked %v", f.asked)
	}

	// What cannot be read is not kept; a nil service keeps nothing.
	if _, err := s.Upload(t.Context(), other.ID, "bad.lrc", []byte{0xff, 0xfe}); !errors.Is(err, ErrNotLyrics) {
		t.Errorf("binary upload: %v", err)
	}
	if got, ok := s.Lyrics(t.Context(), other, 0); !ok || got.Lines[0].Text != "Typed by hand" {
		t.Errorf("a refused upload replaced lyrics: %v %+v", ok, got)
	}
	var none *Service
	if _, err := none.Upload(t.Context(), track.ID, "song.txt", []byte("words")); !errors.Is(err, ErrNotFound) {
		t.Errorf("a nil service: %v", err)
	}
}

func FuzzParseUpload(f *testing.F) {
	for _, seed := range []string{syncedLRC, "Plain\r\nwords", "\xef\xbb\xbf[00:01]a", "[00:01][00:02][9999:59.999]x", "\x00", "[ar:x]\n[length: 03:00]"} {
		f.Add("song.lrc", []byte(seed))
		f.Add("song.txt", []byte(seed))
	}
	f.Fuzz(func(t *testing.T, name string, body []byte) {
		text, found, err := ParseUpload(name, body)
		if err != nil {
			return
		}
		if len(text) > MaxUpload || len(found.Lines) == 0 {
			t.Fatalf("%d bytes, %d lines", len(text), len(found.Lines))
		}
		for i, line := range found.Lines {
			if strings.ContainsAny(line.Text, "\r\n\x00") || line.Start < 0 || !found.Synced && line.Start != 0 {
				t.Fatalf("line %+v", line)
			}
			if i > 0 && line.Start < found.Lines[i-1].Start {
				t.Fatalf("line %+v after %+v", line, found.Lines[i-1])
			}
		}
		// What is kept reads back to the same lyrics.
		kind := kindPlain
		if found.Synced {
			kind = kindSynced
		}
		again, ok := answer{kind: kind, lyrics: text}.parse()
		if !ok || again.Synced != found.Synced || !slices.Equal(again.Lines, found.Lines) {
			t.Fatalf("kept %q reads as %+v, not %+v", text, again, found)
		}
	})
}
