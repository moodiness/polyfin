package dlna

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

const serverID = "0123456789abcdef0123456789abcdef"

// fakeSettings are settings a test changes, telling the service as the
// accounts store does.
type fakeSettings struct {
	mu       sync.Mutex
	settings accounts.Settings
	changed  chan struct{}
}

func (f *fakeSettings) get() accounts.Settings {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.settings
}

func (f *fakeSettings) next() <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.changed
}

func (f *fakeSettings) set(change func(*accounts.Settings)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	change(&f.settings)
	close(f.changed)
	f.changed = make(chan struct{})
}

// start starts SSDP on port, 0 for a free one, with sharing on, the HTTP
// server on port 8096, and applied told of each change of the settings
// applied. It reads no database.
func start(t *testing.T, port int) (*Service, *fakeSettings, chan struct{}) {
	t.Helper()
	settings := &fakeSettings{settings: accounts.Settings{ServerName: "Polyfin", DLNA: true}, changed: make(chan struct{})}
	applied := make(chan struct{}, 1)
	s := Start(t.Context(), Options{Port: port, HTTP: &net.TCPAddr{Port: 8096}, ServerID: serverID, Settings: settings.get,
		Changed: settings.next, Version: "test", Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		route:   func(netip.Addr) (netip.Addr, error) { return netip.MustParseAddr("192.168.1.10"), nil },
		applied: func() { applied <- struct{}{} }})
	t.Cleanup(func() {
		s.ssdp.mu.Lock()
		s.ssdp.stop()
		s.ssdp.mu.Unlock()
	})
	return s, settings, applied
}

const search = "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: %s\r\n\r\n"

// An SSDP search from the local network is answered, with the address of
// the description on the interface that reaches the device; one from a
// public address is not, nor any while sharing is off.
func TestSSDPSearchesAreAnsweredOnTheLocalNetworkOnly(t *testing.T) {
	s, settings, applied := start(t, 0)
	all := []byte(strings.Replace(search, "%s", "ssdp:all", 1))
	answers := s.answer(all, netip.MustParseAddrPort("192.168.1.20:50000"))
	if len(answers) != 5 {
		t.Fatalf("ssdp:all answers: %d", len(answers))
	}
	first := string(answers[0])
	for _, part := range []string{"HTTP/1.1 200 OK\r\n", "LOCATION: http://192.168.1.10:8096/dlna/description.xml\r\n", "ST: upnp:rootdevice\r\n",
		"USN: uuid:01234567-89ab-cdef-0123-456789abcdef::upnp:rootdevice\r\n"} {
		if !strings.Contains(first, part) {
			t.Errorf("answer lacks %q:\n%s", part, first)
		}
	}
	server := []byte(strings.Replace(search, "%s", "urn:schemas-upnp-org:device:MediaServer:1", 1))
	if answers := s.answer(server, netip.MustParseAddrPort("10.0.0.5:1900")); len(answers) != 1 ||
		!strings.Contains(string(answers[0]), "ST: urn:schemas-upnp-org:device:MediaServer:1\r\n") {
		t.Errorf("MediaServer search: %q", answers)
	}
	if answers := s.answer(server, netip.MustParseAddrPort("203.0.113.9:1900")); answers != nil {
		t.Errorf("a public address is answered: %q", answers)
	}
	renderer := []byte(strings.Replace(search, "%s", "urn:schemas-upnp-org:device:MediaRenderer:1", 1))
	if answers := s.answer(renderer, netip.MustParseAddrPort("192.168.1.20:50000")); answers != nil {
		t.Errorf("a search for something else is answered: %q", answers)
	}
	if answers := s.answer([]byte("NOTIFY * HTTP/1.1\r\n\r\n"), netip.MustParseAddrPort("192.168.1.20:50000")); answers != nil {
		t.Errorf("a notification is answered: %q", answers)
	}
	// Over UDP, from the loopback interface.
	port := s.Status().Port
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write(server); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	buffer := make([]byte, maxDatagram)
	n, err := conn.Read(buffer)
	if err != nil || !strings.Contains(string(buffer[:n]), "/dlna/description.xml") {
		t.Fatalf("UDP answer: %q %v", buffer[:n], err)
	}
	settings.set(func(s *accounts.Settings) { s.DLNA = false })
	<-applied
	if answers := s.answer(server, netip.MustParseAddrPort("192.168.1.20:50000")); answers != nil {
		t.Errorf("answered while sharing is off: %q", answers)
	}
}

