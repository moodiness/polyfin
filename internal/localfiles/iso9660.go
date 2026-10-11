package localfiles

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// A disc image's ISO 9660 file system, which DVD images carry beside UDF
// and many Blu-ray images too: enough of it to list a folder and read a
// file, each a contiguous extent.

// errNotISO reports an image without an ISO 9660 file system.
var errNotISO = errors.New("no ISO 9660 file system")

// isoSector is the size of an image's sectors.
const isoSector = 2048

// maxISODirectory bounds the directories read: a disc's hold a few dozen
// entries.
const maxISODirectory = 1 << 20

// isoImage is an image's ISO 9660 file system.
type isoImage struct {
	r     io.ReaderAt
	block int64
	root  isoEntry
}

// isoEntry is a file or folder of an image: its name, without version,
// and its extent.
type isoEntry struct {
	name     string
	dir      bool
	at, size int64
}

// openISO reads the primary volume descriptor of the image r, at sector
// 16 or after, up to the set's terminator.
func openISO(r io.ReaderAt) (*isoImage, error) {
	sector := make([]byte, isoSector)
	for n := int64(16); n < 32; n++ {
		if _, err := r.ReadAt(sector, n*isoSector); err != nil {
			return nil, fmt.Errorf("%w: %v", errNotISO, err)
		}
		if string(sector[1:6]) != "CD001" {
			return nil, errNotISO
		}
		switch sector[0] {
		case 1:
			block := int64(binary.LittleEndian.Uint16(sector[128:]))
			root, ok := isoRecord(sector[156:190], block)
			if !ok || block == 0 {
				return nil, fmt.Errorf("%w: no root folder", errNotISO)
			}
			return &isoImage{r: r, block: block, root: root}, nil
		case 255:
			return nil, fmt.Errorf("%w: no primary volume", errNotISO)
		}
	}
	return nil, errNotISO
}

// isoRecord reads a directory record, of block-sized blocks.
func isoRecord(record []byte, block int64) (isoEntry, bool) {
	if len(record) < 34 || int(record[0]) > len(record) || int(record[0]) < 33+int(record[32]) {
		return isoEntry{}, false
	}
	name := string(record[33 : 33+int(record[32])])
	if i := strings.IndexByte(name, ';'); i >= 0 {
		name = name[:i]
	}
	return isoEntry{name: strings.TrimSuffix(name, "."), dir: record[25]&2 != 0,
		at: int64(binary.LittleEndian.Uint32(record[2:])) * block, size: int64(binary.LittleEndian.Uint32(record[10:]))}, true
}

// list lists a folder's entries, the folder itself and its parent left
// out.
func (img *isoImage) list(dir isoEntry) ([]isoEntry, error) {
	if !dir.dir || dir.size > maxISODirectory {
		return nil, fmt.Errorf("%w: %q is no folder", errNotISO, dir.name)
	}
	data := make([]byte, dir.size)
	if _, err := img.r.ReadAt(data, dir.at); err != nil {
		return nil, err
	}
	var entries []isoEntry
	for at := 0; at < len(data); {
		length := int(data[at])
		if length == 0 {
			// Records do not cross sectors: the rest of this one is padding.
			at = (at/isoSector + 1) * isoSector
			continue
		}
		if at+length > len(data) {
			break
		}
		record := data[at : at+length]
		if entry, ok := isoRecord(record, img.block); ok && record[32] == 1 && (record[33] == 0 || record[33] == 1) {
			// The folder itself and its parent.
		} else if ok {
			entries = append(entries, entry)
		}
		at += length
	}
	return entries, nil
}

// lookup finds the entry at path, its elements any case.
func (img *isoImage) lookup(path ...string) (isoEntry, error) {
	entry := img.root
	for _, name := range path {
		entries, err := img.list(entry)
		if err != nil {
			return isoEntry{}, err
		}
		found := false
		for _, e := range entries {
			if strings.EqualFold(e.name, name) {
				entry, found = e, true
				break
			}
		}
		if !found {
			return isoEntry{}, fmt.Errorf("%w: no %s", errNotISO, name)
		}
	}
	return entry, nil
}

// reader reads a file of the image.
func (img *isoImage) reader(file isoEntry) *io.SectionReader {
	return io.NewSectionReader(img.r, file.at, file.size)
}
