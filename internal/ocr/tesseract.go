// Package ocr reads picture subtitles, PGS and VobSub, into text: it
// decodes their pictures in Go, prepares them as Tesseract reads best, and
// runs Tesseract as a program, as Polyfin runs FFmpeg.
package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// Tesseract runs the tesseract program.
type Tesseract struct {
	// Path is the program, "tesseract" from PATH by default.
	Path string
	// DataDir is a folder of .traineddata files besides the program's own,
	// for languages it lacks; empty for none.
	DataDir string
	// TempDir is the folder the pictures are written to for the program,
	// the system's temporary folder when empty.
	TempDir string

	mu     sync.Mutex
	listed time.Time
	own    []string
}

// ErrNoLanguage reports a track whose language has no Tesseract data.
var ErrNoLanguage = errors.New("no Tesseract data for the language")

// listFor is how long the program's own languages are remembered.
const listFor = 10 * time.Minute

// path is the program run.
func (t *Tesseract) path() string {
	if t.Path == "" {
		return "tesseract"
	}
	return t.Path
}

// Languages lists the languages Tesseract has data for, by Tesseract's
// codes ("eng", "fra"): the program's own, and those of DataDir.
func (t *Tesseract) Languages(ctx context.Context) ([]string, error) {
	own, err := t.ownLanguages(ctx)
	if err != nil {
		return nil, err
	}
	languages := slices.Clone(own)
	for _, language := range t.dirLanguages() {
		if !slices.Contains(languages, language) {
			languages = append(languages, language)
		}
	}
	slices.Sort(languages)
	return languages, nil
}

// ownLanguages are the languages of the program's own data folder.
func (t *Tesseract) ownLanguages(ctx context.Context) ([]string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.listed.IsZero() && time.Since(t.listed) < listFor {
		return t.own, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, t.path(), "--list-langs").Output()
	if err != nil {
		return nil, fmt.Errorf("list Tesseract's languages: %w", err)
	}
	var languages []string
	// The first line names the folder.
	for _, line := range strings.Split(string(out), "\n")[1:] {
		if line = strings.TrimSpace(line); line != "" && line != "osd" {
			languages = append(languages, line)
		}
	}
	t.own, t.listed = languages, time.Now()
	return languages, nil
}

// dirLanguages are the languages of DataDir's .traineddata files.
func (t *Tesseract) dirLanguages() []string {
	if t.DataDir == "" {
		return nil
	}
	files, _ := filepath.Glob(filepath.Join(t.DataDir, "*.traineddata"))
	languages := make([]string, 0, len(files))
	for _, file := range files {
		if name := strings.TrimSuffix(filepath.Base(file), ".traineddata"); name != "osd" {
			languages = append(languages, name)
		}
	}
	return languages
}

// Read reads the text of pictures with the data of language, a Tesseract
// code, with one process limited to one thread, which loads the data
// once. The cues keep the pictures' times; pictures without text are left
// out.
func (t *Tesseract) Read(ctx context.Context, pictures []Picture, language string) ([]subtitles.Cue, error) {
	if len(pictures) == 0 {
		return nil, nil
	}
	dir, err := os.MkdirTemp(t.TempDir, "polyfin-ocr-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	var list bytes.Buffer
	for i, picture := range pictures {
		name := filepath.Join(dir, strconv.Itoa(i)+".png")
		var data bytes.Buffer
		if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&data, picture.Image); err != nil {
			return nil, err
		}
		if err := os.WriteFile(name, data.Bytes(), 0o600); err != nil {
			return nil, err
		}
		list.WriteString(name + "\n")
	}
	listFile := filepath.Join(dir, "pictures.txt")
	if err := os.WriteFile(listFile, list.Bytes(), 0o600); err != nil {
		return nil, err
	}
	// Each picture is a block of text; the pages are separated by a form
	// feed, Tesseract's default page separator, written after each page.
	args := []string{listFile, "stdout", "-l", language, "--psm", "6", "-c", "page_separator=\f"}
	if t.DataDir != "" && slices.Contains(t.dirLanguages(), language) {
		args = append([]string{"--tessdata-dir", t.DataDir}, args...)
	}
	cmd := exec.CommandContext(ctx, t.path(), args...)
	cmd.Env = append(os.Environ(), "OMP_THREAD_LIMIT=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tesseract: %w: %s", err, lastLine(stderr.String()))
	}
	pages := strings.Split(string(out), "\f")
	if len(pages) < len(pictures) {
		return nil, fmt.Errorf("tesseract read %d pictures of %d", len(pages), len(pictures))
	}
	var cues []subtitles.Cue
	for i, picture := range pictures {
		var lines []string
		for line := range strings.SplitSeq(pages[i], "\n") {
			if line = strings.Join(strings.Fields(line), " "); line != "" {
				lines = append(lines, line)
			}
		}
		if len(lines) > 0 {
			cues = append(cues, subtitles.Cue{Start: picture.Start, End: picture.End, Lines: lines})
		}
	}
	return cues, nil
}

// lastLine is the last line of a program's error output.
func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return lines[len(lines)-1]
}

// tesseractCodes are Tesseract's codes for languages its data names
// otherwise than by their ISO 639-2/T code (or 639-2 when there is a
// single one).
var tesseractCodes = map[string]string{"chi": "chi_sim", "nob": "nor", "nno": "nor"}

// Language is the Tesseract code of a track's language, given by any of
// its ISO 639 codes: "fre", "fra" and "fr" all give "fra". A track of no
// stated language is read in fallback, the server's language. Empty when
// neither names a language.
func Language(code, fallback string) string {
	if undetermined(code) {
		code = fallback
	}
	bibliographic := localization.LanguageCode(code)
	if !localization.KnownLanguage(bibliographic) {
		return ""
	}
	if tesseract, ok := tesseractCodes[bibliographic]; ok {
		return tesseract
	}
	for _, l := range localization.Languages() {
		if l.Bibliographic == bibliographic {
			if l.Terminology != "" {
				return l.Terminology
			}
			return l.Bibliographic
		}
	}
	return bibliographic
}

// undetermined reports a code that states no language.
func undetermined(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "", "und", "mul", "zxx", "mis":
		return true
	}
	return false
}
