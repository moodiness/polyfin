package trackers

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// The rules Last.fm sets for a scrobble, which ListenBrainz follows too: a
// song is scrobbled once it played for half its length or scrobbleAfter,
// whichever comes first, and never when it lasts shortestSong or less.
const (
	scrobbleAfter = 4 * time.Minute
	shortestSong  = 30 * time.Second
)

// lastFMAuthPage is the page of Last.fm where a user signs in and allows an
// app to scrobble for them.
const lastFMAuthPage = "https://www.last.fm/api/auth/"

// Song identifies a song to the music services, as its music addon names
// it.
type Song struct {
	// Item is the track's identifier in Polyfin.
	Item        accounts.ID `json:"item"`
	Artist      string      `json:"artist"`
	Title       string      `json:"title"`
	Album       string      `json:"album,omitempty"`
	AlbumArtist string      `json:"albumArtist,omitempty"`
	// TrackNumber is the song's place on its album, zero when unknown.
	TrackNumber int `json:"trackNumber,omitempty"`
	// DurationMS is the song's length, zero when unknown.
	DurationMS int64  `json:"durationMs,omitempty"`
	ISRC       string `json:"isrc,omitempty"`
}

func (s Song) duration() time.Duration {
	return time.Duration(s.DurationMS) * time.Millisecond
}

// SongOf identifies the track item of a music addon, played for length (its
// stream's, once analyzed, or the addon's), and reports false for another
// kind of item, or a track without the artist and title the services need.
func SongOf(item library.Item, length time.Duration) (Song, bool) {
	song := Song{Item: item.ID, Title: strings.TrimSpace(item.Name), Album: strings.TrimSpace(item.Album),
		DurationMS: max(length, 0).Milliseconds(), ISRC: item.ISRC}
	if len(item.Artists) > 0 {
		song.Artist = strings.TrimSpace(item.Artists[0].Name)
	}
	// Last.fm wants the album's artist only when it is not the song's.
	if item.AlbumArtist != nil && !strings.EqualFold(strings.TrimSpace(item.AlbumArtist.Name), song.Artist) {
		song.AlbumArtist = strings.TrimSpace(item.AlbumArtist.Name)
	}
	// A track's index is its place where it was listed: on its album, or
	// in a playlist, which is not a track number.
	if item.AlbumID != (accounts.ID{}) && item.ParentID == item.AlbumID && item.IndexNumber > 0 {
		song.TrackNumber = item.IndexNumber
	}
	return song, item.Kind == library.KindTrack && song.Artist != "" && song.Title != ""
}

// Listening is a playback report of a song.
type Listening struct {
	Event Event
	// Device is the device playing.
	Device accounts.ID
	// Position is where playback is in the song, when PositionKnown.
	Position      time.Duration
	PositionKnown bool
	Paused        bool
	Song          Song
	// at is when the report came.
	at time.Time
}

// Listen sends a playback report of a song of user to their music
// services, in the background: the song playing now as it starts, and a
// scrobble once it played long enough.
func (s *Service) Listen(user accounts.ID, report Listening) {
	if s == nil {
		return
	}
	// The time played is counted between reports as they came, not as
	// the background gets to them.
	report.at = s.now()
	s.later(user, func(ctx context.Context) { s.listen(ctx, user, report) })
}

// listen is the playback of a song under way, and how long it played.
type listen struct {
	song Song
	// started is when the song started playing, which its scrobble gives.
	started time.Time
	// last is when the last report came, position where the song was then
	// (when positionKnown), and paused whether it has been paused since.
	last          time.Time
	position      time.Duration
	positionKnown bool
	paused        bool
	// played is how long the song played; scrobbled tells that it was.
	played    time.Duration
	scrobbled bool
}

func newListen(report Listening, started time.Time) *listen {
	return &listen{song: report.Song, started: started, last: report.at, position: report.Position,
		positionKnown: report.PositionKnown, paused: report.Paused}
}

// advance counts the time the song played until at, where it reached
// position when known. Only time spent playing counts, and no more than
// the song moved meanwhile: a pause the player did not report and a seek
// forward add nothing, and a seek back adds nothing until the song plays
// on.
func (l *listen) advance(at time.Time, position time.Duration, known bool) {
	if wall := at.Sub(l.last); wall > 0 && !l.paused {
		played := wall
		if known && l.positionKnown {
			played = min(wall, max(position-l.position, 0))
		}
		l.played += played
	}
	if at.After(l.last) {
		l.last = at
	}
	if known {
		l.position, l.positionKnown = position, true
	}
}

