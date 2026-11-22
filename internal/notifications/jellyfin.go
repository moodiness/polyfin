package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/i18n"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/plays"
	"github.com/moodiness/polyfin/internal/userdata"
)

// Jellyfin targets receive what Jellyfin's webhook plugin sends its
// generic destination when it sends all properties: one JSON object whose
// fields are named as the plugin's template variables, such as ItemId,
// Provider_imdb or PlaybackPositionTicks, so that tools made for
// Jellyfin's webhooks read Polyfin's. Each target chooses the plugin's
// notification types it receives, which Polyfin's events map to (see
// jellyfinTypes), and may send headers of its own; a server's target may
// keep to the events of some users. The item an event is about is looked
// up once, as its message is first sent, for every target.

// The notification types of Jellyfin's webhook plugin that Jellyfin
// targets choose from, as their events.
const (
	// JellyfinItemAdded is a new episode of a followed series, or a title
	// of a user's watchlist found available.
	JellyfinItemAdded = "ItemAdded"
	// JellyfinPlaybackStart, JellyfinPlaybackProgress and
	// JellyfinPlaybackStop are a playback's start, its positions, pauses
	// and resumptions, and its stop.
	JellyfinPlaybackStart    = "PlaybackStart"
	JellyfinPlaybackProgress = "PlaybackProgress"
	JellyfinPlaybackStop     = "PlaybackStop"
	// JellyfinUserDataSaved is a user's data saved for an item: played or
	// unplayed, favorite, rated, or the position of a playback.
	JellyfinUserDataSaved = "UserDataSaved"
	// JellyfinUserLockedOut is an account blocked after wrong passwords.
	JellyfinUserLockedOut = "UserLockedOut"
	// jellyfinGeneric is the type of "Send a test".
	jellyfinGeneric = "Generic"
)

// JellyfinEvents lists the notification types Jellyfin targets choose
// from, in the order the admin app shows them.
var JellyfinEvents = []string{JellyfinItemAdded, JellyfinPlaybackStart, JellyfinPlaybackProgress, JellyfinPlaybackStop, JellyfinUserDataSaved,
	JellyfinUserLockedOut}

// jellyfinTypes map Polyfin's events to the notification types Jellyfin
// targets receive them as; no Jellyfin target receives the others.
var jellyfinTypes = map[string]string{
	NewEpisode:         JellyfinItemAdded,
	TitleAvailable:     JellyfinItemAdded,
	PlaybackStarted:    JellyfinPlaybackStart,
	PlaybackPaused:     JellyfinPlaybackProgress,
	PlaybackResumed:    JellyfinPlaybackProgress,
	PlaybackProgressed: JellyfinPlaybackProgress,
	PlaybackStopped:    JellyfinPlaybackStop,
	DataSaved:          JellyfinUserDataSaved,
	AccountBlocked:     JellyfinUserLockedOut,
	Test:               jellyfinGeneric,
}

const (
	// maxHeaders bounds the headers of a Jellyfin target, maxHeaderName
	// the bytes of a header's name and maxHeaderValue those of its value.
	maxHeaders     = 10
	maxHeaderName  = 64
	maxHeaderValue = 1024
	// maxUserFilter bounds the users of a user filter.
	maxUserFilter = 1000
)

// Header is a header a Jellyfin target sends with each message.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// reservedHeaders are the headers the HTTP client writes itself, which a
// Jellyfin target does not set.
var reservedHeaders = []string{"Host", "Content-Length", "Transfer-Encoding", "Connection", "Te", "Trailer", "Upgrade", "Keep-Alive",
	"Proxy-Connection"}

