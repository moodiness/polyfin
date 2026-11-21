package playback

import (
	"context"
	"encoding/binary"
	"net/http"
	"slices"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// rangedSource is a fileSource recording the ranges asked.
type rangedSource struct {
	fileSource
	mu     sync.Mutex
	ranges []string
}

func (f *rangedSource) Open(ctx context.Context, method, target string, header http.Header, confined bool) (*http.Response, error) {
	f.mu.Lock()
	f.ranges = append(f.ranges, header.Get("Range"))
	f.mu.Unlock()
	return f.fileSource.Open(ctx, method, target, header, confined)
}

// hashedFile is a file of 200,000 bytes whose hash is its size plus 3: its
// first 8 bytes read 1, its last 2, little-endian, and the others are 0.
func hashedFile() []byte {
	data := make([]byte, 200_000)
	binary.LittleEndian.PutUint64(data, 1)
	binary.LittleEndian.PutUint64(data[len(data)-8:], 2)
	return data
}

func TestTheVideoHashAddsTheSizeAndTheWordsOfBothEnds(t *testing.T) {
	t.Parallel()
	data := hashedFile()
	if got := videoHash(int64(len(data)), data[:videoHashBytes], data[len(data)-videoHashBytes:]); got != "0000000000030d43" {
		t.Errorf("hash %s, want 0000000000030d43", got)
	}
	// The sum wraps around.
	head := make([]byte, videoHashBytes)
	binary.LittleEndian.PutUint64(head, ^uint64(0))
	if got := videoHash(2, head, make([]byte, videoHashBytes)); got != "0000000000000001" {
		t.Errorf("wrapped hash %s, want 0000000000000001", got)
	}
}

// A link's hash is read from its two ends, 64 KiB each, one request each,
// and kept: the file's size comes with it, in place of the addon's.
func TestSubtitleHintsReadTheEndsOfALinkOnce(t *testing.T) {
	t.Parallel()
	host := &rangedSource{fileSource: fileSource{data: hashedFile()}}
	s := newService(t, host, "ffprobe", nil)
	version := library.Version{ID: accounts.ID{61}, Addon: "addon", Origin: library.Origin{Addon: accounts.ID{9}},
		URL: "https://93.184.216.34/movie.mkv", Filename: "Movie.mkv", Size: 199_000}
	if got := s.SubtitleHints(t.Context(), version); got != (library.SubtitleHints{Filename: "Movie.mkv", Size: 199_000}) {
		t.Errorf("before the read: %+v", got)
	}
	// The read the hints started, or this one, which joins it.
	s.readVideoHash(version)
	want := library.SubtitleHints{Filename: "Movie.mkv", Size: 200_000, Hash: "0000000000030d43"}
	for range 2 {
		if got := s.SubtitleHints(t.Context(), version); got != want {
			t.Errorf("after the read: %+v, want %+v", got, want)
		}
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if want := []string{"bytes=0-65535", "bytes=134464-199999"}; !slices.Equal(host.ranges, want) {
		t.Errorf("ranges asked %q, want %q", host.ranges, want)
	}
}

// The ends a play read already make the hash, with no request; a
// torrent's are never read for it, as only a play joins its swarm.
func TestSubtitleHintsUseTheBytesAlreadyRead(t *testing.T) {
	t.Parallel()
	host := &rangedSource{fileSource: fileSource{data: hashedFile()}}
	s := newService(t, host, "ffprobe", nil)
	version := library.Version{ID: accounts.ID{62}, Addon: "addon", Origin: library.Origin{Addon: accounts.ID{9}},
		URL: "https://93.184.216.34/movie.mkv", Filename: "Movie.mkv"}
	src := s.OpenSource(version)
	whole := make([]byte, len(host.data))
	if _, err := src.ReadAt(t.Context(), whole, 0); err != nil {
		t.Fatal(err)
	}
	src.Release()
	host.mu.Lock()
	read := len(host.ranges)
	host.mu.Unlock()
	if got := s.SubtitleHints(t.Context(), version); got.Hash != "0000000000030d43" || got.Size != 200_000 {
		t.Errorf("from the bytes read: %+v", got)
	}
	host.mu.Lock()
	if len(host.ranges) != read {
		t.Errorf("%d requests for the hash", len(host.ranges)-read)
	}
	host.mu.Unlock()

	torrent := library.Version{ID: accounts.ID{63}, Addon: "addon", Filename: "Movie.mkv", Size: 200_000,
		Torrent: &library.Torrent{InfoHash: "0123456789abcdef0123456789abcdef01234567"}}
	if got := s.SubtitleHints(t.Context(), torrent); got != (library.SubtitleHints{Filename: "Movie.mkv", Size: 200_000}) {
		t.Errorf("a torrent's: %+v", got)
	}
	if _, failed := s.unhashed.Get(torrent.ID); failed {
		t.Error("a torrent's ends were read")
	}
	if _, ok := s.hashes.Get(torrent.ID); ok {
		t.Error("a torrent's hash was read")
	}
}
