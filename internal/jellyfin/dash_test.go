package jellyfin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
)

// The ClearKey key of the encrypted test stream, and its key ID.
const (
	dashKID = "0123456789abcdef0123456789abcdef"
	dashKey = "00112233445566778899aabbccddeeff"
	// wrongKey is another key for the same key ID.
	wrongKey = "ffeeddccbbaa99887766554433221100"
	// licenceToken is in the licence server's address, a secret too.
	licenceToken = "licence-token-a1b2c3"
)

const (
	// dashSegment is how long a segment of the test streams lasts: two
	// seconds, as providers' last at least. Refreshing a live manifest,
	// FFmpeg's DASH demuxer rounds the start of the segment it reads down
	// to a whole second (dashdec.c, refresh_manifest): with segments of a
	// second, the first audio one a little shorter, it asks that one again
	// and again, and the stream never plays.
	dashSegment = 2 * time.Second
	// dashSegments are the segments of a test stream, a minute of it.
	dashSegments = 30
	// dashLead are the segments a test stream has when it is first asked
	// for, its window of 20 seconds; the next ones come one every
	// dashSegment, as a live source's.
	dashLead = 10
)

// dashFFmpeg returns the FFmpeg of POLYFIN_TEST_FFMPEG and the ffprobe
// beside it, skipping the test without them, or when their DASH demuxer
// is missing or takes no keys: FFmpeg built without libxml2 has none, as
// Homebrew's; the builds Polyfin ships have it.
func dashFFmpeg(t *testing.T) (string, string) {
	t.Helper()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	if out, err := exec.Command(ffprobe, "-hide_banner", "-h", "demuxer=dash").Output(); err != nil || !bytes.Contains(out, []byte("cenc_decryption_keys")) {
		t.Skip("the FFmpeg of POLYFIN_TEST_FFMPEG has no DASH demuxer that takes keys")
	}
	return ffmpeg, ffprobe
}

// dashStream makes a minute of a test pattern and a tone into a DASH
// stream of dashSegment segments in a folder, with FFmpeg's DASH muxer;
// encrypted when encrypted is set, CENC (AES-CTR) with dashKey under
// dashKID.
func dashStream(t *testing.T, ffmpeg string, encrypted bool) string {
	t.Helper()
	dir := t.TempDir()
	seconds := strconv.Itoa(dashSegments * int(dashSegment/time.Second))
	args := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25:duration=" + seconds,
		"-f", "lavfi", "-i", "sine=duration=" + seconds, "-c:v", "libx264", "-preset", "ultrafast", "-g", strconv.Itoa(25 * int(dashSegment/time.Second)),
		"-pix_fmt", "yuv420p", "-c:a", "aac", "-f", "dash", "-seg_duration", strconv.Itoa(int(dashSegment / time.Second)), "-use_template", "1",
		"-use_timeline", "1"}
	if encrypted {
		args = append(args, "-format_options", "encryption_scheme=cenc-aes-ctr:encryption_key="+dashKey+":encryption_kid="+dashKID)
	}
	if output, err := exec.Command(ffmpeg, append(args, filepath.Join(dir, "manifest.mpd"))...).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	return dir
}

