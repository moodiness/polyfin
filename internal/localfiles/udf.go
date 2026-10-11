package localfiles

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
)

// A disc image's UDF file system (ECMA-167, as UDF restricts it), which
// Blu-ray images often carry alone: enough of it to list a folder and read
// a file, as an ISO 9660 file system is read (see discFS). It reads UDF
// 1.02 to 2.01, whose partitions are physical, and UDF 2.50 and 2.60,
// whose files are described in a metadata partition, itself a file of a
// physical partition; not the sparable and virtual partitions of
// rewritable and write-once media, nor 512-byte blocks, nor extended
// allocation descriptors: those layouts are told apart from broken file
// systems (see errUnsupportedUDF). Every read is bounded: a broken or
// hostile image fails, it never loops nor makes Polyfin hold more than a
// few MB.

// errNotUDF reports an image without a UDF file system Polyfin reads.
var errNotUDF = errors.New("no readable UDF file system")

// errUnsupportedUDF reports a well-formed UDF file system laid out in a way
// Polyfin does not read, which FFmpeg's bluray protocol may (see
// Service.readImage).
var errUnsupportedUDF = errors.New("a UDF layout that is not read")

// udfBlock is the size of the logical blocks and sectors of the UDF file
// systems read, those of optical discs.
const udfBlock = 2048

const (
	// maxUDFDescriptors bounds the volume descriptors read: a sequence
	// holds a few, in at least 16 blocks.
	maxUDFDescriptors = 64
	// maxUDFExtents bounds the extents of a file or of the metadata
	// partition, and maxUDFChains the blocks of further allocation
	// descriptors a file's lead to.
	maxUDFExtents = 4096
	maxUDFChains  = 16
)

// Descriptor tags' identifiers.
const (
	udfAnchor      = 2
	udfPartition   = 5
	udfLogical     = 6
	udfTerminating = 8
	udfFileSet     = 256
	udfFileID      = 257
	udfExtension   = 258
	udfFileEntry   = 261
	udfExtended    = 266
)

// udfImage is an image's UDF file system: its partitions, by reference
// number, and its root folder.
type udfImage struct {
	r     io.ReaderAt
	parts []*udfPart
	root  udfNode
}

// udfPart is a partition: physical, from block start for length blocks
// of the image, or a metadata partition, the bytes of its metadata file,
// in spans of the image; or one of a kind that is not read, unsupported.
type udfPart struct {
	start, length int64
	meta          []span
	unsupported   bool
}

// span is a part of a file in the image: n bytes at at, or zeros when at
// is negative.
type span struct {
	at, n int64
}

// udfAddress is a logical block of a partition.
type udfAddress struct {
	part  uint16
	block uint32
}

// udfNode is a file or folder: its size, and its bytes, in spans of the
// image or embedded in its entry.
type udfNode struct {
	dir      bool
	size     int64
	spans    []span
	embedded []byte
}

// openUDF reads the UDF file system of the image r: its anchor at block
// 256, its volume descriptors, the main sequence or else the reserve one,
// its partitions and its file set's root folder.
func openUDF(r io.ReaderAt) (*udfImage, error) {
	anchor := make([]byte, udfBlock)
	if _, err := r.ReadAt(anchor, 256*udfBlock); err != nil {
		anchor = anchor[:0]
	}
	if id, ok := udfTag(anchor); !ok || id != udfAnchor {
		// An anchor at sector 256 of 512-byte sectors.
		small := make([]byte, 512)
		if _, err := r.ReadAt(small, 256*512); err == nil {
			if id, ok := udfTag(small); ok && id == udfAnchor && binary.LittleEndian.Uint32(small[12:]) == 256 {
				return nil, fmt.Errorf("%w: 512-byte sectors", errUnsupportedUDF)
			}
		}
		return nil, fmt.Errorf("%w: no anchor", errNotUDF)
	}
	var err error
	for _, at := range []int{16, 24} {
		var img *udfImage
		length, location := binary.LittleEndian.Uint32(anchor[at:]), binary.LittleEndian.Uint32(anchor[at+4:])
		if img, err = readUDFVolume(r, int64(location), int64(length)/udfBlock); err == nil {
			return img, nil
		}
	}
	return nil, err
}

