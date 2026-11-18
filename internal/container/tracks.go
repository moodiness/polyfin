package container

import (
	"bytes"
	"cmp"
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
)

// Track is a track of a Matroska file, as its TrackEntry describes it.
type Track struct {
	Number uint64
	// Type is the TrackType: 1 video, 2 audio, 0x11 subtitle, 0x21
	// metadata, ...
	Type    uint64
	CodecID string
	// CodecPrivate is decoded: decompressed or its header restored when the
	// track's content encodings cover it. It is as stored when the track
	// is not Decodable.
	CodecPrivate []byte
	// Decodable tells whether the track's blocks can be decoded: its
	// TrackEntry reads, and they are not encrypted, and stored as they are,
	// compressed with zlib or with their common header stripped.
	Decodable bool
	// Video is the data of the TrackEntry's Video element, as stored: the
	// size of a video track's frames and the shape they are shown in.
	Video []byte
}

// contentEncoding is a ContentEncoding of a track: a transformation its
// muxer applied to its frames, its CodecPrivate, or both.
type contentEncoding struct {
	order, scope, kind uint64
	algorithm          uint64
	settings           []byte
}

const (
	// The scopes of a ContentEncoding, a bit field.
	scopeFrames  = 1
	scopePrivate = 2
	// The types of a ContentEncoding, and the algorithms of compression
	// undone.
	encodingCompression = 0
	compressionZlib     = 0
	compressionStripped = 3
)

const (
	// maxTracks bounds the TrackEntries kept, far above what files hold;
	// maxPrivate bounds the size of a CodecPrivate once decoded, and
	// maxPrivates what decoding them all may produce, those that do not
	// decode included, so that compressed ones cannot exhaust memory nor
	// keep a processor.
	maxTracks   = 1 << 16
	maxPrivate  = 16 << 20
	maxPrivates = 64 << 20
)

// trackList is what the Tracks element tells: the tracks, why each
// TrackEntry could not be read, when it could not, and the content
// encodings to undo on the frames of each Decodable track.
type trackList struct {
	tracks []Track
	// errs holds, in TrackEntry order, the error the TrackEntry's
	// structure, TrackNumber or TrackType gave. Its track is kept, as not
	// Decodable: the file's other tracks remain usable.
	errs   []error
	frames map[uint64][]contentEncoding
}

// parseTracks reads the TrackEntries of a Tracks element, until ctx ends.
func parseTracks(ctx context.Context, data []byte) (trackList, error) {
	list := trackList{frames: map[uint64][]contentEncoding{}}
	budget := int64(maxPrivates)
	err := children(data, func(id uint32, entry []byte) error {
		if id != idTrackEntry {
			return nil
		}
		if len(list.tracks) == maxTracks {
			return fmt.Errorf("more than %d tracks: %w", maxTracks, errInvalid)
		}
		// Several callers read on behalf of no request: the budget bounds
		// the work whether ctx ends or not.
		if err := ctx.Err(); err != nil {
			return err
		}
		track, encodings, err := parseTrack(entry, &budget)
		list.tracks = append(list.tracks, track)
		list.errs = append(list.errs, err)
		if _, seen := list.frames[track.Number]; !seen && track.Decodable {
			list.frames[track.Number] = encodings
		}
		return nil
	})
	if err != nil {
		return trackList{}, err
	}
	return list, nil
}

// parseTrack reads a TrackEntry, and the content encodings to undo on its
// frames. Decoding its CodecPrivate takes from budget, whether it decodes
// or not, and is not tried once budget is spent. The error tells
// that the TrackEntry's structure, TrackNumber or TrackType does not read:
// the track is then returned as far as it was read, not Decodable.
func parseTrack(entry []byte, budget *int64) (Track, []contentEncoding, error) {
	track := Track{Decodable: true}
	var private, encodings []byte
	err := children(entry, func(id uint32, data []byte) error {
		var err error
		switch id {
		case idTrackNumber:
			track.Number, err = unsigned(data)
		case idTrackType:
			track.Type, err = unsigned(data)
		case idCodecID:
			track.CodecID = text(data)
		case idCodecPrivate:
			private = data
		case idVideo:
			track.Video = data
		case idContentEncodings:
			encodings = data
		}
		return err
	})
	track.CodecPrivate = private
	if err != nil {
		track.Decodable = false
		return track, nil, err
	}

	// Encodings that cannot be read or undone leave the track described,
	// not Decodable.
	list, err := parseEncodings(encodings)
	if err != nil {
		track.Decodable = false
		return track, nil, nil
	}
	var onFrames, onPrivate []contentEncoding
	for _, encoding := range list {
		if encoding.kind != encodingCompression ||
			encoding.algorithm != compressionZlib && encoding.algorithm != compressionStripped {
			track.Decodable = false
			return track, nil, nil
		}
		if encoding.scope&scopeFrames != 0 {
			onFrames = append(onFrames, encoding)
		}
		if encoding.scope&scopePrivate != 0 {
			onPrivate = append(onPrivate, encoding)
		}
	}
	if len(onPrivate) > 0 && private != nil {
		decoded, err := decodePrivate(private, onPrivate, budget)
		if err != nil {
			track.Decodable = false
			return track, nil, nil
		}
		track.CodecPrivate = decoded
	}
	return track, onFrames, nil
}

