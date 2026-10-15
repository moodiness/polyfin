package ocr

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/subtitles"
	"github.com/moodiness/polyfin/internal/testsubtitles"
)

// captions are two captions of known text, the second of two lines.
var captions = []testsubtitles.Caption{
	{Lines: []string{"The lighthouse keeper waited"}, Start: 1500 * time.Millisecond, End: 4 * time.Second},
	{Lines: []string{"Seven boats came home", "before the storm"}, Start: 5 * time.Second, End: 8250 * time.Millisecond},
}

// tesseract is the Tesseract of POLYFIN_TEST_TESSERACT, with the data of
// POLYFIN_TEST_TESSDATA when set, writing its pictures in the test's
// folder; tests reading text skip without it.
func tesseract(t *testing.T) *Tesseract {
	t.Helper()
	path := os.Getenv("POLYFIN_TEST_TESSERACT")
	if path == "" {
		t.Skip("POLYFIN_TEST_TESSERACT is not set")
	}
	return &Tesseract{Path: path, DataDir: os.Getenv("POLYFIN_TEST_TESSDATA"), TempDir: t.TempDir()}
}

// times are the start and end of each picture.
func times(pictures []Picture) [][2]time.Duration {
	var spans [][2]time.Duration
	for _, p := range pictures {
		spans = append(spans, [2]time.Duration{p.Start, p.End})
	}
	return spans
}

func wantTimes(t *testing.T, pictures []Picture, tolerance time.Duration) {
	t.Helper()
	if len(pictures) != len(captions) {
		t.Fatalf("pictures at %v, want %d", times(pictures), len(captions))
	}
	for i, p := range pictures {
		if d := p.Start - captions[i].Start; d < -tolerance || d > tolerance {
			t.Errorf("picture %d starts at %v, want %v", i, p.Start, captions[i].Start)
		}
		if d := p.End - captions[i].End; d < -tolerance || d > tolerance {
			t.Errorf("picture %d ends at %v, want %v", i, p.End, captions[i].End)
		}
	}
}

// wantText checks the cues read have the captions' text and times: the
// words compared without case or punctuation, which Tesseract may add.
func wantText(t *testing.T, cues []subtitles.Cue, want []testsubtitles.Caption, tolerance time.Duration) {
	t.Helper()
	if len(cues) != len(want) {
		t.Fatalf("read %d cues, want %d: %+v", len(cues), len(want), cues)
	}
	normalize := func(lines []string) string {
		return strings.Join(strings.FieldsFunc(strings.ToLower(strings.Join(lines, " ")), func(r rune) bool {
			return !('a' <= r && r <= 'z' || r >= 0x80)
		}), " ")
	}
	for i, cue := range cues {
		if got, wanted := normalize(cue.Lines), normalize(want[i].Lines); got != wanted {
			t.Errorf("cue %d reads %q, want %q", i, cue.Lines, want[i].Lines)
		}
		if len(cue.Lines) != len(want[i].Lines) {
			t.Errorf("cue %d has lines %q, want %d", i, cue.Lines, len(want[i].Lines))
		}
		if d := cue.Start - want[i].Start; d < -tolerance || d > tolerance {
			t.Errorf("cue %d starts at %v, want %v", i, cue.Start, want[i].Start)
		}
		if d := cue.End - want[i].End; d < -tolerance || d > tolerance {
			t.Errorf("cue %d ends at %v, want %v", i, cue.End, want[i].End)
		}
	}
}

func TestPGSPicturesKeepTheirTimes(t *testing.T) {
	sup := testsubtitles.Captions{CanvasWidth: 1920, CanvasHeight: 1080, Size: 56, Shown: captions}.PGS()
	pictures, err := PGS(sup)
	if err != nil {
		t.Fatal(err)
	}
	wantTimes(t, pictures, time.Millisecond)
	for i, p := range pictures {
		// Cropped to the text, with its margin: far smaller than the
		// canvas, and not upscaled, its lines being tall enough.
		if b := p.Image.Bounds(); b.Dx() >= 1920 || b.Dy() >= 400 {
			t.Errorf("picture %d is %v, not cropped", i, b)
		}
		// Dark text on white: the corners are white, some pixels dark.
		if p.Image.GrayAt(0, 0).Y != 255 || !slices.ContainsFunc(p.Image.Pix, func(v uint8) bool { return v < 64 }) {
			t.Errorf("picture %d is not dark text on white", i)
		}
	}
}

