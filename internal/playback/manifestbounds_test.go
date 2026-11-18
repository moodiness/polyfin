package playback

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// listed is a manifest of a segment list holding inner, after what head
// gives its MPD.
func listed(head, inner string) string {
	return `<?xml version="1.0"?><MPD>` + head + `<Period><AdaptationSet><Representation><SegmentList>` + inner +
		`</SegmentList></Representation></AdaptationSet></Period></MPD>`
}

// counting is folderOf, its calls counted.
func counting(calls *int) func(string) string {
	return func(origin string) string {
		*calls++
		return folderOf(origin)
	}
}

// Each address a manifest's rewriting writes holds the folder of its
// origin, far longer than a short address: the addresses rewritten are
// bounded, the MPD's BaseURL and its segments' together. One more than
// the bound refuses the manifest, at that address: no folder is named
// past the bound. At the bound it is rewritten whole.
func TestManifestsNamingMoreAddressesThanTheBoundAreRefused(t *testing.T) {
	t.Parallel()
	base, _ := url.Parse("https://tv.example/live/manifest.mpd")
	const segment, segmentWritten = `<SegmentURL media="/s.m4s"/>`, `<SegmentURL media="http://lo/live/k/~https://tv.example/s.m4s"/>`
	const template, templateWritten = `<SegmentTemplate media="//cdn.example/$Number$.m4s" initialization="https://cdn.example/init.mp4"/>`,
		`<SegmentTemplate media="http://lo/live/k/~https://cdn.example/$Number$.m4s" initialization="http://lo/live/k/~https://cdn.example/init.mp4"/>`
	for _, tc := range []struct {
		name string
		// full holds maxManifestAddresses addresses with the MPD's
		// BaseURL, more one more; written is full once rewritten.
		head, full, more, headWritten, written string
	}{
		{"segments after the folder the MPD is given", "", strings.Repeat(segment, maxManifestAddresses-1), segment,
			"<BaseURL>http://lo/live/k/~https://tv.example/live/</BaseURL>", strings.Repeat(segmentWritten, maxManifestAddresses-1)},
		{"segments after the MPD's BaseURL", "<BaseURL>https://cdn.example/a/</BaseURL>", strings.Repeat(segment, maxManifestAddresses-1), segment,
			"<BaseURL>http://lo/live/k/~https://cdn.example/a/</BaseURL>",
			strings.Repeat(`<SegmentURL media="http://lo/live/k/~https://cdn.example/s.m4s"/>`, maxManifestAddresses-1)},
		{"two addresses an element", "", strings.Repeat(template, maxManifestAddresses/2-1) + segment, "<BaseURL>b/</BaseURL>",
			"<BaseURL>http://lo/live/k/~https://tv.example/live/</BaseURL>", strings.Repeat(templateWritten, maxManifestAddresses/2-1) + segmentWritten},
	} {
		calls := 0
		got, err := rewriteManifest([]byte(listed(tc.head, tc.full)), base, counting(&calls))
		if err != nil {
			t.Errorf("%s, %d addresses: %v", tc.name, maxManifestAddresses, err)
			continue
		}
		if want := listed(tc.headWritten, tc.written); string(got) != want {
			t.Errorf("%s, %d addresses: %d bytes written, want the manifest whole, %d bytes", tc.name, maxManifestAddresses, len(got), len(want))
		}
		if calls != maxManifestAddresses {
			t.Errorf("%s: %d folders named, want %d", tc.name, calls, maxManifestAddresses)
		}
		// One more, followed by others, which are not read.
		calls = 0
		got, err = rewriteManifest([]byte(listed(tc.head, tc.full+tc.more+strings.Repeat(tc.more, 100))), base, counting(&calls))
		if !errors.Is(err, ErrUnsafePlaylist) || got != nil {
			t.Errorf("%s, one address more: %d bytes and %v, want the manifest refused", tc.name, len(got), err)
		}
		if calls > maxManifestAddresses {
			t.Errorf("%s, one address more: %d folders named, want at most %d", tc.name, calls, maxManifestAddresses)
		}
	}
}