// checkHeaders checks the headers given for a Jellyfin target, whose
// saved headers are saved: at most maxHeaders, each named once, by an
// HTTP token of at most maxHeaderName bytes that is not reserved, and
// valued by at most maxHeaderValue bytes of text without control
// characters. A header given without a value keeps the value saved under
// its name; there must be one. Names are written as HTTP writes them.
func checkHeaders(given, saved []Header) ([]Header, error) {
	if len(given) > maxHeaders {
		return nil, ErrInvalidHeaders
	}
	checked := make([]Header, 0, len(given))
	for _, h := range given {
		name, value := strings.TrimSpace(h.Name), strings.TrimSpace(h.Value)
		if !headerName(name) {
			return nil, ErrInvalidHeaders
		}
		name = http.CanonicalHeaderKey(name)
		named := func(other Header) bool { return other.Name == name }
		if slices.Contains(reservedHeaders, name) || slices.ContainsFunc(checked, named) {
			return nil, ErrInvalidHeaders
		}
		if value == "" {
			i := slices.IndexFunc(saved, named)
			if i < 0 {
				return nil, ErrInvalidHeaders
			}
			value = saved[i].Value
		}
		if len(value) > maxHeaderValue || !utf8.ValidString(value) || strings.ContainsFunc(value, unicode.IsControl) {
			return nil, ErrInvalidHeaders
		}
		checked = append(checked, Header{Name: name, Value: value})
	}
	return checked, nil
}

// headerName reports whether name is an HTTP token of at most
// maxHeaderName bytes.
func headerName(name string) bool {
	if name == "" || len(name) > maxHeaderName {
		return false
	}
	for i := range len(name) {
		c := name[i]
		if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0) {
			return false
		}
	}
	return true
}

// jellyfinSecret is what a Jellyfin target keeps sealed: its address, then
// on a second line, when it has some, its headers as JSON.
type jellyfinSecret struct {
	address string
	headers []Header
}

// openJellyfin reads an opened Jellyfin target's secret.
func openJellyfin(secret string) jellyfinSecret {
	address, headers, _ := strings.Cut(secret, "\n")
	opened := jellyfinSecret{address: address}
	if headers != "" {
		_ = json.Unmarshal([]byte(headers), &opened.headers)
	}
	return opened
}

// String writes j as a Jellyfin target keeps it, before it is sealed.
func (j jellyfinSecret) String() string {
	if len(j.headers) == 0 {
		return j.address
	}
	headers, _ := json.Marshal(j.headers)
	return j.address + "\n" + string(headers)
}

// secrets lists what a target's answer must never show: the address and
// the headers' values.
func (j jellyfinSecret) secrets() []string {
	hidden := []string{j.address}
	for _, h := range j.headers {
		hidden = append(hidden, h.Value)
	}
	return hidden
}

// applyJellyfin checks a Jellyfin target's new address and headers, nil
// to keep the saved ones, and seals them into t, with the address's
// scheme and host shown.
func (s *Service) applyJellyfin(ctx context.Context, t *target, address *string, headers *[]Header, confined bool) error {
	var saved jellyfinSecret
	opened := ""
	if t.secret != "" {
		var err error
		if opened, err = s.box.Open(t.secret); err != nil {
			if address == nil || headers == nil {
				return ErrUnreadable
			}
		} else {
			saved = openJellyfin(opened)
		}
	}
	next := saved
	if address != nil {
		trimmed := strings.TrimSpace(*address)
		parsed, err := s.checkAddress(ctx, trimmed, confined)
		if err != nil {
			return err
		}
		next.address, t.address = trimmed, parsed.Scheme+"://"+parsed.Host
	}
	if headers != nil {
		checked, err := checkHeaders(*headers, saved.headers)
		if err != nil {
			return err
		}
		next.headers = checked
	}
	if t.secret != "" && opened != "" && next.String() == opened {
		// Nothing changed: the target keeps its problem.
		return nil
	}
	t.secret = s.box.Seal(next.String())
	return nil
}

// jellyfinSource is what Jellyfin targets are told of an event beyond the
// event itself: the item it is about, the playback that plays it, and a
// user's data saved for it. It is shared by every target's copy of the
// event, and the item looked up once (see resolve).
type jellyfinSource struct {
	// viewer is the user as whom the item and its series are looked up,
	// zero to look nothing up.
	viewer accounts.ID
	// itemID is the item, looked up when item is nil; title describes it
	// as a playback knew it, when the lookup fails.
	itemID accounts.ID
	item   *library.Item
	title  *plays.Title
	// session is the playback, data and reason the user's data saved and
	// why (see userdata.SavedPlaybackStart and the others).
	session *jellyfinSession
	data    *userdata.Data
	reason  string

	// once looks the item and its series up; series is an episode's or a
	// season's series, nil when it cannot be.
	once   sync.Once
	series *library.Item
}

