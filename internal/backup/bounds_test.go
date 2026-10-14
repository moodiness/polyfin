package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/webdav"
)

// A destination that moves no byte for this long fails, in these tests.
const testIdle = 300 * time.Millisecond

// stall is how a test server stops: it never moves a byte again, until
// the test ends.
type stall struct{ release chan struct{} }

func newStall(t *testing.T) stall {
	s := stall{release: make(chan struct{})}
	t.Cleanup(func() { close(s.release) })
	return s
}

// stallingDAV is a WebDAV server, without authentication, that stalls
// as asked: reading the body of an upload after a few bytes, or answering
// a download after half the file. endless answers the test file read back
// with endless bytes, counting those it wrote; extra answers a download
// with more bytes than the file has.
type stallingDAV struct {
	url, dir                   string
	stallUpload, stallDownload atomic.Bool
	endless, extra             atomic.Bool
	written                    atomic.Int64
	uploadStalled              chan struct{}
	once                       sync.Once
	stall                      stall
}

func newStallingDAV(t *testing.T) *stallingDAV {
	t.Helper()
	d := &stallingDAV{dir: t.TempDir(), uploadStalled: make(chan struct{})}
	handler := &webdav.Handler{Prefix: "/dav", FileSystem: webdav.Dir(d.dir), LockSystem: webdav.NewMemLS()}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && d.stallUpload.Load():
			_, _ = io.ReadFull(r.Body, make([]byte, 4))
			d.once.Do(func() { close(d.uploadStalled) })
			<-d.stall.release
			return
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, ".polyfin-test-") && d.endless.Load():
			chunk := make([]byte, 32<<10)
			for d.written.Load() < 32<<20 {
				n, err := w.Write(chunk)
				d.written.Add(int64(n))
				if err != nil {
					return
				}
			}
			return
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, ".dump") && (d.stallDownload.Load() || d.extra.Load()):
			data, err := os.ReadFile(filepath.Join(d.dir, filepath.Base(r.URL.Path)))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			if d.extra.Load() {
				_, _ = w.Write(append(data, bytes.Repeat([]byte("x"), 1<<20)...))
				return
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(data)))
			_, _ = w.Write(data[:len(data)/2])
			w.(http.Flusher).Flush()
			<-d.stall.release
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	// Cleanups run last first: the stalled handlers are released before
	// the server waits for them to end.
	d.stall = newStall(t)
	d.url = server.URL + "/dav/"
	return d
}

// stallingConn is a server's connection that stops reading, and writing,
// once it read after bytes.
type stallingConn struct {
	net.Conn
	after int64
	read  atomic.Int64
	stall stall
}

func (c *stallingConn) Read(p []byte) (int, error) {
	if c.read.Load() >= c.after {
		<-c.stall.release
		return 0, io.EOF
	}
	n, err := c.Conn.Read(p)
	c.read.Add(int64(n))
	return n, err
}

// newStallingSFTP is an SFTP server for user and password, keeping its
// files in dir, that stalls on each connection once it read after bytes.
func newStallingSFTP(t *testing.T, user, password, dir string, after int64) *testSFTP {
	t.Helper()
	config := &ssh.ServerConfig{PasswordCallback: func(meta ssh.ConnMetadata, given []byte) (*ssh.Permissions, error) {
		if meta.User() == user && string(given) == password {
			return nil, nil
		}
		return nil, os.ErrPermission
	}}
	config.AddHostKey(newHostKey(t))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	stalled := newStall(t)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveSFTP(&stallingConn{Conn: conn, after: after, stall: stalled}, config, dir)
		}
	}()
	return &testSFTP{host: "127.0.0.1", port: listener.Addr().(*net.TCPAddr).Port, dir: dir}
}

// within fails the test unless do returns within limit.
func within(t *testing.T, limit time.Duration, what string, do func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		do()
	}()
	select {
	case <-done:
	case <-time.After(limit):
		t.Fatalf("%s did not return within %s", what, limit)
	}
}

