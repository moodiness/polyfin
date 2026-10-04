package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// The answers Jellyfin gives, as JSON strings, when a policy change would
// leave the server without an administrator or disable one.
const (
	lastAdministrator     = "There must be at least one user in the system with administrative access."
	administratorDisabled = "Administrators cannot be disabled."
)

// policyUpdate is the part of Jellyfin's UserPolicy Polyfin keeps. Apps
// post the whole policy, as they read it from the user; the capabilities
// Polyfin does not offer are left as they are reported.
//
// LoginAttemptsBeforeLockout and InvalidLoginAttemptCount are left out on
// purpose: Polyfin sets the limit for the whole server, from the admin
// interface, and keeps the count itself, which a sign-in, a new password
// or the admin interface's Unblock starts again.
type policyUpdate struct {
	IsAdministrator      bool
	IsHidden             bool
	IsDisabled           bool
	MaxParentalRating    *int
	MaxParentalSubRating *int
	BlockUnratedItems    []string
	// The user's permissions to have video or audio converted, and to
	// download. Like Jellyfin's, a policy that leaves them out grants them.
	EnableVideoPlaybackTranscoding bool
	EnableAudioPlaybackTranscoding bool
	EnableContentDownloading       bool
	// The user's playback and access limits. Like Jellyfin's, a policy that
	// leaves them out sets no limit, grants Live TV and SyncPlay, and denies
	// controlling other users' apps.
	MaxActiveSessions               int
	RemoteClientBitrateLimit        int
	EnableLiveTvAccess              bool
	SyncPlayAccess                  syncPlayAccessValue
	EnableRemoteControlOfOtherUsers bool
	// The server's libraries the user sees: all of them, or those
	// EnabledFolders names. BlockedTags are left out: Jellyfin's tags are
	// not genres, and titles in Polyfin have none.
	EnableAllFolders bool
	EnabledFolders   []string
	AccessSchedules  []scheduleUpdate
}

// scheduleUpdate is an AccessSchedule as apps post it. jellyfin-web sends
// the hours as strings, which Jellyfin reads as numbers.
type scheduleUpdate struct {
	DayOfWeek json.RawMessage
	StartHour looseFloat
	EndHour   looseFloat
}

// looseFloat reads a number that apps send as a JSON number or string.
type looseFloat float64

func (n *looseFloat) UnmarshalJSON(data []byte) error {
	value, err := strconv.ParseFloat(strings.TrimSpace(strings.Trim(string(data), `"`)), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return errors.New("not a number")
	}
	*n = looseFloat(value)
	return nil
}

// scheduleDay reads a DynamicDayOfWeek as Jellyfin does: its name, in any
// case, or its number.
func scheduleDay(raw json.RawMessage) (string, bool) {
	var name string
	if json.Unmarshal(raw, &name) == nil {
		i := slices.IndexFunc(accounts.ScheduleDays, func(day string) bool { return strings.EqualFold(day, name) })
		return accounts.ScheduleDays[max(i, 0)], i >= 0
	}
	var number int
	if json.Unmarshal(raw, &number) == nil && number >= 0 && number < len(accounts.ScheduleDays) {
		return accounts.ScheduleDays[number], true
	}
	return "", false
}

// libraryAccess reports the user's visible libraries in policy, as
// Jellyfin's EnableAllFolders and EnabledFolders do: all while the user
// hides none of libraries, the server's, else those they see.
func libraryAccess(policy *UserPolicy, user accounts.User, libraries []library.ServerLibrary) {
	policy.EnableAllFolders, policy.EnabledFolders = true, []string{}
	for _, l := range libraries {
		if slices.Contains(user.HiddenLibraries, l.ID) {
			policy.EnableAllFolders = false
		} else {
			policy.EnabledFolders = append(policy.EnabledFolders, l.ID.String())
		}
	}
	if policy.EnableAllFolders {
		policy.EnabledFolders = []string{}
	}
}

// hiddenLibraries returns the server's libraries a policy leaves out: none
// with EnableAllFolders, else those EnabledFolders does not name. Like
// Jellyfin's, a library added later is not among EnabledFolders; unlike
// Jellyfin's, Polyfin keeps the libraries hidden rather than those shown,
// so that the user sees it.
func (h *Handler) hiddenLibraries(ctx context.Context, policy policyUpdate, enabled []accounts.ID) ([]accounts.ID, error) {
	hidden := []accounts.ID{}
	if policy.EnableAllFolders {
		return hidden, nil
	}
	libraries, err := h.Library.ServerLibraries(ctx)
	if err != nil {
		return nil, err
	}
	for _, l := range libraries {
		if !slices.Contains(enabled, l.ID) {
			hidden = append(hidden, l.ID)
		}
	}
	return hidden, nil
}

