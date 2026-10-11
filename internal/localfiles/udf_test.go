package localfiles

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"io"
	"math/rand/v2"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// udfLayout is how buildUDF writes an image: UDF 2.50, with a metadata
// partition and extended file entries, or 2.01, all in one physical
// partition with file entries. partition, for 2.01, names the partition
// map of rewritable or write-once media its one partition is described
// by instead, such as "*UDF Sparable Partition".
type udfLayout struct {
	metadata  bool
	partition string
}

// buildUDF writes an image holding files, by path with slashes, with a
// UDF file system only, as Blu-ray discs' images often are: its volume
// recognition sequence, anchors at block 256 and at the end, main and
// reserve volume descriptor sequences, an integrity descriptor, and one
// partition holding a file set, its folders and files, through a metadata
// partition for UDF 2.50.
func buildUDF(files map[string][]byte, layout udfLayout) []byte {
	b := &udfBuilder{layout: layout, files: files, children: map[string][]string{"": nil}}
	return b.build()
}

type udfBuilder struct {
	layout   udfLayout
	files    map[string][]byte
	children map[string][]string
	// entries are the logical blocks of the file entries, by path, in the
	// partition holding them; data those of the folders' and files' bytes.
	entries, data map[string]uint32
	// meta is the number of blocks holding the file set and the entries
	// and folders, physical the partition's blocks.
	meta, physical uint32
	img            []byte
}

func (b *udfBuilder) build() []byte {
	// Each folder is a key of children, with its entries.
	for name := range b.files {
		for p := name; p != ""; {
			parent := path.Dir(p)
			if parent == "." {
				parent = ""
			}
			if !slices.Contains(b.children[parent], p) {
				b.children[parent] = append(b.children[parent], p)
			}
			p = parent
		}
	}
	for dir := range b.children {
		slices.Sort(b.children[dir])
	}
	var dirs []string
	for dir := range b.children {
		dirs = append(dirs, dir)
	}
	slices.Sort(dirs)
	var names []string
	for name := range b.files {
		names = append(names, name)
	}
	slices.Sort(names)
	// The metadata blocks: the file set, its terminator, then each
	// folder's and file's entry, then each folder's identifiers.
	b.entries, b.data = map[string]uint32{}, map[string]uint32{}
	next := uint32(2)
	for _, p := range append(slices.Clone(dirs), names...) {
		b.entries[p] = next
		next++
	}
	for _, dir := range dirs {
		b.data[dir] = next
		next += blocks(int64(len(b.folder(dir))))
	}
	b.meta = next
	// The physical partition: for 2.50, the metadata file and its mirror,
	// then the metadata blocks; then the files' bytes.
	next = b.meta
	if b.layout.metadata {
		next = 2 + b.meta
	}
	for _, name := range names {
		b.data[name] = next
		next += max(blocks(int64(len(b.files[name]))), 1)
	}
	b.physical = next
	total := 257 + b.physical + 1
	b.img = make([]byte, int64(total)*udfBlock)
	b.write(total)
	return b.img
}

func blocks(n int64) uint32 { return uint32((n + udfBlock - 1) / udfBlock) }

// metaRef is the partition reference of the file set, entries and
// folders: the metadata partition's for 2.50, else the physical one's.
func (b *udfBuilder) metaRef() uint16 {
	if b.layout.metadata {
		return 1
	}
	return 0
}

// metaBlock is where metadata block n is in the image.
func (b *udfBuilder) metaBlock(n uint32) []byte {
	at := 257 + n
	if b.layout.metadata {
		at += 2
	}
	return b.block(at)
}

func (b *udfBuilder) block(n uint32) []byte {
	return b.img[int64(n)*udfBlock : int64(n+1)*udfBlock]
}

// folder writes a folder's file identifiers: its parent, then its
// children.
func (b *udfBuilder) folder(dir string) []byte {
	parent := path.Dir(dir)
	if parent == "." {
		parent = ""
	}
	data := fileID(nil, "", 8|2, b.entries[parent], b.metaRef())
	for _, child := range b.children[dir] {
		_, isDir := b.children[child]
		traits := byte(0)
		if isDir {
			traits = 2
		}
		data = fileID(data, path.Base(child), traits, b.entries[child], b.metaRef())
	}
	return data
}