// A destination that stalls mid-upload, an HTTP one or an SFTP one, fails
// on its own once it moved no byte for the idle time: the backup is made,
// the other destinations get it, and the run ends.
func TestAStalledDestinationFailsAlone(t *testing.T) {
	h := withDestinations(t)
	h.service.idle = testIdle
	h.kept = 7
	stalled := newStallingDAV(t)
	stalled.stallUpload.Store(true)
	// The SFTP server stalls past what signing in and listing read: in the
	// middle of a backup of 2 MB.
	big := bytes.Repeat([]byte("PGDMP"), 400<<10)
	if err := os.WriteFile(filepath.Join(h.dir, "polyfin-20260930-040000.dump"), big, 0o600); err != nil {
		t.Fatal(err)
	}
	sftpServer := newStallingSFTP(t, "polyfin", "sftp password", t.TempDir(), 256<<10)
	healthy := newTestWebDAV(t, "polyfin", "dav password")
	stalledDAV := h.create(t, Draft{Kind: KindWebDAV, Address: new(stalled.url)})
	stalledSFTP := h.create(t, Draft{Kind: KindSFTP, Address: new(sftpServer.host), Port: new(sftpServer.port), Folder: new(sftpServer.dir),
		Username: new("polyfin"), Secret: new("sftp password")})
	h.create(t, Draft{Kind: KindWebDAV, Address: new(healthy.url), Username: new("polyfin"), Secret: new("dav password")})

	within(t, 20*time.Second, "a backup with stalled destinations", func() { h.runAt(t, time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local)) })
	if got := fileNames(t, healthy.dir); !slices.Equal(got, []string{"polyfin-20260930-040000.dump", "polyfin-20261001-040000.dump"}) {
		t.Errorf("the healthy destination: %q", got)
	}
	for _, id := range []Destination{stalledDAV, stalledSFTP} {
		d, err := h.service.Destination(t.Context(), id.ID)
		if err != nil || d.Error == "" || d.Problem(time.Date(2026, 10, 1, 5, 0, 0, 0, time.Local)) != ProblemFailed {
			t.Errorf("%s: %+v %v", id.Name(), d, err)
		}
	}
	if status, _ := h.service.Status(t.Context()); status.Error != "" || status.File != "polyfin-20261001-040000.dump" {
		t.Errorf("the backup: %+v", status)
	}
}

// A backup before a restore never waits on a destination: while a run
// sends to a stalled one, a restore starts at once.
func TestARestoreDoesNotWaitOnADestination(t *testing.T) {
	h := withDestinations(t)
	stalled := newStallingDAV(t)
	stalled.stallUpload.Store(true)
	h.create(t, Draft{Kind: KindWebDAV, Address: new(stalled.url)})
	h.service.now = func() time.Time { return time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local) }
	ctx, cancel := context.WithCancel(t.Context())
	ran := make(chan error, 1)
	go func() { ran <- h.service.Run(ctx) }()
	defer func() {
		cancel()
		<-ran
	}()
	select {
	case <-stalled.uploadStalled:
	case <-time.After(10 * time.Second):
		t.Fatal("the upload never started")
	}
	h.service.now = func() time.Time { return time.Date(2026, 10, 1, 4, 5, 0, 0, time.Local) }
	h.service.mu.Lock()
	h.service.plan = &Plan{ID: "prepared", File: "polyfin-20261001-040000.dump", path: filepath.Join(h.dir, "polyfin-20261001-040000.dump")}
	h.service.mu.Unlock()
	within(t, 5*time.Second, "a restore during a stalled upload", func() {
		if err := h.service.Start(t.Context(), "prepared", true, "root"); err != nil {
			t.Error(err)
		}
	})
	if _, err := os.Stat(filepath.Join(h.dir, "polyfin-20261001-040500-before-restore.dump")); err != nil {
		t.Errorf("the backup before the restore: %v", err)
	}
}

// A destination stalling mid-download fails Prepare once idle; one sending
// more than it lists fails it at once; neither leaves a file. The test
// file read back is never more than written: an endless answer fails.
func TestDownloadsAreBounded(t *testing.T) {
	h := withDestinations(t)
	h.service.idle = testIdle
	h.service.cfg.PgRestore = "true"
	dav := newStallingDAV(t)
	name := "polyfin-20261001-040000.dump"
	if err := os.WriteFile(filepath.Join(dav.dir, name), bytes.Repeat([]byte("PGDMP"), 1000), 0o600); err != nil {
		t.Fatal(err)
	}
	d := h.create(t, Draft{Kind: KindWebDAV, Address: new(dav.url)})

	dav.stallDownload.Store(true)
	within(t, 10*time.Second, "a stalled download", func() {
		if _, err := h.service.Prepare(t.Context(), &d.ID, name); err == nil {
			t.Error("a stalled download prepared a restore")
		}
	})
	dav.stallDownload.Store(false)
	dav.extra.Store(true)
	if _, err := h.service.Prepare(t.Context(), &d.ID, name); !errors.Is(err, errTooLarge) {
		t.Errorf("a download larger than listed: %v", err)
	}
	if got := fileNames(t, h.dir); len(got) != 0 {
		t.Errorf("left in the folder: %q", got)
	}

	dav.endless.Store(true)
	_, err := h.service.TestDestination(t.Context(), d.ID)
	if !errors.Is(err, errTooLarge) || dav.written.Load() > 8<<20 {
		t.Errorf("an endless test file: %v, %d bytes sent", err, dav.written.Load())
	}
}