// liveManifest makes FFmpeg's static manifest a live one, whose timelines
// list the first available segments, and names, when encrypted, the key
// ID of its tracks and the systems that decrypt them, as providers' do.
func liveManifest(data []byte, encrypted bool, available int) []byte {
	text := regexp.MustCompile(`\s*mediaPresentationDuration="[^"]*"`).ReplaceAllString(string(data), "")
	text = strings.Replace(text, `type="static"`, `type="dynamic" availabilityStartTime="1970-01-01T00:00:00Z" minimumUpdatePeriod="PT2S"`, 1)
	text = dashTimeline.ReplaceAllStringFunc(text, func(timeline string) string {
		var b strings.Builder
		t, n := int64(0), 0
		for _, s := range dashEntry.FindAllStringSubmatch(timeline, -1) {
			if s[2] != "" {
				t, _ = strconv.ParseInt(s[2], 10, 64)
			}
			d, _ := strconv.ParseInt(s[3], 10, 64)
			repeat, _ := strconv.Atoi(s[5])
			for range repeat + 1 {
				if n < available {
					fmt.Fprintf(&b, `<S t="%d" d="%d" />`, t, d)
				}
				t, n = t+d, n+1
			}
		}
		return "<SegmentTimeline>" + b.String() + "</SegmentTimeline>"
	})
	if encrypted {
		text = strings.Replace(text, `xmlns="urn:mpeg:dash:schema:mpd:2011"`, `xmlns="urn:mpeg:dash:schema:mpd:2011" xmlns:cenc="urn:mpeg:cenc:2013"`, 1)
		kid := dashKID[:8] + "-" + dashKID[8:12] + "-" + dashKID[12:16] + "-" + dashKID[16:20] + "-" + dashKID[20:]
		text = regexp.MustCompile(`(<AdaptationSet[^>]*>)`).ReplaceAllString(text, `$1`+
			`<ContentProtection schemeIdUri="urn:mpeg:dash:mp4protection:2011" value="cenc" cenc:default_KID="`+kid+`"/>`+
			`<ContentProtection schemeIdUri="urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed"/>`+
			`<ContentProtection schemeIdUri="urn:uuid:e2719d58-a985-b3c9-781a-b030af78d30e"/>`)
	}
	return []byte(text)
}

// A timeline of FFmpeg's manifests, and its entries.
var (
	dashTimeline = regexp.MustCompile(`(?s)<SegmentTimeline>.*?</SegmentTimeline>`)
	dashEntry    = regexp.MustCompile(`<S (t="(\d+)" )?d="(\d+)"( r="(\d+)")? />`)
)

// dashChunk reads the number of a segment of FFmpeg's DASH muxer.
var dashChunk = regexp.MustCompile(`^chunk-stream\d+-(\d+)\.m4s$`)

// dashProvider serves an M3U list of DASH channels, their streams live,
// clear under /clear/ and encrypted under /enc/, and a ClearKey licence
// server. A stream is live from its first request on, as an origin
// serves one: its first dashLead segments are there, the next ones come
// one every dashSegment; its manifest lists those there, and a segment
// asked before its time is answered 404, and counted in early. It counts
// its requests by the first part of their path.
type dashProvider struct {
	url      string
	mu       sync.Mutex
	requests map[string]int
	early    int
	// started is when each stream was first asked for.
	started map[string]time.Time
	// licences are the licence requests' faults, "" for each one right.
	licences []string
}

func (p *dashProvider) asked(part string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests[part]
}

func newDASHProvider(t *testing.T, ffmpeg string, list func(base string) string, encrypted bool) *dashProvider {
	t.Helper()
	dirs := map[string]string{}
	if encrypted {
		dirs["enc"] = dashStream(t, ffmpeg, true)
	} else {
		dirs["clear"] = dashStream(t, ffmpeg, false)
	}
	p := &dashProvider{requests: map[string]int{}, started: map[string]time.Time{}}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		part, rest, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		p.mu.Lock()
		p.requests[part]++
		if _, ok := p.started[part]; !ok {
			p.started[part] = time.Now()
		}
		started := p.started[part]
		p.mu.Unlock()
		dir, stream := dirs[part]
		// The segments there now.
		available := dashLead + int(time.Since(started)/dashSegment)
		switch {
		case part == "get.php":
			_, _ = io.WriteString(w, list(server.URL))
		case part == "licence":
			p.licence(w, r)
		case stream && rest == "manifest.mpd":
			data, err := os.ReadFile(filepath.Join(dir, rest))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/dash+xml")
			_, _ = w.Write(liveManifest(data, part == "enc", available))
		case stream:
			if match := dashChunk.FindStringSubmatch(rest); match != nil {
				if n, _ := strconv.Atoi(match[1]); n > available {
					p.mu.Lock()
					p.early++
					p.mu.Unlock()
					http.NotFound(w, r)
					return
				}
			}
			http.ServeFile(w, r, filepath.Join(dir, filepath.Base(rest)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	p.url = server.URL + "/get.php"
	return p
}

// licence answers a W3C ClearKey licence request for dashKID with its key,
// as a JSON Web Key set, when it is posted with the channel's headers and
// the licence's own.
func (p *dashProvider) licence(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Kids []string `json:"kids"`
		Type string   `json:"type"`
	}
	kid, _ := hex.DecodeString(dashKID)
	key, _ := hex.DecodeString(dashKey)
	fault := ""
	switch {
	case r.Method != http.MethodPost || r.URL.Query().Get("token") != licenceToken:
		fault = "method or address " + r.Method
	case r.Header.Get("X-Licence") != "yes" || r.Header.Get("Referer") != "https://portal.example/":
		fault = fmt.Sprintf("headers %v", r.Header)
	case json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Kids) != 1 || request.Kids[0] != base64.RawURLEncoding.EncodeToString(kid) ||
		request.Type != "temporary":
		fault = fmt.Sprintf("request %+v", request)
	}
	p.mu.Lock()
	p.licences = append(p.licences, fault)
	p.mu.Unlock()
	if fault != "" {
		http.Error(w, "refused", http.StatusForbidden)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "temporary", "keys": []map[string]string{
		{"kty": "oct", "kid": base64.RawURLEncoding.EncodeToString(kid), "k": base64.RawURLEncoding.EncodeToString(key)}}})
}

