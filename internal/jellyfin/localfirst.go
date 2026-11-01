package jellyfin

import (
	"github.com/moodiness/polyfin/internal/library"
)

// Play local files first (accounts.User.LocalFirst) offers a user the
// versions of local folders and network shares before the others: once the
// hide rules, the quality group, the criteria of the version order and the
// order for the app (see appOrder) have done their part (see ordered), the
// local files move ahead, each side keeping the order they gave it. It
// hides no version, and brings back none a rule or the quality group leaves
// out. A play picking no version takes a local file that plays at all
// before a remote stream, even with Prefer versions the app plays without
// conversion, which prefers one only among the local files then (see
// pick). Other users keep the server's order.

// remoteRank is 0 for a local folder's version, 1 for another.
func remoteRank(version library.Version) int {
	if version.LocalFolder {
		return 0
	}
	return 1
}