// A port another program holds never stops Polyfin: Status tells it, for
// System › Health, until it is free.
func TestATakenSSDPPortIsTold(t *testing.T) {
	taken, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	port := taken.LocalAddr().(*net.UDPAddr).Port
	s, settings, applied := start(t, port)
	status := s.Status()
	if !status.Unavailable || status.Port != port {
		t.Fatalf("status with the port taken: %+v", status)
	}
	_ = taken.Close()
	settings.set(func(*accounts.Settings) {})
	<-applied
	if status := s.Status(); status.Unavailable {
		t.Errorf("status once the port is free: %+v", status)
	}
	settings.set(func(s *accounts.Settings) { s.DLNA = false })
	<-applied
	if status := s.Status(); status.Unavailable {
		t.Errorf("status while sharing is off: %+v", status)
	}
}

// Devices are given the profile their headers or their announced model
// match, added profiles first, else the generic one.
func TestProfilesAreChosenByWhatDevicesSend(t *testing.T) {
	headers := func(name, value string) http.Header {
		h := http.Header{}
		h.Set(name, value)
		return h
	}
	added, err := ParseProfile([]byte(`<Profile><Name>Hall TV</Name><Identification><FriendlyName>^Hall</FriendlyName></Identification>
		<DirectPlayProfiles><DirectPlayProfile container="mp4" type="Video" /></DirectPlayProfiles></Profile>`))
	if err != nil {
		t.Fatal(err)
	}
	profiles := append([]Profile{added}, builtins[1:]...)
	for _, tc := range []struct {
		seen Seen
		want string
	}{
		{Seen{Headers: headers("User-Agent", "SEC_HHP_[TV] Living Room/1.0 DLNADOC/1.50")}, "Samsung Smart TV"},
		{Seen{Headers: headers("X-AV-Client-Info", `av=5.0; cn="Sony Corporation"; mn="BRAVIA KDL-40"; mv="1.7";`)}, "Sony Bravia"},
		{Seen{Manufacturer: "Sony Corporation", Model: "BRAVIA 4K", Headers: http.Header{}}, "Sony Bravia"},
		{Seen{Manufacturer: "Sony Corporation", Model: "Speaker", Headers: http.Header{}}, GenericProfile},
		{Seen{Headers: headers("User-Agent", "Linux/5.4 UPnP/1.0 LGE_DLNA_SDK/1.6.0")}, "LG Smart TV"},
		{Seen{Name: "Hall TV", Headers: headers("User-Agent", "SEC_HHP_[TV]")}, "Hall TV"},
		{Seen{Name: "Hall TV", Headers: http.Header{}}, "Hall TV"},
		{Seen{Headers: headers("User-Agent", "SomePlayer/2.0")}, GenericProfile},
	} {
		if got := Choose(profiles, tc.seen).Name; got != tc.want {
			t.Errorf("%+v: %s, want %s", tc.seen, got, tc.want)
		}
	}
	for _, bad := range []string{"<Profile><Name></Name></Profile>", "not xml", "<Profile><Name>No playback</Name></Profile>",
		`<Profile><Name>Bad</Name><Identification><ModelName>(</ModelName></Identification><DirectPlayProfiles><DirectPlayProfile container="ts" type="Video"/></DirectPlayProfiles></Profile>`} {
		if _, err := ParseProfile([]byte(bad)); err == nil {
			t.Errorf("%q was read as a profile", bad)
		}
	}
	// Each built-in profile converts video to MPEG-TS and takes MP3.
	for _, p := range Builtins() {
		profile := p.Playback()
		if len(profile.TranscodingProfiles) != 1 || profile.TranscodingProfiles[0].Container != "ts" || !p.TakesAudio("mp3", "mp3") {
			t.Errorf("%s: %+v", p.Name, profile.TranscodingProfiles)
		}
	}
}

