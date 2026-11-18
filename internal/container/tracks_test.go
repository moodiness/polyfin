package container

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

// compressedPrivate is a video TrackEntry whose CodecPrivate, private, is
// compressed with zlib stages times over.
func compressedPrivate(number uint64, private []byte, stages int) []byte {
	var encodings [][]byte
	for order := range stages {
		encodings = append(encodings, contentEncodingElement(uint64(order), scopePrivate, encodingCompression,
			ebmlUint(idContentCompAlgo, compressionZlib)))
	}
	return ebmlElement(idTrackEntry,
		ebmlUint(idTrackNumber, number),
		ebmlUint(idTrackType, trackTypeVideo),
		ebmlElement(idCodecPrivate, private),
		contentEncodings(encodings...),
	)
}

// The CodecPrivates of a file are decoded within one budget, four times
// the size one may decode to, and one that inflates past that size took
// as much of it as it inflated: after three of them a small one still
// decodes, after four none does, however many TrackEntries follow.
func TestCodecPrivatesThatDoNotDecodeTakeFromTheBudget(t *testing.T) {
	t.Parallel()
	// A few hundred bytes that inflate to one byte more than a
	// CodecPrivate may.
	oversized := deflate(deflate(make([]byte, maxPrivate+1)))
	small := []byte("[Script Info]\nScriptType: v4.00+\n")
	for _, test := range []struct {
		name      string
		oversized int
		decoded   bool
	}{
		{"three leave some of the budget", 3, true},
		{"four spend it", 4, false},
		{"forty spend no more", 40, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var entries [][]byte
			for i := range test.oversized {
				entries = append(entries, compressedPrivate(uint64(i+1), oversized, 2))
			}
			entries = append(entries, compressedPrivate(uint64(test.oversized+1), deflate(small), 1))
			tracks := openMemory(t, matroskaFile(ebmlElement(idTracks, entries...))).Tracks()
			if len(tracks) != test.oversized+1 {
				t.Fatalf("%d tracks, want %d", len(tracks), test.oversized+1)
			}
			for _, track := range tracks[:test.oversized] {
				if track.Decodable || !bytes.Equal(track.CodecPrivate, oversized) {
					t.Fatalf("track %d: decodable %t, a CodecPrivate of %d bytes", track.Number, track.Decodable, len(track.CodecPrivate))
				}
			}
			last := tracks[test.oversized]
			if last.Decodable != test.decoded {
				t.Fatalf("the track after %d that do not decode: decodable %t, want %t", test.oversized, last.Decodable, test.decoded)
			}
			if test.decoded && !bytes.Equal(last.CodecPrivate, small) {
				t.Errorf("its CodecPrivate: %q", last.CodecPrivate)
			}
		})
	}
}

// What undoing each encoding of a CodecPrivate made is taken from the
// budget to the byte, and nothing is undone once it is spent: 8
// TrackEntries and 64 make as many bytes, the budget and one.
func TestDecodingCodecPrivatesStopsOnceTheBudgetIsSpent(t *testing.T) {
	t.Parallel()
	inner := deflate(make([]byte, maxPrivate+1))
	oversized := compressedPrivate(1, deflate(inner), 2)
	small := compressedPrivate(2, deflate([]byte("[Script Info]\n")), 1)
	// parse reads a TrackEntry with what budget has left, as the
	// TrackEntries of a Tracks element are.
	parse := func(entry []byte, budget *int64) Track {
		t.Helper()
		var track Track
		err := children(entry, func(_ uint32, data []byte) error {
			var err error
			track, _, err = parseTrack(data, budget)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return track
	}

	// The first encoding undone made the inner stream, the second a byte
	// more than a CodecPrivate may be.
	budget := int64(maxPrivates)
	if track := parse(oversized, &budget); track.Decodable {
		t.Fatal("a CodecPrivate inflating past its limit decoded")
	}
	if made, want := maxPrivates-budget, int64(len(inner))+maxPrivate+1; made != want {
		t.Fatalf("one TrackEntry took %d bytes from the budget, want the %d it inflated", made, want)
	}

	for _, count := range []int{8, 64} {
		budget := int64(maxPrivates)
		for range count {
			if track := parse(oversized, &budget); track.Decodable {
				t.Fatal("a CodecPrivate inflating past its limit decoded")
			}
		}
		if made := maxPrivates - budget; made != maxPrivates+1 {
			t.Errorf("%d TrackEntries made %d bytes, want %d", count, made, maxPrivates+1)
		}
		// A CodecPrivate that would decode is left as stored.
		before := budget
		if track := parse(small, &budget); track.Decodable || budget != before {
			t.Errorf("with the budget spent: decodable %t, %d bytes taken", track.Decodable, before-budget)
		}
	}
}

// The TrackEntries are not read on once the context ended, where the
// reader itself goes on answering.
func TestTracksAreNotReadOnceTheContextEnded(t *testing.T) {
	t.Parallel()
	data := matroskaFile(ebmlElement(idTracks, trackEntry(1, trackTypeVideo), trackEntry(2, trackTypeSubtitle)))
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := OpenMatroska(ctx, &memory{data: data}, int64(len(data))); err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := OpenMatroska(ctx, &memory{data: data}, int64(len(data))); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want the context's error", err)
	}
}