// jellyfinSession is a playback as a Jellyfin target is told it: the app's
// identifier of its device, the device's and app's names, where it is,
// whether it is paused, how it reaches the app (see plays.Methods), and
// whether it stopped.
type jellyfinSession struct {
	deviceID, deviceName, app string
	position                  time.Duration
	paused                    bool
	method                    string
	stopped                   bool
}

// resolve looks up the item of src and its series as src's viewer sees
// them, once, within a request's time: what cannot be found is left out.
func (s *Service) resolve(src *jellyfinSource) {
	src.once.Do(func() {
		if src.viewer == (accounts.ID{}) {
			return
		}
		ctx, cancel := context.WithTimeout(s.ctx, s.timing.request)
		defer cancel()
		viewer, err := s.accounts.User(ctx, src.viewer)
		if err != nil {
			return
		}
		if src.item == nil && src.itemID != (accounts.ID{}) {
			if item, err := s.library.Item(ctx, viewer, src.itemID); err == nil {
				src.item = &item
			}
		}
		if item := src.item; item != nil && (item.Kind == library.KindEpisode || item.Kind == library.KindSeason) && item.SeriesID != (accounts.ID{}) {
			if series, err := s.library.Item(ctx, viewer, item.SeriesID); err == nil {
				src.series = &series
			}
		}
	})
}

// jellyfinBody is the JSON object a Jellyfin target receives for ev.
func (s *Service) jellyfinBody(ev Event) map[string]any {
	body := map[string]any{
		"NotificationType": jellyfinTypes[ev.Type],
		"ServerId":         s.serverID,
		"ServerName":       ev.Server.Name,
		"ServerVersion":    s.jellyfinVersion,
		"ServerUrl":        "",
		"Timestamp":        ev.At.In(s.zone),
		"UtcTimestamp":     ev.At.UTC(),
	}
	if ev.Server.URL != nil {
		body["ServerUrl"] = *ev.Server.URL
	}
	if ev.User != nil {
		body["NotificationUsername"], body["Username"], body["UserId"] = ev.User.Name, ev.User.Name, ev.User.ID
	}
	if ev.Type == Test {
		body["Name"], body["Description"] = ev.Title, ev.Message
	}
	src := ev.jellyfin
	if src == nil {
		return body
	}
	s.resolve(src)
	switch {
	case src.item != nil:
		addJellyfinItem(body, *src.item, src.series)
	case src.title != nil:
		addJellyfinTitle(body, *src.title)
	}
	if session := src.session; session != nil {
		body["DeviceId"], body["DeviceName"] = session.deviceID, session.deviceName
		body["ClientName"], body["Client"] = session.app, session.app
		body["PlaybackPositionTicks"], body["PlaybackPosition"] = ticks(session.position), timeSpan(session.position)
		body["IsPaused"], body["IsAutomated"] = session.paused, false
		if method := jellyfinPlayMethods[session.method]; method != "" {
			body["PlayMethod"] = method
		}
		if session.stopped {
			// As far into the item as marks it played.
			settings := s.accounts.Settings()
			thresholds := userdata.Thresholds{Resume: settings.ResumePercent, Played: settings.PlayedPercent}
			body["PlayedToCompletion"] = src.item != nil && thresholds.Reaches(session.position, src.item.Runtime)
		}
	}
	if d := src.data; d != nil {
		likes := d.Likes()
		body["Likes"] = likes != nil && *likes
		body["Rating"] = 0.0
		if d.Rating != nil {
			body["Rating"] = *d.Rating
		}
		body["PlaybackPositionTicks"], body["PlaybackPosition"] = ticks(d.Position), timeSpan(d.Position)
		body["PlayCount"], body["Favorite"], body["Played"] = d.PlayCount, d.Favorite, d.Played
		if d.LastPlayed != nil {
			body["LastPlayedDate"] = d.LastPlayed.UTC()
		}
		body["SaveReason"] = src.reason
	}
	return body
}

