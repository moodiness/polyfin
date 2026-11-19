package torrent

import (
	"crypto/sha1"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

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
	// A dictionary refused tells why: refused is that reason.
	for _, c := range []struct {
		name    string
		data    []byte
		files   int
		refused string
	}{
		{"a name as long as a file's", infoOf(long(maxFileName), filesNamed(2)...), 2, ""},
		{"a longer name", infoOf(long(maxFileName+1), filesNamed(2)...), 0, "its name is longer than 765 bytes"},
		{"one file named as long as a file may be", oneFile(long(maxFileName)), 1, ""},
		{"one file of a longer name", oneFile(long(maxFileName + 1)), 0, "its name is longer than 765 bytes"},
		{"a folder named as long as a file may be", infoOf("n", []string{long(maxFileName), "f"}), 1, ""},
		{"a folder of a longer name", infoOf("n", []string{long(maxFileName + 1), "f"}), 0,
			"a file or folder name is longer than 765 bytes"},
		{"a path as long as a path may be", infoOf(long(maxFileName), deep(maxFilePath)), 1, ""},
		{"a longer path", infoOf(long(maxFileName), deep(maxFilePath+1)), 0, "a path is longer than 4096 bytes"},
		{"as many files as a torrent lists", infoOf(long(maxFileName), filesNamed(maxInfoFiles)...), maxInfoFiles, ""},
		{"one file more", infoOf("n", filesNamed(maxInfoFiles+1)...), 0, "it lists more than 50000 files"},
		{"as many values as a dictionary holds", valued(maxInfoValues), 1, ""},
		{"one value more", valued(maxInfoValues + 1), 0, "it holds more than 800000 values, or is malformed"},
		{"a long name that many files would multiply", infoOf(long(64<<10), filesNamed(1000)...), 0,
			"its name is longer than 765 bytes"},
		{"pieces as long as a block", pieced(blockSize), 1, ""},
		{"pieces a byte shorter", pieced(blockSize - 1), 0, "its pieces are shorter than 16384 bytes"},
		{"pieces of one byte", pieced(1), 0, "its pieces are shorter than 16384 bytes"},
	} {
		if len(c.data) > maxInfoSize {
			t.Fatalf("%s: a dictionary of %d bytes", c.name, len(c.data))
		}
		info, err := parseInfo(c.data)
		if c.refused != "" {
			if want := "the torrent's info dictionary is refused: " + c.refused; !errors.Is(err, ErrInfoRefused) || err.Error() != want {
				t.Errorf("%s: read as %d files, error %v, want %q", c.name, len(info.Files), err, want)
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
// nothing, and keeps no info dictionary once it stopped. As every peer
// sends that dictionary, the torrent stops at once: its reads fail with
// why, not as a start out of time, the log tells it, and the next run,
// while what this one learned is kept, does not start.
func TestAPeersInfoNamingTooMuchIsNotTaken(t *testing.T) {
	t.Parallel()
	const nameBytes, files = 32 << 10, 400
	data := infoOf(strings.Repeat("A", nameBytes), filesNamed(files)...)
	tor := &torrenttest.Torrent{Info: data, Hash: sha1.Sum(data), Data: []byte{0}, PieceLength: 16 << 10, Pieces: 1}
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	proxy := torrenttest.StartProxy(t, false, "", "")
	var log lockedBuffer
	// The dictionary comes within moments; a read waiting for the start's
	// whole time is the torrent taken for one that did not start.
	e := newEngineWith(t, proxied(proxy), func(c *Config) {
		c.StartTimeout = 20 * time.Second
		c.Logger = slog.New(slog.NewTextHandler(&log, nil))
	})
	// As an addon lists it: an info hash and a tracker, nothing else.
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(spec)
	defer stop()

	const why = "the torrent's info dictionary is refused: its name is longer than 765 bytes"
	r, err := e.Open(t.Context(), spec)
	if err == nil {
		r.Close()
		t.Fatal("the torrent's file opens")
	}
	if !errors.Is(err, ErrInfoRefused) || err.Error() != why {
		t.Fatalf("the read failed with %v, want %q", err, why)
	}
	waitFor(t, "the torrent to stop", func() bool { return !e.Running(spec.Hash) })
	if !strings.Contains(log.String(), `msg="A torrent did not start: its info dictionary is refused" error="`+why+`"`) {
		t.Errorf("the log tells:\n%s", log.String())
	}
	e.mu.Lock()
	run := e.kept[spec.Hash]
	e.mu.Unlock()
	if run.info != nil || run.infoData != nil || run.infoSize != 0 {
		t.Fatalf("the stopped torrent kept an info dictionary of %d bytes", run.infoSize)
	}

	// Played again, it does not start, and its reads tell why at once.
	again := e.Play(spec)
	defer again()
	if e.Running(spec.Hash) {
		t.Error("the torrent runs again")
	}
	if _, listed, err := e.Files(t.Context(), spec); !errors.Is(err, ErrInfoRefused) || err.Error() != why {
		t.Errorf("the torrent's files: %d, %v", len(listed), err)
	}
	if _, err := e.Open(t.Context(), spec); !errors.Is(err, ErrInfoRefused) || err.Error() != why {
		t.Errorf("the torrent's file read again: %v", err)
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
