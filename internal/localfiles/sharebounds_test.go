package localfiles

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// readBodies counts the bytes a share reads of the answers to its
// requests.
type readBodies struct {
	base http.RoundTripper
	read atomic.Int64
}

func (c *readBodies) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := c.base.RoundTrip(request)
	if err == nil {
		response.Body = &readBody{ReadCloser: response.Body, read: &c.read}
	}
	return response, err
}

type readBody struct {
	io.ReadCloser
	read *atomic.Int64
}

func (b *readBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.read.Add(int64(n))
	return n, err
}

// countReads counts the bytes a share's client reads of its answers.
func countReads(client *http.Client) *readBodies {
	counted := &readBodies{base: client.Transport}
	client.Transport = counted
	return counted
}

// serveAnswer answers the requests of method at path, any path when
// empty, with status and body.
func serveAnswer(t *testing.T, method, path string, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method || path != "" && r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

// boundMet tells whether err is the bound of a folder's listing, met.
func boundMet(err error) bool {
	var bound *shareBound
	return errors.As(err, &bound) && bound.failure == errBoundListing
}

// davEntry is an entry of a PROPFIND's answer.
func davEntry(href string, folder bool) string {
	kind := ""
	if folder {
		kind = "<D:collection/>"
	}
	return "<D:response><D:href>" + href + "</D:href><D:propstat><D:prop><D:resourcetype>" + kind +
		"</D:resourcetype><D:getcontentlength>1</D:getcontentlength></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>"
}

// The answer listing a WebDAV folder is read within the bounds of a
// listing, and no further: its bytes, those of one entry, its entries,
// the bytes of a name and of the names kept. A folder at its bounds
// lists whole. The bounds are those the folder's page and the
// documentation tell.
func TestAWebDAVListingIsReadWithinItsBounds(t *testing.T) {
	t.Parallel()
	if want := (listBounds{answer: 64 << 20, entry: 256 << 10, entries: 500_000, name: 1024, names: 64 << 20}); folderBounds != want {
		t.Errorf("a listing is bounded by %+v, want %+v", folderBounds, want)
	}
	const (
		head = `<?xml version="1.0" encoding="utf-8"?><D:multistatus xmlns:D="DAV:">`
		tail = `</D:multistatus>`
		name = "Nosferatu (1922).mkv"
	)
	self, movie := davEntry("/dav/", true), davEntry("/dav/"+name, false)
	ten := head + self + strings.Repeat(movie, 10) + tail
	many := head + self + strings.Repeat(movie, 20000) + tail
	small := listBounds{answer: 1 << 20, entry: 4096, entries: 100, name: 64, names: 1 << 20}
	exact := listBounds{answer: int64(len(ten)), entry: int64(len(movie)), entries: 11, name: len(name), names: 10 * len(name)}
	with := func(change func(b *listBounds)) listBounds {
		b := exact
		change(&b)
		return b
	}
	for _, c := range []struct {
		name   string
		bounds listBounds
		body   string
		// entries are the entries listed, none when the listing fails
		// at a bound; most the bytes read of the answer at most, when
		// set.
		entries int
		most    int
	}{
		{name: "an entry of more bytes than one holds", bounds: folderBounds,
			body: head + self + davEntry("/dav/"+strings.Repeat("a", 8<<20), false) + tail,
			most: len(head+self) + int(folderBounds.entry) + 4096},
		{name: "more entries than a folder holds", bounds: small, body: many, most: len(head+self) + (small.entries+1)*len(movie) + 4096},
		{name: "an answer of more bytes than one holds", bounds: listBounds{answer: 64 << 10, entry: 4096, entries: 1 << 30, name: 64, names: 1 << 30},
			body: many, most: 64<<10 + 4096},
		{name: "a name longer than a file's", bounds: folderBounds,
			body: head + self + davEntry("/dav/"+strings.Repeat("a", folderBounds.name+1), false) + tail},
		{name: "a name as long as a file's", bounds: folderBounds,
			body: head + self + davEntry("/dav/"+strings.Repeat("a", folderBounds.name), false) + tail, entries: 1},
		{name: "a folder at its bounds", bounds: exact, body: ten, entries: 10},
		{name: "one entry more", bounds: with(func(b *listBounds) { b.entries-- }), body: ten},
		{name: "one byte more in an entry", bounds: with(func(b *listBounds) { b.entry-- }), body: ten},
		{name: "one byte more in the answer", bounds: with(func(b *listBounds) { b.answer-- }), body: ten},
		{name: "one byte more in a name", bounds: with(func(b *listBounds) { b.name-- }), body: ten},
		{name: "one byte more in the names", bounds: with(func(b *listBounds) { b.names-- }), body: ten},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			server := serveAnswer(t, "PROPFIND", "", http.StatusMultiStatus, c.body)
			address, err := url.Parse(server.URL + "/dav")
			if err != nil {
				t.Fatal(err)
			}
			share := newWebDAV(address, "", "")
			t.Cleanup(share.close)
			if share.bounds != folderBounds {
				t.Fatalf("a WebDAV folder is listed within %+v, want %+v", share.bounds, folderBounds)
			}
			share.bounds = c.bounds
			counted := countReads(share.client)
			entries, err := share.list(t.Context(), "")
			read := int(counted.read.Load())
			switch {
			case c.entries > 0 && (err != nil || len(entries) != c.entries):
				t.Fatalf("%d entries listed, %v; want %d", len(entries), err, c.entries)
			case c.entries == 0 && !boundMet(err):
				t.Fatalf("%d entries listed of an answer of %d bytes, %d read: %v; want the bound of a listing met", len(entries), len(c.body), read, err)
			case c.most > 0 && read > c.most:
				t.Fatalf("%d bytes read of an answer of %d, want %d at most", read, len(c.body), c.most)
			}
		})
	}
}