// The folder of an address holds its host, and an address without one its
// base's: a host is bounded to a DNS name's length and a port, wherever
// the manifest names it, and no folder is named for a longer one. A host
// of the bound's length is rewritten.
func TestManifestsNamingAHostPastTheBoundAreRefused(t *testing.T) {
	t.Parallel()
	base, _ := url.Parse("https://tv.example/live/manifest.mpd")
	longest := strings.Repeat("a", maxManifestHost-len(":65535")) + ":65535"
	longer := "a" + longest
	if len(longest) != maxManifestHost {
		t.Fatalf("a host of %d bytes, want %d", len(longest), maxManifestHost)
	}
	for name, inner := range map[string]string{
		"a BaseURL":                 "<BaseURL>https://%s/</BaseURL>" + strings.Repeat(`<SegmentURL media="/s.m4s"/>`, 1000),
		"a BaseURL without scheme":  "<BaseURL>//%s/a/</BaseURL>" + `<SegmentURL media="s.m4s"/>`,
		"a segment's address":       `<SegmentURL media="https://%s/s.m4s"/>`,
		"an address without scheme": `<SegmentURL media="//%s/s.m4s"/>`,
		"an initialization section": `<Initialization sourceURL="http://%s/init.mp4"/>`,
	} {
		calls := 0
		got, err := rewriteManifest([]byte(listed("", strings.ReplaceAll(inner, "%s", longest))), base, counting(&calls))
		if err != nil || !strings.Contains(string(got), "://"+longest+"/") {
			t.Errorf("%s, a host of %d bytes: %v, want it rewritten", name, len(longest), err)
		}
		calls = 0
		got, err = rewriteManifest([]byte(listed("", strings.ReplaceAll(inner, "%s", longer))), base, counting(&calls))
		if !errors.Is(err, ErrUnsafePlaylist) || got != nil {
			t.Errorf("%s, a host of %d bytes: %d bytes and %v, want the manifest refused", name, len(longer), len(got), err)
		}
		// The MPD's own folder alone was named, before the host was read.
		if calls > 1 {
			t.Errorf("%s, a host of %d bytes: %d folders named, want at most 1", name, len(longer), calls)
		}
	}
	// The manifest's own address is a base as any other.
	own, _ := url.Parse("https://" + longer + "/live/manifest.mpd")
	if got, err := rewriteManifest([]byte(listed("", `<SegmentURL media="s.m4s"/>`)), own, folderOf); !errors.Is(err, ErrUnsafePlaylist) {
		t.Errorf("a manifest at a host of %d bytes: %d bytes and %v, want it refused", len(longer), len(got), err)
	}
}

