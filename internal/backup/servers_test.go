package backup

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/webdav"
)

// The servers the destinations are tested against, in the test's process:
// a WebDAV server, an SFTP server and a small S3-compatible storage, each
// keeping its files in a folder of the test.

// testWebDAV serves dir at /dav/, to user and password only; down answers
// 503 to everything.
type testWebDAV struct {
	url  string
	dir  string
	mu   sync.Mutex
	down bool
}

func newTestWebDAV(t *testing.T, user, password string) *testWebDAV {
	t.Helper()
	w := &testWebDAV{dir: t.TempDir()}
	handler := &webdav.Handler{Prefix: "/dav", FileSystem: webdav.Dir(w.dir), LockSystem: webdav.NewMemLS()}
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w.mu.Lock()
		down := w.down
		w.mu.Unlock()
		if down {
			http.Error(rw, "maintenance", http.StatusServiceUnavailable)
			return
		}
		if u, p, ok := r.BasicAuth(); !ok || u != user || p != password {
			rw.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			http.Error(rw, "who are you", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(rw, r)
	}))
	t.Cleanup(server.Close)
	w.url = server.URL + "/dav/"
	return w
}

func (w *testWebDAV) setDown(down bool) {
	w.mu.Lock()
	w.down = down
	w.mu.Unlock()
}

// testSFTP is an SFTP server for user and password, whose files are in
// dir; key is its host key.
type testSFTP struct {
	host string
	port int
	dir  string
}

func newHostKey(t *testing.T) ssh.Signer {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func newTestSFTP(t *testing.T, user, password string, key ssh.Signer, dir string) *testSFTP {
	t.Helper()
	config := &ssh.ServerConfig{PasswordCallback: func(meta ssh.ConnMetadata, given []byte) (*ssh.Permissions, error) {
		if meta.User() == user && string(given) == password {
			return nil, nil
		}
		return nil, os.ErrPermission
	}}
	config.AddHostKey(key)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveSFTP(conn, config, dir)
		}
	}()
	return &testSFTP{host: "127.0.0.1", port: listener.Addr().(*net.TCPAddr).Port, dir: dir}
}

func serveSFTP(conn net.Conn, config *ssh.ServerConfig, dir string) {
	defer conn.Close()
	_, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(requests)
	for request := range channels {
		if request.ChannelType() != "session" {
			_ = request.Reject(ssh.UnknownChannelType, "")
			continue
		}
		channel, requests, err := request.Accept()
		if err != nil {
			return
		}
		go func() {
			for r := range requests {
				ok := r.Type == "subsystem" && len(r.Payload) > 4 && string(r.Payload[4:]) == "sftp"
				_ = r.Reply(ok, nil)
				if ok {
					if server, err := sftp.NewServer(channel, sftp.WithServerWorkingDirectory(dir)); err == nil {
						_ = server.Serve()
					}
					channel.Close()
				}
			}
		}()
	}
}

// testS3 is a bucket of S3-compatible storage at url, whose objects are
// files under dir, by key. It refuses a request not signed for access, or
// whose hash is not its body's.
type testS3 struct {
	url    string
	bucket string
	dir    string
	access string
}

func newTestS3(t *testing.T, bucket, access string) *testS3 {
	t.Helper()
	s := &testS3{bucket: bucket, dir: t.TempDir(), access: access}
	server := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(server.Close)
	s.url = server.URL
	return s
}

func (s *testS3) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	sum := sha256.Sum256(body)
	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential="+s.access+"/") ||
		r.Header.Get("X-Amz-Content-Sha256") != hex.EncodeToString(sum[:]) || r.Header.Get("X-Amz-Date") == "" {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><Error><Code>InvalidAccessKeyId</Code><Message>Unknown key</Message></Error>`)
		return
	}
	bucket, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if bucket != s.bucket {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><Error><Code>NoSuchBucket</Code><Message>No bucket</Message></Error>`)
		return
	}
	path := filepath.Join(s.dir, filepath.FromSlash(key))
	switch {
	case key == "" && r.Method == http.MethodGet:
		s.list(w, r.URL.Query().Get("prefix"), r.URL.Query().Get("continuation-token"))
	case r.Method == http.MethodPut:
		_ = os.MkdirAll(filepath.Dir(path), 0o700)
		_ = os.WriteFile(path, body, 0o600)
	case r.Method == http.MethodGet:
		data, err := os.ReadFile(path)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(data)
	case r.Method == http.MethodDelete:
		_ = os.Remove(path)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// list answers ListObjectsV2 with a delimiter of /, two keys a page, which
// makes the client follow the continuation.
func (s *testS3) list(w http.ResponseWriter, prefix, token string) {
	var keys []string
	_ = filepath.WalkDir(s.dir, func(path string, entry os.DirEntry, err error) error {
		if err == nil && entry.Type().IsRegular() {
			key, _ := filepath.Rel(s.dir, path)
			key = filepath.ToSlash(key)
			if rest, ok := strings.CutPrefix(key, prefix); ok && !strings.Contains(rest, "/") {
				keys = append(keys, key)
			}
		}
		return nil
	})
	slices.Sort(keys)
	start, _ := strconv.Atoi(token)
	end := min(start+2, len(keys))
	type content struct {
		Key  string
		Size int64
	}
	var result struct {
		XMLName               xml.Name `xml:"ListBucketResult"`
		Contents              []content
		IsTruncated           bool
		NextContinuationToken string `xml:",omitempty"`
	}
	for _, key := range keys[min(start, end):end] {
		info, _ := os.Stat(filepath.Join(s.dir, filepath.FromSlash(key)))
		result.Contents = append(result.Contents, content{Key: key, Size: info.Size()})
	}
	if end < len(keys) {
		result.IsTruncated, result.NextContinuationToken = true, strconv.Itoa(end)
	}
	w.Header().Set("Content-Type", "application/xml")
	_ = xml.NewEncoder(w).Encode(result)
}

// fileNames lists the names of the files in dir, folders left out.
func fileNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			names = append(names, entry.Name())
		}
	}
	return names
}
