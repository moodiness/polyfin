// Package discovery answers the messages Jellyfin apps broadcast on the
// local network to find servers, as Jellyfin's auto discovery does: an app
// sends "who is JellyfinServer?" to UDP port 7359, Emby apps "who is
// EmbyServer?", and each server answers the sender with its address, ID
// and name, which the app lists among the servers found.
package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"sync"
)

// Port is the UDP port Jellyfin apps send their discovery messages to.
const Port = 7359

// messages are the discovery messages answered, in lower case: Jellyfin
// apps send the first, Emby apps the second. As Jellyfin 12.2 does for its
// own, a datagram that holds one, in any letter case, is answered, once.
var messages = [][]byte{[]byte("who is jellyfinserver?"), []byte("who is embyserver?")}

// maxMessage is how much of a datagram is read: the messages are far
// shorter.
const maxMessage = 1024

// Answer is what a discovery message is answered with: Jellyfin's
// ServerDiscoveryInfo, in its order, EndpointAddress always null as
// Jellyfin leaves it.
type Answer struct {
	Address         string
	Id              string
	Name            string
	EndpointAddress *string
}

// Settings are what the answers follow, read at every message.
type Settings struct {
	// Enabled answers the messages; off, the port is left free.
	Enabled bool
	// Name is the server's name, as /System/Info/Public gives it.
	Name string
	// Address is the address announced, such as http://192.168.1.10:8096;
	// empty announces the address of the interface that reaches the app.
	Address string
}

// Options are what Start needs.
type Options struct {
	// Port is the UDP port listened on: Port, or 0 for a free one, kept
	// from then on.
	Port int
	// HTTP is the address Polyfin's HTTP server listens on: the address
	// announced has its port, and its IP when it listens on one only.
	HTTP *net.TCPAddr
	// ServerID is the server's ID, as /System/Info/Public gives it.
	ServerID string
	// Settings returns the current settings.
	Settings func() Settings
	// Changed returns a channel closed at the next change of the settings.
	Changed func() <-chan struct{}
	// InContainer tells that Polyfin runs in a container (see
	// InContainer), where the address announced may be the container's.
	InContainer bool
	Logger      *slog.Logger

	// route returns the local address the system reaches remote from;
	// routeTo by default.
	route func(remote netip.Addr) (netip.Addr, error)
	// networks returns the networks of the system's interfaces;
	// interfaceNetworks by default.
	networks func() ([]netip.Prefix, error)
	// applied, when set, is told once a change of the settings is applied.
	applied func()
}

// Service answers the discovery messages while the settings turn it on.
type Service struct {
	options Options
	// mu guards what follows.
	mu sync.Mutex
	// port is the UDP port listened on, or tried.
	port int
	// conn listens for the messages, nil while it does not.
	conn *net.UDPConn
	// failure is why the port could not be listened on, nil while it is,
	// or while discovery is off.
	failure error
	// container is the address the last answer announced when it is the
	// container's (see offNetwork), else empty.
	container string
}

// Start listens for the discovery messages while the settings turn
// discovery on, until ctx ends, following the settings without a restart.
// A port that cannot be listened on, often because another server such as
// Jellyfin uses it, never stops Polyfin: it is logged, Status tells it, and
// it is tried again at each change of the settings.
func Start(ctx context.Context, options Options) *Service {
	if options.route == nil {
		options.route = routeTo
	}
	if options.networks == nil {
		options.networks = interfaceNetworks
	}
	s := &Service{options: options, port: options.Port}
	changed := options.Changed()
	s.apply()
	go s.follow(ctx, changed)
	return s
}

// follow applies each change of the settings until ctx ends, which stops
// the listening.
func (s *Service) follow(ctx context.Context, changed <-chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.stop()
			s.mu.Unlock()
			return
		case <-changed:
			changed = s.options.Changed()
			s.apply()
			if s.options.applied != nil {
				s.options.applied()
			}
		}
	}
}

// apply listens while discovery is on, and frees the port while it is off.
func (s *Service) apply() {
	enabled := s.options.Settings().Enabled
	s.mu.Lock()
	defer s.mu.Unlock()
	if !enabled {
		s.stop()
		return
	}
	if s.conn != nil {
		return
	}
	// Every interface, IPv4 and IPv6 alike, as broadcasts arrive on the
	// wildcard address only.
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: s.port})
	if err != nil {
		s.failure = err
		s.options.Logger.Warn("Polyfin cannot listen on this UDP port for local network discovery: Jellyfin apps on the local network do "+
			"not find it by themselves. Another program, such as a Jellyfin server, may use it", "port", s.port, "error", err)
		return
	}
	s.conn, s.failure = conn, nil
	s.port = conn.LocalAddr().(*net.UDPAddr).Port
	s.options.Logger.Info("Answering local network discovery", "port", s.port)
	go s.serve(conn)
}

// stop frees the port, and forgets what the answers found.
func (s *Service) stop() {
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
	s.failure, s.container = nil, ""
}

