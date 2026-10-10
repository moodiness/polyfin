package localfiles

import (
	"bytes"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"golang.org/x/net/webdav"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/secrets"
)

// serveWebDAV serves dir over WebDAV to user with password, as a NAS does.
func serveWebDAV(t *testing.T, dir, user, password string) *httptest.Server {
	t.Helper()
	handler := &webdav.Handler{FileSystem: webdav.Dir(dir), LockSystem: webdav.NewMemLS()}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != user || p != password {
			w.Header().Set("WWW-Authenticate", `Basic realm="share"`)
			http.Error(w, "sign in", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

// content is a file's bytes, each telling its offset.
func content(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251)
	}
	return data
}

// fileKey is the key Streams gives a folder's file.
func (e env) fileKey(id accounts.ID, rel string) string {
	e.t.Helper()
	var file accounts.ID
	if err := e.pool.QueryRow(e.t.Context(), "SELECT id FROM local_files WHERE addon_id = $1 AND path = $2", id, rel).Scan(&file); err != nil {
		e.t.Fatal(err)
	}
	return file.String() + filepath.Ext(rel)
}

// A WebDAV folder is scanned folder by folder and matched as a local one
// is, its password stored sealed, and its files read from the share with
// byte ranges.
func TestWebDAVFoldersAreScannedAndRead(t *testing.T) {
	e := newEnv(t)
	box, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e.service.box = box
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Night of the Living Dead (1968).mkv"), 10)
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922) - 1080p.mkv"), 10)
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Extras", "The General (1926).mkv"), 10)
	write(t, filepath.Join(dir, "movies", "An Unknown Picture (1950).mkv"), 10)
	write(t, filepath.Join(dir, "elsewhere", "The General (1926).mkv"), 10)
	data := content(300_000)
	if err := os.WriteFile(filepath.Join(dir, "movies", "Night of the Living Dead (1968).mkv"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	server := serveWebDAV(t, dir, "reader", "pass phrase")
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: server.URL + "/movies/", Kind: KindMovies, User: "reader",
		Password: "pass phrase"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	want := map[string]string{
		"Night of the Living Dead (1968).mkv":           "tt0063350",
		"Nosferatu (1922)/Nosferatu (1922) - 1080p.mkv": "tt0013442",
		"An Unknown Picture (1950).mkv":                 "unmatched: not_found",
	}
	if got := e.files(addon.ID); !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil || folder.Error != "" || folder.Share != ShareWebDAV || folder.User != "reader" || !folder.PasswordSet {
		t.Errorf("folder: %+v %v", folder, err)
	}
	var stored string
	if err := e.pool.QueryRow(t.Context(), "SELECT share_password FROM local_folders WHERE addon_id = $1", addon.ID).Scan(&stored); err != nil ||
		!secrets.Sealed(stored) {
		t.Errorf("the password is not stored sealed: %v", err)
	}

	file, _, err := e.service.Open(t.Context(), e.fileKey(addon.ID, "Night of the Living Dead (1968).mkv"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if size, err := file.Seek(0, io.SeekEnd); err != nil || size != int64(len(data)) {
		t.Fatalf("size: %d %v", size, err)
	}
	for _, offset := range []int64{123_456, 7, 299_990} {
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, 10)
		if _, err := io.ReadFull(file, got); err != nil || !bytes.Equal(got, data[offset:offset+10]) {
			t.Errorf("bytes at %d: %v %v", offset, got, err)
		}
	}
}

// A share refusing the password is reported as such, and read once the
// right one is entered; a share that cannot be reached keeps the files
// found before.
func TestUnreadableSharesKeepTheirFiles(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Night of the Living Dead (1968).mkv"), 10)
	server := serveWebDAV(t, dir, "reader", "right")
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: server.URL, Kind: KindMovies, User: "reader", Password: "wrong"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	if folder, _ := e.service.Folder(t.Context(), addon.ID); folder.Error != errRefused || folder.Files != 0 {
		t.Errorf("a wrong password: %+v", folder)
	}
	if _, err := e.service.Update(t.Context(), addon.ID, Changes{Password: new("right")}); err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	if folder, _ := e.service.Folder(t.Context(), addon.ID); folder.Error != "" || folder.Files != 1 {
		t.Errorf("the right password: %+v", folder)
	}

	server.Close()
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil || folder.Error != errUnreachable || folder.Files != 1 || folder.Matched != 1 {
		t.Errorf("an unreachable share: %+v %v", folder, err)
	}
}

// relay forwards connections to target, as a network between Polyfin and
// a server; cut closes those open, as a server restarting does.
type relay struct {
	mu    sync.Mutex
	conns []net.Conn
}

func newRelay(t *testing.T, target string) (*relay, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	r := &relay{}
	t.Cleanup(r.cut)
	go func() {
		for {
			in, err := listener.Accept()
			if err != nil {
				return
			}
			out, err := net.Dial("tcp", target)
			if err != nil {
				in.Close()
				continue
			}
			r.mu.Lock()
			r.conns = append(r.conns, in, out)
			r.mu.Unlock()
			go func() { _, _ = io.Copy(out, in); out.Close() }()
			go func() { _, _ = io.Copy(in, out); in.Close() }()
		}
	}()
	return r, listener.Addr().String()
}

func (r *relay) cut() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.conns {
		c.Close()
	}
	r.conns = nil
}

// An SMB share is scanned and read, and read again once its connection
// was cut, as when the server restarts. It runs when POLYFIN_TEST_SMB
// names a share holding video files, its user and password in the
// address: smb://user:password@host:port/share/folder.
func TestSMBSharesAreScannedAndRead(t *testing.T) {
	raw := os.Getenv("POLYFIN_TEST_SMB")
	if raw == "" {
		t.Skip("POLYFIN_TEST_SMB is not set")
	}
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		t.Fatal("POLYFIN_TEST_SMB is not smb://user:password@host/share/folder")
	}
	user := u.User.Username()
	password, _ := u.User.Password()
	u.User = nil
	port := u.Port()
	if port == "" {
		port = "445"
	}
	network, address := newRelay(t, net.JoinHostPort(u.Hostname(), port))
	u.Host = address
	e := newEnv(t)
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: u.String(), Kind: KindMovies, User: user, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil || folder.Error != "" || folder.Files == 0 {
		t.Fatalf("folder: %+v %v", folder, err)
	}
	var rel string
	var size int64
	if err := e.pool.QueryRow(t.Context(), "SELECT path, size FROM local_files WHERE addon_id = $1 ORDER BY path LIMIT 1", addon.ID).
		Scan(&rel, &size); err != nil {
		t.Fatal(err)
	}
	// read reads the start of the file, then its second half again after a
	// seek, which must give the same bytes.
	read := func() []byte {
		t.Helper()
		file, _, err := e.service.Open(t.Context(), e.fileKey(addon.ID, rel))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if end, err := file.Seek(0, io.SeekEnd); err != nil || end != size {
			t.Fatalf("size: %d %v", end, err)
		}
		head := make([]byte, min(8192, size))
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		if _, err := io.ReadFull(file, head); err != nil {
			t.Fatal(err)
		}
		half := len(head) / 2
		if _, err := file.Seek(int64(half), io.SeekStart); err != nil {
			t.Fatal(err)
		}
		again := make([]byte, len(head)-half)
		if _, err := io.ReadFull(file, again); err != nil || !bytes.Equal(again, head[half:]) {
			t.Fatalf("the bytes read after a seek differ: %v", err)
		}
		return head
	}
	first := read()
	network.cut()
	if again := read(); !bytes.Equal(first, again) {
		t.Error("the file read again differs")
	}

	if _, err := e.service.Update(t.Context(), addon.ID, Changes{Password: new(password + "-wrong")}); err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	if folder, _ := e.service.Folder(t.Context(), addon.ID); folder.Error != errRefused || folder.Files == 0 {
		t.Errorf("a wrong password: %+v", folder)
	}
}
