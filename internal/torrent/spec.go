package torrent

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ErrInvalidSpec reports a torrent stream Polyfin cannot play: no valid
// info hash.
var ErrInvalidSpec = errors.New("invalid torrent stream")

// Spec names the file of a torrent a version plays, as an addon lists it:
// its info hash, the file's index, -1 when the addon gives none, and where
// to find peers besides: the trackers and DHT nodes of the stream's
// sources, and whether to look the torrent up on the DHT. Without an
// index, Name, the file's name the addon gives, and Season and Episode,
// the episode played (Episode 0 for another title), tell the file (see
// Info.File).
type Spec struct {
	Hash            Hash
	File            int
	Name            string
	Season, Episode int
	Trackers        []string
	Nodes           []string
	DHT             bool
}

// Bounds on what an addon's sources may add.
const (
	maxTrackers = 30
	maxNodes    = 20
)

// NewSpec reads an addon's torrent stream: its infoHash, its fileIdx (nil
// when it names none) and its sources, entries "tracker:<url>" for HTTP,
// HTTPS and UDP trackers, and "dht:<info hash>" or "dht:<host>:<port>" for
// the DHT. Other entries are ignored.
func NewSpec(infoHash string, fileIdx *int, sources []string) (Spec, error) {
	hash, err := ParseHash(infoHash)
	if err != nil {
		return Spec{}, err
	}
	spec := Spec{Hash: hash, File: -1}
	if fileIdx != nil {
		if *fileIdx < 0 {
			return Spec{}, ErrInvalidSpec
		}
		spec.File = *fileIdx
	}
	for _, entry := range sources {
		kind, value, _ := strings.Cut(strings.TrimSpace(entry), ":")
		switch kind {
		case "tracker":
			if validTracker(value) && len(spec.Trackers) < maxTrackers && !contains(spec.Trackers, value) {
				spec.Trackers = append(spec.Trackers, value)
			}
		case "dht":
			if _, err := ParseHash(value); err == nil {
				spec.DHT = true
				continue
			}
			if validNode(value) && len(spec.Nodes) < maxNodes && !contains(spec.Nodes, value) {
				spec.DHT = true
				spec.Nodes = append(spec.Nodes, value)
			}
		}
	}
	return spec, nil
}

func validTracker(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return false
	}
	switch u.Scheme {
	case "http", "https":
		return true
	case "udp":
		return u.Port() != ""
	}
	return false
}

func validNode(hostport string) bool {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil || host == "" {
		return false
	}
	n, err := strconv.Atoi(port)
	return err == nil && n > 0 && n < 65536
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}
