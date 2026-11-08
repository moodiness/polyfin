package ocr

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// dataServer serves files by name, as Tesseract's data is served, and
// lists them for a Downloader with the SHA-256 of listed, by language.
func dataServer(t *testing.T, served map[string][]byte, listed map[string][]byte) (*httptest.Server, map[string]DataFile) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := served[filepath.Base(r.URL.Path)]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(server.Close)
	files := map[string]DataFile{}
	for language, data := range listed {
		sum := sha256.Sum256(data)
		files[language] = DataFile{Language: language, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
	}
	return server, files
}

func TestDownloadKeepsTheCheckedFile(t *testing.T) {
	t.Parallel()
	data := []byte("Finnish model")
	server, files := dataServer(t, map[string][]byte{"fin.traineddata": data}, map[string][]byte{"fin": data})
	// The folder is created when missing.
	dir := filepath.Join(t.TempDir(), "tessdata")
	d := &Downloader{Dir: dir, URL: server.URL + "/data/", Files: files}
	if err := d.Download(t.Context(), "fin"); err != nil {
		t.Fatal(err)
	}
	kept, err := os.ReadFile(filepath.Join(dir, "fin.traineddata"))
	if err != nil || string(kept) != string(data) {
		t.Fatalf("kept %q, %v", kept, err)
	}
	// Tesseract reads it, and nothing else is left in the folder.
	if languages := (&Tesseract{DataDir: dir}).dirLanguages(); !slices.Equal(languages, []string{"fin"}) {
		t.Errorf("languages of the folder: %v", languages)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("the folder holds %d files", len(entries))
	}
}

func TestDownloadRefusesAFileOfAnotherChecksum(t *testing.T) {
	t.Parallel()
	server, files := dataServer(t, map[string][]byte{"fin.traineddata": []byte("Finnish model, altered"), "est.traineddata": []byte("Estonian")},
		map[string][]byte{"fin": []byte("Finnish model"), "est": []byte("Estonien")})
	dir := t.TempDir()
	d := &Downloader{Dir: dir, URL: server.URL, Files: files}
	// Longer than listed, and of the listed length with other bytes.
	for _, language := range []string{"fin", "est"} {
		if err := d.Download(t.Context(), language); !errors.Is(err, ErrChecksum) {
			t.Errorf("%s: %v", language, err)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("kept %v", entries)
	}
}

func TestDownloadNamesWhatFailed(t *testing.T) {
	t.Parallel()
	data := []byte("Finnish model")
	server, files := dataServer(t, map[string][]byte{}, map[string][]byte{"fin": data})
	d := &Downloader{Dir: t.TempDir(), URL: server.URL, Files: files}
	var fetch *FetchError
	if err := d.Download(t.Context(), "fin"); !errors.As(err, &fetch) {
		t.Errorf("a file not found: %v", err)
	}
	if err := d.Download(t.Context(), "xyz"); !errors.Is(err, ErrUnknownLanguage) {
		t.Errorf("a language not listed: %v", err)
	}
	if err := (&Downloader{URL: server.URL, Files: files}).Download(t.Context(), "fin"); !errors.Is(err, ErrNoDataDir) {
		t.Errorf("no folder: %v", err)
	}
	// A path that cannot be a folder, as a file stands there.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var folder *FolderError
	if err := (&Downloader{Dir: filepath.Join(file, "tessdata"), URL: server.URL, Files: files}).Download(t.Context(), "fin"); !errors.As(err, &folder) ||
		folder.Dir != filepath.Join(file, "tessdata") {
		t.Errorf("a folder that cannot be made: %v", err)
	}
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		readOnly := t.TempDir()
		if err := os.Chmod(readOnly, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) })
		if err := (&Downloader{Dir: readOnly, URL: server.URL, Files: files}).Download(t.Context(), "fin"); !errors.As(err, &folder) {
			t.Errorf("a folder that cannot be written: %v", err)
		}
	}
}