func TestVobSubPicturesAreUpscaled(t *testing.T) {
	idx, sub := testsubtitles.Captions{CanvasWidth: 720, CanvasHeight: 480, Size: 22, Shown: captions}.VobSub("en")
	pictures, err := VobSub([]byte(idx), sub)
	if err != nil {
		t.Fatal(err)
	}
	// The stop delay counts in 1024/90000 s.
	wantTimes(t, pictures, 12*time.Millisecond)
	for i, p := range pictures {
		// DVD lines, some 25 pixels tall, are enlarged at least twice.
		if b := p.Image.Bounds(); b.Dy() < 2*25 {
			t.Errorf("picture %d is %v, not upscaled", i, b)
		}
		if p.Image.GrayAt(0, 0).Y != 255 {
			t.Errorf("picture %d is not on white", i)
		}
	}
}

func TestTesseractReadsPGS(t *testing.T) {
	reader := tesseract(t)
	pictures, err := PGS(testsubtitles.Captions{CanvasWidth: 1920, CanvasHeight: 1080, Size: 56, Shown: captions}.PGS())
	if err != nil {
		t.Fatal(err)
	}
	cues, err := reader.Read(context.Background(), pictures, "eng")
	if err != nil {
		t.Fatal(err)
	}
	wantText(t, cues, captions, time.Millisecond)
}

func TestTesseractReadsVobSub(t *testing.T) {
	reader := tesseract(t)
	idx, sub := testsubtitles.Captions{CanvasWidth: 720, CanvasHeight: 480, Size: 22, Shown: captions}.VobSub("en")
	pictures, err := VobSub([]byte(idx), sub)
	if err != nil {
		t.Fatal(err)
	}
	cues, err := reader.Read(context.Background(), pictures, "eng")
	if err != nil {
		t.Fatal(err)
	}
	wantText(t, cues, captions, 12*time.Millisecond)
}

// TestTesseractReadsWithTheDataFolder reads French with the data of
// DataDir, the program's own folder lacking it.
func TestTesseractReadsWithTheDataFolder(t *testing.T) {
	reader := tesseract(t)
	languages, err := reader.Languages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(languages, "fra") {
		t.Skip("no French data: set POLYFIN_TEST_TESSDATA to a folder holding fra.traineddata")
	}
	french := []testsubtitles.Caption{{Lines: []string{"Le gardien du phare attendait"}, Start: time.Second, End: 3 * time.Second}}
	pictures, err := PGS(testsubtitles.Captions{CanvasWidth: 1920, CanvasHeight: 1080, Size: 56, Shown: french}.PGS())
	if err != nil {
		t.Fatal(err)
	}
	cues, err := reader.Read(context.Background(), pictures, "fra")
	if err != nil {
		t.Fatal(err)
	}
	wantText(t, cues, french, time.Millisecond)
}

func TestDataFolderLanguages(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"fra.traineddata", "osd.traineddata", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := (&Tesseract{DataDir: dir}).dirLanguages(); !slices.Equal(got, []string{"fra"}) {
		t.Errorf("languages %v, want [fra]", got)
	}
}

func TestLanguage(t *testing.T) {
	for _, c := range []struct{ code, fallback, want string }{
		{"fre", "en", "fra"},
		{"fra", "en", "fra"},
		{"fr", "en", "fra"},
		{"ger", "en", "deu"},
		{"dut", "en", "nld"},
		{"eng", "fr", "eng"},
		{"", "fr", "fra"},
		{"und", "en", "eng"},
		{"chi", "en", "chi_sim"},
		{"xx", "en", ""},
	} {
		if got := Language(c.code, c.fallback); got != c.want {
			t.Errorf("Language(%q, %q) = %q, want %q", c.code, c.fallback, got, c.want)
		}
	}
}
