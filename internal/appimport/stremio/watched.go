package stremio

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"io"
	"slices"
	"strconv"
	"strings"
)

// maxWatched bounds the bytes of a watched field read: a bit for each of
// half a million episodes.
const maxWatched = 1 << 16

// watched is the field in which Stremio's apps keep the episodes of a
// series watched: "<anchor>:<length>:<bits>", where bits is the base64 of
// a zlib stream of bytes, whose bit i, the least significant first, tells
// whether the video at place i of the series' list (see ordered) was
// watched. The anchor is the identifier of the video watched last, at
// place length-1 when the field was written: a list that changed since is
// realigned on it.
type watched struct {
	anchor string
	length int
	bits   []byte
}

// parseWatched reads a watched field. Its anchor may hold colons, as an
// episode's identifier does: the length and the bits are its last parts.
func parseWatched(text string) (watched, bool) {
	parts := strings.Split(strings.TrimSpace(text), ":")
	if len(parts) < 3 {
		return watched{}, false
	}
	length, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil || length < 0 {
		return watched{}, false
	}
	compressed, err := base64.StdEncoding.DecodeString(parts[len(parts)-1])
	if err != nil {
		return watched{}, false
	}
	reader, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return watched{}, false
	}
	defer reader.Close()
	bits, err := io.ReadAll(io.LimitReader(reader, maxWatched))
	if err != nil {
		return watched{}, false
	}
	return watched{anchor: strings.Join(parts[:len(parts)-2], ":"), length: length, bits: bits}, true
}

// any reports whether the field tells any video watched.
func (w watched) any() bool {
	return slices.ContainsFunc(w.bits, func(b byte) bool { return b != 0 })
}

// places returns the places in ids, a series' videos as listed now, of
// those watched, as Stremio's apps read them: the anchor found at place
// p, the video at place i is the one at place i + length - p - 1 when the
// field was written, which absorbs the videos added or removed before the
// anchor. It reports false when the anchor is not listed: the field can
// then not be read.
func (w watched) places(ids []string) ([]int, bool) {
	p := slices.Index(ids, w.anchor)
	if p < 0 {
		return nil, false
	}
	offset := w.length - p - 1
	var result []int
	for i := range ids {
		old := i + offset
		if old >= 0 && old < len(ids) && old/8 < len(w.bits) && w.bits[old/8]>>(old%8)&1 == 1 {
			result = append(result, i)
		}
	}
	return result, true
}