// due reports whether the song has just played long enough to be
// scrobbled: it is, once. A song of unknown length needs scrobbleAfter.
func (l *listen) due() bool {
	if l.scrobbled {
		return false
	}
	need := scrobbleAfter
	if length := l.song.duration(); length > 0 {
		if length <= shortestSong {
			return false
		}
		need = min(length/2, scrobbleAfter)
	}
	if l.played < need {
		return false
	}
	l.scrobbled = true
	return true
}

// scrobble is the change that scrobbles the song of l.
func (l *listen) scrobble() change {
	song, started := l.song, l.started.UTC()
	return change{kind: kindListen, durable: true, event: event{Song: &song, ListenedAt: &started}}
}

func (s *Service) listen(ctx context.Context, user accounts.ID, report Listening) {
	key := sessionKey{user, report.Device}
	services, err := s.active(ctx, user, true)
	if err != nil || len(services) == 0 {
		if report.Event == Stopped {
			s.mu.Lock()
			delete(s.listens, key)
			s.mu.Unlock()
		}
		if err != nil {
			s.logger.Warn("A song could not be sent to music services", "error", err)
		}
		return
	}

	var changes []change
	nowPlaying := false
	s.mu.Lock()
	current := s.listens[key]
	if current != nil && (report.Event == Started || current.song.Item != report.Song.Item) {
		// The device went on to another song, or the same again, without
		// a stop: the one before played until now.
		current.advance(report.at, 0, false)
		if current.due() {
			changes = append(changes, current.scrobble())
		}
		delete(s.listens, key)
		current = nil
	}
	switch {
	case report.Event == Started:
		current = newListen(report, report.at)
		s.listens[key] = current
		nowPlaying = !report.Paused
	case current == nil && report.Event == Stopped:
		// The stop of a song whose start was not seen: how long it played
		// is unknown.
	case current == nil:
		// A report of a song whose start was not seen: its time counts from
		// now, and it started where its position says.
		started := report.at
		if report.PositionKnown {
			started = started.Add(-report.Position)
		}
		current = newListen(report, started)
		s.listens[key] = current
		nowPlaying = !report.Paused
	default:
		current.advance(report.at, report.Position, report.PositionKnown)
		// A song played on after a pause is playing again.
		nowPlaying = current.paused && !report.Paused && report.Event != Stopped
		current.paused = report.Paused
	}
	if current != nil {
		if current.due() {
			changes = append(changes, current.scrobble())
		}
		if report.Event == Stopped {
			delete(s.listens, key)
		}
	}
	s.mu.Unlock()
	if nowPlaying {
		song := report.Song
		changes = append([]change{{kind: kindNowPlaying, event: event{Song: &song}}}, changes...)
	}
	for _, service := range services {
		for _, c := range changes {
			s.queue(ctx, user, service, c)
		}
	}
}

// Last.fm's error codes Polyfin tells apart.
const (
	lastFMOperationFailed    = 8
	lastFMInvalidSession     = 9
	lastFMInvalidAPIKey      = 10
	lastFMOffline            = 11
	lastFMInvalidSignature   = 13
	lastFMTokenNotAuthorized = 14
	lastFMUnavailable        = 16
	lastFMSuspendedAPIKey    = 26
	lastFMRateLimited        = 29
)

// lastFMRefusesApp reports whether code refuses the server's API account: a
// wrong key or shared secret, or a suspended key.
func lastFMRefusesApp(code int) bool {
	return code == lastFMInvalidAPIKey || code == lastFMInvalidSignature || code == lastFMSuspendedAPIKey
}

// lastFMTemporary reports whether code is a failure of Last.fm that passes.
func lastFMTemporary(code int) bool {
	return code == lastFMOperationFailed || code == lastFMOffline || code == lastFMUnavailable
}

// lastFMError reads the error code of a Last.fm answer, zero for none.
func lastFMError(body []byte) int {
	var answer struct {
		Error int `json:"error"`
	}
	_ = json.Unmarshal(body, &answer)
	return answer.Error
}

