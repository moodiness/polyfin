package media

import "testing"

// ffprobe tells, of an HLS playlist, the playlist's own bytes as its size
// and the bitrate they make over the stream's duration (here 1135 bytes
// over 5751 seconds, 1 b/s): the analysis keeps no size, and the bitrate
// of the variant Polyfin plays, its first video's, as FFmpeg tags it; of
// a media playlist, the bitrates ffprobe tells of its tracks, none here.
func TestParseTakesAnHLSBitrateFromItsVariant(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		streams string
		want    int64
	}{
		"a master playlist of two variants": {`
			{"index": 0, "codec_type": "audio", "codec_name": "aac", "channels": 2, "tags": {"variant_bitrate": "836280"}},
			{"index": 1, "codec_type": "video", "codec_name": "h264", "width": 640, "height": 480, "tags": {"variant_bitrate": "836280"}},
			{"index": 2, "codec_type": "audio", "codec_name": "aac", "channels": 2, "tags": {"variant_bitrate": "460560"}},
			{"index": 3, "codec_type": "video", "codec_name": "h264", "width": 512, "height": 384, "tags": {"variant_bitrate": "460560"}}`, 836280},
		"variants listed lowest first": {`
			{"index": 0, "codec_type": "video", "codec_name": "h264", "tags": {"variant_bitrate": "460560"}},
			{"index": 1, "codec_type": "video", "codec_name": "h264", "tags": {"variant_bitrate": "836280"}}`, 460560},
		"a media playlist whose tracks tell their bitrates": {`
			{"index": 0, "codec_type": "video", "codec_name": "h264", "bit_rate": "700000", "tags": {"variant_bitrate": "0"}},
			{"index": 1, "codec_type": "audio", "codec_name": "aac", "bit_rate": "128000", "tags": {"variant_bitrate": "0"}}`, 828000},
		"a media playlist whose tracks tell none": {`
			{"index": 0, "codec_type": "video", "codec_name": "h264", "tags": {"variant_bitrate": "0"}},
			{"index": 1, "codec_type": "audio", "codec_name": "aac", "tags": {"variant_bitrate": "0"}}`, 0},
	} {
		analysis, err := Parse([]byte(`{"format": {"format_name": "hls", "duration": "5751.284610", "size": "1135", "bit_rate": "1"},
			"streams": [` + c.streams + `]}`))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if analysis.Size != 0 || analysis.Bitrate != c.want {
			t.Errorf("%s: a size of %d and a bitrate of %d, want none and %d", name, analysis.Size, analysis.Bitrate, c.want)
		}
	}
	// Any other container keeps what ffprobe tells.
	analysis, err := Parse([]byte(`{"format": {"format_name": "mpegts", "size": "1135", "bit_rate": "1"},
		"streams": [{"index": 0, "codec_type": "video", "codec_name": "h264", "tags": {"variant_bitrate": "836280"}}]}`))
	if err != nil || analysis.Size != 1135 || analysis.Bitrate != 1 {
		t.Errorf("an MPEG-TS file: %+v, %v", analysis, err)
	}
}

// PlaylistBitrate reads what FFmpeg does not: a variant's AVERAGE-BANDWIDTH,
// found by its BANDWIDTH, and the bytes a media playlist's segments give
// over their duration.
func TestPlaylistBitrateReadsAveragesAndByteRanges(t *testing.T) {
	t.Parallel()
	master := "#EXTM3U\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=836280,AVERAGE-BANDWIDTH=700000,CODECS=\"mp4a.40.2,avc1.64001f\",RESOLUTION=640x480\nhigh.m3u8\n" +
		"#EXT-X-STREAM-INF:CODECS=\"mp4a.40.2,avc1.42001e\",BANDWIDTH=460560\r\nlow.m3u8\r\n"
	ranged := "#EXTM3U\n#EXT-X-TARGETDURATION:4\n#EXT-X-MAP:URI=\"init.mp4\",BYTERANGE=\"700@0\"\n" +
		"#EXTINF:4.000,\n#EXT-X-BYTERANGE:500000@700\nmedia.mp4\n#EXTINF:4.000,\n#EXT-X-BYTERANGE:300000\nmedia.mp4\n#EXT-X-ENDLIST\n"
	unranged := "#EXTM3U\n#EXTINF:4.000,\n#EXT-X-BYTERANGE:500000@0\nmedia.mp4\n#EXTINF:4.000,\ns1.ts\n#EXT-X-ENDLIST\n"
	for name, c := range map[string]struct {
		playlist  string
		bandwidth int64
		want      int64
	}{
		"a variant with an average":     {master, 836280, 700000},
		"a variant without an average":  {master, 460560, 0},
		"no such variant":               {master, 1, 0},
		"a master playlist, no variant": {master, 0, 0},
		"segments giving their bytes":   {ranged, 0, 800000},
		"a segment without its bytes":   {unranged, 0, 0},
		"segments of a variant asked":   {ranged, 836280, 0},
	} {
		if got := PlaylistBitrate([]byte(c.playlist), c.bandwidth); got != c.want {
			t.Errorf("%s: %d, want %d", name, got, c.want)
		}
	}
}
