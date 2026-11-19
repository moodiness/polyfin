package playback

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// A relayed playlist's addresses each become a link of Polyfin's, far
// longer than a short address: the addresses a playlist names are bounded,
// its lines and its tags' URI attributes together, inline data included.
// One more than the bound refuses the playlist, at that address: no link
// is made past the bound. At the bound it is rewritten whole.
func TestPlaylistsNamingMoreAddressesThanTheBoundAreRefused(t *testing.T) {
	t.Parallel()
	base, _ := url.Parse("https://tv.example/live/channel.m3u8")
	const segment, key, inline = "#EXTINF:2,\ns.ts\n", "#EXT-X-KEY:METHOD=AES-128,URI=k,IV=0x1\n", "#EXT-X-KEY:METHOD=AES-128,URI=\"data:text/plain;base64,AAAA\"\n"
	const segmentWritten, keyWritten = "#EXTINF:2,\nLINK(https://tv.example/live/s.ts)\n", "#EXT-X-KEY:METHOD=AES-128,URI=\"LINK(https://tv.example/live/k)\",IV=0x1\n"
	half := maxPlaylistAddresses / 2
	for _, tc := range []struct {
		name string
		// full names maxPlaylistAddresses addresses, more one more; written
		// is full once rewritten, links how many of its addresses are links.
		full, more, written string
		links               int
	}{
		{"address lines", strings.Repeat(segment, maxPlaylistAddresses), segment, strings.Repeat(segmentWritten, maxPlaylistAddresses), maxPlaylistAddresses},
		{"URI attributes", strings.Repeat(key, maxPlaylistAddresses), key, strings.Repeat(keyWritten, maxPlaylistAddresses), maxPlaylistAddresses},
		{"ten URI attributes a tag", strings.Repeat("#EXT-X-KEY:"+strings.Repeat("URI=k,", 10)+"\n", maxPlaylistAddresses/10), "#EXT-X-MAP:URI=\"init.mp4\"\n",
			strings.Repeat("#EXT-X-KEY:"+strings.Repeat("URI=\"LINK(https://tv.example/live/k)\",", 10)+"\n", maxPlaylistAddresses/10), maxPlaylistAddresses},
		{"lines and attributes", strings.Repeat(key+segment, half), key, strings.Repeat(keyWritten+segmentWritten, half), maxPlaylistAddresses},
		{"inline data among them", strings.Repeat(inline+segment, half), segment, strings.Repeat(inline+segmentWritten, half), half},
	} {
		calls := 0
		link := func(target, _ string) string {
			calls++
			return "LINK(" + target + ")"
		}
		got, err := rewritePlaylist([]byte("#EXTM3U\n"+tc.full), base, link)
		if err != nil {
			t.Errorf("%s, %d addresses: %v", tc.name, maxPlaylistAddresses, err)
			continue
		}
		if string(got) != "#EXTM3U\n"+tc.written {
			t.Errorf("%s, %d addresses: %d bytes written, want the playlist whole, %d bytes", tc.name, maxPlaylistAddresses, len(got), len("#EXTM3U\n"+tc.written))
		}
		if calls != tc.links {
			t.Errorf("%s, %d addresses: %d links made, want %d", tc.name, maxPlaylistAddresses, calls, tc.links)
		}
		// One more, followed by as many again, which are not read.
		calls = 0
		got, err = rewritePlaylist([]byte("#EXTM3U\n"+tc.full+tc.more+tc.full), base, link)
		if !errors.Is(err, ErrUnsafePlaylist) || got != nil {
			t.Errorf("%s, one address more: %d bytes and %v, want the playlist refused", tc.name, len(got), err)
		}
		if calls > tc.links {
			t.Errorf("%s, one address more: %d links made, want at most %d", tc.name, calls, tc.links)
		}
	}
}