// The bytes a manifest is rewritten to are bounded too: a BaseURL is
// written resolved, the path of its parent's in front, however short it
// is. The manifest is refused at the address that takes its addresses
// past the bound, the rest neither read nor written, and when what is
// written passes the bound by what is no address. A manifest written to
// the bound's last byte is rewritten whole.
func TestManifestsWrittenPastTheBoundAreRefusedBeforeTheyAreBuilt(t *testing.T) {
	t.Parallel()
	base, _ := url.Parse("https://tv.example/live/manifest.mpd")
	// Each BaseURL of one letter is written after a path of 4 kB.
	path := strings.Repeat("p", 4000)
	head := "<BaseURL>https://cdn.example/" + path + "/</BaseURL>"
	const relative = "<BaseURL>x</BaseURL>"
	each := len(folderOf("https://cdn.example") + path + "/x")
	count := maxManifestWritten / each
	calls := 0
	for name, repeated := range map[string]int{"past the bound": count + 1, "twice the bound": 2 * count} {
		calls = 0
		got, err := rewriteManifest([]byte(listed(head, strings.Repeat(relative, repeated))), base, counting(&calls))
		if !errors.Is(err, ErrUnsafePlaylist) || got != nil {
			t.Errorf("%s: %d bytes and %v, want the manifest refused", name, len(got), err)
		}
		// The address that takes the addresses past the bound is the last
		// one written, after the MPD's.
		if calls > count+2 {
			t.Errorf("%s: %d folders named, want at most %d", name, calls, count+2)
		}
	}
	// What is no address counts: a comment fills the manifest to the
	// bound's last byte, then to one more.
	short, err := rewriteManifest([]byte(listed(head, strings.Repeat(relative, count-100))), base, folderOf)
	if err != nil {
		t.Fatal(err)
	}
	fill := maxManifestWritten - len(short) - len("<!---->")
	for more, refused := range map[int]bool{0: false, 1: true} {
		comment := "<!--" + strings.Repeat("c", fill+more) + "-->"
		got, err := rewriteManifest([]byte(listed(head, strings.Repeat(relative, count-100))+comment), base, folderOf)
		switch {
		case refused && (!errors.Is(err, ErrUnsafePlaylist) || got != nil):
			t.Errorf("a manifest written as %d bytes: %d bytes and %v, want it refused", maxManifestWritten+more, len(got), err)
		case !refused && (err != nil || len(got) != maxManifestWritten || !bytes.HasPrefix(got, short) || !bytes.HasSuffix(got, []byte(comment))):
			t.Errorf("a manifest written as %d bytes: %d bytes and %v, want it whole", maxManifestWritten, len(got), err)
		}
	}
}

// manifestOrigin answers every request with a DASH manifest, as the
// source of a live stream does.
type manifestOrigin struct{ manifest string }

func (o manifestOrigin) Open(context.Context, string, string, http.Header, bool) (*http.Response, error) {
	return sized(http.StatusOK, "application/dash+xml", []byte(o.manifest)), nil
}

// FFmpeg asking the loopback interface for a live stream's manifest that
// passes a bound is answered as for one naming an address that cannot be
// relayed: the same 502 and text, nothing of the manifest. Within the
// bounds the manifest is relayed, rewritten.
func TestTheLoopbackInterfaceRefusesAManifestPastTheBounds(t *testing.T) {
	t.Parallel()
	ask := func(manifest string) (int, string, string) {
		s := newService(t, manifestOrigin{manifest: manifest}, "ffprobe-not-installed", nil)
		live := s.loopback.registerLiveFor(library.Version{ID: accounts.ID{0xDA}, URL: "https://tv.example/live/manifest.mpd"}, accounts.ID{9})
		defer live.release()
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, live.target, nil)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response.StatusCode, response.Header.Get("Content-Type"), string(body)
	}
	status, kind, unsafe := ask(listed("", `<SegmentURL media="ftp://10.0.0.1/s.m4s"/>`))
	if status != http.StatusBadGateway {
		t.Fatalf("a manifest naming an address that is not HTTP: %d %s", status, unsafe)
	}
	long := strings.Repeat("a", 8192)
	for name, manifest := range map[string]string{
		"a long host":                     listed("<BaseURL>https://"+long+"/</BaseURL>", strings.Repeat(`<SegmentURL media="/"/>`, 356)),
		"a long path":                     listed("<BaseURL>https://cdn.example/"+long+"/</BaseURL>", strings.Repeat("<BaseURL>x</BaseURL>", 5000)),
		"one address more than the bound": listed("", strings.Repeat(`<SegmentURL media="/"/>`, maxManifestAddresses)),
	} {
		if got, gotKind, body := ask(manifest); got != status || gotKind != kind || body != unsafe {
			t.Errorf("%s: answered %d %s with %d bytes, want %d %s %q", name, got, gotKind, len(body), status, kind, unsafe)
		}
	}
	got, gotKind, body := ask(listed("<BaseURL>https://cdn.example/a/</BaseURL>", strings.Repeat(`<SegmentURL media="/"/>`, 356)))
	if got != http.StatusOK || gotKind != "application/dash+xml" || strings.Count(body, "/~") != 357 {
		t.Errorf("a manifest within the bounds: %d %s with %d addresses rewritten, want 357", got, gotKind, strings.Count(body, "/~"))
	}
}