// readUDFVolume reads the volume descriptor sequence of count blocks from
// block at, and the file system it describes.
func readUDFVolume(r io.ReaderAt, at, count int64) (*udfImage, error) {
	partitions := map[uint16][2]int64{}
	var volume []byte
	block := make([]byte, udfBlock)
	for n := range min(count, maxUDFDescriptors) {
		if _, err := r.ReadAt(block, (at+n)*udfBlock); err != nil {
			return nil, fmt.Errorf("%w: %v", errNotUDF, err)
		}
		id, ok := udfTag(block)
		if !ok || id == udfTerminating {
			break
		}
		switch id {
		case udfPartition:
			partitions[binary.LittleEndian.Uint16(block[22:])] = [2]int64{int64(binary.LittleEndian.Uint32(block[188:])),
				int64(binary.LittleEndian.Uint32(block[192:]))}
		case udfLogical:
			volume = bytes.Clone(block)
		}
	}
	if volume == nil {
		return nil, fmt.Errorf("%w: no logical volume", errNotUDF)
	}
	switch size := binary.LittleEndian.Uint32(volume[212:]); {
	case size == udfBlock:
	case size >= 512 && size <= 32768 && size&(size-1) == 0:
		return nil, fmt.Errorf("%w: %d-byte blocks", errUnsupportedUDF, size)
	default:
		return nil, fmt.Errorf("%w: %d-byte blocks", errNotUDF, size)
	}
	img := &udfImage{r: r}
	// The partition maps: type 1, a physical partition by its number;
	// type 2, here a metadata partition, its metadata file in a physical
	// partition.
	table := int64(binary.LittleEndian.Uint32(volume[264:]))
	maps := volume[440:min(440+table, udfBlock)]
	for range min(binary.LittleEndian.Uint32(volume[268:]), 64) {
		if len(maps) < 2 || int(maps[1]) < 2 || int(maps[1]) > len(maps) {
			return nil, fmt.Errorf("%w: a partition map is cut short", errNotUDF)
		}
		m := maps[:maps[1]]
		maps = maps[maps[1]:]
		switch {
		case m[0] == 1 && len(m) >= 6:
			p, ok := partitions[binary.LittleEndian.Uint16(m[4:])]
			if !ok {
				return nil, fmt.Errorf("%w: no partition %d", errNotUDF, binary.LittleEndian.Uint16(m[4:]))
			}
			img.parts = append(img.parts, &udfPart{start: p[0], length: p[1]})
		case m[0] == 2 && len(m) >= 64 && strings.HasPrefix(string(m[5:28]), "*UDF Metadata Partition"):
			p, ok := partitions[binary.LittleEndian.Uint16(m[38:])]
			if !ok {
				return nil, fmt.Errorf("%w: no partition %d", errNotUDF, binary.LittleEndian.Uint16(m[38:]))
			}
			img.parts = append(img.parts, img.metadata(&udfPart{start: p[0], length: p[1]}, binary.LittleEndian.Uint32(m[40:]),
				binary.LittleEndian.Uint32(m[44:])))
		case m[0] == 2 && len(m) >= 64 && (strings.HasPrefix(string(m[5:28]), "*UDF Sparable Partition") ||
			strings.HasPrefix(string(m[5:28]), "*UDF Virtual Partition")):
			// The partition of rewritable or write-once media, which
			// nothing here reads.
			img.parts = append(img.parts, &udfPart{unsupported: true})
		default:
			img.parts = append(img.parts, nil)
		}
	}
	// The file set descriptor, then its root folder.
	fileSet, err := img.block(longAddress(volume[248:]))
	if err != nil {
		return nil, err
	}
	if id, ok := udfTag(fileSet); !ok || id != udfFileSet {
		return nil, fmt.Errorf("%w: no file set", errNotUDF)
	}
	if img.root, err = img.node(longAddress(fileSet[400:])); err != nil {
		return nil, err
	}
	if !img.root.dir {
		return nil, fmt.Errorf("%w: the root is no folder", errNotUDF)
	}
	return img, nil
}

// metadata is the metadata partition whose file is at block file of the
// physical partition p, or its mirror's at block mirror; nil when neither
// can be read.
func (img *udfImage) metadata(p *udfPart, file, mirror uint32) *udfPart {
	physical := &udfImage{r: img.r, parts: []*udfPart{p}}
	for _, block := range []uint32{file, mirror} {
		if node, err := physical.node(udfAddress{block: block}); err == nil && node.embedded == nil {
			return &udfPart{meta: node.spans}
		}
	}
	return nil
}

// longAddress reads the address of a long allocation descriptor.
func longAddress(ad []byte) udfAddress {
	return udfAddress{block: binary.LittleEndian.Uint32(ad[4:]), part: binary.LittleEndian.Uint16(ad[8:])}
}

// udfTag reads a descriptor's tag: its identifier, if its checksum holds.
func udfTag(d []byte) (uint16, bool) {
	if len(d) < 16 {
		return 0, false
	}
	var sum byte
	for i, b := range d[:16] {
		if i != 4 {
			sum += b
		}
	}
	return binary.LittleEndian.Uint16(d), sum == d[4]
}

