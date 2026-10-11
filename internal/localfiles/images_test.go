package localfiles

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/secrets"
)

// unpackImage writes the test image name, testdata/<name>.iso.gz, at
// path: dvd, the test DVD's image, or bluray, the test Blu-ray disc's.
func unpackImage(t *testing.T, name, path string) {
	t.Helper()
	archive, err := os.Open(filepath.Join("testdata", name+".iso.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	unzipped, err := gzip.NewReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(unzipped)
	if err != nil {
		t.Fatal(err)
	}
	write(t, path, 0)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// udfImage writes at path an image with a UDF volume and no ISO 9660 file
// system, as Blu-ray images often are.
func udfImage(t *testing.T, path string) {
	t.Helper()
	data := make([]byte, 20*isoSector)
	for i, id := range []string{"BEA01", "NSR03", "TEA01"} {
		copy(data[(16+i)*isoSector+1:], id)
	}
	write(t, path, 0)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// An image is a DVD's when its ISO 9660 file system holds VIDEO_TS, its
// main title read from its IFO files; a Blu-ray disc's when it holds BDMV,
// its main title its longest playlist; a UDF-only image is a Blu-ray
// disc's when FFmpeg reads it so, libbluray choosing its main title.
func TestReadImage(t *testing.T) {
	dir := t.TempDir()
	unpackImage(t, "dvd", filepath.Join(dir, "dvd.iso"))
	unpackImage(t, "bluray", filepath.Join(dir, "bluray.iso"))
	udfImage(t, filepath.Join(dir, "udf.iso"))
	write(t, filepath.Join(dir, "zero.iso"), 40*isoSector)
	write(t, filepath.Join(dir, "short.iso"), 10)
	var probed []media.Input
	s := &Service{probe: func(_ context.Context, in media.Input) bool {
		probed = append(probed, in)
		return true
	}}
	for name, want := range map[string]storedImage{
		"dvd.iso":    {kind: imageDVD, title: 2},
		"bluray.iso": {kind: imageBluRay, title: 0},
		"udf.iso":    {kind: imageBluRay, title: -1},
		"zero.iso":   {},
		"short.iso":  {},
	} {
		if kind, title := s.readImage(t.Context(), filepath.Join(dir, name)); kind != want.kind || title != want.title {
			t.Errorf("%s: %q %d", name, kind, title)
		}
	}
	if len(probed) != 1 || !slices.Equal(probed[0].Args(), []string{"-playlist", "-1"}) || probed[0].Path != "bluray:"+filepath.Join(dir, "udf.iso") {
		t.Errorf("probed: %+v", probed)
	}
	// Without FFmpeg to probe it, or when FFmpeg reads no Blu-ray disc,
	// a UDF-only image is no disc's.
	s.probe = nil
	if kind, _ := s.readImage(t.Context(), filepath.Join(dir, "udf.iso")); kind != "" {
		t.Errorf("unprobed: %q", kind)
	}
	s.probe = func(context.Context, media.Input) bool { return false }
	if kind, _ := s.readImage(t.Context(), filepath.Join(dir, "udf.iso")); kind != "" {
		t.Errorf("not a Blu-ray disc's: %q", kind)
	}

	file, _ := os.Open(filepath.Join(dir, "dvd.iso"))
	defer file.Close()
	img, err := openISO(file)
	if err != nil {
		t.Fatal(err)
	}
	folder, err := img.lookup("video_ts")
	entries, _ := img.list(folder)
	var names []string
	for _, e := range entries {
		names = append(names, e.name)
	}
	slices.Sort(names)
	if err != nil || !slices.Equal(names, []string{"VIDEO_TS.IFO", "VTS_01_0.IFO", "VTS_01_1.VOB", "VTS_02_0.IFO", "VTS_02_1.VOB"}) {
		t.Errorf("VIDEO_TS: %v %v", names, err)
	}
	ifo, err := img.lookup("VIDEO_TS", "VTS_02_1.VOB")
	if err != nil || ifo.size != 520192 || ifo.dir {
		t.Errorf("a file: %+v %v", ifo, err)
	}
}

// A disc image in a movies folder is one movie, matched by its file's
// name, or its folder's when the file's is no title's, the image its unit, read from the image itself. What a scan read
// of an image is kept while its size and modification time stay; images
// of no disc are left out, and so are a shows folder's and a share's.
func TestDiscImagesAreMovies(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	unpackImage(t, "bluray", filepath.Join(dir, "The General (1926).iso"))
	unpackImage(t, "dvd", filepath.Join(dir, "Nosferatu (1922)", "disc.ISO"))
	unpackImage(t, "dvd", filepath.Join(dir, "Lost Tapes (2003).iso"))
	write(t, filepath.Join(dir, "Backup (2010).iso"), 40*isoSector)
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Shelf", Path: dir, Kind: KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	want := map[string]string{
		"The General (1926).iso":    "tt0017925",
		"Nosferatu (1922)/disc.ISO": "tt0013442",
		"Lost Tapes (2003).iso":     "unmatched: not_found",
	}
	if got := e.files(addon.ID); !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	reads := e.reads(addon.ID)
	if got := reads["The General (1926).iso"]; got != (read{unit: "The General (1926).iso", title: "The General", year: 1926}) {
		t.Errorf("read: %+v", got)
	}
	if got := reads["Nosferatu (1922)/disc.ISO"]; got != (read{unit: "Nosferatu (1922)/disc.ISO", title: "Nosferatu", year: 1922}) {
		t.Errorf("read in a folder: %+v", got)
	}
	unmatched, total, err := e.service.Unmatched(t.Context(), addon.ID)
	discs := map[string]string{}
	for _, u := range unmatched {
		discs[u.Unit] = u.Disc
	}
	if err != nil || total != 1 || !maps.Equal(discs, map[string]string{"Lost Tapes (2003).iso": DiscDVDImage}) {
		t.Errorf("unmatched: %d %+v %v", total, unmatched, err)
	}
	streams, err := e.service.Streams(t.Context(), addon.ID, "tt0017925")
	if err != nil || len(streams) != 1 || streams[0].BehaviorHints.Filename != "The General (1926).iso" ||
		!strings.HasPrefix(streams[0].URL, "http://inputs.example/") {
		t.Fatalf("streams: %+v %v", streams, err)
	}
	key := strings.TrimPrefix(streams[0].URL, "http://inputs.example/")
	real, _ := filepath.EvalSymlinks(filepath.Join(dir, "The General (1926).iso"))
	if in, err := e.service.Input(t.Context(), key); err != nil || !slices.Equal(in.Args(), []string{"-playlist", "0"}) || in.Path != "bluray:"+real {
		t.Errorf("the Blu-ray image's input: %+v %v", in, err)
	}
	if err := e.service.Link(t.Context(), addon.ID, "Lost Tapes (2003).iso", "tt0063350"); err != nil {
		t.Fatal(err)
	}
	streams, _ = e.service.Streams(t.Context(), addon.ID, "tt0063350")
	real, _ = filepath.EvalSymlinks(filepath.Join(dir, "Lost Tapes (2003).iso"))
	if len(streams) != 1 {
		t.Fatalf("the DVD image's streams: %+v", streams)
	}
	if in, err := e.service.Input(t.Context(), strings.TrimPrefix(streams[0].URL, "http://inputs.example/")); err != nil ||
		!slices.Equal(in.Args(), []string{"-f", "dvdvideo", "-title", "2"}) || in.Path != real {
		t.Errorf("the DVD image's input: %+v %v", in, err)
	}

	// What was read is kept until the image changes.
	if _, err := e.pool.Exec(t.Context(), "UPDATE local_disc_images SET title = 7 WHERE addon_id = $1 AND path = $2", addon.ID, "The General (1926).iso"); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if in, _ := e.service.Input(t.Context(), key); !slices.Equal(in.Args(), []string{"-playlist", "7"}) {
		t.Errorf("read again unchanged: %v", in.Args())
	}
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(filepath.Join(dir, "The General (1926).iso"), later, later); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	streams, _ = e.service.Streams(t.Context(), addon.ID, "tt0017925")
	if in, _ := e.service.Input(t.Context(), strings.TrimPrefix(streams[0].URL, "http://inputs.example/")); !slices.Equal(in.Args(), []string{"-playlist", "0"}) {
		t.Errorf("not read again once changed: %v", in.Args())
	}
	var kept []string
	rows, _ := e.pool.Query(t.Context(), "SELECT path || ' ' || kind FROM local_disc_images WHERE addon_id = $1 ORDER BY path", addon.ID)
	for rows.Next() {
		var row string
		_ = rows.Scan(&row)
		kept = append(kept, row)
	}
	if !slices.Equal(kept, []string{"Backup (2010).iso ", "Lost Tapes (2003).iso dvd", "Nosferatu (1922)/disc.ISO dvd", "The General (1926).iso bluray"}) {
		t.Errorf("kept: %q", kept)
	}
	if err := os.Remove(filepath.Join(dir, "Backup (2010).iso")); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := e.pool.QueryRow(t.Context(), "SELECT count(*) FROM local_disc_images WHERE addon_id = $1", addon.ID).Scan(&count); err != nil || count != 3 {
		t.Errorf("an image gone is forgotten: %d %v", count, err)
	}

	shows := t.TempDir()
	unpackImage(t, "dvd", filepath.Join(shows, "The Lone Ranger", "Season 01", "The Lone Ranger - S01E01.iso"))
	write(t, filepath.Join(shows, "The Lone Ranger", "Season 01", "The Lone Ranger - S01E02.mkv"), 10)
	showsFolder, err := e.service.Add(t.Context(), NewFolder{Name: "Shows", Path: shows, Kind: KindShows})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(showsFolder.ID)
	if got := e.files(showsFolder.ID); !maps.Equal(got, map[string]string{"The Lone Ranger/Season 01/The Lone Ranger - S01E02.mkv": "tt0041038"}) {
		t.Errorf("shows: %v", got)
	}

	box, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e.service.box = box
	shared := t.TempDir()
	unpackImage(t, "bluray", filepath.Join(shared, "The General (1926).iso"))
	write(t, filepath.Join(shared, "Nosferatu (1922).mkv"), 10)
	server := serveWebDAV(t, shared, "reader", "pass phrase")
	share, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: server.URL + "/", Kind: KindMovies, User: "reader",
		Password: "pass phrase"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(share.ID)
	if got := e.files(share.ID); !maps.Equal(got, map[string]string{"Nosferatu (1922).mkv": "tt0013442"}) {
		t.Errorf("a share: %v", got)
	}
}
