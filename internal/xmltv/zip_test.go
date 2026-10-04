package xmltv

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

type zipFile struct {
	name string
	data string
}

func zipped(t *testing.T, files ...zipFile) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range files {
		w, err := writer.Create(file.name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(w, file.data)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// spool returns a folder for ZIP archives that must be empty at the end
// of the test.
func spool(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(func() {
		if left, _ := os.ReadDir(dir); len(left) > 0 {
			t.Errorf("left in the spool folder: %v", left)
		}
	})
	return dir
}

// A guide in a ZIP archive is read like a plain one, from its .xml file,
// the largest when it holds several.
func TestGuidesReadFromZIPArchives(t *testing.T) {
	dir := spool(t)
	single := zipped(t, zipFile{"guide.xml", guide})
	channels, programmes, err := read(t, single, Options{Language: "fr", SpoolDir: dir})
	if err != nil || len(channels) != 1 || len(programmes) != 2 || programmes[0].Title != "Le Journal" {
		t.Fatalf("one XML file: %v %+v %+v", err, channels, programmes)
	}

	small := `<tv><channel id="tiny.zz"><display-name>Tiny</display-name></channel></tv>`
	several := zipped(t,
		zipFile{"readme.txt", strings.Repeat("Not a guide. ", 1000)},
		zipFile{"small.xml", small},
		zipFile{"folder/GUIDE.XML", guide},
		zipFile{"other.xml", small})
	channels, _, err = read(t, several, Options{SpoolDir: dir})
	if err != nil || len(channels) != 1 || channels[0].ID != "one.fr" {
		t.Errorf("several files, the largest XML one wins: %v %+v", err, channels)
	}
}

func TestBrokenZIPArchivesFail(t *testing.T) {
	dir := spool(t)
	valid := zipped(t, zipFile{"guide.xml", guide})
	for name, data := range map[string][]byte{
		"no XML file":  zipped(t, zipFile{"guide.txt", guide}, zipFile{"logo.png", "png"}),
		"empty":        zipped(t),
		"truncated":    valid[:len(valid)/2],
		"header only":  valid[:4],
		"not XML file": zipped(t, zipFile{"guide.xml", "<html>Sign in</html>"}),
	} {
		if _, _, err := read(t, data, Options{SpoolDir: dir}); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: %v, want ErrMalformed", name, err)
		}
	}
	// Without a folder to write it to, an archive is not read.
	if _, _, err := read(t, valid, Options{}); !errors.Is(err, ErrMalformed) {
		t.Errorf("without a spool folder: %v", err)
	}
}

// An archive is bound by the download limit, and its XML file by four
// times as much.
func TestZIPArchivesOverTheLimitFail(t *testing.T) {
	dir := spool(t)
	valid := zipped(t, zipFile{"guide.xml", guide})
	if _, _, err := read(t, valid, Options{Limit: int64(len(valid)), SpoolDir: dir}); err != nil {
		t.Errorf("an archive of exactly the limit: %v", err)
	}
	if _, _, err := read(t, valid, Options{Limit: int64(len(valid)) - 1, SpoolDir: dir}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("an archive one byte over the limit: %v", err)
	}
	padded := zipped(t, zipFile{"guide.xml", "<tv>" + strings.Repeat("<!-- padding -->", 10000) + "</tv>"})
	if _, _, err := read(t, padded, Options{Limit: int64(len(padded)), SpoolDir: dir}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("an XML file expanding past four times the archive: %v", err)
	}
}

// failing gives data, then fails as a download cut short or cancelled.
type failing struct {
	data []byte
	err  error
}

func (f *failing) Read(p []byte) (int, error) {
	if len(f.data) == 0 {
		return 0, f.err
	}
	n := copy(p, f.data)
	f.data = f.data[n:]
	return n, nil
}

// The archive written to the spool folder is removed however reading
// ends: cut short, stopped by a callback, or done.
func TestZIPArchivesAreRemovedAfterReading(t *testing.T) {
	dir := spool(t)
	valid := zipped(t, zipFile{"guide.xml", guide})
	cut := errors.New("connection reset")
	if err := Read(&failing{data: valid[:len(valid)/2], err: cut}, Options{SpoolDir: dir},
		func(Channel) error { return nil }, func(Programme) error { return nil }); !errors.Is(err, cut) {
		t.Errorf("download cut short: %v", err)
	}
	stop := errors.New("stop")
	if err := Read(bytes.NewReader(valid), Options{SpoolDir: dir},
		func(Channel) error { return stop }, func(Programme) error { return nil }); !errors.Is(err, stop) {
		t.Errorf("stopped by a callback: %v", err)
	}
	if _, _, err := read(t, valid, Options{SpoolDir: dir}); err != nil {
		t.Error(err)
	}
}
