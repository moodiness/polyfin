package remuxdb

import (
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
)

// movieAnswer is RemuxDB's answer about a movie, as its versions endpoint
// gives it: a Dolby Vision file found in a torrent's folder, whose fourth
// track ffprobe numbers but RemuxDB leaves out, and an HLG file from an NZB.
const movieAnswer = `[
  {"content_hash": "6f1c", "container": "matroska,webm", "duration": 8160.5, "size": 10000000000, "bitrate": 9803921,
   "virtual_chapters": false, "chapters": [{"id": 1, "start_time": 0, "end_time": 600.5, "title": "Chapter 1"}],
   "sources": [{"kind": "torrent", "filename": "The.Movie.2160p/The.Movie.2160p.mkv",
     "torrent_info_hash": "0123456789abcdef0123456789abcdef01234567", "torrent_file_idx": 1}],
   "tracks": [
     {"kind": "video", "idx": 0, "codec": "hevc", "profile": "Main 10", "width": 3840, "height": 1600, "fps": 23.976,
      "aspect_ratio": "12:5", "pixel_format": "yuv420p10le", "bit_depth": 10, "level": 153, "ref_frames": 1,
      "color_range": "limited", "color_space": "bt2020_nc", "color_transfer": "smpte2084", "color_primaries": "bt2020",
      "dv_profile": 8, "hdr10_plus_present": false, "is_default": true, "is_forced": false,
      "is_hearing_impaired": false, "is_external": false, "is_anamorphic": false},
     {"kind": "audio", "idx": 1, "codec": "truehd", "profile": "Dolby TrueHD + Dolby Atmos", "language": "eng",
      "channels": 8, "channel_layout": "7.1", "sample_rate": 48000, "is_default": true, "is_forced": false,
      "is_hearing_impaired": false, "is_external": false, "is_anamorphic": false, "hdr10_plus_present": false},
     {"kind": "audio", "idx": 2, "codec": "eac3", "language": "fre", "title": "VFF", "channels": 6,
      "channel_layout": "5.1(side)", "sample_rate": 48000, "bit_rate": 640000, "is_default": false, "is_forced": false,
      "is_hearing_impaired": false, "is_external": false, "is_anamorphic": false, "hdr10_plus_present": false},
     {"kind": "subtitle", "idx": 4, "codec": "srt", "language": "fre", "title": "Forced", "is_default": false,
      "is_forced": true, "is_hearing_impaired": false, "is_external": false, "is_anamorphic": false,
      "hdr10_plus_present": false},
     {"kind": "subtitle", "idx": 5, "codec": "subrip", "language": "eng", "is_default": false, "is_forced": false,
      "is_hearing_impaired": true, "is_external": true, "is_anamorphic": false, "hdr10_plus_present": false}
   ]},
  {"content_hash": "9e2d", "container": "mov,mp4,m4a,3gp,3g2,mj2", "duration": 8160, "size": 2500000000,
   "sources": [{"kind": "nzb", "filename": "The.Movie.1080p.HLG.mp4", "indexer": "indexer", "indexer_guid": "1e8b"}],
   "tracks": [
     {"kind": "video", "idx": 0, "codec": "h264", "width": 1920, "height": 800, "color_transfer": "arib_std_b67",
      "is_default": true, "is_forced": false, "is_hearing_impaired": false, "is_external": false,
      "is_anamorphic": false, "hdr10_plus_present": false}
   ]}
]`

// episodeAnswer is RemuxDB's answer about an episode, a single file.
const episodeAnswer = `[
  {"container": "matroska,webm", "duration": 3480, "size": 1200000000,
   "sources": [{"kind": "torrent", "filename": "Show.S01E02.1080p.mkv", "torrent_info_hash": "aa", "torrent_file_idx": 0}],
   "tracks": [{"kind": "video", "idx": 0, "codec": "h264", "width": 1920, "height": 1080},
     {"kind": "audio", "idx": 1, "codec": "aac", "language": "eng", "channels": 2}]}
]`

