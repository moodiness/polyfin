package jellyfin

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/media"
)

// sentVideo is what ffprobe reads of the video of a segment.
type sentVideo struct {
	Width, Height int
	Profile       string
}

// probeSegment reads the video of an HLS segment with ffprobe.
func probeSegment(t *testing.T, ffprobe string, segment []byte) sentVideo {
	t.Helper()
	path := filepath.Join(t.TempDir(), "segment.ts")
	if err := os.WriteFile(path, segment, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(ffprobe, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height,profile", "-of", "json", path).Output()
	var probed struct{ Streams []sentVideo }
	if err != nil || json.Unmarshal(out, &probed) != nil || len(probed.Streams) != 1 {
		t.Fatalf("probe the segment: %v %s", err, out)
	}
	return probed.Streams[0]
}

// An app whose profile limits the video it decodes gets the video it
// converts for it within those limits, as Jellyfin converts it: a codec
// profile's Height or Width, required or not, sizes the picture down,
// its bitrate following the smaller picture; a codec profile of a codec
// the app is not sent leaves it as it is; a VideoProfile condition
// allowing only Main and Baseline gets Main. The TranscodingUrl, the
// master playlist and the video FFmpeg writes agree.
func TestConversionKeepsWithinTheAppsCodecProfiles(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: Polyfin converts video with the encoders FFmpeg has")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	if _, err := os.Stat(ffprobe); err != nil {
		t.Skip("no ffprobe next to POLYFIN_TEST_FFMPEG")
	}
	// Twelve seconds of 1080p H.264 High and AAC.
	source := filepath.Join(t.TempDir(), "movie.mkv")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "12", "-c:v", "libx264", "-preset", "ultrafast", "-profile:v", "high",
		"-g", "48", "-pix_fmt", "yuv420p", "-b:v", "8M", "-c:a", "aac", "-b:a", "128k", source).CombinedOutput(); err != nil {
		t.Fatalf("make the source: %v: %s", err, out)
	}
	probe, err := exec.Command(ffprobe, "-v", "error", "-show_format", "-show_streams", "-show_chapters", "-of", "json", source).Output()
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := media.Parse(probe)
	if err != nil {
		t.Fatal(err)
	}
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), downloadAddon(t, &heldFile{path: source}), false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("items: %+v", page.Items)
	}
	movie := page.Items[0].Id
	id := mustID(t, movie)
	versions, err := s.library.Versions(t.Context(), member, id)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions: %v %v", versions, err)
	}
	data, _ := json.Marshal(analysis)
	if _, err := s.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", versions[0].ID, data); err != nil {
		t.Fatal(err)
	}

	condition := func(property, value string, required bool) map[string]any {
		return map[string]any{"Condition": "LessThanEqual", "Property": property, "Value": value, "IsRequired": required}
	}
	codec := func(name string, conditions ...map[string]any) map[string]any {
		return map[string]any{"Type": "Video", "Codec": name, "Conditions": conditions}
	}
	// A native player's profile, as the upgrade check's: H.264 and HEVC in
	// Matroska and MP4 as they are, H.264 in MPEG-TS over HLS converted.
	profile := func(codecs ...map[string]any) json.RawMessage {
		data, _ := json.Marshal(map[string]any{
			"Name": "native", "MaxStreamingBitrate": 120_000_000,
			"DirectPlayProfiles": []map[string]any{{"Container": "mkv,mp4", "Type": "Video", "VideoCodec": "h264,hevc", "AudioCodec": "aac,ac3"}},
			"TranscodingProfiles": []map[string]any{{"Container": "ts", "Type": "Video", "VideoCodec": "h264", "AudioCodec": "aac,ac3",
				"Protocol": "hls", "Context": "Streaming", "MaxAudioChannels": "6", "MinSegments": "1", "BreakOnNonKeyFrames": true}},
			"CodecProfiles": codecs, "SubtitleProfiles": []map[string]any{},
		})
		return data
	}
	bandwidth := regexp.MustCompile(`BANDWIDTH=(\d+)`)
	resolution := regexp.MustCompile(`RESOLUTION=(\d+x\d+)`)
	codecs := regexp.MustCompile(`CODECS="(avc1\.[0-9A-F]{4})`)
	type sent struct {
		capped     []string
		resolution string
		bandwidth  int
		codec      string
		video      sentVideo
	}
	play := func(t *testing.T, profile json.RawMessage, more map[string]any) sent {
		t.Helper()
		answer := s.ask(t, token, movie, profile, more)
		if len(answer.MediaSources) != 1 || answer.MediaSources[0].TranscodingUrl == "" {
			t.Fatalf("PlaybackInfo: %+v", answer)
		}
		transcoding := answer.MediaSources[0].TranscodingUrl
		var got sent
		for _, name := range []string{"MaxWidth", "MaxHeight", "h264-profile"} {
			if value := regexp.MustCompile(`[?&]` + name + `=([^&]*)`).FindStringSubmatch(transcoding); value != nil {
				got.capped = append(got.capped, name+"="+value[1])
			}
		}
		status, master := s.call(http.MethodGet, transcoding, "", nil)
		if status != http.StatusOK {
			t.Fatalf("master: %d %s", status, master)
		}
		if m := resolution.FindSubmatch(master); m != nil {
			got.resolution = string(m[1])
		}
		if m := bandwidth.FindSubmatch(master); m != nil {
			got.bandwidth, _ = strconv.Atoi(string(m[1]))
		}
		if m := codecs.FindSubmatch(master); m != nil {
			got.codec = string(m[1])
		}
		base := transcoding[:strings.Index(transcoding, "master.m3u8")]
		var media string
		for line := range strings.Lines(string(master)) {
			if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
				media = base + line
				break
			}
		}
		status, playlist := s.call(http.MethodGet, media, "", nil)
		if status != http.StatusOK {
			t.Fatalf("media playlist: %d %s", status, playlist)
		}
		for line := range strings.Lines(string(playlist)) {
			if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
				status, segment := s.call(http.MethodGet, base+line, "", nil)
				if status != http.StatusOK {
					t.Fatalf("segment: %d %s", status, segment)
				}
				got.video = probeSegment(t, ffprobe, segment)
				break
			}
		}
		return got
	}

	// A codec profile of HEVC, which the app is not sent: the video,
	// converted as the app asks rather than played or copied, keeps its
	// size.
	whole := play(t, profile(codec("hevc", condition("Height", "720", true))), map[string]any{"EnableDirectPlay": false, "AllowVideoStreamCopy": false})
	if whole.capped != nil || whole.resolution != "1920x1080" || whole.video != (sentVideo{1920, 1080, "High"}) {
		t.Fatalf("another codec's cap: %+v", whole)
	}
	for _, test := range []struct {
		name    string
		profile json.RawMessage
		capped  string
	}{
		{"a height cap", profile(codec("h264", condition("Height", "720", true))), "MaxHeight=720"},
		{"a width cap", profile(codec("h264", condition("Width", "1280", true))), "MaxWidth=1280"},
		{"an optional height cap", profile(codec("h264", condition("Height", "720", false))), "MaxHeight=720"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := play(t, test.profile, nil)
			if len(got.capped) != 1 || got.capped[0] != test.capped || got.resolution != "1280x720" || got.video != (sentVideo{1280, 720, "High"}) {
				t.Errorf("%+v", got)
			}
			if got.bandwidth <= 0 || got.bandwidth >= whole.bandwidth {
				t.Errorf("bandwidth %d, uncapped %d", got.bandwidth, whole.bandwidth)
			}
		})
	}
	t.Run("the profiles allowed", func(t *testing.T) {
		only := profile(codec("h264", map[string]any{"Condition": "EqualsAny", "Property": "VideoProfile", "Value": "main|baseline", "IsRequired": false}))
		got := play(t, only, nil)
		if got.resolution != "1920x1080" || !strings.HasPrefix(got.codec, "avc1.4D") || got.video != (sentVideo{1920, 1080, "Main"}) {
			t.Errorf("%+v", got)
		}
	})
}