// parseEncodings reads a ContentEncodings element, returning its encodings
// in the order they are undone: the specification has decoders start
// with the highest ContentEncodingOrder.
func parseEncodings(data []byte) ([]contentEncoding, error) {
	var list []contentEncoding
	err := children(data, func(id uint32, data []byte) error {
		if id != idContentEncoding {
			return nil
		}
		if len(list) == 16 {
			return fmt.Errorf("more than 16 content encodings: %w", errInvalid)
		}
		encoding := contentEncoding{scope: scopeFrames}
		err := children(data, func(id uint32, data []byte) error {
			var err error
			switch id {
			case idContentOrder:
				encoding.order, err = unsigned(data)
			case idContentScope:
				encoding.scope, err = unsigned(data)
			case idContentType:
				encoding.kind, err = unsigned(data)
			case idContentCompression:
				err = children(data, func(id uint32, data []byte) error {
					var err error
					switch id {
					case idContentCompAlgo:
						encoding.algorithm, err = unsigned(data)
					case idContentCompSettings:
						encoding.settings = data
					}
					return err
				})
			}
			return err
		})
		if err != nil {
			return err
		}
		list = append(list, encoding)
		return nil
	})
	slices.SortStableFunc(list, func(a, b contentEncoding) int {
		return cmp.Compare(b.order, a.order)
	})
	return list, err
}

// decode undoes encodings on data, failing rather than producing more than
// limit bytes.
func decode(data []byte, encodings []contentEncoding, limit int64) ([]byte, error) {
	for _, encoding := range encodings {
		var err error
		if data, _, err = undo(data, encoding, limit); err != nil {
			return nil, err
		}
	}
	return data, nil
}

// decodePrivate undoes encodings on a CodecPrivate, each within maxPrivate
// and what budget has left. The bytes undoing each made are taken from
// budget whether it could be undone or not: an inflation that fails past
// its limit made as many as one that fits. Nothing is undone once budget
// is spent, so the CodecPrivates of a file never make more than
// maxPrivates bytes and one, however many its TrackEntries.
func decodePrivate(data []byte, encodings []contentEncoding, budget *int64) ([]byte, error) {
	for _, encoding := range encodings {
		if *budget <= 0 {
			return nil, fmt.Errorf("CodecPrivates decoding to more than %d bytes: %w", maxPrivates, errInvalid)
		}
		decoded, made, err := undo(data, encoding, min(maxPrivate, *budget))
		*budget -= made
		if err != nil {
			return nil, err
		}
		data = decoded
	}
	return data, nil
}

// undo undoes encoding on data, failing rather than producing more than
// limit bytes. made is how many bytes it produced, failing or not.
func undo(data []byte, encoding contentEncoding, limit int64) (decoded []byte, made int64, err error) {
	switch encoding.algorithm {
	case compressionZlib:
		r, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, 0, fmt.Errorf("zlib content: %v: %w", err, errInvalid)
		}
		inflated, err := io.ReadAll(io.LimitReader(r, limit+1))
		made = int64(len(inflated))
		if err != nil {
			return nil, made, fmt.Errorf("zlib content: %v: %w", err, errInvalid)
		}
		if made > limit {
			return nil, made, fmt.Errorf("zlib content of more than %d bytes: %w", limit, errInvalid)
		}
		return inflated, made, nil
	case compressionStripped:
		if int64(len(encoding.settings)+len(data)) > limit {
			return nil, 0, fmt.Errorf("content of more than %d bytes: %w", limit, errInvalid)
		}
		decoded = slices.Concat(encoding.settings, data)
		return decoded, int64(len(decoded)), nil
	}
	return nil, 0, fmt.Errorf("content compressed with algorithm %d: %w", encoding.algorithm, errInvalid)
}

// text reads a string element, which may be padded with zeros.
func text(data []byte) string {
	return strings.TrimRight(string(data), "\x00")
}
