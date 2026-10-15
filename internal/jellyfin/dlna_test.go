package jellyfin

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/dlna"
	"github.com/moodiness/polyfin/internal/forwarded"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/stremio"
)

// dlnaSetup is the streaming addon's movies shared over DLNA, the
// device's requests coming from the loopback interface, which is of the
// local network.
type dlnaSetup struct {
	playbackSetup
	service *dlna.Service
	site    string
	handler http.Handler
}

func sharingOverDLNA(t *testing.T) dlnaSetup {
	t.Helper()
	p := playing(t)
	p.remuxable(t)
	d := shareOverDLNA(t, p.testServer)
	d.playbackSetup = p
	return d
}

// shareOverDLNA turns sharing over DLNA on for s, and serves its routes.
func shareOverDLNA(t *testing.T, s testServer) dlnaSetup {
	t.Helper()
	s.setting(t, func(settings *accounts.Settings) { settings.DLNA = true })
	service := dlna.Start(t.Context(), dlna.Options{HTTP: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8096}, ServerID: testServerID,
		DB: s.pool, Settings: s.store.Settings, Changed: s.store.SettingsChanged, User: s.store.User, Signer: s.handler.Playback.Signer(),
		Library: s.library, Player: s.handler, Version: "test", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	handler := service.Handler()
	site := httptest.NewServer(handler)
	t.Cleanup(site.Close)
	return dlnaSetup{playbackSetup: playbackSetup{testServer: s}, service: service, site: site.URL, handler: handler}
}

// device returns the device the tests' requests stand for.
func (s dlnaSetup) device(t *testing.T) dlna.Device {
	t.Helper()
	devices, err := s.service.Devices(t.Context())
	if err != nil || len(devices) != 1 || devices[0].Address != "127.0.0.1" {
		t.Fatalf("devices: %+v %v", devices, err)
	}
	return devices[0]
}

// show makes the device show the libraries of user.
func (s dlnaSetup) show(t *testing.T, user accounts.User) {
	t.Helper()
	id := &user.ID
	if _, err := s.service.UpdateDevice(t.Context(), s.device(t).ID, dlna.DeviceChanges{User: &id}); err != nil {
		t.Fatal(err)
	}
}

// browseResult is a Browse answer.
type browseResult struct {
	status          int
	didl            string
	returned, total int
}

// browse sends a Browse as a device does.
func (s dlnaSetup) browse(t *testing.T, object, flag string, start, count int) browseResult {
	t.Helper()
	body := fmt.Sprintf(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>`+
		`<u:Browse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1"><ObjectID>%s</ObjectID><BrowseFlag>%s</BrowseFlag>`+
		`<Filter>*</Filter><StartingIndex>%d</StartingIndex><RequestedCount>%d</RequestedCount><SortCriteria></SortCriteria></u:Browse>`+
		`</s:Body></s:Envelope>`, object, flag, start, count)
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, s.site+"/dlna/control/ContentDirectory", strings.NewReader(body))
	request.Header.Set("SOAPAction", `"urn:schemas-upnp-org:service:ContentDirectory:1#Browse"`)
	request.Header.Set("User-Agent", "TestTV DLNADOC/1.50")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct {
		Body struct {
			Response struct {
				Result         string
				NumberReturned int
				TotalMatches   int
			} `xml:",any"`
		}
	}
	data, _ := io.ReadAll(response.Body)
	_ = xml.Unmarshal(data, &envelope)
	r := envelope.Body.Response
	return browseResult{status: response.StatusCode, didl: r.Result, returned: r.NumberReturned, total: r.TotalMatches}
}

// didlObject is a container or an item of a DIDL-Lite answer.
type didlObject struct {
	ID    string `xml:"id,attr"`
	Title string `xml:"title"`
	Res   []struct {
		ProtocolInfo string `xml:"protocolInfo,attr"`
		URL          string `xml:",chardata"`
	} `xml:"res"`
}

