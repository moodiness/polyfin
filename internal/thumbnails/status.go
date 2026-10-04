package thumbnails

import (
	"slices"
	"strings"
	"time"
)

// Status is where the service's work stands, as the admin dashboard shows
// it.
type Status struct {
	// Waiting counts the versions waiting for their images, of at most
	// QueueLength; Working is set while the images of one are made.
	Waiting     int
	QueueLength int
	Working     bool
	// Paused are the hosts that asked to slow down, whose images wait.
	Paused []PausedHost
}

// PausedHost is a host whose images are paused until a time.
type PausedHost struct {
	Host  string
	Until time.Time
}

// Status tells where the service's work stands.
func (s *Service) Status() Status {
	s.mu.Lock()
	status := Status{Waiting: len(s.jobs), QueueLength: queueLength, Working: s.working}
	s.mu.Unlock()
	status.Paused = s.gate.pausedHosts()
	return status
}

func (s *Service) setWorking(working bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.working = working
}

// pausedHosts lists the hosts paused now, by name.
func (g *gate) pausedHosts() []PausedHost {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	paused := []PausedHost{}
	for host, state := range g.hosts {
		if now.Before(state.paused) {
			paused = append(paused, PausedHost{Host: host, Until: state.paused})
		}
	}
	slices.SortFunc(paused, func(a, b PausedHost) int { return strings.Compare(a.Host, b.Host) })
	return paused
}