// lastFMSignature signs the parameters of a call with the shared secret,
// as Last.fm asks: the MD5 of every parameter but format and callback,
// sorted by name, each name followed by its value, then the secret.
func lastFMSignature(params url.Values, secret string) string {
	names := make([]string, 0, len(params))
	for name := range params {
		if name != "format" && name != "callback" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	hash := md5.New()
	for _, name := range names {
		_, _ = io.WriteString(hash, name)
		_, _ = io.WriteString(hash, params.Get(name))
	}
	_, _ = io.WriteString(hash, secret)
	return hex.EncodeToString(hash.Sum(nil))
}

// lastFM calls a method of Last.fm's API with params, signed with the API
// account of settings: method GET for reads, POST, as a form, for writes,
// which keeps the session key out of the address.
func (s *Service) lastFM(ctx context.Context, settings accounts.Settings, method string, params url.Values) (reply, error) {
	params.Set("api_key", settings.LastFMAPIKey)
	params.Set("api_sig", lastFMSignature(params, settings.LastFMSecret))
	params.Set("format", "json")
	header := http.Header{}
	target := s.urls[LastFM]
	var body []byte
	if method == http.MethodGet {
		target += "?" + params.Encode()
	} else {
		header.Set("Content-Type", "application/x-www-form-urlencoded")
		body = []byte(params.Encode())
	}
	return s.call(ctx, method, target, header, body)
}

// lastFMToken asks Last.fm for a request token, and returns the sign-in it
// starts for user: the page where they allow Polyfin, which then asks for
// their session with the token (Last.fm's desktop authentication). Unlike
// the web authentication, nothing comes back to Polyfin through the
// browser, so the admin app works at any address, behind any proxy.
func (s *Service) lastFMToken(ctx context.Context, user accounts.ID, settings accounts.Settings) (*pending, error) {
	r, err := s.lastFM(ctx, settings, http.MethodGet, url.Values{"method": {"auth.getToken"}})
	if err != nil {
		return nil, ErrUnreachable
	}
	code := lastFMError(r.body)
	if lastFMRefusesApp(code) {
		return nil, ErrAppRefused
	}
	var answer struct {
		Token string `json:"token"`
	}
	if r.status != http.StatusOK || code != 0 || json.Unmarshal(r.body, &answer) != nil || answer.Token == "" {
		return nil, ErrUnreachable
	}
	page := lastFMAuthPage + "?" + url.Values{"api_key": {settings.LastFMAPIKey}, "token": {answer.Token}}.Encode()
	return &pending{
		user:            user,
		service:         LastFM,
		deviceCode:      answer.Token,
		verificationURL: page,
		// Last.fm keeps a token an hour; the sign-in waits less.
		expiresAt: s.now().UTC().Add(s.timing.signInTime).Truncate(time.Second),
		interval:  time.Duration(s.timing.signInPoll) * s.timing.pollUnit,
		done:      make(chan struct{}),
	}, nil
}

// pollLastFM asks Last.fm for the session of the token of p, which it gives
// once the user allowed Polyfin.
func (s *Service) pollLastFM(p *pending, settings accounts.Settings) (pollState, tokens) {
	r, err := s.lastFM(s.ctx, settings, http.MethodGet, url.Values{"method": {"auth.getSession"}, "token": {p.deviceCode}})
	if err != nil {
		return waiting, tokens{}
	}
	switch code := lastFMError(r.body); {
	case code == 0 && r.status == http.StatusOK:
		var answer struct {
			Session struct {
				Name string `json:"name"`
				Key  string `json:"key"`
			} `json:"session"`
		}
		if json.Unmarshal(r.body, &answer) != nil || answer.Session.Key == "" {
			return ended, tokens{}
		}
		return approved, tokens{AccessToken: answer.Session.Key, account: answer.Session.Name}
	case code == lastFMTokenNotAuthorized:
		return waiting, tokens{}
	case code == lastFMRateLimited:
		return slowDown, tokens{}
	case lastFMRefusesApp(code):
		return appRefused, tokens{}
	case lastFMTemporary(code), r.status >= 500:
		return waiting, tokens{}
	}
	// The token expired or is unknown.
	return ended, tokens{}
}

// classifyLastFM reads Last.fm's answer to a change, which tells its
// errors by code.
func classifyLastFM(r reply) result {
	res := result{status: r.status}
	switch code := lastFMError(r.body); {
	case code == 0 && r.status >= 200 && r.status < 300:
		res.outcome = delivered
	case code == lastFMInvalidSession:
		// The user revoked Polyfin's access on Last.fm.
		res.outcome = reconnect
	case code == lastFMRateLimited, r.status == http.StatusTooManyRequests:
		res.outcome, res.after = retry, retryAfter(r.header)
	case lastFMTemporary(code), lastFMRefusesApp(code), r.status >= 500:
		// The server's API account is the administrator's to fix; the
		// scrobbles wait for it.
		res.outcome = retry
	default:
		res.outcome = refused
	}
	return res
}

// lastFMDailyLimit is the ignored code of a scrobble past the daily limit
// of the user's account.
const lastFMDailyLimit = 5

// lastFMIgnored reads whether Last.fm ignored the scrobble it answered
// about, and the code it gave.
func lastFMIgnored(body []byte) (bool, int) {
	type message struct {
		IgnoredMessage struct {
			Code any `json:"code"`
		} `json:"ignoredMessage"`
	}
	var answer struct {
		Scrobbles struct {
			Attr struct {
				Ignored any `json:"ignored"`
			} `json:"@attr"`
			Scrobble json.RawMessage `json:"scrobble"`
		} `json:"scrobbles"`
	}
	if json.Unmarshal(body, &answer) != nil || lastFMNumber(answer.Scrobbles.Attr.Ignored) == 0 {
		return false, 0
	}
	// One scrobble comes as an object, several as a list.
	var one message
	if json.Unmarshal(answer.Scrobbles.Scrobble, &one) != nil {
		var many []message
		if json.Unmarshal(answer.Scrobbles.Scrobble, &many) == nil && len(many) > 0 {
			one = many[0]
		}
	}
	return true, lastFMNumber(one.IgnoredMessage.Code)
}

// lastFMNumber reads a number Last.fm gives as a number or a string.
func lastFMNumber(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

// sendLastFM sends the song playing now, or a scrobble, to Last.fm with
// the user's session key.
func (s *Service) sendLastFM(ctx context.Context, sessionKey string, settings accounts.Settings, kind string, ev event) result {
	song := ev.Song
	if song == nil {
		return result{outcome: refused}
	}
	params := url.Values{"sk": {sessionKey}}
	// A scrobble names its fields by their place in a batch: Polyfin
	// sends one at a time.
	suffix := ""
	switch {
	case kind == kindNowPlaying:
		params.Set("method", "track.updateNowPlaying")
	case kind == kindListen && ev.ListenedAt != nil:
		params.Set("method", "track.scrobble")
		suffix = "[0]"
		params.Set("timestamp"+suffix, strconv.FormatInt(ev.ListenedAt.Unix(), 10))
	default:
		return result{outcome: refused}
	}
	set := func(name, value string) {
		if value != "" {
			params.Set(name+suffix, value)
		}
	}
	set("artist", song.Artist)
	set("track", song.Title)
	set("album", song.Album)
	set("albumArtist", song.AlbumArtist)
	if song.TrackNumber > 0 {
		set("trackNumber", strconv.Itoa(song.TrackNumber))
	}
	if seconds := song.duration().Round(time.Second) / time.Second; seconds > 0 {
		set("duration", strconv.Itoa(int(seconds)))
	}
	r, err := s.lastFM(ctx, settings, http.MethodPost, params)
	if err != nil {
		return result{outcome: retry}
	}
	res := classifyLastFM(r)
	if res.outcome == delivered && kind == kindListen {
		switch ignored, code := lastFMIgnored(r.body); {
		case ignored && code == lastFMDailyLimit:
			// The account took all it takes today: tomorrow it will.
			res.outcome = retry
		case ignored:
			// Last.fm will never take it: an artist or song it ignores,
			// or a time too old.
			res.outcome = refused
		}
	}
	return res
}

// sendListenBrainz sends the song playing now, or a listen, to ListenBrainz
// with the user's token.
func (s *Service) sendListenBrainz(ctx context.Context, token string, settings accounts.Settings, kind string, ev event) result {
	song := ev.Song
	if song == nil {
		return result{outcome: refused}
	}
	info := map[string]any{"media_player": "Polyfin", "media_player_version": s.version,
		"submission_client": "Polyfin", "submission_client_version": s.version}
	if song.DurationMS > 0 {
		info["duration_ms"] = song.DurationMS
	}
	if song.TrackNumber > 0 {
		info["tracknumber"] = song.TrackNumber
	}
	if song.ISRC != "" {
		info["isrc"] = song.ISRC
	}
	metadata := map[string]any{"artist_name": song.Artist, "track_name": song.Title, "additional_info": info}
	if song.Album != "" {
		metadata["release_name"] = song.Album
	}
	listen := map[string]any{"track_metadata": metadata}
	var listenType string
	switch {
	case kind == kindNowPlaying:
		listenType = "playing_now"
	case kind == kindListen && ev.ListenedAt != nil:
		listenType = "single"
		listen["listened_at"] = ev.ListenedAt.Unix()
	default:
		return result{outcome: refused}
	}
	r, err := s.api(ctx, ListenBrainz, token, settings, http.MethodPost, "/1/submit-listens", nil,
		map[string]any{"listen_type": listenType, "payload": []any{listen}})
	if err != nil {
		return result{outcome: retry}
	}
	return classify(ListenBrainz, r)
}