// fileID appends a file identifier descriptor.
func fileID(data []byte, name string, traits byte, entry uint32, ref uint16) []byte {
	var id []byte
	if name != "" {
		id = append([]byte{8}, name...)
	}
	d := make([]byte, (38+len(id)+3)&^3)
	binary.LittleEndian.PutUint16(d[16:], 1)
	d[18], d[19] = traits, byte(len(id))
	binary.LittleEndian.PutUint32(d[20:], udfBlock)
	binary.LittleEndian.PutUint32(d[24:], entry)
	binary.LittleEndian.PutUint16(d[28:], ref)
	copy(d[38:], id)
	tag(d, udfFileID, entry, 38+len(id))
	return append(data, d...)
}

// tag writes a descriptor's tag: its identifier, version, location, CRC
// over length bytes after the tag, and checksum.
func tag(d []byte, id uint16, location uint32, length int) {
	binary.LittleEndian.PutUint16(d, id)
	binary.LittleEndian.PutUint16(d[2:], 3)
	binary.LittleEndian.PutUint16(d[8:], crc16(d[16:16+length-16]))
	binary.LittleEndian.PutUint16(d[10:], uint16(length-16))
	binary.LittleEndian.PutUint32(d[12:], location)
	var sum byte
	for i, v := range d[:16] {
		if i != 4 {
			sum += v
		}
	}
	d[4] = sum
}

