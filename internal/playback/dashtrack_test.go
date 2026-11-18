package playback

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
)

// dashTrack encodes four seconds of AAC as DASH with FFmpeg, twice, and
// returns each layout's files by their names, its manifest, manifest.mpd,
// included: files named by a template, and one file read by byte ranges.
func dashTrack(t *testing.T, ffmpeg string) (templated, ranged map[string][]byte) {
	t.Helper()
	layouts := map[string][]string{
		"templated": {"-use_template", "1", "-use_timeline", "0", "-init_seg_name", "init.m4s", "-media_seg_name", "chunk-$Number$.m4s"},
		"ranged":    {"-single_file", "1", "-global_sidx", "1"},
	}
	read := map[string]map[string][]byte{}
	for layout, options := range layouts {
		dir := t.TempDir()
		args := slices.Concat([]string{"-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=4", "-c:a", "aac", "-f", "dash", "-seg_duration", "2"},
			options, []string{dashOutput(dir)})
		if output, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
			t.Fatalf("encoding: %v %s", err, output)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		read[layout] = map[string][]byte{}
		for _, entry := range entries {
			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			read[layout][entry.Name()] = data
		}
	}
	return read["templated"], read["ranged"]
}

// trackHost is the public host of a track's DASH stream: it serves its
// files by their names, byte ranges included, whatever their folder.
type trackHost struct {
	files map[string][]byte
}

func (h trackHost) Open(ctx context.Context, method, target string, header http.Header, _ bool) (*http.Response, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	name := parsed.Path[strings.LastIndexByte(parsed.Path, '/')+1:]
	data, ok := h.files[name]
	if !ok {
		return sized(http.StatusNotFound, "text/plain", []byte("no such file")), nil
	}
	request := httptest.NewRequestWithContext(ctx, method, target, nil)
	request.Header = header.Clone()
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "audio/mp4")
	if strings.HasSuffix(name, ".mpd") {
		recorder.Header().Set("Content-Type", "application/dash+xml")
	}
	http.ServeContent(recorder, request, "", time.Time{}, bytes.NewReader(data))
	response := recorder.Result()
	response.Request = request
	return response, nil
}

// dashService is a service whose ffprobe and FFmpeg read DASH.
func dashService(t *testing.T, opener *confiningOpener, ffmpeg, ffprobe string) *Service {
	t.Helper()
	s := newService(t, opener, ffprobe, nil)
	segments, err := hls.NewManager(ffmpeg, t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(segments.Close)
	s.segments = segments
	return s
}

// readDirectly is what FFmpeg itself reads of a track's DASH files, from
// start, when a host serves them to it as they are: its audio copied
// into ADTS frames, as ConvertAudio copies it.
func readDirectly(t *testing.T, ffmpeg string, files map[string][]byte, start time.Duration) []byte {
	t.Helper()
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := files[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
	}))
	defer host.Close()
	args := []string{"-v", "error", "-nostdin"}
	if start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(start.Seconds(), 'f', 3, 64))
	}
	args = append(args, "-i", host.URL+"/manifest.mpd", "-map", "0:a:0", "-c:a", "copy", "-f", "adts", "pipe:1")
	read, err := exec.CommandContext(t.Context(), ffmpeg, args...).Output()
	if err != nil {
		t.Fatalf("FFmpeg reading the track itself: %v", err)
	}
	return read
}

