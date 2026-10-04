package playback

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

// A user's own addon may only reach public addresses. Its channel's
// playlist is read by FFmpeg through Polyfin, which fetches any address
// left in it itself: every address must become a link, or the playlist is
// refused, in each form FFmpeg's HLS demuxer reads.
func TestPlaylistsNamingAddressesPolyfinCannotRelayAreRefused(t *testing.T) {
	base, _ := url.Parse("https://tv.example/live/channel.m3u8")
	link := func(target string) string { return "LINK(" + target + ")" }
	for name, playlist := range map[string]string{
		"segment after a carriage return":  "#EXTM3U\n#EXTINF:2,\rhttp://10.0.0.1:80x/x.ts\n",
		"segment after a NUL":              "#EXTM3U\n#EXTINF:2,\x00file:///etc/passwd\n",
		"unparsable segment":               "#EXTM3U\n#EXTINF:2,\nhttp://10.0.0.1/a%zz\n",
		"unquoted key address":             "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=gopher://169.254.169.254/latest\n",
		"unparsable quoted map address":    "#EXTM3U\n#EXT-X-MAP:URI=\"http://10.0.0.1:x/init.mp4\"\n",
		"other scheme in a rendition":      "#EXTM3U\n#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"a\",URI=\"ftp://10.0.0.1/a.m3u8\"\n",
		"scheme-relative without a host":   "#EXTM3U\n#EXTINF:2,\nhttp:///x.ts\n",
		"crypto protocol wrapping a fetch": "#EXTM3U\n#EXTINF:2,\ncrypto+http://10.0.0.1/x.ts\n",
	} {
		if _, err := rewritePlaylist([]byte(playlist), base, link); !errors.Is(err, ErrUnsafePlaylist) {
			t.Errorf("%s: got %v, want the playlist refused", name, err)
		}
	}

	playlist := "#EXTM3U\r\n#EXT-X-KEY:METHOD=AES-128,URI=key.bin,IV=0x1\n#EXT-X-MAP:URI=\"init.mp4\",BYTERANGE=\"720@0\"\n" +
		"#EXT-X-SESSION-KEY:METHOD=SAMPLE-AES,URI=\"data:text/plain;base64,AAAA\"\n#EXTINF:2.000,Title, with comma\nseg1.ts\r#EXTINF:2,\n https://cdn.example/seg2.ts \n"
	got, err := rewritePlaylist([]byte(playlist), base, link)
	if err != nil {
		t.Fatal(err)
	}
	want := "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"LINK(https://tv.example/live/key.bin)\",IV=0x1\n" +
		"#EXT-X-MAP:URI=\"LINK(https://tv.example/live/init.mp4)\",BYTERANGE=\"720@0\"\n" +
		"#EXT-X-SESSION-KEY:METHOD=SAMPLE-AES,URI=\"data:text/plain;base64,AAAA\"\n#EXTINF:2.000,Title, with comma\n" +
		"LINK(https://tv.example/live/seg1.ts)\n#EXTINF:2,\nLINK(https://cdn.example/seg2.ts)\n"
	if string(got) != want {
		t.Errorf("rewritten playlist:\n%s\nwant:\n%s", got, want)
	}
	for line := range strings.Lines(string(got)) {
		if strings.Contains(line, "://") && !strings.Contains(line, "LINK(") {
			t.Errorf("an address left as it is: %q", line)
		}
	}
}