// The bytes a playlist is rewritten to are bounded too, for the links long
// addresses make: the playlist is refused at the line, and within a tag at
// the address, that passes the bound, the rest neither read nor linked. A
// playlist written to the bound's last byte is rewritten whole.
func TestPlaylistsWrittenPastTheBoundAreRefusedBeforeTheyAreBuilt(t *testing.T) {
	t.Parallel()
	base, _ := url.Parse("https://tv.example/live/channel.m3u8")
	// Each address line is written as 4096 bytes.
	long := strings.Repeat("L", 4095)
	lines := maxPlaylistWritten / (len(long) + 1)
	calls := 0
	link := func(string, string) string {
		calls++
		return long
	}
	got, err := rewritePlaylist([]byte(strings.Repeat("s\n", lines)), base, link)
	if err != nil {
		t.Fatalf("a playlist written as %d bytes: %v", maxPlaylistWritten, err)
	}
	if len(got) != maxPlaylistWritten || string(got) != strings.Repeat(long+"\n", lines) || calls != lines {
		t.Errorf("a playlist at the bound: %d bytes written with %d links, want %d with %d", len(got), calls, maxPlaylistWritten, lines)
	}
	for name, playlist := range map[string]string{
		"a tag past the bound":      strings.Repeat("s\n", lines) + "#EXT-X-ENDLIST\n",
		"an address past the bound": "#EXTM3U\n" + strings.Repeat("s\n", lines),
		"three times the bound":     strings.Repeat("s\n", 3*lines),
		"three times in one tag":    "#EXT-X-KEY:" + strings.Repeat("URI=k,", 3*lines) + "\n",
	} {
		calls = 0
		got, err := rewritePlaylist([]byte(playlist), base, link)
		if !errors.Is(err, ErrUnsafePlaylist) || got != nil {
			t.Errorf("%s: %d bytes and %v, want the playlist refused", name, len(got), err)
		}
		// The link that takes the links alone past the bound is the last
		// one made.
		if most := maxPlaylistWritten/len(long) + 1; calls > most {
			t.Errorf("%s: %d links made, want at most %d", name, calls, most)
		}
	}
}

// An ordinary playlist, a master and a media one, is rewritten as it was
// before playlists were bounded as written, to the byte: empty lines
// dropped, each line ended by a line feed, each address a link, a
// segment's by its segment name.
func TestOrdinaryPlaylistsAreRewrittenAsBefore(t *testing.T) {
	t.Parallel()
	link := func(target, name string) string { return "LINK(" + target + " as " + name + ")" }
	for _, tc := range []struct{ name, base, playlist, want string }{
		{"master", "https://cdn.example/vod/master.m3u8",
			"#EXTM3U\r\n#EXT-X-VERSION:7\r\n\r\n#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"a\",NAME=\"English, stereo\",URI=\"audio/index.m3u8?t=1\"\r\n" +
				"#EXT-X-STREAM-INF:BANDWIDTH=800000,AUDIO=\"a\"\r\nvideo/index.m3u8\r\n# a comment\r\n  \r\n" +
				"#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=90000,URI=/vod/iframes.m3u8\r\n",
			"#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"a\",NAME=\"English, stereo\",URI=\"LINK(https://cdn.example/vod/audio/index.m3u8?t=1 as index.m3u8)\"\n" +
				"#EXT-X-STREAM-INF:BANDWIDTH=800000,AUDIO=\"a\"\nLINK(https://cdn.example/vod/video/index.m3u8 as index.m3u8)\n# a comment\n  \n" +
				"#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=90000,URI=\"LINK(https://cdn.example/vod/iframes.m3u8 as iframes.m3u8)\"\n"},
		{"media", "https://cdn.example/vod/video/index.m3u8",
			"#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-TARGETDURATION:2\n\n" +
				"#EXT-X-KEY:METHOD=AES-128,URI=\"../keys/k.bin\",IV=0x1\n#EXT-X-MAP:URI=\"init.mp4\",BYTERANGE=\"4@0\"\n" +
				"#EXTINF:2.000,\n#EXT-X-BYTERANGE:6@4\nmedia.mp4\n#EXTINF:2.000,A title\x00https://other.example/media/2.mp4?token=a%2Fb\n" +
				"#EXT-X-KEY:METHOD=NONE\n#EXTINF:1.500,\n//third.example/3.mp4\n#EXT-X-ENDLIST",
			"#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-TARGETDURATION:2\n" +
				"#EXT-X-KEY:METHOD=AES-128,URI=\"LINK(https://cdn.example/vod/keys/k.bin as k.bin)\",IV=0x1\n" +
				"#EXT-X-MAP:URI=\"LINK(https://cdn.example/vod/video/init.mp4 as init.mp4)\",BYTERANGE=\"4@0\"\n" +
				"#EXTINF:2.000,\n#EXT-X-BYTERANGE:6@4\nLINK(https://cdn.example/vod/video/media.mp4 as " + segmentName("media.mp4") + ")\n" +
				"#EXTINF:2.000,A title\nLINK(https://other.example/media/2.mp4?token=a%2Fb as " + segmentName("2.mp4") + ")\n" +
				"#EXT-X-KEY:METHOD=NONE\n#EXTINF:1.500,\nLINK(https://third.example/3.mp4 as " + segmentName("3.mp4") + ")\n#EXT-X-ENDLIST\n"},
	} {
		base, _ := url.Parse(tc.base)
		got, err := rewritePlaylist([]byte(tc.playlist), base, link)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("%s, rewritten:\n%q\nwant:\n%q", tc.name, got, tc.want)
		}
	}
}

