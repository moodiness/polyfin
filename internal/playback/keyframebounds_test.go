package playback

import (
	"encoding/binary"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

func isoBox(typ string, children ...[]byte) []byte {
	payload := slices.Concat(children...)
	return slices.Concat(binary.BigEndian.AppendUint32(nil, uint32(8+len(payload))), []byte(typ), payload)
}

func isoFullBox(typ string, values ...uint32) []byte {
	payload := make([]byte, 4)
	for _, value := range values {
		payload = binary.BigEndian.AppendUint32(payload, value)
	}
	return isoBox(typ, payload)
}

// allSyncMP4 is an MP4 file whose tables declare count samples of 1 byte
// after its head, each a sync sample, a ninetieth of a second apart: an
// index of a few hundred bytes whatever the count. The samples are zeros.
func allSyncMP4(count uint32) []byte {
	build := func(offset uint32) []byte {
		entry := isoBox("avc1", make([]byte, 78), isoBox("avcC", []byte{1, 100, 0, 31}))
		stbl := isoBox("stbl",
			isoBox("stsd", make([]byte, 4), binary.BigEndian.AppendUint32(nil, 1), entry),
			isoFullBox("stts", 1, count, 1),
			isoFullBox("stsz", 1, count),
			isoFullBox("stsc", 1, 1, count, 1),
			isoFullBox("stco", 1, offset))
		hdlr := isoBox("hdlr", make([]byte, 8), []byte("vide"), make([]byte, 13))
		trak := isoBox("trak", isoBox("mdia", isoFullBox("mdhd", 0, 0, 90, 0), hdlr, isoBox("minf", stbl)))
		return slices.Concat(isoBox("ftyp", []byte("isom"), make([]byte, 4)),
			isoBox("moov", isoFullBox("mvhd", 0, 0, 1000, 0), trak))
	}
	head := build(uint32(len(build(0))))
	return append(head, make([]byte, count)...)
}

// A file whose index declares more keyframes than a container index is
// read for has an unreadable index: nothing of it is kept or saved, and
// it is not read again for a while.
func TestAnIndexDeclaringMoreKeyframesThanAFileHasIsNotKept(t *testing.T) {
	t.Parallel()
	// One more than the keyframes read from an index.
	const declared = 1<<20 + 1
	s := newService(t, &fileSource{data: allSyncMP4(declared)}, "ffprobe", nil)
	ctx := t.Context()
	version := library.Version{ID: accounts.ID{0xA2}, URL: "https://93.184.216.34/video.mp4"}
	times, err := s.keyframes(ctx, version, media.Analysis{})
	if !errors.Is(err, container.ErrUnreadable) || times != nil {
		t.Fatalf("%d keyframes and %v, want none and an unreadable index", len(times), err)
	}
	if _, kept := s.unindexed.Get(version.ID); !kept {
		t.Error("the index would be read again at the next play")
	}
	if s.indexes.Len() != 0 || s.offsets.Len() != 0 {
		t.Errorf("%d indexes and %d lists of places are kept, want none", s.indexes.Len(), s.offsets.Len())
	}
	var saved int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM media_keyframes WHERE version_id = $1", version.ID).Scan(&saved); err != nil {
		t.Fatal(err)
	}
	if saved != 0 {
		t.Error("an index was saved")
	}
}

// The longest index read from a container is kept whole, its times and
// its places counted by their bytes, and saved in a row of a few bytes a
// keyframe, which a service started again reads back.
func TestAnIndexOfAsManyKeyframesAsAFileMayHaveIsKept(t *testing.T) {
	t.Parallel()
	// As many as are read from an index.
	const count = 1 << 20
	pool := migrated.New(t)
	s := newServiceOn(t, pool, &fileSource{data: allSyncMP4(count)}, "ffprobe", nil)
	ctx := t.Context()
	version := library.Version{ID: accounts.ID{0xA3}, URL: "https://93.184.216.34/video.mp4"}
	times, err := s.keyframes(ctx, version, media.Analysis{})
	if err != nil {
		t.Fatal(err)
	}
	indexed, _ := s.indexes.Get(version.ID)
	offsets, _ := s.offsets.Get(version.ID)
	if len(times) != count || len(indexed) != count || len(offsets) != count {
		t.Fatalf("%d keyframes, %d kept with their times and %d with their places; want %d each", len(times), len(indexed), len(offsets), count)
	}
	if got := s.indexes.Bytes(); got != count*8 || got > maxKeptIndexes {
		t.Errorf("the times kept count for %d bytes, want %d, within %d", got, count*8, maxKeptIndexes)
	}
	if got := s.offsets.Bytes(); got != count*16 || got > maxKeptOffsets {
		t.Errorf("the places kept count for %d bytes, want %d, within %d", got, count*16, maxKeptOffsets)
	}
	var saved int
	if err := s.db.QueryRow(ctx, "SELECT octet_length(keyframes) FROM media_keyframes WHERE version_id = $1", version.ID).Scan(&saved); err != nil {
		t.Fatal(err)
	}
	// The first time takes a byte, each next one, 11,111 microseconds
	// later, three.
	if want := 1 + 3*(count-1); saved != want {
		t.Errorf("the row saved holds %d bytes, want %d", saved, want)
	}
	// Times are saved to the microsecond.
	again := newServiceOn(t, pool, &fileSource{}, "ffprobe", nil)
	back, ok := again.savedKeyframes(ctx, version.ID)
	if !ok || len(back) != count || back[count-1] != times[count-1].Truncate(time.Microsecond) {
		t.Errorf("the row saved is read back as %d keyframes (%t), want the %d saved", len(back), ok, count)
	}
}