func TestTimeSeeksAreRead(t *testing.T) {
	for header, want := range map[string]time.Duration{"npt=10.5-": 10500 * time.Millisecond, "npt=0:01:02.250-0:02:00": 62250 * time.Millisecond,
		"npt=0-": 0, " npt=1:00:00.000-": time.Hour} {
		if got, ok := parseTimeSeek(header); !ok || got != want {
			t.Errorf("%q: %v %v, want %v", header, got, ok, want)
		}
	}
	for _, bad := range []string{"bytes=0-", "npt=-", "npt=a-", "npt=-5-"} {
		if _, ok := parseTimeSeek(bad); ok {
			t.Errorf("%q was read", bad)
		}
	}
	if got := NPT(3723*time.Second + 45*time.Millisecond); got != "1:02:03.045" {
		t.Errorf("NPT: %s", got)
	}
}

func TestSOAPActionsAreRead(t *testing.T) {
	action, err := readAction(strings.NewReader(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>` +
		`<u:Search xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1"><ContainerID>0</ContainerID>` +
		`<SearchCriteria>dc:title contains "the &quot;sea&quot;"</SearchCriteria><StartingIndex>0</StartingIndex></u:Search></s:Body></s:Envelope>`))
	if err != nil || action.name != "Search" || action.args["ContainerID"] != "0" || action.args["SearchCriteria"] != `dc:title contains "the "sea""` {
		t.Fatalf("%+v %v", action, err)
	}
	if match := titleTerm.FindStringSubmatch(`upnp:class derivedfrom "object.item.videoItem" and dc:title contains "Movie"`); match == nil ||
		match[1] != "Movie" {
		t.Errorf("title term: %q", match)
	}
}

// The server's own announcements, which multicast loops back to it, and
// those of another Polyfin server, are not taken for a device's: the
// server would otherwise read its own description and record itself.
func TestOwnAnnouncementsAreNotDevices(t *testing.T) {
	s, _, _ := start(t, 0)
	notify := func(nt, location, usn string) []byte {
		return []byte("NOTIFY * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nCACHE-CONTROL: max-age=1800\r\nLOCATION: " + location + "\r\nNT: " + nt +
			"\r\nNTS: ssdp:alive\r\nSERVER: Linux UPnP/1.0\r\nUSN: " + usn + "\r\n\r\n")
	}
	from := netip.MustParseAddr("192.168.1.35")
	own := notify("upnp:rootdevice", "http://192.168.1.35:8096/dlna/description.xml", s.usn("upnp:rootdevice"))
	if location, ok := s.renderer(own, from); ok {
		t.Errorf("own announcement taken for a device's: %s", location)
	}
	other := notify("upnp:rootdevice", "http://192.168.1.36:8096/dlna/description.xml", "uuid:aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee::upnp:rootdevice")
	if location, ok := s.renderer(other, netip.MustParseAddr("192.168.1.36")); ok {
		t.Errorf("another Polyfin taken for a device: %s", location)
	}
	tv := notify("urn:schemas-upnp-org:device:MediaRenderer:1", "http://192.168.1.40:7676/smp_2_", "uuid:1234::urn:schemas-upnp-org:device:MediaRenderer:1")
	if location, ok := s.renderer(tv, netip.MustParseAddr("192.168.1.40")); !ok || location != "http://192.168.1.40:7676/smp_2_" {
		t.Errorf("a TV's announcement: %q %v", location, ok)
	}
	if _, ok := s.renderer(tv, netip.MustParseAddr("203.0.113.5")); ok {
		t.Error("a public address's announcement was taken")
	}
}