// updatePolicy sets a user's policy from an administrator's app: whether
// the user is an administrator, hidden or disabled, their parental control,
// whether they may have video or audio converted and download, the
// server's libraries they see and their allowed hours. Like Jellyfin, the
// policy replaces the stored one whole, and disabling a user signs out
// their devices but the one that asked.
func (h *Handler) updatePolicy(w http.ResponseWriter, r *http.Request) {
	caller := callerFrom(r.Context())
	// Jellyfin checks the caller's rights before reading the request.
	if !caller.User.IsAdministrator {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if !jsonContent(r.Header.Get("Content-Type")) {
		unsupportedMediaTypeProblem(w)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		processingError(w, http.StatusRequestEntityTooLarge)
		return
	}
	errs := bindErrors{}
	id := errs.pathID(r, "userId")
	policy := policyUpdate{EnableVideoPlaybackTranscoding: true, EnableAudioPlaybackTranscoding: true, EnableContentDownloading: true,
		EnableLiveTvAccess: true, SyncPlayAccess: syncPlayAccessValue{access: accounts.SyncPlayCreateAndJoin, known: true},
		EnableAllFolders: true}
	var enabled []accounts.ID
	var schedules []accounts.AccessSchedule
	if len(bytes.TrimSpace(raw)) == 0 {
		errs.add("", "A non-empty request body is required.")
		errs.add("newPolicy", "The newPolicy field is required.")
	} else if json.Unmarshal(raw, &policy) != nil {
		errs.add("$", "The JSON value could not be converted.")
		errs.add("newPolicy", "The newPolicy field is required.")
	} else if i := slices.IndexFunc(policy.BlockUnratedItems, unknownUnratedKind); i >= 0 {
		// Jellyfin reads these as an enumeration, which refuses other names;
		// its message also gives the position in the request.
		path := fmt.Sprintf("$.BlockUnratedItems[%d]", i)
		errs.add(path, "The JSON value could not be converted to Jellyfin.Data.Enums.UnratedItem. Path: "+path+".")
		errs.add("newPolicy", "The newPolicy field is required.")
	} else if !policy.SyncPlayAccess.known {
		errs.add("$.SyncPlayAccess", "The JSON value could not be converted to Jellyfin.Database.Implementations.Enums.SyncPlayUserAccessType. Path: $.SyncPlayAccess.")
		errs.add("newPolicy", "The newPolicy field is required.")
	} else if policy.MaxActiveSessions > accounts.MaxMaxPlaybacks {
		// Jellyfin takes any number; Polyfin's limit stops at 20, and says
		// so as ASP.NET's range validation would.
		errs.add("MaxActiveSessions", fmt.Sprintf("The field MaxActiveSessions must be between %d and %d.", accounts.MinMaxPlaybacks, accounts.MaxMaxPlaybacks))
	} else if policy.RemoteClientBitrateLimit > accounts.MaxMaxBitrate {
		// Jellyfin reads the limit as a 32-bit number.
		errs.add("$.RemoteClientBitrateLimit", "The JSON value could not be converted to System.Int32. Path: $.RemoteClientBitrateLimit.")
		errs.add("newPolicy", "The newPolicy field is required.")
	} else {
		enabled, schedules = policyContent(policy, errs)
	}
	if len(errs) > 0 {
		validationProblem(w, errs)
		return
	}
	user, err := h.Accounts.User(r.Context(), id)
	if errors.Is(err, accounts.ErrNotFound) {
		notFoundProblem(w)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if policy.IsDisabled && user.IsAdministrator {
		writeJSON(w, http.StatusForbidden, administratorDisabled)
		return
	}
	hidden, err := h.hiddenLibraries(r.Context(), policy, enabled)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	_, err = h.Accounts.UpdateUserFromDevice(r.Context(), id, accounts.UserChanges{
		IsAdministrator: &policy.IsAdministrator,
		IsHidden:        &policy.IsHidden,
		IsDisabled:      &policy.IsDisabled,
		Parental: &accounts.ParentalControl{
			MaxRating:    policy.MaxParentalRating,
			MaxSubRating: policy.MaxParentalSubRating,
			BlockUnrated: policy.BlockUnratedItems,
		},
		VideoTranscoding:   &policy.EnableVideoPlaybackTranscoding,
		AudioTranscoding:   &policy.EnableAudioPlaybackTranscoding,
		ContentDownloading: &policy.EnableContentDownloading,
		// Jellyfin reads a limit of zero or less as none.
		MaxPlaybacks:  new(max(policy.MaxActiveSessions, 0)),
		MaxBitrate:    new(max(policy.RemoteClientBitrateLimit, 0)),
		LiveTv:        &policy.EnableLiveTvAccess,
		SyncPlay:      &policy.SyncPlayAccess.access,
		RemoteControl: &policy.EnableRemoteControlOfOtherUsers,
		// The server's libraries the user does not see, and their hours.
		HiddenLibraries: &hidden,
		AccessSchedules: &schedules,
	}, caller.Device.ID)
	switch {
	case errors.Is(err, accounts.ErrLastAdministrator):
		writeJSON(w, http.StatusForbidden, lastAdministrator)
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func unknownUnratedKind(kind string) bool {
	return !slices.ContainsFunc(accounts.UnratedKinds, func(known string) bool { return strings.EqualFold(known, kind) })
}

// syncPlayAccessValue reads Jellyfin's SyncPlayUserAccessType as Jellyfin
// reads its enumerations: a name, without regard to case, or its number.
// known is false for anything else.
type syncPlayAccessValue struct {
	access accounts.SyncPlayAccess
	known  bool
}

func (v *syncPlayAccessValue) UnmarshalJSON(data []byte) error {
	*v = syncPlayAccessValue{}
	var name string
	if json.Unmarshal(data, &name) == nil {
		for _, access := range accounts.SyncPlayAccesses {
			if strings.EqualFold(name, string(access)) {
				*v = syncPlayAccessValue{access: access, known: true}
			}
		}
		return nil
	}
	var number int
	if json.Unmarshal(data, &number) == nil && number >= 0 && number < len(accounts.SyncPlayAccesses) {
		*v = syncPlayAccessValue{access: accounts.SyncPlayAccesses[number], known: true}
	}
	return nil
}

// policyContent reads the libraries a policy enables and its schedules,
// adding to errs what Jellyfin refuses: a library identifier that is not
// one, or a day it does not know. Jellyfin also takes any hours; Polyfin
// refuses those outside 0 to 24, or that end before they start, which
// jellyfin-web never sends, as they allow no time.
func policyContent(policy policyUpdate, errs bindErrors) ([]accounts.ID, []accounts.AccessSchedule) {
	enabled := make([]accounts.ID, 0, len(policy.EnabledFolders))
	for i, raw := range policy.EnabledFolders {
		id, ok := parseGUID(raw)
		if !ok {
			path := fmt.Sprintf("$.EnabledFolders[%d]", i)
			errs.add(path, "The JSON value could not be converted to System.Guid. Path: "+path+".")
			errs.add("newPolicy", "The newPolicy field is required.")
			return nil, nil
		}
		enabled = append(enabled, id)
	}
	schedules := make([]accounts.AccessSchedule, 0, len(policy.AccessSchedules))
	for i, schedule := range policy.AccessSchedules {
		day, ok := scheduleDay(schedule.DayOfWeek)
		start, end := float64(schedule.StartHour), float64(schedule.EndHour)
		switch {
		case !ok:
			path := fmt.Sprintf("$.AccessSchedules[%d].DayOfWeek", i)
			errs.add(path, "The JSON value could not be converted to Jellyfin.Database.Implementations.Enums.DynamicDayOfWeek. Path: "+path+".")
		case start < accounts.MinScheduleHour || end > accounts.MaxScheduleHour || start >= end:
			errs.add(fmt.Sprintf("$.AccessSchedules[%d]", i), "StartHour and EndHour must be hours from 0 to 24, StartHour before EndHour.")
		default:
			schedules = append(schedules, accounts.AccessSchedule{Day: day, StartHour: start, EndHour: end})
			continue
		}
		errs.add("newPolicy", "The newPolicy field is required.")
		return nil, nil
	}
	return enabled, schedules
}
