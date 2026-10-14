package notifications

import (
	"fmt"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/reminders"
)

// ProgrammeJSON is the Live TV programme a reminder tells of: its item and
// its channel's, as Jellyfin apps know them, and when it starts and ends.
type ProgrammeJSON struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ChannelID   string    `json:"channelId"`
	ChannelName string    `json:"channelName"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
}

// ProgrammeStarting tells the user of a reminder due, on their own targets
// only, that its programme starts soon, and returns the event's title and
// message, which the user's apps may show too. They are written in the
// reminder's language, else the server's, the start in the server's time
// zone; the link opens the programme's channel in the web client. It
// returns at once: the message is sent in the background.
func (s *Service) ProgrammeStarting(due reminders.Due) (title, message string) {
	r := due.Reminder
	ev := s.newEvent(ProgrammeReminder)
	ev.User = &UserJSON{ID: due.User.ID.String(), Name: due.User.Name}
	ev.Programme = &ProgrammeJSON{ID: r.Program.String(), Name: r.Name, ChannelID: r.Channel.String(), ChannelName: r.ChannelName,
		Start: r.Start.UTC(), End: r.End.UTC()}
	french := r.Language == "fr" || r.Language == "" && s.accounts.Settings().Language == "fr"
	say := func(english, inFrench string, args ...any) string {
		if french {
			return fmt.Sprintf(inFrench, args...)
		}
		return fmt.Sprintf(english, args...)
	}
	// The minutes left, rounded up: a reminder is sent before its
	// programme starts.
	minutes := int((due.Left + time.Minute - 1) / time.Minute)
	minutes = max(minutes, 1)
	left := say("%d minutes", "%d minutes", minutes)
	if minutes == 1 {
		left = say("1 minute", "1 minute")
	}
	channel := r.ChannelName
	if channel == "" {
		channel = say("its channel", "sa chaîne")
	}
	at := r.Start.In(s.zone).Format("15:04")
	ev.Title = say("Starting at %s: %s", "Commence à %s : %s", at, r.Name)
	ev.Message = say("%s starts in %s on %s.", "%s commence dans %s sur %s.", r.Name, left, channel)
	ev.URL = s.webLink(r.Channel)
	user := due.User.ID
	s.dispatch(ev, func(t target) bool { return t.owner != nil && *t.owner == user })
	return ev.Title, ev.Message
}

// Receives reports whether one of user's own targets, turned on, receives
// event.
func (s *Service) Receives(user accounts.ID, event string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.targets {
		if t.owner != nil && *t.owner == user && t.wants(event) {
			return true
		}
	}
	return false
}