// addJellyfinItem adds what Jellyfin's webhook plugin tells of item to
// body, with its series for an episode or a season, nil when unknown.
func addJellyfinItem(body map[string]any, item library.Item, series *library.Item) {
	body["Name"], body["Overview"], body["Tagline"] = item.Name, item.Overview, ""
	if len(item.Taglines) > 0 {
		body["Tagline"] = item.Taglines[0]
	}
	body["ItemId"], body["ItemType"] = item.ID.String(), jellyfinItemTypes[item.Kind]
	if body["ItemType"] == "" {
		body["ItemType"] = "Video"
	}
	body["RunTimeTicks"], body["RunTime"] = ticks(item.Runtime), timeSpan(item.Runtime)
	body["Genres"] = strings.Join(item.Genres, ", ")
	if item.ProductionYear > 0 {
		body["Year"] = item.ProductionYear
	}
	if item.PremiereDate != nil {
		body["PremiereDate"] = item.PremiereDate.UTC().Format(time.DateOnly)
	}
	for provider, id := range item.ProviderIDs {
		if id != "" {
			body["Provider_"+strings.ToLower(provider)] = id
		}
	}
	switch item.Kind {
	case library.KindEpisode, library.KindSeason:
		body["SeriesName"] = item.SeriesName
		if item.SeriesID != (accounts.ID{}) {
			body["SeriesId"] = item.SeriesID.String()
		}
		if item.Kind == library.KindSeason {
			addNumber(body, "SeasonNumber", item.IndexNumber)
		} else {
			if item.SeasonID != (accounts.ID{}) {
				body["SeasonId"] = item.SeasonID.String()
			}
			addNumber(body, "SeasonNumber", item.ParentIndexNumber)
			addNumber(body, "EpisodeNumber", item.IndexNumber)
		}
		// The year of an episode or a season is its series'.
		if series != nil && series.ProductionYear > 0 {
			body["Year"] = series.ProductionYear
		}
		if series != nil && series.PremiereDate != nil {
			body["SeriesPremiereDate"] = series.PremiereDate.UTC().Format(time.DateOnly)
		}
	case library.KindTrack, library.KindAudiobook, library.KindAlbum:
		if item.Album != "" {
			body["Album"] = item.Album
		}
		if artists := creditsOr(item.Artists, item.AlbumArtists); len(artists) > 0 {
			body["Artist"] = artists[0].Name
		}
	}
}

// creditsOr is a, or b when a is empty.
func creditsOr(a, b []library.Credit) []library.Credit {
	if len(a) > 0 {
		return a
	}
	return b
}

// addJellyfinTitle adds what a playback knew of its title to body, when
// its item cannot be looked up.
func addJellyfinTitle(body map[string]any, t plays.Title) {
	body["Name"], body["ItemId"], body["ItemType"] = t.Name, t.Item.String(), jellyfinTitleTypes[t.Kind]
	body["RunTimeTicks"], body["RunTime"] = int64(0), timeSpan(0)
	if t.Kind == plays.Episode {
		body["SeriesName"] = t.SeriesName
		if t.SeriesID != (accounts.ID{}) {
			body["SeriesId"] = t.SeriesID.String()
		}
		addNumber(body, "SeasonNumber", t.Season)
		addNumber(body, "EpisodeNumber", t.Episode)
	}
	if t.Artist != "" {
		body["Artist"] = t.Artist
	}
}

// addNumber adds a season's or episode's number to body under name, and
// written on two and three digits, as name00 and name000.
func addNumber(body map[string]any, name string, n int) {
	body[name], body[name+"00"], body[name+"000"] = n, fmt.Sprintf("%02d", n), fmt.Sprintf("%03d", n)
}

// ticks is d in Jellyfin's ticks, of 100 nanoseconds.
func ticks(d time.Duration) int64 {
	return int64(max(d, 0) / 100)
}

// timeSpan writes d as hh:mm:ss, as .NET writes a TimeSpan so: its days
// are left out.
func timeSpan(d time.Duration) string {
	total := int64(max(d, 0) / time.Second)
	return fmt.Sprintf("%02d:%02d:%02d", total/3600%24, total/60%60, total%60)
}

