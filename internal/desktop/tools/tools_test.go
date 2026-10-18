package tools

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ulikunitz/xz"
)

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0o755)
		f, err := w.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type entry struct {
	name, content, link string
}

func tarXZOf(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	x, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	w := tar.NewWriter(x)
	for _, e := range entries {
		header := &tar.Header{Name: e.name, Mode: 0o755, Size: int64(len(e.content)), Typeflag: tar.TypeReg}
		if e.link != "" {
			header = &tar.Header{Name: e.name, Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: e.link}
		}
		if err := w.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if e.link == "" {
			if _, err := w.Write([]byte(e.content)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := x.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fake serves archives by path, counting the requests.
type fake struct {
	*httptest.Server
	mu       sync.Mutex
	files    map[string][]byte
	requests atomic.Int32
}

func newFake(t *testing.T, files map[string][]byte) *fake {
	f := &fake{files: files}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		f.mu.Lock()
		body, ok := f.files[r.URL.Path]
		f.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(f.Close)
	return f
}

func pin(url string, body []byte, format string) File {
	sum := sha256.Sum256(body)
	return File{URL: url, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body)), Format: format}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Two archives of one program each, as macOS's, are downloaded with
// progress, checked, unpacked into bin, and not downloaded again.
func TestEnsureDownloadsChecksAndUnpacks(t *testing.T) {
	ffmpeg := zipOf(t, map[string]string{"ffmpeg": "fake ffmpeg", "README": "left out"})
	ffprobe := zipOf(t, map[string]string{"ffprobe": "fake ffprobe"})
	server := newFake(t, map[string][]byte{"/ffmpeg.zip": ffmpeg, "/ffprobe.zip": ffprobe})
	first := pin(server.URL+"/ffmpeg.zip", ffmpeg, "zip")
	first.Into, first.Keep = "bin", []string{"ffmpeg"}
	second := pin(server.URL+"/ffprobe.zip", ffprobe, "zip")
	second.Into, second.Keep = "bin", []string{"ffprobe"}
	dir := t.TempDir()
	// An older version, removed once the new one is in place.
	if err := os.MkdirAll(filepath.Join(dir, "ffmpeg-8.1"), 0o700); err != nil {
		t.Fatal(err)
	}
	tools := Tools{Dir: dir, Client: server.Client(),
		Manifest: Manifest{Version: "9.0.2", FFmpeg: "bin/ffmpeg", FFprobe: "bin/ffprobe", Files: []File{first, second}}}

	var seen []int64
	var total int64
	paths, err := tools.Ensure(context.Background(), func(done, all int64) {
		seen = append(seen, done)
		total = all
	})
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if want := int64(len(ffmpeg) + len(ffprobe)); total != want || seen[len(seen)-1] != want {
		t.Errorf("progress ended at %d of %d, want %d", seen[len(seen)-1], total, want)
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] < seen[i-1] {
			t.Fatalf("progress went back: %v", seen)
		}
	}
	if paths.FFmpeg != filepath.Join(dir, "ffmpeg-9.0.2", "bin", "ffmpeg") || read(t, paths.FFmpeg) != "fake ffmpeg" || read(t, paths.FFprobe) != "fake ffprobe" {
		t.Errorf("paths = %+v", paths)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(paths.FFmpeg); err != nil || info.Mode().Perm()&0o100 == 0 {
			t.Errorf("ffmpeg is not executable: %v, %v", info, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "ffmpeg-9.0.2", "bin", "README")); !os.IsNotExist(err) {
		t.Errorf("a member left out was unpacked: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ffmpeg-8.1")); !os.IsNotExist(err) {
		t.Errorf("the older version was kept: %v", err)
	}
	requests := server.requests.Load()
	if again, err := tools.Ensure(context.Background(), nil); err != nil || again != paths {
		t.Fatalf("second Ensure: %+v, %v", again, err)
	}
	if server.requests.Load() != requests {
		t.Error("the second Ensure downloaded again")
	}
	// A new pin unpacks again.
	tools.Manifest.Files[0].SHA256 = strings.Repeat("0", 64)
	if _, ok := tools.Ready(); ok {
		t.Error("the tools of another checksum count as ready")
	}
}

// An archive whose checksum or size is not the pinned one is refused, and
// nothing is put in place.
func TestEnsureRefusesAWrongChecksum(t *testing.T) {
	good := zipOf(t, map[string]string{"ffmpeg": "fake ffmpeg", "ffprobe": "fake ffprobe"})
	tampered := zipOf(t, map[string]string{"ffmpeg": "evil ffmpeg", "ffprobe": "evil ffprobe"})
	larger := append(append([]byte{}, good...), 0)
	server := newFake(t, map[string][]byte{"/tampered.zip": tampered, "/larger.zip": larger})
	for _, path := range []string{"/tampered.zip", "/larger.zip"} {
		t.Run(path, func(t *testing.T) {
			file := pin(server.URL+path, good, "zip")
			file.Into = "bin"
			dir := t.TempDir()
			tools := Tools{Dir: dir, Client: server.Client(),
				Manifest: Manifest{Version: "9.0.2", FFmpeg: "bin/ffmpeg", FFprobe: "bin/ffprobe", Files: []File{file}}}
			_, err := tools.Ensure(context.Background(), nil)
			if !errors.Is(err, ErrChecksum) {
				t.Fatalf("Ensure = %v, want ErrChecksum", err)
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 0 {
				t.Errorf("the refused download left %v", entries)
			}
			if _, ok := tools.Ready(); ok {
				t.Error("refused tools count as ready")
			}
		})
	}
}

// A shared Linux build: the top folder left out, the libraries and the
// links to them kept, the rest left out.
func TestEnsureUnpacksTarXZWithLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need a privilege on Windows")
	}
	archive := tarXZOf(t, []entry{
		{name: "ffmpeg-n9/bin/ffmpeg", content: "ffmpeg"},
		{name: "ffmpeg-n9/bin/ffprobe", content: "ffprobe"},
		{name: "ffmpeg-n9/bin/ffplay", content: "left out"},
		{name: "ffmpeg-n9/lib/libavcodec.so.62.1", content: "library"},
		{name: "ffmpeg-n9/lib/libavcodec.so.62", link: "libavcodec.so.62.1"},
		{name: "ffmpeg-n9/doc/manual.html", content: "left out"},
	})
	server := newFake(t, map[string][]byte{"/ffmpeg.tar.xz": archive})
	file := pin(server.URL+"/ffmpeg.tar.xz", archive, "tar.xz")
	file.Strip, file.Keep = 1, []string{"bin/ffmpeg", "bin/ffprobe", "lib/"}
	dir := t.TempDir()
	tools := Tools{Dir: dir, Client: server.Client(),
		Manifest: Manifest{Version: "9.0.2", FFmpeg: "bin/ffmpeg", FFprobe: "bin/ffprobe", Files: []File{file}}}
	paths, err := tools.Ensure(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "ffmpeg-9.0.2")
	if read(t, paths.FFprobe) != "ffprobe" || read(t, filepath.Join(root, "lib", "libavcodec.so.62")) != "library" {
		t.Error("the programs or the linked library are not unpacked")
	}
	for _, out := range []string{"bin/ffplay", "doc/manual.html"} {
		if _, err := os.Stat(filepath.Join(root, out)); !os.IsNotExist(err) {
			t.Errorf("%s was unpacked", out)
		}
	}
}

// A member whose path leaves the folder is refused.
func TestEnsureRefusesAMemberLeavingTheFolder(t *testing.T) {
	archive := tarXZOf(t, []entry{{name: "top/bin/ffmpeg", content: "x"}, {name: "top/lib/evil", link: "../../../outside"}})
	server := newFake(t, map[string][]byte{"/a.tar.xz": archive})
	file := pin(server.URL+"/a.tar.xz", archive, "tar.xz")
	file.Strip = 1
	tools := Tools{Dir: t.TempDir(), Client: server.Client(),
		Manifest: Manifest{Version: "9", FFmpeg: "bin/ffmpeg", FFprobe: "bin/ffmpeg", Files: []File{file}}}
	if _, err := tools.Ensure(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "leaves") {
		t.Fatalf("Ensure = %v", err)
	}
	if _, _, err := target(File{}, "../../etc/passwd", t.TempDir()); err != nil {
		t.Fatalf("a cleaned path was refused: %v", err)
	}
}

// Every system the app is built for has its pinned downloads.
func TestEverySystemIsPinned(t *testing.T) {
	for _, system := range []string{"linux/amd64", "linux/arm64", "windows/amd64", "windows/arm64", "darwin/amd64", "darwin/arm64"} {
		goos, goarch, _ := strings.Cut(system, "/")
		m, ok := Pinned(goos, goarch)
		if !ok {
			t.Errorf("%s has no pinned tools", system)
			continue
		}
		if err := m.validate(); err != nil {
			t.Errorf("%s: %v", system, err)
		}
		for _, f := range m.Files {
			if !strings.HasPrefix(f.URL, "https://") || f.Size <= 0 {
				t.Errorf("%s: %+v", system, f)
			}
		}
	}
	manifest := filepath.Join(t.TempDir(), "tools.json")
	if err := os.WriteFile(manifest, []byte(`{"version":"9.0.2","ffmpeg":"bin/ffmpeg","ffprobe":"bin/ffprobe",
		"files":[{"url":"http://127.0.0.1:1/a.zip","sha256":"`+strings.Repeat("a", 64)+`","size":3,"format":"zip","into":"bin"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := Load(manifest)
	if err != nil || m.Files[0].Into != "bin" {
		t.Fatalf("Load = %+v, %v", m, err)
	}
}