// A listing whose continuation token repeats fails at once rather than
// asking forever. Pages holding no key, or only keys listed before, as
// storages may send, are followed: the file on the last page is found.
func TestS3ListingEnds(t *testing.T) {
	pages := map[string][]string{
		// A key per page, and the same token each time.
		"the same token": {
			`<Contents><Key>p/polyfin-1.dump</Key><Size>1</Size></Contents><IsTruncated>true</IsTruncated><NextContinuationToken>again</NextContinuationToken>`,
			`<Contents><Key>p/polyfin-2.dump</Key><Size>1</Size></Contents><IsTruncated>true</IsTruncated><NextContinuationToken>again</NextContinuationToken>`,
		},
		// An empty page, a key listed again, then the file.
		"pages with nothing new": {
			`<Contents><Key>p/polyfin-1.dump</Key><Size>1</Size></Contents><IsTruncated>true</IsTruncated><NextContinuationToken>a</NextContinuationToken>`,
			`<IsTruncated>true</IsTruncated><NextContinuationToken>b</NextContinuationToken>`,
			`<Contents><Key>p/polyfin-1.dump</Key><Size>1</Size></Contents><IsTruncated>true</IsTruncated><NextContinuationToken>c</NextContinuationToken>`,
			`<Contents><Key>p/polyfin-20261001-040000.dump</Key><Size>7</Size></Contents><IsTruncated>false</IsTruncated>`,
		},
	}
	for name, answers := range pages {
		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			page := int(requests.Add(1))
			fmt.Fprintf(w, `<ListBucketResult>%s</ListBucketResult>`, answers[min(page, len(answers))-1])
		}))
		r, err := newS3Remote(Destination{Address: server.URL, Bucket: "backups", Folder: "p", Username: "a"}, "b", testIdle)
		if err != nil {
			t.Fatal(err)
		}
		files, err := r.list(t.Context())
		server.Close()
		if name == "the same token" {
			if !errors.Is(err, errListing) || requests.Load() > 3 {
				t.Errorf("%s: %v after %d requests", name, err, requests.Load())
			}
			continue
		}
		want := []remoteFile{{name: "polyfin-1.dump", size: 1}, {name: "polyfin-20261001-040000.dump", size: 7}}
		if err != nil || !slices.Equal(files, want) || requests.Load() != 4 {
			t.Errorf("%s: %v %v after %d requests", name, files, err, requests.Load())
		}
	}
}

