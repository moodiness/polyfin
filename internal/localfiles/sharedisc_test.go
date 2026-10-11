package localfiles

import (
	"bytes"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/secrets"
)

// A play item's bytes run from the aligned unit of the entry point at or
// before its in time to that of the first entry point at or after its out
// time, or the clip's end.
func TestClipRange(t *testing.T) {
	points := []epEntry{{pts: 900, spn: 4}, {pts: 90900, spn: 157}, {pts: 180900, spn: 313}, {pts: 270900, spn: 470}}
	const size = 100 * alignedUnit
	for _, tc := range []struct {
		in, out    uint64
		start, end int64
	}{
		{0, 1 << 40, 0, size},
		{900, 400000, 0, size},
		{90900, 400000, 4 * alignedUnit, size},
		{100000, 180900, 4 * alignedUnit, 9 * alignedUnit},
		{100000, 200000, 4 * alignedUnit, 14 * alignedUnit},
	} {
		if start, end := clipRange(points, tc.in, tc.out, size); start != tc.start || end != tc.end {
			t.Errorf("%d to %d: %d to %d, want %d to %d", tc.in, tc.out, start, end, tc.start, tc.end)
		}
	}
	if start, end := clipRange(nil, 5, 10, 777); start != 0 || end != 777 {
		t.Errorf("no entry point: %d to %d", start, end)
	}
}

