package library

import "slices"

// maxMirrors bounds the mirrors a version keeps: each one a read tries
// takes the source cache's tries before the next (see source.Mirror).
const maxMirrors = 3

// withMirror returns first with other as one more of its mirrors, and
// whether other is one: the same file, by its name and size, as a link
// both, listed by another addon than first and its mirrors. A torrent is
// no one's mirror and has none, as its swarm is joined only while a
// playback asks for it, and takes longer to start than a player waits. A
// file known by its link alone has no mirror: two links are not known to
// be the same file. first is left as it is.
func withMirror(first, other Version) (Version, bool) {
	if first.Torrent != nil || other.Torrent != nil || first.Filename == "" || first.Size <= 0 ||
		other.Filename != first.Filename || other.Size != first.Size || other.URL == first.URL ||
		len(first.Mirrors) >= maxMirrors || other.Origin.Addon == first.Origin.Addon {
		return first, false
	}
	for _, mirror := range first.Mirrors {
		if mirror.Origin.Addon == other.Origin.Addon || mirror.URL == other.URL {
			return first, false
		}
	}
	other.Mirrors = nil
	first.Mirrors = slices.Concat(first.Mirrors, []Version{other})
	return first, true
}

// keepMirrors returns version, renewed by its addon, and the version to
// remember under its identifier: version itself, with the mirrors of the
// one remembered, when it renews that one; that one, with version in the
// place of its mirror from the same addon, when it renews a mirror. Only
// the version remembered lists mirrors, so a renewal never loses them.
func (s *Service) keepMirrors(version Version) (renewed, kept Version) {
	known, ok := s.versions.Get(version.ID)
	if !ok {
		return version, version
	}
	if known.Origin == version.Origin {
		version.Mirrors = known.Mirrors
		return version, version
	}
	i := slices.IndexFunc(known.Mirrors, func(mirror Version) bool { return mirror.Origin == version.Origin })
	if i < 0 {
		return version, version
	}
	known.Mirrors = slices.Clone(known.Mirrors)
	known.Mirrors[i] = version
	return version, known
}