func parseDIDL(t *testing.T, didl string) (containers, items []didlObject) {
	t.Helper()
	var parsed struct {
		Containers []didlObject `xml:"container"`
		Items      []didlObject `xml:"item"`
	}
	if err := xml.Unmarshal([]byte(didl), &parsed); err != nil {
		t.Fatalf("DIDL: %v\n%s", err, didl)
	}
	return parsed.Containers, parsed.Items
}

func titles(objects []didlObject) []string {
	var names []string
	for _, o := range objects {
		names = append(names, o.Title)
	}
	return names
}

// movieRes browses the device's first library and returns the movie's
// resource.
func (s dlnaSetup) movieRes(t *testing.T) (protocolInfo, address string) {
	t.Helper()
	root, _ := parseDIDL(t, s.browse(t, "0", "BrowseDirectChildren", 0, 0).didl)
	if len(root) == 0 {
		t.Fatal("no views")
	}
	_, items := parseDIDL(t, s.browse(t, root[0].ID, "BrowseDirectChildren", 0, 0).didl)
	for _, item := range items {
		if item.Title == "Movie" && len(item.Res) > 0 {
			return item.Res[0].ProtocolInfo, item.Res[0].URL
		}
	}
	t.Fatalf("no movie among %v", titles(items))
	return "", ""
}

// fetch asks for an address as a device does, with headers.
func fetch(t *testing.T, address string, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, address, nil)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response, data
}

// A device seen for the first time browses an empty server until it is
// given a user; then it sees that user's views, and parental control
// applies to them as everywhere.
func TestADLNADeviceBrowsesNothingUntilGivenAUser(t *testing.T) {
	t.Parallel()
	s := sharingOverDLNA(t)
	if empty := s.browse(t, "0", "BrowseDirectChildren", 0, 0); empty.status != http.StatusOK || empty.returned != 0 || empty.total != 0 {
		t.Fatalf("new device's root: %+v", empty)
	}
	device := s.device(t)
	if device.User != nil || device.UserAgent != "TestTV DLNADOC/1.50" {
		t.Errorf("new device: %+v", device)
	}
	s.show(t, s.user)
	result := s.browse(t, "0", "BrowseDirectChildren", 0, 0)
	views, _ := parseDIDL(t, result.didl)
	names := titles(views)
	if len(views) < 3 || names[len(names)-2] != "Continue Watching" || names[len(names)-1] != "Next Up" || result.total != len(views) {
		t.Fatalf("views: %v (%d of %d)", names, result.returned, result.total)
	}
	_, items := parseDIDL(t, s.browse(t, views[0].ID, "BrowseDirectChildren", 0, 0).didl)
	if got := titles(items); len(got) != 2 {
		t.Errorf("member's library: %v", got)
	}
	child := s.testServer.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(0)} })
	s.show(t, child)
	views, _ = parseDIDL(t, s.browse(t, "0", "BrowseDirectChildren", 0, 0).didl)
	_, items = parseDIDL(t, s.browse(t, views[0].ID, "BrowseDirectChildren", 0, 0).didl)
	if len(items) != 0 {
		t.Errorf("child's library shows PG movies: %v", titles(items))
	}
	// Another user's address gives nothing: it was signed for the member.
	s.show(t, s.user)
	_, address := s.movieRes(t)
	s.show(t, child)
	if response, _ := fetch(t, address, nil); response.StatusCode != http.StatusForbidden {
		t.Errorf("an address signed for another user: %d", response.StatusCode)
	}
}