// crc16 is the CRC-ITU-T of descriptors.
func crc16(data []byte) uint16 {
	var crc uint16
	for _, v := range data {
		crc ^= uint16(v) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// entity writes an entity identifier at d.
func entity(d []byte, id string, revision uint16) {
	copy(d[1:24], id)
	binary.LittleEndian.PutUint16(d[24:], revision)
}

// dstring writes a fixed-length string field.
func dstring(d []byte, s string) {
	d[0] = 8
	copy(d[1:], s)
	d[len(d)-1] = byte(1 + len(s))
}

func (b *udfBuilder) write(total uint32) {
	revision, nsr := uint16(0x0201), "NSR02"
	if b.layout.metadata {
		revision, nsr = 0x0250, "NSR03"
	}
	for i, id := range []string{"BEA01", nsr, "TEA01"} {
		d := b.block(uint32(16 + i))
		copy(d[1:], id)
		d[6] = 1
	}
	// The volume descriptor sequences, at blocks 32 and 48, and the
	// integrity descriptor at 64.
	for _, at := range []uint32{32, 48} {
		pvd := b.block(at)
		dstring(pvd[24:56], "MOVIE_BD")
		binary.LittleEndian.PutUint16(pvd[56:], 1)
		binary.LittleEndian.PutUint16(pvd[58:], 1)
		binary.LittleEndian.PutUint16(pvd[60:], 2)
		binary.LittleEndian.PutUint16(pvd[62:], 3)
		dstring(pvd[72:200], "MOVIE_BD")
		tag(pvd, 1, at, 512)
		pd := b.block(at + 1)
		binary.LittleEndian.PutUint32(pd[16:], 1)
		binary.LittleEndian.PutUint16(pd[20:], 1)
		entity(pd[24:56], "+"+nsr, 0)
		binary.LittleEndian.PutUint32(pd[184:], 1)
		binary.LittleEndian.PutUint32(pd[188:], 257)
		binary.LittleEndian.PutUint32(pd[192:], b.physical)
		tag(pd, udfPartition, at+1, 512)
		lvd := b.block(at + 2)
		binary.LittleEndian.PutUint32(lvd[16:], 2)
		copy(lvd[20:], "\x00OSTA Compressed Unicode")
		dstring(lvd[84:212], "MOVIE_BD")
		binary.LittleEndian.PutUint32(lvd[212:], udfBlock)
		entity(lvd[216:248], "*OSTA UDF Compliant", revision)
		binary.LittleEndian.PutUint32(lvd[248:], udfBlock)
		binary.LittleEndian.PutUint16(lvd[256:], b.metaRef())
		maps := []byte{1, 6, 1, 0, 0, 0}
		if b.layout.partition != "" {
			maps = make([]byte, 64)
			maps[0], maps[1] = 2, 64
			entity(maps[4:36], b.layout.partition, revision)
			binary.LittleEndian.PutUint16(maps[36:], 1)
		}
		if b.layout.metadata {
			meta := make([]byte, 64)
			meta[0], meta[1] = 2, 64
			entity(meta[4:36], "*UDF Metadata Partition", revision)
			binary.LittleEndian.PutUint16(meta[36:], 1)
			binary.LittleEndian.PutUint32(meta[40:], 0)
			binary.LittleEndian.PutUint32(meta[44:], 1)
			binary.LittleEndian.PutUint32(meta[48:], 0xFFFFFFFF)
			binary.LittleEndian.PutUint32(meta[52:], 32)
			binary.LittleEndian.PutUint16(meta[56:], 1)
			maps = append(maps, meta...)
		}
		binary.LittleEndian.PutUint32(lvd[264:], uint32(len(maps)))
		binary.LittleEndian.PutUint32(lvd[268:], uint32(1+b.metaRef()))
		entity(lvd[272:304], "*Polyfin tests", 0)
		binary.LittleEndian.PutUint32(lvd[432:], udfBlock)
		binary.LittleEndian.PutUint32(lvd[436:], 64)
		copy(lvd[440:], maps)
		tag(lvd, udfLogical, at+2, 440+len(maps))
		usd := b.block(at + 3)
		binary.LittleEndian.PutUint32(usd[16:], 3)
		tag(usd, 7, at+3, 24)
		tag(b.block(at+4), udfTerminating, at+4, 512)
	}
	lvid := b.block(64)
	binary.LittleEndian.PutUint32(lvid[28:], 1)
	parts := 1 + int(b.metaRef())
	binary.LittleEndian.PutUint32(lvid[72:], uint32(parts))
	binary.LittleEndian.PutUint32(lvid[76:], 46)
	use := lvid[80+8*parts:]
	entity(use[:32], "*Polyfin tests", 0)
	binary.LittleEndian.PutUint32(use[32:], uint32(len(b.files)))
	binary.LittleEndian.PutUint32(use[36:], uint32(len(b.children)))
	binary.LittleEndian.PutUint16(use[40:], revision)
	binary.LittleEndian.PutUint16(use[42:], revision)
	binary.LittleEndian.PutUint16(use[44:], revision)
	tag(lvid, 9, 64, 80+8*parts+46)
	for _, at := range []uint32{256, total - 1} {
		avdp := b.block(at)
		binary.LittleEndian.PutUint32(avdp[16:], 16*udfBlock)
		binary.LittleEndian.PutUint32(avdp[20:], 32)
		binary.LittleEndian.PutUint32(avdp[24:], 16*udfBlock)
		binary.LittleEndian.PutUint32(avdp[28:], 48)
		tag(avdp, udfAnchor, at, 512)
	}
	// The metadata file and its mirror, at the partition's start.
	if b.layout.metadata {
		for i, kind := range []byte{250, 251} {
			fe := b.block(257 + uint32(i))
			ad := binary.LittleEndian.AppendUint32(nil, b.meta*udfBlock)
			ad = binary.LittleEndian.AppendUint32(ad, 2)
			b.fileEntry(fe, uint32(i), kind, int64(b.meta)*udfBlock, 0, ad)
		}
	}
	// The file set descriptor and its terminator.
	fsd := b.metaBlock(0)
	binary.LittleEndian.PutUint16(fsd[28:], 3)
	binary.LittleEndian.PutUint16(fsd[30:], 3)
	binary.LittleEndian.PutUint32(fsd[32:], 1)
	binary.LittleEndian.PutUint32(fsd[36:], 1)
	copy(fsd[48:], "\x00OSTA Compressed Unicode")
	dstring(fsd[112:240], "MOVIE_BD")
	copy(fsd[240:], "\x00OSTA Compressed Unicode")
	dstring(fsd[304:336], "MOVIE_BD")
	binary.LittleEndian.PutUint32(fsd[400:], udfBlock)
	binary.LittleEndian.PutUint32(fsd[404:], b.entries[""])
	binary.LittleEndian.PutUint16(fsd[408:], b.metaRef())
	entity(fsd[416:448], "*OSTA UDF Compliant", revision)
	tag(fsd, udfFileSet, 0, 512)
	tag(b.metaBlock(1), udfTerminating, 1, 512)
	// The folders: their entries, their identifiers in the metadata
	// blocks, short allocation descriptors.
	for dir := range b.children {
		data := b.folder(dir)
		at := b.data[dir]
		for i := uint32(0); i < blocks(int64(len(data))); i++ {
			copy(b.metaBlock(at+i), data[min(int(i)*udfBlock, len(data)):])
		}
		ad := binary.LittleEndian.AppendUint32(nil, uint32(len(data)))
		ad = binary.LittleEndian.AppendUint32(ad, at)
		b.fileEntry(b.metaBlock(b.entries[dir]), b.entries[dir], 4, int64(len(data)), 0, ad)
	}
	// The files: their bytes in the physical partition, long allocation
	// descriptors for 2.50, which reach it from the metadata partition.
	for name, data := range b.files {
		at := b.data[name]
		copy(b.img[int64(257+at)*udfBlock:], data)
		var ad []byte
		kind := uint16(0)
		ad = binary.LittleEndian.AppendUint32(ad, uint32(len(data)))
		ad = binary.LittleEndian.AppendUint32(ad, at)
		if b.layout.metadata {
			kind = 1
			ad = append(ad, make([]byte, 8)...)
		}
		b.fileEntry(b.metaBlock(b.entries[name]), b.entries[name], 5, int64(len(data)), kind, ad)
	}
}

// fileEntry writes a file entry, extended for 2.50, of a file of type
// kind and size bytes at location, its allocation descriptors ads of
// type adType.
func (b *udfBuilder) fileEntry(d []byte, location uint32, kind byte, size int64, adType uint16, ads []byte) {
	base, id := 176, uint16(udfFileEntry)
	if b.layout.metadata {
		base, id = 216, udfExtended
	}
	binary.LittleEndian.PutUint16(d[20:], 4)
	binary.LittleEndian.PutUint16(d[24:], 1)
	d[27] = kind
	binary.LittleEndian.PutUint16(d[34:], adType)
	binary.LittleEndian.PutUint32(d[36:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(d[40:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(d[44:], 0x14A5)
	binary.LittleEndian.PutUint16(d[48:], 1)
	binary.LittleEndian.PutUint64(d[56:], uint64(size))
	if b.layout.metadata {
		binary.LittleEndian.PutUint64(d[64:], uint64(size))
		binary.LittleEndian.PutUint64(d[72:], uint64(blocks(size)))
		binary.LittleEndian.PutUint32(d[128:], 1)
		entity(d[168:200], "*Polyfin tests", 0)
	} else {
		binary.LittleEndian.PutUint64(d[64:], uint64(blocks(size)))
		binary.LittleEndian.PutUint32(d[108:], 1)
		entity(d[128:160], "*Polyfin tests", 0)
	}
	binary.LittleEndian.PutUint32(d[base-4:], uint32(len(ads)))
	copy(d[base:], ads)
	tag(d, id, location, base+len(ads))
}

// blurayFiles reads the files of the test Blu-ray disc, by path, those
// of its BDMV folder.
func blurayFiles(t *testing.T) map[string][]byte {
	t.Helper()
	dir := t.TempDir()
	unpackBluRay(t, dir)
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		data, err := os.ReadFile(p)
		files[filepath.ToSlash(rel)] = data
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// writeUDF writes the image of files, laid out as layout, at path.
func writeUDF(t *testing.T, path string, files map[string][]byte, layout udfLayout) {
	t.Helper()
	write(t, path, 0)
	if err := os.WriteFile(path, buildUDF(files, layout), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The UDF file system of an image without an ISO 9660 one is read, UDF
// 2.50 through its metadata partition, 2.01 without: its folders, any
// case, and its files, whose bytes are those written.
func TestUDFImagesAreRead(t *testing.T) {
	files := blurayFiles(t)
	for name, layout := range map[string]udfLayout{"2.50": {metadata: true}, "2.01": {}} {
		image := buildUDF(files, layout)
		if _, err := openISO(readerOf(image)); err == nil {
			t.Fatalf("%s: an ISO 9660 file system", name)
		}
		fsys, err := openDiscFS(readerOf(image))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, ok := fsys.(*udfImage); !ok {
			t.Fatalf("%s: read as %T", name, fsys)
		}
		got, err := fsys.names("bdmv", "Playlist")
		slices.Sort(got)
		if err != nil || !slices.Equal(got, []string{"00000.mpls", "00001.mpls"}) {
			t.Errorf("%s: playlists %v %v", name, got, err)
		}
		for _, rel := range []string{"BDMV/STREAM/00001.m2ts", "BDMV/CLIPINF/00000.clpi"} {
			file, err := fsys.file(strings.Split(rel, "/")...)
			if err != nil {
				t.Fatalf("%s: %s: %v", name, rel, err)
			}
			data, err := io.ReadAll(file)
			if err != nil || !slices.Equal(data, files[rel]) {
				t.Errorf("%s: %s: %d bytes, %v", name, rel, len(data), err)
			}
		}
		if _, err := fsys.file("BDMV", "STREAM"); err == nil {
			t.Errorf("%s: a folder was read as a file", name)
		}
		if _, err := fsys.names("BDMV", "MISSING"); err == nil {
			t.Errorf("%s: a missing folder was listed", name)
		}
	}
}

func readerOf(data []byte) io.ReaderAt { return strings.NewReader(string(data)) }

// A broken or hostile image fails, at once, without reading or holding
// more than a few MB: a folder that is its own child, a huge folder, a
// file larger than the image, extents out of the partition or chained to
// themselves, a metadata partition whose file cannot be read, a cut image,
// no anchor; and images with random bytes overwritten neither panic nor
// hang.
func TestHostileUDFImagesFail(t *testing.T) {
	disc := blurayFiles(t)
	files := map[string][]byte{
		"BDMV/PLAYLIST/00000.mpls": disc["BDMV/PLAYLIST/00000.mpls"],
		"BDMV/STREAM/00000.m2ts":   make([]byte, 3*udfBlock),
		"BDMV/STREAM/00001.m2ts":   make([]byte, 3*udfBlock),
	}
	b := &udfBuilder{layout: udfLayout{metadata: true}, files: files, children: map[string][]string{"": nil}}
	good := b.build()
	metaBlock := func(img []byte, n uint32) []byte {
		return img[int64(257+2+n)*udfBlock:][:udfBlock]
	}
	retag := func(d []byte) {
		tag(d, binary.LittleEndian.Uint16(d), binary.LittleEndian.Uint32(d[12:]), int(binary.LittleEndian.Uint16(d[10:]))+16)
	}
	// fails reads the image's playlists and a stream: one fails.
	fails := func(img []byte) bool {
		fsys, err := openDiscFS(readerOf(img))
		if err != nil {
			return true
		}
		_, err = imagePlaylists(fsys)
		_, fileErr := fsys.file("BDMV", "STREAM", "00000.m2ts")
		return err != nil || fileErr != nil
	}
	if fsys, err := openDiscFS(readerOf(good)); err != nil || fails(good) {
		t.Fatalf("the image itself: %v", err)
	} else if titles, err := imagePlaylists(fsys); !slices.Equal(titles, []int{0}) {
		t.Fatalf("the image's titles: %v %v", titles, err)
	}
	for name, mutate := range map[string]func(img []byte) []byte{
		"a folder its own child": func(img []byte) []byte {
			// BDMV's identifier, after the root's parent's, names the
			// root's entry.
			fid := metaBlock(img, b.data[""])[40:]
			binary.LittleEndian.PutUint32(fid[24:], b.entries[""])
			retag(fid)
			return img
		},
		"a huge folder": func(img []byte) []byte {
			d := metaBlock(img, b.entries["BDMV"])
			binary.LittleEndian.PutUint64(d[56:], 1<<40)
			retag(d)
			return img
		},
		"a file larger than the image": func(img []byte) []byte {
			d := metaBlock(img, b.entries["BDMV/STREAM/00000.m2ts"])
			binary.LittleEndian.PutUint64(d[56:], 1<<45)
			binary.LittleEndian.PutUint32(d[216:], 0x3FFFF800)
			retag(d)
			return img
		},
		"an extent out of the partition": func(img []byte) []byte {
			d := metaBlock(img, b.entries["BDMV/PLAYLIST"])
			binary.LittleEndian.PutUint32(d[220:], 0xFFFFFF00)
			retag(d)
			return img
		},
		"extents chained to themselves": func(img []byte) []byte {
			// The folder's descriptor leads to an allocation extent
			// descriptor, in metadata block 1, that leads to itself.
			d := metaBlock(img, b.entries["BDMV/PLAYLIST"])
			binary.LittleEndian.PutUint32(d[216:], 3<<30|udfBlock)
			binary.LittleEndian.PutUint32(d[220:], 1)
			retag(d)
			aed := metaBlock(img, 1)
			clear(aed)
			binary.LittleEndian.PutUint32(aed[20:], 8)
			binary.LittleEndian.PutUint32(aed[24:], 3<<30|udfBlock)
			binary.LittleEndian.PutUint32(aed[28:], 1)
			tag(aed, udfExtension, 1, 32)
			return img
		},
		"no metadata file": func(img []byte) []byte {
			clear(img[257*udfBlock : 259*udfBlock])
			return img
		},
		"cut short": func(img []byte) []byte { return img[:262*udfBlock] },
		"no anchor": func(img []byte) []byte {
			clear(img[256*udfBlock : 257*udfBlock])
			return img
		},
	} {
		img := mutate(slices.Clone(good))
		start := time.Now()
		if !fails(img) {
			t.Errorf("%s: read", name)
		}
		// A broken image is no layout FFmpeg would be asked to read.
		if _, _, _, err := readDisc(readerOf(img)); errors.Is(err, errUnsupportedUDF) {
			t.Errorf("%s: %v", name, err)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("%s: %v", name, elapsed)
		}
	}
	random := rand.New(rand.NewPCG(1, 2))
	for range 300 {
		img := slices.Clone(good)
		for range 1 + random.IntN(8) {
			block := []int{16, 17, 32, 33, 34, 256, 257, 258, 259, 260}[random.IntN(10)] + random.IntN(12)
			img[block*udfBlock+random.IntN(udfBlock)] = byte(random.Uint32())
		}
		start := time.Now()
		fails(img)
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("a random image took %v", elapsed)
		}
	}
}

// testdata/bluray-udf.iso.gz is the image buildUDF writes of the test
// Blu-ray disc, UDF 2.50 with a metadata partition, for the tests of other
// packages; POLYFIN_UPDATE_FIXTURES=1 writes it again.
func TestUDFFixtureIsTheBuildersImage(t *testing.T) {
	image := buildUDF(blurayFiles(t), udfLayout{metadata: true})
	fixture := filepath.Join("testdata", "bluray-udf.iso.gz")
	if os.Getenv("POLYFIN_UPDATE_FIXTURES") == "1" {
		var packed bytes.Buffer
		zipped, _ := gzip.NewWriterLevel(&packed, gzip.BestCompression)
		if _, err := zipped.Write(image); err != nil || zipped.Close() != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fixture, packed.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	unpackImage(t, "bluray-udf", filepath.Join(dir, "image.iso"))
	data, err := os.ReadFile(filepath.Join(dir, "image.iso"))
	if err != nil || !bytes.Equal(data, image) {
		t.Errorf("the fixture is not the builder's image: %d bytes, %v", len(data), err)
	}
}
