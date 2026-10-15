package dlna

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/ipv4"
)

// Port is the UDP port of SSDP, on which DLNA devices search for servers
// and servers announce themselves.
const Port = 1900

// group is SSDP's multicast group.
var group = net.IPv4(239, 255, 255, 250)

const (
	deviceType = "urn:schemas-upnp-org:device:MediaServer:1"
	// ContentDirectory and ConnectionManager are the services described.
	contentDirectory  = "urn:schemas-upnp-org:service:ContentDirectory:1"
	connectionManager = "urn:schemas-upnp-org:service:ConnectionManager:1"
	// maxAge is how long devices keep an announcement, in seconds: it is
	// sent again well before.
	maxAge         = 1800
	announceEvery  = 10 * time.Minute
	maxDatagram    = 4096
	descriptionMax = 64 << 10
	// describeEvery is how long a device's description, once read or
	// tried, is not read again, whatever it announces meanwhile.
	describeEvery = 10 * time.Minute
	// maxDescribed bounds the devices whose descriptions are kept.
	maxDescribed = 256
)

// ssdp answers the searches of devices on the local network and announces
// the server to them, while the settings share the libraries. A port that
// cannot be listened on never stops Polyfin: it is logged, Status tells it,
// and it is tried again at each change of the settings.
type ssdp struct {
	s *Service
	// mu guards what follows.
	mu sync.Mutex
	// port is the UDP port listened on, or tried.
	port    int
	conn    *net.UDPConn
	packet  *ipv4.PacketConn
	failure error
	// stopAnnouncing ends the announcements of the listening.
	stopAnnouncing context.CancelFunc
	// described are the names devices announced at their addresses, which
	// their first request takes, and the reads of their descriptions.
	described map[netip.Addr]*described
}

// described is what a device's description gave, read at, unless it is
// being read.
type described struct {
	sighting Sighting
	ok       bool
	reading  bool
	at       time.Time
}

// apply listens while sharing is on, and frees the port while it is off.
func (d *ssdp) apply(ctx context.Context) {
	enabled := d.s.options.Settings().DLNA
	d.mu.Lock()
	defer d.mu.Unlock()
	if !enabled {
		d.stop()
		return
	}
	if d.conn != nil {
		return
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: d.port})
	if err != nil {
		d.failure = err
		d.s.options.Logger.Warn("Polyfin cannot listen on this UDP port for DLNA: devices on the local network do not find it by themselves. "+
			"Another program, such as another DLNA server, may use it", "port", d.port, "error", err)
		return
	}
	packet := ipv4.NewPacketConn(conn)
	_ = packet.SetMulticastLoopback(true)
	_ = packet.SetMulticastTTL(2)
	joined := 0
	for _, ifi := range multicastInterfaces() {
		if packet.JoinGroup(&ifi, &net.UDPAddr{IP: group}) == nil {
			joined++
		}
	}
	if joined == 0 {
		d.s.options.Logger.Info("DLNA joined no multicast group: only devices that search the server directly find it")
	}
	d.conn, d.packet, d.failure = conn, packet, nil
	d.port = conn.LocalAddr().(*net.UDPAddr).Port
	d.s.options.Logger.Info("Sharing the libraries over DLNA", "port", d.port)
	go d.serve(conn)
	announcing, stop := context.WithCancel(ctx)
	d.stopAnnouncing = stop
	go d.announce(announcing, packet, d.port)
}

// stop frees the port, saying goodbye to the devices.
func (d *ssdp) stop() {
	if d.stopAnnouncing != nil {
		d.stopAnnouncing()
		d.stopAnnouncing = nil
	}
	if d.conn != nil {
		d.notify(d.packet, "ssdp:byebye", d.port)
		_ = d.conn.Close()
		d.conn, d.packet = nil, nil
	}
	d.failure = nil
}

// serve answers the searches conn receives until it is closed, and notes
// what devices announce.
func (d *ssdp) serve(conn *net.UDPConn) {
	buffer := make([]byte, maxDatagram)
	for {
		n, sender, err := conn.ReadFromUDPAddrPort(buffer)
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			continue
		}
		message := buffer[:n]
		if bytes.HasPrefix(message, []byte("NOTIFY ")) {
			d.heard(message, sender.Addr().Unmap())
			continue
		}
		for _, answer := range d.s.answer(message, sender) {
			if _, err := conn.WriteToUDPAddrPort(answer, sender); err != nil {
				d.s.options.Logger.Debug("An SSDP answer could not be sent", "to", sender, "error", err)
			}
		}
	}
}

// searchRequest reads an SSDP search: its target, and whether it is one.
func searchRequest(message []byte) (string, bool) {
	request, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(message)))
	if err != nil || request.Method != "M-SEARCH" || !strings.EqualFold(strings.Trim(request.Header.Get("Man"), `"`), "ssdp:discover") {
		return "", false
	}
	return strings.TrimSpace(request.Header.Get("St")), true
}

