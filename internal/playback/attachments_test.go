package playback

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
)

// The files attached to versions are kept within a bound in bytes, beside
// the bound on the versions: the fonts of several versions stay kept
// together, the most one version has is kept alone, and the least
// recently used give way to the next.
func TestAttachedFilesAreKeptWithinABoundInBytes(t *testing.T) {
	t.Parallel()
	s, _, version, analysis := subtitled(t)
	const fontStream = 3
	font, err := s.Attachment(t.Context(), version, analysis, fontStream)
	dummy, _ := os.ReadFile(filepath.Join("..", "container", "testdata", "Dummy.ttf"))
	if err != nil || !bytes.Equal(font, dummy) {
		t.Fatalf("font: %v, %d bytes, want %d", err, len(font), len(dummy))
	}
	// What was read is kept, and counted: its names, types and data.
	files, ok := s.attached.Get(version.ID)
	if !ok || s.attached.Bytes() != attachedSize(files) || s.attached.Bytes() <= len(font) {
		t.Fatalf("kept: %v, counted for %d bytes, the files being of %d and the font of %d", ok, s.attached.Bytes(), attachedSize(files), len(font))
	}
	s.attached.Delete(version.ID)

	// The files below share one buffer, and count for their length each.
	const most = 8
	data := make([]byte, maxKeptAttached/4)
	fonts := func(count, length int) []container.Attachment {
		files := make([]container.Attachment, count)
		for i := range files {
			files[i].Data = data[:length]
		}
		return files
	}
	keep := func(id byte, files []container.Attachment) {
		t.Helper()
		s.attached.Put(accounts.ID{id}, files)
		if s.attached.Bytes() > maxKeptAttached || s.attached.Len() > most {
			t.Fatalf("after version %d: %d bytes of %d versions kept, want at most %d of %d", id, s.attached.Bytes(), s.attached.Len(), maxKeptAttached, most)
		}
	}
	kept := func(id byte) bool {
		_, ok := s.attached.Get(accounts.ID{id})
		return ok
	}

	// Fonts of a few megabytes: as many versions as before stay kept.
	for id := byte(1); id <= most; id++ {
		keep(id, fonts(2, 4<<20))
	}
	if s.attached.Len() != most || s.attached.Bytes() != most*2*(4<<20) {
		t.Errorf("fonts of 8 MiB a version: %d versions kept, %d bytes", s.attached.Len(), s.attached.Bytes())
	}
	// The most one version has is kept, alone.
	keep(9, fonts(4, len(data)))
	if !kept(9) || s.attached.Len() != 1 || s.attached.Bytes() != maxKeptAttached {
		t.Errorf("files of %d bytes: kept %v, %d versions, %d bytes", maxKeptAttached, kept(9), s.attached.Len(), s.attached.Bytes())
	}
	// Versions of a quarter of the bound: four are kept, and a fifth
	// takes the place of the one used longest ago.
	for id := byte(10); id <= 13; id++ {
		keep(id, fonts(1, len(data)))
	}
	if kept(9) || s.attached.Len() != 4 || s.attached.Bytes() != maxKeptAttached {
		t.Errorf("four versions of a quarter: the largest kept %v, %d versions, %d bytes", kept(9), s.attached.Len(), s.attached.Bytes())
	}
	if !kept(10) {
		t.Error("the first of four versions is not kept")
	}
	keep(14, fonts(1, len(data)))
	if !kept(10) || kept(11) || !kept(14) || s.attached.Len() != 4 {
		t.Errorf("a fifth version: kept %v, the first %v as it was used last, the second %v; %d versions",
			kept(14), kept(10), kept(11), s.attached.Len())
	}
	// More than the bound, which the container package does not read: it
	// is not kept, and takes nobody's place.
	keep(15, fonts(5, len(data)))
	if kept(15) || s.attached.Len() != 4 {
		t.Errorf("files past the bound: kept %v, %d versions", kept(15), s.attached.Len())
	}
}
