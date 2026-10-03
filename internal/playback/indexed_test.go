package playback

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// fileOpener fetches from a local file server, counting requests.
type fileOpener struct{ requests atomic.Int64 }

func (o *fileOpener) Open(ctx context.Context, method, target string, header http.Header, _ bool) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, err
	}
	request.Header = header.Clone()
	o.requests.Add(1)
	return http.DefaultClient.Do(request)
}

// subtitled serves the Matroska fixture with an SRT and an ASS track and
// a font, and returns its version and analysis, as ffprobe gave it.
func subtitled(t *testing.T) (*Service, *fileOpener, library.Version, media.Analysis) {
	t.Helper()
	return served(t, filepath.Join("..", "container", "testdata"), "subtitles")
}

// served serves the Matroska fixture name.mkv in dir, and returns its
// version and analysis, ffprobe's in name.ffprobe.json.
func served(t *testing.T, dir, name string) (*Service, *fileOpener, library.Version, media.Analysis) {
	t.Helper()
	server := httptest.NewServer(http.FileServer(http.Dir(dir)))
	t.Cleanup(server.Close)
	probe, err := os.ReadFile(filepath.Join(dir, name+".ffprobe.json"))
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := media.Parse(probe)
	if err != nil {
		t.Fatal(err)
	}
	opener := &fileOpener{}
	s := newService(t, opener, "ffprobe", nil)
	version := library.Version{ID: accounts.ID{7}, URL: server.URL + "/" + name + ".mkv", Addon: "files"}
	s.analyses.Put(version.ID, analysis)
	return s, opener, version, analysis
}

func TestTracksAreReadWholeThroughTheIndex(t *testing.T) {
	s, opener, version, analysis := subtitled(t)
	ctx := t.Context()
	const srtStream, assStream, fontStream = 1, 2, 3
	if located := s.SubtitlesLocated(ctx, version, analysis); !located[srtStream] || !located[assStream] || len(located) != 2 {
		t.Fatalf("located: %v", located)
	}

	srt, err := s.SubtitleTrack(ctx, version, analysis, srtStream)
	if err != nil || srt.Format != "srt" {
		t.Fatalf("SubRip track: %s %v", srt.Format, err)
	}
	original, _ := os.ReadFile(filepath.Join("..", "container", "testdata", "subtitles.srt"))
	want, _ := subtitles.Parse(original)
	if got, err := subtitles.Parse(srt.Data); err != nil || !slices.EqualFunc(got, want, sameCue) {
		t.Errorf("SubRip cues:\n got %+v\nwant %+v", got, want)
	}

	// The ASS track keeps its script: header, styles and every event.
	ass, err := s.SubtitleTrack(ctx, version, analysis, assStream)
	if err != nil || ass.Format != "ass" {
		t.Fatalf("ASS track: %s %v", ass.Format, err)
	}
	script, _ := os.ReadFile(filepath.Join("..", "container", "testdata", "subtitles.ass"))
	wantScript, _ := subtitles.ParseScript(script)
	if got, err := subtitles.ParseScript(ass.Data); err != nil || !bytes.Equal(got.Header, wantScript.Header) || !slices.Equal(got.Events, wantScript.Events) {
		t.Errorf("ASS script:\n%s\nwant\n%s", ass.Data, wantScript.Bytes())
	}

	// Kept: read again from the database, without the file.
	s.tracks.Delete(trackKey{version.ID, assStream})
	requests := opener.requests.Load()
	if again, err := s.SubtitleTrack(ctx, version, analysis, assStream); err != nil || !bytes.Equal(again.Data, ass.Data) || opener.requests.Load() != requests {
		t.Errorf("kept track: %v, %d more requests", err, opener.requests.Load()-requests)
	}

	// An HLS segment of a track read whole is cut from it, without FFmpeg.
	segment, err := s.RemuxSubtitle(ctx, Remux{Session: "session", Version: version}, srtStream, 0)
	if err != nil || !strings.Contains(string(segment), want[0].Lines[0]) {
		t.Errorf("HLS segment: %v\n%s", err, segment)
	}

	font, err := s.Attachment(ctx, version, analysis, fontStream)
	dummy, _ := os.ReadFile(filepath.Join("..", "container", "testdata", "Dummy.ttf"))
	if err != nil || !bytes.Equal(font, dummy) {
		t.Errorf("font: %v, %d bytes, want %d", err, len(font), len(dummy))
	}
	if _, err := s.Attachment(ctx, version, analysis, srtStream); err != ErrNoAttachment {
		t.Errorf("a subtitle stream as an attachment: %v", err)
	}
}

