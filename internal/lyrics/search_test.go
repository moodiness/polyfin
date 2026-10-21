package lyrics

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// searchTracks are what the fake LRCLIB lists for "Echo Chamber": tracks
// of several lengths, with synced and plain lyrics, either, or none, one
// of an unknown length, one without an identifier and one of a length out
// of bounds.
var searchTracks = []map[string]any{
	{"id": 11, "trackName": "Echo Chamber", "artistName": "Tone Quartet", "albumName": "Live at the Hall", "duration": 260.0,
		"instrumental": false, "plainLyrics": "Far away", "syncedLyrics": "[00:01.00] Far away"},
	{"id": 12, "trackName": "Echo Chamber", "artistName": "Tone Quartet", "albumName": "Sine Studies", "duration": 201.0,
		"instrumental": false, "plainLyrics": "Close by\n\nAgain", "syncedLyrics": nil},
	{"id": 13, "trackName": "Echo Chamber (Edit)", "artistName": "Tone Quartet", "albumName": "Sine Studies", "duration": 195.5,
		"instrumental": false, "plainLyrics": nil, "syncedLyrics": "[00:02.00] Near\n[00:04.50]Here"},
	{"id": 14, "trackName": "Echo Chamber", "artistName": "Tone Quartet", "albumName": nil, "duration": nil,
		"instrumental": false, "plainLyrics": "Unknown length", "syncedLyrics": nil},
	{"id": 15, "trackName": "Echo Chamber", "artistName": "Tone Quartet", "albumName": "Sine Studies", "duration": 200.0,
		"instrumental": true, "plainLyrics": nil, "syncedLyrics": nil},
	{"trackName": "Echo Chamber", "artistName": "Tone Quartet", "duration": 200.0, "plainLyrics": "No identifier"},
	{"id": 16, "trackName": "Echo Chamber", "artistName": "Tone Quartet", "duration": 1e12, "plainLyrics": "Too long"},
}

// answerSearch answers LRCLIB's search, by track name: searchTracks for
// "Echo Chamber", a truncated answer, one over maxSearchAnswer, one that
// is no list, a failure while failing is set, and an empty list
// otherwise.
func (f *fakeLRCLIB) answerSearch(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("track_name")
	f.mu.Lock()
	f.asked["search:"+name]++
	f.lastQuery, f.userAgent = r.URL.Query(), r.Header.Get("User-Agent")
	f.mu.Unlock()
	switch name {
	case "Echo Chamber":
		_ = json.NewEncoder(w).Encode(searchTracks)
	case "Broken Song":
		_, _ = w.Write([]byte(`[{"id": 1, "trackName": "Broken`))
	case "Huge Song":
		_, _ = w.Write([]byte(`[{"id": 1, "plainLyrics": "` + strings.Repeat("la ", maxSearchAnswer/3+1) + `"}]`))
	case "Wrong Shape":
		_, _ = w.Write([]byte(`{"id": 1, "plainLyrics": "Not a list"}`))
	case "Flaky Song":
		if f.failing.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	default:
		_, _ = w.Write([]byte(`[]`))
	}
}

