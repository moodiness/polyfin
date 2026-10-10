package accounts

import (
	"errors"
	"math"
	"slices"
)

// ErrInvalidMaxPlaybacks reports a User.MaxPlaybacks outside
// [MinMaxPlaybacks, MaxMaxPlaybacks].
var ErrInvalidMaxPlaybacks = errors.New("invalid number of playbacks at once")

// ErrInvalidMaxBitrate reports a User.MaxBitrate outside [MinMaxBitrate,
// MaxMaxBitrate].
var ErrInvalidMaxBitrate = errors.New("invalid maximum bitrate")

// ErrInvalidSyncPlay reports a User.SyncPlay that is none of the
// SyncPlayAccess values.
var ErrInvalidSyncPlay = errors.New("invalid SyncPlay access")

// ErrInvalidQualityGroup reports a User.QualityGroup that is not one of
// ConversionHeights.
var ErrInvalidQualityGroup = errors.New("invalid quality group")

// The bounds and defaults of User.MaxPlaybacks and User.MaxBitrate; 0 sets
// no limit. A bitrate is in bits per second, a 32-bit number as in
// Jellyfin's user policy.
const (
	MinMaxPlaybacks     = 0
	MaxMaxPlaybacks     = 20
	DefaultMaxPlaybacks = 0
	MinMaxBitrate       = 0
	MaxMaxBitrate       = math.MaxInt32
	DefaultMaxBitrate   = 0
)

// SyncPlayAccess is what a user may do in SyncPlay groups, named as
// Jellyfin's SyncPlayUserAccessType.
type SyncPlayAccess string

const (
	// SyncPlayCreateAndJoin lets the user create groups and join them, the
	// default.
	SyncPlayCreateAndJoin SyncPlayAccess = "CreateAndJoinGroups"
	// SyncPlayJoin lets the user join the groups others created.
	SyncPlayJoin SyncPlayAccess = "JoinGroups"
	// SyncPlayNone keeps the user out of SyncPlay.
	SyncPlayNone SyncPlayAccess = "None"
)

// SyncPlayAccesses lists the SyncPlayAccess values, as Jellyfin numbers
// them.
var SyncPlayAccesses = []SyncPlayAccess{SyncPlayCreateAndJoin, SyncPlayJoin, SyncPlayNone}

// MayCreateGroups reports whether the access lets the user create groups.
func (a SyncPlayAccess) MayCreateGroups() bool { return a == SyncPlayCreateAndJoin }

// MayJoinGroups reports whether the access lets the user join groups, and
// list them.
func (a SyncPlayAccess) MayJoinGroups() bool { return a == SyncPlayCreateAndJoin || a == SyncPlayJoin }

// FitsGroup reports whether video height lines tall fits the user's quality
// group: any does under Original, and so does video of unknown height, 0.
func (user User) FitsGroup(height int) bool {
	return user.QualityGroup == 0 || height <= user.QualityGroup
}

// ConversionCaps are the heights video converted for user is scaled down
// to at most, 0 when it does not limit it: their quality group, and the
// settings' MaxConversionHeight.
func (s *Store) ConversionCaps(user User) (group, server int) {
	return user.QualityGroup, s.Settings().MaxConversionHeight
}

// checkAccess checks the playback and access limits changes set.
func (changes UserChanges) checkAccess() error {
	if n := changes.MaxPlaybacks; n != nil && (*n < MinMaxPlaybacks || *n > MaxMaxPlaybacks) {
		return ErrInvalidMaxPlaybacks
	}
	if n := changes.MaxBitrate; n != nil && (*n < MinMaxBitrate || *n > MaxMaxBitrate) {
		return ErrInvalidMaxBitrate
	}
	if a := changes.SyncPlay; a != nil && !slices.Contains(SyncPlayAccesses, *a) {
		return ErrInvalidSyncPlay
	}
	if group := changes.QualityGroup; group != nil && !slices.Contains(ConversionHeights, *group) {
		return ErrInvalidQualityGroup
	}
	return nil
}
