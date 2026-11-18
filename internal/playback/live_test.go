package playback

import (
	"errors"
	"math/rand/v2"
	"net/url"
	"strings"
	"testing"
)

// A user's own addon may only reach public addresses. Its channel's
// playlist is read by FFmpeg through Polyfin, which fetches any address
// left in it itself: every address must become a link, or the playlist is
// refused, in each form FFmpeg's HLS demuxer reads.
func TestPlaylistsNamingAddressesPolyfinCannotRelayAreRefused(t *testing.T) {
	t.Parallel()
	base, _ := url.Parse("https://tv.example/live/channel.m3u8")
	link := func(target, _ string) string { return "LINK(" + target + ")" }
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

// ffmpegAttribute is an attribute of a tag as FFmpeg reads it.
type ffmpegAttribute struct{ key, value string }

// ffmpegAttributes reads the attribute list of a tag as FFmpeg does: a
// port of ff_parse_key_value (libavformat/utils.c, FFmpeg 9.0), whose keys
// run to their equal sign, included.
func ffmpegAttributes(list string) []ffmpegAttribute {
	space := func(c byte) bool { return strings.IndexByte(" \f\n\r\t\v", c) >= 0 }
	var read []ffmpegAttribute
	i := 0
	for {
		for i < len(list) && (space(list[i]) || list[i] == ',') {
			i++
		}
		if i == len(list) {
			return read
		}
		equal := strings.IndexByte(list[i:], '=')
		if equal < 0 {
			return read
		}
		key := list[i : i+equal+1]
		i += equal + 1
		var value []byte
		if i < len(list) && list[i] == '"' {
			i++
			for i < len(list) && list[i] != '"' {
				if list[i] == '\\' {
					if i+1 == len(list) {
						break
					}
					value = append(value, list[i+1])
					i += 2
				} else {
					value = append(value, list[i])
					i++
				}
			}
			if i < len(list) && list[i] == '"' {
				i++
			}
		} else {
			for ; i < len(list) && !space(list[i]) && list[i] != ','; i++ {
				value = append(value, list[i])
			}
		}
		read = append(read, ffmpegAttribute{key, string(value)})
	}
}

// The attribute list of a tag is read as FFmpeg reads it: an unquoted
// value ends at white space as at a comma, and a backslash within quotes
// takes the next character as it is, a quote included. Every address
// FFmpeg then reads in the list is a link.
func TestAttributeListsAreReadAsFFmpegReadsThem(t *testing.T) {
	t.Parallel()
	link := func(ref string) (string, error) { return "LINK(" + ref + ")", nil }
	for _, tc := range []struct{ name, line, want string }{
		{"a quoted address", `#EXT-X-KEY:METHOD=AES-128,URI="k.key",IV=0x1`, `#EXT-X-KEY:METHOD=AES-128,URI="LINK(k.key)",IV=0x1`},
		{"an unquoted address", `#EXT-X-KEY:METHOD=AES-128,URI=k.key,IV=0x1`, `#EXT-X-KEY:METHOD=AES-128,URI="LINK(k.key)",IV=0x1`},
		{"an unquoted value ended by a space", `#EXT-X-KEY:METHOD=AES-128,KEYFORMAT=identity URI=http://10.0.0.5/key`,
			`#EXT-X-KEY:METHOD=AES-128,KEYFORMAT=identity URI="LINK(http://10.0.0.5/key)"`},
		{"an unquoted value ended by a tab", "#EXT-X-MEDIA:TYPE=AUDIO\tURI=http://10.0.0.5/a.m3u8,GROUP-ID=\"a\"",
			"#EXT-X-MEDIA:TYPE=AUDIO\tURI=\"LINK(http://10.0.0.5/a.m3u8)\",GROUP-ID=\"a\""},
		{"an unquoted value ended by a form feed", "#EXT-X-MAP:BYTERANGE=720@0\fURI=http://10.0.0.5/init.mp4",
			"#EXT-X-MAP:BYTERANGE=720@0\fURI=\"LINK(http://10.0.0.5/init.mp4)\""},
		{"an unquoted address ended by a space", `#EXT-X-MAP:URI=init.mp4 BYTERANGE=720@0`, `#EXT-X-MAP:URI="LINK(init.mp4)" BYTERANGE=720@0`},
		{"a quoted value holding an escaped quote", `#EXT-X-KEY:METHOD=AES-128,X="a\",Y=",URI=http://10.0.0.5/key,Z="`,
			`#EXT-X-KEY:METHOD=AES-128,X="a\",Y=",URI="LINK(http://10.0.0.5/key)",Z="`},
		{"a quoted value ending with an escaped backslash", `#EXT-X-MEDIA:NAME="a\\",URI="a.m3u8"`, `#EXT-X-MEDIA:NAME="a\\",URI="LINK(a.m3u8)"`},
		{"an address holding an escaped character", `#EXT-X-MAP:URI="init\.mp4"`, `#EXT-X-MAP:URI="LINK(init.mp4)"`},
		{"an address whose quote never closes", `#EXT-X-MAP:URI="init.mp4`, `#EXT-X-MAP:URI="LINK(init.mp4)"`},
		{"an address ended by a lone backslash", `#EXT-X-MAP:URI="init.mp4\`, `#EXT-X-MAP:URI="LINK(init.mp4)"\`},
		{"an empty unquoted address", `#EXT-X-KEY:METHOD=AES-128,URI=,IV=0x1`, `#EXT-X-KEY:METHOD=AES-128,URI="LINK()",IV=0x1`},
		{"an empty quoted address", `#EXT-X-KEY:METHOD=AES-128,URI="",IV=0x1`, `#EXT-X-KEY:METHOD=AES-128,URI="LINK()",IV=0x1`},
		{"an address that ends the line", `#EXT-X-KEY:METHOD=AES-128,URI=`, `#EXT-X-KEY:METHOD=AES-128,URI="LINK()"`},
		{"commas and white space around", "#EXT-X-KEY:, ,METHOD=AES-128,,\t URI=k.key , ,", "#EXT-X-KEY:, ,METHOD=AES-128,,\t URI=\"LINK(k.key)\" , ,"},
		{"two addresses", `#EXT-X-KEY:URI=a.key,URI="b.key"`, `#EXT-X-KEY:URI="LINK(a.key)",URI="LINK(b.key)"`},
		{"a name FFmpeg does not take for an address", `#EXT-X-KEY:uri=a.key,URI =b.key`, `#EXT-X-KEY:uri="LINK(a.key)",URI ="LINK(b.key)"`},
		{"a name that runs over a comma", `#EXT-X-KEY:METHOD,URI=a.key`, `#EXT-X-KEY:METHOD,URI=a.key`},
		{"a tag of no attribute", `#EXTINF:2.000,Title, with comma`, `#EXTINF:2.000,Title, with comma`},
		{"a tag of no list", `#EXT-X-ENDLIST`, `#EXT-X-ENDLIST`},
	} {
		got, err := rewriteAttributes(tc.line, link)
		if err != nil || got != tc.want {
			t.Errorf("%s: %q is rewritten %q, %v, want %q", tc.name, tc.line, got, err, tc.want)
		}
	}

	// Whatever the list, FFmpeg reads no address in what is written of it
	// but a link.
	random := rand.New(rand.NewPCG(1, 2))
	pieces := []string{"URI=", "URI=", "X=", "=", `"`, `"`, `\`, ",", " ", "\t", "a", "http://10.0.0.5/k", "uri", "URI"}
	for range 20000 {
		var list strings.Builder
		for range 1 + random.IntN(10) {
			list.WriteString(pieces[random.IntN(len(pieces))])
		}
		written, err := rewriteAttributes("#EXT-X-KEY:"+list.String(), link)
		if err != nil {
			if !errors.Is(err, ErrUnsafePlaylist) {
				t.Fatalf("%q: %v", list.String(), err)
			}
			continue
		}
		for _, attribute := range ffmpegAttributes(strings.TrimPrefix(written, "#EXT-X-KEY:")) {
			if attribute.key == "URI=" && !strings.HasPrefix(attribute.value, "LINK(") {
				t.Fatalf("%q is written %q, where FFmpeg reads the address %q", list.String(), written, attribute.value)
			}
		}
	}
}

// A line FFmpeg would read otherwise than it is written is refused: an
// address that holds a quote or a backslash, which no quoted value holds
// for every reader, and a line FFmpeg would cut, at 4095 bytes, within the
// host of a link, which would then name another.
func TestPlaylistLinesFFmpegWouldReadOtherwiseAreRefused(t *testing.T) {
	t.Parallel()
	base, _ := url.Parse("https://tv.example/live/channel.m3u8")
	link := func(target, _ string) string { return "http://127.0.0.1:49152/live/key/" + target }
	for name, line := range map[string]string{
		"inline data holding a quote":     `#EXT-X-KEY:METHOD=AES-128,URI="data:text/plain,a\"b"`,
		"inline data holding a backslash": `#EXT-X-KEY:METHOD=AES-128,URI="data:text/plain,a\\b"`,
	} {
		if _, err := rewritePlaylist([]byte("#EXTM3U\n"+line+"\n"), base, link); !errors.Is(err, ErrUnsafePlaylist) {
			t.Errorf("%s: got %v, want the playlist refused", name, err)
		}
	}
	// The link starts at the byte given: its host, 127.0.0.1:49152, is cut
	// when the line's 4095th byte falls within "http://127.0.0.1:49152".
	for _, tc := range []struct {
		start   int
		refused bool
	}{{4000, false}, {4072, false}, {4073, false}, {4074, true}, {4090, true}, {4094, true}, {4095, false}, {5000, false}} {
		prefix := "#EXT-X-KEY:METHOD=AES-128,X="
		line := prefix + strings.Repeat("x", tc.start-len(prefix)-len(`,URI="`)) + ",URI=key.bin"
		written, err := rewritePlaylist([]byte("#EXTM3U\n"+line+"\n"), base, link)
		if refused := errors.Is(err, ErrUnsafePlaylist); refused != tc.refused || err != nil && !refused {
			t.Errorf("a link from byte %d: got %v, want refused: %t", tc.start, err, tc.refused)
		}
		if err == nil {
			if at := strings.Index(string(written), "http://127.0.0.1:49152") - len("#EXTM3U\n"); at != tc.start {
				t.Fatalf("the link starts at byte %d of its line, want %d", at, tc.start)
			}
		}
	}
}