// dashSetup is a server with a member signed in, whose M3U source is a
// dashProvider's list, its channels by name. The log keeps every record,
// and the streams' health is saved as the server saves it.
type dashSetup struct {
	s        testServer
	token    string
	source   accounts.ID
	channels map[string]string
	profile  []byte
	provider *dashProvider
}

func newDASHSetup(t *testing.T, ffprobe string, provider *dashProvider) dashSetup {
	t.Helper()
	s := newProbingServer(t, 10, ffprobe)
	s.warnings.all.Store(true)
	s.handler.Playback.LiveSources(s.iptv.Connections, func(ctx context.Context, v library.Version, failure string) {
		_ = s.iptv.ReportStream(ctx, v.Origin.Addon, v.Origin.ID, v.URL, failure)
	})
	s.user("member", nil)
	d := dashSetup{s: s, token: s.signIn("member", "tv"), channels: map[string]string{}, provider: provider}
	addon, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Provider",
		Account: iptv.Account{Kind: addons.KindM3U, URL: provider.url}}, false)
	if err != nil {
		t.Fatal(err)
	}
	d.source = addon.ID
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", d.token, &channels)
	for _, channel := range channels.Items {
		d.channels[channel.Name] = channel.Id
	}
	if d.profile, err = os.ReadFile(filepath.Join(playbackFixtures, "profiles", "jellyfin-web-chrome.json")); err != nil {
		t.Fatal(err)
	}
	return d
}

// playbackInfo asks how a channel plays, for jellyfin-web.
func (d dashSetup) playbackInfo(t *testing.T, name string) playbackInfoResponse {
	t.Helper()
	status, body := d.s.call(http.MethodPost, "/Items/"+d.channels[name]+"/PlaybackInfo", app("tv", d.token),
		map[string]any{"DeviceProfile": json.RawMessage(d.profile), "IsPlayback": true, "AutoOpenLiveStream": true})
	var info playbackInfoResponse
	if status != http.StatusOK || json.Unmarshal(body, &info) != nil {
		t.Fatalf("PlaybackInfo of %s: %d %s", name, status, body)
	}
	return info
}

// health is how the stream of a channel last answered.
func (d dashSetup) health(t *testing.T, name string) iptv.StreamHealth {
	t.Helper()
	id, _ := accounts.ParseID(d.channels[name])
	c, err := d.s.iptv.Channel(t.Context(), addons.Shared(), d.source, id)
	if err != nil || len(c.Streams) != 1 {
		t.Fatalf("the streams of %s: %+v %v", name, c, err)
	}
	return c.Streams[0].Health
}

