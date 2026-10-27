package dlna

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
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
	return startAt(t, port, nil)
}

// startAt starts as start does, with now as the clock, time.Now for nil.
func startAt(t *testing.T, port int, now func() time.Time) (*Service, *fakeSettings, chan struct{}) {
	t.Helper()
	settings := &fakeSettings{settings: accounts.Settings{ServerName: "Polyfin", DLNA: true}, changed: make(chan struct{})}
	applied := make(chan struct{}, 1)
	s := Start(t.Context(), Options{Port: port, HTTP: &net.TCPAddr{Port: 8096}, ServerID: serverID, Settings: settings.get,
		Changed: settings.next, Version: "test", Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		route:   func(netip.Addr) (netip.Addr, error) { return netip.MustParseAddr("192.168.1.10"), nil },
		applied: func() { applied <- struct{}{} }, now: now})
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

// A DLNA device gets its conversion as one MPEG-TS stream, not as HLS
// segments: the audio codecs it takes in MPEG-TS are copied, MP2 among
// them, as Jellyfin's DLNA streams copy them.
func TestDLNAConversionsCopyTheAudioMPEGTSCarries(t *testing.T) {
	tv, err := ParseProfile([]byte(`<Profile><Name>Hall TV</Name><DirectPlayProfiles><DirectPlayProfile container="mp4" type="Video" videoCodec="h264" audioCodec="aac" /></DirectPlayProfiles>
		<TranscodingProfiles><TranscodingProfile container="ts" type="Video" videoCodec="h264" audioCodec="mp2,aac" context="Streaming" /></TranscodingProfiles></Profile>`))
	if err != nil {
		t.Fatal(err)
	}
	recording := playback.MediaSource{Container: "ts", Streams: []playback.MediaStream{
		{Type: "Video", Index: 0, Codec: "mpeg2video", Width: new(720), Height: new(576)},
		{Type: "Audio", Index: 1, Codec: "mp2", Channels: new(2), IsDefault: true},
	}}
	got := playback.Decide(tv.Playback(), recording, playback.Options{EnableDirectPlay: true, EnableDirectStream: true,
		Can: playback.Capabilities{Encoders: []string{"libx264"}}})
	if !got.HLS || got.Video == nil || got.Audio != nil {
		t.Errorf("converted %v, video %+v, audio %+v; want the video converted and the MP2 copied", got.HLS, got.Video, got.Audio)
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

// pagingLibrary records the start and count of the pages, and the limit
// of the searches, asked of the library.
type pagingLibrary struct {
	Library
	asked chan [2]int
}

func (l pagingLibrary) Search(_ context.Context, _ accounts.User, _ string, _ []library.Kind, limit int) ([]library.Item, error) {
	l.asked <- [2]int{0, limit}
	return nil, nil
}

func (l pagingLibrary) Children(_ context.Context, _ accounts.User, _ accounts.ID, start, count int, _ string) (library.Page, error) {
	l.asked <- [2]int{start, count}
	return library.Page{}, nil
}

// A StartingIndex as large as integers go reads past any container or
// search: the end of the page asked of the library does not overflow.
func TestHugeStartingIndexesAreClamped(t *testing.T) {
	s, _, _ := start(t, 0)
	asked := make(chan [2]int, 1)
	s.options.Library = pagingLibrary{asked: asked}
	r := httptest.NewRequest(http.MethodPost, "/dlna/control/ContentDirectory", nil)
	r = r.WithContext(withClient(r.Context(), client{hasUser: true, user: accounts.User{Name: "member"}}))
	huge := strconv.Itoa(math.MaxInt)
	if _, failure := s.search(r, soapAction{name: "Search", args: map[string]string{"StartingIndex": huge, "RequestedCount": "10",
		"SearchCriteria": `dc:title contains "a"`}}); failure != nil {
		t.Fatalf("search: %+v", failure)
	}
	if limit := (<-asked)[1]; limit < 10 {
		t.Errorf("a search from the largest index asked the library for %d titles", limit)
	}
	folder := folderPrefix + "6f1c2c1e-1d2a-4f3b-9a1b-2c3d4e5f6a7b"
	if _, failure := s.browse(r, soapAction{name: "Browse", args: map[string]string{"ObjectID": folder, "BrowseFlag": "BrowseDirectChildren",
		"StartingIndex": huge, "RequestedCount": "10"}}); failure != nil {
		t.Fatalf("browse: %+v", failure)
	}
	if page := <-asked; page[0] < 0 || page[1] != 10 || page[0]+page[1] < page[0] {
		t.Errorf("a page from the largest index: start %d, count %d", page[0], page[1])
	}
}

// A SOAP body that stops coming is given up at the deadline: the device
// gets a fault and the request ends. The clock is an hour behind, so that
// the deadline has passed when the body is read.
func TestSOAPBodiesAreReadWithinADeadline(t *testing.T) {
	s, _, _ := startAt(t, 0, func() time.Time { return time.Now().Add(-time.Hour) })
	returned := make(chan struct{})
	handler := s.control(s.connectionManagerAction)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(returned)
		handler(w, r)
	}))
	defer server.Close()
	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := fmt.Fprintf(conn, "POST /dlna/control/ConnectionManager HTTP/1.1\r\nHost: x\r\nContent-Length: 1000\r\n\r\n<s:Envelope"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-returned:
	case <-time.After(30 * time.Second):
		t.Fatal("the request still waits for the rest of its body")
	}
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if body, _ := io.ReadAll(response.Body); response.StatusCode != http.StatusInternalServerError || !strings.Contains(string(body), "<errorCode>401</errorCode>") {
		t.Errorf("answer: %d %s", response.StatusCode, body)
	}
}

