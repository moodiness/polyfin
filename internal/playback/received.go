package playback

import (
	"io"
	"sync"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/bandwidth"
)

// CountReceived has the bytes the versions' sources send Polyfin counted in
// meters, by version: those the source cache reads, of files, torrents
// and network shares, and those relayed past it, of links read as players
// ask and of live streams. It is called before the service plays.
func (s *Service) CountReceived(meters *bandwidth.Meters) {
	s.received = meters
	s.sources.Count(meters)
}

// ReceivedRate is the bytes per second the source of version sends Polyfin,
// averaged over bandwidth.Window; 0 when nothing is counted.
func (s *Service) ReceivedRate(version accounts.ID) float64 {
	if s.received == nil {
		return 0
	}
	return s.received.Rate(version.String())
}

// ReceivedTotal is the bytes per second every source sends Polyfin,
// averaged over bandwidth.Window; 0 when nothing is counted.
func (s *Service) ReceivedTotal() float64 {
	if s.received == nil {
		return 0
	}
	return s.received.TotalRate()
}

// countReceived counts what is read of body under version, until it is
// closed; body as it is when nothing is counted.
func (s *Service) countReceived(version accounts.ID, body io.ReadCloser) io.ReadCloser {
	if s.received == nil {
		return body
	}
	return &receivedBody{ReadCloser: body, meters: s.received, meter: s.received.Hold(version.String())}
}

// receivedBody is a source's body whose reads are counted.
type receivedBody struct {
	io.ReadCloser
	meters *bandwidth.Meters
	meter  *bandwidth.Meter
	once   sync.Once
}

func (b *receivedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.meter.Add(n)
	return n, err
}

func (b *receivedBody) Close() error {
	b.once.Do(func() { b.meters.Release(b.meter) })
	return b.ReadCloser.Close()
}