// writeFiles writes files, by path with slashes, under dir.
func writeFiles(t *testing.T, dir string, files map[string][]byte) {
	t.Helper()
	for rel, data := range files {
		write(t, filepath.Join(dir, filepath.FromSlash(rel)), 0)
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// A Blu-ray disc on a share, in its folder or in an image, ISO 9660 or
// UDF, has the titles it has on the server's disk, each with its own
// unit. Each plays as one MPEG-TS file the loopback interface serves: its
// clips' bytes from its play items' in times to their out times, which a
// seek reads as well, with keyframes read from the clips' EP maps on the
// share. A DVD on a share is left out.
func TestShareDiscsAreTitles(t *testing.T) {
	defer func(old time.Duration) { minMovie = old }(minMovie)
	minMovie = 20 * time.Second
	e := newEnv(t)
	box, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e.service.box = box
	dir := t.TempDir()
	disc := blurayFiles(t)
	for rel, data := range disc {
		writeFiles(t, dir, map[string][]byte{"movies/The General (1926)/" + rel: data})
	}
	writeUDF(t, filepath.Join(dir, "movies", "Nosferatu (1922).iso"), disc, udfLayout{metadata: true})
	unpackImage(t, "bluray", filepath.Join(dir, "movies", "Night of the Living Dead (1968).iso"))
	unpackImage(t, "dvd", filepath.Join(dir, "movies", "Lost Tapes (2003).iso"))
	copyDVD(t, filepath.Join(dir, "movies", "Metropolis (1927)"), false)
	server := serveWebDAV(t, dir, "reader", "pass phrase")
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: server.URL + "/movies/", Kind: KindMovies, User: "reader",
		Password: "pass phrase"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	want := map[string]string{
		"The General (1926)/BDMV/PLAYLIST/00000.mpls": "tt0017925",
		"The General (1926)/BDMV/PLAYLIST/00001.mpls": "unmatched: disc_title",
		"Nosferatu (1922).iso":                        "tt0013442",
		"Nosferatu (1922).iso#1":                      "unmatched: disc_title",
		"Night of the Living Dead (1968).iso":         "tt0063350",
		"Night of the Living Dead (1968).iso#1":       "unmatched: disc_title",
		"Metropolis (1927)/VIDEO_TS/VTS_02_1.VOB":     "unmatched: not_found",
		"Lost Tapes (2003).iso":                       "unmatched: not_found",
	}
	if got := e.files(addon.ID); !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	reads := e.reads(addon.ID)
	if reads["The General (1926)/BDMV/PLAYLIST/00001.mpls"].unit != "The General (1926)#00001" ||
		reads["Nosferatu (1922).iso#1"].unit != "Nosferatu (1922).iso#00001" {
		t.Errorf("units: %+v", reads)
	}
	main := slices.Concat(disc["BDMV/STREAM/00000.m2ts"], disc["BDMV/STREAM/00001.m2ts"])
	for rel, title := range map[string]struct {
		data      []byte
		keyframes int
	}{
		"The General (1926)/BDMV/PLAYLIST/00000.mpls": {main, 190},
		"The General (1926)/BDMV/PLAYLIST/00001.mpls": {disc["BDMV/STREAM/00005.m2ts"], 30},
		"Nosferatu (1922).iso":                        {main, 190},
		"Nosferatu (1922).iso#1":                      {disc["BDMV/STREAM/00005.m2ts"], 30},
		"Night of the Living Dead (1968).iso":         {main, 190},
	} {
		key := strings.TrimSuffix(e.fileKey(addon.ID, rel), filepath.Ext(rel))
		in, err := e.service.Input(t.Context(), key)
		if err != nil || in.Format != "mpegts" || len(in.Options) != 0 || in.Path != "http://files.example/"+key+".m2ts" {
			t.Errorf("%s: input %+v %v", rel, in, err)
		}
		if got, err := e.service.Keyframes(t.Context(), key); err != nil || len(got) != title.keyframes {
			t.Errorf("%s: %d keyframes, %v", rel, len(got), err)
		}
		file, _, err := e.service.Open(t.Context(), key+".m2ts")
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		if size, err := file.Seek(0, io.SeekEnd); err != nil || size != int64(len(title.data)) {
			t.Errorf("%s: %d bytes, %v", rel, size, err)
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(file)
		if err != nil || !bytes.Equal(got, title.data) {
			t.Errorf("%s: the title's bytes differ: %d bytes, %v", rel, len(got), err)
		}
		// A seek across the clips reads the same bytes.
		middle := int64(len(title.data)/2 - 1000)
		if _, err := file.Seek(middle, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		part := make([]byte, 5000)
		if _, err := io.ReadFull(file, part); err != nil || !bytes.Equal(part, title.data[middle:middle+5000]) {
			t.Errorf("%s: the bytes after a seek differ: %v", rel, err)
		}
		file.Close()
	}

	// A DVD on a share, in its folder or in an image, plays its main
	// title's cells as one MPEG program stream, its tracks' languages read
	// from its IFO files; it has no keyframes.
	vob, err := os.ReadFile(filepath.Join("testdata", "dvd", "VIDEO_TS", "VTS_02_1.VOB"))
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"Metropolis (1927)/VIDEO_TS/VTS_02_1.VOB", "Lost Tapes (2003).iso"} {
		key := strings.TrimSuffix(e.fileKey(addon.ID, rel), filepath.Ext(rel))
		in, err := e.service.Input(t.Context(), key)
		if err != nil || in.Format != "mpeg" || in.Path != "http://files.example/"+key+".vob" ||
			!maps.Equal(in.Languages, map[string]string{"0x80": "en", "0x81": "fr", "0x20": "en"}) {
			t.Errorf("%s: input %+v %v", rel, in, err)
		}
		if _, err := e.service.Keyframes(t.Context(), key); !errors.Is(err, errNoKeyframes) {
			t.Errorf("%s: keyframes %v", rel, err)
		}
		file, _, err := e.service.Open(t.Context(), key+".vob")
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		got, err := io.ReadAll(file)
		file.Close()
		if err != nil || !bytes.Equal(got, vob) {
			t.Errorf("%s: the title's cells: %d bytes of %d, %v", rel, len(got), len(vob), err)
		}
	}
	// A disc whose clips' information cannot be read on the share plays
	// no title, without failing otherwise.
	if err := os.WriteFile(filepath.Join(dir, "movies", "The General (1926)", "BDMV", "CLIPINF", "00005.clpi"), []byte("HDMV0200 broken"),
		0o644); err != nil {
		t.Fatal(err)
	}
	key := strings.TrimSuffix(e.fileKey(addon.ID, "The General (1926)/BDMV/PLAYLIST/00001.mpls"), ".mpls")
	e.service.layouts = nil
	if _, _, err := e.service.Open(t.Context(), key+".m2ts"); !errors.Is(err, errNotClip) {
		t.Errorf("a broken clip: %v", err)
	}
	if _, err := e.service.Keyframes(t.Context(), key); !errors.Is(err, errNotClip) {
		t.Errorf("a broken clip's keyframes: %v", err)
	}
}

// A DVD title whose cells cannot be laid out fails: one without cells, a
// cell ending before it starts, a title the disc does not list, a set
// without VOBs.
func TestBrokenDVDTitlesFail(t *testing.T) {
	locate := func(name string, start, end int64) ([]titlePiece, error) {
		return []titlePiece{{rel: name, at: start, n: end - start}}, nil
	}
	for name, titles := range map[string][]ifoTitle{
		"no cells":         {{set: 1, seconds: 60}},
		"a backwards cell": {{set: 1, seconds: 60, cells: []ifoCell{{first: 9, last: 3, seconds: 60}}}},
	} {
		dir := t.TempDir()
		writeIFOs(t, dir, titles...)
		write(t, filepath.Join(dir, "VTS_01_1.VOB"), 20*ifoSector)
		open := func(name string) (*io.SectionReader, error) {
			data, err := os.ReadFile(filepath.Join(dir, name))
			return io.NewSectionReader(strings.NewReader(string(data)), 0, int64(len(data))), err
		}
		if pieces, _, err := dvdPieces(open, 1, locate); !errors.Is(err, errNotDVD) {
			t.Errorf("%s: %v %v", name, pieces, err)
		}
		if _, _, err := dvdPieces(open, 5, locate); !errors.Is(err, errNotDVD) {
			t.Errorf("%s: a title the disc does not list: %v", name, err)
		}
		os.Remove(filepath.Join(dir, "VTS_01_1.VOB"))
		if _, _, err := dvdPieces(open, 1, locate); !errors.Is(err, errNotDVD) {
			t.Errorf("%s: no VOB: %v", name, err)
		}
	}
	// A title's cells across its set's VOBs, one after the other.
	dir := t.TempDir()
	writeIFOs(t, dir, ifoTitle{set: 1, seconds: 60, cells: []ifoCell{{first: 2, last: 5, seconds: 30}, {first: 8, last: 11, seconds: 30}}})
	write(t, filepath.Join(dir, "VTS_01_1.VOB"), 6*ifoSector)
	write(t, filepath.Join(dir, "VTS_01_2.VOB"), 10*ifoSector)
	open := func(name string) (*io.SectionReader, error) {
		data, err := os.ReadFile(filepath.Join(dir, name))
		return io.NewSectionReader(strings.NewReader(string(data)), 0, int64(len(data))), err
	}
	pieces, _, err := dvdPieces(open, 1, locate)
	want := []titlePiece{{"VTS_01_1.VOB", 2 * ifoSector, 4 * ifoSector}, {"VTS_01_2.VOB", 2 * ifoSector, 4 * ifoSector}}
	if err != nil || !slices.Equal(pieces, want) {
		t.Errorf("pieces: %+v %v", pieces, err)
	}
}