// spans locates n bytes from a logical block of a partition in the image.
func (img *udfImage) spans(at udfAddress, n int64) ([]span, error) {
	if int(at.part) >= len(img.parts) || img.parts[at.part] == nil {
		return nil, fmt.Errorf("%w: no partition %d", errNotUDF, at.part)
	}
	p := img.parts[at.part]
	if p.unsupported {
		return nil, fmt.Errorf("%w: partition %d", errUnsupportedUDF, at.part)
	}
	off := int64(at.block) * udfBlock
	if p.meta == nil {
		if off+n > p.length*udfBlock {
			return nil, fmt.Errorf("%w: block %d is out of its partition", errNotUDF, at.block)
		}
		return []span{{at: p.start*udfBlock + off, n: n}}, nil
	}
	var spans []span
	for _, s := range p.meta {
		if n == 0 {
			break
		}
		if off >= s.n {
			off -= s.n
			continue
		}
		piece := min(s.n-off, n)
		at := s.at
		if at >= 0 {
			at += off
		}
		spans = append(spans, span{at: at, n: piece})
		off, n = 0, n-piece
	}
	if n > 0 {
		return nil, fmt.Errorf("%w: block %d is out of the metadata partition", errNotUDF, at.block)
	}
	return spans, nil
}

// block reads a logical block of a partition.
func (img *udfImage) block(at udfAddress) ([]byte, error) {
	spans, err := img.spans(at, udfBlock)
	if err != nil {
		return nil, err
	}
	data := make([]byte, udfBlock)
	if _, err := (spanReader{img.r, spans}).ReadAt(data, 0); err != nil {
		return nil, fmt.Errorf("%w: %v", errNotUDF, err)
	}
	return data, nil
}

// node reads the file entry at an address: what it is, and where its
// bytes are.
func (img *udfImage) node(at udfAddress) (udfNode, error) {
	entry, err := img.block(at)
	if err != nil {
		return udfNode{}, err
	}
	id, ok := udfTag(entry)
	var base int
	switch {
	case ok && id == udfFileEntry:
		base = 176
	case ok && id == udfExtended:
		base = 216
	default:
		return udfNode{}, fmt.Errorf("%w: no file entry at block %d", errNotUDF, at.block)
	}
	size := binary.LittleEndian.Uint64(entry[56:])
	extended, length := int64(binary.LittleEndian.Uint32(entry[base-8:])), int64(binary.LittleEndian.Uint32(entry[base-4:]))
	if size > 1<<50 || int64(base)+extended+length > udfBlock {
		return udfNode{}, fmt.Errorf("%w: a file entry is broken", errNotUDF)
	}
	node := udfNode{dir: entry[27] == 4, size: int64(size)}
	ads := entry[int64(base)+extended : int64(base)+extended+length]
	kind := binary.LittleEndian.Uint16(entry[34:]) & 7
	if kind == 3 {
		if node.size > int64(len(ads)) {
			return udfNode{}, fmt.Errorf("%w: embedded data is cut short", errNotUDF)
		}
		node.embedded = bytes.Clone(ads[:node.size])
		return node, nil
	}
	if node.spans, err = img.extents(ads, kind, at.part, node.size); err != nil {
		return udfNode{}, err
	}
	return node, nil
}

// extents reads a file's allocation descriptors, short (kind 0, in the
// partition of its entry, part) or long (kind 1), and those they lead to,
// up to size bytes.
func (img *udfImage) extents(ads []byte, kind, part uint16, size int64) ([]span, error) {
	width := 8
	switch kind {
	case 0:
	case 1:
		width = 16
	case 2:
		return nil, fmt.Errorf("%w: extended allocation descriptors", errUnsupportedUDF)
	default:
		return nil, fmt.Errorf("%w: allocation descriptors of type %d", errNotUDF, kind)
	}
	var spans []span
	left := size
	for chains := 0; left > 0; {
		var next *udfAddress
		for ; len(ads) >= width && left > 0; ads = ads[width:] {
			raw := binary.LittleEndian.Uint32(ads)
			n, at := int64(raw&0x3FFFFFFF), udfAddress{part: part, block: binary.LittleEndian.Uint32(ads[4:])}
			if width == 16 {
				at.part = binary.LittleEndian.Uint16(ads[8:])
			}
			if n == 0 {
				break
			}
			if raw>>30 == 3 {
				next = &at
				break
			}
			n = min(n, left)
			if raw>>30 != 0 {
				// Allocated or not, nothing is recorded: zeros.
				spans = append(spans, span{at: -1, n: n})
			} else {
				pieces, err := img.spans(at, n)
				if err != nil {
					return nil, err
				}
				spans = append(spans, pieces...)
			}
			left -= n
			if len(spans) > maxUDFExtents {
				return nil, fmt.Errorf("%w: too many extents", errNotUDF)
			}
		}
		if left == 0 {
			break
		}
		if next == nil || chains == maxUDFChains {
			return nil, fmt.Errorf("%w: a file's extents are cut short", errNotUDF)
		}
		chains++
		// The next allocation descriptors, in an allocation extent
		// descriptor.
		block, err := img.block(*next)
		if err != nil {
			return nil, err
		}
		if id, ok := udfTag(block); !ok || id != udfExtension {
			return nil, fmt.Errorf("%w: no allocation extent at block %d", errNotUDF, next.block)
		}
		ads = block[24:min(24+int64(binary.LittleEndian.Uint32(block[20:])), udfBlock)]
	}
	return spans, nil
}

