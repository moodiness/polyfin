package playback

import (
	"strconv"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// The lookback a playback of a searched version learns is kept by the
// file: the next playback, after a restart too, starts with it, and it
// only grows. A local file that changed learns it again; the programmes
// of a channel's archive share theirs; an unsearched version never uses
// one.
func TestLookbacksAreKeptByFile(t *testing.T) {
	first := newService(t, &fakeSource{}, "ffprobe-not-installed", nil)
	// The same database, as after a restart.
	again := newServiceOn(t, first.db, &fakeSource{}, "ffprobe-not-installed", nil)
	transport := media.Analysis{Format: "mpegts", Duration: 180 * time.Second,
		Streams: []media.Stream{{Index: 0, Type: "video", Codec: "h264"}, {Index: 1, Type: "audio", Codec: "aac"}}}
	states := map[string]string{}
	for _, s := range []*Service{first, again} {
		s.LocalFiles(nil)
		s.LocalInputs(fakeInputs{states: states})
	}
	// opened opens an encoding of version in s, its file an MPEG-TS file
	// without an index, or analysis, and returns what it starts with.
	opened := func(s *Service, version library.Version, analysis media.Analysis) hls.Remux {
		t.Helper()
		s.analyses.Put(version.ID, analysis)
		s.unindexed.Put(version.ID, container.ErrNoIndex)
		convert := &VideoConversion{Codec: "h264", Encoder: "libx264", Width: 1280, Height: 720, Bitrate: 4_000_000}
		r, release, err := s.remuxOpener(Remux{Session: "session", Version: version, Audio: 1, Format: hls.TS, ConvertVideo: convert})(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		release()
		return r
	}
	learn := func(s *Service, version library.Version, lookback time.Duration) {
		t.Helper()
		r := opened(s, version, transport)
		if r.Learned == nil {
			t.Fatal("the encoding keeps no lookback it learns")
		}
		r.Learned(lookback)
	}
	expect := func(what string, s *Service, version library.Version, want time.Duration) {
		t.Helper()
		if got := opened(s, version, transport).Lookback; got != want {
			t.Errorf("%s: starts %v back, want %v", what, got, want)
		}
	}

	addon := library.Version{ID: accounts.ID{1}, URL: "http://addon.example/movie.ts", Filename: "movie.ts", Size: 1 << 30}
	expect("an addon's file never played", first, addon, 0)
	learn(first, addon, 9736100*time.Microsecond)
	expect("an addon's file, after a restart", again, addon, 9737*time.Millisecond)
	learn(again, addon, 3*time.Second)
	expect("an addon's file, after a shorter lookback", first, addon, 9737*time.Millisecond)
	learn(again, addon, 12*time.Second)
	expect("an addon's file, after a longer lookback", first, addon, 12*time.Second)
	expect("another file of the addon", first, library.Version{ID: accounts.ID{2}, URL: addon.URL}, 0)

	local := library.Version{ID: accounts.ID{3}, URL: first.localBase() + "7.ts"}
	learn(first, local, 5*time.Second)
	expect("a local file, unchanged", again, library.Version{ID: accounts.ID{3}, URL: again.localBase() + "7.ts"}, 5*time.Second)
	states["7.ts"] = "changed"
	expect("a local file, changed", first, local, 0)
	learn(first, local, 2*time.Second)
	expect("a local file, changed, learned again", first, local, 2*time.Second)
	disc := library.Version{ID: accounts.ID{4}, URL: first.inputBase() + "8"}
	learn(first, disc, 4*time.Second)
	expect("a disc's title", again, library.Version{ID: accounts.ID{4}, URL: again.inputBase() + "8"}, 4*time.Second)

	channel := iptv.IDPrefix + "11111111-2222-3333-4444-555555555555:news"
	programme := func(id byte, start time.Time) library.Version {
		replay, ok := iptv.ReplayID(channel, start, start.Add(time.Hour))
		if !ok {
			t.Fatal("no replay identifier")
		}
		return library.Version{ID: accounts.ID{id}, URL: "http://provider.example/archive/" + strconv.Itoa(int(id)) + ".ts", HoldsConnection: true,
			Origin: library.Origin{Type: "tv", ID: replay}}
	}
	evening := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)
	learn(first, programme(5, evening), 6*time.Second)
	expect("another programme of the channel", again, programme(6, evening.Add(24*time.Hour)), 6*time.Second)
	other, _ := iptv.ReplayID(iptv.IDPrefix+"11111111-2222-3333-4444-555555555555:sports", evening, evening.Add(time.Hour))
	expect("a programme of another channel", again, library.Version{ID: accounts.ID{7}, URL: "http://provider.example/archive/7.ts",
		HoldsConnection: true, Origin: library.Origin{Type: "tv", ID: other}}, 0)

	// A Matroska file's index cuts it on its keyframes, which FFmpeg lands
	// on: its version is not searched.
	indexed := library.Version{ID: accounts.ID{8}, URL: "http://addon.example/movie.mkv"}
	first.saveLookback(t.Context(), seekFile{kind: "version", name: indexed.ID.String()}, 10*time.Second)
	first.indexes.Put(indexed.ID, []time.Duration{0, 4 * time.Second, 8 * time.Second})
	matroska := transport
	matroska.Format = "matroska,webm"
	if r := opened(first, indexed, matroska); r.Plan.IsSearched() || r.Lookback != 0 || r.Learned != nil {
		t.Errorf("an unsearched version starts %v back, searched %v, learning %v", r.Lookback, r.Plan.IsSearched(), r.Learned != nil)
	}
}
