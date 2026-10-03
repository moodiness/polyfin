package container

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fetcher serves a file in memory to SubtitleBlocks, counting the requests
// and how many run at once, each taking delay.
type fetcher struct {
	data  []byte
	delay time.Duration

	mu            sync.Mutex
	calls         int
	running, most int
	fetched       int64
}

func (f *fetcher) Fetch(ctx context.Context, off int64, n int) ([]byte, error) {
	f.mu.Lock()
	f.calls++
	f.running++
	f.most = max(f.most, f.running)
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.running--
		f.mu.Unlock()
	}()
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if off < 0 || n < 0 || off > int64(len(f.data)) {
		return nil, errors.New("fetch out of the file")
	}
	end := min(off+int64(n), int64(len(f.data)))
	f.mu.Lock()
	f.fetched += end - off
	f.mu.Unlock()
	return slices.Clone(f.data[off:end]), nil
}

func openMemory(t *testing.T, data []byte) *Matroska {
	t.Helper()
	m, err := OpenMatroska(context.Background(), &memory{data: data}, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMatroskaSubtitles(t *testing.T) {
	data := fixture(t, "subtitles.mkv")
	r := &memory{data: data}
	m, err := OpenMatroska(context.Background(), r, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	header := fixture(t, "subtitles.ass")
	header = header[:bytes.Index(header, []byte("Dialogue:"))]
	tracks := m.Tracks()
	if len(tracks) != 3 {
		t.Fatalf("%d tracks", len(tracks))
	}
	want := []Track{
		{Number: 1, Type: 1, CodecID: "V_MPEG4/ISO/AVC", Language: "und", Decodable: true},
		{Number: 2, Type: 0x11, CodecID: "S_TEXT/UTF8", Language: "fre", Name: "Français", Decodable: true},
		{Number: 3, Type: 0x11, CodecID: "S_TEXT/ASS", Language: "eng", Default: true, Forced: true, Decodable: true,
			CodecPrivate: header},
	}
	for i, track := range tracks {
		w := want[i]
		if i == 0 {
			w.CodecPrivate = track.CodecPrivate
		}
		// FFmpeg writes the ASS header with its own line breaks.
		track.CodecPrivate = bytes.TrimRight(track.CodecPrivate, "\n")
		w.CodecPrivate = bytes.TrimRight(w.CodecPrivate, "\n")
		if track.Number != w.Number || track.Type != w.Type || track.CodecID != w.CodecID || track.Language != w.Language ||
			track.Name != w.Name || track.Default != w.Default || track.Forced != w.Forced || track.Decodable != w.Decodable ||
			!bytes.Equal(track.CodecPrivate, w.CodecPrivate) {
			t.Errorf("track %d:\n got %+v\nwant %+v", i, track, w)
		}
	}

	locations, err := m.SubtitleLocations(context.Background())
	if err != nil || !maps.Equal(locations, map[uint64]int{2: 7, 3: 5}) {
		t.Fatalf("locations %v, %v", locations, err)
	}

	ms := time.Millisecond
	for _, test := range []struct {
		track uint64
		want  []Block
	}{
		{2, []Block{
			{500 * ms, 1300 * ms, []byte("Bonjour.")},
			{2000 * ms, 1250 * ms, []byte("Deux lignes,\nune seconde.")},
			{3400 * ms, 600 * ms, []byte("<i>En italique.</i>")},
			{6100 * ms, 1800 * ms, []byte("Après une pause.")},
			{8000 * ms, 500 * ms, []byte("Vite.")},
			{8600 * ms, 500 * ms, []byte("Encore.")},
			{10250 * ms, 1500 * ms, []byte("La fin.")},
		}},
		{3, []Block{
			{500 * ms, 1500 * ms, []byte("0,0,Default,,0,0,0,,Hello.")},
			{1000 * ms, 3000 * ms, []byte(`1,0,Sign,,0,0,0,,{\pos(320,40)}A sign`)},
			{2500 * ms, 1250 * ms, []byte(`2,0,Default,Narrator,0,0,0,,{\i1}Two lines{\i0},\Nthe second.`)},
			{6100 * ms, 1800 * ms, []byte(`3,1,Default,,0,0,0,,After\ha pause.`)},
			{10250 * ms, 1500 * ms, []byte("4,0,Default,,0,0,0,,The end.")},
		}},
	} {
		f := &fetcher{data: data}
		blocks, err := m.SubtitleBlocks(context.Background(), f, test.track)
		if err != nil {
			t.Fatalf("track %d: %v", test.track, err)
		}
		checkBlocks(t, blocks, test.want)
		// A Cluster's header, the Cluster checked, and the blocks, close
		// together.
		if f.calls != 3 {
			t.Errorf("track %d: %d requests, want 3", test.track, f.calls)
		}
	}

	attachments, err := m.Attachments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 1 {
		t.Fatalf("%d attachments", len(attachments))
	}
	a := attachments[0]
	if a.FileName != "Dummy.ttf" || a.MimeType != "application/x-truetype-font" || a.Description != "" || a.UID == 0 ||
		!bytes.Equal(a.Data, fixture(t, "Dummy.ttf")) {
		t.Errorf("attachment %+v", a)
	}
	// The head, the Cues and the Attachments all came with the first read.
	if r.calls != 1 {
		t.Errorf("%d reads", r.calls)
	}

	for _, number := range []uint64{1, 4} {
		if _, err := m.SubtitleBlocks(context.Background(), &fetcher{data: data}, number); err == nil {
			t.Errorf("track %d read as subtitles", number)
		}
	}
}

func checkBlocks(t *testing.T, got, want []Block) {
	t.Helper()
	equal := len(got) == len(want)
	for i := 0; equal && i < len(got); i++ {
		equal = got[i].Start == want[i].Start && got[i].Duration == want[i].Duration && bytes.Equal(got[i].Data, want[i].Data)
	}
	if !equal {
		var b strings.Builder
		for _, block := range got {
			b.WriteString("\n" + block.Start.String() + " " + block.Duration.String() + " " + string(block.Data))
		}
		t.Errorf("got %d blocks:%s\nwant %d", len(got), b.String(), len(want))
	}
}

func TestMatroskaOpenNotMatroska(t *testing.T) {
	for _, name := range []string{"tail.mp4", "README.md"} {
		data := fixture(t, name)
		if _, err := OpenMatroska(context.Background(), &memory{data: data}, int64(len(data))); !errors.Is(err, ErrNoIndex) {
			t.Errorf("%s: got %v, want ErrNoIndex", name, err)
		}
	}
}

// testFile is a Matroska file of subtitle tracks, built for a test: the
// Cues list the blocks of its Clusters, but for those marked unlisted.
type testFile struct {
	tracks   [][]byte
	clusters []testCluster
	// attachments come before the Clusters, or after the Cues when
	// attachmentsLast.
	attachments     []byte
	attachmentsLast bool
	// extraCues are CuePoints added to those listing the blocks;
	// hiddenCues leaves the Cues out of the SeekHead.
	extraCues  [][]byte
	hiddenCues bool
}

type testCluster struct {
	// sizeLength is the length of the Cluster's size, 8 when zero, and
	// unknownSize has it unknown; padding is the bytes before its blocks.
	sizeLength  int
	unknownSize bool
	padding     int
	blocks      []testBlock
	// raw is appended to the Cluster's data, after the blocks, and listed
	// as a block of rawTrack when it is not zero.
	raw      []byte
	rawTrack uint64
}

type testBlock struct {
	// track is the block's track, and cueTrack, when not zero, the one
	// the Cues list it for.
	track, cueTrack uint64
	// time is the block's time in milliseconds; duration, when not zero,
	// its BlockDuration, which a BlockGroup holds, and cueDuration its
	// CueDuration.
	time, duration, cueDuration uint64
	group                       bool
	flags                       byte
	data                        []byte
	unlisted                    bool
}

func (tf testFile) build() []byte {
	info := ebmlElement(idInfo, ebmlUint(idTimestampScale, 1_000_000))
	tracks := ebmlElement(idTracks, tf.tracks...)
	seekHead := func(info, tracks, cues, attachments int64) []byte {
		seeks := [][]byte{seek(idInfo, info), seek(idTracks, tracks)}
		if !tf.hiddenCues {
			seeks = append(seeks, seek(idCues, cues))
		}
		if tf.attachments != nil {
			seeks = append(seeks, seek(idAttachments, attachments))
		}
		return ebmlElement(idSeekHead, seeks...)
	}
	infoAt := int64(len(seekHead(0, 0, 0, 0)))
	tracksAt := infoAt + int64(len(info))
	attachmentsAt := tracksAt + int64(len(tracks))
	head := tf.attachments
	if tf.attachmentsLast {
		head = nil
	}
	pos := attachmentsAt + int64(len(head))
	var clusters []byte
	points := slices.Clone(tf.extraCues)
	for _, c := range tf.clusters {
		payload := ebmlUint(0xE7, 0)
		if c.padding > 0 {
			payload = append(payload, ebmlElement(idVoid, make([]byte, c.padding))...)
		}
		for _, b := range c.blocks {
			relative := len(payload)
			payload = append(payload, b.encode()...)
			if !b.unlisted {
				points = append(points, testCuePoint(orDefault(b.cueTrack, b.track), b.time, b.cueDuration, pos, relative))
			}
		}
		if c.rawTrack != 0 {
			points = append(points, testCuePoint(c.rawTrack, 0, 0, pos, len(payload)))
		}
		payload = append(payload, c.raw...)
		cluster := sized(idCluster, orDefault(c.sizeLength, 8), payload)
		if c.unknownSize {
			cluster = slices.Concat(idBytes(idCluster), []byte{0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, payload)
		}
		clusters = append(clusters, cluster...)
		pos += int64(len(cluster))
	}
	cues := ebmlElement(idCues, points...)
	if !tf.attachmentsLast {
		return matroskaFile(seekHead(infoAt, tracksAt, pos, attachmentsAt), info, tracks, head, clusters, cues)
	}
	attachmentsAt = pos + int64(len(cues))
	return matroskaFile(seekHead(infoAt, tracksAt, pos, attachmentsAt), info, tracks, clusters, cues, tf.attachments)
}

func testCuePoint(track, time, duration uint64, cluster int64, relative int) []byte {
	positions := [][]byte{ebmlUint(idCueTrack, track), ebmlUint(idCueClusterPosition, uint64(cluster)),
		ebmlUint(idCueRelativePosition, uint64(relative))}
	if duration > 0 {
		positions = append(positions, ebmlUint(idCueDuration, duration))
	}
	return ebmlElement(idCuePoint, ebmlUint(idCueTime, time), ebmlElement(idCueTrackPositions, positions...))
}

func orDefault[T comparable](value, otherwise T) T {
	var zero T
	if value == zero {
		return otherwise
	}
	return value
}

func (b testBlock) encode() []byte {
	block := slices.Concat([]byte{0x80 | byte(b.track)}, binary.BigEndian.AppendUint16(nil, uint16(b.time)), []byte{b.flags}, b.data)
	if !b.group {
		return sized(idSimpleBlock, 2, block)
	}
	children := [][]byte{sized(idBlock, 2, block)}
	if b.duration > 0 {
		children = append(children, ebmlUint(idBlockDuration, b.duration))
	}
	return sized(idBlockGroup, 2, children...)
}

// sized encodes an element with a size of the given length.
func sized(id uint32, length int, children ...[]byte) []byte {
	payload := slices.Concat(children...)
	size := binary.BigEndian.AppendUint64(nil, uint64(len(payload))|1<<(7*length))
	return slices.Concat(idBytes(id), size[8-length:], payload)
}

func subtitleTrack(number uint64, codec string, private []byte, more ...[]byte) []byte {
	children := [][]byte{ebmlUint(idTrackNumber, number), ebmlUint(idTrackType, trackTypeSubtitle), ebmlElement(idCodecID, []byte(codec))}
	if private != nil {
		children = append(children, ebmlElement(idCodecPrivate, private))
	}
	return ebmlElement(idTrackEntry, append(children, more...)...)
}

func contentEncodings(encodings ...[]byte) []byte {
	return ebmlElement(idContentEncodings, encodings...)
}

func contentEncodingElement(order, scope, kind uint64, compression ...[]byte) []byte {
	children := [][]byte{ebmlUint(idContentOrder, order), ebmlUint(idContentScope, scope), ebmlUint(idContentType, kind)}
	if compression != nil {
		children = append(children, ebmlElement(idContentCompression, compression...))
	}
	return ebmlElement(idContentEncoding, children...)
}

func deflate(data []byte) []byte {
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	_, _ = w.Write(data)
	_ = w.Close()
	return b.Bytes()
}

// groupBlock is a block in a BlockGroup, with a BlockDuration.
func groupBlock(track uint64, time, duration uint64, data string) testBlock {
	return testBlock{track: track, time: time, duration: duration, group: true, data: []byte(data)}
}

func TestMatroskaTrackDescriptions(t *testing.T) {
	private := []byte("[Script Info]\nScriptType: v4.00+\n")
	data := testFile{tracks: [][]byte{
		subtitleTrack(1, "S_TEXT/ASS", deflate(private),
			ebmlElement(idLanguage, []byte("fre\x00")), ebmlElement(idLanguageBCP47, []byte("fr-CA")),
			ebmlElement(idName, []byte("Signs")), ebmlUint(idFlagDefault, 0), ebmlUint(idFlagForced, 1),
			contentEncodings(contentEncodingElement(0, scopeFrames|scopePrivate, 0, ebmlUint(idContentCompAlgo, 0)))),
		subtitleTrack(2, "S_TEXT/UTF8", nil, ebmlElement(idLanguage, []byte("jpn"))),
		// Encrypted blocks are not decodable.
		subtitleTrack(3, "S_TEXT/UTF8", nil, contentEncodings(contentEncodingElement(0, scopeFrames, 1))),
		// Nor are blocks compressed with bzlib.
		subtitleTrack(4, "S_TEXT/UTF8", nil,
			contentEncodings(contentEncodingElement(0, scopeFrames, 0, ebmlUint(idContentCompAlgo, 1)))),
		// A CodecPrivate inflating to more than 16 MiB is not decoded.
		subtitleTrack(5, "S_TEXT/ASS", deflate(make([]byte, 17<<20)),
			contentEncodings(contentEncodingElement(0, scopePrivate, 0, ebmlUint(idContentCompAlgo, 0)))),
		// Header stripping applies to the CodecPrivate in its scope.
		subtitleTrack(6, "S_TEXT/ASS", []byte("Info]\n"),
			contentEncodings(contentEncodingElement(0, scopePrivate, 0, ebmlUint(idContentCompAlgo, 3), ebmlElement(idContentCompSettings, []byte("[Script "))))),
	}}.build()
	tracks := openMemory(t, data).Tracks()
	want := []Track{
		{Number: 1, Type: 0x11, CodecID: "S_TEXT/ASS", CodecPrivate: private, Language: "fr-CA", Name: "Signs", Forced: true, Decodable: true},
		{Number: 2, Type: 0x11, CodecID: "S_TEXT/UTF8", Language: "jpn", Default: true, Decodable: true},
		{Number: 3, Type: 0x11, CodecID: "S_TEXT/UTF8", Language: "eng", Default: true},
		{Number: 4, Type: 0x11, CodecID: "S_TEXT/UTF8", Language: "eng", Default: true},
		{Number: 5, Type: 0x11, CodecID: "S_TEXT/ASS", Language: "eng", Default: true},
		{Number: 6, Type: 0x11, CodecID: "S_TEXT/ASS", CodecPrivate: []byte("[Script Info]\n"), Language: "eng", Default: true, Decodable: true},
	}
	if len(tracks) != len(want) {
		t.Fatalf("%d tracks", len(tracks))
	}
	for i, track := range tracks {
		if i == 4 {
			track.CodecPrivate = nil
		}
		w := want[i]
		if track.Number != w.Number || track.Type != w.Type || track.CodecID != w.CodecID || track.Language != w.Language ||
			track.Name != w.Name || track.Default != w.Default || track.Forced != w.Forced || track.Decodable != w.Decodable ||
			!bytes.Equal(track.CodecPrivate, w.CodecPrivate) {
			t.Errorf("track %d:\n got %+v\nwant %+v", i+1, track, w)
		}
	}
}

func TestMatroskaSubtitleBlocks(t *testing.T) {
	ms := time.Millisecond
	zlibTrack := subtitleTrack(1, "S_TEXT/UTF8", nil,
		contentEncodings(contentEncodingElement(0, scopeFrames, 0, ebmlUint(idContentCompAlgo, 0))))
	strippedTrack := subtitleTrack(1, "S_TEXT/ASS", nil,
		contentEncodings(contentEncodingElement(0, scopeFrames, 0, ebmlUint(idContentCompAlgo, 3), ebmlElement(idContentCompSettings, []byte("0,0,Default,,")))))
	// Both encodings, undone from the highest order: the frames were
	// stripped, then compressed.
	bothTrack := subtitleTrack(1, "S_TEXT/UTF8", nil, contentEncodings(
		contentEncodingElement(0, scopeFrames, 0, ebmlUint(idContentCompAlgo, 3), ebmlElement(idContentCompSettings, []byte("<i>"))),
		contentEncodingElement(1, scopeFrames, 0, ebmlUint(idContentCompAlgo, 0))))
	plain := subtitleTrack(1, "S_TEXT/UTF8", nil)

	tests := []struct {
		name string
		file testFile
		want []Block
	}{
		{"zlib", testFile{tracks: [][]byte{zlibTrack}, clusters: []testCluster{{blocks: []testBlock{
			{track: 1, time: 1000, duration: 500, group: true, data: deflate([]byte("Compressed."))},
			{track: 1, time: 2000, cueDuration: 700, data: deflate([]byte("Simple, with a CueDuration."))},
		}}}}, []Block{{1000 * ms, 500 * ms, []byte("Compressed.")}, {2000 * ms, 700 * ms, []byte("Simple, with a CueDuration.")}}},
		{"header stripping", testFile{tracks: [][]byte{strippedTrack}, clusters: []testCluster{{blocks: []testBlock{
			groupBlock(1, 1000, 500, "Hello."),
		}}}}, []Block{{1000 * ms, 500 * ms, []byte("0,0,Default,,Hello.")}}},
		{"both", testFile{tracks: [][]byte{bothTrack}, clusters: []testCluster{{blocks: []testBlock{
			{track: 1, time: 1000, duration: 500, group: true, data: deflate([]byte("Italic.</i>"))},
		}}}}, []Block{{1000 * ms, 500 * ms, []byte("<i>Italic.</i>")}}},
		// Blocks come sorted by time, ties in file order, whatever the
		// order of the Clusters; other tracks' blocks are skipped.
		{"order", testFile{tracks: [][]byte{plain, subtitleTrack(2, "S_TEXT/UTF8", nil)}, clusters: []testCluster{
			{blocks: []testBlock{groupBlock(1, 5000, 100, "Third."), groupBlock(2, 4000, 100, "Other track."), groupBlock(1, 5000, 100, "Fourth.")}},
			{blocks: []testBlock{groupBlock(1, 3000, 100, "Second."), groupBlock(1, 1000, 100, "First.")}},
		}}, []Block{
			{1000 * ms, 100 * ms, []byte("First.")}, {3000 * ms, 100 * ms, []byte("Second.")},
			{5000 * ms, 100 * ms, []byte("Third.")}, {5000 * ms, 100 * ms, []byte("Fourth.")},
		}},
		// The Cues are found after the last Cluster.
		{"Cues no SeekHead lists", testFile{tracks: [][]byte{plain}, hiddenCues: true, clusters: []testCluster{{blocks: []testBlock{
			groupBlock(1, 1000, 500, "Found."),
		}}}}, []Block{{1000 * ms, 500 * ms, []byte("Found.")}}},
		// The Cluster checked has the longest header; the others are read
		// again for theirs.
		{"Cluster headers of different lengths", testFile{tracks: [][]byte{plain}, clusters: []testCluster{
			{sizeLength: 2, blocks: []testBlock{groupBlock(1, 1000, 100, "One."), groupBlock(1, 2000, 100, "Two.")}},
			{sizeLength: 8, blocks: []testBlock{groupBlock(1, 3000, 100, "Three."), groupBlock(1, 4000, 100, "Four."), groupBlock(1, 5000, 100, "Five.")}},
			{sizeLength: 1, blocks: []testBlock{groupBlock(1, 6000, 100, "Six.")}},
			{sizeLength: 4, blocks: []testBlock{groupBlock(1, 7000, 100, "Seven.")}},
		}}, []Block{
			{1000 * ms, 100 * ms, []byte("One.")}, {2000 * ms, 100 * ms, []byte("Two.")}, {3000 * ms, 100 * ms, []byte("Three.")},
			{4000 * ms, 100 * ms, []byte("Four.")}, {5000 * ms, 100 * ms, []byte("Five.")}, {6000 * ms, 100 * ms, []byte("Six.")},
			{7000 * ms, 100 * ms, []byte("Seven.")},
		}},
		// The Cluster checked ends where the next begins.
		{"Cluster of unknown size", testFile{tracks: [][]byte{plain}, clusters: []testCluster{
			{unknownSize: true, blocks: []testBlock{groupBlock(1, 1000, 100, "One."), groupBlock(1, 2000, 100, "Two.")}},
			{blocks: []testBlock{groupBlock(1, 3000, 100, "Three.")}},
		}}, []Block{{1000 * ms, 100 * ms, []byte("One.")}, {2000 * ms, 100 * ms, []byte("Two.")}, {3000 * ms, 100 * ms, []byte("Three.")}}},
		// A block longer than the window read at its position is read
		// whole.
		{"long block", testFile{tracks: [][]byte{plain}, clusters: []testCluster{{blocks: []testBlock{
			groupBlock(1, 1000, 100, strings.Repeat("Long. ", 2000)), groupBlock(1, 2000, 100, "Short."),
		}}}}, []Block{{1000 * ms, 100 * ms, []byte(strings.Repeat("Long. ", 2000))}, {2000 * ms, 100 * ms, []byte("Short.")}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := test.file.build()
			m := openMemory(t, data)
			blocks, err := m.SubtitleBlocks(context.Background(), &fetcher{data: data}, 1)
			if err != nil {
				t.Fatal(err)
			}
			checkBlocks(t, blocks, test.want)
		})
	}
}

func TestMatroskaClusterHeadersAreLearned(t *testing.T) {
	data := testFile{tracks: [][]byte{subtitleTrack(1, "S_TEXT/UTF8", nil)}, clusters: []testCluster{
		{sizeLength: 8, blocks: []testBlock{groupBlock(1, 1000, 100, "One."), groupBlock(1, 2000, 100, "Two.")}},
		{sizeLength: 8, blocks: []testBlock{groupBlock(1, 3000, 100, "Three.")}},
		{sizeLength: 2, blocks: []testBlock{groupBlock(1, 4000, 100, "Four.")}},
		{sizeLength: 8, blocks: []testBlock{groupBlock(1, 5000, 100, "Five.")}},
	}}.build()
	f := &fetcher{data: data}
	blocks, err := openMemory(t, data).SubtitleBlocks(context.Background(), f, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 5 || string(blocks[3].Data) != "Four." {
		t.Fatalf("blocks %v", blocks)
	}
	// The first Cluster's header and data, the blocks together, and the
	// header of the one Cluster whose header is shorter.
	if f.calls != 4 {
		t.Errorf("%d requests, want 4", f.calls)
	}
}

func TestMatroskaSubtitleBlocksAreGrouped(t *testing.T) {
	track := subtitleTrack(1, "S_TEXT/UTF8", nil)
	var spread, packed []testCluster
	// A block every 150 KiB, as a film's, one per Cluster.
	for i := range 30 {
		spread = append(spread, testCluster{padding: 150 << 10, blocks: []testBlock{groupBlock(1, uint64(i)*1000, 500, "Spread.")}})
	}
	// Hundreds of blocks in a few Clusters, as typeset anime's.
	for i := range 3 {
		var blocks []testBlock
		for j := range 300 {
			blocks = append(blocks, groupBlock(1, uint64(i*300+j)*10, 5, "{\\pos(1,2)}Packed."))
		}
		packed = append(packed, testCluster{padding: 1 << 10, blocks: blocks})
	}
	for _, test := range []struct {
		name     string
		clusters []testCluster
		blocks   int
		calls    int
	}{
		// The check reads a Cluster's header and data, then each block is
		// read alone.
		{"spread", spread, 30, 2 + 30},
		{"packed", packed, 900, 2 + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := testFile{tracks: [][]byte{track}, clusters: test.clusters}.build()
			f := &fetcher{data: data, delay: 2 * time.Millisecond}
			blocks, err := openMemory(t, data).SubtitleBlocks(context.Background(), f, 1)
			if err != nil {
				t.Fatal(err)
			}
			if len(blocks) != test.blocks {
				t.Fatalf("%d blocks, want %d", len(blocks), test.blocks)
			}
			if f.calls != test.calls {
				t.Errorf("%d requests, want %d", f.calls, test.calls)
			}
			if f.most > maxFetches || test.calls > maxFetches+2 && f.most < 2 {
				t.Errorf("%d requests at once", f.most)
			}
			t.Logf("%d requests of %d bytes, at most %d at once", f.calls, f.fetched, f.most)
		})
	}
}

func TestMatroskaSubtitleBlocksFail(t *testing.T) {
	plain := subtitleTrack(1, "S_TEXT/UTF8", nil)
	zlibTrack := subtitleTrack(1, "S_TEXT/UTF8", nil,
		contentEncodings(contentEncodingElement(0, scopeFrames, 0, ebmlUint(idContentCompAlgo, 0))))
	encrypted := subtitleTrack(1, "S_TEXT/UTF8", nil, contentEncodings(contentEncodingElement(0, scopeFrames, 1)))
	// The Cluster checked: the one with the most blocks listed.
	good := testCluster{blocks: []testBlock{groupBlock(1, 1000, 100, "One."), groupBlock(1, 2000, 100, "Two.")}}
	pointing := func(cluster, relative uint64) [][]byte {
		return [][]byte{ebmlElement(idCuePoint, ebmlUint(idCueTime, 0), ebmlElement(idCueTrackPositions,
			ebmlUint(idCueTrack, 1), ebmlUint(idCueClusterPosition, cluster), ebmlUint(idCueRelativePosition, relative)))}
	}

	tests := []struct {
		name string
		file testFile
		// err is the error expected, nil for any but ErrIncompleteIndex.
		err error
	}{
		{"a block the Cues do not list", testFile{tracks: [][]byte{plain}, clusters: []testCluster{{blocks: []testBlock{
			groupBlock(1, 1000, 100, "Listed."), {track: 1, time: 1500, data: []byte("Unlisted."), unlisted: true}, groupBlock(1, 2000, 100, "Listed."),
		}}}}, ErrIncompleteIndex},
		{"a track the Cues do not list", testFile{tracks: [][]byte{plain}, clusters: []testCluster{{blocks: []testBlock{
			{track: 1, time: 1000, data: []byte("Unlisted."), unlisted: true},
		}}}}, ErrIncompleteIndex},
		{"laced block", testFile{tracks: [][]byte{plain}, clusters: []testCluster{{blocks: []testBlock{
			{track: 1, time: 1000, flags: 0x02, data: []byte{0x01, 0x03, 'a', 'b', 'c', 'd'}},
		}}}}, errLaced},
		{"encrypted", testFile{tracks: [][]byte{encrypted}, clusters: []testCluster{good}}, nil},
		{"zlib bomb", testFile{tracks: [][]byte{zlibTrack}, clusters: []testCluster{{blocks: []testBlock{
			{track: 1, time: 1000, data: deflate(make([]byte, 2<<20))},
		}}}}, errInvalid},
		{"corrupt zlib", testFile{tracks: [][]byte{zlibTrack}, clusters: []testCluster{{blocks: []testBlock{
			{track: 1, time: 1000, data: []byte("not zlib")},
		}}}}, errInvalid},
		{"a block past the segment", testFile{tracks: [][]byte{plain}, clusters: []testCluster{good}, extraCues: pointing(0, 1<<40)}, errInvalid},
		// The Segment starts with the SeekHead.
		{"Cues pointing to no Cluster", testFile{tracks: [][]byte{plain}, clusters: []testCluster{good}, extraCues: pointing(0, 3)}, errInvalid},
		{"Cues pointing to another track's block", testFile{tracks: [][]byte{plain, subtitleTrack(2, "S_TEXT/UTF8", nil)}, clusters: []testCluster{
			good, {blocks: []testBlock{{track: 2, cueTrack: 1, time: 3000, data: []byte("Other.")}}},
		}}, errInvalid},
		// A SimpleBlock claiming 1 GiB, though its Cluster holds less.
		{"a block of 1 GiB", testFile{tracks: [][]byte{plain}, clusters: []testCluster{
			good, {raw: slices.Concat(idBytes(idSimpleBlock), sizeBytes(1<<30), []byte{0x81, 0, 0, 0}, []byte("Huge.")), rawTrack: 1},
		}}, errInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := test.file.build()
			blocks, err := openMemory(t, data).SubtitleBlocks(context.Background(), &fetcher{data: data}, 1)
			t.Log(err)
			if err == nil || test.err != nil && !errors.Is(err, test.err) {
				t.Errorf("got %v, %v, want %v", blocks, err, test.err)
			}
			if test.err != ErrIncompleteIndex && errors.Is(err, ErrIncompleteIndex) {
				t.Errorf("got %v", err)
			}
		})
	}
}

func TestMatroskaSubtitleLocations(t *testing.T) {
	// Track 1's blocks are all listed, track 2's not at all, track 3 has
	// a CueTrackPositions without a relative position, track 4 is not a
	// subtitle track.
	data := testFile{
		tracks: [][]byte{subtitleTrack(1, "S_TEXT/UTF8", nil), subtitleTrack(2, "S_TEXT/UTF8", nil),
			subtitleTrack(3, "S_TEXT/UTF8", nil), trackEntry(4, 2)},
		clusters: []testCluster{{blocks: []testBlock{
			groupBlock(1, 1000, 100, "One."), {track: 2, data: []byte("Two."), unlisted: true}, groupBlock(3, 1000, 100, "Three."),
			groupBlock(1, 1000, 100, "Four."), groupBlock(4, 1000, 100, "Audio."),
		}}},
		extraCues: [][]byte{ebmlElement(idCuePoint, ebmlUint(idCueTime, 0), ebmlElement(idCueTrackPositions,
			ebmlUint(idCueTrack, 3), ebmlUint(idCueClusterPosition, 0)))},
	}.build()
	locations, err := openMemory(t, data).SubtitleLocations(context.Background())
	if err != nil || !maps.Equal(locations, map[uint64]int{1: 2}) {
		t.Errorf("got %v, %v", locations, err)
	}

	// Without Cues, no track is located.
	noCues := matroskaFile(ebmlElement(idTracks, subtitleTrack(1, "S_TEXT/UTF8", nil)))
	m := openMemory(t, noCues)
	locations, err = m.SubtitleLocations(context.Background())
	if err != nil || len(locations) != 0 {
		t.Errorf("without Cues: got %v, %v", locations, err)
	}
	if _, err := m.SubtitleBlocks(context.Background(), &fetcher{data: noCues}, 1); !errors.Is(err, ErrIncompleteIndex) {
		t.Errorf("without Cues: got %v", err)
	}
}

func TestMatroskaAttachments(t *testing.T) {
	track := subtitleTrack(1, "S_TEXT/ASS", nil)
	file := func(name, mime, description string, uid uint64, data []byte) []byte {
		return ebmlElement(idAttachedFile, ebmlElement(idFileName, []byte(name)), ebmlElement(idFileMediaType, []byte(mime)),
			ebmlElement(idFileDescription, []byte(description)), ebmlUint(idFileUID, uid), ebmlElement(idFileData, data))
	}
	two := testFile{tracks: [][]byte{track}, attachments: ebmlElement(idAttachments,
		file("A.ttf", "font/ttf", "", 1, []byte("first font")),
		ebmlElement(idVoid, []byte{0}),
		file("B.otf", "font/otf", "Bold", 2, []byte("second font")),
	)}.build()
	attachments, err := openMemory(t, two).Attachments(context.Background())
	want := []Attachment{
		{FileName: "A.ttf", MimeType: "font/ttf", UID: 1, Data: []byte("first font")},
		{FileName: "B.otf", MimeType: "font/otf", Description: "Bold", UID: 2, Data: []byte("second font")},
	}
	if err != nil || !slices.EqualFunc(attachments, want, func(a, b Attachment) bool {
		return a.FileName == b.FileName && a.MimeType == b.MimeType && a.Description == b.Description && a.UID == b.UID &&
			bytes.Equal(a.Data, b.Data)
	}) {
		t.Errorf("got %+v, %v", attachments, err)
	}

	none := testFile{tracks: [][]byte{track}}.build()
	if attachments, err := openMemory(t, none).Attachments(context.Background()); err != nil || attachments != nil {
		t.Errorf("without attachments: got %v, %v", attachments, err)
	}

	// A file's data overruns its parent; the Attachments claim 1 GiB,
	// more than the Segment holds. A Cluster comes first, which ends the
	// walk of the head.
	for _, attachments := range [][]byte{
		ebmlElement(idAttachments, slices.Concat(idBytes(idAttachedFile), sizeBytes(1<<20))),
		slices.Concat(idBytes(idAttachments), sizeBytes(1<<30)),
	} {
		data := testFile{tracks: [][]byte{track}, clusters: []testCluster{{}}, attachments: attachments, attachmentsLast: true}.build()
		m := openMemory(t, data)
		if _, err := m.Attachments(context.Background()); !errors.Is(err, errInvalid) {
			t.Errorf("got %v, want errInvalid", err)
		}
	}

	// The Attachments claim 200 MiB, which a Segment of unknown size may
	// hold: they are not read.
	head := testFile{tracks: [][]byte{track}, clusters: []testCluster{{}},
		attachments: slices.Concat(idBytes(idAttachments), sizeBytes(200<<20)), attachmentsLast: true}.build()
	copy(head[bytes.Index(head, idBytes(idSegment))+4:], []byte{0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
	large := &sparse{parts: []part{{0, head}}, size: int64(len(head)) + 200<<20}
	m, err := OpenMatroska(context.Background(), large, large.size)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Attachments(context.Background()); !errors.Is(err, errInvalid) || large.read > 2*window {
		t.Errorf("got %v after reading %d bytes, want errInvalid", err, large.read)
	}
}

// TestMatroskaCorrupted alters the subtitles fixture, a byte at a time
// every few: reading its tracks, blocks and attachments may fail, but must
// not panic.
func TestMatroskaCorrupted(t *testing.T) {
	original := fixture(t, "subtitles.mkv")
	data := slices.Clone(original)
	for i := 0; i < len(data); i += 7 {
		for _, value := range []byte{0x00, 0xFF, original[i] ^ 0x80} {
			data[i] = value
			m, err := OpenMatroska(context.Background(), &memory{data: data}, int64(len(data)))
			if err != nil {
				continue
			}
			_, _ = m.SubtitleLocations(context.Background())
			for _, track := range m.Tracks() {
				if track.Type == trackTypeSubtitle {
					_, _ = m.SubtitleBlocks(context.Background(), &fetcher{data: data}, track.Number)
				}
			}
			_, _ = m.Attachments(context.Background())
		}
		data[i] = original[i]
	}
}
