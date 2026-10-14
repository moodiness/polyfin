package mosaic

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/sfnt"
)

// Fonts are the fonts names are written with: those of a folder, its
// subfolders included, as the fallback fonts apps load for subtitles are
// (POLYFIN_FONTS_DIR), then Go Bold, built in, for the characters none of
// them has. Each character is written with the first font that has it, the
// bold sans-serif ones first, as DejaVu Sans Bold in the Docker image; a
// font is read the first time a character needs it. Fonts are safe for
// concurrent use.
type Fonts struct {
	dir string

	once  sync.Once
	files []string

	mu sync.Mutex
	// read holds the fonts read so far, in the order of files, and next
	// the first file not read yet.
	read []*sfnt.Font
	next int
	// chosen remembers the font of each character written so far, nil
	// for those no font has.
	chosen map[rune]*sfnt.Font
}

// NewFonts returns the fonts of dir; an empty or missing folder gives Go
// Bold alone.
func NewFonts(dir string) *Fonts {
	return &Fonts{dir: dir, chosen: map[rune]*sfnt.Font{}}
}

// builtin is Go Bold, which covers Latin, Greek and Cyrillic.
var builtin = sync.OnceValue(func() *sfnt.Font {
	font, err := sfnt.Parse(gobold.TTF)
	if err != nil {
		panic(err)
	}
	return font
})

// list finds the font files of the folder, in the order they are tried:
// bold sans-serif fonts first, then other upright sans-serif ones but
// light ones, then the others but italic ones, then italic ones, each by
// path. WOFF fonts, which apps load but Go does not read, are left out.
func (f *Fonts) list() {
	if f.dir == "" {
		return
	}
	_ = filepath.WalkDir(f.dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".ttf", ".otf", ".ttc", ".otc":
			f.files = append(f.files, path)
		}
		return nil
	})
	rank := func(path string) int {
		name := strings.ToLower(filepath.Base(path))
		sans := strings.Contains(name, "sans") && !strings.Contains(name, "mono")
		light := strings.Contains(name, "light") || strings.Contains(name, "thin")
		switch {
		case strings.Contains(name, "italic"), strings.Contains(name, "oblique"):
			return 3
		case sans && strings.Contains(name, "bold") && !strings.Contains(name, "condensed"):
			return 0
		case sans && !light:
			return 1
		}
		return 2
	}
	slices.SortStableFunc(f.files, func(a, b string) int {
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra - rb
		}
		return strings.Compare(a, b)
	})
}

// readFile reads the fonts of a file: one, or each of a collection's.
// A file that cannot be read gives none.
func readFile(path string) []*sfnt.Font {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if font, err := sfnt.Parse(data); err == nil {
		return []*sfnt.Font{font}
	}
	collection, err := sfnt.ParseCollection(data)
	if err != nil {
		return nil
	}
	var fonts []*sfnt.Font
	for i := range collection.NumFonts() {
		if font, err := collection.Font(i); err == nil {
			fonts = append(fonts, font)
		}
	}
	return fonts
}

// has reports whether font has a glyph for r.
func has(font *sfnt.Font, buffer *sfnt.Buffer, r rune) bool {
	index, err := font.GlyphIndex(buffer, r)
	return err == nil && index != 0
}

// fontFor returns the font r is written with, nil when none has it.
func (f *Fonts) fontFor(r rune) *sfnt.Font {
	f.once.Do(f.list)
	f.mu.Lock()
	defer f.mu.Unlock()
	if font, ok := f.chosen[r]; ok {
		return font
	}
	var buffer sfnt.Buffer
	found := func() *sfnt.Font {
		for _, font := range f.read {
			if has(font, &buffer, r) {
				return font
			}
		}
		for f.next < len(f.files) {
			fonts := readFile(f.files[f.next])
			f.next++
			f.read = append(f.read, fonts...)
			for _, font := range fonts {
				if has(font, &buffer, r) {
					return font
				}
			}
		}
		if has(builtin(), &buffer, r) {
			return builtin()
		}
		return nil
	}()
	f.chosen[r] = found
	return found
}

// primary is the font whose metrics place the lines: the first of the
// folder, else Go Bold.
func (f *Fonts) primary() *sfnt.Font {
	f.once.Do(f.list)
	f.mu.Lock()
	defer f.mu.Unlock()
	for len(f.read) == 0 && f.next < len(f.files) {
		f.read = append(f.read, readFile(f.files[f.next])...)
		f.next++
	}
	if len(f.read) > 0 {
		return f.read[0]
	}
	return builtin()
}