// serve answers the messages conn receives until it is closed.
func (s *Service) serve(conn *net.UDPConn) {
	buffer := make([]byte, maxMessage)
	for {
		n, sender, err := conn.ReadFromUDPAddrPort(buffer)
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			s.options.Logger.Debug("A local network discovery message could not be read", "error", err)
			continue
		}
		answer := s.answer(buffer[:n], sender)
		if answer == nil {
			continue
		}
		if _, err := conn.WriteToUDPAddrPort(answer, sender); err != nil {
			s.options.Logger.Debug("A local network discovery answer could not be sent", "to", sender, "error", err)
		}
	}
}

// answer returns the answer to message, sent by sender, or nil for none:
// only discovery messages are answered, while discovery is on, and only to
// addresses of the local network, so that a forwarded port does not answer
// the internet.
func (s *Service) answer(message []byte, sender netip.AddrPort) []byte {
	settings := s.options.Settings()
	from := sender.Addr().Unmap()
	if !settings.Enabled || sender.Port() == 0 || !onLocalNetwork(from) || !discoveryMessage(message) {
		return nil
	}
	address, container := settings.Address, ""
	if address == "" {
		local, err := s.announced(from)
		if err != nil {
			s.options.Logger.Debug("No local address reaches the sender of a local network discovery message", "from", from, "error", err)
			return nil
		}
		address = "http://" + netip.AddrPortFrom(local.WithZone(""), uint16(s.options.HTTP.Port)).String()
		if s.options.InContainer && s.offNetwork(local, from) {
			container = address
		}
	}
	s.mu.Lock()
	s.container = container
	s.mu.Unlock()
	body, err := json.Marshal(Answer{Address: address, Id: s.options.ServerID, Name: settings.Name})
	if err != nil {
		return nil
	}
	return body
}

// announced is the IP the address announced to from has: the HTTP
// server's when it listens on one only, else the one the system reaches
// from from.
func (s *Service) announced(from netip.Addr) (netip.Addr, error) {
	if listened, ok := netip.AddrFromSlice(s.options.HTTP.IP); ok && !listened.IsUnspecified() {
		return listened.Unmap(), nil
	}
	return s.options.route(from)
}

// offNetwork reports whether from is outside the network of the interface
// local belongs to. In a container on a bridge network, the address
// announced is the container's own, on the bridge's network, and the
// messages come from elsewhere: from apps on the local network through a
// published port, or from the forwarder of Docker Desktop. With host
// networking, the interface that reaches an app on the local network is on
// its network. An address on no known interface is not told.
func (s *Service) offNetwork(local, from netip.Addr) bool {
	networks, err := s.options.networks()
	if err != nil {
		return false
	}
	for _, network := range networks {
		if network.Addr().WithZone("") == local.WithZone("") {
			return !network.Contains(from.WithZone(""))
		}
	}
	return false
}

// Status is how discovery goes, for System › Health.
type Status struct {
	// Port is the UDP port listened on, or tried.
	Port int
	// Unavailable tells that discovery is on but its port could not be
	// listened on.
	Unavailable bool
	// ContainerAddress is the address the last answer announced, when it
	// is the container's: apps on the local network cannot reach it. It is
	// empty when the settings give the address to announce.
	ContainerAddress string
}

// Status returns how discovery goes.
func (s *Service) Status() Status {
	settings := s.options.Settings()
	s.mu.Lock()
	defer s.mu.Unlock()
	status := Status{Port: s.port, Unavailable: s.failure != nil}
	if settings.Enabled && settings.Address == "" {
		status.ContainerAddress = s.container
	}
	return status
}

// discoveryMessage reports whether message holds a discovery message, in
// any letter case.
func discoveryMessage(message []byte) bool {
	lower := bytes.ToLower(message)
	for _, m := range messages {
		if bytes.Contains(lower, m) {
			return true
		}
	}
	return false
}

// onLocalNetwork reports whether address is on the local network: private
// IPv4 ranges and IPv6 unique local addresses, link-local addresses, and
// the server's own loopback.
func onLocalNetwork(address netip.Addr) bool {
	return address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsLoopback()
}

// routeTo returns the local address the system reaches remote from, as
// the routing table gives it: a UDP socket connected to remote sends
// nothing.
func routeTo(remote netip.Addr) (netip.Addr, error) {
	conn, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(netip.AddrPortFrom(remote, Port)))
	if err != nil {
		return netip.Addr{}, err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).AddrPort().Addr().Unmap(), nil
}

// interfaceNetworks returns the address and network of each address of
// the system's interfaces.
func interfaceNetworks() ([]netip.Prefix, error) {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	var networks []netip.Prefix
	for _, address := range addresses {
		network, ok := address.(*net.IPNet)
		if !ok {
			continue
		}
		ip, ok := netip.AddrFromSlice(network.IP)
		ones, bits := network.Mask.Size()
		if !ok || bits == 0 {
			continue
		}
		if ip.Is4In6() && bits == 128 {
			ones -= 96
		}
		networks = append(networks, netip.PrefixFrom(ip.Unmap(), ones))
	}
	return networks, nil
}

// InContainer reports whether Polyfin runs in a container: Docker makes
// /.dockerenv in its containers, Podman /run/.containerenv.
func InContainer() bool {
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	return false
}