// targets lists what the server is, as searches name it, the most general
// first: its root device, its UUID, its device type and its services.
func (s *Service) targets() []string {
	return []string{"upnp:rootdevice", "uuid:" + s.udn(), deviceType, contentDirectory, connectionManager}
}

// answer returns the answers to an SSDP search sent by sender, none to a
// message that is not one, while sharing is off, or from an address that is
// not of the local network, so that a forwarded port does not answer the
// internet.
func (s *Service) answer(message []byte, sender netip.AddrPort) [][]byte {
	from := sender.Addr().Unmap()
	if !s.options.Settings().DLNA || sender.Port() == 0 || !onLocalNetwork(from) {
		return nil
	}
	target, ok := searchRequest(message)
	if !ok {
		return nil
	}
	location, err := s.location(from)
	if err != nil {
		s.options.Logger.Debug("No local address reaches the sender of an SSDP search", "from", from, "error", err)
		return nil
	}
	var answers [][]byte
	for _, t := range s.targets() {
		if target != "ssdp:all" && !strings.EqualFold(target, t) {
			continue
		}
		answers = append(answers, []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nCACHE-CONTROL: max-age=%d\r\nDATE: %s\r\nEXT:\r\nLOCATION: %s\r\n"+
			"SERVER: %s\r\nST: %s\r\nUSN: %s\r\nCONTENT-LENGTH: 0\r\n\r\n", maxAge, s.now().UTC().Format(http.TimeFormat), location,
			s.serverHeader(), t, s.usn(t))))
	}
	return answers
}

// usn is the unique service name of a target.
func (s *Service) usn(target string) string {
	if strings.HasPrefix(target, "uuid:") {
		return target
	}
	return "uuid:" + s.udn() + "::" + target
}

// location is the address of the description, on the interface that
// reaches from.
func (s *Service) location(from netip.Addr) (string, error) {
	local, err := s.announced(from)
	if err != nil {
		return "", err
	}
	return "http://" + netip.AddrPortFrom(local.WithZone(""), uint16(s.options.HTTP.Port)).String() + descriptionPath, nil
}

// announced is the IP the description is announced at to from: the HTTP
// server's when it listens on one only, else the one the system reaches
// from from.
func (s *Service) announced(from netip.Addr) (netip.Addr, error) {
	if listened, ok := netip.AddrFromSlice(s.options.HTTP.IP); ok && !listened.IsUnspecified() {
		return listened.Unmap(), nil
	}
	return s.options.route(from)
}

// announce tells the devices of the local network that the server is
// there, three times at first as UDP may lose some, then every
// announceEvery, until ctx ends.
func (d *ssdp) announce(ctx context.Context, packet *ipv4.PacketConn, port int) {
	for i := range 3 {
		d.notify(packet, "ssdp:alive", port)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(i+1) * time.Second):
		}
	}
	ticker := time.NewTicker(announceEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.notify(packet, "ssdp:alive", port)
		}
	}
}

// notify sends the server's announcements of kind, ssdp:alive or
// ssdp:byebye, to port on each interface of the local network, with the
// address of the description there.
func (d *ssdp) notify(packet *ipv4.PacketConn, kind string, port int) {
	if packet == nil {
		return
	}
	s := d.s
	destination := &net.UDPAddr{IP: group, Port: port}
	for _, ifi := range multicastInterfaces() {
		local, ok := interfaceIPv4(ifi)
		if !ok || !onLocalNetwork(local) {
			continue
		}
		if packet.SetMulticastInterface(&ifi) != nil {
			continue
		}
		if listened, ok := netip.AddrFromSlice(s.options.HTTP.IP); ok && !listened.IsUnspecified() {
			local = listened.Unmap()
		}
		location := "http://" + netip.AddrPortFrom(local, uint16(s.options.HTTP.Port)).String() + descriptionPath
		for _, target := range s.targets() {
			message := fmt.Sprintf("NOTIFY * HTTP/1.1\r\nHOST: 239.255.255.250:%d\r\nCACHE-CONTROL: max-age=%d\r\nLOCATION: %s\r\n"+
				"NT: %s\r\nNTS: %s\r\nSERVER: %s\r\nUSN: %s\r\n\r\n", port, maxAge, location, target, kind, s.serverHeader(), s.usn(target))
			_, _ = packet.WriteTo([]byte(message), nil, destination)
		}
	}
}

// heard notes what a device announces of itself: the names its
// description gives, read from its own address only, once at a time, and
// not again for describeEvery.
func (d *ssdp) heard(message []byte, from netip.Addr) {
	location, ok := d.s.renderer(message, from)
	if !ok {
		return
	}
	now := d.s.now()
	d.mu.Lock()
	entry, known := d.described[from]
	if known && (entry.reading || now.Sub(entry.at) < describeEvery) {
		d.mu.Unlock()
		return
	}
	if !known {
		if len(d.described) >= maxDescribed {
			d.forgetDescribed()
		}
		entry = &described{}
		d.described[from] = entry
	}
	entry.reading = true
	d.mu.Unlock()
	go func() {
		sighting, err := describe(location, from)
		d.mu.Lock()
		defer d.mu.Unlock()
		entry.reading, entry.at = false, d.s.now()
		if err == nil {
			entry.sighting, entry.ok = sighting, true
		}
	}()
}

