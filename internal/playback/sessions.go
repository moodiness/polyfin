package playback

import (
	"cmp"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// PlayState is what a player reports about the item it plays.
type PlayState struct {
	Item          accounts.ID
	MediaSourceID string
	PlaySessionID string
	PlayMethod    string
	Position      time.Duration
	Paused        bool
	Muted         bool
	CanSeek       bool
	RepeatMode    string
	PlaybackOrder string
	// AudioStreamIndex and SubtitleStreamIndex are nil when not reported.
	AudioStreamIndex    *int
	SubtitleStreamIndex *int
	VolumeLevel         *int
}

// NowPlaying is what one device plays, as last reported.
type NowPlaying struct {
	PlayState
	User      accounts.ID
	Started   time.Time
	CheckedIn time.Time
	// PausedAt is when the current pause began, zero while playing.
	PausedAt time.Time
}

// Sessions tracks what each signed-in device plays, from the reports
// Jellyfin apps send. Devices are known by their sign-in, so one device
// plays one item at a time.
type Sessions struct {
	now func() time.Time

	mu      sync.Mutex
	devices map[accounts.ID]*NowPlaying
}

// NewSessions returns an empty tracker.
func NewSessions() *Sessions {
	return &Sessions{now: time.Now, devices: map[accounts.ID]*NowPlaying{}}
}

// Start records that a device started playing.
func (s *Sessions) Start(device, user accounts.ID, state PlayState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices[device] = s.started(user, state)
}

func (s *Sessions) started(user accounts.ID, state PlayState) *NowPlaying {
	now := s.now()
	playing := &NowPlaying{PlayState: withDefaults(state, PlayState{}), User: user, Started: now, CheckedIn: now}
	if state.Paused {
		playing.PausedAt = now
	}
	return playing
}

// Progress records a device's progress. A report for another item than the
// one known starts it, as when a player skips the start report. Fields a
// report leaves out keep their value, except CanSeek, which Jellyfin resets.
func (s *Sessions) Progress(device, user accounts.ID, state PlayState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.devices[device]
	if !ok || (state.Item != (accounts.ID{}) && current.Item != state.Item) {
		s.devices[device] = s.started(user, state)
		return
	}
	now := s.now()
	switch {
	case !state.Paused:
		current.PausedAt = time.Time{}
	case !current.Paused:
		current.PausedAt = now
	}
	current.PlayState = withDefaults(state, current.PlayState)
	current.CheckedIn = now
}

// Stop records that a device stopped playing.
func (s *Sessions) Stop(device accounts.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.devices, device)
}

// Playing returns what a device plays. While it is not paused, the position
// moves on from the last report, as Jellyfin shows it.
func (s *Sessions) Playing(device accounts.ID) (NowPlaying, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.devices[device]
	if !ok {
		return NowPlaying{}, false
	}
	result := *current
	if !result.Paused {
		result.Position += s.now().Sub(result.CheckedIn)
	}
	return result, true
}

// All returns what every device plays, by device, as last reported.
func (s *Sessions) All() map[accounts.ID]NowPlaying {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := make(map[accounts.ID]NowPlaying, len(s.devices))
	for device, playing := range s.devices {
		all[device] = *playing
	}
	return all
}

// withDefaults completes a report with what the previous one said.
func withDefaults(state, previous PlayState) PlayState {
	if state.Item == (accounts.ID{}) {
		state.Item = previous.Item
	}
	state.MediaSourceID = cmp.Or(state.MediaSourceID, previous.MediaSourceID)
	state.PlaySessionID = cmp.Or(state.PlaySessionID, previous.PlaySessionID)
	state.PlayMethod = cmp.Or(state.PlayMethod, previous.PlayMethod)
	if state.AudioStreamIndex == nil {
		state.AudioStreamIndex = previous.AudioStreamIndex
	}
	if state.SubtitleStreamIndex == nil {
		state.SubtitleStreamIndex = previous.SubtitleStreamIndex
	}
	if state.VolumeLevel == nil {
		state.VolumeLevel = previous.VolumeLevel
	}
	state.RepeatMode = cmp.Or(state.RepeatMode, previous.RepeatMode, "RepeatNone")
	state.PlaybackOrder = cmp.Or(state.PlaybackOrder, previous.PlaybackOrder, "Default")
	return state
}
