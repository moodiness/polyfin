package workfolder

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// keep writes a file at path, with its folders.
func keep(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// names lists what dir holds.
func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// kinds are the folders emptied at start, each with names a version
// before the marker wrote there.
var kinds = []struct {
	name    string
	kind    Kind
	earlier []string
}{
	{"segments", Segments, []string{"0123456789abcdef01234567/0.ts", "live-0123456789abcdef01234567/1.ts", "encoding-2147483647/init.mp4",
		"audio-1/0.ts", "burn-2/burn.ass", "live-3/0.ts", "measure-4/hdr.mkv", "tone-mapping-5/sdr.mkv", "joined-6",
		"playout-0190a1b2-c3d4-7e5f-8a6b-0c1d2e3f4a5b/0.ts"}},
	{"torrents", Torrents, []string{"0123456789abcdef0123456789abcdef01234567/0", "89abcdef0123456789abcdef0123456789abcdef/12"}},
	{"guides", Guides, []string{"guide-1234567890.zip", "guide-7.zip"}},
	{"ocr", OCR, []string{"polyfin-ocr-1234567890/1.png", "polyfin-pictures-42.mks"}},
}

// A folder that is not there is made with its marker, an empty one gets
// the marker, and a folder holding the marker is emptied, whatever a run
// left in it: Polyfin made it.
func TestAFolderPolyfinMadeIsEmptied(t *testing.T) {
	t.Parallel()
	for _, k := range kinds {
		t.Run(k.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "cache", k.name)
			if err := k.kind.Empty(dir); err != nil {
				t.Fatal(err)
			}
			if got := names(t, dir); len(got) != 1 || got[0] != Marker {
				t.Fatalf("a new folder holds %v", got)
			}
			keep(t, filepath.Join(dir, "of a run", "file"))
			keep(t, filepath.Join(dir, "notes.txt"))
			if err := k.kind.Check(dir); err != nil {
				t.Errorf("a folder with its marker: %v", err)
			}
			if err := k.kind.Empty(dir); err != nil {
				t.Fatal(err)
			}
			if got := names(t, dir); len(got) != 1 || got[0] != Marker {
				t.Errorf("after the next start it holds %v", got)
			}

			empty := t.TempDir()
			if err := k.kind.Empty(empty); err != nil {
				t.Fatal(err)
			}
			if got := names(t, empty); len(got) != 1 || got[0] != Marker {
				t.Errorf("an empty folder holds %v", got)
			}
		})
	}
}

// A folder that holds files and no marker is not Polyfin's: it is left as
// it is, and so is a file or a link of the folder's name.
func TestAFolderWithOtherFilesIsLeftAsItIs(t *testing.T) {
	t.Parallel()
	for _, k := range kinds {
		t.Run(k.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), k.name)
			kept := filepath.Join(dir, "holidays", "2019.mkv")
			keep(t, kept)
			if err := k.kind.Check(dir); !errors.Is(err, ErrOccupied) {
				t.Errorf("check: %v", err)
			}
			if err := k.kind.Empty(dir); !errors.Is(err, ErrOccupied) {
				t.Errorf("emptying: %v", err)
			}
			if _, err := os.Stat(kept); err != nil {
				t.Errorf("the file is gone: %v", err)
			}
			if got := names(t, dir); len(got) != 1 || got[0] != "holidays" {
				t.Errorf("the folder holds %v", got)
			}

			file := filepath.Join(t.TempDir(), k.name)
			keep(t, file)
			if err := k.kind.Empty(file); !errors.Is(err, ErrOccupied) {
				t.Errorf("a file of that name: %v", err)
			}
			if info, err := os.Lstat(file); err != nil || !info.Mode().IsRegular() {
				t.Errorf("the file of that name: %v", err)
			}

			if runtime.GOOS == "windows" {
				return // Links need a privilege there.
			}
			link := filepath.Join(t.TempDir(), k.name)
			if err := os.Symlink(dir, link); err != nil {
				t.Fatal(err)
			}
			if err := k.kind.Empty(link); !errors.Is(err, ErrOccupied) {
				t.Errorf("a link of that name: %v", err)
			}
			if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Errorf("the link of that name: %v", err)
			}
			if _, err := os.Stat(kept); err != nil {
				t.Errorf("the file behind the link is gone: %v", err)
			}
		})
	}
}

// At the first start after an upgrade, a folder holds no marker yet: one
// that holds nothing but what the versions before wrote there is marked
// and emptied, and one that holds anything else beside is left as it is.
func TestAFolderOfAnEarlierVersionIsMarkedAndEmptied(t *testing.T) {
	t.Parallel()
	for _, k := range kinds {
		t.Run(k.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), k.name)
			for _, name := range k.earlier {
				keep(t, filepath.Join(dir, filepath.FromSlash(name)))
			}
			if err := k.kind.Check(dir); err != nil {
				t.Errorf("check: %v", err)
			}
			if err := k.kind.Empty(dir); err != nil {
				t.Fatal(err)
			}
			if got := names(t, dir); len(got) != 1 || got[0] != Marker {
				t.Errorf("after the upgrade it holds %v", got)
			}

			// The same names, and one file more that no version wrote.
			other := filepath.Join(t.TempDir(), k.name)
			for _, name := range k.earlier {
				keep(t, filepath.Join(other, filepath.FromSlash(name)))
			}
			keep(t, filepath.Join(other, "notes.txt"))
			before := names(t, other)
			if err := k.kind.Empty(other); !errors.Is(err, ErrOccupied) {
				t.Errorf("with another file: %v", err)
			}
			if got := names(t, other); len(got) != len(before) {
				t.Errorf("with another file it holds %v, held %v", got, before)
			}
		})
	}
	for name, kind := range map[string]Kind{"Movie (2008)": Segments, "live-concert.ts": Segments, "encoding-notes": Segments, "0123456789ABCDEF01234567": Segments,
		"ubuntu.iso": Torrents, "0123456789abcdef": Torrents, "guide.zip": Guides, "guide-2019.zip.bak": Guides, "polyfin-ocr": OCR, "scan.png": OCR} {
		if kind.earlier.MatchString(name) {
			t.Errorf("%s is taken for a name of an earlier version", name)
		}
	}
}

// A folder of Polyfin's own data folder is marked whatever it holds, and
// emptied then; nothing is made where there is no folder, and a file of
// the folder's name gives way.
func TestMarkTellsAFolderOfTheDataFolder(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "segments")
	if err := Mark(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Errorf("a folder was made: %v", err)
	}
	keep(t, filepath.Join(dir, "of a version long gone", "file"))
	if err := Segments.Check(dir); !errors.Is(err, ErrOccupied) {
		t.Fatalf("before the mark: %v", err)
	}
	if err := Mark(dir); err != nil {
		t.Fatal(err)
	}
	if err := Segments.Empty(dir); err != nil {
		t.Fatal(err)
	}
	if got := names(t, dir); len(got) != 1 || got[0] != Marker {
		t.Errorf("it holds %v", got)
	}

	file := filepath.Join(t.TempDir(), "segments")
	keep(t, file)
	if err := Mark(file); err != nil {
		t.Fatal(err)
	}
	if err := Segments.Empty(file); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(file); err != nil || !info.IsDir() {
		t.Errorf("in the file's place: %v", err)
	}
}