// A track whose stream is a DASH manifest is read as a channel's is:
// through the loopback interface, which rewrites the manifest, so that
// every file of it is asked of Polyfin, with the headers the source needs
// and within its confinement. For a source confined to public addresses
// as for one that is not, FFmpeg reads of it what it reads of the same
// files itself, from the start and further in, whether a template names
// its files, byte ranges cut one file, or one file is read whole.
func TestADASHTrackPlaysThroughTheLoopbackInterface(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := dashFFprobe(t)
	templated, ranged := dashTrack(t, ffmpeg)
	// One file read whole, as a manifest of a SegmentBase tells it: its
	// initialization's range kept, the segments' left to the file's index.
	indexed := map[string][]byte{"manifest-stream0.mp4": ranged["manifest-stream0.mp4"]}
	list := string(ranged["manifest.mpd"])
	from, to := strings.Index(list, "<SegmentList"), strings.Index(list, "</SegmentList>")
	initialization := regexp.MustCompile(`<Initialization range="[0-9]+-[0-9]+" */>`).FindString(list)
	if from < 0 || to < from || initialization == "" {
		t.Fatalf("FFmpeg's manifest of one file has no segment list:\n%s", list)
	}
	indexed["manifest.mpd"] = []byte(list[:from] + `<SegmentBase>` + initialization + `</SegmentBase>` + list[to+len("</SegmentList>"):])
	for _, layout := range []struct {
		name  string
		files map[string][]byte
		media []string
	}{
		{"files a template names", templated, []string{"init.m4s", "chunk-1.m4s", "chunk-2.m4s"}},
		{"one file by byte ranges", ranged, []string{"manifest-stream0.mp4"}},
		{"one file by its index", indexed, []string{"manifest-stream0.mp4"}},
	} {
		for _, confined := range []bool{false, true} {
			name := layout.name + ", of a source that may reach any address"
			if confined {
				name = layout.name + ", of a source confined to public addresses"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				local := newLocalHost(t, nil)
				opener := &confiningOpener{next: trackHost{layout.files}}
				s := dashService(t, opener, ffmpeg, ffprobe)
				version := library.Version{ID: accounts.ID{0xA2, 1}, Name: "Track", URL: "https://music.example/album/1/manifest.mpd?token=t",
					Confined: confined, Headers: map[string]string{"Authorization": "Bearer of the track"},
					Audio: &library.AudioSource{Manifest: "dash", Codec: "aac"}}
				// What FFmpeg reads of the same files itself, as a track of a
				// source that may reach any address played before.
				wholeItself, laterItself := readDirectly(t, ffmpeg, layout.files, 0), readDirectly(t, ffmpeg, layout.files, 2*time.Second)
				if len(wholeItself) < 10_000 || !bytes.HasPrefix(wholeItself, []byte{0xff, 0xf1}) {
					t.Fatalf("FFmpeg itself reads %d bytes of the track, starting % x", len(wholeItself), wholeItself[:min(4, len(wholeItself))])
				}
				var whole, later bytes.Buffer
				if err := s.ConvertAudio(t.Context(), version, nil, "adts", 0, &whole); err != nil {
					t.Fatalf("the track from its start: %v", err)
				}
				if err := s.ConvertAudio(t.Context(), version, nil, "adts", 2*time.Second, &later); err != nil {
					t.Fatalf("the track from two seconds in: %v", err)
				}
				if !bytes.Equal(whole.Bytes(), wholeItself) {
					t.Errorf("the track from its start is %d bytes, where FFmpeg itself reads %d of its files", whole.Len(), len(wholeItself))
				}
				if !bytes.Equal(later.Bytes(), laterItself) {
					t.Errorf("the track from two seconds in is %d bytes, where FFmpeg itself reads %d of its files", later.Len(), len(laterItself))
				}
				if asked := local.requests(); len(asked) > 0 {
					t.Errorf("a host other than the track's was asked: %q", asked)
				}
				requests := opener.requests()
				for _, file := range slices.Concat([]string{"manifest.mpd?token=t"}, layout.media) {
					if !slices.ContainsFunc(requests, func(request opened) bool { return request.target == "https://music.example/album/1/"+file }) {
						t.Errorf("Polyfin was not asked for %s", file)
					}
				}
				for _, request := range requests {
					if request.confined != confined || request.header.Get("Authorization") != "Bearer of the track" {
						t.Errorf("%s was asked confined: %t, with %q", request.target, request.confined, request.header.Get("Authorization"))
					}
				}

				// A track whose addon tells no codec is analyzed the same way.
				unknown := version
				unknown.ID, unknown.Audio = accounts.ID{0xA2, 2}, &library.AudioSource{Manifest: "dash"}
				analysis, err := s.AnalyzeAudio(t.Context(), unknown)
				if err != nil {
					t.Fatalf("the analysis: %v", err)
				}
				if !DASH(analysis) || len(analysis.Streams) != 1 || analysis.Streams[0].Codec != "aac" {
					t.Errorf("the analysis kept is not of the track: %+v", analysis)
				}
			})
		}
	}
}

// A track's manifest that names a host of the local network is read
// within its source's confinement: for a source confined to public
// addresses, Polyfin refuses what FFmpeg asks it for, the track does not
// play, and the host is asked nothing. A source that may reach any
// address plays it from there.
func TestADASHTrackIsReadWithinItsSourcesConfinement(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := dashFFprobe(t)
	templated, _ := dashTrack(t, ffmpeg)
	for _, tc := range []struct {
		name string
		// manifest is the track's, given the local host's address.
		manifest func(local string) string
	}{
		{"in a BaseURL", func(local string) string {
			return strings.Replace(string(templated["manifest.mpd"]), "<SegmentTemplate", "<BaseURL>"+local+"/music/</BaseURL>\n<SegmentTemplate", 1)
		}},
		{"in a template", func(local string) string {
			return strings.NewReplacer(`initialization="`, `initialization="`+local+`/music/`, `media="`, `media="`+local+`/music/`).Replace(string(templated["manifest.mpd"]))
		}},
	} {
		for _, confined := range []bool{true, false} {
			name := tc.name + ", of a source that may reach any address"
			if confined {
				name = tc.name + ", of a source confined to public addresses"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				local := newLocalHost(t, map[string][]byte{"/music/init.m4s": templated["init.m4s"], "/music/chunk-1.m4s": templated["chunk-1.m4s"],
					"/music/chunk-2.m4s": templated["chunk-2.m4s"]})
				opener := &confiningOpener{next: trackHost{map[string][]byte{"manifest.mpd": []byte(tc.manifest(local.URL))}}}
				s := dashService(t, opener, ffmpeg, ffprobe)
				version := library.Version{ID: accounts.ID{0xA2, 3}, Name: "Track", URL: "https://music.example/album/1/manifest.mpd", Confined: confined,
					Audio: &library.AudioSource{Manifest: "dash", Codec: "aac"}}
				var played bytes.Buffer
				err := s.ConvertAudio(t.Context(), version, nil, "adts", 0, &played)
				if !confined {
					if err != nil || played.Len() < 30_000 {
						t.Errorf("the track plays %d bytes: %v", played.Len(), err)
					}
					if asked := local.requests(); !slices.ContainsFunc(asked, func(request string) bool { return strings.HasPrefix(request, "GET /music/init.m4s by Go-http-client") }) ||
						slices.ContainsFunc(asked, func(request string) bool { return strings.Contains(request, "Lavf") }) {
						t.Errorf("the local host was asked %q, want its files asked by Polyfin alone", asked)
					}
					return
				}
				if err == nil || played.Len() > 0 {
					t.Errorf("the track plays %d bytes: %v", played.Len(), err)
				}
				readThroughPolyfin(t, local, opener, true)
			})
		}
	}
}