// plays checks that a channel plays converted into HLS, as jellyfin-web
// plays it: its first segment, after its initialization, decodes without
// an error.
func (d dashSetup) plays(t *testing.T, ffmpeg, name string) {
	t.Helper()
	info := d.playbackInfo(t, name)
	if len(info.MediaSources) != 1 {
		t.Fatalf("%s plays nothing: %+v %q", name, info, d.s.warnings.logged())
	}
	source := info.MediaSources[0]
	if source.SupportsDirectPlay || source.TranscodingUrl == "" || source.TranscodingSubProtocol != "hls" {
		t.Fatalf("%s plays as it is: %+v", name, source)
	}
	get := func(path string) (int, []byte) { return d.s.call(http.MethodGet, path, "", nil) }
	status, master := get(source.TranscodingUrl)
	if status != http.StatusOK || !strings.Contains(string(master), "live.m3u8?") {
		t.Fatalf("%s's master playlist: %d %s", name, status, master)
	}
	base := source.TranscodingUrl[:strings.Index(source.TranscodingUrl, "master.m3u8")]
	query := source.TranscodingUrl[strings.Index(source.TranscodingUrl, "?"):]
	status, media := get(base + "live.m3u8" + query)
	init := regexp.MustCompile(`URI="(hls/live/[^"]+)"`).FindStringSubmatch(string(media))
	if status != http.StatusOK || init == nil || !strings.Contains(string(media), "#EXTINF") {
		t.Fatalf("%s's live playlist: %d %s", name, status, media)
	}
	var first string
	for line := range strings.Lines(string(media)) {
		if strings.HasPrefix(line, "hls/live/") {
			first = strings.TrimSpace(line)
			break
		}
	}
	_, header := get(base + init[1])
	status, segment := get(base + first)
	if status != http.StatusOK || len(header) == 0 || len(segment) == 0 {
		t.Fatalf("%s's first segment %s: %d", name, first, status)
	}
	played := filepath.Join(t.TempDir(), "played.mp4")
	if err := os.WriteFile(played, append(header, segment...), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(ffmpeg, "-v", "error", "-xerror", "-i", played, "-f", "null", "-").CombinedOutput(); err != nil || len(output) > 0 {
		t.Errorf("%s's first segment does not decode: %v %s", name, err, output)
	}
	session := source.TranscodingUrl[strings.Index(source.TranscodingUrl, "PlaySessionId=")+len("PlaySessionId="):]
	session = session[:strings.Index(session, "&")]
	if status, _ := d.s.call(http.MethodDelete, "/Videos/ActiveEncodings?playSessionId="+session, app("tv", d.token), nil); status != http.StatusNoContent {
		t.Fatalf("stopping %s: %d", name, status)
	}
	// FFmpeg reads the stream at its pace: it never asked a segment before
	// its time, which its source answers 404, and which would end the
	// stream, read again a minute behind.
	d.provider.mu.Lock()
	early := d.provider.early
	d.provider.mu.Unlock()
	if early > 0 {
		t.Errorf("%s: %d segments asked before their time", name, early)
	}
	if h := d.health(t, name); h.Failure != "" || h.OKAt == nil {
		t.Errorf("%s's health: %+v", name, h)
	}
}

// keysUnlogged checks that the keys, as lists and licence servers give
// them, and the licence server's address, show nowhere in the log.
func (d dashSetup) keysUnlogged(t *testing.T) {
	t.Helper()
	secrets := []string{dashKey, strings.ToUpper(dashKey), wrongKey, licenceToken}
	for _, k := range []string{dashKey, wrongKey} {
		raw, _ := hex.DecodeString(k)
		secrets = append(secrets, base64.RawURLEncoding.EncodeToString(raw), base64.StdEncoding.EncodeToString(raw))
	}
	w := d.s.warnings
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.messages) == 0 {
		t.Error("nothing was logged")
	}
	for i, message := range w.messages {
		line := message
		for key, value := range w.attrs[i] {
			line += " " + key + "=" + value
		}
		for _, secret := range secrets {
			if strings.Contains(line, secret) {
				t.Errorf("a key in the log: %s", line)
			}
		}
	}
}

// A DASH channel of an M3U list plays converted into HLS, a live stream
// read as it comes.
func TestDASHChannelPlays(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := dashFFmpeg(t)
	provider := newDASHProvider(t, ffmpeg, func(base string) string {
		return "#EXTM3U\n#EXTINF:-1,Zeb Clear\n" + base + "/clear/manifest.mpd\n"
	}, false)
	d := newDASHSetup(t, ffprobe, provider)
	d.plays(t, ffmpeg, "Zeb Clear")
}

