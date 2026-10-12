package localfiles

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
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

// dvdFiles reads the files of the test DVD, by path, those of its
// VIDEO_TS folder.
func dvdFiles(t *testing.T) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join("testdata", "dvd", "VIDEO_TS"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join("testdata", "dvd", "VIDEO_TS", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files["VIDEO_TS/"+e.Name()] = data
	}
	return files
}

// An image is a DVD's when its file system holds VIDEO_TS, its titles
// read from its IFO files; a Blu-ray disc's when it holds BDMV, its
// titles its playlists: through ISO 9660, or UDF when the image has no
// ISO 9660 file system, 2.50 and 2.01 alike. An image whose UDF file
// system is laid out in a way that is not read, a sparable or virtual
// partition or 4096-byte blocks, is a Blu-ray disc's when FFmpeg reads it
// so, libbluray choosing its title, as it was before UDF was read. Other
// images, and a UDF volume without a file system, are no disc's, and are
// not handed to FFmpeg.
func TestReadImage(t *testing.T) {
	defer func(old time.Duration) { minMovie = old }(minMovie)
	dir := t.TempDir()
	unpackImage(t, "dvd", filepath.Join(dir, "dvd.iso"))
	unpackImage(t, "bluray", filepath.Join(dir, "bluray.iso"))
	writeUDF(t, filepath.Join(dir, "udf.iso"), blurayFiles(t), udfLayout{metadata: true})
	writeUDF(t, filepath.Join(dir, "udf201.iso"), blurayFiles(t), udfLayout{})
	writeUDF(t, filepath.Join(dir, "dvd-udf.iso"), dvdFiles(t), udfLayout{metadata: true})
	writeUDF(t, filepath.Join(dir, "sparable.iso"), blurayFiles(t), udfLayout{partition: "*UDF Sparable Partition"})
	writeUDF(t, filepath.Join(dir, "virtual.iso"), blurayFiles(t), udfLayout{partition: "*UDF Virtual Partition"})
	large := buildUDF(blurayFiles(t), udfLayout{})
	for _, at := range []int{34, 50} {
		lvd := large[at*udfBlock:][:udfBlock]
		binary.LittleEndian.PutUint32(lvd[212:], 4096)
		tag(lvd, udfLogical, uint32(at), int(binary.LittleEndian.Uint16(lvd[10:]))+16)
	}
	if err := os.WriteFile(filepath.Join(dir, "4096.iso"), large, 0o644); err != nil {
		t.Fatal(err)
	}
	vrs := make([]byte, 20*isoSector)
	for i, id := range []string{"BEA01", "NSR03", "TEA01"} {
		copy(vrs[(16+i)*isoSector+1:], id)
	}
	if err := os.WriteFile(filepath.Join(dir, "vrs.iso"), vrs, 0o644); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "zero.iso"), 40*isoSector)
	write(t, filepath.Join(dir, "short.iso"), 10)
	var probed []string
	s := &Service{probe: func(_ context.Context, in media.Input) bool {
		if !slices.Equal(in.Args(), []string{"-playlist", "-1"}) {
			t.Errorf("probed with %v", in.Args())
		}
		probed = append(probed, filepath.Base(in.Path))
		return true
	}}
	for _, movie := range []time.Duration{20 * time.Minute, 20 * time.Second} {
		// The disc's 30-second playlist is a movie of its own when movies
		// may be that short.
		minMovie = movie
		var others []int
		if movie < time.Minute {
			others = []int{1}
		}
		for name, want := range map[string]storedImage{
			"dvd.iso":      {kind: imageDVD, title: 2},
			"dvd-udf.iso":  {kind: imageDVD, title: 2},
			"bluray.iso":   {kind: imageBluRay, title: 0, titles: others},
			"udf.iso":      {kind: imageBluRay, title: 0, titles: others},
			"udf201.iso":   {kind: imageBluRay, title: 0, titles: others},
			"sparable.iso": {kind: imageBluRay, title: -1},
			"virtual.iso":  {kind: imageBluRay, title: -1},
			"4096.iso":     {kind: imageBluRay, title: -1},
			"vrs.iso":      {},
			"zero.iso":     {},
			"short.iso":    {},
		} {
			if got := s.readImage(t.Context(), filepath.Join(dir, name), KindMovies); got.kind != want.kind || got.title != want.title ||
				!slices.Equal(got.titles, want.titles) {
				t.Errorf("%s, movies from %v: %q %d %v", name, movie, got.kind, got.title, got.titles)
			}
		}
	}
	slices.Sort(probed)
	if want := []string{"4096.iso", "4096.iso", "sparable.iso", "sparable.iso", "virtual.iso", "virtual.iso"}; !slices.Equal(probed, want) {
		t.Errorf("probed: %v", probed)
	}
	// Without FFmpeg to probe it, or when FFmpeg reads no Blu-ray disc,
	// such an image is no disc's.
	s.probe = nil
	if image := s.readImage(t.Context(), filepath.Join(dir, "sparable.iso"), KindMovies); image.kind != "" {
		t.Errorf("unprobed: %q", image.kind)
	}
	s.probe = func(context.Context, media.Input) bool { return false }
	if image := s.readImage(t.Context(), filepath.Join(dir, "virtual.iso"), KindMovies); image.kind != "" {
		t.Errorf("not a Blu-ray disc's: %q", image.kind)
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
// name, or its folder's when the file's is no title's, the image its
// unit, read from the image itself. What a scan read of an image is kept
// while its size and modification time stay; images of no disc are left
// out, and so are a shows folder's; a share's are read on the share.
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
	if got := e.files(share.ID); !maps.Equal(got, map[string]string{"Nosferatu (1922).mkv": "tt0013442", "The General (1926).iso": "tt0017925"}) {
		t.Errorf("a share: %v", got)
	}
}