func TestTracksOutsideAMatroskaIndexAreNotRead(t *testing.T) {
	s, opener, version, analysis := subtitled(t)
	mp4 := analysis
	mp4.Format = "mov,mp4,m4a,3gp,3g2,mj2"
	if located := s.SubtitlesLocated(t.Context(), version, mp4); len(located) != 0 {
		t.Errorf("an MP4 file located %v", located)
	}
	if _, err := s.SubtitleTrack(t.Context(), version, mp4, 1); err != ErrNotLocated {
		t.Errorf("an MP4 track: %v", err)
	}
	if opener.requests.Load() != 0 {
		t.Errorf("%d requests for a file without a Matroska index", opener.requests.Load())
	}
}

// FFmpeg names some text tracks otherwise than their Matroska codec does:
// a plain text track is read whole like SubRip, and a track FFmpeg cannot
// read leaves the others readable.
func TestTracksFFmpegNamesOtherwiseAreRead(t *testing.T) {
	s, _, version, analysis := served(t, "testdata", "codecs")
	ctx := t.Context()
	const srtStream, webvttStream, asciiStream = 1, 2, 3
	if located := s.SubtitlesLocated(ctx, version, analysis); !located[srtStream] || !located[asciiStream] || len(located) != 2 {
		t.Fatalf("located: %v", located)
	}
	ascii, err := s.SubtitleTrack(ctx, version, analysis, asciiStream)
	original, _ := os.ReadFile(filepath.Join("testdata", "codecs-ascii.vtt"))
	want, _ := subtitles.Parse(original)
	if got, parseErr := subtitles.Parse(ascii.Data); err != nil || ascii.Format != "srt" || parseErr != nil || !slices.EqualFunc(got, want, sameCue) {
		t.Errorf("text track: %s %v\n%s", ascii.Format, err, ascii.Data)
	}
	if _, err := s.SubtitleTrack(ctx, version, analysis, webvttStream); err != ErrNotLocated {
		t.Errorf("a track FFmpeg cannot read: %v", err)
	}
}

