// Package workfolder keeps what Polyfin empties at each start to the
// folders it made. The segments of conversions, the torrents' pieces, the
// guides and the picture subtitles being read each have a folder that is
// emptied when Polyfin starts, and that folder may be in one an
// administrator chose, beside files that are theirs. So Polyfin writes a
// marker file in each such folder it makes, and empties one only when it
// holds the marker, or nothing.
package workfolder

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

// Marker is the file Polyfin writes in a folder it makes for the files of
// a run: a folder that holds it is emptied at start.
const Marker = ".polyfin-work"

// markerText tells whoever finds the marker what it is for.
const markerText = "Polyfin made this folder, and empties it each time it starts.\n"

// ErrOccupied reports a folder that holds files and no marker, or a file
// or a link in the folder's place: Polyfin did not make it, and leaves
// it as it is.
var ErrOccupied = errors.New("it holds files Polyfin did not make")

// A Kind is one of the folders emptied at start. The versions before the
// marker wrote files and folders of a few known names there, by which a
// folder they left is told from any other (see Check).
type Kind struct {
	earlier *regexp.Regexp
}

// The folders emptied at start, each with the names the versions before
// the marker gave to what they wrote there. These names are those of the
// past: what is written there now needs no name here, as the folder has
// its marker.
var (
	// Segments is the folder of the segments of conversions and
	// repackagings. It held a folder for each of them, named by 24
	// hexadecimal digits, alone or after "live-", by "playout-" and a
	// channel's identifier, or by what the conversion is and a number; and
	// the files joined for a download, named "joined-" and a number.
	Segments = Kind{regexp.MustCompile(`^(?:(?:live-)?[0-9a-f]{24}|playout-[0-9a-f-]{32,36}|(?:audio|burn|encoding|joined|live|measure|tone-mapping)-[0-9]+)$`)}
	// Torrents is the folder of the torrents' pieces. It held a folder for
	// each torrent, named by the 40 hexadecimal digits of its hash.
	Torrents = Kind{regexp.MustCompile(`^[0-9a-f]{40}$`)}
	// Guides is the folder of the guides being read: each a file named
	// "guide-", a number and ".zip".
	Guides = Kind{regexp.MustCompile(`^guide-[0-9]+\.zip$`)}
	// OCR is the folder of the picture subtitles being read into text:
	// folders named "polyfin-ocr-" and a number, and files named
	// "polyfin-pictures-", a number and ".mks".
	OCR = Kind{regexp.MustCompile(`^polyfin-(?:ocr-[0-9]+|pictures-[0-9]+\.mks)$`)}
)

// Check reports whether dir is a folder Empty would empty: nil when
// there is nothing at dir, or a folder that holds the marker, nothing, or
// nothing but what a version before the marker wrote in a folder of this
// kind, which marks it at the first start after an upgrade. Any other
// folder, and a file or a link of that name, is ErrOccupied. Nothing is
// written.
func (k Kind) Check(dir string) error {
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s: %w", dir, ErrOccupied)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	earlier := true
	for _, entry := range entries {
		if entry.Name() == Marker {
			return nil
		}
		earlier = earlier && k.earlier.MatchString(entry.Name())
	}
	if !earlier {
		return fmt.Errorf("%s: %w", dir, ErrOccupied)
	}
	return nil
}

// Empty makes dir, a folder for the files of a run, with its marker, or
// removes what an earlier run left in a folder Polyfin made (see Check).
// A folder Polyfin did not make is left as it is, with ErrOccupied.
func (k Kind) Empty(dir string) error {
	if err := k.Check(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// The marker first: a folder left half emptied is still told at the
	// next start.
	if err := writeMarker(dir); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var errs []error
	for _, entry := range entries {
		if entry.Name() != Marker {
			errs = append(errs, os.RemoveAll(filepath.Join(dir, entry.Name())))
		}
	}
	return errors.Join(errs...)
}

// Mark tells that Polyfin made the folder dir, whatever it holds: for the
// folders of Polyfin's own data folder, which no administrator's files
// share, and which Empty then empties as every version did. A file or a
// link of that name is removed, and nothing is done where there is
// nothing.
func Mark(dir string) error {
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return os.Remove(dir)
	}
	return writeMarker(dir)
}

func writeMarker(dir string) error {
	return os.WriteFile(filepath.Join(dir, Marker), []byte(markerText), 0o600)
}