// forgetDescribed forgets the descriptions read, but those being read.
func (d *ssdp) forgetDescribed() {
	for address, entry := range d.described {
		if !entry.reading {
			delete(d.described, address)
		}
	}
}

// renderer returns the description's address of a device's announcement:
// an ssdp:alive of a renderer or a root device, from the local network.
// The server's own announcements, which multicast loops back to it, and
// those of other Polyfin servers, are not devices: they are told by their
// USN, and by their description's path.
func (s *Service) renderer(message []byte, from netip.Addr) (string, bool) {
	if !onLocalNetwork(from) {
		return "", false
	}
	request, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(message)))
	if err != nil || !strings.EqualFold(request.Header.Get("Nts"), "ssdp:alive") {
		return "", false
	}
	nt := request.Header.Get("Nt")
	if !strings.Contains(strings.ToLower(nt), "mediarenderer") && !strings.EqualFold(nt, "upnp:rootdevice") {
		return "", false
	}
	location := request.Header.Get("Location")
	if location == "" || strings.Contains(strings.ToLower(request.Header.Get("Usn")), "uuid:"+strings.ToLower(s.udn())) {
		return "", false
	}
	if address, err := url.Parse(location); err != nil || address.Path == descriptionPath {
		return "", false
	}
	return location, true
}

// describe reads the names a device's description gives, at location on
// its own address.
func describe(location string, from netip.Addr) (Sighting, error) {
	request, err := http.NewRequest(http.MethodGet, location, nil)
	if err != nil {
		return Sighting{}, err
	}
	host, err := netip.ParseAddr(request.URL.Hostname())
	if err != nil || host.Unmap() != from || request.URL.Scheme != "http" {
		return Sighting{}, errors.New("a description elsewhere than its device")
	}
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return Sighting{}, err
	}
	defer response.Body.Close()
	var description struct {
		Device struct {
			FriendlyName string `xml:"friendlyName"`
			Manufacturer string `xml:"manufacturer"`
			ModelName    string `xml:"modelName"`
		} `xml:"device"`
	}
	if err := xml.NewDecoder(io.LimitReader(response.Body, descriptionMax)).Decode(&description); err != nil {
		return Sighting{}, err
	}
	return Sighting{Address: from.String(), Name: description.Device.FriendlyName, Manufacturer: description.Device.Manufacturer,
		Model: description.Device.ModelName}, nil
}

// announcedAt returns what the device at address announced, if it did.
func (d *ssdp) announcedAt(address netip.Addr) (Sighting, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if entry, ok := d.described[address]; ok && entry.ok {
		return entry.sighting, true
	}
	return Sighting{}, false
}

// multicastInterfaces lists the interfaces that are up and take multicast.
func multicastInterfaces() []net.Interface {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var result []net.Interface
	for _, ifi := range interfaces {
		if ifi.Flags&net.FlagUp != 0 && ifi.Flags&net.FlagMulticast != 0 && ifi.Flags&net.FlagLoopback == 0 {
			result = append(result, ifi)
		}
	}
	return result
}

// interfaceIPv4 returns the first IPv4 address of an interface.
func interfaceIPv4(ifi net.Interface) (netip.Addr, bool) {
	addresses, err := ifi.Addrs()
	if err != nil {
		return netip.Addr{}, false
	}
	for _, address := range addresses {
		if prefix, err := netip.ParsePrefix(address.String()); err == nil && prefix.Addr().Is4() {
			return prefix.Addr(), true
		}
	}
	return netip.Addr{}, false
}

// onLocalNetwork reports whether address is on the local network, as
// discovery judges it: private IPv4 ranges and IPv6 unique local addresses,
// link-local addresses, and the server's own loopback.
func onLocalNetwork(address netip.Addr) bool {
	return address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsLoopback()
}

// routeTo returns the local address the system reaches remote from, as the
// routing table gives it: a UDP socket connected to remote sends nothing.
func routeTo(remote netip.Addr) (netip.Addr, error) {
	conn, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(netip.AddrPortFrom(remote, Port)))
	if err != nil {
		return netip.Addr{}, err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).AddrPort().Addr().Unmap(), nil
}

// Status is how sharing over DLNA goes, for System › Health.
type Status struct {
	// Unavailable tells that the UDP port cannot be listened on while
	// sharing is on.
	Unavailable bool
	Port        int
	Error       string
}

// Status returns how sharing goes.
func (s *Service) Status() Status {
	d := s.ssdp
	d.mu.Lock()
	defer d.mu.Unlock()
	status := Status{Port: d.port}
	if d.failure != nil {
		status.Unavailable, status.Error = true, d.failure.Error()
	}
	return status
}