// remuxDB is a fake RemuxDB that answers movieAnswer and episodeAnswer, and
// counts the requests it gets. While status is set, it answers only that;
// while hold is set, its answers wait for it to close.
type remuxDB struct {
	*httptest.Server
	mu     sync.Mutex
	asked  map[string]int
	status int
	hold   chan struct{}
	// clientIDs and agents are what the requests named themselves.
	clientIDs, agents []string
}

func newRemuxDB(t *testing.T) *remuxDB {
	t.Helper()
	db := &remuxDB{asked: map[string]int{}}
	db.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		db.mu.Lock()
		db.asked[r.URL.Path]++
		db.clientIDs = append(db.clientIDs, r.Header.Get("X-Client-Id"))
		db.agents = append(db.agents, r.Header.Get("User-Agent"))
		status, hold := db.status, db.hold
		db.mu.Unlock()
		if hold != nil {
			<-hold
		}
		if status != 0 {
			if status == http.StatusTooManyRequests {
				w.Header().Set("Retry-After", "120")
			}
			w.WriteHeader(status)
			return
		}
		switch r.URL.Path {
		case "/api/media/tt0133093/versions":
			_, _ = io.WriteString(w, movieAnswer)
		case "/api/media/tt0903747:1:2/versions":
			_, _ = io.WriteString(w, episodeAnswer)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(db.Close)
	return db
}

// set has the fake answer status, or wait for hold, from now on.
func (db *remuxDB) set(status int, hold chan struct{}) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.status, db.hold = status, hold
}

// counts are the requests made, by path.
func (db *remuxDB) counts() map[string]int {
	db.mu.Lock()
	defer db.mu.Unlock()
	return maps.Clone(db.asked)
}

// requests is how many requests were made, over every path.
func (db *remuxDB) requests() int {
	total := 0
	for _, n := range db.counts() {
		total += n
	}
	return total
}

// names are what each request named itself: its client ID and its user
// agent.
func (db *remuxDB) names() (clientIDs, agents []string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	return slices.Clone(db.clientIDs), slices.Clone(db.agents)
}

// service is a service asking db, with settings turning RemuxDB on.
func service(db *remuxDB, settings *accounts.Settings) *Service {
	settings.RemuxDB, settings.RemuxDBURL = true, db.URL
	return New("0123456789abcdef0123456789abcdef", "test", slog.New(slog.DiscardHandler), func() accounts.Settings { return *settings })
}

// version is a version listed for the title of id, a file name and a size
// as addons give them, and a runtime of two hours and a quarter.
func version(n byte, id, filename string, size int64) library.Version {
	return library.Version{ID: accounts.ID{n}, Item: accounts.ID{0xff}, Filename: filename, Size: size,
		Origin: library.Origin{Type: "movie", ID: id}, Runtime: 135 * time.Minute}
}

func TestMatchFindsTheFileAVersionIs(t *testing.T) {
	probed := func(size int64, duration float64, names ...string) file {
		f := file{Size: size, Duration: duration}
		for _, name := range names {
			f.Sources = append(f.Sources, source{Filename: name})
		}
		return f
	}
	files := []file{
		probed(80_000_000_000, 8100, "Movie.2160p/Movie.2160p.mkv", "Movie.2160p.Other.mkv"),
		probed(3_000_000_000, 8100, "Movie.1080p.mkv"),
		probed(3_060_000_000, 8100, "Movie.1080p.mkv"),
		probed(50_000_000, 60, "Movie.Sample.mkv"),
		probed(30, 0, "Broken.mkv"),
	}
	runtime := 135 * time.Minute
	for _, test := range []struct {
		name     string
		filename string
		size     int64
		want     int
	}{
		// The size addons round to a tenth of a GiB: 74.5 GiB.
		{"in a folder, in another case, with a rounded size", "movie.2160P.MKV", 79_993_765_888, 0},
		{"the closest of the files of a name", "Movie.1080p.mkv", 3_050_000_000, 2},
		{"a name several files share, without a size", "Movie.1080p.mkv", 0, -1},
		{"a size far from the file's", "Movie.2160p.mkv", 40_000_000_000, -1},
		{"a sample of the title", "Movie.Sample.mkv", 50_000_000, -1},
		{"a file RemuxDB could not read", "Broken.mkv", 2_000_000_000, -1},
		{"another name", "Movie.720p.mkv", 3_000_000_000, -1},
		{"no name", "", 3_000_000_000, -1},
	} {
		got := match(files, library.Version{Filename: test.filename, Size: test.size, Runtime: runtime})
		want := (*file)(nil)
		if test.want >= 0 {
			want = &files[test.want]
		}
		if got != want {
			t.Errorf("%s: matched %+v, want %+v", test.name, got, want)
		}
	}
}

