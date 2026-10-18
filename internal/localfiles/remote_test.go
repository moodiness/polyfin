package localfiles

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/moodiness/polyfin/internal/sigv4"
)

// The fakes of the storages a folder may be on: an S3-compatible bucket,
// an SFTP server and an rclone instance serving its remote control
// interface, each serving a folder of the test's disk.

// fakeS3 serves dir as the bucket of its name, path-style, to requests
// signed with key, and keeps the ranges of the objects' reads.
type fakeS3 struct {
	dir, bucket string
	key         sigv4.Key
	// pageSize is how many entries a page of a listing holds.
	pageSize int
	// rangesOK has a range answered with 200 OK and its Content-Range,
	// as rclone serve s3 answers it.
	rangesOK bool

	mu     sync.Mutex
	ranges []string
}

func serveS3(t *testing.T, dir, bucket, access, secret string) (*fakeS3, *httptest.Server) {
	t.Helper()
	f := &fakeS3{dir: dir, bucket: bucket, key: sigv4.Key{Access: access, Secret: secret, Region: "eu-test-1"}, pageSize: 2}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	return f, server
}

// s3Error answers an S3 error with its code.
func s3Error(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, "<Error><Code>"+code+"</Code></Error>")
}

// signed reports whether a request carries the signature its key gives.
func (f *fakeS3) signed(r *http.Request) bool {
	at, err := time.Parse("20060102T150405Z", r.Header.Get("X-Amz-Date"))
	if err != nil {
		return false
	}
	check := r.Clone(r.Context())
	check.URL.Host = r.Host
	check.Header = http.Header{}
	f.key.Sign(check, r.Header.Get("X-Amz-Content-Sha256"), at)
	return check.Header.Get("Authorization") == r.Header.Get("Authorization")
}

func (f *fakeS3) serve(w http.ResponseWriter, r *http.Request) {
	if !f.signed(r) {
		s3Error(w, http.StatusForbidden, "SignatureDoesNotMatch")
		return
	}
	bucket, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if bucket != f.bucket {
		s3Error(w, http.StatusNotFound, "NoSuchBucket")
		return
	}
	if key == "" && r.URL.Query().Get("list-type") == "2" {
		f.list(w, r)
		return
	}
	file, err := os.Open(filepath.Join(f.dir, filepath.FromSlash(key)))
	if err != nil {
		s3Error(w, http.StatusNotFound, "NoSuchKey")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		s3Error(w, http.StatusNotFound, "NoSuchKey")
		return
	}
	if r.Method == http.MethodGet {
		f.mu.Lock()
		f.ranges = append(f.ranges, r.Header.Get("Range"))
		f.mu.Unlock()
	}
	if f.rangesOK && r.Header.Get("Range") != "" {
		w = okRanges{w}
	}
	http.ServeContent(w, r, "", info.ModTime(), file)
}

// okRanges answers 200 OK in place of 206 Partial Content.
type okRanges struct{ http.ResponseWriter }

func (w okRanges) WriteHeader(status int) {
	if status == http.StatusPartialContent {
		status = http.StatusOK
	}
	w.ResponseWriter.WriteHeader(status)
}

