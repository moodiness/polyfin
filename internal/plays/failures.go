package plays

import (
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The reasons a play fails.
const (
	// FailedNoStream: none of the title's versions plays on the app now.
	FailedNoStream = "no_compatible_stream"
	// FailedLimit: a limit refused the play: the user's playbacks at once,
	// or the connections of a provider.
	FailedLimit = "limit"
	// FailedStream: the stream broke while it played, or could not be
	// served.
	FailedStream = "stream_error"
)

// Failure is a play that failed: who tried it, on which app and device,
// what, why, and when. App and DeviceName are empty when not known.
type Failure struct {
	User       accounts.ID
	UserName   string
	App        string
	DeviceName string
	Title      Title
	Reason     string
	At         time.Time
}
