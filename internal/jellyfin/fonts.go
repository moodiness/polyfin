package jellyfin

import (
	"cmp"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Fallback fonts.
//
// jellyfin-web renders ASS subtitles in the browser, with the fonts the
// file carries, then with the server's fallback fonts, which it lists and
// downloads here. Polyfin offers the fonts of its fonts folder (Options.
// FontsDir), which the Docker image fills with DejaVu.

// maxFallbackFonts bounds the size of the fonts listed, as Jellyfin does:
// the smallest come first, until the next would reach 20 MB.
const maxFallbackFonts = 20 << 20

// fallbackExtensions are those of the font files Jellyfin offers.
var fallbackExtensions = []string{".woff", ".woff2", ".ttf", ".otf"}

// FontFile is Jellyfin's description of a fallback font.
type FontFile struct {
	Name         string
	Size         int64
	DateCreated  Time
	DateModified Time
}

// fallbackFontFiles finds the fonts of the fonts folder, by file name.
// Jellyfin reads only the files at the top of its folder; Polyfin reads
// the folders within too, as system font folders such as /usr/share/fonts
// keep each family in its own. A name found twice is the first found.
func (h *Handler) fallbackFontFiles() map[string]string {
	files := map[string]string{}
	if h.FontsDir == "" {
		return files
	}
	_ = filepath.WalkDir(h.FontsDir, func(file string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		name := strings.ToLower(entry.Name())
		if slices.Contains(fallbackExtensions, path.Ext(name)) && files[name] == "" {
			files[name] = file
		}
		return nil
	})
	return files
}

// fallbackFonts lists the fallback fonts, smallest first, as Jellyfin does.
// A folder that does not exist lists none.
func (h *Handler) fallbackFonts(w http.ResponseWriter, _ *http.Request) {
	fonts := []FontFile{}
	for _, file := range h.fallbackFontFiles() {
		info, err := os.Stat(file)
		if err != nil || info.Size() == 0 {
			continue
		}
		// Go cannot tell when a file was created everywhere: it is given
		// as when it last changed.
		modified := info.ModTime().UTC()
		fonts = append(fonts, FontFile{Name: filepath.Base(file), Size: info.Size(),
			DateCreated: Time(modified), DateModified: Time(modified)})
	}
	slices.SortFunc(fonts, func(a, b FontFile) int {
		return cmp.Or(cmp.Compare(a.Size, b.Size), strings.Compare(a.Name, b.Name))
	})
	var total int64
	for i, font := range fonts {
		if total += font.Size; total >= maxFallbackFonts {
			fonts = fonts[:i]
			break
		}
	}
	writeJSON(w, http.StatusOK, fonts)
}

// fallbackFont serves a fallback font by its file name, compared without
// regard to case. Like Jellyfin, an unknown name answers 200 without a
// body, which the subtitle renderer copes with better than an error.
func (h *Handler) fallbackFont(w http.ResponseWriter, r *http.Request) {
	file, ok := h.fallbackFontFiles()[strings.ToLower(r.PathValue("name"))]
	if !ok {
		w.WriteHeader(http.StatusOK)
		return
	}
	font, err := os.Open(file)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	defer font.Close()
	info, err := font.Stat()
	if err != nil || info.Size() == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", fontFormats[strings.ToLower(path.Ext(file))])
	http.ServeContent(w, r, "", info.ModTime(), font)
}
