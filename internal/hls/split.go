package hls

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
)

// piece is a part of FFmpeg's output: the fMP4 initialization segment, or
// media that starts on a video keyframe, at its presentation time, or that
// continues the piece before it.
type piece struct {
	data     []byte
	init     bool
	keyframe bool
	at       time.Duration
}

// maxBox bounds the boxes read from FFmpeg: a fragment holds the media
// between two keyframes, a few seconds even at remux bitrates.
const maxBox = 1 << 30

// splitMP4 reads a fragmented MP4 stream as FFmpeg writes it with
// frag_keyframe: ftyp and moov, which make the initialization segment, then
// a moof and an mdat for each video keyframe.
func splitMP4(r io.Reader, emit func(piece) error) error {
	in := bufio.NewReaderSize(r, 1<<16)
	var header []byte
	var video uint32
	var timescale uint64
	var fragment []byte
	var keyframe bool
	var at time.Duration
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
			video, timescale, err = videoTrack(box[8:])
			if err != nil {
				return err
			}
			if err := emit(piece{data: append(header, box...), init: true}); err != nil {
				return err
			}
		case "moof":
			if timescale == 0 {
				return errors.New("a fragment came before the movie header")
			}
			fragment = box
			var presentation uint64
			presentation, keyframe = fragmentStart(box[8:], video)
			at = ticks(presentation, timescale) - timestampOffset
		case "mdat":
			if fragment == nil {
				return errors.New("media data came without a fragment header")
			}
			if err := emit(piece{data: append(fragment, box...), keyframe: keyframe, at: at}); err != nil {
				return err
			}
			fragment = nil
		}
		// Other boxes, such as the closing mfra, are not part of segments.
	}
}

// readBox reads a whole top-level box and returns its type.
func readBox(in *bufio.Reader) ([]byte, string, error) {
	head := make([]byte, 8, 16)
	if _, err := io.ReadFull(in, head); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, "", errors.New("the output ends inside a box header")
		}
		return nil, "", err
	}
	size := uint64(binary.BigEndian.Uint32(head))
	if size == 1 {
		head = head[:16]
		if _, err := io.ReadFull(in, head[8:]); err != nil {
			return nil, "", fmt.Errorf("read a box size: %w", err)
		}
		size = binary.BigEndian.Uint64(head[8:])
	}
	if size < uint64(len(head)) || size > maxBox {
		return nil, "", fmt.Errorf("a %q box has an unsupported size: %d", head[4:8], size)
	}
	box := make([]byte, size)
	copy(box, head)
	if _, err := io.ReadFull(in, box[len(head):]); err != nil {
		return nil, "", fmt.Errorf("read a %q box: %w", head[4:8], err)
	}
	return box, string(head[4:8]), nil
}

// children calls visit with the type and the content of each box in data.
func children(data []byte, visit func(kind string, content []byte)) {
	for len(data) >= 8 {
		size := uint64(binary.BigEndian.Uint32(data))
		start := uint64(8)
		if size == 1 && len(data) >= 16 {
			size, start = binary.BigEndian.Uint64(data[8:]), 16
		}
		if size < start || size > uint64(len(data)) {
			return
		}
		visit(string(data[4:8]), data[start:size])
		data = data[size:]
	}
}

// videoTrack finds the video track of a moov's content: its id and the
// timescale of its media.
func videoTrack(moov []byte) (uint32, uint64, error) {
	var id uint32
	var timescale uint64
	children(moov, func(kind string, trak []byte) {
		if kind != "trak" || timescale != 0 {
			return
		}
		var trackID uint32
		var scale uint64
		var isVideo bool
		children(trak, func(kind string, content []byte) {
			switch kind {
			case "tkhd":
				// Version 1 has 64-bit creation and modification times.
				if len(content) >= 24 && content[0] == 1 {
					trackID = binary.BigEndian.Uint32(content[20:])
				} else if len(content) >= 16 {
					trackID = binary.BigEndian.Uint32(content[12:])
				}
			case "mdia":
				children(content, func(kind string, content []byte) {
					switch kind {
					case "mdhd":
						if len(content) >= 24 && content[0] == 1 {
							scale = uint64(binary.BigEndian.Uint32(content[20:]))
						} else if len(content) >= 16 {
							scale = uint64(binary.BigEndian.Uint32(content[12:]))
						}
					case "hdlr":
						isVideo = len(content) >= 12 && string(content[8:12]) == "vide"
					}
				})
			}
		})
		if isVideo && scale > 0 {
			id, timescale = trackID, scale
		}
	})
	if timescale == 0 {
		return 0, 0, errors.New("the output has no video track")
	}
	return id, timescale, nil
}