// A WebDAV server that does not tell a file's length lists it as of
// unknown size; it is downloaded whole, up to maxDownload, and restored.
func TestAnUnknownSizeIsDownloaded(t *testing.T) {
	h := withDestinations(t)
	h.service.cfg.PgRestore = fakePgRestore(t)
	name := "polyfin-20261001-040000.dump"
	data := bytes.Repeat([]byte("PGDMP"), 1000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			fmt.Fprintf(w, `<?xml version="1.0"?><D:multistatus xmlns:D="DAV:">`+
				`<D:response><D:href>/dav/</D:href><D:propstat><D:prop><D:resourcetype><D:collection/></D:resourcetype></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>`+
				`<D:response><D:href>/dav/%s</D:href><D:propstat><D:prop><D:resourcetype/></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat>`+
				`<D:propstat><D:prop><D:getcontentlength/></D:prop><D:status>HTTP/1.1 404 Not Found</D:status></D:propstat></D:response></D:multistatus>`, name)
		case http.MethodGet:
			_, _ = w.Write(data)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	d := h.create(t, Draft{Kind: KindWebDAV, Address: new(server.URL + "/dav/")})
	listing, err := h.service.Backups(t.Context())
	if err != nil || len(listing.Destinations) != 1 || len(listing.Destinations[0].Files) != 1 || listing.Destinations[0].Files[0].Size != -1 {
		t.Fatalf("listing: %+v %v", listing, err)
	}
	plan, err := h.service.Prepare(t.Context(), &d.ID, name)
	if err != nil || plan.Size != int64(len(data)) {
		t.Fatalf("prepared: %+v %v", plan, err)
	}
	if !h.service.Discard(plan.ID) {
		t.Error("the plan was not prepared")
	}
}

// A plan whose time ran out while the backup before its restore was made,
// which then failed, is dropped with its file.
func TestAPlanExpiredDuringAFailedStartIsDropped(t *testing.T) {
	h := newHarness(t, nil)
	h.service.cfg.PgRestore = fakePgRestore(t)
	plan, err := h.service.PrepareUpload(t.Context(), "sent.dump", strings.NewReader("PGDMP"))
	if err != nil {
		t.Fatal(err)
	}
	// The hour passed, and the timer found the restore under way.
	h.service.mu.Lock()
	h.service.plan.expires = time.Now().Add(-time.Second)
	h.service.mu.Unlock()
	t.Setenv("FAKE_PG_DUMP_FAIL", "1")
	if err := h.service.Start(t.Context(), plan.ID, true, "root"); err == nil {
		t.Fatal("a restore started without its backup first")
	}
	if got := restoreFiles(t, h.dir); len(got) != 0 {
		t.Errorf("left: %q", got)
	}
	if err := h.service.Start(t.Context(), plan.ID, true, "root"); !errors.Is(err, ErrNoPlan) {
		t.Errorf("the plan stayed: %v", err)
	}

	// One still in its time stays, to try again.
	t.Setenv("FAKE_PG_DUMP_FAIL", "1")
	plan, err = h.service.PrepareUpload(t.Context(), "sent.dump", strings.NewReader("PGDMP"))
	if err != nil {
		t.Fatal(err)
	}
	if err := h.service.Start(t.Context(), plan.ID, true, "root"); err == nil || errors.Is(err, ErrNoPlan) {
		t.Fatalf("start: %v", err)
	}
	if got := restoreFiles(t, h.dir); len(got) != 1 {
		t.Errorf("a plan in its time lost its file: %q", got)
	}
}

// A read left pending on a destination's connection, as net/http keeps
// one while it sends a request's body and as SSH keeps one throughout,
// lasts while writes move bytes, longer than idle, and ends once nothing
// has moved for idle: an upload lasting longer than idle goes through,
// over HTTP and SFTP alike.
func TestWritesKeepAPendingReadAlive(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		if peer, err := listener.Accept(); err == nil {
			defer peer.Close()
			_, _ = io.Copy(io.Discard, peer)
		}
	}()
	const idle = 2 * time.Second
	conn, err := dialIdle(t.Context(), "tcp", listener.Addr().String(), idle)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	read := make(chan error, 1)
	go func() {
		_, err := conn.Read(make([]byte, 1))
		read <- err
	}()
	chunk := make([]byte, 1024)
	for until := time.Now().Add(9 * idle / 4); time.Now().Before(until); {
		if _, err := conn.Write(chunk); err != nil {
			t.Fatalf("a write while bytes move: %v", err)
		}
		select {
		case err := <-read:
			t.Fatalf("the pending read ended while writes moved bytes: %v", err)
		case <-time.After(50 * time.Millisecond):
		}
	}
	select {
	case err := <-read:
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Errorf("once nothing moved, the read ended with %v", err)
		}
	case <-time.After(10 * idle):
		t.Fatal("the pending read went on with nothing moving")
	}
}

