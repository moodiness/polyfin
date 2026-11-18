package torrent

import (
	"bytes"
	"context"
	"crypto/sha1"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// infoOf is an info dictionary of one piece, with the name given, whose
// files are at the paths given, each a list of parts: the first file is
// of one byte, the others empty.
func infoOf(name string, paths ...[]string) []byte {
	list := make([]any, len(paths))
	for i, parts := range paths {
		length := 0
		if i == 0 {
			length = 1
		}
		list[i] = map[string]any{"length": length, "path": parts}
	}
	sum := sha1.Sum([]byte{0})
	return encode(map[string]any{"name": name, "piece length": 16 << 10, "pieces": string(sum[:]), "files": list})
}

// filesNamed are n files named "f" at a torrent's root.
func filesNamed(n int) [][]string {
	return slices.Repeat([][]string{{"f"}}, n)
}

// values counts the values of what decode read: a list, a dictionary, and
// each value and key they hold.
func values(value any) int {
	count := 1
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			count += values(item)
		}
	case map[string]any:
		for _, item := range v {
			count += 1 + values(item)
		}
	}
	return count
}

// An info dictionary names what a disk holds: a name and the parts of a
// path as long as a file's name is anywhere at most, a path as long as a
// path, and a bounded number of files and of values. One past any of
// these is not a torrent's, so that the name, which is part of each
// file's path, is not multiplied by the files: what is read stays within
// the dictionary's bytes and 806 bytes a file. Nor is one whose pieces
// are shorter than a block, which would be as many as its bytes.
func TestAnInfoDictionaryNamesWhatADiskHolds(t *testing.T) {
	t.Parallel()
	long := func(n int) string { return strings.Repeat("A", n) }
	// The parts of a path of n bytes under the longest name: as many as
	// long as fit, and one more.
	deep := func(n int) []string {
		var parts []string
		left := n - maxFileName
		for ; left > maxFileName+1; left -= maxFileName + 1 {
			parts = append(parts, long(maxFileName))
		}
		return append(parts, long(left-1))
	}
	sum := sha1.Sum([]byte{0})
	oneFile := func(name string) []byte {
		return encode(map[string]any{"name": name, "piece length": 16 << 10, "pieces": string(sum[:]), "length": 1})
	}
	// A torrent of one file of 3 bytes, in pieces of length bytes.
	pieced := func(length int) []byte {
		return encode(map[string]any{"name": "n", "piece length": length, "length": 3,
			"pieces": strings.Repeat(string(sum[:]), (3+length-1)/length)})
	}
	// A dictionary of n values: the 15 of a torrent of one file "f", and
	// empty parts in that file's path.
	valued := func(n int) []byte {
		return []byte("d5:filesld6:lengthi1e4:pathl" + strings.Repeat("0:", n-15) + "1:feee4:name1:n12:piece lengthi16384e6:pieces20:" +
			string(sum[:]) + "e")
	}
	for _, c := range []struct {
		name    string
		data    []byte
		files   int
		refused bool
	}{
		{"a name as long as a file's", infoOf(long(maxFileName), filesNamed(2)...), 2, false},
		{"a longer name", infoOf(long(maxFileName+1), filesNamed(2)...), 0, true},
		{"one file named as long as a file may be", oneFile(long(maxFileName)), 1, false},
		{"one file of a longer name", oneFile(long(maxFileName + 1)), 0, true},
		{"a folder named as long as a file may be", infoOf("n", []string{long(maxFileName), "f"}), 1, false},
		{"a folder of a longer name", infoOf("n", []string{long(maxFileName + 1), "f"}), 0, true},
		{"a path as long as a path may be", infoOf(long(maxFileName), deep(maxFilePath)), 1, false},
		{"a longer path", infoOf(long(maxFileName), deep(maxFilePath+1)), 0, true},
		{"as many files as a torrent lists", infoOf(long(maxFileName), filesNamed(maxInfoFiles)...), maxInfoFiles, false},
		{"one file more", infoOf("n", filesNamed(maxInfoFiles+1)...), 0, true},
		{"as many values as a dictionary holds", valued(maxInfoValues), 1, false},
		{"one value more", valued(maxInfoValues + 1), 0, true},
		{"a long name that many files would multiply", infoOf(long(64<<10), filesNamed(1000)...), 0, true},
		{"pieces as long as a block", pieced(blockSize), 1, false},
		{"pieces a byte shorter", pieced(blockSize - 1), 0, true},
		{"pieces of one byte", pieced(1), 0, true},
	} {
		if len(c.data) > maxInfoSize {
			t.Fatalf("%s: a dictionary of %d bytes", c.name, len(c.data))
		}
		info, err := parseInfo(c.data)
		if c.refused {
			if !errors.Is(err, errInfo) {
				t.Errorf("%s: read as %d files, error %v", c.name, len(info.Files), err)
			}
			continue
		}
		if err != nil || len(info.Files) != c.files {
			t.Errorf("%s: %d files, error %v, want %d files", c.name, len(info.Files), err, c.files)
			continue
		}
		paths := 0
		for _, f := range info.Files {
			if len(f.Path) > maxFilePath {
				t.Errorf("%s: a path of %d bytes", c.name, len(f.Path))
			}
			paths += len(f.Path)
		}
		if most := len(c.data) + len(info.Files)*(maxFileName+1); paths > most || info.size() > most+len(info.Files)*fileBytes ||
			info.size() < paths {
			t.Errorf("%s: %d bytes of paths and %d in all read from %d bytes, %d of paths at most", c.name, paths, info.size(),
				len(c.data), most)
		}
	}

	// The path of 4,096 bytes is one, and the values counted are those
	// decoding reads.
	if info, err := parseInfo(infoOf(long(maxFileName), deep(maxFilePath))); err != nil || len(info.Files[0].Path) != maxFilePath {
		t.Errorf("the longest path: %v", err)
	}
	for _, data := range [][]byte{[]byte("i1e"), []byte("l1:ai2ee"), []byte("d1:ad1:bleee"), valued(40), infoOf("n", filesNamed(3)...),
		fuzzTorrent(false).Info} {
		value, rest, err := decode(data)
		if err != nil || len(rest) != 0 {
			t.Fatalf("%q: %v", data, err)
		}
		if n := values(value); !valuesWithin(data, n) || valuesWithin(data, n-1) {
			t.Errorf("%q holds %d values, which is not what was counted", data, n)
		}
	}
	if valuesWithin([]byte("l1:a"+"x"), 100) || valuesWithin([]byte("5:ab"), 100) || valuesWithin([]byte("i1"), 100) {
		t.Error("bytes that are no values were counted")
	}
}

