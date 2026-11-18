package container

import (
	"context"
	"errors"
	"testing"
	"time"
)

// cuesShaped returns n units of 14 bytes, each a CuePoint of no track
// whose Void element holds the ID of Cues and a size reaching the end of
// the units: n elements shaped as Cues that end where what holds the
// units does, none listing a keyframe.
func cuesShaped(n int) []byte {
	units := make([]byte, 0, n*14)
	for k := range n {
		rest := 14 * (n - 1 - k)
		units = append(units,
			0xBB, 0x8C, // CuePoint, 12 bytes
			0xB3, 0x81, 0x00, // CueTime 0
			0xEC, 0x87, // Void, 7 bytes
			0x1C, 0x53, 0xBB, 0x6B, // the ID of Cues
			0x20|byte(rest>>16), byte(rest>>8), byte(rest), // its size: the units left
		)
	}
	return units
}

// Cues no SeekHead lists are looked for among the last elements shaped as
// Cues at the end of the file, a few of them: the search reads each to
// its end, and gives up rather than read as many as a file may hold.
func TestCuesAreSearchedAmongAFewCandidates(t *testing.T) {
	t.Parallel()
	cues := ebmlElement(idCues, cuePoint(0, 1), cuePoint(2000, 1))
	for _, test := range []struct {
		name   string
		shaped int
		found  bool
	}{
		{"the Cues alone", 0, true},
		{"the Cues last of those tried", maxCuesTried - 1, true},
		{"the Cues after those tried", maxCuesTried, false},
		{"the Cues after thousands", 3000, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// The elements shaped as Cues follow the Cues, in a Void
			// element ending the Segment: the search, from the end, meets
			// them first.
			data := matroskaFile(
				ebmlElement(idTracks, trackEntry(1, trackTypeVideo)),
				ebmlElement(idCluster, make([]byte, 64)),
				cues,
				ebmlElement(idVoid, cuesShaped(test.shaped)),
			)
			if int64(len(data)) > tailWindows*window {
				t.Fatalf("the file of %d bytes is not searched whole", len(data))
			}
			times, err := Keyframes(context.Background(), &memory{data: data}, int64(len(data)))
			if !test.found {
				if !errors.Is(err, ErrNoIndex) {
					t.Fatalf("got %v, %v; want ErrNoIndex", times, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			checkTimes(t, times, []time.Duration{0, 2 * time.Second})
		})
	}
}
