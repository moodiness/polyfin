package notifications

import (
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/i18n"
)

// groupLine bounds each event's line in a grouped message, in characters.
const groupLine = 200

// groupedEvent is the one message of events a target grouped: titled by
// how many there are, each told on a line of its own, its title then its
// message, oldest first. A webhook receives each event whole in Events.
func (s *Service) groupedEvent(events []Event) Event {
	ev := s.newEvent(Grouped)
	ev.Events = events
	ev.Title = s.phrase(i18n.Text{"%s notifications", "%s notifications", "%s Benachrichtigungen", "%s notificaciones", "%s notifiche", "%s notificações"},
		strconv.Itoa(len(events)))
	lines := make([]string, 0, len(events))
	for _, e := range events {
		line := e.Title
		if e.Message != "" {
			line += " · " + strings.ReplaceAll(e.Message, "\n", " ")
		}
		lines = append(lines, "• "+clip(line, groupLine))
	}
	ev.Message = strings.Join(lines, "\n")
	return ev
}
