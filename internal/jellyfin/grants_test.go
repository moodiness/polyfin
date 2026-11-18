package jellyfin

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
)

// grantedRoute is a media address that carries a grant and no token, with
// the status it answers while its grant stands.
type grantedRoute struct {
	name, address string
	status        int
}

// grantedPlay is what a device was given to play the movie: its play
// session, the Path of its media source and the play session of its HLS
// stream, three grants, and the addresses of each kind of media route
// under them.
type grantedPlay struct {
	session, path, hls string
	routes             []grantedRoute
}

// granted asks PlaybackInfo for the movie's second version as the device
// of token does, once for the file as it is and once, as an app that
// could not play the file, for its HLS stream, which needs the version
// keyframed.
func (p playbackSetup) granted(t *testing.T, device, token string) grantedPlay {
	t.Helper()
	ask := func(body map[string]any) playbackAnswer {
		t.Helper()
		status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app(device, token), body)
		var answer playbackAnswer
		if err := json.Unmarshal(data, &answer); status != http.StatusOK || err != nil || len(answer.MediaSources) == 0 {
			t.Fatalf("PlaybackInfo: %d %s", status, data)
		}
		return answer
	}
	file := ask(map[string]any{"MediaSourceId": p.versions[1].ID.String()})
	converted := ask(map[string]any{"MediaSourceId": p.versions[1].ID.String(), "MaxStreamingBitrate": 120_000_000,
		"DeviceProfile": p.profile(t, "jellyfin-web-chrome"), "EnableDirectPlay": false})
	source := file.MediaSources[0].Id
	play := grantedPlay{session: file.PlaySessionId, path: file.MediaSources[0].Path, hls: converted.PlaySessionId}
	if play.path == "" || converted.MediaSources[0].TranscodingUrl == "" {
		t.Fatalf("no Path or no TranscodingUrl: %+v %+v", file.MediaSources[0], converted.MediaSources[0])
	}
	// The HLS address without the token PlaybackInfo puts beside its play
	// session: the HLS routes read the play session alone.
	playlist, err := url.Parse(p.url + converted.MediaSources[0].TranscodingUrl)
	if err != nil {
		t.Fatal(err)
	}
	query := playlist.Query()
	query.Del("ApiKey")
	playlist.RawQuery = query.Encode()
	target := "https://example.com/segment.ts"
	session := url.QueryEscape(play.session)
	play.routes = []grantedRoute{
		{"the stream by its play session", p.url + "/Videos/" + p.movie + "/stream?static=true&mediaSourceId=" + source + "&playSessionId=" + session, http.StatusPartialContent},
		{"the media source's Path", play.path, http.StatusPartialContent},
		{"the download", p.url + "/Items/" + p.movie + "/Download?PolyfinGrant=" + session, http.StatusPartialContent},
		{"the HLS playlist", playlist.String(), http.StatusOK},
		// The movie has no attachment, is no track and plays no relayed
		// playlist: its grant is read first, then nothing is found.
		{"an attachment", p.url + "/Videos/" + p.movie + "/" + source + "/Attachments/0?playSessionId=" + session, http.StatusNotFound},
		{"an audio track", p.url + "/Audio/" + p.movie + "/stream?playSessionId=" + session, http.StatusNotFound},
		{"a relayed file", p.url + "/Videos/" + p.movie + "/live/segment.ts?PolyfinGrant=" + session +
			"&target=" + base64.RawURLEncoding.EncodeToString([]byte(target)) + "&sign=" + p.handler.Playback.Signer().Link(play.session, target), http.StatusNotFound},
	}
	return play
}

// mediaStatus asks for the first bytes of a media address, with no
// credentials but those of its query.
func mediaStatus(t *testing.T, address string) int {
	t.Helper()
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, address, nil)
	request.Header.Set("Range", "bytes=0-3")
	response, err := testClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode
}