// The answer listing a folder of an rclone remote is read item by item
// within the bounds of a listing, and no further: an answer of more items
// than a folder holds fails whatever the items are, and makes room for
// none. A call answered whole, as the one listing the remotes, is read
// within a bound of its own.
func TestAnRcloneListingIsReadWithinItsBounds(t *testing.T) {
	t.Parallel()
	// list is the answer listing n times item.
	list := func(item string, n int) string { return `{"list":[` + strings.Repeat(item+",", n-1) + item + `]}` }
	named := func(name string) string {
		return `{"Name":"` + name + `","Size":1,"ModTime":"2020-01-02T03:04:05Z","IsDir":false}`
	}
	const name = "Nosferatu (1922).mkv"
	movie := named(name)
	ten, many := list(movie, 10), list(movie, 20000)
	rclone := folderBounds
	rclone.answer = rcloneListBytes
	// The items after the first follow a comma.
	exact := listBounds{answer: int64(len(ten)), entry: int64(len(movie) + 1), entries: 10, name: len(name), names: 10 * len(name)}
	with := func(change func(b *listBounds)) listBounds {
		b := exact
		change(&b)
		return b
	}
	for _, c := range []struct {
		name   string
		bounds listBounds
		body   string
		// entries are the entries listed, -1 when the listing fails at
		// a bound, -2 when it fails as a share that cannot be reached;
		// most the bytes read of the answer at most, when set.
		entries int
		most    int
	}{
		{name: "more items than a folder holds, each empty", bounds: rclone, body: list("{}", 1<<20),
			entries: -1, most: len(`{"list":[`) + 3*(rclone.entries+1) + 8192},
		{name: "an item of more bytes than one holds", bounds: rclone, body: list(named(strings.Repeat("a", 8<<20)), 1), entries: -1,
			most: len(`{"list":[`) + int(rclone.entry) + 8192},
		{name: "an answer of more bytes than one holds", bounds: listBounds{answer: 64 << 10, entry: 4096, entries: 1 << 30, name: 64, names: 1 << 30},
			body: many, entries: -1, most: 64<<10 + 8192},
		{name: "a name longer than a file's", bounds: rclone, body: list(named(strings.Repeat("a", rclone.name+1)), 1), entries: -1},
		{name: "a name as long as a file's", bounds: rclone, body: list(named(strings.Repeat("a", rclone.name)), 1), entries: 1},
		{name: "a folder at its bounds", bounds: exact, body: ten, entries: 10},
		{name: "one item more", bounds: with(func(b *listBounds) { b.entries-- }), body: ten, entries: -1},
		{name: "one byte more in an item", bounds: with(func(b *listBounds) { b.entry-- }), body: ten, entries: -1},
		{name: "one byte more in the answer", bounds: with(func(b *listBounds) { b.answer-- }), body: ten, entries: -1},
		{name: "one byte more in a name", bounds: with(func(b *listBounds) { b.name-- }), body: ten, entries: -1},
		{name: "one byte more in the names", bounds: with(func(b *listBounds) { b.names-- }), body: ten, entries: -1},
		{name: "an empty folder", bounds: rclone, body: `{"list":null}`},
		{name: "what else the answer tells", bounds: rclone, body: `{"more":{"of":[1,"it"]},"list":[` + movie + `],"last":true}`, entries: 1},
		{name: "an answer cut short", bounds: rclone, body: `{"list":[` + movie + `,`, entries: -2},
		{name: "a list cut short", bounds: rclone, body: `{"list":[` + movie, entries: -2},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			server := serveAnswer(t, http.MethodPost, "/operations/list", http.StatusOK, c.body)
			address, err := url.Parse("rclone://" + strings.TrimPrefix(server.URL, "http://") + "/movies")
			if err != nil {
				t.Fatal(err)
			}
			share := newRclone(address, shareAccess{remote: "media"})
			t.Cleanup(share.close)
			if share.bounds != rclone {
				t.Fatalf("an rclone remote's folder is listed within %+v, want %+v", share.bounds, rclone)
			}
			share.bounds = c.bounds
			counted := countReads(share.client)
			entries, err := share.list(t.Context(), "")
			read := int(counted.read.Load())
			switch {
			case c.entries >= 0 && (err != nil || len(entries) != c.entries):
				t.Fatalf("%d entries listed, %v; want %d", len(entries), err, c.entries)
			case c.entries == -1 && (!boundMet(err) || cap(entries) != 0):
				t.Fatalf("%d entries listed, with room for %d, of an answer of %d bytes, %d read: %v; want the bound of a listing met", len(entries),
					cap(entries), len(c.body), read, err)
			case c.entries == -2 && (!errors.Is(err, errShareUnreachable) || boundMet(err)):
				t.Fatalf("%d entries listed, %v; want a share that cannot be reached", len(entries), err)
			case c.most > 0 && read > c.most:
				t.Fatalf("%d bytes read of an answer of %d, want %d at most", read, len(c.body), c.most)
			}
		})
	}

	t.Run("the remotes of an instance", func(t *testing.T) {
		t.Parallel()
		body := `{"remotes":[` + strings.Repeat(`"",`, 1<<20) + `""]}`
		server := serveAnswer(t, http.MethodPost, "/config/listremotes", http.StatusOK, body)
		share := newRclone(&url.URL{Host: strings.TrimPrefix(server.URL, "http://")}, shareAccess{})
		t.Cleanup(share.close)
		counted := countReads(share.client)
		remotes, err := share.remotes(t.Context())
		if read := counted.read.Load(); !errors.Is(err, errShareUnreachable) || read > maxCallBytes {
			t.Fatalf("%d remotes listed of an answer of %d bytes, %d read: %v; want none, and %d bytes read at most", len(remotes), len(body), read, err,
				maxCallBytes)
		}
	})
}

