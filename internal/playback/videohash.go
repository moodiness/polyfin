package playback

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/moodiness/polyfin/internal/library"
)

// videoHashBytes is how much of each end of a file its video hash
// reads.
const videoHashBytes = 64 << 10

// videoHashTime bounds the background read of a version's ends for its
// hash (see SubtitleHints).
const videoHashTime = 30 * time.Second

// hashedVideo is the video hash of a version's file, with the size
// it was computed with.
type hashedVideo struct {
	size int64
	hash string
}

// errTooShort reports a file shorter than the hash reads at each end.
var errTooShort = errors.New("the file is shorter than its hash reads")

// videoHash returns the video hash of a file of size bytes, as subtitle
// addons take it, whose first and last videoHashBytes are head and tail:
// its size plus each 8-byte little-endian word of both, modulo 2^64, in 16
// hexadecimal digits. Subtitle addons find the files timed for that file
// by it.
func videoHash(size int64, head, tail []byte) string {
	sum := uint64(size)
	for _, part := range [2][]byte{head, tail} {
		for i := 0; i+8 <= len(part); i += 8 {
			sum += binary.LittleEndian.Uint64(part[i:])
		}
	}
	return fmt.Sprintf("%016x", sum)
}

// SubtitleHints describe version to subtitle addons (see
// library.SubtitleHints): its file's name, its size, and its video
// hash once known. The hash is computed from the bytes of the file's ends
// the shared cache holds, as its analysis and its plays read them, and
// kept for the version. Until it holds them, a link's or a local file's
// ends are read in the background, videoHashBytes each, for the next time:
// a torrent's are not, as only a play joins its swarm, nor a replay's, a
// read taking one of its provider's connections. A version read from its
// input, not one file, has no hash.
func (s *Service) SubtitleHints(ctx context.Context, version library.Version) library.SubtitleHints {
	hints := library.SubtitleHints{Filename: version.Filename, Size: version.Size}
	if analysis, ok := s.analyses.Get(version.ID); ok && analysis.Size > 0 {
		hints.Size = analysis.Size
	}
	if s.ReadsInput(version) || version.HoldsConnection || version.Audio != nil {
		return hints
	}
	if hashed, ok := s.hashes.Get(version.ID); ok {
		hints.Size, hints.Hash = hashed.size, hashed.hash
		return hints
	}
	if hashed, ok := s.cachedVideoHash(ctx, version); ok {
		s.hashes.Put(version.ID, hashed)
		hints.Size, hints.Hash = hashed.size, hashed.hash
		return hints
	}
	if _, failed := s.unhashed.Get(version.ID); !failed && version.Torrent == nil {
		go s.readVideoHash(version)
	}
	return hints
}

// cachedVideoHash computes the hash of version's file from its ends as the
// shared cache holds them, with no request; false when it holds too few.
func (s *Service) cachedVideoHash(ctx context.Context, version library.Version) (hashedVideo, bool) {
	cached, ok := s.CachedSource(version)
	if !ok {
		return hashedVideo{}, false
	}
	size, known := cached.KnownSize()
	if !known || size < videoHashBytes {
		return hashedVideo{}, false
	}
	head, err := cached.Fetch(ctx, 0, videoHashBytes)
	if err != nil || len(head) < videoHashBytes {
		return hashedVideo{}, false
	}
	tail, err := cached.Fetch(ctx, size-videoHashBytes, videoHashBytes)
	if err != nil || len(tail) < videoHashBytes {
		return hashedVideo{}, false
	}
	return hashedVideo{size: size, hash: videoHash(size, head, tail)}, true
}

// readVideoHash reads the ends of version's file, in the background, one
// attempt each, and keeps its hash. The reads of the same version share
// one; one that fails is not tried again for failureTTL.
func (s *Service) readVideoHash(version library.Version) {
	_, _, _ = s.flight.Do("video hash "+version.ID.String(), func() (any, error) {
		if _, ok := s.hashes.Get(version.ID); ok {
			return nil, nil
		}
		ctx, cancel := context.WithTimeout(Background(context.Background()), videoHashTime)
		defer cancel()
		hashed, err := s.fetchVideoHash(ctx, version)
		if err != nil {
			s.unhashed.Put(version.ID, err)
			s.logger.Debug("The ends of a version could not be read for its hash", "addon", version.AddonLog, "error", err)
			return nil, nil
		}
		s.hashes.Put(version.ID, hashed)
		return nil, nil
	})
}

// fetchVideoHash reads the ends of version's file for its hash.
func (s *Service) fetchVideoHash(ctx context.Context, version library.Version) (hashedVideo, error) {
	src := s.open(version)
	defer src.Release()
	once := src.Once()
	head, err := once.Fetch(ctx, 0, videoHashBytes)
	if err != nil && !errors.Is(err, io.EOF) {
		return hashedVideo{}, err
	}
	size, known := src.KnownSize()
	if !known || size < videoHashBytes || len(head) < videoHashBytes {
		return hashedVideo{}, errTooShort
	}
	tail, err := once.Fetch(ctx, size-videoHashBytes, videoHashBytes)
	if err != nil {
		return hashedVideo{}, err
	}
	if len(tail) < videoHashBytes {
		return hashedVideo{}, errTooShort
	}
	return hashedVideo{size: size, hash: videoHash(size, head, tail)}, nil
}
