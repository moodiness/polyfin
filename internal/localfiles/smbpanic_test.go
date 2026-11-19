package localfiles

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cloudsoda/go-smb2"
	"github.com/pkg/sftp"
)

// A panic of the SMB client under a request fails that request, logged
// once with its stack, and ends the session; the server goes on, and the
// next request signs in again.
func TestAPanicUnderAnSMBRequestIsItsError(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	share := newSMB(serveSMB(t).url(), "reader", "pass phrase", slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(share.close)
	err := share.do(t.Context(), func(*smb2.Share, context.Context) error {
		panic("a broken answer")
	})
	if !errors.Is(err, errShareUnreachable) || !strings.Contains(err.Error(), "a broken answer") {
		t.Fatalf("a request that panicked: %v, want the share unreachable, with the panic", err)
	}
	if got := strings.Count(logs.String(), "ended on a panic"); got != 1 || !strings.Contains(logs.String(), "stack=") {
		t.Fatalf("the panic was logged %d times, want once with its stack: %s", got, logs.String())
	}
	share.mu.Lock()
	dropped := share.session == nil
	share.mu.Unlock()
	if !dropped {
		t.Error("the session a request panicked in is kept")
	}
	if err := share.do(t.Context(), func(*smb2.Share, context.Context) error { return nil }); err != nil {
		t.Fatalf("the next request: %v", err)
	}
}

// A panic of the SMB client reading an opened file, outside any request
// to the share, fails that read, logged with its stack.
func TestAPanicReadingAnSMBFileIsItsError(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	share := newSMB(&url.URL{Scheme: "smb", Host: "127.0.0.1:1", Path: "/media"}, "reader", "pass phrase",
		slog.New(slog.NewTextHandler(&logs, nil)))
	// A file the client never opened: reading it panics in the client.
	f := &smbFile{share: share, file: &smb2.File{}, size: 1 << 10}
	if _, err := f.Read(make([]byte, 16)); !errors.Is(err, errShareUnreachable) {
		t.Fatalf("a read that panicked: %v, want the share unreachable", err)
	}
	if got := strings.Count(logs.String(), "ended on a panic"); got != 1 || !strings.Contains(logs.String(), "stack=") {
		t.Fatalf("the panic was logged %d times, want once with its stack: %s", got, logs.String())
	}
}

// panicReader panics as it is read, as a parser of a broken file may.
type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("a broken file") }
func (panicReader) Close() error             { return nil }

// A panic reading an NFO file during a scan fails that file's read,
// logged with its stack: the scan goes on, as for a file that cannot be
// read.
func TestAPanicReadingAnNFOFileFailsThatFileOnly(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	var logs syncBuffer
	e.service.logger = slog.New(slog.NewTextHandler(&logs, nil))
	list := listing{sides: map[string]found{"Nosferatu (1922)/movie.nfo": {size: 10}},
		open: func(context.Context, string) (io.ReadCloser, error) { return panicReader{}, nil }}
	infos, err := e.service.readNFOs(t.Context(), testID(1), list, map[string]string{"Nosferatu (1922)": "Nosferatu (1922)/movie.nfo"})
	if err != nil {
		t.Fatalf("the scan's NFO files: %v, want the scan to go on", err)
	}
	if info := infos["Nosferatu (1922)/movie.nfo"]; info != (nfoInfo{}) {
		t.Errorf("the NFO file that panicked gave %+v", info)
	}
	if out := logs.String(); !strings.Contains(out, "A step of a scan ended on a panic") || !strings.Contains(out, "stack=") {
		t.Fatalf("the panic was not logged with its stack: %s", out)
	}
}

// syncBuffer is a buffer logs are written to from several goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// A panic under an SFTP request, as of what reads the server's answers to
// a listing, fails that request, logged once with its stack, and drops
// the connection; the next request connects again.
func TestAPanicUnderAnSFTPRequestIsItsError(t *testing.T) {
	t.Parallel()
	var logs syncBuffer
	server := serveSFTP(t, "reader", "pass phrase", nil)
	r, err := newRemote(shareAccess{address: "sftp://" + server.addr + sftpPath(t.TempDir()), user: "reader", password: "pass phrase"},
		hostKeys{}, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.close)
	share := r.(*sftpShare)
	err = share.do(t.Context(), func(*sftp.Client) error { panic("a broken answer") })
	if !errors.Is(err, errShareUnreachable) || !strings.Contains(err.Error(), "a broken answer") {
		t.Fatalf("a request that panicked: %v, want the share unreachable, with the panic", err)
	}
	if out := logs.String(); strings.Count(out, "ended on a panic") != 1 || !strings.Contains(out, "stack=") {
		t.Fatalf("the panic was not logged once with its stack: %s", out)
	}
	share.mu.Lock()
	dropped := share.client == nil
	share.mu.Unlock()
	if !dropped {
		t.Error("the connection a request panicked in is kept")
	}
	if _, err := share.list(t.Context(), ""); err != nil {
		t.Fatalf("the next listing: %v", err)
	}
}