// A row saved with more keyframes than an index is read for, as rows
// were before indexes were bounded so, is not read back: its file is read
// as a version without a saved index is. The row stays.
func TestASavedIndexOfMoreKeyframesThanAFileMayHaveIsNotReadBack(t *testing.T) {
	t.Parallel()
	// One keyframe more than is read from an index, a microsecond apart:
	// a row of a byte each.
	const saved = 1<<20 + 1
	times := make([]time.Duration, saved)
	for i := range times {
		times[i] = time.Duration(i) * time.Microsecond
	}
	row := encodeKeyframes(times)
	tests := []struct {
		name string
		// declared is how many keyframes the file's index lists now.
		declared uint32
	}{
		{"a file whose index lists as many", saved},
		{"a file whose index lists fewer now", 1000},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := newService(t, &fileSource{data: allSyncMP4(test.declared)}, "ffprobe", nil)
			ctx := t.Context()
			version := library.Version{ID: accounts.ID{0xA4, byte(i)}, URL: "https://93.184.216.34/video.mp4"}
			if _, err := s.db.Exec(ctx, "INSERT INTO media_keyframes (version_id, keyframes) VALUES ($1, $2)", version.ID, row); err != nil {
				t.Fatal(err)
			}
			got, err := s.keyframes(ctx, version, media.Analysis{})
			_, refused := s.unindexed.Get(version.ID)
			var length int
			if err := s.db.QueryRow(ctx, "SELECT octet_length(keyframes) FROM media_keyframes WHERE version_id = $1", version.ID).Scan(&length); err != nil {
				t.Fatal(err)
			}
			if test.declared < saved {
				// The file is read, and its index replaces the row.
				if err != nil || len(got) != int(test.declared) || refused || length >= len(row) {
					t.Fatalf("%d keyframes and %v, kept as unreadable: %t, a row of %d bytes; want the file's %d keyframes saved",
						len(got), err, refused, length, test.declared)
				}
				return
			}
			if !errors.Is(err, container.ErrUnreadable) || got != nil || !refused || s.indexes.Len() != 0 {
				t.Fatalf("%d keyframes and %v, kept as unreadable: %t, %d indexes kept; want an unreadable index and nothing kept",
					len(got), err, refused, s.indexes.Len())
			}
			if length != len(row) {
				t.Errorf("the row now holds %d bytes, want the %d saved before", length, len(row))
			}
			// While the version is kept so, the row is not read back.
			if back, ok := s.savedKeyframes(ctx, version.ID); ok || back != nil {
				t.Errorf("the row is read back as %d keyframes", len(back))
			}
		})
	}
}

// The keyframe times and places kept in memory are bounded by their
// bytes, not only by the number of versions: the least recently used
// leave once many long indexes pass the bound.
func TestTheKeyframeIndexesKeptAreBoundedInBytes(t *testing.T) {
	t.Parallel()
	s := newService(t, &fileSource{}, "ffprobe", nil)
	// 2 MiB of times and 4 MiB of places a version.
	const each = 1 << 18
	var latest accounts.ID
	for i := range 40 {
		latest = accounts.ID{0xB0, byte(i)}
		s.indexes.Put(latest, make([]time.Duration, each))
		s.offsets.Put(latest, make([]container.Keyframe, each))
		if times, places := s.indexes.Bytes(), s.offsets.Bytes(); times > maxKeptIndexes || places > maxKeptOffsets {
			t.Fatalf("after %d versions: %d bytes of times and %d of places are kept, want at most %d and %d",
				i+1, times, places, maxKeptIndexes, maxKeptOffsets)
		}
	}
	if got, want := s.indexes.Len(), maxKeptIndexes/(each*8); got != want || s.indexes.Bytes() != maxKeptIndexes {
		t.Errorf("the times of %d versions are kept in %d bytes, want %d in %d", got, s.indexes.Bytes(), want, maxKeptIndexes)
	}
	if got, want := s.offsets.Len(), maxKeptOffsets/(each*16); got != want || s.offsets.Bytes() != maxKeptOffsets {
		t.Errorf("the places of %d versions are kept in %d bytes, want %d in %d", got, s.offsets.Bytes(), want, maxKeptOffsets)
	}
	if _, kept := s.indexes.Get(latest); !kept {
		t.Error("the times stored last are not kept")
	}
	if _, kept := s.offsets.Get(latest); !kept {
		t.Error("the places stored last are not kept")
	}
	// A slice counts for the room it holds, not for what it lists.
	if got := indexBytes(make([]time.Duration, 1, 100)); got != 800 {
		t.Errorf("room for 100 times counts for %d bytes, want 800", got)
	}
	if got := offsetBytes(make([]container.Keyframe, 1, 100)); got != 1600 {
		t.Errorf("room for 100 places counts for %d bytes, want 1600", got)
	}
}
