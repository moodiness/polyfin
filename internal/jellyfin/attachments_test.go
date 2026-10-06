package jellyfin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/media"
)

// recordedClip is how FFmpeg describes the clip the attachment fixtures
// were recorded with: two subtitle tracks, three fonts, then cover art,
// each tagged with the zero bytes of Matroska's empty codec tags.
var recordedClip = media.Analysis{Format: "matroska,webm", Streams: []media.Stream{
	{Index: 0, Type: "video", Codec: "h264", CodecTag: matroskaTag},
	{Index: 1, Type: "audio", Codec: "aac", CodecTag: matroskaTag},
	{Index: 2, Type: "subtitle", Codec: "subrip", CodecTag: matroskaTag, Language: "fre"},
	{Index: 3, Type: "subtitle", Codec: "ass", CodecTag: matroskaTag, Language: "eng"},
	{Index: 4, Type: "attachment", Codec: "ttf", CodecTag: matroskaTag, FileName: "Test.ttf", MimeType: "application/x-truetype-font"},
	{Index: 5, Type: "attachment", Codec: "otf", CodecTag: matroskaTag, FileName: "Test.otf", MimeType: "application/vnd.ms-opentype"},
	{Index: 6, Type: "attachment", Codec: "ttf", CodecTag: matroskaTag, FileName: "Odd.ttf", MimeType: "application/x-font-ttf"},
	{Index: 7, Type: "video", Codec: "mjpeg", CodecTag: matroskaTag, AttachedPicture: true, FileName: "cover.jpg", MimeType: "image/jpeg"},
}}

// matroskaTag is ffprobe's codec_tag_string of a Matroska track.
const matroskaTag = "[0][0][0][0]"

func TestAttachedFilesAreListedAsJellyfinListsThem(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.2", "ass", "media-source.json"))
	if err != nil {
		t.Fatal(err)
	}
	var recorded struct{ MediaAttachments []MediaAttachment }
	if err := json.Unmarshal(raw, &recorded); err != nil {
		t.Fatal(err)
	}
	want := recorded.MediaAttachments
	// Jellyfin repeats the type the muxer gave a font, which jellyfin-web
	// then skips: Polyfin names it by its format.
	want[2].MimeType = "font/ttf"
	if got := mediaAttachments(recordedClip); !slices.Equal(got, want) {
		t.Errorf("attachments:\n got %+v\nwant %+v", got, want)
	}

	// The same fields as Jellyfin's, nothing more.
	var wantShape, gotShape any
	_ = json.Unmarshal(raw, &wantShape)
	data, _ := json.Marshal(map[string]any{"MediaAttachments": mediaAttachments(recordedClip)})
	_ = json.Unmarshal(data, &gotShape)
	attachments := func(shape any) any { return shape.(map[string]any)["MediaAttachments"] }
	for _, difference := range compareShapes("media-source.MediaAttachments", attachments(wantShape), attachments(gotShape), shapeRules{}) {
		t.Error(difference)
	}
	if got := mediaAttachments(media.Analysis{Streams: recordedClip.Streams[:4]}); got == nil || len(got) != 0 {
		t.Errorf("a version without attachments: %#v, want an empty list", got)
	}
}

func TestFontsAreNamedSoJellyfinWebLoadsThem(t *testing.T) {
	for _, test := range []struct {
		stream media.Stream
		want   string
	}{
		{media.Stream{Codec: "ttf", FileName: "Sign.ttf", MimeType: "application/x-truetype-font"}, "application/x-truetype-font"},
		{media.Stream{Codec: "otf", FileName: "Sign.otf", MimeType: "font/otf"}, "font/otf"},
		{media.Stream{Codec: "ttf", FileName: "Sign.ttf", MimeType: "FONT/TTF"}, "font/ttf"},
		{media.Stream{Codec: "ttf", FileName: "Sign.TTF", MimeType: "application/x-font-ttf"}, "font/ttf"},
		{media.Stream{Codec: "otf", FileName: "Sign.otf", MimeType: "application/x-font-opentype"}, "font/otf"},
		{media.Stream{Codec: "none", FileName: "Songs.ttc", MimeType: "font/collection"}, "font/ttf"},
		{media.Stream{FileName: "Web.woff2", MimeType: "application/octet-stream"}, "font/woff2"},
		{media.Stream{Codec: "otf", FileName: "Untitled", MimeType: ""}, "font/otf"},
		{media.Stream{Codec: "ttf", FileName: "Untitled", MimeType: "application/font-sfnt"}, "font/ttf"},
		{media.Stream{Codec: "mjpeg", FileName: "cover.jpg", MimeType: "image/jpeg"}, "image/jpeg"},
		{media.Stream{FileName: "notes.txt", MimeType: "text/plain"}, "text/plain"},
		{media.Stream{FileName: "data.bin", MimeType: "application/octet-stream"}, "application/octet-stream"},
	} {
		if got := attachmentMimeType(test.stream); got != test.want {
			t.Errorf("%s (%s, %q): %q, want %q", test.stream.FileName, test.stream.Codec, test.stream.MimeType, got, test.want)
		}
	}
}
