package hls

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
)

// Kept video is converted apart from its audio (see KeepVideo): the
// converting FFmpeg writes the video alone, and another FFmpeg the audio
// track played, cut at the same segment times. Each segment served joins
// the two, copying both. Fragmented MP4 gets an initialization segment
// holding both tracks, the audio as track joinedAudio, and in each segment
// the audio's fragment after the video's. MPEG-TS gets a program map
// listing both streams, and their packets interleaved by decode time.

// joinedAudio is the track id of the audio in joined fragmented MP4: the
// video FFmpeg writes alone is track 1.
const joinedAudio = 2

// audioPID is the PID of the audio in joined MPEG-TS: FFmpeg gives the
// video alone 0x100.
const audioPID = 0x101

// audioPiece is a part of the audio FFmpeg writes: the fMP4
// initialization segment, or MPEG-TS's program map entry of the audio
// stream, or audio starting at its presentation time at: in MPEG-TS, the
// packets of one PES; in fMP4, the samples of one fragment, the first
// decoded at decode, in the track's timescale.
type audioPiece struct {
	init    bool
	data    []byte
	at      time.Duration
	decode  uint64
	samples []fragmentSample
}

// fragmentSample is a sample of an fMP4 fragment: how long it lasts and
// its flags, as the fragment or its track's defaults tell, and its bytes.
type fragmentSample struct {
	duration, flags uint32
	data            []byte
}

// sampleDefaults are what a trex or a tfhd tells the samples that do not
// tell their own.
type sampleDefaults struct{ duration, size, flags uint32 }

// splitAudioMP4 reads the fragmented MP4 FFmpeg writes of an audio track
// alone, a fragment per frame (frag_every_frame), and emits its
// initialization segment, then the samples of each fragment.
func splitAudioMP4(r io.Reader, emit func(audioPiece) error) error {
	in := bufio.NewReaderSize(r, 1<<16)
	var header, fragment []byte
	var timescale uint64
	var defaults sampleDefaults
	for {
		box, kind, err := readBox(in)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch kind {
		case "ftyp":
			header = append(header, box...)
		case "moov":
			if _, timescale, err = mediaTrack(box[8:], "soun"); err != nil {
				return err
			}
			defaults = trackExtends(box[8:])
			if err := emit(audioPiece{init: true, data: append(header, box...)}); err != nil {
				return err
			}
		case "moof":
			if timescale == 0 {
				return errors.New("a fragment came before the movie header")
			}
			fragment = box
		case "mdat":
			if fragment == nil {
				return errors.New("media data came without a fragment header")
			}
			decode, samples, err := fragmentSamples(append(fragment, box...), defaults)
			if err != nil {
				return err
			}
			fragment = nil
			if len(samples) == 0 {
				continue
			}
			if err := emit(audioPiece{at: ticks(decode, timescale) - timestampOffset, decode: decode, samples: samples}); err != nil {
				return err
			}
		}
	}
}

// trackExtends reads the sample defaults of a moov's first trex.
func trackExtends(moov []byte) sampleDefaults {
	var d sampleDefaults
	children(moov, func(kind string, mvex []byte) {
		if kind != "mvex" {
			return
		}
		children(mvex, func(kind string, trex []byte) {
			if kind == "trex" && len(trex) >= 24 {
				d = sampleDefaults{binary.BigEndian.Uint32(trex[12:]), binary.BigEndian.Uint32(trex[16:]), binary.BigEndian.Uint32(trex[20:])}
			}
		})
	})
	return d
}

// fragmentSamples reads the samples of the first track of a fragment, a
// moof followed by its mdat, whose data offsets count from the moof
// (default-base-is-moof), and when the first was decoded.
func fragmentSamples(fragment []byte, d sampleDefaults) (uint64, []fragmentSample, error) {
	moofSize := uint64(binary.BigEndian.Uint32(fragment))
	if moofSize < 8 || moofSize > uint64(len(fragment)) {
		return 0, nil, errors.New("a fragment header has an unsupported size")
	}
	var decode uint64
	var samples []fragmentSample
	var failed error
	seen := false
	children(fragment[8:moofSize], func(kind string, traf []byte) {
		if kind != "traf" || seen {
			return
		}
		seen = true
		defaults := d
		// A trun without a data offset follows the data of the one before.
		next := -1
		children(traf, func(kind string, c []byte) {
			if failed != nil {
				return
			}
			switch kind {
			case "tfhd":
				failed = defaults.read(c)
			case "tfdt":
				if len(c) >= 12 && c[0] == 1 {
					decode = binary.BigEndian.Uint64(c[4:])
				} else if len(c) >= 8 {
					decode = uint64(binary.BigEndian.Uint32(c[4:]))
				}
			case "trun":
				var run []fragmentSample
				run, next, failed = runSamples(c, defaults, fragment, next)
				samples = append(samples, run...)
			}
		})
	})
	return decode, samples, failed
}

