package torrent

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/episodename"
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
	// Private marks a torrent whose peers come from its trackers alone
	// (BEP 27): no peer exchange.
	Private bool
}

// maxPieceLength and maxInfoSize bound what a torrent may claim, so that
// a hostile peer's metadata cannot ask for unbounded memory; maxPieces is
// the most pieces such an info dictionary lists, 20 bytes of hash each.
const (
	maxPieceLength = 64 << 20
	maxInfoSize    = 16 << 20
	maxPieces      = maxInfoSize / 20
)

// maxFileName, maxFilePath and maxInfoFiles bound the files an info
// dictionary names, so that what is read from it stays a small multiple
// of its bytes, a torrent's name being part of the path of each of its
// files. Nothing writes these names to a disk: they bound what a
// dictionary has the server hold, and a torrent made on any system
// plays. The name and each part of a path are as long as a file's name
// is anywhere: 255 units of UTF-16 where names count so, which is 765
// bytes of the UTF-8 torrents write, and 255 bytes elsewhere. A path is
// as long as the longest path a system takes unless told otherwise,
// 4,096 bytes; what is read is bounded without it, by the name and the
// files. The files are no more than several times those of the largest
// packs of seasons. maxInfoValues bounds the values the dictionary
// holds, its lists, its dictionaries and their keys counted as what
// they hold is: decoding keeps up to a hundred bytes and more for each,
// however short it is written. That is 16 values for each of
// maxInfoFiles files, where the entry of a file in a folder holds 7.
const (
	maxFileName   = 765
	maxFilePath   = 4096
	maxInfoFiles  = 50_000
	maxInfoValues = 16 * maxInfoFiles
)

// minPieceLength is the least a torrent's pieces are long: a block, as
// the programs that make torrents make them. A torrent of shorter pieces
// would have as many pieces as it likes for few bytes, down to one a
// byte, and Polyfin as many pieces to fetch past a read, to count, and to
// keep as files, which its bounds in bytes do not see.
const minPieceLength = blockSize

// fileBytes is what a File takes in memory beside its path.
const fileBytes = 40

// errInfo reports an info dictionary that does not describe a torrent.
var errInfo = errors.New("the torrent's info dictionary is invalid")

// parseInfo reads an info dictionary, whose hash it does not check.
func parseInfo(data []byte) (Info, error) {
	if len(data) > maxInfoSize || !valuesWithin(data, maxInfoValues) {
		return Info{}, errInfo
	}
	d, rest, err := decodeDict(data)
	if err != nil || len(rest) != 0 {
		return Info{}, errInfo
	}
	private, _ := num(d, "private")
	info := Info{Name: str(d, "name"), Private: private == 1}
	if len(info.Name) > maxFileName {
		return Info{}, errInfo
	}
	pieceLength, _ := num(d, "piece length")
	pieces := str(d, "pieces")
	if pieceLength < minPieceLength || pieceLength > maxPieceLength || len(pieces) == 0 || len(pieces)%20 != 0 {
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
		files := list(d, "files")
		if len(files) > maxInfoFiles {
			return Info{}, errInfo
		}
		info.Files = make([]File, 0, len(files))
		for _, entry := range files {
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
				if len(name) > maxFileName {
					return Info{}, errInfo
				}
				parts = append(parts, name)
			}
			joined := path.Join(parts...)
			if len(joined) > maxFilePath {
				return Info{}, errInfo
			}
			info.Files = append(info.Files, File{Path: joined, Offset: info.Length, Length: length,
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

// valuesWithin reports whether bencoded data holds limit values at most,
// which it reads as decode does, keeping none: a list and a dictionary
// count, and so does each value and each key they hold. It is false too
// where the bytes stop being values, which decode refuses.
func valuesWithin(data []byte, limit int) bool {
	for count := 0; len(data) > 0; {
		switch c := data[0]; {
		case c == 'e':
			data = data[1:]
			continue
		case c == 'l' || c == 'd':
			data = data[1:]
		case c == 'i':
			end := bytes.IndexByte(data, 'e')
			if end < 0 {
				return false
			}
			data = data[end+1:]
		case c >= '0' && c <= '9':
			colon := bytes.IndexByte(data, ':')
			if colon < 0 {
				return false
			}
			n, err := strconv.Atoi(string(data[:colon]))
			if err != nil || n < 0 || n > len(data)-colon-1 {
				return false
			}
			data = data[colon+1+n:]
		default:
			return false
		}
		if count++; count > limit {
			return false
		}
	}
	return true
}

// size estimates the bytes info takes in memory: its pieces' hashes, and
// its files with their paths.
func (info Info) size() int {
	size := len(info.Name) + len(info.Pieces)*len(Hash{}) + len(info.Files)*fileBytes
	for _, f := range info.Files {
		size += len(f.Path)
	}
	return size
}

// checkInfo reads an info dictionary whose SHA-1 must be hash.
func checkInfo(hash Hash, data []byte) (Info, error) {
	if sha1.Sum(data) != hash {
		return Info{}, errors.New("the torrent's info dictionary does not match its hash")
	}
	return parseInfo(data)
}

// File returns the file of the torrent spec plays: the one at its index;
// without one, the file of the name the addon gives, else, for an
// episode, the largest video whose name tells that episode (see
// episodename.Of), as in a season's pack, else the largest.
func (info Info) File(spec Spec) (File, error) {
	if index := spec.File; index >= 0 {
		if index >= len(info.Files) || info.Files[index].Padding {
			return File{}, fmt.Errorf("the torrent has no file %s", strconv.Itoa(index))
		}
		return info.Files[index], nil
	}
	name := path.Base(strings.ReplaceAll(spec.Name, `\`, "/"))
	named, episode, largest := -1, -1, -1
	larger := func(i, than int) bool { return than < 0 || info.Files[i].Length > info.Files[than].Length }
	for i, f := range info.Files {
		if f.Padding {
			continue
		}
		if spec.Name != "" && strings.EqualFold(path.Base(f.Path), name) && larger(i, named) {
			named = i
		}
		if spec.Episode > 0 && episodename.Video(f.Path) && larger(i, episode) {
			if e, ok := episodename.Of(f.Path); ok && e.Holds(spec.Season, spec.Episode) {
				episode = i
			}
		}
		if larger(i, largest) {
			largest = i
		}
	}
	for _, i := range []int{named, episode, largest} {
		if i >= 0 {
			return info.Files[i], nil
		}
	}
	return File{}, errors.New("the torrent has no file to play")
}

// pieceSize is the length of piece index, the last one being shorter.
func (info Info) pieceSize(index int) int64 {
	if index == len(info.Pieces)-1 {
		return info.Length - int64(index)*info.PieceLength
	}
	return info.PieceLength
}