// A blocked device, and any address outside the local network, get 403.
func TestBlockedAndRemoteDLNADevicesAreRefused(t *testing.T) {
	t.Parallel()
	s := sharingOverDLNA(t)
	response, description := fetch(t, s.site+"/dlna/description.xml", nil)
	if response.StatusCode != http.StatusOK || !strings.Contains(string(description), "<friendlyName>Polyfin</friendlyName>") ||
		!strings.Contains(string(description), "urn:schemas-upnp-org:service:ContentDirectory:1") {
		t.Fatalf("description: %d %s", response.StatusCode, description)
	}
	if _, err := s.service.UpdateDevice(t.Context(), s.device(t).ID, dlna.DeviceChanges{Blocked: new(true)}); err != nil {
		t.Fatal(err)
	}
	if response, _ := fetch(t, s.site+"/dlna/description.xml", nil); response.StatusCode != http.StatusForbidden {
		t.Errorf("blocked device's description: %d", response.StatusCode)
	}
	if result := s.browse(t, "0", "BrowseDirectChildren", 0, 0); result.status != http.StatusForbidden {
		t.Errorf("blocked device's browse: %d", result.status)
	}
	remote := httptest.NewRequest(http.MethodGet, "/dlna/description.xml", nil)
	remote.RemoteAddr = "203.0.113.7:40000"
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, remote)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("public address: %d", recorder.Code)
	}
	// Off, nothing is shared.
	s.setting(t, func(settings *accounts.Settings) { settings.DLNA = false })
	if response, _ := fetch(t, s.site+"/dlna/description.xml", nil); response.StatusCode != http.StatusNotFound {
		t.Errorf("sharing off: %d", response.StatusCode)
	}
}

// takingProfile is a profile that takes the movie's first version, H.264
// and Opus in Matroska, as it is.
const takingProfile = `<?xml version="1.0"?>
<Profile>
  <Name>Takes Matroska</Name>
  <DirectPlayProfiles>
    <DirectPlayProfile container="mkv" videoCodec="h264" audioCodec="opus" type="Video" />
  </DirectPlayProfiles>
  <TranscodingProfiles>
    <TranscodingProfile container="ts" type="Video" videoCodec="h264" audioCodec="aac" />
  </TranscodingProfiles>
</Profile>`