// panicFile panics at each of its reads, seeks and its close, as the SFTP
// client reading a broken answer may.
type panicFile struct{}

func (panicFile) Read([]byte) (int, error)          { panic("a broken read") }
func (panicFile) ReadAt([]byte, int64) (int, error) { panic("a broken read") }
func (panicFile) Seek(int64, int) (int64, error)    { panic("a broken read") }
func (panicFile) Close() error                      { panic("a broken read") }

// A panic of the SFTP client under a file opened on an SFTP folder, read
// outside any request to the share, fails that read, seek or close,
// logged with its stack, and drops the connection; the next request
// connects again.
func TestAPanicReadingAnSFTPFileIsItsError(t *testing.T) {
	t.Parallel()
	var logs syncBuffer
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Nosferatu (1922).mkv"), 10)
	server := serveSFTP(t, "reader", "pass phrase", nil)
	r, err := newRemote(shareAccess{address: "sftp://" + server.addr + sftpPath(dir), user: "reader", password: "pass phrase"},
		hostKeys{}, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.close)
	share := r.(*sftpShare)
	steps := map[string]func(f io.ReadSeekCloser) error{
		"read": func(f io.ReadSeekCloser) error { _, err := f.Read(make([]byte, 4)); return err },
		"read at": func(f io.ReadSeekCloser) error {
			_, err := f.(io.ReaderAt).ReadAt(make([]byte, 4), 2)
			return err
		},
		"seek":  func(f io.ReadSeekCloser) error { _, err := f.Seek(2, io.SeekStart); return err },
		"close": func(f io.ReadSeekCloser) error { return f.Close() },
	}
	for name, step := range steps {
		opened, _, err := share.open(t.Context(), "Nosferatu (1922).mkv")
		if err != nil {
			t.Fatalf("%s: opening the file: %v", name, err)
		}
		f := opened.(*sftpFile)
		if err := f.file.Close(); err != nil {
			t.Fatal(err)
		}
		f.file = panicFile{}
		if err := step(f); !errors.Is(err, errShareUnreachable) || !strings.Contains(err.Error(), "a broken read") {
			t.Errorf("a %s that panicked: %v, want the share unreachable, with the panic", name, err)
		}
		share.mu.Lock()
		dropped := share.client == nil
		share.mu.Unlock()
		if !dropped {
			t.Errorf("the connection a %s panicked under is kept", name)
		}
	}
	if out := logs.String(); strings.Count(out, "ended on a panic") != len(steps) || !strings.Contains(out, "stack=") {
		t.Fatalf("the panics were not logged once each with their stack: %s", out)
	}
	if _, err := share.list(t.Context(), ""); err != nil {
		t.Fatalf("the next listing: %v", err)
	}
}

// panicSeeker is a file of a share whose reads panic.
type panicSeeker struct{ panicReader }

func (panicSeeker) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekEnd {
		return 100, nil
	}
	return offset, nil
}

// A panic serving a file of a share to ffprobe ends that request, which
// is cut short, logged with its stack; the server goes on serving.
func TestAPanicServingAShareFileIsLogged(t *testing.T) {
	t.Parallel()
	var logs syncBuffer
	list := listing{files: map[string]found{"clip.mkv": {size: 100}},
		open: func(context.Context, string) (io.ReadCloser, error) { return panicSeeker{}, nil }}
	target, stop, err := probeServer(list, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	for range 2 {
		response, err := http.Get(target("clip.mkv"))
		if err == nil {
			_, err = io.ReadAll(response.Body)
			response.Body.Close()
		}
		if err == nil {
			t.Fatal("the file that panicked was served whole")
		}
	}
	if out := logs.String(); strings.Count(out, "Serving a file of a share to ffprobe ended on a panic") != 2 || !strings.Contains(out, "stack=") {
		t.Fatalf("the panics were not logged with their stack: %s", out)
	}
}