// Each other movie of a disc image is a title of its own, kept as the
// image's path with its number: a unit of its own, unmatched until linked
// by hand, named after the image and its number, and played from the
// image. A UDF-only Blu-ray image is read through its UDF file system:
// its titles, and their keyframes. One whose UDF partition is sparable is
// one movie when FFmpeg reads it, libbluray choosing its title, without
// keyframes.
func TestImageMoviesAreTitles(t *testing.T) {
	defer func(old time.Duration) { minMovie = old }(minMovie)
	minMovie = 20 * time.Second
	e := newEnv(t)
	e.service.UseProbe(func(context.Context, media.Input) bool { return true })
	dir := t.TempDir()
	writeUDF(t, filepath.Join(dir, "Nosferatu (1922).iso"), blurayFiles(t), udfLayout{metadata: true})
	writeUDF(t, filepath.Join(dir, "The General (1926).iso"), blurayFiles(t), udfLayout{partition: "*UDF Sparable Partition"})
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Shelf", Path: dir, Kind: KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	want := map[string]string{
		"Nosferatu (1922).iso":   "tt0013442",
		"Nosferatu (1922).iso#1": "unmatched: disc_title",
		"The General (1926).iso": "tt0017925",
	}
	if got := e.files(addon.ID); !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	streams, _ := e.service.Streams(t.Context(), addon.ID, "tt0017925")
	if len(streams) != 1 {
		t.Fatalf("the sparable image's streams: %+v", streams)
	}
	key := strings.TrimPrefix(streams[0].URL, "http://inputs.example/")
	if in, err := e.service.Input(t.Context(), key); err != nil || !slices.Equal(in.Args(), []string{"-playlist", "-1"}) {
		t.Errorf("the sparable image's input: %+v %v", in, err)
	}
	if _, err := e.service.Keyframes(t.Context(), key); !errors.Is(err, errNoKeyframes) {
		t.Errorf("the sparable image's keyframes: %v", err)
	}
	if got := e.reads(addon.ID)["Nosferatu (1922).iso#1"]; got != (read{unit: "Nosferatu (1922).iso#00001"}) {
		t.Errorf("read: %+v", got)
	}
	unmatched, total, err := e.service.Unmatched(t.Context(), addon.ID)
	if err != nil || total != 1 || unmatched[0].Unit != "Nosferatu (1922).iso#00001" || unmatched[0].Disc != DiscBluRayImage {
		t.Errorf("unmatched: %d %+v %v", total, unmatched, err)
	}
	if err := e.service.Link(t.Context(), addon.ID, "Nosferatu (1922).iso#1", "tt0063350"); err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(filepath.Join(dir, "Nosferatu (1922).iso"))
	for id, title := range map[string]struct {
		filename  string
		playlist  string
		keyframes int
	}{
		"tt0013442": {"Nosferatu (1922).iso", "0", 190},
		"tt0063350": {"Nosferatu (1922) - title 1.iso", "1", 30},
	} {
		streams, err := e.service.Streams(t.Context(), addon.ID, id)
		if err != nil || len(streams) != 1 || streams[0].BehaviorHints.Filename != title.filename || !strings.HasPrefix(streams[0].URL, "http://inputs.example/") {
			t.Fatalf("%s: %+v %v", id, streams, err)
		}
		key := strings.TrimPrefix(streams[0].URL, "http://inputs.example/")
		if in, err := e.service.Input(t.Context(), key); err != nil || !slices.Equal(in.Args(), []string{"-playlist", title.playlist}) || in.Path != "bluray:"+real {
			t.Errorf("%s: input %+v %v", id, in, err)
		}
		if got, err := e.service.Keyframes(t.Context(), key); err != nil || len(got) != title.keyframes {
			t.Errorf("%s: %d keyframes, %v", id, len(got), err)
		}
	}
	// What was read is kept: a scan of the image unchanged keeps its
	// titles and links.
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.files(addon.ID)["Nosferatu (1922).iso#1"]; got != "tt0063350" {
		t.Errorf("after a scan: %s", got)
	}
}