// force makes the device play with a profile added from xml.
func (s dlnaSetup) force(t *testing.T, xml string) {
	t.Helper()
	profile, err := s.service.SaveProfile(t.Context(), nil, []byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.service.UpdateDevice(t.Context(), s.device(t).ID, dlna.DeviceChanges{Profile: new(profile.ID.String())}); err != nil {
		t.Fatal(err)
	}
}

// A file the device's profile takes is sent as it is, with byte ranges;
// the device's playback shows in Sessions as the DLNA app's, which takes
// no commands.
func TestADLNADeviceGetsAFileItTakesWithRanges(t *testing.T) {
	t.Parallel()
	s := sharingOverDLNA(t)
	s.browse(t, "0", "BrowseDirectChildren", 0, 0)
	s.show(t, s.user)
	s.force(t, takingProfile)
	protocol, address := s.movieRes(t)
	if !strings.HasPrefix(protocol, "http-get:*:video/x-matroska:DLNA.ORG_OP=01") || !strings.Contains(address, "/dlna/media/") ||
		strings.Contains(address, "c=1") {
		t.Fatalf("resource: %s %s", protocol, address)
	}
	whole, err := os.ReadFile("../container/testdata/forced.mkv")
	if err != nil {
		t.Fatal(err)
	}
	response, data := fetch(t, address, map[string]string{"Range": "bytes=100-199"})
	if response.StatusCode != http.StatusPartialContent || string(data) != string(whole[100:200]) ||
		response.Header.Get("Content-Range") != "bytes 100-199/"+strconv.Itoa(len(whole)) {
		t.Fatalf("range: %d %q, %d bytes", response.StatusCode, response.Header.Get("Content-Range"), len(data))
	}
	if features := response.Header.Get("ContentFeatures.DLNA.ORG"); !strings.Contains(features, "DLNA.ORG_OP=01") {
		t.Errorf("content features: %q", features)
	}
	status, body := s.call(http.MethodGet, "/Sessions", app("tv", s.token), nil)
	var sessions []SessionInfo
	_ = json.Unmarshal(body, &sessions)
	found := false
	for _, session := range sessions {
		if session.Client == "DLNA" {
			found = true
			if session.SupportsRemoteControl || session.NowPlayingItem == nil || session.NowPlayingItem.Name != "Movie" {
				t.Errorf("DLNA session: %+v", session)
			}
		}
	}
	if status != http.StatusOK || !found {
		t.Errorf("sessions: %d %s", status, body)
	}
}

// A file the profile does not take is converted to MPEG-TS, from the time
// the device asks.
func TestADLNADeviceGetsWhatItCannotPlayConverted(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	s := sharingOverDLNA(t)
	s.browse(t, "0", "BrowseDirectChildren", 0, 0)
	s.show(t, s.user)
	protocol, address := s.movieRes(t)
	if !strings.HasPrefix(protocol, "http-get:*:video/mpeg:DLNA.ORG_OP=10") || !strings.Contains(address, "c=1") {
		t.Fatalf("resource: %s %s", protocol, address)
	}
	response, data := fetch(t, address, map[string]string{"TimeSeekRange.dlna.org": "npt=5.000-"})
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "video/mpeg" || len(data) < 188*10 || data[0] != 0x47 ||
		response.Header.Get("TimeSeekRange.DLNA.ORG") != "npt=0:00:05.000-0:00:15.008/0:00:15.008" {
		t.Fatalf("conversion: %d %v %d bytes", response.StatusCode, response.Header, len(data))
	}
}

// Devices send no position: a title they read past Marked played after
// (%) is marked played only while Mark played what DLNA devices play is
// on.
func TestDLNAPlaysMarkPlayedOnlyWhenTurnedOn(t *testing.T) {
	t.Parallel()
	s := sharingOverDLNA(t)
	ready := func() string {
		t.Helper()
		s.browse(t, "0", "BrowseDirectChildren", 0, 0)
		s.show(t, s.user)
		s.force(t, takingProfile)
		_, address := s.movieRes(t)
		return address
	}
	address := ready()
	played := func() bool {
		t.Helper()
		var movie BaseItemDto
		s.get(t, "/Users/"+s.user.ID.String()+"/Items/"+s.movie, s.token, &movie)
		return movie.UserData.Played
	}
	if response, _ := fetch(t, address, nil); response.StatusCode != http.StatusOK || played() {
		t.Fatalf("switch off: %d, played %v", response.StatusCode, played())
	}
	// Forgotten, the device is new when seen again, with a playback of its
	// own.
	if err := s.service.RemoveDevice(t.Context(), s.device(t).ID); err != nil {
		t.Fatal(err)
	}
	s.setting(t, func(settings *accounts.Settings) { settings.DLNAMarkPlayed = true })
	address = ready()
	if response, _ := fetch(t, address, map[string]string{"Range": "bytes=0-1000"}); response.StatusCode != http.StatusPartialContent || played() {
		t.Fatalf("the start read: %d, played %v", response.StatusCode, played())
	}
	if response, _ := fetch(t, address, nil); response.StatusCode != http.StatusOK || !played() {
		t.Errorf("the whole file read with the switch on: %d, played %v", response.StatusCode, played())
	}
}

// bigCatalog is an addon whose catalog holds count movies, a page of 100
// at a time.
func bigCatalog(t *testing.T, count int) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "big", Name: "Big", Version: "1", Types: []string{"movie"}, IDPrefixes: []string{"bg"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "all", Name: "All", Extra: []stremio.Extra{{Name: "skip"}}}}})
		case strings.HasPrefix(path, "/catalog/movie/all"):
			skip := 0
			if match := regexp.MustCompile(`skip=(\d+)`).FindStringSubmatch(path); match != nil {
				skip, _ = strconv.Atoi(match[1])
			}
			metas := []stremio.Meta{}
			for i := skip; i < min(skip+100, count); i++ {
				metas = append(metas, stremio.Meta{ID: fmt.Sprintf("bg%04d", i), Type: "movie", Name: fmt.Sprintf("Title %04d", i)})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// A big catalog is read page by page, as the device asks for them.
func TestADLNADeviceReadsABigCatalogPageByPage(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	user := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), bigCatalog(t, 450), false); err != nil {
		t.Fatal(err)
	}
	s.setting(t, func(settings *accounts.Settings) { settings.DLNANewDevicesUser = &user.ID })
	d := shareOverDLNA(t, s)
	views, _ := parseDIDL(t, d.browse(t, "0", "BrowseDirectChildren", 0, 0).didl)
	if len(views) == 0 {
		t.Fatal("a new device does not show the libraries of the user chosen for new devices")
	}
	for _, page := range []struct{ start, count int }{{0, 50}, {150, 50}, {390, 50}} {
		result := d.browse(t, views[0].ID, "BrowseDirectChildren", page.start, page.count)
		_, items := parseDIDL(t, result.didl)
		if result.returned != page.count || len(items) != page.count || items[0].Title != fmt.Sprintf("Title %04d", page.start) ||
			result.total < page.start+page.count {
			t.Errorf("page from %d: %d returned of %d, first %v", page.start, result.returned, result.total, titles(items[:min(1, len(items))]))
		}
	}
	// From as far as integers go, past every title.
	if result := d.browse(t, views[0].ID, "BrowseDirectChildren", math.MaxInt, 50); result.status != http.StatusOK || result.returned != 0 {
		t.Errorf("page from the largest index: %d, %d returned", result.status, result.returned)
	}
}