// answers checks what each route of a play answers: its own status while
// its grant stands, 401 once it stands no more.
func (play grantedPlay) answers(t *testing.T, when string, stands bool) {
	t.Helper()
	for _, route := range play.routes {
		want := http.StatusUnauthorized
		if stands {
			want = route.status
		}
		if got := mediaStatus(t, route.address); got != want {
			t.Errorf("%s, %s: %d, want %d", when, route.name, got, want)
		}
	}
}

// The grants given to a device, its play sessions and the Path of its
// media sources, end with its sign-in: once its app signed out, an
// administrator removed it, or its account's password changed, every
// media route refuses them, as it refuses its token.
func TestAGrantEndsWithItsDevicesSignIn(t *testing.T) {
	t.Parallel()
	for name, signOut := range map[string]func(t *testing.T, p playbackSetup){
		"signed out by its app": func(t *testing.T, p playbackSetup) {
			if status, _ := p.call(http.MethodPost, "/Sessions/Logout", app("tv", p.token), nil); status != http.StatusNoContent {
				t.Fatalf("sign-out: %d", status)
			}
		},
		"removed by an administrator": func(t *testing.T, p playbackSetup) {
			device, _, err := p.store.DeviceByToken(t.Context(), p.token, "127.0.0.1")
			if err != nil {
				t.Fatal(err)
			}
			if err := p.store.RevokeDevice(t.Context(), p.user.ID, device.ID); err != nil {
				t.Fatal(err)
			}
		},
		"signed out by a password change": func(t *testing.T, p playbackSetup) {
			password := "a brand new password"
			if _, err := p.store.UpdateUser(t.Context(), p.user.ID, accounts.UserChanges{Password: &password}, nil); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := playing(t)
			p.keyframed(t, p.versions[1])
			play := p.granted(t, "tv", p.token)
			play.answers(t, "while signed in", true)
			signOut(t, p)
			if status := p.get(t, "/Users/Me", p.token, nil); status != http.StatusUnauthorized {
				t.Fatalf("the token once signed out: %d", status)
			}
			play.answers(t, "once signed out", false)
		})
	}
}

// A play started on one device goes on when another device of its user
// signs out: only the grants given to the device signed out end.
func TestAPlayGoesOnWhenAnotherDeviceOfItsUserSignsOut(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.keyframed(t, p.versions[1])
	phoneToken := p.signIn("member", "phone")
	tv, phone := p.granted(t, "tv", p.token), p.granted(t, "phone", phoneToken)
	tv.answers(t, "the TV, both signed in", true)
	phone.answers(t, "the phone, both signed in", true)
	if status, _ := p.call(http.MethodPost, "/Sessions/Logout", app("phone", phoneToken), nil); status != http.StatusNoContent {
		t.Fatalf("sign-out: %d", status)
	}
	tv.answers(t, "the TV, once the phone signed out", true)
	phone.answers(t, "the phone, once signed out", false)
}

// A grant stands two days after it was signed, on the handler's clock,
// then only while its play goes on: as long as no hour passes without a
// request it is accepted for. A grant left unused in the last hour of its
// two days is over at their end, and so is one signed further ahead of
// the clock than grants live.
func TestAGrantStandsTwoDaysThenWhileItsPlayGoesOn(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.keyframed(t, p.versions[1])
	var clock atomic.Pointer[time.Time]
	set := func(at time.Time) { clock.Store(&at) }
	start := time.Now().Truncate(time.Second)
	set(start)
	p.handler.now = func() time.Time { return *clock.Load() }
	// Two plays given at the same time: one is asked for all along, the
	// other left.
	going, left := p.granted(t, "tv", p.token), p.granted(t, "tv", p.token)

	// Within the two days, nothing is asked of a grant but its signature.
	at := start.Add(grantLife - time.Second)
	set(at)
	going.answers(t, "a second before two days", true)
	// A second past them, the play asked for two seconds ago goes on, on
	// every route, and the one left is over.
	at = start.Add(grantLife + time.Second)
	set(at)
	going.answers(t, "a second past two days, asked for two seconds ago", true)
	left.answers(t, "a second past two days, left since it was given", false)
	// It goes on as long as its app asks: every half hour here.
	for range 5 {
		at = at.Add(grantIdle / 2)
		set(at)
		going.answers(t, "asked for every half hour, "+at.Sub(start).String()+" after it was given", true)
	}
	// Then nothing for more than an hour: it is over, and a refused
	// request does not start it again.
	at = at.Add(grantIdle + time.Second)
	set(at)
	going.answers(t, "after an hour without a request", false)
	going.answers(t, "asked for again", false)

	// A grant from further ahead than grants live is refused; one from
	// less far ahead, as after a clock was corrected, stands.
	ahead := p.granted(t, "tv", p.token)
	set(at.Add(-grantLife - time.Second))
	ahead.answers(t, "signed more than two days ahead of the clock", false)
	set(at.Add(-grantLife + time.Second))
	ahead.answers(t, "signed less than two days ahead of the clock", true)
}

