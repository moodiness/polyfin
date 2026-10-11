package localfiles

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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

	"github.com/moodiness/polyfin/internal/secrets"
)

// unpackBluRay unpacks the test disc, testdata/bluray.tar.gz, into the
// disc folder dir. Its main playlist, 00000, plays clips 00000 and 00001,
// 96 and 94 seconds long; playlist 00001 plays clip 00005, 30 seconds long.
func unpackBluRay(t *testing.T, dir string) {
	t.Helper()
	archive, err := os.Open(filepath.Join("testdata", "bluray.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	unzipped, err := gzip.NewReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	files := tar.NewReader(unzipped)
	for {
		header, err := files.Next()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		target := filepath.Join(dir, filepath.FromSlash(header.Name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(files)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// mplsItem is a play item writeMPLS writes: its clip and how long it plays.
type mplsItem struct {
	clip    string
	seconds int
}

// writeMPLS writes at path a playlist of items, each played from the
// clip's 600th second, as Blu-ray clips start.
func writeMPLS(t *testing.T, path string, items ...mplsItem) {
	t.Helper()
	data := []byte("MPLS0200")
	data = binary.BigEndian.AppendUint32(data, 20)
	data = append(data, make([]byte, 8)...)
	data = binary.BigEndian.AppendUint32(data, 0)
	data = binary.BigEndian.AppendUint16(data, 0)
	data = binary.BigEndian.AppendUint16(data, uint16(len(items)))
	data = binary.BigEndian.AppendUint16(data, 0)
	for _, item := range items {
		data = binary.BigEndian.AppendUint16(data, 20)
		data = append(data, item.clip+"M2TS"...)
		data = append(data, 0, 1, 0)
		data = binary.BigEndian.AppendUint32(data, 600*45000)
		data = binary.BigEndian.AppendUint32(data, uint32(600+item.seconds)*45000)
	}
	write(t, path, 0)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A playlist file tells the clips it plays, in order, and for how long.
func TestReadPlaylist(t *testing.T) {
	dir := t.TempDir()
	unpackBluRay(t, dir)
	for name, want := range map[string]playlist{
		// tsMuxeR splits the main title on keyframes.
		"00000.mpls": {clips: []clipPart{{clip: "00000", length: 96 * time.Second}, {clip: "00001", length: 94 * time.Second}}, length: 190 * time.Second},
		"00001.mpls": {clips: []clipPart{{clip: "00005", length: 30 * time.Second}}, length: 30 * time.Second},
	} {
		file, err := os.Open(filepath.Join(dir, "BDMV", "PLAYLIST", name))
		if err != nil {
			t.Fatal(err)
		}
		got, err := readPlaylist(file)
		file.Close()
		// tsMuxeR ends a clip on its last frame's end, within a frame.
		if err != nil || len(got.clips) != len(want.clips) || (got.length-want.length).Abs() > 100*time.Millisecond {
			t.Errorf("%s: %+v %v", name, got, err)
			continue
		}
		for i, part := range got.clips {
			if part.clip != want.clips[i].clip || (part.length-want.clips[i].length).Abs() > 100*time.Millisecond {
				t.Errorf("%s: clip %d: %+v", name, i, part)
			}
		}
	}
	writeMPLS(t, filepath.Join(dir, "written.mpls"), mplsItem{"00007", 300}, mplsItem{"00008", 60})
	file, _ := os.Open(filepath.Join(dir, "written.mpls"))
	got, err := readPlaylist(file)
	file.Close()
	if err != nil || got.length != 6*time.Minute || len(got.clips) != 2 || got.clips[1] != (clipPart{clip: "00008", in: 600 * 45000, out: 660 * 45000, length: time.Minute}) {
		t.Errorf("written: %+v %v", got, err)
	}
	for _, bad := range []string{"", "MPLS0200", "INDX0200" + strings.Repeat("\x00", 40)} {
		if _, err := readPlaylist(strings.NewReader(bad)); err == nil {
			t.Errorf("%q was read", bad)
		}
	}
	if disc, number, ok := discPlaylist("Box/DISC02/bdmv/playlist/00042.MPLS"); !ok || disc != "Box/DISC02" || number != 42 {
		t.Errorf("a playlist's path: %q %d %v", disc, number, ok)
	}
	for _, rel := range []string{"BDMV/BACKUP/PLAYLIST/00000.mpls", "BDMV/PLAYLIST/menu.mpls", "BDMV/STREAM/00000.m2ts"} {
		if _, _, ok := discPlaylist(rel); ok {
			t.Errorf("%s is read as a playlist", rel)
		}
	}
}

// A disc's titles are its longest playlist, then the others long enough
// to be movies that play mostly clips of their own; another cut of a movie
// counts once, and a disc of decoy playlists keeps its main title.
func TestDiscTitles(t *testing.T) {
	list := func(number int, parts ...clipPart) playlist {
		p := playlist{number: number, clips: parts}
		for _, part := range parts {
			p.length += part.length
		}
		return p
	}
	minutes := func(clip string, n int) clipPart { return clipPart{clip: clip, length: time.Duration(n) * time.Minute} }
	numbers := func(titles []playlist) []int {
		var got []int
		for _, title := range titles {
			got = append(got, title.number)
		}
		return got
	}
	for _, tc := range []struct {
		name      string
		playlists []playlist
		want      []int
	}{
		{"none", nil, nil},
		{"one", []playlist{list(3, minutes("00001", 5))}, []int{3}},
		{"the first of the longest", []playlist{list(9, minutes("00001", 90)), list(4, minutes("00002", 90))}, []int{4, 9}},
		{"a box set disc", []playlist{
			list(1, minutes("00001", 50), minutes("00002", 45)),
			list(2, minutes("00010", 88)),
			list(3, minutes("00020", 21)),
			list(4, minutes("00030", 19)),
		}, []int{1, 2, 3}},
		{"another cut and a language", []playlist{
			list(800, minutes("00001", 40), minutes("00002", 40), minutes("00003", 10)),
			list(801, minutes("00001", 40), minutes("00002", 40)),
			list(802, minutes("00001", 40), minutes("00004", 2), minutes("00002", 40), minutes("00003", 10)),
		}, []int{802}},
		{"half new", []playlist{list(1, minutes("00001", 60)), list(2, minutes("00001", 30), minutes("00002", 30))}, []int{1}},
	} {
		if got := numbers(discTitles(tc.playlists)); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
	// Hundreds of playlists as long as the movie, its clips in another
	// order, count once; so many movies of their own make decoys.
	var decoys []playlist
	for i := range 300 {
		parts := []clipPart{minutes("00001", 50), minutes("00002", 40), minutes("00003", 30)}
		decoys = append(decoys, list(i, append(parts[i%3:], parts[:i%3]...)...))
	}
	if got := numbers(discTitles(decoys)); !slices.Equal(got, []int{0}) {
		t.Errorf("decoys reusing the movie's clips: %v", got)
	}
	decoys = decoys[:0]
	for i := range 40 {
		decoys = append(decoys, list(i, minutes(strings.Repeat("0", 3)+string(rune('A'+i%26))+string(rune('a'+i/26)), 100)))
	}
	if got := numbers(discTitles(decoys)); !slices.Equal(got, []int{0}) {
		t.Errorf("decoys of their own clips: %v", got)
	}
}

// A Blu-ray disc in a folder in the container is its titles: the main
// playlist, matched by the disc folder's name, with the disc folder as
// its unit, and each other movie, a unit of its own linked by hand. Each
// plays its playlist through the bluray protocol, sized as the clips it
// plays. A disc whose playlists cannot be read, or are too short for
// FFmpeg, plays its largest stream.
func TestDiscPlaylistsAreTitles(t *testing.T) {
	defer func(old time.Duration) { minMovie = old }(minMovie)
	minMovie = 20 * time.Second
	e := newEnv(t)
	dir := t.TempDir()
	unpackBluRay(t, filepath.Join(dir, "The General (1926)"))
	unpackBluRay(t, filepath.Join(dir, "Box", "DISC02"))
	// A playlist playing a clip the disc lacks is left out.
	writeMPLS(t, filepath.Join(dir, "Box", "DISC02", "BDMV", "PLAYLIST", "00009.mpls"), mplsItem{"00042", 600})
	// Playlists that cannot be read leave the disc's largest stream.
	write(t, filepath.Join(dir, "Nosferatu (1922)", "BDMV", "PLAYLIST", "00000.mpls"), 30)
	write(t, filepath.Join(dir, "Nosferatu (1922)", "BDMV", "STREAM", "00000.m2ts"), 30)
	write(t, filepath.Join(dir, "Nosferatu (1922)", "BDMV", "STREAM", "00001.m2ts"), 40)
	// So does a disc whose playlists FFmpeg would not play, shorter than 3
	// minutes.
	writeMPLS(t, filepath.Join(dir, "Short (1990)", "BDMV", "PLAYLIST", "00000.mpls"), mplsItem{"00000", 170})
	write(t, filepath.Join(dir, "Short (1990)", "BDMV", "STREAM", "00000.m2ts"), 50)
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Shelf", Path: dir, Kind: KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	want := map[string]string{
		"The General (1926)/BDMV/PLAYLIST/00000.mpls": "tt0017925",
		"The General (1926)/BDMV/PLAYLIST/00001.mpls": "unmatched: disc_title",
		"Box/DISC02/BDMV/PLAYLIST/00000.mpls":         "unmatched: not_found",
		"Box/DISC02/BDMV/PLAYLIST/00001.mpls":         "unmatched: disc_title",
		"Nosferatu (1922)/BDMV/STREAM/00001.m2ts":     "tt0013442",
		"Short (1990)/BDMV/STREAM/00000.m2ts":         "unmatched: not_found",
	}
	if got := e.files(addon.ID); !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	reads := e.reads(addon.ID)
	for rel, r := range map[string]read{
		"The General (1926)/BDMV/PLAYLIST/00000.mpls": {unit: "The General (1926)", title: "The General", year: 1926},
		"The General (1926)/BDMV/PLAYLIST/00001.mpls": {unit: "The General (1926)#00001"},
		"Box/DISC02/BDMV/PLAYLIST/00001.mpls":         {unit: "Box/DISC02#00001"},
	} {
		if reads[rel] != r {
			t.Errorf("%s: %+v", rel, reads[rel])
		}
	}
	unmatched, total, err := e.service.Unmatched(t.Context(), addon.ID)
	discs := map[string]string{}
	for _, u := range unmatched {
		discs[u.Unit] = u.Disc
	}
	if err != nil || total != 4 || !maps.Equal(discs, map[string]string{"Box/DISC02": DiscBluRay, "Box/DISC02#00001": DiscBluRay,
		"The General (1926)#00001": DiscBluRay, "Short (1990)": DiscBluRay}) {
		t.Errorf("unmatched: %d %+v %v", total, unmatched, err)
	}

	// The main title is sized as both its clips, named after the disc,
	// and read from its input.
	streams, err := e.service.Streams(t.Context(), addon.ID, "tt0017925")
	if err != nil || len(streams) != 1 || streams[0].BehaviorHints.Filename != "The General (1926).m2ts" ||
		streams[0].BehaviorHints.VideoSize != 2893824+2838528 || !strings.HasPrefix(streams[0].URL, "http://inputs.example/") {
		t.Fatalf("streams: %+v %v", streams, err)
	}
	in, err := e.service.Input(t.Context(), strings.TrimPrefix(streams[0].URL, "http://inputs.example/"))
	real, _ := filepath.EvalSymlinks(filepath.Join(dir, "The General (1926)"))
	if err != nil || !slices.Equal(in.Args(), []string{"-playlist", "0"}) || in.Path != "bluray:"+real {
		t.Errorf("input: %+v %v", in, err)
	}
	streams, _ = e.service.Streams(t.Context(), addon.ID, "tt0013442")
	if len(streams) != 1 || !strings.HasPrefix(streams[0].URL, "http://files.example/") || streams[0].BehaviorHints.Filename != "Nosferatu (1922).m2ts" {
		t.Errorf("a disc's largest stream: %+v", streams)
	}

	// The other title is linked by hand, as a unit of its own, and plays
	// its own playlist under its number.
	if err := e.service.Link(t.Context(), addon.ID, "Box/DISC02/BDMV/PLAYLIST/00001.mpls", "tt0063350"); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	files := e.files(addon.ID)
	if files["Box/DISC02/BDMV/PLAYLIST/00001.mpls"] != "tt0063350" || files["Box/DISC02/BDMV/PLAYLIST/00000.mpls"] != "unmatched: not_found" {
		t.Errorf("linked: %v", files)
	}
	folder, _ := e.service.Folder(t.Context(), addon.ID)
	if !slices.Equal(folder.Links, []Link{{Unit: "Box/DISC02#00001", IMDb: "tt0063350"}}) {
		t.Errorf("links: %+v", folder.Links)
	}
	streams, err = e.service.Streams(t.Context(), addon.ID, "tt0063350")
	if err != nil || len(streams) != 1 || streams[0].BehaviorHints.Filename != "Box - DISC02 - title 1.m2ts" || streams[0].BehaviorHints.VideoSize != 909312 {
		t.Fatalf("the other title's stream: %+v %v", streams, err)
	}
	in, err = e.service.Input(t.Context(), strings.TrimPrefix(streams[0].URL, "http://inputs.example/"))
	if err != nil || !slices.Equal(in.Args(), []string{"-playlist", "1"}) {
		t.Errorf("the other title's input: %+v %v", in, err)
	}

	// With the threshold of movies, the short playlist is no title.
	minMovie = 20 * time.Minute
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.files(addon.ID)["The General (1926)/BDMV/PLAYLIST/00001.mpls"]; ok {
		t.Error("an extra is a title")
	}
}

// A disc a 0.30 scan read as its largest stream is read as its main
// playlist at the next scan, keeping its link; a share's discs keep their
// largest stream.
func TestDiscStreamsBecomePlaylists(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	disc := filepath.Join(dir, "Box", "DISC01")
	unpackBluRay(t, disc)
	playlists := filepath.Join(dir, "playlists")
	if err := os.Rename(filepath.Join(disc, "BDMV", "PLAYLIST"), playlists); err != nil {
		t.Fatal(err)
	}
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Shelf", Path: dir, Kind: KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	if got := e.files(addon.ID); !maps.Equal(got, map[string]string{"Box/DISC01/BDMV/STREAM/00000.m2ts": "unmatched: not_found"}) {
		t.Fatalf("before: %v", got)
	}
	if err := e.service.Link(t.Context(), addon.ID, "Box/DISC01/BDMV/STREAM/00000.m2ts", "tt0017925"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(playlists, filepath.Join(disc, "BDMV", "PLAYLIST")); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.files(addon.ID); !maps.Equal(got, map[string]string{"Box/DISC01/BDMV/PLAYLIST/00000.mpls": "tt0017925"}) {
		t.Errorf("after: %v", got)
	}
	folder, _ := e.service.Folder(t.Context(), addon.ID)
	if !slices.Equal(folder.Links, []Link{{Unit: "Box/DISC01", IMDb: "tt0017925"}}) {
		t.Errorf("links: %+v", folder.Links)
	}

	shared := t.TempDir()
	unpackBluRay(t, filepath.Join(shared, "The General (1926)"))
	server := serveWebDAV(t, shared, "reader", "pass phrase")
	box, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e.service.box = box
	share, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: server.URL + "/", Kind: KindMovies, User: "reader",
		Password: "pass phrase"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(share.ID)
	if got := e.files(share.ID); !maps.Equal(got, map[string]string{"The General (1926)/BDMV/STREAM/00000.m2ts": "tt0017925"}) {
		t.Errorf("a share: %v", got)
	}
}