// Devices never send forwarded headers: a request that carries them from
// an address that is not a known proxy comes through a reverse proxy
// missing from Known proxies, which would show every client on the
// internet as one device of the local network. It is refused.
func TestForwardedDLNARequestsComeThroughKnownProxiesOnly(t *testing.T) {
	t.Parallel()
	s := sharingOverDLNA(t)
	id := s.user.ID
	s.setting(t, func(settings *accounts.Settings) { settings.DLNANewDevicesUser = &id })
	browse := `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>` +
		`<u:Browse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1"><ObjectID>0</ObjectID><BrowseFlag>BrowseDirectChildren</BrowseFlag>` +
		`<Filter>*</Filter><StartingIndex>0</StartingIndex><RequestedCount>0</RequestedCount><SortCriteria></SortCriteria></u:Browse>` +
		`</s:Body></s:Envelope>`
	ask := func(handler http.Handler, name, value string) (int, string) {
		request := httptest.NewRequest(http.MethodPost, "/dlna/control/ContentDirectory", strings.NewReader(browse))
		request.RemoteAddr = "172.18.0.2:40000"
		request.Header.Set(name, value)
		request.Header.Set("SOAPAction", `"urn:schemas-upnp-org:service:ContentDirectory:1#Browse"`)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder.Code, recorder.Body.String()
	}
	for name, value := range map[string]string{"X-Forwarded-For": "203.0.113.7", "X-Real-IP": "203.0.113.7", "Forwarded": "for=203.0.113.7"} {
		if status, body := ask(s.handler, name, value); status != http.StatusForbidden {
			t.Errorf("%s from a proxy missing from Known proxies: %d %.200s", name, status, body)
		}
	}
	// Listed, the proxy forwards the devices of the local network.
	known := forwarded.New(func() []netip.Prefix { return []netip.Prefix{netip.MustParsePrefix("172.18.0.2/32")} }, nil).Handler(s.handler)
	if status, body := ask(known, "X-Forwarded-For", "192.168.1.22"); status != http.StatusOK || !strings.Contains(body, "NumberReturned") {
		t.Errorf("a device through a known proxy: %d %.200s", status, body)
	}
	if status, _ := ask(known, "X-Forwarded-For", "203.0.113.7"); status != http.StatusForbidden {
		t.Errorf("an internet client through a known proxy: %d", status)
	}
	// Polyfin does not read Forwarded: a known proxy that names its client
	// there only would stand for every client behind it.
	if status, _ := ask(known, "Forwarded", "for=203.0.113.7"); status != http.StatusForbidden {
		t.Errorf("Forwarded alone through a known proxy: %d", status)
	}
}