// A grant that carries no time, as those given out before grants carried
// one, is refused on every route, whether it names a device or not. The
// token of a request, when it carries one, then decides alone, as for a
// play session Polyfin did not sign.
func TestAGrantWithoutATimeIsRefused(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.keyframed(t, p.versions[1])
	device, _, err := p.store.DeviceByToken(t.Context(), p.token, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	play := p.granted(t, "tv", p.token)
	play.answers(t, "the grants PlaybackInfo gives", true)
	signer := p.handler.Playback.Signer()
	for name, untimed := range map[string]string{
		"naming the device": signer.Sign(playback.Grant{Version: p.versions[1].ID, User: p.user.ID, Device: device.ID}),
		"naming no device":  signer.Sign(playback.Grant{Version: p.versions[1].ID, User: p.user.ID}),
	} {
		if _, err := signer.Verify(untimed); err != nil {
			t.Fatalf("%s: the signer refuses its own grant: %v", name, err)
		}
		old := url.QueryEscape(untimed)
		var stream string
		for _, route := range play.routes {
			address := route.address
			for _, given := range []string{play.session, play.hls} {
				address = strings.ReplaceAll(address, url.QueryEscape(given), old)
			}
			if route.name == "the media source's Path" {
				parsed, err := url.Parse(address)
				if err != nil {
					t.Fatal(err)
				}
				query := parsed.Query()
				query.Set(grantParameter, untimed)
				parsed.RawQuery = query.Encode()
				address = parsed.String()
			}
			if route.name == "a relayed file" {
				target, _ := base64.RawURLEncoding.DecodeString(mustQuery(t, address, "target"))
				address = address[:strings.Index(address, "&sign=")] + "&sign=" + signer.Link(untimed, string(target))
			}
			if !strings.Contains(address, old) {
				t.Fatalf("%s: %s does not carry the grant: %s", name, route.name, address)
			}
			if got := mediaStatus(t, address); got != http.StatusUnauthorized {
				t.Errorf("a grant without a time %s, %s: %d, want 401", name, route.name, got)
			}
			if route.name == "the stream by its play session" {
				stream = address
			}
		}
		if got := mediaStatus(t, stream+"&api_key="+p.token); got != http.StatusPartialContent {
			t.Errorf("a grant without a time %s, beside the device's token: %d, want 206", name, got)
		}
	}
}

// mustQuery reads a parameter of an address's query.
func mustQuery(t *testing.T, address, name string) string {
	t.Helper()
	parsed, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Query().Get(name)
}

// The address a DLNA device is given opens the DLNA routes only: the
// Jellyfin API refuses its grant, for its own title as for another, and
// the DLNA routes refuse the Jellyfin API's grants for the same user and
// title. The device plays its address as before.
func TestDLNAAddressesOpenOnlyTheDLNARoutes(t *testing.T) {
	t.Parallel()
	s := sharingOverDLNA(t)
	s.keyframed(t, s.versions[1])
	if seen := s.browse(t, "0", "BrowseDirectChildren", 0, 0); seen.status != http.StatusOK {
		t.Fatalf("first browse: %+v", seen)
	}
	s.show(t, s.user)
	s.force(t, takingProfile)
	_, address := s.movieRes(t)
	parsed, err := url.Parse(address)
	if err != nil || parsed.Query().Get("g") == "" {
		t.Fatalf("the movie's address %q: %v", address, err)
	}
	given := parsed.Query().Get("g")
	if response, body := fetch(t, address, map[string]string{"Range": "bytes=0-3"}); response.StatusCode != http.StatusPartialContent || len(body) != 4 {
		t.Fatalf("the device's own address: %d, %d bytes", response.StatusCode, len(body))
	}

	// The device shown, sharing on and the member signed in: the Jellyfin
	// API still takes the address's grant for nothing.
	if grant, err := s.testServer.handler.Playback.Signer().Verify(given); err == nil {
		t.Errorf("the Jellyfin API's signer takes the address's grant: %+v", grant)
	}
	for title, target := range map[string]string{
		"its title":     "/Videos/" + s.movie + "/stream?static=true&mediaSourceId=" + s.versions[1].ID.String(),
		"another title": "/Videos/" + s.remote + "/stream?static=true&mediaSourceId=" + s.remote,
		"a download":    "/Items/" + s.movie + "/Download?",
	} {
		for _, parameter := range []string{grantParameter, "playSessionId"} {
			if got := mediaStatus(t, s.url+target+"&"+parameter+"="+url.QueryEscape(given)); got != http.StatusUnauthorized {
				t.Errorf("the address's grant as %s on the Jellyfin API, %s: %d, want 401", parameter, title, got)
			}
		}
	}

	// The other way: a play session PlaybackInfo gave the member, and a
	// grant of the Jellyfin API for the very user and item of the address.
	movie, err := accounts.ParseID(s.movie)
	if err != nil {
		t.Fatal(err)
	}
	play := s.granted(t, "tv", s.token)
	play.answers(t, "the member's own play", true)
	for name, grant := range map[string]string{
		"a play session":                     play.session,
		"a grant for the same user and item": s.testServer.handler.grant(playback.Grant{Version: movie, User: s.user.ID, Relay: true}),
	} {
		query := parsed.Query()
		query.Set("g", grant)
		foreign := *parsed
		foreign.RawQuery = query.Encode()
		if response, _ := fetch(t, foreign.String(), nil); response.StatusCode != http.StatusForbidden {
			t.Errorf("%s of the Jellyfin API on the DLNA route: %d, want 403", name, response.StatusCode)
		}
	}
	if response, _ := fetch(t, address, map[string]string{"Range": "bytes=0-3"}); response.StatusCode != http.StatusPartialContent {
		t.Errorf("the device's own address afterwards: %d", response.StatusCode)
	}
}

// relayedChannel is a channel a signed-in device tuned whose HLS playlist
// Polyfin relays, its files named through Polyfin with a grant: the
// address PlaybackInfo gave for it, which carries a grant and no token.
type relayedChannel struct {
	testServer
	token, path string
	device      accounts.Device
}

func tunedToARelayedChannel(t *testing.T) relayedChannel {
	t.Helper()
	s, token, user := tuned(t, newTVAddon(t, false, ""))
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	channel := channels.Items[0].Id
	liveAnalysis(t, s, user, channel)
	device, _, err := s.store.DeviceByToken(t.Context(), token, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	return relayedChannel{testServer: s, token: token, path: livePlaybackInfo(t, s, token, channel, "jellyfin-web-chrome").Path, device: device}
}

// asked is the channel's address with its query changed.
func (c relayedChannel) asked(t *testing.T, change func(url.Values)) string {
	t.Helper()
	parsed, err := url.Parse(c.path)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	change(query)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// file asks for the playlist at an address, with the Authorization header
// given if any, which must answer, and returns the address of the last
// file it names with the grant that address carries.
func (c relayedChannel) file(t *testing.T, playlist, authorization string) (string, playback.Grant) {
	t.Helper()
	response, data := fetch(t, playlist, c.header(authorization))
	body := string(data)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("the playlist %s: %d %s", playlist, response.StatusCode, body)
	}
	var file string
	for line := range strings.Lines(body) {
		if strings.HasPrefix(line, "http") {
			file = strings.TrimSpace(line)
		}
	}
	grant, err := c.handler.Playback.Signer().Verify(mustQuery(t, file, grantParameter))
	if err != nil {
		t.Fatalf("the grant of the file %q: %v", file, err)
	}
	return file, grant
}

// header is the Authorization header of a request, none when empty.
func (relayedChannel) header(authorization string) map[string]string {
	if authorization == "" {
		return nil
	}
	return map[string]string{"Authorization": authorization}
}

// The files a relayed playlist names are the same play's: their grant
// names the device the playlist's request was accepted for, however it
// was asked, by the address PlaybackInfo gave, by the play session alone,
// or by the device's token in a header. Once the device signs out, none
// of them answers, and none opens the playlist again.
func TestTheFilesOfARelayedPlaylistEndWithItsDevicesSignIn(t *testing.T) {
	t.Parallel()
	c := tunedToARelayedChannel(t)
	session := mustQuery(t, c.path, grantParameter)
	type asking struct{ address, authorization string }
	playlists := map[string]asking{
		"the address PlaybackInfo gave": {address: c.path},
		"the play session alone": {address: c.asked(t, func(query url.Values) {
			query.Del(grantParameter)
			query.Del("static")
			query.Set("playSessionId", session)
		})},
		"the device's token": {address: c.asked(t, func(query url.Values) { query.Del(grantParameter) }), authorization: app("tv", c.token)},
	}
	files := map[string]string{}
	for name, playlist := range playlists {
		file, grant := c.file(t, playlist.address, playlist.authorization)
		if grant.Device != c.device.ID || grant.Signed.IsZero() {
			t.Errorf("the files of the playlist asked by %s: a grant for device %s signed %v, want the device %s", name, grant.Device, grant.Signed, c.device.ID)
		}
		if status := mediaStatus(t, file); status != http.StatusOK && status != http.StatusPartialContent {
			t.Errorf("a file of the playlist asked by %s, while signed in: %d", name, status)
		}
		files[name] = file
	}

	if status, body := c.call(http.MethodPost, "/Sessions/Logout", app("tv", c.token), nil); status != http.StatusNoContent {
		t.Fatalf("sign-out: %d %s", status, body)
	}
	for name, playlist := range playlists {
		if response, _ := fetch(t, playlist.address, c.header(playlist.authorization)); response.StatusCode != http.StatusUnauthorized {
			t.Errorf("the playlist asked by %s, once signed out: %d, want 401", name, response.StatusCode)
		}
		if status := mediaStatus(t, files[name]); status != http.StatusUnauthorized {
			t.Errorf("a file of the playlist asked by %s, once signed out: %d, want 401", name, status)
		}
		// The file's grant in the place of the playlist's.
		again := c.asked(t, func(query url.Values) { query.Set(grantParameter, mustQuery(t, files[name], grantParameter)) })
		if status := mediaStatus(t, again); status != http.StatusUnauthorized {
			t.Errorf("the playlist asked with the grant of a file named when asked by %s, once signed out: %d, want 401", name, status)
		}
	}
}

// A grant that names no device, as an API key's caller is given for the
// user it names, has no sign-in to end with: it stands two days from its
// signing and no longer, however often it is used meanwhile.
func TestAGrantThatNamesNoDeviceEndsAfterTwoDays(t *testing.T) {
	t.Parallel()
	p := playing(t)
	var clock atomic.Pointer[time.Time]
	set := func(at time.Time) { clock.Store(&at) }
	start := time.Now().Truncate(time.Second)
	set(start)
	p.handler.now = func() time.Time { return *clock.Load() }
	_, key, err := p.store.CreateAPIKey(t.Context(), "Script")
	if err != nil {
		t.Fatal(err)
	}
	status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo?userId="+p.user.ID.String(), app("script", key),
		map[string]any{"MediaSourceId": p.versions[1].ID.String()})
	var answer playbackAnswer
	if err := json.Unmarshal(data, &answer); status != http.StatusOK || err != nil || len(answer.MediaSources) == 0 {
		t.Fatalf("PlaybackInfo with an API key: %d %s", status, data)
	}
	grant, err := p.handler.Playback.Signer().Verify(answer.PlaySessionId)
	if err != nil || grant.Device != (accounts.ID{}) || grant.User != p.user.ID || !grant.Signed.Equal(start) {
		t.Fatalf("the play session an API key's caller is given: %+v %v, want one for the user, naming no device, signed at %v", grant, err, start)
	}
	stream := p.url + "/Videos/" + p.movie + "/stream?static=true&mediaSourceId=" + answer.MediaSources[0].Id + "&playSessionId=" + url.QueryEscape(answer.PlaySessionId)
	// Asked every half hour from its last hours on: it answers until its
	// two days are over, then no more.
	for at := start.Add(grantLife - 3*grantIdle/2); !at.After(start.Add(grantLife)); at = at.Add(grantIdle / 2) {
		set(at)
		if got := mediaStatus(t, stream); got != http.StatusPartialContent {
			t.Errorf("%v after it was given: %d, want 206", at.Sub(start), got)
		}
	}
	for _, past := range []time.Duration{time.Second, grantIdle / 2} {
		set(start.Add(grantLife + past))
		if got := mediaStatus(t, stream); got != http.StatusUnauthorized {
			t.Errorf("%v past its two days, asked half an hour before at most: %d, want 401", past, got)
		}
	}
}

// A relayed playlist asked with a grant that names no device names its
// files with a grant of that grant's own time, not the clock's: the files
// end with the grant that opened them, and a file's grant put in the
// playlist's place opens nothing past that time either.
func TestTheFilesOfAGrantThatNamesNoDeviceEndWithIt(t *testing.T) {
	t.Parallel()
	c := tunedToARelayedChannel(t)
	var clock atomic.Pointer[time.Time]
	set := func(at time.Time) { clock.Store(&at) }
	start := time.Now().Truncate(time.Second)
	set(start)
	c.handler.now = func() time.Time { return *clock.Load() }
	given, err := c.handler.Playback.Signer().Verify(mustQuery(t, c.path, grantParameter))
	if err != nil {
		t.Fatal(err)
	}
	unnamed := c.handler.grant(playback.Grant{Version: given.Version, User: given.User, Relay: true})
	with := func(grant string) string {
		return c.asked(t, func(query url.Values) { query.Set(grantParameter, grant) })
	}

	// A day later, the playlist names its files with a grant signed when
	// the playlist's was.
	set(start.Add(grantLife / 2))
	file, grant := c.file(t, with(unnamed), "")
	if grant.Device != (accounts.ID{}) || !grant.Signed.Equal(start) {
		t.Fatalf("the files' grant a day later: device %s, signed %v, want no device and %v", grant.Device, grant.Signed, start)
	}
	if status := mediaStatus(t, file); status != http.StatusOK && status != http.StatusPartialContent {
		t.Errorf("a file, a day later: %d", status)
	}
	// So does the playlist asked with the file's grant, an hour before
	// the end.
	set(start.Add(grantLife - grantIdle))
	chained, grant := c.file(t, with(mustQuery(t, file, grantParameter)), "")
	if grant.Device != (accounts.ID{}) || !grant.Signed.Equal(start) {
		t.Errorf("the files' grant of the playlist asked with a file's grant: device %s, signed %v, want no device and %v", grant.Device, grant.Signed, start)
	}
	// Two days after the first grant was signed, all of them are over.
	set(start.Add(grantLife + time.Second))
	for name, address := range map[string]string{
		"the playlist":                              with(unnamed),
		"a file":                                    file,
		"the playlist asked with a file's grant":    with(mustQuery(t, file, grantParameter)),
		"a file of the playlist asked that way":     chained,
		"the playlist asked with that file's grant": with(mustQuery(t, chained, grantParameter)),
	} {
		if status := mediaStatus(t, address); status != http.StatusUnauthorized {
			t.Errorf("%s, two days after the grant was signed: %d, want 401", name, status)
		}
	}
}
