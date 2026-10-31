package library

import (
	"context"
	"errors"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// listedFor returns version, remembered from any user's listing, as the
// user's own sources list it, and reports whether they list it: the
// sources their view holds (see viewSources), their own addons and the
// server's while they use them, enabled. A version an addon of theirs
// listed is theirs, with the mirrors their sources list only (see
// withMirror). A version another addon listed first is the one of its
// mirrors their sources list, in its place, with the mirrors after it
// that they list too. Each check is a lookup in the addons held in memory,
// asking no addon: a stream request costs no listing.
func (s *Service) listedFor(ctx context.Context, user accounts.User, version Version) (Version, bool, error) {
	shared, personal := viewSources(user, s.settings())
	theirs := func(addon accounts.ID) (bool, error) {
		installed, owner, err := s.addons.FindOwner(ctx, addon)
		if errors.Is(err, addons.ErrNotFound) {
			return false, nil
		}
		if err != nil || !installed.Enabled {
			return false, err
		}
		if owner == (accounts.ID{}) {
			return shared, nil
		}
		return personal && owner == user.ID, nil
	}
	listed, err := theirs(version.Origin.Addon)
	if err != nil {
		return Version{}, false, err
	}
	if !listed {
		promoted := -1
		for i, mirror := range version.Mirrors {
			if listed, err = theirs(mirror.Origin.Addon); err != nil {
				return Version{}, false, err
			}
			if listed {
				promoted = i
				break
			}
		}
		if promoted < 0 {
			return Version{}, false, nil
		}
		after := version.Mirrors[promoted+1:]
		version = version.Mirrors[promoted]
		version.Mirrors = after[:len(after):len(after)]
	}
	// The mirrors are left as they are when all of them are theirs, the
	// usual case: a copy is made only to leave others out.
	var mirrors []Version
	for i, mirror := range version.Mirrors {
		ok, err := theirs(mirror.Origin.Addon)
		if err != nil {
			return Version{}, false, err
		}
		switch {
		case !ok && mirrors == nil:
			mirrors = append(make([]Version, 0, len(version.Mirrors)-1), version.Mirrors[:i]...)
		case ok && mirrors != nil:
			mirrors = append(mirrors, mirror)
		}
	}
	if mirrors != nil {
		version.Mirrors = mirrors
	}
	return version, true, nil
}