func TestDescriptionsReadAsTheFilesAnalyses(t *testing.T) {
	db := newRemuxDB(t)
	s := service(db, &accounts.Settings{})
	dolbyVision := version(1, "tt0133093", "The.Movie.2160p.mkv", 10_000_000_000)
	hlg := version(2, "tt0133093", "The.Movie.1080p.HLG.mp4", 2_500_000_000)
	s.Describe(t.Context(), []library.Version{dolbyVision, hlg})

	analysis, ok := s.Described(dolbyVision)
	if !ok {
		t.Fatal("the Dolby Vision file is not described")
	}
	if playback.Container(analysis) != "mkv" || analysis.Duration != 8160500*time.Millisecond || analysis.Size != 10_000_000_000 ||
		len(analysis.Chapters) != 0 {
		t.Errorf("file: %s %s %d %d chapters", playback.Container(analysis), analysis.Duration, analysis.Size, len(analysis.Chapters))
	}
	// The tracks keep the indexes ffprobe gives them, as playback will
	// number them once the version is analyzed; the subtitle file RemuxDB
	// found beside the video is left out.
	streams := playback.MediaStreams(analysis, nil, "en")
	if len(streams) != 4 {
		t.Fatalf("streams: %+v", streams)
	}
	video, english, french, forced := streams[0], streams[1], streams[2], streams[3]
	if video.Type != "Video" || video.Index != 0 || video.Height == nil || *video.Height != 1600 ||
		video.VideoRangeType != "DOVIWithHDR10" || video.ColorSpace != "bt2020nc" {
		t.Errorf("video: %+v", video)
	}
	if english.Type != "Audio" || english.Index != 1 || !english.IsDefault || english.AudioSpatialFormat != "DolbyAtmos" ||
		english.Channels == nil || *english.Channels != 8 {
		t.Errorf("English audio: %+v", english)
	}
	if french.Type != "Audio" || french.Index != 2 || french.IsDefault || french.Title != "VFF" {
		t.Errorf("French audio: %+v", french)
	}
	if forced.Type != "Subtitle" || forced.Index != 4 || forced.Codec != "subrip" || !forced.IsForced {
		t.Errorf("forced subtitles: %+v", forced)
	}

	analysis, ok = s.Described(hlg)
	if streams := playback.MediaStreams(analysis, nil, "en"); !ok || len(streams) != 1 || streams[0].VideoRangeType != "HLG" {
		t.Errorf("HLG file: %v %+v", ok, streams)
	}
}

