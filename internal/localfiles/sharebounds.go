package localfiles

import (
	"bufio"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// errBoundListing is the failure of a share's scan one of whose folders
// lists more than is read of one folder (see listBounds), beside those
// of shareFailure.
const errBoundListing = "listing_too_large"

// shareBound reports a share that holds more than a scan reads: failure
// is the bound met, as a folder's Error, and detail what met it, for the
// log.
type shareBound struct {
	failure, detail string
}

func (e *shareBound) Error() string { return "the share holds more than a scan reads: " + e.detail }

// listBounds bound what the listing of one folder reads and keeps,
// whatever the share's server answers: answer, the bytes of one answer,
// a page for an S3 bucket; entry, the bytes of one entry of an answer;
// entries, the entries the folder lists, kept or not; name, the bytes of
// an entry's name; names, the bytes of the names kept together.
type listBounds struct {
	answer, entry int64
	entries       int
	name, names   int
}

// folderBounds are the bounds of a folder's listing: an answer of 64 MiB,
// each of its entries within 256 KiB, where a server spends a few hundred
// bytes on one; 500,000 entries, several times the files of the largest
// libraries kept in one folder; names of 1,024 bytes, the longest key of
// an S3 object, when file systems stop at 255 bytes or at 255 UTF-16
// units, 765 bytes; and 64 MiB of names kept, 128 bytes for each entry of
// a folder at its bound.
var folderBounds = listBounds{answer: 64 << 20, entry: 256 << 10, entries: 500_000, name: 1024, names: 64 << 20}

// boundedReader gives the answer listing a folder to its decoder within
// the answer's bound, and no further than the end of the entry being
// read, which the decoder moves on before each (see next): the decoders
// read an entry whole before they give it, whatever its size. over is the
// bound met, nil until one is.
type boundedReader struct {
	r *bufio.Reader
	// read counts the bytes given; end and stop are the offsets the
	// answer and the entry being read end at, at the latest.
	read, end, stop int64
	entry           int64
	over            *shareBound
}

func newBoundedReader(body io.Reader, b listBounds) *boundedReader {
	return &boundedReader{r: bufio.NewReader(body), end: b.answer, stop: b.entry, entry: b.entry}
}

// next lets the decoder read one more entry, from offset on, the offset
// it read the answer to.
func (b *boundedReader) next(offset int64) { b.stop = offset + b.entry }

// left is how many bytes the decoder may still be given: none once a
// bound is met, with the error to answer it.
func (b *boundedReader) left() (int64, error) {
	switch {
	case b.over != nil:
	case b.read >= b.end:
		// An answer that ends at its bound is whole.
		if _, err := b.r.Peek(1); err != nil {
			return 0, err
		}
		b.over = &shareBound{failure: errBoundListing, detail: fmt.Sprintf("an answer of more than %d bytes", b.end)}
	case b.read >= b.stop:
		b.over = &shareBound{failure: errBoundListing, detail: fmt.Sprintf("an entry of more than %d bytes in an answer", b.entry)}
	default:
		return min(b.end, b.stop) - b.read, nil
	}
	return 0, b.over
}

func (b *boundedReader) Read(p []byte) (int, error) {
	left, err := b.left()
	if err != nil {
		return 0, err
	}
	if int64(len(p)) > left {
		p = p[:left]
	}
	n, err := b.r.Read(p)
	b.read += int64(n)
	return n, err
}

// ReadByte has an XML decoder read the answer byte by byte, without a
// buffer of its own that would read past the entry.
func (b *boundedReader) ReadByte() (byte, error) {
	if _, err := b.left(); err != nil {
		return 0, err
	}
	c, err := b.r.ReadByte()
	if err == nil {
		b.read++
	}
	return c, err
}

// eachElement reads the XML answer listing a folder element by element,
// never whole, within b: read is given in turn each element the answer's
// first one holds, and decodes it or skips it. An element nested deeper is
// read with the one that holds it, within the bytes of one entry.
func eachElement(body io.Reader, b listBounds, read func(d *xml.Decoder, start xml.StartElement) error) error {
	in := newBoundedReader(body, b)
	d := xml.NewDecoder(in)
	first := true
	for {
		in.next(d.InputOffset())
		token, err := d.Token()
		if start, ok := token.(xml.StartElement); ok && err == nil {
			if first {
				first = false
				continue
			}
			err = read(d, start)
		}
		switch {
		case in.over != nil:
			return in.over
		case errors.Is(err, io.EOF):
			return nil
		case err != nil:
			return err
		}
	}
}

// folderList keeps the entries of one folder's listing, within bounds.
type folderList struct {
	bounds  listBounds
	entries []remoteEntry
	// sent counts the entries the server listed, kept or not; names the
	// bytes of the names kept.
	sent, names int
}

// another counts one more entry of the listing, kept or not.
func (l *folderList) another() error {
	if l.sent++; l.sent > l.bounds.entries {
		return &shareBound{failure: errBoundListing, detail: fmt.Sprintf("more than %d entries in one folder", l.bounds.entries)}
	}
	return nil
}

// keep keeps an entry of the listing, under a name of its own: the one
// read may be a part of a longer text of the answer, which it would
// keep whole.
func (l *folderList) keep(entry remoteEntry) error {
	if err := l.room(entry.name); err != nil {
		return err
	}
	entry.name = strings.Clone(entry.name)
	l.entries = append(l.entries, entry)
	return nil
}

// add keeps an entry of the listing whose name is a string of its own,
// as the client of a share decodes it.
func (l *folderList) add(entry remoteEntry) error {
	if err := l.room(entry.name); err != nil {
		return err
	}
	l.entries = append(l.entries, entry)
	return nil
}

// room counts the bytes of a name to keep.
func (l *folderList) room(name string) error {
	if len(name) > l.bounds.name {
		return &shareBound{failure: errBoundListing, detail: fmt.Sprintf("a name of more than %d bytes", l.bounds.name)}
	}
	if l.names += len(name); l.names > l.bounds.names {
		return &shareBound{failure: errBoundListing, detail: fmt.Sprintf("names of more than %d bytes together in one folder", l.bounds.names)}
	}
	return nil
}