// read reads the defaults a tfhd overrides.
func (d *sampleDefaults) read(tfhd []byte) error {
	if len(tfhd) < 8 {
		return errors.New("a track fragment header is cut short")
	}
	flags := binary.BigEndian.Uint32(tfhd) & 0xffffff
	if flags&0x01 != 0 {
		return errors.New("a track fragment counts its data from the file")
	}
	at := 8
	for _, field := range []struct {
		flag  uint32
		value *uint32
	}{{0x02, nil}, {0x08, &d.duration}, {0x10, &d.size}, {0x20, &d.flags}} {
		if flags&field.flag == 0 {
			continue
		}
		if len(tfhd) < at+4 {
			return errors.New("a track fragment header is cut short")
		}
		if field.value != nil {
			*field.value = binary.BigEndian.Uint32(tfhd[at:])
		}
		at += 4
	}
	return nil
}

// runSamples reads the samples of a trun, their data in fragment from its
// data offset, or from next when it gives none. It returns where the
// data of the next trun follows.
func runSamples(trun []byte, d sampleDefaults, fragment []byte, next int) ([]fragmentSample, int, error) {
	short := errors.New("a track run is cut short")
	if len(trun) < 8 {
		return nil, 0, short
	}
	flags := binary.BigEndian.Uint32(trun) & 0xffffff
	count := int(binary.BigEndian.Uint32(trun[4:]))
	at := 8
	offset := next
	if flags&0x01 != 0 {
		if len(trun) < at+4 {
			return nil, 0, short
		}
		offset = int(int32(binary.BigEndian.Uint32(trun[at:])))
		at += 4
	}
	if offset < 0 {
		return nil, 0, errors.New("a track run has no data offset")
	}
	first := d.flags
	hasFirst := false
	if flags&0x04 != 0 {
		if len(trun) < at+4 {
			return nil, 0, short
		}
		first, hasFirst = binary.BigEndian.Uint32(trun[at:]), true
		at += 4
	}
	perSample := 0
	for _, flag := range []uint32{0x100, 0x200, 0x400, 0x800} {
		if flags&flag != 0 {
			perSample += 4
		}
	}
	if count < 0 || len(trun) < at+count*perSample {
		return nil, 0, short
	}
	samples := make([]fragmentSample, 0, count)
	for i := range count {
		s := fragmentSample{duration: d.duration, flags: d.flags}
		size := d.size
		if i == 0 && hasFirst {
			s.flags = first
		}
		if flags&0x100 != 0 {
			s.duration = binary.BigEndian.Uint32(trun[at:])
			at += 4
		}
		if flags&0x200 != 0 {
			size = binary.BigEndian.Uint32(trun[at:])
			at += 4
		}
		if flags&0x400 != 0 {
			s.flags = binary.BigEndian.Uint32(trun[at:])
			at += 4
		}
		if flags&0x800 != 0 {
			// Audio has no composition offset to keep.
			at += 4
		}
		if uint64(offset)+uint64(size) > uint64(len(fragment)) {
			return nil, 0, errors.New("a sample lies past its fragment")
		}
		s.data = fragment[offset : offset+int(size)]
		offset += int(size)
		samples = append(samples, s)
	}
	return samples, offset, nil
}

// audioFragment writes samples, the first decoded at decode, as one
// fragment of track joinedAudio, numbered sequence.
func audioFragment(sequence uint32, decode uint64, samples []fragmentSample) []byte {
	size := 0
	for _, s := range samples {
		size += len(s.data)
	}
	trunSize := 8 + 12 + 12*len(samples)
	trafSize := 8 + 16 + 20 + trunSize
	moofSize := 8 + 16 + trafSize
	out := make([]byte, 0, moofSize+8+size)
	out = appendHeader(out, moofSize, "moof")
	out = appendHeader(out, 16, "mfhd")
	out = binary.BigEndian.AppendUint32(out, 0)
	out = binary.BigEndian.AppendUint32(out, sequence)
	out = appendHeader(out, trafSize, "traf")
	// default-base-is-moof.
	out = appendHeader(out, 16, "tfhd")
	out = binary.BigEndian.AppendUint32(out, 0x020000)
	out = binary.BigEndian.AppendUint32(out, joinedAudio)
	out = appendHeader(out, 20, "tfdt")
	out = binary.BigEndian.AppendUint32(out, 1<<24)
	out = binary.BigEndian.AppendUint64(out, decode)
	// A data offset, then each sample's duration, size and flags.
	out = appendHeader(out, trunSize, "trun")
	out = binary.BigEndian.AppendUint32(out, 0x000701)
	out = binary.BigEndian.AppendUint32(out, uint32(len(samples)))
	out = binary.BigEndian.AppendUint32(out, uint32(moofSize+8))
	for _, s := range samples {
		out = binary.BigEndian.AppendUint32(out, s.duration)
		out = binary.BigEndian.AppendUint32(out, uint32(len(s.data)))
		out = binary.BigEndian.AppendUint32(out, s.flags)
	}
	out = appendHeader(out, 8+size, "mdat")
	for _, s := range samples {
		out = append(out, s.data...)
	}
	return out
}