// reader reads a file's bytes.
func (img *udfImage) reader(node udfNode) *io.SectionReader {
	if node.embedded != nil {
		return io.NewSectionReader(bytes.NewReader(node.embedded), 0, node.size)
	}
	return io.NewSectionReader(spanReader{img.r, node.spans}, 0, node.size)
}

// udfEntry is an entry of a folder: its name, whether it is a folder, and
// the address of its file entry.
type udfEntry struct {
	name string
	dir  bool
	at   udfAddress
}

// list lists a folder's entries, its parent and the deleted ones left
// out.
func (img *udfImage) list(dir udfNode) ([]udfEntry, error) {
	if !dir.dir || dir.size > maxISODirectory {
		return nil, fmt.Errorf("%w: no folder", errNotUDF)
	}
	data := make([]byte, dir.size)
	if _, err := img.reader(dir).ReadAt(data, 0); err != nil {
		return nil, fmt.Errorf("%w: %v", errNotUDF, err)
	}
	var entries []udfEntry
	for at := 0; at+38 <= len(data); {
		fid := data[at:]
		if id, ok := udfTag(fid); !ok || id != udfFileID {
			return nil, fmt.Errorf("%w: no file identifier at %d", errNotUDF, at)
		}
		name, use := int(fid[19]), int(binary.LittleEndian.Uint16(fid[36:]))
		if 38+use+name > len(fid) {
			return nil, fmt.Errorf("%w: a file identifier is cut short", errNotUDF)
		}
		if traits := fid[18]; traits&(4|8) == 0 {
			entries = append(entries, udfEntry{name: udfName(fid[38+use : 38+use+name]), dir: traits&2 != 0, at: longAddress(fid[20:])})
		}
		at += (38 + use + name + 3) &^ 3
	}
	return entries, nil
}

// udfName decodes a file identifier: OSTA compressed Unicode, 8 or 16
// bits a character.
func udfName(d []byte) string {
	if len(d) == 0 {
		return ""
	}
	switch d[0] {
	case 8, 254:
		runes := make([]rune, len(d)-1)
		for i, b := range d[1:] {
			runes[i] = rune(b)
		}
		return string(runes)
	case 16, 255:
		units := make([]uint16, (len(d)-1)/2)
		for i := range units {
			units[i] = binary.BigEndian.Uint16(d[1+2*i:])
		}
		return string(utf16.Decode(units))
	}
	return ""
}

// lookup finds the entry at path, its elements any case.
func (img *udfImage) lookup(path ...string) (udfNode, error) {
	node := img.root
	for _, name := range path {
		entries, err := img.list(node)
		if err != nil {
			return udfNode{}, err
		}
		i := 0
		for i < len(entries) && !strings.EqualFold(entries[i].name, name) {
			i++
		}
		if i == len(entries) {
			return udfNode{}, fmt.Errorf("%w: no %s", errNotUDF, name)
		}
		if node, err = img.node(entries[i].at); err != nil {
			return udfNode{}, err
		}
		node.dir = node.dir && entries[i].dir
	}
	return node, nil
}

// names lists the names of the folder at path's entries.
func (img *udfImage) names(path ...string) ([]string, error) {
	dir, err := img.lookup(path...)
	if err != nil {
		return nil, err
	}
	entries, err := img.list(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	return names, nil
}

// file opens the file at path.
func (img *udfImage) file(path ...string) (*io.SectionReader, error) {
	node, err := img.lookup(path...)
	if err != nil {
		return nil, err
	}
	if node.dir {
		return nil, fmt.Errorf("%w: %s is a folder", errNotUDF, strings.Join(path, "/"))
	}
	return img.reader(node), nil
}

// spanReader reads the bytes of spans of r, one after the other.
type spanReader struct {
	r     io.ReaderAt
	spans []span
}

func (s spanReader) ReadAt(p []byte, off int64) (int, error) {
	read := 0
	for _, sp := range s.spans {
		if len(p) == 0 {
			break
		}
		if off >= sp.n {
			off -= sp.n
			continue
		}
		piece := p[:min(int64(len(p)), sp.n-off)]
		if sp.at < 0 {
			clear(piece)
		} else if n, err := s.r.ReadAt(piece, sp.at+off); err != nil {
			return read + n, err
		}
		read += len(piece)
		p, off = p[len(piece):], 0
	}
	if len(p) > 0 {
		return read, io.EOF
	}
	return read, nil
}
