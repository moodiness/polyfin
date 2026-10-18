package torrent

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
)

// Hash is a torrent's info hash, the SHA-1 of its info dictionary.
type Hash [20]byte

func (h Hash) String() string { return hex.EncodeToString(h[:]) }

// ParseHash reads an info hash written in hexadecimal, as addons give it.
func ParseHash(text string) (Hash, error) {
	var h Hash
	raw, err := hex.DecodeString(strings.TrimSpace(text))
	if err != nil || len(raw) != len(h) {
		return h, fmt.Errorf("%w: not 40 hexadecimal digits", ErrInvalidSpec)
	}
	copy(h[:], raw)
	return h, nil
}

// File is a file of a torrent: where it starts among the torrent's bytes,
// and its length.
type File struct {
	Path   string
	Offset int64
	Length int64
	// Padding marks a file of zeros aligning the next one on a piece.
	Padding bool
}

// Info is what a torrent's info dictionary tells: its files, and its
// pieces with their hashes.
type Info struct {
	Name        string
	PieceLength int64
	Pieces      []Hash
	Files       []File
	Length      int64
}

// maxPieceLength and maxInfoSize bound what a torrent may claim, so that
// a hostile peer's metadata cannot ask for unbounded memory; maxPieces is
// the most pieces such an info dictionary lists, 20 bytes of hash each.
const (
	maxPieceLength = 64 << 20
	maxInfoSize    = 16 << 20
	maxPieces      = maxInfoSize / 20
)

// errInfo reports an info dictionary that does not describe a torrent.
var errInfo = errors.New("the torrent's info dictionary is invalid")

// parseInfo reads an info dictionary, whose hash it does not check.
func parseInfo(data []byte) (Info, error) {
	if len(data) > maxInfoSize {
		return Info{}, errInfo
	}
	d, rest, err := decodeDict(data)
	if err != nil || len(rest) != 0 {
		return Info{}, errInfo
	}
	info := Info{Name: str(d, "name")}
	pieceLength, _ := num(d, "piece length")
	pieces := str(d, "pieces")
	if pieceLength <= 0 || pieceLength > maxPieceLength || len(pieces) == 0 || len(pieces)%20 != 0 {
		return Info{}, errInfo
	}
	info.PieceLength = pieceLength
	info.Pieces = make([]Hash, len(pieces)/20)
	for i := range info.Pieces {
		copy(info.Pieces[i][:], pieces[i*20:])
	}
	// The files lie within the pieces, which a sum of lengths that
	// overflows would not tell: a length is checked against the bytes the
	// pieces leave, at most maxPieces*maxPieceLength, before it is added.
	room := int64(len(info.Pieces)) * pieceLength
	if length, ok := num(d, "length"); ok {
		if length < 0 || length > room {
			return Info{}, errInfo
		}
		info.Files = []File{{Path: info.Name, Length: length}}
		info.Length = length
	} else {
		for _, entry := range list(d, "files") {
			f, ok := entry.(map[string]any)
			if !ok {
				return Info{}, errInfo
			}
			length, ok := num(f, "length")
			if !ok || length < 0 || length > room-info.Length {
				return Info{}, errInfo
			}
			parts := []string{info.Name}
			for _, part := range list(f, "path") {
				name, _ := part.(string)
				parts = append(parts, name)
			}
			info.Files = append(info.Files, File{Path: path.Join(parts...), Offset: info.Length, Length: length,
				Padding: strings.Contains(str(f, "attr"), "p")})
			info.Length += length
		}
		if len(info.Files) == 0 {
			return Info{}, errInfo
		}
	}
	if pieces := (info.Length + pieceLength - 1) / pieceLength; pieces != int64(len(info.Pieces)) {
		return Info{}, errInfo
	}
	return info, nil
}

// checkInfo reads an info dictionary whose SHA-1 must be hash.
func checkInfo(hash Hash, data []byte) (Info, error) {
	if sha1.Sum(data) != hash {
		return Info{}, errors.New("the torrent's info dictionary does not match its hash")
	}
	return parseInfo(data)
}

// File returns the file of the torrent to play: the one at index, else,
// for index -1, the largest, as Stremio picks it.
func (info Info) File(index int) (File, error) {
	if index >= 0 {
		if index >= len(info.Files) || info.Files[index].Padding {
			return File{}, fmt.Errorf("the torrent has no file %s", strconv.Itoa(index))
		}
		return info.Files[index], nil
	}
	largest := -1
	for i, f := range info.Files {
		if !f.Padding && (largest < 0 || f.Length > info.Files[largest].Length) {
			largest = i
		}
	}
	if largest < 0 {
		return File{}, errors.New("the torrent has no file to play")
	}
	return info.Files[largest], nil
}

// pieceSize is the length of piece index, the last one being shorter.
func (info Info) pieceSize(index int) int64 {
	if index == len(info.Pieces)-1 {
		return info.Length - int64(index)*info.PieceLength
	}
	return info.PieceLength
}