// answerID answers a track of searchTracks by its identifier, another
// track for 77, a truncated answer for 66, and LRCLIB's 404 otherwise.
func (f *fakeLRCLIB) answerID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/get/")
	f.mu.Lock()
	f.asked["id:"+id]++
	f.lastQuery, f.userAgent = r.URL.Query(), r.Header.Get("User-Agent")
	f.mu.Unlock()
	switch id {
	case "77":
		_ = json.NewEncoder(w).Encode(searchTracks[0])
		return
	case "66":
		_, _ = w.Write([]byte(`{"id": 66, "plainLyrics": "Cut`))
		return
	}
	for _, track := range searchTracks {
		if n, ok := track["id"].(int); ok && strconv.Itoa(n) == id {
			_ = json.NewEncoder(w).Encode(track)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"code":404,"name":"TrackNotFound","message":"Failed to find specified track"}`))
}

// asking counts the requests whose key starts with prefix.
func (f *fakeLRCLIB) asking(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for key, count := range f.asked {
		if strings.HasPrefix(key, prefix) {
			n += count
		}
	}
	return n
}

func ids(results []Result) []string {
	list := make([]string, len(results))
	for i, result := range results {
		list[i] = result.ID
	}
	return list
}

func TestSearchListsTheClosestLengthFirst(t *testing.T) {
	f := newFakeLRCLIB(t)
	s, _, _ := newService(t, f)
	track := song("Echo Chamber")
	track.Duration = 200 * time.Second

	results := s.Search(t.Context(), track)
	// Jellyfin's identifiers: the MD5 of the provider's lowercase name,
	// then LRCLIB's track and the kind of lyrics; synced ones first, and
	// tracks of an unknown length last.
	want := []string{"5ef86bcecd71e4b91c950b94ebc1556e_12_plain", "5ef86bcecd71e4b91c950b94ebc1556e_13_synced",
		"5ef86bcecd71e4b91c950b94ebc1556e_11_synced", "5ef86bcecd71e4b91c950b94ebc1556e_11_plain",
		"5ef86bcecd71e4b91c950b94ebc1556e_14_plain", "5ef86bcecd71e4b91c950b94ebc1556e_16_plain"}
	if got := ids(results); !slices.Equal(got, want) {
		t.Fatalf("results %v, want %v", got, want)
	}
	edit := results[1]
	if edit.Title != "Echo Chamber (Edit)" || edit.Artist != "Tone Quartet" || edit.Album != "Sine Studies" || edit.Duration != 195500*time.Millisecond ||
		!edit.Lyrics.Synced || !slices.Equal(edit.Lyrics.Lines, []Line{{"Near", 2 * time.Second}, {"Here", 4500 * time.Millisecond}}) {
		t.Errorf("synced result: %+v", edit)
	}
	if plain := results[0].Lyrics; plain.Synced || !slices.Equal(plain.Lines, []Line{{Text: "Close by"}, {Text: ""}, {Text: "Again"}}) {
		t.Errorf("plain result: %+v", plain)
	}
	if results[4].Duration != 0 || results[5].Duration != 0 {
		t.Errorf("unknown lengths: %v %v", results[4].Duration, results[5].Duration)
	}
	// LRCLIB is searched by the track's title, artist and album, without
	// its length, by Polyfin as its look-ups are.
	query, agent := f.last()
	if wantQuery := (url.Values{"track_name": {"Echo Chamber"}, "artist_name": {"Tone Quartet"}, "album_name": {"Sine Studies"}}); query.Encode() != wantQuery.Encode() {
		t.Errorf("query %v, want %v", query, wantQuery)
	}
	if agent != "Polyfin/1.2.3 (https://github.com/moodiness/polyfin)" {
		t.Errorf("User-Agent %q", agent)
	}

	// Without a length, LRCLIB's order stays; without an album nor an
	// artist, LRCLIB is not told them.
	track.Duration, track.Album, track.Artist = 0, "", " "
	want = []string{want[2], want[3], want[0], want[1], want[4], want[5]}
	if got := ids(s.Search(t.Context(), track)); !slices.Equal(got, want) {
		t.Errorf("unranked results %v, want %v", got, want)
	}
	if query, _ := f.last(); query.Has("album_name") || query.Has("artist_name") {
		t.Errorf("query %v", query)
	}
}

func TestSearchAsksNothingWhenOffOrUntitled(t *testing.T) {
	f := newFakeLRCLIB(t)
	s, on, _ := newService(t, f)
	track := song("Echo Chamber")

	on.Store(false)
	if results := s.Search(t.Context(), track); results != nil {
		t.Errorf("results while turned off: %v", ids(results))
	}
	if _, err := s.Choose(t.Context(), track.ID, resultID(12, false)); !errors.Is(err, ErrNotFound) {
		t.Errorf("chosen while turned off: %v", err)
	}
	on.Store(true)
	track.Title = " "
	if results := s.Search(t.Context(), track); results != nil {
		t.Errorf("results without a title: %v", ids(results))
	}
	if n := f.asking(""); n != 0 {
		t.Errorf("LRCLIB was asked %d times", n)
	}
	var none *Service
	if _, err := none.Choose(t.Context(), track.ID, resultID(12, false)); none.Search(t.Context(), song("Echo Chamber")) != nil || !errors.Is(err, ErrNotFound) {
		t.Errorf("a nil service searched or chose: %v", err)
	}
	if _, ok := none.Chosen(t.Context(), track.ID); ok {
		t.Error("a nil service has chosen lyrics")
	}
}

func TestMalformedOrOversizedAnswersAreRefused(t *testing.T) {
	f := newFakeLRCLIB(t)
	s, _, c := newService(t, f)
	f.failing.Store(true)
	for _, name := range []string{"Broken Song", "Huge Song", "Wrong Shape", "Flaky Song"} {
		if results := s.Search(t.Context(), song(name)); results != nil {
			t.Errorf("%s: %v", name, ids(results))
		}
		// A failure leaves LRCLIB alone for a while, searches and
		// look-ups alike.
		s.Search(t.Context(), song("Echo Chamber"))
		s.Lyrics(t.Context(), song("Synced Song"), 5*time.Second)
		if f.count("search:Echo Chamber")+f.count("Synced Song") != 0 {
			t.Errorf("LRCLIB was asked right after %s: %v", name, f.asked)
		}
		c.advance(failedFor)
	}
	track := song("Echo Chamber")
	for _, id := range []int64{66, 77} {
		if _, err := s.Choose(t.Context(), track.ID, resultID(id, false)); !errors.Is(err, ErrNotFound) {
			t.Errorf("track %d: %v", id, err)
		}
		c.advance(failedFor)
	}
	if _, ok := s.Lyrics(t.Context(), track, 0); ok || f.count("id:66") != 1 || f.count("id:77") != 1 {
		t.Errorf("lyrics kept from a refused answer, asked %v", f.asked)
	}
	if len(s.Search(t.Context(), track)) == 0 {
		t.Error("no results once LRCLIB answers again")
	}
}

func TestSearchesShareThePacingOfLookUps(t *testing.T) {
	f := newFakeLRCLIB(t)
	s, _, _ := newService(t, f)
	var held []*lookup
	for range maxAsking {
		held = append(held, s.ask(song("Slow Song")))
	}
	track := song("Echo Chamber")
	if results := s.Search(t.Context(), track); results != nil {
		t.Errorf("results past maxAsking: %v", ids(results))
	}
	if _, err := s.Choose(t.Context(), track.ID, resultID(12, false)); !errors.Is(err, ErrNotFound) {
		t.Errorf("chosen past maxAsking: %v", err)
	}
	if n := f.asking("search:") + f.asking("id:"); n != 0 {
		t.Errorf("LRCLIB was asked %d times past maxAsking", n)
	}
	close(f.release)
	for _, l := range held {
		<-l.done
	}
	if len(s.Search(t.Context(), track)) == 0 {
		t.Error("no results once the look-ups are done")
	}
}

func TestChosenLyricsStay(t *testing.T) {
	f := newFakeLRCLIB(t)
	s, on, c := newService(t, f)
	track := song("Unknown Song")
	if _, ok := s.Lyrics(t.Context(), track, 5*time.Second); ok {
		t.Fatal("lyrics of an unknown song")
	}

	chosen, err := s.Choose(t.Context(), track.ID, resultID(13, true))
	want := Lyrics{Synced: true, Lines: []Line{{"Near", 2 * time.Second}, {"Here", 4500 * time.Millisecond}}}
	if err != nil || !chosen.Synced || !slices.Equal(chosen.Lines, want.Lines) {
		t.Fatalf("chosen: %v %+v", err, chosen)
	}
	if query, agent := f.last(); f.count("id:13") != 1 || len(query) != 0 || agent != "Polyfin/1.2.3 (https://github.com/moodiness/polyfin)" {
		t.Errorf("asked %v, query %v, User-Agent %q", f.asked, query, agent)
	}
	// They are the song's lyrics, and a month later, when LRCLIB would be
	// asked again about a song it did not know, they stay.
	c.advance(missingFor + time.Hour)
	for _, read := range []func() (Lyrics, bool){
		func() (Lyrics, bool) { return s.Lyrics(t.Context(), track, 5*time.Second) },
		func() (Lyrics, bool) { return s.Chosen(t.Context(), track.ID) },
	} {
		if got, ok := read(); !ok || !slices.Equal(got.Lines, want.Lines) {
			t.Errorf("lyrics after the choice: %v %+v", ok, got)
		}
	}
	if !s.Known(t.Context(), []accounts.ID{track.ID})[track.ID] {
		t.Error("chosen lyrics are not known")
	}
	if n := f.count("Unknown Song"); n != 1 {
		t.Errorf("LRCLIB was asked %d times about a song with lyrics chosen", n)
	}
	// Another choice replaces them, plain lyrics too.
	if _, err := s.Choose(t.Context(), track.ID, resultID(12, false)); err != nil {
		t.Fatal(err)
	}
	if got, ok := s.Lyrics(t.Context(), track, 0); !ok || got.Synced || got.Lines[0].Text != "Close by" {
		t.Errorf("lyrics chosen again: %v %+v", ok, got)
	}

	// A look-up under way when lyrics are chosen does not replace them.
	slow := song("Slow Song")
	l := s.ask(slow)
	if _, err := s.Choose(t.Context(), slow.ID, resultID(11, true)); err != nil {
		t.Fatal(err)
	}
	close(f.release)
	<-l.done
	if got, ok := s.Lyrics(t.Context(), slow, 0); !ok || !got.Synced || got.Lines[0].Text != "Far away" || f.count("Slow Song") != 1 {
		t.Errorf("lyrics after a look-up: %v %+v, asked %v", ok, got, f.asked)
	}

	// Turned off, chosen lyrics are not served either.
	on.Store(false)
	if _, ok := s.Chosen(t.Context(), track.ID); ok {
		t.Error("chosen lyrics served while turned off")
	}
	if _, ok := s.Lyrics(t.Context(), track, 0); ok {
		t.Error("lyrics served while turned off")
	}
}

func TestChooseRefusesWhatIsNoResult(t *testing.T) {
	f := newFakeLRCLIB(t)
	s, _, _ := newService(t, f)
	track := song("Echo Chamber")
	for _, id := range []string{"", "x", "12_plain", "0123456789abcdef0123456789abcdef_12_plain", providerID + "_12_lyrics",
		providerID + "_+12_plain", providerID + "_012_plain", providerID + "_0_plain", providerID + "_-12_plain",
		providerID + "_99999999999999999999_plain", providerID + "_12_plain_x", providerID + "_12", providerID + "_1" + strings.Repeat("0", 40) + "_plain"} {
		if _, err := s.Choose(t.Context(), track.ID, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q: %v", id, err)
		}
	}
	if n := f.asking("id:"); n != 0 {
		t.Errorf("LRCLIB was asked %d times about malformed identifiers", n)
	}
	// Lyrics LRCLIB does not have: a track it does not know, synced
	// lyrics of a track that has plain ones only, an instrumental. Its
	// 404 is an answer, which leaves it free to be asked again.
	for _, id := range []string{resultID(99, false), resultID(12, true), resultID(15, false)} {
		if _, err := s.Choose(t.Context(), track.ID, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: %v", id, err)
		}
	}
	if f.count("id:99") != 1 || f.count("id:12") != 1 || f.count("id:15") != 1 {
		t.Errorf("asked %v", f.asked)
	}
	if _, ok := s.Chosen(t.Context(), track.ID); ok {
		t.Error("lyrics kept from a refused choice")
	}
}

func FuzzParseSearch(f *testing.F) {
	tracks, err := json.Marshal(searchTracks)
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range []string{string(tracks), "[]", "null", `{"id": 1}`, `[{"id": 1, "duration": -5, "syncedLyrics": "[00:01]a"}]`,
		`[{"id": 9223372036854775807, "duration": 86400.5, "plainLyrics": " \n "}, {"id": 2, "duration": 3, "syncedLyrics": "[ar:x]", "plainLyrics": "b"}]`} {
		f.Add([]byte(seed), uint16(200))
	}
	f.Fuzz(func(t *testing.T, data []byte, seconds uint16) {
		length := time.Duration(seconds) * time.Second
		results, err := parseSearch(data, length)
		if err != nil {
			return
		}
		if len(results) > 2*maxResults {
			t.Fatalf("%d results", len(results))
		}
		previous := time.Duration(-1)
		for _, result := range results {
			id, synced, ok := parseResultID(result.ID)
			if !ok || id <= 0 || synced != result.Lyrics.Synced || resultID(id, synced) != result.ID {
				t.Fatalf("result %q", result.ID)
			}
			if result.Duration < 0 || result.Duration > maxLength || len(result.Lyrics.Lines) == 0 {
				t.Fatalf("result %+v", result)
			}
			if length > 0 {
				distance := time.Duration(1<<63 - 1)
				if result.Duration > 0 {
					distance = (result.Duration - length).Abs()
				}
				if distance < previous {
					t.Fatalf("%v listed after a result %v away", result.Duration, previous)
				}
				previous = distance
			}
		}
	})
}

func FuzzParseResultID(f *testing.F) {
	for _, seed := range []string{resultID(12, true), resultID(1, false), "x_1_plain", providerID + "_01_plain"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		id, synced, ok := parseResultID(raw)
		if ok && (id <= 0 || resultID(id, synced) != raw) {
			t.Fatalf("%q read as %d %v", raw, id, synced)
		}
	})
}

func TestRemovedLyricsStayRemoved(t *testing.T) {
	f := newFakeLRCLIB(t)
	s, on, c := newService(t, f)
	track := song("Synced Song")
	if _, ok := s.Lyrics(t.Context(), track, 5*time.Second); !ok {
		t.Fatal("no lyrics found")
	}
	if _, err := s.Choose(t.Context(), track.ID, resultID(13, true)); err != nil {
		t.Fatal(err)
	}

	// Removed, chosen lyrics are none, and a month later no look-up
	// gives the track lyrics again.
	if err := s.Remove(t.Context(), track.ID); err != nil {
		t.Fatal(err)
	}
	c.advance(missingFor + time.Hour)
	if got, ok := s.Lyrics(t.Context(), track, 5*time.Second); ok {
		t.Errorf("lyrics after the removal: %+v", got)
	}
	if _, ok := s.Chosen(t.Context(), track.ID); ok {
		t.Error("chosen lyrics after the removal")
	}
	if known := s.Known(t.Context(), []accounts.ID{track.ID}); known[track.ID] {
		t.Error("removed lyrics are known")
	}
	// A track never looked up is not looked up once its lyrics are removed,
	// nor is one whose look-up was under way, whatever it answers.
	never, slow := song("Plain Song"), song("Slow Song")
	l := s.ask(slow)
	for _, id := range []accounts.ID{never.ID, slow.ID} {
		if err := s.Remove(t.Context(), id); err != nil {
			t.Fatal(err)
		}
	}
	close(f.release)
	<-l.done
	for _, removed := range []Track{never, slow} {
		if got, ok := s.Lyrics(t.Context(), removed, 5*time.Second); ok {
			t.Errorf("%s: lyrics after the removal: %+v", removed.Title, got)
		}
	}
	if f.count("Synced Song") != 1 || f.count("Plain Song") != 0 || f.count("Slow Song") != 1 {
		t.Errorf("asked %v", f.asked)
	}

	// Lyrics chosen later replace the removal, and lyrics turned off
	// leave removals possible.
	if _, err := s.Choose(t.Context(), track.ID, resultID(12, false)); err != nil {
		t.Fatal(err)
	}
	if got, ok := s.Lyrics(t.Context(), track, 0); !ok || got.Lines[0].Text != "Close by" {
		t.Errorf("lyrics chosen after the removal: %v %+v", ok, got)
	}
	on.Store(false)
	if err := s.Remove(t.Context(), track.ID); err != nil {
		t.Fatal(err)
	}
	on.Store(true)
	if _, ok := s.Lyrics(t.Context(), track, 0); ok {
		t.Error("lyrics removed while turned off are back")
	}
	var none *Service
	if err := none.Remove(t.Context(), track.ID); err != nil {
		t.Errorf("a nil service: %v", err)
	}
}