func appendHeader(out []byte, size int, kind string) []byte {
	return append(binary.BigEndian.AppendUint32(out, uint32(size)), kind...)
}

// box is a box of an MP4 file: its type, and its bytes, header included.
type box struct {
	kind  string
	bytes []byte
}

// boxesOf lists the boxes of data, which must hold whole boxes.
func boxesOf(data []byte) ([]box, error) {
	var boxes []box
	for len(data) > 0 {
		if len(data) < 8 {
			return nil, errors.New("a box is cut short")
		}
		size := uint64(binary.BigEndian.Uint32(data))
		if size == 1 && len(data) >= 16 {
			size = binary.BigEndian.Uint64(data[8:])
		}
		if size < 8 || size > uint64(len(data)) {
			return nil, fmt.Errorf("a %q box has an unsupported size: %d", data[4:8], size)
		}
		boxes = append(boxes, box{string(data[4:8]), data[:size]})
		data = data[size:]
	}
	return boxes, nil
}

// content is a box's bytes after its 8-byte header, as FFmpeg writes those
// of a movie header.
func (b box) content() []byte { return b.bytes[8:] }

// joinInit joins the initialization segment of video alone with that of
// an audio track alone: the video's movie, with the audio's track as
// track joinedAudio.
func joinInit(video, audio []byte) ([]byte, error) {
	videoBoxes, err := boxesOf(video)
	if err != nil {
		return nil, err
	}
	audioBoxes, err := boxesOf(audio)
	if err != nil {
		return nil, err
	}
	var ftyp, videoMoov, audioMoov []byte
	for _, b := range videoBoxes {
		switch b.kind {
		case "ftyp":
			ftyp = b.bytes
		case "moov":
			videoMoov = b.content()
		}
	}
	for _, b := range audioBoxes {
		if b.kind == "moov" {
			audioMoov = b.content()
		}
	}
	if videoMoov == nil || audioMoov == nil {
		return nil, errors.New("an initialization segment has no movie header")
	}
	var trak, trex []byte
	inAudio, err := boxesOf(audioMoov)
	if err != nil {
		return nil, err
	}
	for _, b := range inAudio {
		switch b.kind {
		case "trak":
			if trak == nil {
				if trak, err = renumberTrack(b); err != nil {
					return nil, err
				}
			}
		case "mvex":
			extends, err := boxesOf(b.content())
			if err != nil {
				return nil, err
			}
			for _, e := range extends {
				if e.kind == "trex" && len(e.bytes) >= 16 {
					trex = append([]byte(nil), e.bytes...)
					binary.BigEndian.PutUint32(trex[12:], joinedAudio)
				}
			}
		}
	}
	if trak == nil || trex == nil {
		return nil, errors.New("the audio's initialization segment has no fragmented track")
	}
	inVideo, err := boxesOf(videoMoov)
	if err != nil {
		return nil, err
	}
	var moov []byte
	for _, b := range inVideo {
		switch b.kind {
		case "mvhd":
			mvhd := append([]byte(nil), b.bytes...)
			if len(mvhd) < 12 || binary.BigEndian.Uint32(mvhd[len(mvhd)-4:]) != joinedAudio {
				return nil, errors.New("the video's initialization segment holds more than one track")
			}
			binary.BigEndian.PutUint32(mvhd[len(mvhd)-4:], joinedAudio+1)
			moov = append(moov, mvhd...)
		case "mvex":
			mvex := appendHeader(nil, len(b.bytes)+len(trex), "mvex")
			mvex = append(append(mvex, b.content()...), trex...)
			// The audio's track comes after the video's.
			moov = append(append(moov, trak...), mvex...)
			trak = nil
		default:
			moov = append(moov, b.bytes...)
		}
	}
	if trak != nil {
		return nil, errors.New("the video's initialization segment has no fragmented track")
	}
	out := append([]byte(nil), ftyp...)
	out = appendHeader(out, 8+len(moov), "moov")
	return append(out, moov...), nil
}