// A bucket's listing follows the bounds of a folder's, over its pages:
// its entries, the bytes of a name and of the names kept, those of a
// page and of one of its entries.
func TestABucketListingIsReadWithinItsBounds(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const name = "Movie A (1950).mkv"
	for i := range 7 {
		write(t, filepath.Join(dir, "movies", "Movie "+string(rune('A'+i))+" (1950).mkv"), 10)
	}
	write(t, filepath.Join(dir, "movies", "Shows", "Old", "The General (1926).mkv"), 10)
	_, server := serveS3(t, dir, "media", "AKIDTEST", "secret")
	// The bucket lists 7 files and a folder, 2 entries a page.
	exact := listBounds{answer: 4096, entry: 1024, entries: 8, name: len(name), names: 7*len(name) + len("Shows")}
	with := func(change func(b *listBounds)) listBounds {
		b := exact
		change(&b)
		return b
	}
	for _, c := range []struct {
		name   string
		bounds listBounds
		whole  bool
	}{
		{name: "a folder at its bounds", bounds: exact, whole: true},
		{name: "one entry more", bounds: with(func(b *listBounds) { b.entries-- })},
		{name: "one byte more in a name", bounds: with(func(b *listBounds) { b.name-- })},
		{name: "one byte more in the names", bounds: with(func(b *listBounds) { b.names-- })},
		{name: "a page of more bytes than one holds", bounds: with(func(b *listBounds) { b.answer = 200 })},
		{name: "an entry of more bytes than one holds", bounds: with(func(b *listBounds) { b.entry = 100 })},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r, err := newRemote(shareAccess{address: "s3://media/movies", endpoint: server.URL, region: "eu-test-1", user: "AKIDTEST",
				password: "secret"}, hostKeys{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(r.close)
			share := r.(*s3Share)
			if share.bounds != folderBounds {
				t.Fatalf("a bucket's folder is listed within %+v, want %+v", share.bounds, folderBounds)
			}
			share.bounds = c.bounds
			entries, err := share.list(t.Context(), "")
			switch {
			case c.whole && (err != nil || len(entries) != 8):
				t.Fatalf("%d entries listed, %v; want 8", len(entries), err)
			case !c.whole && !boundMet(err):
				t.Fatalf("%d entries listed, %v; want the bound of a listing met", len(entries), err)
			}
		})
	}
}