// posterAddon lists one PG movie, "Rated", with a poster, in a catalog
// that searches.
func posterAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	picture := image.NewRGBA(image.Rect(0, 0, 8, 12))
	for i := range picture.Pix {
		picture.Pix[i] = 200
	}
	var poster bytes.Buffer
	if err := png.Encode(&poster, picture); err != nil {
		t.Fatal(err)
	}
	movie := func() stremio.Meta {
		return stremio.Meta{ID: "tt3000", Type: "movie", Name: "Rated", Year: "2008", Poster: server.URL + "/poster.png",
			Genres: []string{"Drama"}, Extras: &stremio.Extras{Certification: "PG"}}
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "posters", Name: "Posters", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "search"}}}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie()}})
		case path == "/meta/movie/tt3000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie()})
		case path == "/poster.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(poster.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// sharingPosters shares posterAddon's movie over DLNA, with member and
// child users, and returns the movie's ID, which the device has no user
// to see yet.
func sharingPosters(t *testing.T) (d dlnaSetup, member, child accounts.User, movie accounts.ID) {
	t.Helper()
	s := newTestServer(t, 10)
	member = s.user("member", nil)
	child = s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(0)} })
	if _, err := s.addons.Install(t.Context(), addons.Shared(), posterAddon(t), false); err != nil {
		t.Fatal(err)
	}
	d = shareOverDLNA(t, s)
	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 || page.Items[0].Name != "Rated" {
		t.Fatalf("member's library: %+v", page.Items)
	}
	movie, _ = accounts.ParseID(page.Items[0].Id)
	if response, _ := fetch(t, d.site+"/dlna/description.xml", nil); response.StatusCode != http.StatusOK {
		t.Fatalf("description: %d", response.StatusCode)
	}
	return d, member, child, movie
}

// A device gets the poster of a title its user may see only, as browsing
// lists it: parental control applies to art as to media.
func TestDLNAArtFollowsTheDevicesUser(t *testing.T) {
	t.Parallel()
	s, member, child, movie := sharingPosters(t)
	art := s.site + "/dlna/art/" + movie.String() + ".jpg"
	if response, _ := fetch(t, art, nil); response.StatusCode != http.StatusNotFound {
		t.Errorf("art for a device without a user: %d", response.StatusCode)
	}
	s.show(t, child)
	if response, _ := fetch(t, art, nil); response.StatusCode != http.StatusNotFound {
		t.Errorf("the PG movie's art for the child's device: %d", response.StatusCode)
	}
	grant := s.testServer.handler.Playback.Signer().Sign(playback.Grant{Version: movie, User: child.ID, Relay: true})
	media := s.site + "/dlna/media/" + movie.String() + ".mkv?g=" + url.QueryEscape(grant)
	if response, _ := fetch(t, media, nil); response.StatusCode != http.StatusNotFound {
		t.Errorf("the PG movie's media for the child's device: %d", response.StatusCode)
	}
	captions := s.site + "/dlna/captions/" + movie.String() + ".srt?g=" + url.QueryEscape(grant)
	if response, _ := fetch(t, captions, nil); response.StatusCode != http.StatusNotFound {
		t.Errorf("the PG movie's captions for the child's device: %d", response.StatusCode)
	}
	s.show(t, member)
	if response, body := fetch(t, art, nil); response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "image/jpeg" || len(body) == 0 {
		t.Errorf("the movie's art for the member's device: %d %s, %d bytes", response.StatusCode, response.Header.Get("Content-Type"), len(body))
	}
}

// A search from as far as integers go finds nothing past the titles found,
// as a page would.
func TestDLNASearchesFromHugeIndexes(t *testing.T) {
	t.Parallel()
	s, member, _, _ := sharingPosters(t)
	s.show(t, member)
	search := func(start int) (int, string) {
		body := fmt.Sprintf(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>`+
			`<u:Search xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1"><ContainerID>0</ContainerID>`+
			`<SearchCriteria>dc:title contains "Rated"</SearchCriteria><Filter>*</Filter><StartingIndex>%d</StartingIndex>`+
			`<RequestedCount>10</RequestedCount><SortCriteria></SortCriteria></u:Search></s:Body></s:Envelope>`, start)
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, s.site+"/dlna/control/ContentDirectory", strings.NewReader(body))
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("a search from %d: %v", start, err)
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		return response.StatusCode, string(data)
	}
	if status, body := search(0); status != http.StatusOK || !strings.Contains(body, "Rated") {
		t.Fatalf("a search from 0: %d %.300s", status, body)
	}
	if status, body := search(math.MaxInt); status != http.StatusOK || !strings.Contains(body, "<NumberReturned>0</NumberReturned>") {
		t.Errorf("a search from the largest index: %d %.300s", status, body)
	}
}