func TestDescribeAsksRemuxDBOncePerTitle(t *testing.T) {
	db := newRemuxDB(t)
	settings := &accounts.Settings{}
	s := service(db, settings)
	movie := version(1, "tt0133093", "The.Movie.2160p.mkv", 10_000_000_000)
	unknown := version(2, "tt0133093", "The.Movie.720p.mkv", 0)
	episode := version(3, "tt0903747:1:2", "Show.S01E02.1080p.mkv", 0)
	otherIDs := version(4, "kitsu:1:2", "Show.S01E02.1080p.mkv", 0)
	s.Describe(t.Context(), []library.Version{movie, unknown, episode, otherIDs})
	asked := db.counts()
	if asked["/api/media/tt0133093/versions"] != 1 || asked["/api/media/tt0903747:1:2/versions"] != 1 || len(asked) != 2 {
		t.Fatalf("asked %v", asked)
	}
	clientIDs, agents := db.names()
	for i, id := range clientIDs {
		if len(id) != 32 || id == "0123456789abcdef0123456789abcdef" || id != clientIDs[0] || agents[i] != "Polyfin/test" {
			t.Errorf("request %d named %q, %q", i, id, agents[i])
		}
	}
	for _, test := range []struct {
		version library.Version
		want    bool
	}{{movie, true}, {unknown, false}, {episode, true}, {otherIDs, false}} {
		if _, ok := s.Described(test.version); ok != test.want {
			t.Errorf("%s of %s: described %v", test.version.Filename, test.version.Origin.ID, ok)
		}
	}

	// What RemuxDB said is kept, whether it knew the file or not: only a
	// version listed since has the title asked about again.
	s.Describe(t.Context(), []library.Version{movie, unknown, episode, otherIDs})
	if db.requests() != 2 {
		t.Errorf("asked again: %v", db.counts())
	}
	s.Describe(t.Context(), []library.Version{movie, version(5, "tt0133093", "The.Movie.1080p.HLG.mp4", 0)})
	if asked := db.counts(); asked["/api/media/tt0133093/versions"] != 2 || db.requests() != 3 {
		t.Errorf("a new version: %v", asked)
	}

	// Turned off, RemuxDB is not asked, and its descriptions are not used.
	settings.RemuxDB = false
	s.Describe(t.Context(), []library.Version{version(6, "tt0133093", "The.Movie.2160p.mkv", 0)})
	if _, ok := s.Described(movie); ok || db.requests() != 3 {
		t.Errorf("off: described %v, asked %v", ok, db.counts())
	}
}

func TestDescribeLeavesRemuxDBAloneAfterItFails(t *testing.T) {
	for _, test := range []struct {
		status int
		pause  time.Duration
	}{{http.StatusBadGateway, failedFor}, {http.StatusTooManyRequests, 2 * time.Minute}} {
		t.Run(strconv.Itoa(test.status), func(t *testing.T) {
			db := newRemuxDB(t)
			db.set(test.status, nil)
			s := service(db, &accounts.Settings{})
			now := time.Now()
			s.now = func() time.Time { return now }
			movie := version(1, "tt0133093", "The.Movie.2160p.mkv", 10_000_000_000)
			s.Describe(t.Context(), []library.Version{movie})
			if _, ok := s.Described(movie); ok || db.requests() != 1 {
				t.Fatalf("described %v after %d requests", ok, db.requests())
			}
			// No title is asked about while the pause lasts.
			now = now.Add(test.pause - time.Second)
			s.Describe(t.Context(), []library.Version{movie, version(2, "tt0903747:1:2", "Show.S01E02.1080p.mkv", 0)})
			if db.requests() != 1 {
				t.Errorf("asked during the pause: %v", db.counts())
			}
			db.set(0, nil)
			now = now.Add(time.Second)
			s.Describe(t.Context(), []library.Version{movie})
			if _, ok := s.Described(movie); !ok || db.requests() != 2 {
				t.Errorf("after the pause: described %v after %d requests", ok, db.requests())
			}
		})
	}
}

func TestDescribeKeepsAnAnswerThatCameLate(t *testing.T) {
	db := newRemuxDB(t)
	hold := make(chan struct{})
	db.set(0, hold)
	s := service(db, &accounts.Settings{})
	s.wait = 20 * time.Millisecond
	first := version(1, "tt0133093", "The.Movie.2160p.mkv", 10_000_000_000)
	second := version(2, "tt0133093", "The.Movie.1080p.HLG.mp4", 2_500_000_000)
	s.Describe(t.Context(), []library.Version{first})
	// Another page listing another version joins the request under way.
	s.Describe(t.Context(), []library.Version{first, second})
	if _, ok := s.Described(first); ok {
		t.Fatal("described before RemuxDB answered")
	}
	s.mu.Lock()
	asking := s.asking["tt0133093"]
	s.mu.Unlock()
	if asking == nil {
		t.Fatal("no request under way")
	}
	close(hold)
	<-asking.done
	for _, v := range []library.Version{first, second} {
		if _, ok := s.Described(v); !ok {
			t.Errorf("%s not described once RemuxDB answered", v.Filename)
		}
	}
	if db.requests() != 1 {
		t.Errorf("asked %v", db.counts())
	}
}