// A peer's info dictionary that names more than a disk holds is not the
// torrent's, whatever its hash: the torrent has no files to tell, plays
// nothing, and keeps no info dictionary once it stopped.
func TestAPeersInfoNamingTooMuchIsNotTaken(t *testing.T) {
	t.Parallel()
	const nameBytes, files = 32 << 10, 400
	data := infoOf(strings.Repeat("A", nameBytes), filesNamed(files)...)
	tor := &torrenttest.Torrent{Info: data, Hash: sha1.Sum(data), Data: []byte{0}, PieceLength: 16 << 10, Pieces: 1}
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	proxy := torrenttest.StartProxy(t, false, "", "")
	e := newEngine(t, proxied(proxy), 64<<20)
	// As an addon lists it: an info hash and a tracker, nothing else.
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(spec)
	defer stop()

	// The dictionary came whole from the peer and was read: taken, or
	// refused, its pieces then to be asked again.
	var taken *Info
	waitFor(t, "the peer's info dictionary", func() bool {
		running := torrentOf(e, spec.Hash)
		if running == nil {
			return false
		}
		running.mu.Lock()
		defer running.mu.Unlock()
		taken = running.info
		return taken != nil || bytes.Equal(running.metadata, data) && !slices.Contains(running.metadataGot, true)
	})
	if taken != nil {
		kept := 0
		for _, f := range taken.Files {
			kept += len(f.Path)
		}
		t.Fatalf("a peer's %d bytes of info dictionary became %d files holding %d bytes of paths", len(data), len(taken.Files), kept)
	}
	ended, cancel := context.WithCancel(context.Background())
	cancel()
	if _, listed, err := e.Files(ended, spec); err == nil {
		t.Fatalf("the torrent tells %d files", len(listed))
	}
	if _, err := e.Open(ended, spec); err == nil {
		t.Fatal("the torrent's file opens")
	}

	stop()
	e.End(spec.Hash)
	waitFor(t, "the torrent to stop", func() bool { return !e.Running(spec.Hash) })
	e.mu.Lock()
	run := e.kept[spec.Hash]
	e.mu.Unlock()
	if run.info != nil || run.infoData != nil || run.infoSize != 0 {
		t.Fatalf("the stopped torrent kept an info dictionary of %d bytes", run.infoSize)
	}
}

// The info dictionaries kept of stopped torrents stay within maxKeptInfo
// as they are in memory: the bytes that came, and what was read from them,
// its paths holding the name once for each file.
func TestKeptInfoDictionariesAreCountedAsRead(t *testing.T) {
	t.Parallel()
	// 5,000 files in folders of long names: about 4 MiB as it comes, and
	// twice that once read.
	folder := strings.Repeat("d", maxFileName)
	data := infoOf(strings.Repeat("A", maxFileName), slices.Repeat([][]string{{folder, "f"}}, 5000)...)
	info, err := parseInfo(data)
	if err != nil {
		t.Fatal(err)
	}
	if info.size() < len(data) {
		t.Fatalf("%d bytes read from a dictionary of %d", info.size(), len(data))
	}
	e := newEngine(t, &testSettings{s: Settings{Enabled: true}}, 64<<20)
	const stopped = 60
	if stopped*len(data) < maxKeptInfo || stopped > maxKept {
		t.Fatalf("%d dictionaries of %d bytes do not reach the bound", stopped, len(data))
	}
	for i := range stopped {
		// As many torrents as stopped, of the same dictionary, which the
		// test holds once.
		stopping := &torrent{hash: Hash{byte(i + 1)}, info: &info, infoData: data}
		e.mu.Lock()
		e.keepLocked(stopping)
		held := 0
		for _, kept := range e.kept {
			held += len(kept.infoData) + kept.info.size()
		}
		_, latest := e.kept[stopping.hash]
		e.mu.Unlock()
		if held > maxKeptInfo {
			t.Fatalf("after %d torrents stopped, their info dictionaries hold %d bytes, %d at most", i+1, held, maxKeptInfo)
		}
		if !latest {
			t.Fatalf("the info dictionary of torrent %d, the last stopped, is not kept", i+1)
		}
	}
}
