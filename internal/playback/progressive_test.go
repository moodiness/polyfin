package playback

import (
	"slices"
	"testing"
)

// An app saving a title for offline viewing, as Streamyfin's download
// sheet asks, gets it streamed as one file when converted downloads are
// on: copied when it fits the app and the bitrate, else converted within
// the bitrate and the user's quality group, its audio copied when the
// profile lists its codec. Off, the decision is Jellyfin's: no stream.
func TestDownloadsStreamAsOneFile(t *testing.T) {
	t.Parallel()
	// H.264 and AAC in Matroska at 2160p, a PGS track and a SubRip one.
	source := MediaSource{
		Container: "mkv,webm",
		Bitrate:   20_000_000,
		Streams: []MediaStream{
			{Type: "Video", Index: 0, Codec: "h264", Profile: "High", Level: new(51.0), VideoRange: "SDR", VideoRangeType: "SDR",
				Width: new(3840), Height: new(2160), AverageFrameRate: new(24.0), BitDepth: new(8)},
			{Type: "Audio", Index: 1, Codec: "aac", IsDefault: true, Channels: new(2), BitRate: new(int64(192_000))},
			{Type: "Subtitle", Index: 2, Codec: "PGSSUB"},
			{Type: "Subtitle", Index: 3, Codec: "subrip", IsTextSubtitleStream: true},
		},
	}
	download := readDeviceProfile(t, "streamyfin-download")
	software := Capabilities{Encoders: []string{"libx264", "libx265"}}
	decide := func(options Options) Decision {
		options.EnableDirectPlay, options.EnableDirectStream, options.Can = true, true, software
		return Decide(download, source, options)
	}

	// The quality "Max": the file as it is.
	if got := decide(Options{Progressive: true}); !got.DirectPlay || got.Streamed() {
		t.Errorf("at the source's quality: direct play %v, streamed %v", got.DirectPlay, got.Streamed())
	}
	// A lower quality: converted within it, the audio copied.
	low := decide(Options{MaxStreamingBitrate: 2_000_000, Progressive: true})
	if low.DirectPlay || low.HLS || !low.Progressive || low.Video == nil || low.Audio != nil || low.Video.Bitrate > 2_000_000 ||
		!slices.Equal(low.Reasons, []string{"ContainerBitrateExceedsLimit"}) {
		t.Errorf("at 2 Mb/s: %+v, video %+v, audio %+v", low, low.Video, low.Audio)
	}
	// Without converted downloads, nothing is streamed, as from Jellyfin.
	if off := decide(Options{MaxStreamingBitrate: 2_000_000}); off.DirectPlay || off.Streamed() || off.Transcoding == nil {
		t.Errorf("off: %+v", off)
	}
	// Taller than the user's quality group: converted down to it.
	if tall := decide(Options{MaxHeight: 1080, Progressive: true}); !tall.Progressive || tall.Video == nil || tall.Video.Height != 1080 ||
		!slices.Equal(tall.Reasons, []string{"VideoResolutionNotSupported"}) {
		t.Errorf("under a 1080p group: %+v, video %+v", tall, tall.Video)
	}
	// The picture subtitle is burned in; the text one is a file beside.
	if subtitles := decide(Options{MaxStreamingBitrate: 2_000_000, SubtitleStreamIndex: new(2), Progressive: true}).Subtitles; subtitles[2].Method != "Encode" ||
		subtitles[3].Method != "External" {
		t.Errorf("subtitles: %+v", subtitles)
	}
	// Audio the profile does not list is converted to a codec it lists.
	dts := source
	dts.Streams = slices.Clone(source.Streams)
	dts.Streams[1].Codec, dts.Streams[1].Channels = "dts", new(6)
	if got := Decide(download, dts, Options{MaxStreamingBitrate: 2_000_000, Progressive: true, Can: software}); !got.Progressive || got.Audio == nil ||
		got.Audio.Codec != "aac" {
		t.Errorf("DTS audio: %+v, audio %+v", got, got.Audio)
	}
	// One file goes out as MP4 or MPEG-TS, not as another container.
	matroska := *download
	matroska.TranscodingProfiles = slices.Clone(download.TranscodingProfiles)
	matroska.TranscodingProfiles[0].Container = "mkv"
	if got := Decide(&matroska, source, Options{MaxStreamingBitrate: 2_000_000, Progressive: true, Can: software}); got.Streamed() {
		t.Errorf("to Matroska: %+v", got)
	}
}