// fragmentStart returns the presentation time of the first sample of a
// moof's video track, and whether it is a sync sample.
func fragmentStart(moof []byte, video uint32) (uint64, bool) {
	var presentation uint64
	var keyframe, found bool
	children(moof, func(kind string, traf []byte) {
		if kind != "traf" || found {
			return
		}
		var track uint32
		var defaultFlags uint32
		var hasDefault, seenRun bool
		var first sample
		var decodeTime uint64
		children(traf, func(kind string, content []byte) {
			switch kind {
			case "tfhd":
				track, defaultFlags, hasDefault = trackHeader(content)
			case "tfdt":
				if len(content) >= 12 && content[0] == 1 {
					decodeTime = binary.BigEndian.Uint64(content[4:])
				} else if len(content) >= 8 {
					decodeTime = uint64(binary.BigEndian.Uint32(content[4:]))
				}
			case "trun":
				if !seenRun {
					seenRun, first = true, firstSample(content)
				}
			}
		})
		if track != video {
			return
		}
		found = true
		presentation = uint64(max(int64(decodeTime)+first.offset, 0))
		flags, known := defaultFlags, hasDefault
		if first.flags != nil {
			flags, known = *first.flags, true
		}
		// A sync sample has sample_is_non_sync_sample clear.
		keyframe = known && flags&0x10000 == 0
	})
	return presentation, keyframe
}

// trackHeader reads a tfhd: the track id and the default sample flags.
func trackHeader(content []byte) (uint32, uint32, bool) {
	if len(content) < 8 {
		return 0, 0, false
	}
	flags := binary.BigEndian.Uint32(content) & 0xffffff
	track := binary.BigEndian.Uint32(content[4:])
	offset := 8
	for _, field := range []struct {
		flag uint32
		size int
	}{{0x01, 8}, {0x02, 4}, {0x08, 4}, {0x10, 4}} {
		if flags&field.flag != 0 {
			offset += field.size
		}
	}
	if flags&0x20 == 0 || len(content) < offset+4 {
		return track, 0, false
	}
	return track, binary.BigEndian.Uint32(content[offset:]), true
}

// sample is what a trun says of its first sample: its flags, if given,
// and its composition time offset.
type sample struct {
	flags  *uint32
	offset int64
}

// firstSample reads a trun's first sample.
func firstSample(content []byte) sample {
	var s sample
	if len(content) < 8 {
		return s
	}
	version := content[0]
	flags := binary.BigEndian.Uint32(content) & 0xffffff
	at := 8
	if flags&0x01 != 0 {
		at += 4
	}
	if flags&0x04 != 0 {
		if len(content) < at+4 {
			return s
		}
		s.flags = new(binary.BigEndian.Uint32(content[at:]))
		at += 4
	}
	// The first sample's fields, in order: duration, size, flags and
	// composition time offset, each present if its flag is set.
	for _, flag := range []uint32{0x100, 0x200, 0x400, 0x800} {
		if flags&flag == 0 {
			continue
		}
		if len(content) < at+4 {
			return s
		}
		value := binary.BigEndian.Uint32(content[at:])
		switch flag {
		case 0x400:
			if s.flags == nil {
				s.flags = new(value)
			}
		case 0x800:
			if version == 1 {
				s.offset = int64(int32(value))
			} else {
				s.offset = int64(value)
			}
		}
		at += 4
	}
	return s
}