// ClearKey channels play decrypted with the keys their list gives, or
// those its licence server answers, asked with the channel's headers and
// the licence's own; the keys never show in the log.
func TestClearKeyChannelsPlay(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := dashFFmpeg(t)
	provider := newDASHProvider(t, ffmpeg, func(base string) string {
		encrypted := base + "/enc/manifest.mpd\n"
		return "#EXTM3U\n" +
			"#EXTINF:-1 tvg-chno=\"1\",Orbe Keys\n" +
			"#KODIPROP:inputstream=inputstream.adaptive\n" +
			"#KODIPROP:inputstream.adaptive.manifest_type=mpd\n" +
			"#KODIPROP:inputstream.adaptive.license_type=org.w3.clearkey\n" +
			"#KODIPROP:inputstream.adaptive.license_key=" + dashKID + ":" + dashKey + "\n" + encrypted +
			"#KODIPROP:inputstream.adaptive.license_type=clearkey\n" +
			"#KODIPROP:inputstream.adaptive.license_key=" + base + "/licence?token=" + licenceToken + "|X-Licence=yes\n" +
			"#EXTINF:-1 tvg-chno=\"2\",Kite Licence\n" +
			"#EXTVLCOPT:http-referrer=https://portal.example/\n" + encrypted
	}, true)
	d := newDASHSetup(t, ffprobe, provider)
	d.plays(t, ffmpeg, "Orbe Keys")
	d.plays(t, ffmpeg, "Kite Licence")
	provider.mu.Lock()
	licences := provider.licences
	provider.mu.Unlock()
	if len(licences) == 0 || strings.Join(licences, "") != "" {
		t.Errorf("licence requests: %q", licences)
	}
	d.keysUnlogged(t)
}

// A wrong key, a Widevine licence, or an encrypted stream whose list gives
// no keys, are told as such failures, once, and not asked again as a
// network failure would be; the Widevine stream's source is never asked.
// The keys never show in the log.
func TestDASHChannelsThatCannotPlay(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := dashFFmpeg(t)
	provider := newDASHProvider(t, ffmpeg, func(base string) string {
		encrypted := base + "/enc/manifest.mpd\n"
		return "#EXTM3U\n" +
			"#EXTINF:-1 tvg-chno=\"1\",Plume Wrong\n" +
			"#KODIPROP:inputstream.adaptive.license_type=org.w3.clearkey\n" +
			"#KODIPROP:inputstream.adaptive.license_key=" + dashKID + ":" + wrongKey + "\n" + encrypted +
			"#EXTINF:-1 tvg-chno=\"2\",Moss Widevine\n" +
			"#KODIPROP:inputstream.adaptive.license_type=com.widevine.alpha\n" +
			"#KODIPROP:inputstream.adaptive.license_key=" + base + "/widevine/licence\n" + base + "/widevine/manifest.mpd\n" +
			"#EXTINF:-1 tvg-chno=\"3\",Fern Locked\n" + encrypted
	}, true)
	d := newDASHSetup(t, ffprobe, provider)
	for _, tc := range []struct{ name, failure, says string }{
		{"Plume Wrong", iptv.FailureKeys, "ClearKey keys do not decrypt"},
		{"Moss Widevine", iptv.FailureDRM, "needs a DRM Polyfin does not support: its list names a Widevine licence"},
		{"Fern Locked", iptv.FailureDRM, "needs a DRM Polyfin does not support: it is encrypted (Widevine, ClearKey), and its list gives no ClearKey keys"},
	} {
		if info := d.playbackInfo(t, tc.name); len(info.MediaSources) != 0 {
			t.Errorf("%s played: %+v", tc.name, info.MediaSources)
		}
		if h := d.health(t, tc.name); h.Failure != tc.failure || h.Failures != 1 || h.HiddenUntil == nil {
			t.Errorf("%s's health: %+v, want %s", tc.name, h, tc.failure)
		}
		told := 0
		for _, attrs := range d.s.warnings.attrsOf("A channel's stream failed") {
			if strings.Contains(attrs["error"], tc.says) {
				told++
			}
		}
		if told != 1 {
			t.Errorf("%s told %d times: %v", tc.name, told, d.s.warnings.attrsOf("A channel's stream failed"))
		}
		if info := d.playbackInfo(t, tc.name); len(info.MediaSources) != 0 {
			t.Errorf("%s played again: %+v", tc.name, info.MediaSources)
		}
		if h := d.health(t, tc.name); h.Failures != 1 {
			t.Errorf("%s asked again: %+v", tc.name, h)
		}
	}
	if n := provider.asked("widevine"); n != 0 {
		t.Errorf("the Widevine channel's source was asked %d times", n)
	}
	d.keysUnlogged(t)
}