// A title counts as played by the bytes of its file sent during the
// playback, wherever they are: devices read the end of a file first to
// find its index, and read some parts again.
func TestDLNAPlaysCountTheBytesSent(t *testing.T) {
	t.Parallel()
	s := sharingOverDLNA(t)
	s.setting(t, func(settings *accounts.Settings) { settings.DLNAMarkPlayed = true })
	s.browse(t, "0", "BrowseDirectChildren", 0, 0)
	s.show(t, s.user)
	s.force(t, takingProfile)
	_, address := s.movieRes(t)
	played := func() bool {
		t.Helper()
		var movie BaseItemDto
		s.get(t, "/Users/"+s.user.ID.String()+"/Items/"+s.movie, s.token, &movie)
		return movie.UserData.Played
	}
	whole, err := os.ReadFile("../container/testdata/forced.mkv")
	if err != nil {
		t.Fatal(err)
	}
	size := len(whole)
	// The movie lasts less than userdata.MinResumeDuration: it is played
	// once past Resume, 5 % by default. The reads below send 2,100
	// different bytes, below that, and 4,201 counted each time they are
	// sent, past it.
	if threshold := size * 5 / 100; threshold <= 2100 || threshold >= 4201 {
		t.Fatalf("the file has %d bytes", size)
	}
	for _, r := range []string{"bytes=0-0", "bytes=" + strconv.Itoa(size-100) + "-", "bytes=-100", "bytes=0-1999", "bytes=0-1999"} {
		if response, _ := fetch(t, address, map[string]string{"Range": r}); response.StatusCode != http.StatusPartialContent {
			t.Fatalf("%s: %d", r, response.StatusCode)
		}
		if played() {
			t.Fatalf("played after %s", r)
		}
	}
	if response, _ := fetch(t, address, map[string]string{"Range": "bytes=2000-"}); response.StatusCode != http.StatusPartialContent {
		t.Fatalf("the rest read: %d", response.StatusCode)
	}
	// The mark is made once the handler has sent the rest, which can come
	// just after the client read it.
	eventually(t, "the movie played once the rest is read", played)
}

// A DLNA playback leaves /Sessions once its device has asked for nothing
// in two minutes, as it leaves the dashboard.
func TestEndedDLNAPlaybacksLeaveSessions(t *testing.T) {
	t.Parallel()
	s := sharingOverDLNA(t)
	var clock atomic.Pointer[time.Time]
	now := time.Now()
	clock.Store(&now)
	s.testServer.handler.now = func() time.Time { return *clock.Load() }
	s.browse(t, "0", "BrowseDirectChildren", 0, 0)
	s.show(t, s.user)
	s.force(t, takingProfile)
	_, address := s.movieRes(t)
	if response, _ := fetch(t, address, map[string]string{"Range": "bytes=100-199"}); response.StatusCode != http.StatusPartialContent {
		t.Fatalf("range: %d", response.StatusCode)
	}
	playing := func() bool {
		t.Helper()
		status, body := s.call(http.MethodGet, "/Sessions", app("tv", s.token), nil)
		var sessions []SessionInfo
		if err := json.Unmarshal(body, &sessions); status != http.StatusOK || err != nil {
			t.Fatalf("sessions: %d %v", status, err)
		}
		return slices.ContainsFunc(sessions, func(session SessionInfo) bool { return session.Client == "DLNA" })
	}
	if !playing() {
		t.Fatal("the playback is not listed")
	}
	later := now.Add(dlnaStale + time.Second)
	clock.Store(&later)
	if playing() {
		t.Error("the playback is still listed two minutes after the device's last request")
	}
}