// ticks converts a time in timescale units, without overflowing.
func ticks(value, timescale uint64) time.Duration {
	seconds := value / timescale
	rest := value % timescale
	return time.Duration(seconds)*time.Second + time.Duration(rest*uint64(time.Second)/timescale)
}

const (
	tsPacket = 188
	// timestampOffset is added to the timestamps of the output (FFmpeg's
	// -output_ts_offset): the first video frames of a stream with B-frames
	// decode before zero, and neither format takes negative timestamps.
	timestampOffset = 10 * time.Second
)

// splitTS reads an MPEG-TS stream and cuts it before every video keyframe:
// a PES of a video stream starting in a packet flagged as a random access
// point. Each such piece begins with the last PAT and PMT seen, so that a
// segment decodes on its own.
func splitTS(r io.Reader, emit func(piece) error) error {
	in := bufio.NewReaderSize(r, 1<<16)
	var pat, pmt []byte
	var pmtPID uint16 = 0x1fff
	var current []byte
	var keyframe bool
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
		pid := binary.BigEndian.Uint16(packet[1:]) & 0x1fff
		switch pid {
		case 0:
			pat = append(pat[:0], packet...)
			if found, ok := programMapPID(packet); ok {
				pmtPID = found
			}
		case pmtPID:
			pmt = append(pmt[:0], packet...)
		}
		if pts, ok := videoKeyframe(packet); ok {
			if len(current) > 0 {
				if err := emit(piece{data: current, keyframe: keyframe, at: at}); err != nil {
					return err
				}
			}
			current = make([]byte, 0, 1<<20)
			current = append(current, pat...)
			current = append(current, pmt...)
			keyframe, at = true, ticks(pts, 90000)-timestampOffset
		}
		current = append(current, packet...)
	}
	if len(current) > 0 {
		return emit(piece{data: current, keyframe: keyframe, at: at})
	}
	return nil
}

// payload returns a transport packet's payload, after its adaptation
// field, and whether it flags a random access point.
func payload(packet []byte) ([]byte, bool) {
	control := packet[3] >> 4 & 0x3
	offset := 4
	random := false
	if control&0x2 != 0 {
		length := int(packet[4])
		random = length > 0 && packet[5]&0x40 != 0
		offset = 5 + length
	}
	if control&0x1 == 0 || offset >= tsPacket {
		return nil, random
	}
	return packet[offset:], random
}

// programMapPID reads the PID of the first program's map from a PAT
// packet.
func programMapPID(packet []byte) (uint16, bool) {
	if packet[1]&0x40 == 0 {
		return 0, false
	}
	data, _ := payload(packet)
	if len(data) < 1 || len(data) < 1+int(data[0])+12 {
		return 0, false
	}
	section := data[1+int(data[0]):]
	// After the 8-byte section header, programs are 4 bytes each; program
	// number 0 is the network PID.
	for entry := section[8:]; len(entry) >= 8; entry = entry[4:] {
		if binary.BigEndian.Uint16(entry) != 0 {
			return binary.BigEndian.Uint16(entry[2:]) & 0x1fff, true
		}
	}
	return 0, false
}

// videoKeyframe reports whether a packet starts a video PES on a random
// access point, and its presentation time in 90 kHz units.
func videoKeyframe(packet []byte) (uint64, bool) {
	if packet[1]&0x40 == 0 {
		return 0, false
	}
	data, random := payload(packet)
	if !random || len(data) < 14 || data[0] != 0 || data[1] != 0 || data[2] != 1 {
		return 0, false
	}
	if data[3] < 0xe0 || data[3] > 0xef || data[7]&0x80 == 0 {
		return 0, false
	}
	p := data[9:14]
	pts := uint64(p[0]>>1&0x7)<<30 | uint64(p[1])<<22 | uint64(p[2]>>1)<<15 | uint64(p[3])<<7 | uint64(p[4]>>1)
	return pts, true
}