// renumberTrack copies a trak, its tkhd naming it joinedAudio.
func renumberTrack(trak box) ([]byte, error) {
	out := append([]byte(nil), trak.bytes...)
	inside, err := boxesOf(trak.content())
	if err != nil {
		return nil, err
	}
	at := 8
	for _, b := range inside {
		if b.kind == "tkhd" {
			// Version 1 has 64-bit creation and modification times.
			id := at + 8 + 12
			if len(b.bytes) > 8 && b.bytes[8] == 1 {
				id = at + 8 + 20
			}
			if id+4 > at+len(b.bytes) {
				return nil, errors.New("a track header is cut short")
			}
			binary.BigEndian.PutUint32(out[id:], joinedAudio)
			return out, nil
		}
		at += len(b.bytes)
	}
	return nil, errors.New("a track has no header")
}

// splitAudioTS reads the MPEG-TS FFmpeg writes of an audio track alone,
// a PES per frame, and emits the program map's entry of its stream, then
// the packets of each PES, their PCR left out: the joined segment's clock
// is the video's.
func splitAudioTS(r io.Reader, emit func(audioPiece) error) error {
	in := bufio.NewReaderSize(r, 1<<16)
	var pmtPID uint16 = 0x1fff
	var pid uint16 = 0x1fff
	var current []byte
	var at time.Duration
	packet := make([]byte, tsPacket)
	for {
		if _, err := io.ReadFull(in, packet); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("read a transport packet: %w", err)
		}
		if packet[0] != 0x47 {
			return errors.New("the output lost transport stream sync")
		}
		switch packetPID := binary.BigEndian.Uint16(packet[1:]) & 0x1fff; packetPID {
		case 0:
			if found, ok := programMapPID(packet); ok {
				pmtPID = found
			}
		case pmtPID:
			if pid != 0x1fff {
				continue
			}
			section, ok := programMap(packet)
			if !ok {
				continue
			}
			entries := section.entries()
			if len(entries) < 5 || len(entries) < 5+int(binary.BigEndian.Uint16(entries[3:])&0x3ff) {
				return errors.New("the audio's program map lists no stream")
			}
			entry := append([]byte(nil), entries[:5+int(binary.BigEndian.Uint16(entries[3:])&0x3ff)]...)
			pid = binary.BigEndian.Uint16(entry[1:]) & 0x1fff
			if err := emit(audioPiece{init: true, data: entry}); err != nil {
				return err
			}
		case pid:
			if pts, ok := pesTime(packet, false); ok {
				if len(current) > 0 {
					if err := emit(audioPiece{data: current, at: at}); err != nil {
						return err
					}
				}
				current = make([]byte, 0, 4*tsPacket)
				at = ticks(pts, 90000) - timestampOffset
			}
			if current != nil {
				current = append(current, withoutPCR(packet)...)
			}
		}
	}
	if len(current) > 0 {
		return emit(audioPiece{data: current, at: at})
	}
	return nil
}

// withoutPCR returns a copy of a packet whose adaptation field carries
// no PCR, when it carried one and nothing after it: the PCR becomes
// stuffing.
func withoutPCR(packet []byte) []byte {
	out := append([]byte(nil), packet...)
	if out[3]>>4&0x2 == 0 || out[4] < 7 || out[5]&0x10 == 0 || out[5]&0x0f != 0 {
		return out
	}
	out[5] &^= 0x10
	for i := 6; i < 12; i++ {
		out[i] = 0xff
	}
	return out
}

// pesTime reads the time of a packet starting a PES: its presentation
// time, or with decode its decode time when it has one, in 90 kHz units.
func pesTime(packet []byte, decode bool) (uint64, bool) {
	if packet[1]&0x40 == 0 {
		return 0, false
	}
	data, _ := payload(packet)
	if len(data) < 14 || data[0] != 0 || data[1] != 0 || data[2] != 1 || data[7]&0x80 == 0 {
		return 0, false
	}
	p := data[9:14]
	if decode && data[7]&0x40 != 0 && len(data) >= 19 {
		p = data[14:19]
	}
	return uint64(p[0]>>1&0x7)<<30 | uint64(p[1])<<22 | uint64(p[2]>>1)<<15 | uint64(p[3])<<7 | uint64(p[4]>>1), true
}