// A track whose file cannot be read whole, whatever host serves it, is not
// offered again, even after a restart. Here its first block is laced,
// which muxers never do for subtitles.
func TestTracksThatCannotBeReadAreNotOfferedAgain(t *testing.T) {
	testdata := filepath.Join("..", "container", "testdata")
	file, err := os.ReadFile(filepath.Join(testdata, "subtitles.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(filepath.Join(testdata, "subtitles.srt"))
	cues, _ := subtitles.Parse(original)
	text := []byte(cues[0].Lines[0])
	// A block's data follows its track number, time and flags.
	at := bytes.Index(file, text)
	if bytes.Count(file, text) != 1 || at < 4 || file[at-4] != 0x82 {
		t.Fatal("the first SubRip block was not found")
	}
	file[at-1] |= 0x06
	dir := t.TempDir()
	probe, _ := os.ReadFile(filepath.Join(testdata, "subtitles.ffprobe.json"))
	for name, data := range map[string][]byte{"subtitles.mkv": file, "subtitles.ffprobe.json": probe} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s, _, version, analysis := served(t, dir, "subtitles")
	ctx := t.Context()
	const srtStream, assStream = 1, 2
	if located := s.SubtitlesLocated(ctx, version, analysis); !located[srtStream] {
		t.Fatalf("located: %v", located)
	}
	if _, err := s.SubtitleTrack(ctx, version, analysis, srtStream); !errors.Is(err, container.ErrUnreadable) {
		t.Fatalf("the laced track: %v", err)
	}
	// As after a restart, the version's index comes from the database.
	s.locations.Delete(version.ID)
	s.untracked.Delete(trackKey{version.ID, srtStream})
	if located := s.SubtitlesLocated(ctx, version, analysis); located[srtStream] || !located[assStream] {
		t.Errorf("located after the failure: %v", located)
	}
}

// Once remuxes have extracted a version's text tracks whole, its HLS
// subtitle segments come from them, and its index is not read for them.
func TestExtractedTracksAreNotReadThroughTheIndex(t *testing.T) {
	s, _, version, analysis := subtitled(t)
	ctx := t.Context()
	extracted := fmt.Sprintf(`{"covered":[[0,%d]],"tracks":{"1":[{"s":1000,"e":2500,"t":"Extracted"}]}}`, analysis.Duration.Milliseconds()+1000)
	if _, err := s.db.Exec(ctx, "INSERT INTO media_subtitles (version_id, extracted) VALUES ($1, $2)", version.ID, extracted); err != nil {
		t.Fatal(err)
	}
	segment, err := s.RemuxSubtitle(ctx, Remux{Session: "session", Version: version}, 1, 0)
	if err != nil || !strings.Contains(string(segment), "Extracted") {
		t.Fatalf("segment: %v\n%s", err, segment)
	}
	var indexes int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM media_subtitle_index WHERE version_id = $1", version.ID).Scan(&indexes); err != nil || indexes != 0 {
		t.Errorf("the index was read: %d rows, %v", indexes, err)
	}
}

// A slow host holds up its own track reads only.
func TestTrackReadsWaitForTheirOwnHostOnly(t *testing.T) {
	var slots hostSlots
	ctx := t.Context()
	for range hostTrackReads {
		if _, err := slots.acquire(ctx, "slow.example"); err != nil {
			t.Fatal(err)
		}
	}
	release, err := slots.acquire(ctx, "fast.example")
	if err != nil {
		t.Fatalf("a read from another host: %v", err)
	}
	release()
	waiting, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, err := slots.acquire(waiting, "slow.example"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("one more read from the slow host: %v", err)
	}
}

// A track read stops at its share of requests, and the track is not
// offered again: a host that serves one range at a time would take a
// request for nearly every block, hundreds for a film. Here the share
// leaves only the check of a Cluster.
func TestTrackReadsStopAtTheirShareOfRequests(t *testing.T) {
	share := trackRequests
	trackRequests = 1
	t.Cleanup(func() { trackRequests = share })
	s, opener, version, analysis := subtitled(t)
	ctx := t.Context()
	const srtStream = 1
	if !s.SubtitlesLocated(ctx, version, analysis)[srtStream] {
		t.Fatal("the track is not located")
	}
	before := opener.requests.Load()
	if _, err := s.SubtitleTrack(ctx, version, analysis, srtStream); !errors.Is(err, errTrackRequests) {
		t.Fatalf("read past its share: %v", err)
	}
	if made := opener.requests.Load() - before; made > 4 {
		t.Errorf("%d requests for a read of one", made)
	}
	s.locations.Delete(version.ID)
	s.untracked.Delete(trackKey{version.ID, srtStream})
	if s.SubtitlesLocated(ctx, version, analysis)[srtStream] {
		t.Error("the track is offered again")
	}
}

func sameCue(a, b subtitles.Cue) bool {
	return a.Start == b.Start && a.End == b.End && slices.Equal(a.Lines, b.Lines)
}

func TestStreamsPairWithTheTracksFFmpegMadeThemFrom(t *testing.T) {
	tracks := []container.Track{
		{Number: 1, Type: trackVideo, CodecID: "V_MPEG4/ISO/AVC"},
		{Number: 2, Type: trackAudio, CodecID: "A_AAC"},
		// FFmpeg makes no stream of a track that names no codec.
		{Number: 3, Type: trackSubtitle},
		{Number: 4, Type: trackSubtitle, CodecID: "S_TEXT/UTF8"},
		{Number: 5, Type: trackSubtitle, CodecID: "S_TEXT/SSA"},
		{Number: 6, Type: trackSubtitle, CodecID: "S_HDMV/PGS"},
	}
	analysis := media.Analysis{Format: "matroska,webm", Streams: []media.Stream{
		{Index: 0, Type: "video"}, {Index: 1, Type: "audio"},
		{Index: 2, Type: "subtitle", Codec: "subrip"}, {Index: 3, Type: "subtitle", Codec: "ass"},
		{Index: 4, Type: "subtitle", Codec: "hdmv_pgs_subtitle"},
		// Attachments follow the tracks.
		{Index: 5, Type: "attachment", Codec: "ttf", FileName: "Sign.ttf"},
	}}
	pairs := streamTracks(analysis, tracks)
	for stream, number := range map[int]uint64{0: 1, 1: 2, 2: 4, 3: 5, 4: 6} {
		if pairs[stream].Number != number {
			t.Errorf("stream %d: track %d, want %d", stream, pairs[stream].Number, number)
		}
	}
	if len(pairs) != 5 {
		t.Errorf("pairs: %v", pairs)
	}

	// Disagreeing on a track pairs nothing: a track is never read for
	// another.
	swapped := media.Analysis{Format: "matroska,webm", Streams: []media.Stream{
		{Index: 0, Type: "video"}, {Index: 1, Type: "audio"},
		{Index: 2, Type: "subtitle", Codec: "ass"}, {Index: 3, Type: "subtitle", Codec: "subrip"},
		{Index: 4, Type: "subtitle", Codec: "hdmv_pgs_subtitle"},
	}}
	if pairs := streamTracks(swapped, tracks); pairs != nil {
		t.Errorf("swapped codecs paired: %v", pairs)
	}
	if pairs := streamTracks(media.Analysis{Streams: analysis.Streams[:3]}, tracks); pairs != nil {
		t.Errorf("missing streams paired: %v", pairs)
	}
}

func TestTracksBecomeTheFilesTheirBlocksMake(t *testing.T) {
	seconds := func(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }
	text := []container.Block{
		{Start: seconds(1), Duration: seconds(1.5), Data: []byte("Hello\r\n<i>there</i>")},
		{Start: seconds(5), Data: []byte("No duration")},
		{Start: seconds(9), Duration: seconds(1), Data: []byte("   ")},
		{Start: seconds(12), Duration: seconds(1), Data: []byte("Last\n")},
	}
	srt, err := trackFile(container.Track{CodecID: "S_TEXT/UTF8"}, text)
	want := "1\n00:00:01,000 --> 00:00:02,500\nHello\n<i>there</i>\n\n" +
		// A block without a duration lasts until the next one.
		"2\n00:00:05,000 --> 00:00:09,000\nNo duration\n\n" +
		"3\n00:00:12,000 --> 00:00:13,000\nLast\n"
	if err != nil || srt.Format != "srt" || string(srt.Data) != want {
		t.Errorf("SubRip: %s %q %v\nwant %q", srt.Format, srt.Data, err, want)
	}
	// A WebM WebVTT block begins with the cue's identifier and settings.
	framed := []container.Block{{Start: seconds(1), Duration: seconds(1.5), Data: []byte("cue-1\nalign:start\nHello\r\n<i>there</i>")},
		{Start: seconds(3), Duration: seconds(1), Data: []byte("\n\nPlain")}}
	wantVTT := "00:00:01.000 --> 00:00:02.500\nHello\n<i>there</i>\n\n00:00:03.000 --> 00:00:04.000\nPlain\n"
	if vtt, err := trackFile(container.Track{CodecID: "D_WEBVTT/SUBTITLES"}, framed); err != nil || vtt.Format != "vtt" ||
		!strings.HasPrefix(string(vtt.Data), "WEBVTT") || !strings.HasSuffix(string(vtt.Data), wantVTT) {
		t.Errorf("WebVTT: %s %q %v", vtt.Format, vtt.Data, err)
	}

	// An ASS track's script, header and styles kept, events in read order.
	header := "[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize\nStyle: Sign,Sign Font,30\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
	script, err := trackFile(container.Track{CodecID: "S_TEXT/ASS", CodecPrivate: []byte(header)}, []container.Block{
		{Start: seconds(4), Duration: seconds(5), Data: []byte("1,1,Sign,,0,0,0,,{\\pos(320,40)}SIGN")},
		{Start: seconds(1), Duration: seconds(1.5), Data: []byte("0,0,Default,,0,0,0,,{\\i1}Hello{\\i0}\\Nthere")},
	})
	wantScript := header +
		"Dialogue: 0,0:00:01.00,0:00:02.50,Default,,0,0,0,,{\\i1}Hello{\\i0}\\Nthere\n" +
		"Dialogue: 1,0:00:04.00,0:00:09.00,Sign,,0,0,0,,{\\pos(320,40)}SIGN\n"
	if err != nil || script.Format != "ass" || string(script.Data) != wantScript {
		t.Errorf("ASS: %s %v\n%s\nwant\n%s", script.Format, err, script.Data, wantScript)
	}

	if _, err := trackFile(container.Track{CodecID: "S_HDMV/PGS"}, text); err == nil {
		t.Error("an image track made a text file")
	}
}

func TestAttachedFilesAreFoundByNameAndOrder(t *testing.T) {
	analysis := media.Analysis{Streams: []media.Stream{
		{Index: 0, Type: "video"},
		{Index: 1, Type: "attachment", FileName: "Sign.ttf"},
		{Index: 2, Type: "attachment", FileName: "Other.ttf"},
		{Index: 3, Type: "attachment", FileName: "Sign.ttf"},
		{Index: 4, Type: "video", AttachedPicture: true, FileName: "cover.jpg"},
		{Index: 5, Type: "attachment"},
	}}
	for index, want := range map[int]struct {
		name   string
		before int
		ok     bool
	}{
		1: {"Sign.ttf", 0, true}, 2: {"Other.ttf", 0, true}, 3: {"Sign.ttf", 1, true},
		4: {"cover.jpg", 0, true}, 0: {"", 0, false}, 5: {"", 0, false}, 9: {"", 0, false},
	} {
		name, before, ok := attachedFile(analysis, index)
		if name != want.name || before != want.before || ok != want.ok {
			t.Errorf("stream %d: %q %d %v, want %+v", index, name, before, ok, want)
		}
	}
}