// list answers ListObjectsV2, pageSize entries a page.
func (f *fakeS3) list(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	prefix := query.Get("prefix")
	type entry struct {
		key, prefix string
		size        int64
		modified    time.Time
	}
	var entries []entry
	seen := map[string]bool{}
	_ = filepath.WalkDir(f.dir, func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(f.dir, name)
		key := filepath.ToSlash(rel)
		rest, ok := strings.CutPrefix(key, prefix)
		if !ok {
			return nil
		}
		if folder, _, nested := strings.Cut(rest, "/"); nested {
			if !seen[folder] {
				seen[folder] = true
				entries = append(entries, entry{prefix: prefix + folder + "/"})
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		entries = append(entries, entry{key: key, size: info.Size(), modified: info.ModTime()})
		return nil
	})
	start, _ := strconv.Atoi(query.Get("continuation-token"))
	end := min(len(entries), start+f.pageSize)
	var page struct {
		XMLName  xml.Name `xml:"ListBucketResult"`
		Contents []struct {
			Key          string `xml:"Key"`
			Size         int64  `xml:"Size"`
			LastModified string `xml:"LastModified"`
		} `xml:"Contents"`
		CommonPrefixes []struct {
			Prefix string `xml:"Prefix"`
		} `xml:"CommonPrefixes"`
		IsTruncated           bool   `xml:"IsTruncated"`
		NextContinuationToken string `xml:"NextContinuationToken,omitempty"`
	}
	for _, e := range entries[min(start, end):end] {
		if e.prefix != "" {
			page.CommonPrefixes = append(page.CommonPrefixes, struct {
				Prefix string `xml:"Prefix"`
			}{e.prefix})
			continue
		}
		page.Contents = append(page.Contents, struct {
			Key          string `xml:"Key"`
			Size         int64  `xml:"Size"`
			LastModified string `xml:"LastModified"`
		}{e.key, e.size, e.modified.UTC().Format(time.RFC3339Nano)})
	}
	if end < len(entries) {
		page.IsTruncated, page.NextContinuationToken = true, strconv.Itoa(end)
	}
	w.Header().Set("Content-Type", "application/xml")
	_ = xml.NewEncoder(w).Encode(page)
}

// readRanges are the ranges the objects were read with.
func (f *fakeS3) readRanges() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.ranges)
}

// fakeSFTP serves the test's disk over SFTP to user, signed in with
// password or the key authorized, under a host key rekey changes.
type fakeSFTP struct {
	addr           string
	user, password string
	authorized     ssh.PublicKey

	mu       sync.Mutex
	hostKey  ssh.Signer
	listener net.Listener
	conns    []net.Conn
}

// newSigner makes an Ed25519 key.
func newSigner(t *testing.T) (ssh.Signer, ed25519.PrivateKey) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return signer, private
}

// openSSHKey writes a private key in the OpenSSH format, encrypted with
// passphrase when there is one.
func openSSHKey(t *testing.T, private ed25519.PrivateKey, passphrase string) string {
	t.Helper()
	var block *pem.Block
	var err error
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(private, "")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(private, "", []byte(passphrase))
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(block))
}

func serveSFTP(t *testing.T, user, password string, authorized ssh.PublicKey) *fakeSFTP {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSFTP{addr: listener.Addr().String(), user: user, password: password, authorized: authorized, listener: listener}
	f.hostKey, _ = newSigner(t)
	t.Cleanup(f.stop)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			f.mu.Lock()
			f.conns = append(f.conns, conn)
			hostKey := f.hostKey
			f.mu.Unlock()
			go f.serve(conn, hostKey)
		}
	}()
	return f
}

func (f *fakeSFTP) serve(conn net.Conn, hostKey ssh.Signer) {
	config := &ssh.ServerConfig{
		PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if meta.User() == f.user && f.password != "" && string(password) == f.password {
				return nil, nil
			}
			return nil, errors.New("refused")
		},
		PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if meta.User() == f.user && f.authorized != nil && string(key.Marshal()) == string(f.authorized.Marshal()) {
				return nil, nil
			}
			return nil, errors.New("refused")
		},
	}
	config.AddHostKey(hostKey)
	_, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		conn.Close()
		return
	}
	go ssh.DiscardRequests(requests)
	for request := range channels {
		if request.ChannelType() != "session" {
			_ = request.Reject(ssh.UnknownChannelType, "")
			continue
		}
		channel, asked, err := request.Accept()
		if err != nil {
			continue
		}
		go func() {
			for ask := range asked {
				ok := ask.Type == "subsystem" && len(ask.Payload) > 4 && string(ask.Payload[4:]) == "sftp"
				_ = ask.Reply(ok, nil)
				if ok {
					server, err := sftp.NewServer(channel, sftp.ReadOnly())
					if err == nil {
						_ = server.Serve()
						server.Close()
					}
					return
				}
			}
		}()
	}
}

// cut closes the connections open, as a server restarting does.
func (f *fakeSFTP) cut() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.conns {
		c.Close()
	}
	f.conns = nil
}

