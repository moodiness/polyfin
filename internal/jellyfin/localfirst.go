package jellyfin

import (
	"cmp"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Play local files first (accounts.User.LocalFirst) offers a user the
// versions of local folders and network shares before the others: once the
// hide rules, the quality group and the criteria of the version order have
// done their part (see ordered), the local files move ahead, each side
// keeping the order the criteria gave it. It hides no version, and brings
// back none a rule or the quality group leaves out. Other users keep the
// server's order.

// localFirst moves the versions of local folders first for a user given
// Play local files first, the local files keeping their order among
// themselves and the others theirs. It sorts versions in place.
func localFirst(user accounts.User, versions []library.Version) []library.Version {
	if user.LocalFirst {
		slices.SortStableFunc(versions, func(a, b library.Version) int { return cmp.Compare(remoteRank(a), remoteRank(b)) })
	}
	return versions
}

// remoteRank is 0 for a local folder's version, 1 for another.
func remoteRank(version library.Version) int {
	if version.LocalFolder {
		return 0
	}
	return 1
}