// fakePgRestore is a pg_restore that describes every file as a backup
// whose schema is the first migration's: a restore can be prepared from
// any file.
func fakePgRestore(t *testing.T) string {
	t.Helper()
	migration, err := os.ReadFile(filepath.Join("..", "database", "migrations", "0001_server_identity.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(migration)
	script := `#!/bin/sh
case "$1" in
--list) printf '1; 0 0 TABLE DATA public schema_migrations polyfin\n2; 1 1 TABLE public settings polyfin\n' ;;
*) printf 'COPY public.schema_migrations (version, name, checksum, applied_at) FROM stdin;\n1\tserver_identity\t` +
		hex.EncodeToString(sum[:]) + `\t2026-01-01\n\\.\n' ;;
esac
`
	path := filepath.Join(t.TempDir(), "pg_restore")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// restoreFiles lists the files downloaded or sent to be restored in dir.
func restoreFiles(t *testing.T, dir string) []string {
	t.Helper()
	var names []string
	for _, name := range fileNames(t, dir) {
		if restoreTemporary.MatchString(name) {
			names = append(names, name)
		}
	}
	return names
}

// While a restore is under way, nothing else is prepared, and its file
// stays; an upload in progress keeps its file while another is prepared.
func TestARestoreUnderWayKeepsItsFile(t *testing.T) {
	h := newHarness(t, nil)
	h.service.cfg.PgRestore = "true"
	underWay := filepath.Join(h.dir, ".polyfin-restore-0123456789abcdef.dump")
	if err := os.WriteFile(underWay, []byte("PGDMP"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.service.plan = &Plan{ID: "restoring", path: underWay, temporary: true}
	h.service.restoring = true
	if _, err := h.service.PrepareUpload(t.Context(), "other.dump", strings.NewReader("PGDMP")); !errors.Is(err, ErrRestoring) {
		t.Errorf("an upload during a restore: %v", err)
	}
	if got := restoreFiles(t, h.dir); !slices.Equal(got, []string{filepath.Base(underWay)}) {
		t.Errorf("files during a restore: %q", got)
	}
	if h.service.Discard("restoring") {
		t.Error("a restore under way was discarded")
	}
	h.service.restoring, h.service.plan = false, nil
	if err := os.Remove(underWay); err != nil {
		t.Fatal(err)
	}

	reader, writer := io.Pipe()
	uploaded := make(chan error, 1)
	go func() {
		_, err := h.service.PrepareUpload(t.Context(), "slow.dump", reader)
		uploaded <- err
	}()
	if _, err := writer.Write([]byte("PGDMP")); err != nil {
		t.Fatal(err)
	}
	slow := restoreFiles(t, h.dir)
	if len(slow) != 1 {
		t.Fatalf("the upload in progress: %q", slow)
	}
	if _, err := h.service.PrepareUpload(t.Context(), "quick.dump", strings.NewReader("not a backup")); !errors.Is(err, ErrNotABackup) {
		t.Errorf("another upload: %v", err)
	}
	if got := restoreFiles(t, h.dir); !slices.Equal(got, slow) {
		t.Errorf("the upload in progress lost its file: %q", got)
	}
	writer.CloseWithError(errors.New("cancelled"))
	if err := <-uploaded; err == nil {
		t.Error("a cancelled upload was prepared")
	}
}

// A backup prepared and never restored loses its file when discarded,
// replaced or expired; those a stop left are deleted at startup.
func TestRestoreFilesDoNotStay(t *testing.T) {
	h := newHarness(t, nil)
	h.service.cfg.PgRestore = fakePgRestore(t)
	prepare := func() Plan {
		t.Helper()
		plan, err := h.service.PrepareUpload(t.Context(), "sent.dump", strings.NewReader("PGDMP"))
		if err != nil {
			t.Fatal(err)
		}
		return plan
	}
	first := prepare()
	if got := restoreFiles(t, h.dir); len(got) != 1 {
		t.Fatalf("prepared: %q", got)
	}
	second := prepare()
	if got := restoreFiles(t, h.dir); len(got) != 1 || filepath.Join(h.dir, got[0]) != second.path {
		t.Errorf("replaced: %q", got)
	}
	if h.service.Discard(first.ID) || !h.service.Discard(second.ID) || len(restoreFiles(t, h.dir)) != 0 {
		t.Errorf("discarded: %q", restoreFiles(t, h.dir))
	}

	h.service.planLifetime = 100 * time.Millisecond
	expiring := prepare()
	deadline := time.Now().Add(5 * time.Second)
	for len(restoreFiles(t, h.dir)) != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := restoreFiles(t, h.dir); len(got) != 0 {
		t.Errorf("expired: %q", got)
	}
	if err := h.service.Start(t.Context(), expiring.ID, true, "root"); !errors.Is(err, ErrNoPlan) {
		t.Errorf("an expired plan started: %v", err)
	}

	others := []string{"notes.txt", "polyfin-20261001-040000.dump", ".polyfin-restore-x.dump"}
	for _, name := range append(slices.Clone(others), ".polyfin-restore-0123456789abcdef.dump", ".polyfin-restore-fedcba9876543210.dump") {
		if err := os.WriteFile(filepath.Join(h.dir, name), []byte("PGDMP"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h.service.RemoveRestoreLeftovers()
	want := slices.Clone(others)
	slices.Sort(want)
	if got := fileNames(t, h.dir); !slices.Equal(got, want) {
		t.Errorf("after startup:\n%q\nwant\n%q", got, want)
	}
}