// pmtSection is a program map section, from its table id to its CRC.
type pmtSection []byte

// programMap reads the program map section starting in a packet.
func programMap(packet []byte) (pmtSection, bool) {
	if packet[1]&0x40 == 0 {
		return nil, false
	}
	data, _ := payload(packet)
	if len(data) < 1 || len(data) < 1+int(data[0])+12 {
		return nil, false
	}
	section := data[1+int(data[0]):]
	length := 3 + int(binary.BigEndian.Uint16(section[1:])&0x3ff)
	if section[0] != 0x02 || length < 16 || length > len(section) {
		return nil, false
	}
	return pmtSection(section[:length]), true
}

// info is the section's program info length.
func (s pmtSection) info() int { return int(binary.BigEndian.Uint16(s[10:]) & 0x3ff) }

// entries are the section's stream entries, each a stream type, a PID and
// descriptors.
func (s pmtSection) entries() []byte {
	start := 12 + s.info()
	if start > len(s)-4 {
		return nil
	}
	return s[start : len(s)-4]
}

// joinTS joins a segment of video alone, as FFmpeg writes it, with the
// audio packets of the same segment: under the video's PAT, a program map
// listing the video's streams and the audio's entry, then the packets of
// both, each PES in decode order. The video's PAT, program map and
// service description repeated within are left out.
func joinTS(video, audio, entry []byte) ([]byte, error) {
	var pat []byte
	var section pmtSection
	var pmtPID uint16 = 0x1fff
	var videoUnits []unit
	for at := 0; at+tsPacket <= len(video); at += tsPacket {
		packet := video[at : at+tsPacket]
		switch pid := binary.BigEndian.Uint16(packet[1:]) & 0x1fff; pid {
		case 0:
			if pat == nil {
				pat = packet
				if found, ok := programMapPID(packet); ok {
					pmtPID = found
				}
			}
		case pmtPID:
			if section == nil {
				section, _ = programMap(packet)
			}
		case 0x11:
		default:
			videoUnits = addUnit(videoUnits, packet)
		}
	}
	if pat == nil || section == nil {
		return nil, errors.New("a segment of video has no program map")
	}
	var audioUnits []unit
	for at := 0; at+tsPacket <= len(audio); at += tsPacket {
		audioUnits = addUnit(audioUnits, audio[at:at+tsPacket])
	}
	joined := make(pmtSection, 0, len(section)+len(entry))
	joined = append(joined, section[:len(section)-4]...)
	joined = append(joined, entry...)
	binary.BigEndian.PutUint16(joined[1:], 0xb000|uint16(len(joined)+4-3))
	joined = binary.BigEndian.AppendUint32(joined, mpegCRC(joined))
	if 5+len(joined) > tsPacket {
		return nil, errors.New("the joined program map does not fit a packet")
	}
	pmt := make([]byte, tsPacket)
	pmt[0], pmt[1], pmt[2], pmt[3] = 0x47, 0x40|byte(pmtPID>>8), byte(pmtPID), 0x10
	copy(pmt[5:], joined)
	for i := 5 + len(joined); i < tsPacket; i++ {
		pmt[i] = 0xff
	}
	out := make([]byte, 0, len(video)+len(audio)+2*tsPacket)
	out = append(append(out, pat...), pmt...)
	for len(videoUnits) > 0 || len(audioUnits) > 0 {
		next := &audioUnits
		if len(audioUnits) == 0 || len(videoUnits) > 0 && videoUnits[0].at <= audioUnits[0].at {
			next = &videoUnits
		}
		for _, packet := range (*next)[0].packets {
			out = append(out, packet...)
		}
		*next = (*next)[1:]
	}
	return out, nil
}

// unit is the packets of one PES, and its decode time in 90 kHz units.
type unit struct {
	at      uint64
	packets [][]byte
}

// addUnit adds a packet to the units of a stream: a new one when it starts
// a PES, else the last, or a first that starts nothing.
func addUnit(units []unit, packet []byte) []unit {
	if at, ok := pesTime(packet, true); ok || len(units) == 0 {
		return append(units, unit{at: at, packets: [][]byte{packet}})
	}
	last := &units[len(units)-1]
	last.packets = append(last.packets, packet)
	return units
}

// mpegCRC is the CRC-32 of MPEG-TS sections.
func mpegCRC(data []byte) uint32 {
	crc := uint32(0xffffffff)
	for _, b := range data {
		crc ^= uint32(b) << 24
		for range 8 {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ 0x04c11db7
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
