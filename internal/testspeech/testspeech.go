// Package testspeech makes versions that speak at known times, for
// tests: their audio is quiet noise but for bursts of voice-band tones,
// cut into syllables.
package testspeech

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/speech"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// Bursts are the times a version of length speaks: bursts of 0.8 to 3
// seconds, 0.6 to 4 seconds apart, irregular so that no other shift fits
// them. Each seed gives other times.
func Bursts(seed uint64, length time.Duration) []speech.Span {
	r := rand.New(rand.NewPCG(seed, seed))
	var spans []speech.Span
	for at := time.Second; at < length-5*time.Second; {
		d := time.Duration(800+r.IntN(2200)) * time.Millisecond
		spans = append(spans, speech.Span{Start: at, End: at + d})
		at += d + time.Duration(600+r.IntN(3400))*time.Millisecond
	}
	return spans
}

// Cues are subtitle cues at the times of spans, each mapped by at.
func Cues(spans []speech.Span, at func(time.Duration) time.Duration) []subtitles.Cue {
	cues := make([]subtitles.Cue, len(spans))
	for i, span := range spans {
		cues[i] = subtitles.Cue{Start: at(span.Start), End: at(span.End), Lines: []string{fmt.Sprintf("Line %d", i+1)}}
	}
	return cues
}

// Version makes, with the FFmpeg at ffmpeg, a 23.976 fps Matroska file of
// length that speaks at the times of spans, with a SubRip track of those
// times when track is set.
func Version(t *testing.T, ffmpeg string, spans []speech.Span, length time.Duration, track bool) []byte {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "audio.wav"), wav(spans, length), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=black:s=64x36:r=24000/1001",
		"-i", filepath.Join(dir, "audio.wav")}
	maps := []string{"-map", "0:v", "-map", "1:a"}
	if track {
		srt := subtitles.SubRip(Cues(spans, func(d time.Duration) time.Duration { return d }))
		if err := os.WriteFile(filepath.Join(dir, "track.srt"), srt, 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, "-i", filepath.Join(dir, "track.srt"))
		maps = append(maps, "-map", "2:s")
	}
	args = append(args, maps...)
	args = append(args, "-t", fmt.Sprint(length.Seconds()), "-c:v", "mpeg4", "-g", "240", "-c:a", "aac", "-b:a", "48k", "-c:s", "srt",
		filepath.Join(dir, "version.mkv"))
	if output, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("FFmpeg: %v: %s", err, output)
	}
	data, err := os.ReadFile(filepath.Join(dir, "version.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// wav is a mono WAV file of length at 16 kHz: quiet noise, with tones of
// 440 and 1250 Hz during spans, cut into syllables as speech is.
func wav(spans []speech.Span, length time.Duration) []byte {
	const rate = 16000
	samples := int(length.Seconds() * rate)
	data := make([]byte, 44+2*samples)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(36+2*samples))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], rate)
	binary.LittleEndian.PutUint32(data[28:], 2*rate)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(2*samples))
	noise := rand.New(rand.NewPCG(7, 7))
	syllables := syllablesOf(spans)
	j := 0
	for i := range samples {
		at := time.Duration(i) * time.Second / rate
		for j < len(syllables) && syllables[j].End <= at {
			j++
		}
		v := (noise.Float64() - 0.5) * 200
		if j < len(syllables) && syllables[j].Start <= at {
			s := at.Seconds()
			v += 6000*math.Sin(2*math.Pi*440*s) + 4000*math.Sin(2*math.Pi*1250*s)
		}
		binary.LittleEndian.PutUint16(data[44+2*i:], uint16(int16(v)))
	}
	return data
}

// syllablesOf cuts spans into syllables of 120 to 280 ms, 40 to 120 ms
// apart, the first starting and the last ending with its span.
func syllablesOf(spans []speech.Span) []speech.Span {
	r := rand.New(rand.NewPCG(3, 3))
	var syllables []speech.Span
	for _, span := range spans {
		for at := span.Start; at < span.End; {
			end := min(at+time.Duration(120+r.IntN(160))*time.Millisecond, span.End)
			syllables = append(syllables, speech.Span{Start: at, End: end})
			at = end + time.Duration(40+r.IntN(80))*time.Millisecond
		}
	}
	return syllables
}