// jellyfinItemTypes name the kinds of items as Jellyfin's webhook plugin
// does, by Jellyfin's classes of items.
var jellyfinItemTypes = map[library.Kind]string{
	library.KindMovie:         "Movie",
	library.KindVideo:         "Video",
	library.KindFolder:        "Folder",
	library.KindCollection:    "BoxSet",
	library.KindSeries:        "Series",
	library.KindSeason:        "Season",
	library.KindEpisode:       "Episode",
	library.KindChannel:       "LiveTvChannel",
	library.KindProgram:       "LiveTvProgram",
	library.KindRecording:     "Video",
	library.KindReplay:        "Video",
	library.KindArtist:        "MusicArtist",
	library.KindAlbum:         "MusicAlbum",
	library.KindTrack:         "Audio",
	library.KindAudiobook:     "AudioBook",
	library.KindMusicPlaylist: "Playlist",
	library.KindExtra:         "Video",
	library.KindThemeSong:     "Audio",
}

// jellyfinTitleTypes are the same for the kinds of what plays.
var jellyfinTitleTypes = map[string]string{
	plays.Movie:     "Movie",
	plays.Episode:   "Episode",
	plays.Channel:   "LiveTvChannel",
	plays.Recording: "Video",
	plays.Replay:    "Video",
	plays.HomeVideo: "Video",
	plays.Song:      "Audio",
	plays.Audiobook: "AudioBook",
}

// jellyfinPlayMethods name the play methods as Jellyfin does.
var jellyfinPlayMethods = map[string]string{
	plays.DirectPlay:   "DirectPlay",
	plays.DirectStream: "DirectStream",
	plays.Conversion:   "Transcode",
}

// paceProgress reports whether the change c of a playback is told: its
// positions at most every timing.progressEvery, and each change of its
// state, which counts as a position. A stop forgets the playback.
func (s *Service) paceProgress(c plays.Change) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	device := c.Playback.Device
	switch c.Kind {
	case plays.Stopped:
		delete(s.progressed, device)
		return true
	case plays.Progressed:
		if last, ok := s.progressed[device]; ok && c.At().Sub(last) < s.timing.progressEvery {
			return false
		}
	}
	s.progressed[device] = c.At()
	return true
}

// savedKey is a user's item.
type savedKey struct {
	user, item accounts.ID
}

// paceSaved reports whether a user's data for item, saved for reason, is
// told: the positions a playback reports at most every
// timing.progressEvery per user and item, counted from its start; every
// other save.
func (s *Service) paceSaved(user, item accounts.ID, reason string) bool {
	key := savedKey{user: user, item: item}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	switch reason {
	case userdata.SavedPlaybackStart:
		s.saved[key] = now
	case userdata.SavedPlaybackFinished:
		delete(s.saved, key)
	case userdata.SavedPlaybackProgress:
		for other, at := range s.saved {
			if now.Sub(at) >= s.timing.progressEvery {
				delete(s.saved, other)
			}
		}
		if _, ok := s.saved[key]; ok {
			return false
		}
		s.saved[key] = now
	}
	return true
}

// UserDataSaved tells the Jellyfin targets of user, and the server's, that
// user's data for item was saved for reason, one of the userdata.Saved
// reasons, data being it afterwards. The positions saved as a playback
// reports them are told at most every timing.progressEvery per user and
// item. It returns at once.
func (s *Service) UserDataSaved(user accounts.User, item library.Item, data userdata.Data, reason string) {
	if !s.paceSaved(user.ID, item.ID, reason) {
		return
	}
	ev := s.newEvent(DataSaved)
	ev.User = &UserJSON{ID: user.ID.String(), Name: user.Name}
	ev.jellyfin = &jellyfinSource{viewer: user.ID, item: &item, data: &data, reason: reason}
	name := item.Name
	if item.Kind == library.KindEpisode && item.SeriesName != "" {
		name = fmt.Sprintf("%s S%02dE%02d", item.SeriesName, item.ParentIndexNumber, item.IndexNumber)
	}
	ev.Title = s.phrase(i18n.Text{"User data saved: %s", "Données utilisateur enregistrées\u202f: %s", "Benutzerdaten gespeichert: %s",
		"Datos de usuario guardados: %s", "Dati utente salvati: %s", "Dados do usuário salvos: %s"}, name)
	ev.Message = user.Name + " · " + reason
	ev.URL = s.webLink(item.ID)
	s.dispatch(ev, func(t target) bool { return t.owner == nil || *t.owner == user.ID })
}