// A playlist that starts with a byte order mark is relayed as one without
// it: its first line stays the #EXTM3U tag, not an address made a link.
func TestAPlaylistStartingWithAByteOrderMarkIsRewrittenWithoutIt(t *testing.T) {
	t.Parallel()
	base, _ := url.Parse("https://tv.example/live/channel.m3u8")
	link := func(target, name string) string { return "LINK(" + target + " as " + name + ")" }
	got, err := rewritePlaylist([]byte("\xef\xbb\xbf#EXTM3U\r\n#EXTINF:2,\r\nseg1.ts\r\n"), base, link)
	if err != nil {
		t.Fatal(err)
	}
	want := "#EXTM3U\n#EXTINF:2,\nLINK(https://tv.example/live/seg1.ts as " + segmentName("seg1.ts") + ")\n"
	if string(got) != want {
		t.Errorf("rewritten:\n%q\nwant:\n%q", got, want)
	}
}

// A relayed channel whose playlist passes a bound is answered as one
// naming an address that cannot be relayed: a 502 with the same headers
// and text, nothing of the playlist, and the error returned for the log.
func TestARelayedChannelPastTheBoundsIsRefusedAsAnUnsafeOne(t *testing.T) {
	t.Parallel()
	serve := func(folder, playlist string) (*httptest.ResponseRecorder, error) {
		s := newService(t, &hlsProvider{playlist: playlist}, "ffprobe-not-installed", nil)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/live/index.m3u8", nil)
		err := s.ServeLive(recorder, request, providerVersion(), "http://10.0.0.1/live/"+folder+"/index.m3u8", relayedLink)
		return recorder, err
	}
	unsafe, err := serve("0123-token", "#EXTM3U\n#EXTINF:2,\nftp://10.0.0.1/s.ts\n")
	if !errors.Is(err, ErrUnsafePlaylist) || unsafe.Code != http.StatusBadGateway {
		t.Fatalf("a playlist naming an address that is not HTTP: %d and %v", unsafe.Code, err)
	}
	for _, tc := range []struct{ name, folder, playlist string }{
		{"one address more than the bound", "0123-token", "#EXTM3U\n" + strings.Repeat("#EXTINF:2,\ns.ts\n", maxPlaylistAddresses+1)},
		// 30,000 addresses in a folder of 2,000 bytes: 60 MB of links for
		// a playlist of 180 kB.
		{"more bytes than the bound", strings.Repeat("a", 2000), "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128," + strings.Repeat("URI=k,", 30_000) + "\n"},
	} {
		name := tc.name
		got, err := serve(tc.folder, tc.playlist)
		if !errors.Is(err, ErrUnsafePlaylist) {
			t.Errorf("%s: %v, want the playlist refused", name, err)
		}
		if got.Code != unsafe.Code || got.Body.String() != unsafe.Body.String() {
			t.Errorf("%s: answered %d with %d bytes, want %d %q", name, got.Code, got.Body.Len(), unsafe.Code, unsafe.Body)
		}
		for header := range unsafe.Header() {
			if got.Header().Get(header) != unsafe.Header().Get(header) {
				t.Errorf("%s: %s is %q, want %q", name, header, got.Header().Get(header), unsafe.Header().Get(header))
			}
		}
		if len(got.Header()) != len(unsafe.Header()) {
			t.Errorf("%s: headers %v, want %v", name, got.Header(), unsafe.Header())
		}
	}
	// At the bound it is relayed whole.
	got, err := serve("0123-token", "#EXTM3U\n"+strings.Repeat("#EXTINF:2,\ns.ts\n", maxPlaylistAddresses))
	if err != nil || got.Code != http.StatusOK || strings.Count(got.Body.String(), "/relayed/") != maxPlaylistAddresses {
		t.Errorf("a playlist of %d addresses: %d, %d links and %v", maxPlaylistAddresses, got.Code, strings.Count(got.Body.String(), "/relayed/"), err)
	}
}