// A device's description is read once at a time, whatever it announces
// meanwhile, and not again for a while once read.
func TestDeviceDescriptionsAreReadOnceAtATime(t *testing.T) {
	var clock atomic.Pointer[time.Time]
	now := time.Now()
	clock.Store(&now)
	s, _, _ := startAt(t, 0, func() time.Time { return *clock.Load() })
	var name atomic.Value
	var reads atomic.Int32
	arrived := make(chan int32, 16)
	release, stop := make(chan struct{}), make(chan struct{})
	description := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arrived <- reads.Add(1)
		select {
		case <-release:
		case <-stop:
			return
		}
		_, _ = fmt.Fprintf(w, `<root><device><friendlyName>%s</friendlyName></device></root>`, name.Load())
	}))
	defer description.Close()
	defer close(stop)
	notify := []byte(fmt.Sprintf("NOTIFY * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nNT: urn:schemas-upnp-org:device:MediaRenderer:1\r\n"+
		"NTS: ssdp:alive\r\nUSN: uuid:renderer::urn:schemas-upnp-org:device:MediaRenderer:1\r\nLOCATION: %s/desc.xml\r\n\r\n", description.URL))
	from := netip.MustParseAddr("127.0.0.1")
	read := func(want string) {
		t.Helper()
		select {
		case <-arrived:
		case <-time.After(30 * time.Second):
			t.Fatal("the description was not read")
		}
		name.Store(want)
		release <- struct{}{}
		for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			if sighting, ok := s.ssdp.announcedAt(from); ok && sighting.Name == want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("the device is not named %s", want)
			}
		}
	}
	// Announcements while the description is read, and right after.
	for range 5 {
		s.ssdp.heard(notify, from)
	}
	read("TV")
	for range 5 {
		s.ssdp.heard(notify, from)
	}
	later := now.Add(24 * time.Hour)
	clock.Store(&later)
	s.ssdp.heard(notify, from)
	read("Renamed TV")
	if n := reads.Load(); n != 2 {
		t.Errorf("the description was read %d times, want 2", n)
	}
}
