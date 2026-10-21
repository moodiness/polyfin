package lyrics

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/accounts"
)

// MaxUpload bounds the bytes of a lyrics file uploaded. A song's LRC file
// is a few kilobytes, and tens with the timing of every word: 1 MiB, the
// bound of LRCLIB's answers too, holds any song's lyrics many times over,
// while what is read is held in memory and kept in the database whole.
const MaxUpload = 1 << 20

var (
	// ErrNoExtension reports an uploaded file named without an extension,
	// which tells how to read it.
	ErrNoExtension = errors.New("a lyrics file name without an extension")
	// ErrNotLyrics reports an uploaded file that is no lyrics: not an LRC
	// or text file by its extension, not UTF-8 text, over MaxUpload, or
	// without a line of text.
	ErrNotLyrics = errors.New("not a lyrics file")
)

// uploadExtensions are the extensions of the lyrics files taken, as
// Jellyfin's lyrics parsers take them: LRC, enhanced LRC, and text.
var uploadExtensions = []string{".lrc", ".elrc", ".txt"}

// ParseUpload reads a lyrics file uploaded under name as the lyrics files
// of local music folders are read (see Parse): synced when it holds a
// timed line, plain otherwise. It returns the text kept, without a byte
// order mark, its kind, and its lyrics. A file that is not text, holding
// invalid UTF-8 or control characters other than tabs and line breaks, is
// refused, as is one over MaxUpload.
func ParseUpload(name string, body []byte) (string, Lyrics, error) {
	extension := strings.ToLower(path.Ext(strings.ReplaceAll(name, `\`, "/")))
	switch {
	case extension == "" || extension == ".":
		return "", Lyrics{}, ErrNoExtension
	case !slices.Contains(uploadExtensions, extension), len(body) > MaxUpload:
		return "", Lyrics{}, ErrNotLyrics
	}
	body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(body) || bytes.ContainsFunc(body, func(r rune) bool { return r < ' ' && r != '\t' && r != '\n' && r != '\r' || r == 0x7f }) {
		return "", Lyrics{}, ErrNotLyrics
	}
	text := string(body)
	found, ok := Parse(text)
	if !ok {
		return "", Lyrics{}, ErrNotLyrics
	}
	return text, found, nil
}

// Upload makes a lyrics file uploaded under name the track's lyrics (see
// ParseUpload), kept as an administrator's choice is: no look-up replaces
// them, a later choice or upload does, and Remove removes them. Unlike
// lyrics chosen among LRCLIB's results, they are served whether the
// settings turn lyrics from LRCLIB on or not. It returns them, or
// ErrNoExtension or ErrNotLyrics for a file it cannot read.
func (s *Service) Upload(ctx context.Context, track accounts.ID, name string, body []byte) (Lyrics, error) {
	text, found, err := ParseUpload(name, body)
	if err != nil {
		return Lyrics{}, err
	}
	if s == nil {
		return Lyrics{}, ErrNotFound
	}
	kind := kindPlain
	if found.Synced {
		kind = kindSynced
	}
	if err := s.keep(ctx, track, kind, text, true); err != nil {
		return Lyrics{}, fmt.Errorf("save the lyrics uploaded: %w", err)
	}
	s.logger.Info("Lyrics were uploaded for a track", "track", track, "synced", found.Synced)
	return found, nil
}