// rekey gives the server another host key, and closes the connections
// made with the former one.
func (f *fakeSFTP) rekey(t *testing.T) ssh.PublicKey {
	t.Helper()
	signer, _ := newSigner(t)
	f.mu.Lock()
	f.hostKey = signer
	f.mu.Unlock()
	f.cut()
	return signer.PublicKey()
}

func (f *fakeSFTP) stop() {
	f.listener.Close()
	f.cut()
}

// fakeRclone serves the remote control interface of an rclone instance
// knowing the remote of its name, dir on the test's disk, to user with
// password, and the remote's objects as --rc-serve does, keeping the
// ranges of their reads.
type fakeRclone struct {
	remote, dir    string
	user, password string

	mu     sync.Mutex
	ranges []string
}

func serveRclone(t *testing.T, remote, dir, user, password string) (*fakeRclone, *httptest.Server) {
	t.Helper()
	f := &fakeRclone{remote: remote, dir: dir, user: user, password: password}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	return f, server
}

func rcloneAnswer(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (f *fakeRclone) serve(w http.ResponseWriter, r *http.Request) {
	if user, password, ok := r.BasicAuth(); !ok || user != f.user || password != f.password {
		rcloneAnswer(w, http.StatusUnauthorized, map[string]any{"error": "authentication required", "status": 401})
		return
	}
	if rest, ok := strings.CutPrefix(r.URL.Path, "/["); ok {
		remote, name, _ := strings.Cut(rest, ":]/")
		file, err := os.Open(filepath.Join(f.dir, filepath.FromSlash(name)))
		if remote != f.remote || err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			f.mu.Lock()
			f.ranges = append(f.ranges, r.Header.Get("Range"))
			f.mu.Unlock()
		}
		http.ServeContent(w, r, "", info.ModTime(), file)
		return
	}
	var input struct {
		FS     string `json:"fs"`
		Remote string `json:"remote"`
	}
	_ = json.NewDecoder(r.Body).Decode(&input)
	item := func(info fs.FileInfo) map[string]any {
		return map[string]any{"Name": info.Name(), "Size": info.Size(), "ModTime": info.ModTime().Format(time.RFC3339Nano), "IsDir": info.IsDir()}
	}
	switch r.URL.Path {
	case "/config/listremotes":
		rcloneAnswer(w, http.StatusOK, map[string]any{"remotes": []string{f.remote}})
		return
	case "/operations/list", "/operations/stat":
	default:
		rcloneAnswer(w, http.StatusNotFound, map[string]any{"error": "couldn't find method", "status": 404})
		return
	}
	if input.FS != f.remote+":" {
		rcloneAnswer(w, http.StatusInternalServerError, map[string]any{"error": "didn't find section in config file", "status": 500})
		return
	}
	target := filepath.Join(f.dir, filepath.FromSlash(input.Remote))
	info, err := os.Stat(target)
	if r.URL.Path == "/operations/stat" {
		if err != nil {
			rcloneAnswer(w, http.StatusOK, map[string]any{"item": nil})
			return
		}
		rcloneAnswer(w, http.StatusOK, map[string]any{"item": item(info)})
		return
	}
	if err != nil || !info.IsDir() {
		rcloneAnswer(w, http.StatusNotFound, map[string]any{"error": "directory not found", "status": 404})
		return
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		rcloneAnswer(w, http.StatusInternalServerError, map[string]any{"error": err.Error(), "status": 500})
		return
	}
	list := []map[string]any{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		listed := item(info)
		listed["Path"] = path.Join(input.Remote, entry.Name())
		list = append(list, listed)
	}
	rcloneAnswer(w, http.StatusOK, map[string]any{"list": list})
}

func (f *fakeRclone) readRanges() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.ranges)
}

// sftpPath is the path in an SFTP address of a local folder the fake SFTP
// server serves: the folder with slashes, and on Windows a slash before its
// drive, /C:/Users/…, which the server reads as C:\Users\….
func sftpPath(dir string) string {
	dir = filepath.ToSlash(dir)
	if !strings.HasPrefix(dir, "/") {
		dir = "/" + dir
	}
	return dir
}
