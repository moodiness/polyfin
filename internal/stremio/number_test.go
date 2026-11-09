package stremio

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"
)

// An addon's videoSize is decoded into 64 bits on every target, exactly,
// whether it is written as a whole number, in exponent form or as a
// string: past 2 GiB it overflowed an int on 32-bit targets, where Go
// leaves the conversion of such a float to the target.
func TestVideoSizesAreDecodedExactly(t *testing.T) {
	t.Parallel()
	for written, want := range map[string]Number64{
		`4e9`:                  4_000_000_000,
		`4000000000`:           4_000_000_000,
		`"4000000000"`:         4_000_000_000,
		`4.0e9`:                4_000_000_000,
		`9e9`:                  9_000_000_000,
		`2147483648`:           2_147_483_648,
		`4294967297`:           4_294_967_297,
		`1234.9`:               1234,
		`9007199254740993`:     9_007_199_254_740_993,
		`9223372036854775807`:  math.MaxInt64,
		`9223372036854775808`:  0,
		`1e19`:                 0,
		`-1e19`:                0,
		`"NaN"`:                0,
		`"a lot"`:              0,
		`null`:                 0,
		`-5`:                   -5,
		`"  12345678901  "`:    12_345_678_901,
		`12345678901.5`:        12_345_678_901,
		`-9223372036854775808`: math.MinInt64,
	} {
		var stream Stream
		if err := json.Unmarshal([]byte(`{"behaviorHints": {"videoSize": `+written+`}}`), &stream); err != nil {
			t.Fatalf("%s: %v", written, err)
		}
		if got := stream.BehaviorHints.VideoSize; got != want {
			t.Errorf("videoSize %s: got %d, want %d", written, got, want)
		}
	}
}

// A Number past int's range, as on 32-bit targets past 2^31, is 0, not
// what the target makes of the conversion.
func TestNumbersOutOfRangeAreZero(t *testing.T) {
	t.Parallel()
	past := strconv.FormatUint(uint64(math.MaxInt)+1, 10)
	for written, want := range map[string]Number{
		`3`:           3,
		`"7"`:         7,
		`2.9`:         2,
		past:          0,
		`-` + past:    math.MinInt,
		`1e300`:       0,
		`"Infinity"`:  0,
		`"-Infinity"`: 0,
	} {
		var got Number
		if err := json.Unmarshal([]byte(written), &got); err != nil || got != want {
			t.Errorf("%s: got %d, want %d (%v)", written, got, want, err)
		}
	}
}
