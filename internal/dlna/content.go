package dlna

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

type clientKey struct{}

func withClient(ctx context.Context, c client) context.Context {
	return context.WithValue(ctx, clientKey{}, c)
}

func clientFrom(ctx context.Context) client {
	c, _ := ctx.Value(clientKey{}).(client)
	return c
}

// randomUUID is a random UUID, for event subscriptions.
func randomUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// The objects a device browses are named by an ID that tells what they
// are: the root, the lists of the user, or an item with how it is listed.
const (
	rootID   = "0"
	resumeID = "resume"
	nextUpID = "nextup"
	liveTvID = "livetv"
	// The prefixes of items: a library or folder listed through the
	// catalogs, a series, a season (series.season), a music folder, and a
	// leaf, which plays.
	folderPrefix = "C"
	seriesPrefix = "S"
	seasonPrefix = "E"
	musicPrefix  = "M"
	leafPrefix   = "I"
)

// maxPage is how many objects a request for every child returns at most:
// devices that ask for all of a catalog are given it page by page.
const maxPage = 200

// object is a container or an item of the ContentDirectory.
type object struct {
	id, parent string
	title      string
	container  bool
	// class is its UPnP class.
	class string
	item  library.Item
	// hasItem tells item describes it.
	hasItem bool
	// children is the count of a container's children, -1 unknown.
	children int
}

// objectOf names an item listed under parent, and tells whether a device
// can browse or play it.
func objectOf(item library.Item, parent string) (object, bool) {
	o := object{parent: parent, title: item.Name, item: item, hasItem: true, children: -1}
	id := item.ID.String()
	switch item.Kind {
	case library.KindMovie:
		o.id, o.class = leafPrefix+id, "object.item.videoItem.movie"
	case library.KindEpisode, library.KindVideo, library.KindRecording, library.KindReplay:
		o.id, o.class = leafPrefix+id, "object.item.videoItem"
	case library.KindChannel:
		o.id, o.class = leafPrefix+id, "object.item.videoItem.videoBroadcast"
	case library.KindTrack, library.KindAudiobook:
		o.id, o.class = leafPrefix+id, "object.item.audioItem.musicTrack"
	case library.KindSeries:
		o.id, o.class, o.container = seriesPrefix+id, "object.container.storageFolder", true
	case library.KindSeason:
		o.id, o.class, o.container = seasonPrefix+item.SeriesID.String()+"."+id, "object.container.storageFolder", true
	case library.KindAlbum:
		o.id, o.class, o.container = musicPrefix+id, "object.container.album.musicAlbum", true
	case library.KindArtist:
		o.id, o.class, o.container = musicPrefix+id, "object.container.person.musicArtist", true
	case library.KindMusicPlaylist:
		o.id, o.class, o.container = musicPrefix+id, "object.container.playlistContainer", true
	case library.KindLibrary, library.KindCollection:
		o.id, o.class, o.container = folderPrefix+id, "object.container.storageFolder", true
		if item.CollectionType == "music" {
			o.id = musicPrefix + id
		}
	default:
		return o, false
	}
	return o, true
}

// listText are the names of the server's own lists, by language.
var listText = map[string]map[string]string{
	"en": {resumeID: "Continue Watching", nextUpID: "Next Up", liveTvID: "Live TV"},
	"fr": {resumeID: "Reprendre", nextUpID: "À suivre", liveTvID: "TV en direct"},
}

// list is one of the server's own lists, under the root.
func (s *Service) list(id string) object {
	text, ok := listText[s.options.Settings().Language]
	if !ok {
		text = listText["en"]
	}
	return object{id: id, parent: rootID, title: text[id], container: true, class: "object.container.storageFolder", children: -1}
}

// roots lists what the root shows the user: their views as Jellyfin apps
// see them, their libraries and local folders, music included, Live TV
// when they have channels, the Watchlist, then Continue Watching and Next
// Up.
func (s *Service) roots(ctx context.Context, user accounts.User) ([]object, error) {
	libraries, err := s.options.Library.Libraries(ctx, user)
	if err != nil {
		return nil, err
	}
	var objects []object
	for _, l := range libraries {
		if o, ok := objectOf(l, rootID); ok {
			objects = append(objects, o)
		}
	}
	if has, err := s.options.Library.HasChannels(ctx, user); err != nil {
		return nil, err
	} else if has {
		objects = append(objects, s.list(liveTvID))
	}
	watchlist, err := s.options.Library.Item(ctx, user, library.WatchlistViewID)
	switch {
	case err == nil:
		o, _ := objectOf(library.Item{ID: watchlist.ID, Kind: library.KindLibrary, Name: watchlist.Name}, rootID)
		objects = append(objects, o)
	case !errors.Is(err, library.ErrNotFound):
		return nil, err
	}
	return append(objects, s.list(resumeID), s.list(nextUpID)), nil
}

// window returns the part of a list a request asks for.
func window(objects []object, start, count int) []object {
	start = min(max(start, 0), len(objects))
	return objects[start:min(start+count, len(objects))]
}

// objects returns the objects of items under parent that a device can
// browse or play.
func objects(items []library.Item, parent string) []object {
	var result []object
	for _, item := range items {
		if o, ok := objectOf(item, parent); ok {
			result = append(result, o)
		}
	}
	return result
}

// children lists a container's children from start, count of them, and
// how many there are in all.
func (s *Service) children(ctx context.Context, user accounts.User, id string, start, count int) ([]object, int, error) {
	whole := func(items []library.Item, err error) ([]object, int, error) {
		if err != nil {
			return nil, 0, err
		}
		all := objects(items, id)
		return window(all, start, count), len(all), nil
	}
	switch {
	case id == rootID:
		all, err := s.roots(ctx, user)
		if err != nil {
			return nil, 0, err
		}
		return window(all, start, count), len(all), nil
	case id == resumeID:
		return whole(s.options.Player.ContinueWatching(ctx, user))
	case id == nextUpID:
		return whole(s.options.Player.NextUp(ctx, user))
	case id == liveTvID:
		return whole(s.options.Library.Channels(ctx, user))
	}
	prefix, rest := id[:1], id[1:]
	switch prefix {
	case folderPrefix:
		parent, err := accounts.ParseID(rest)
		if err != nil {
			return nil, 0, library.ErrNotFound
		}
		page, err := s.options.Library.Children(ctx, user, parent, start, count, "")
		if err != nil {
			return nil, 0, err
		}
		// What cannot be browsed nor played is left out of the page, not
		// of the total, which the catalogs alone know.
		return objects(page.Items, id), page.Total, nil
	case seriesPrefix:
		series, err := accounts.ParseID(rest)
		if err != nil {
			return nil, 0, library.ErrNotFound
		}
		return whole(s.options.Library.Seasons(ctx, user, series))
	case seasonPrefix:
		seriesText, seasonText, _ := strings.Cut(rest, ".")
		series, err := accounts.ParseID(seriesText)
		season, err2 := accounts.ParseID(seasonText)
		if err != nil || err2 != nil {
			return nil, 0, library.ErrNotFound
		}
		return whole(s.options.Library.Episodes(ctx, user, series, &season))
	case musicPrefix:
		folder, err := s.item(ctx, user, rest)
		if err != nil {
			return nil, 0, err
		}
		q := library.MusicQuery{Parent: folder.ID, Kinds: []library.Kind{library.KindTrack, library.KindAudiobook}}
		switch folder.Kind {
		case library.KindLibrary, library.KindCollection:
			q.Kinds, q.Recursive = []library.Kind{library.KindAlbum, library.KindMusicPlaylist}, true
		case library.KindArtist:
			q = library.MusicQuery{ArtistIDs: []accounts.ID{folder.ID}, Kinds: []library.Kind{library.KindAlbum}}
		}
		return whole(s.options.Library.Music(ctx, user, q))
	}
	return nil, 0, library.ErrNotFound
}

// item reads the item an ID's rest names: a library among the user's, or
// any other item.
func (s *Service) item(ctx context.Context, user accounts.User, rest string) (library.Item, error) {
	id, err := accounts.ParseID(rest)
	if err != nil {
		return library.Item{}, library.ErrNotFound
	}
	libraries, err := s.options.Library.Libraries(ctx, user)
	if err != nil {
		return library.Item{}, err
	}
	for _, l := range libraries {
		if l.ID == id {
			return l, nil
		}
	}
	return s.options.Library.Item(ctx, user, id)
}

// metadata describes an object itself.
func (s *Service) metadata(ctx context.Context, user accounts.User, id string) (object, error) {
	switch id {
	case rootID:
		return object{id: rootID, parent: "-1", title: s.name(), container: true, class: "object.container", children: -1}, nil
	case resumeID, nextUpID, liveTvID:
		return s.list(id), nil
	case "":
		return object{}, library.ErrNotFound
	}
	rest := id[1:]
	if id[0] == seasonPrefix[0] {
		_, rest, _ = strings.Cut(rest, ".")
	}
	item, err := s.item(ctx, user, rest)
	if err != nil {
		return object{}, err
	}
	o, ok := objectOf(item, s.parentOf(item))
	if !ok {
		return object{}, library.ErrNotFound
	}
	return o, nil
}

// parentOf names the container an item is listed in, as far as it tells.
func (s *Service) parentOf(item library.Item) string {
	switch {
	case item.Kind == library.KindEpisode && item.SeasonID != (accounts.ID{}):
		return seasonPrefix + item.SeriesID.String() + "." + item.SeasonID.String()
	case item.Kind == library.KindSeason:
		return seriesPrefix + item.SeriesID.String()
	case item.Kind == library.KindLibrary:
		return rootID
	case item.ParentID != (accounts.ID{}):
		return folderPrefix + item.ParentID.String()
	}
	return rootID
}

// contentDirectoryAction answers ContentDirectory.
func (s *Service) contentDirectoryAction(r *http.Request, action soapAction) ([][2]string, *upnpError) {
	switch action.name {
	case "GetSearchCapabilities":
		return [][2]string{{"SearchCaps", "dc:title,upnp:class"}}, nil
	case "GetSortCapabilities":
		return [][2]string{{"SortCaps", ""}}, nil
	case "GetSystemUpdateID":
		return [][2]string{{"Id", "1"}}, nil
	case "Browse":
		return s.browse(r, action)
	case "Search":
		return s.search(r, action)
	case "X_GetFeatureList":
		return [][2]string{{"FeatureList", `<?xml version="1.0" encoding="utf-8"?><Features xmlns="urn:schemas-upnp-org:av:avs" ` +
			`xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="urn:schemas-upnp-org:av:avs ` +
			`http://www.upnp.org/schemas/av/avs.xsd"><Feature name="samsung.com_BASICVIEW" version="1">` +
			`<container id="` + rootID + `" type="object.item.videoItem"/></Feature></Features>`}}, nil
	}
	return nil, &errInvalidAction
}

// paging reads StartingIndex and RequestedCount: 0 asks for every child,
// given up to maxPage at a time.
func paging(action soapAction) (start, count int, ok bool) {
	start, err := strconv.Atoi(strings.TrimSpace(action.args["StartingIndex"]))
	if err != nil || start < 0 {
		return 0, 0, false
	}
	count, err = strconv.Atoi(strings.TrimSpace(action.args["RequestedCount"]))
	if err != nil || count < 0 {
		return 0, 0, false
	}
	if count == 0 || count > maxPage {
		count = maxPage
	}
	return start, count, true
}

// browse answers Browse: an object's children, a page at a time, or the
// object itself. A device that shows no one's libraries browses an empty
// root.
func (s *Service) browse(r *http.Request, action soapAction) ([][2]string, *upnpError) {
	c := clientFrom(r.Context())
	start, count, ok := paging(action)
	if !ok {
		return nil, &errInvalidArgs
	}
	id := action.args["ObjectID"]
	if !c.hasUser && id != rootID {
		return nil, &errNoSuchObject
	}
	var listed []object
	total := 0
	var err error
	switch action.args["BrowseFlag"] {
	case "BrowseMetadata":
		var o object
		if !c.hasUser {
			o, err = object{id: rootID, parent: "-1", title: s.name(), container: true, class: "object.container", children: 0}, nil
		} else {
			o, err = s.metadata(r.Context(), c.user, id)
		}
		listed, total = []object{o}, 1
	case "BrowseDirectChildren":
		if c.hasUser {
			listed, total, err = s.children(r.Context(), c.user, id, start, count)
		}
	default:
		return nil, &errInvalidArgs
	}
	if errors.Is(err, library.ErrNotFound) || errors.Is(err, ErrNotFound) {
		return nil, &errNoSuchObject
	}
	if err != nil {
		if r.Context().Err() == nil {
			s.options.Logger.Warn("A DLNA device's browsing failed", "object", id, "error", err)
		}
		return nil, &errActionFailed
	}
	return s.result(r, c, listed, total), nil
}

// result is a Browse or Search answer.
func (s *Service) result(r *http.Request, c client, listed []object, total int) [][2]string {
	return [][2]string{{"Result", s.didl(r, c, listed)}, {"NumberReturned", strconv.Itoa(len(listed))},
		{"TotalMatches", strconv.Itoa(max(total, len(listed)))}, {"UpdateID", "1"}}
}

// titleTerm reads the title a SearchCriteria looks for, as in
// dc:title contains "word".
var titleTerm = regexp.MustCompile(`(?i)dc:title\s+contains\s+"((?:[^"\\]|\\.)*)"`)

// search answers Search: the titles whose name holds the words asked,
// through the catalogs' search, songs and albums when the device asks for
// audio.
func (s *Service) search(r *http.Request, action soapAction) ([][2]string, *upnpError) {
	c := clientFrom(r.Context())
	start, count, ok := paging(action)
	if !ok {
		return nil, &errInvalidArgs
	}
	criteria := action.args["SearchCriteria"]
	match := titleTerm.FindStringSubmatch(criteria)
	if !c.hasUser || match == nil || strings.TrimSpace(match[1]) == "" {
		return s.result(r, c, nil, 0), nil
	}
	term := strings.ReplaceAll(strings.TrimSpace(match[1]), `\"`, `"`)
	audio := strings.Contains(strings.ToLower(criteria), "audioitem") || strings.Contains(strings.ToLower(criteria), "musicalbum")
	video := strings.Contains(strings.ToLower(criteria), "videoitem")
	limit := start + count
	var items []library.Item
	var err error
	if audio && !video {
		items, err = s.options.Library.SearchMusic(r.Context(), c.user, term, []library.Kind{library.KindTrack, library.KindAlbum}, limit)
	} else {
		items, err = s.options.Library.Search(r.Context(), c.user, term,
			[]library.Kind{library.KindMovie, library.KindSeries, library.KindVideo}, limit)
	}
	if err != nil {
		if r.Context().Err() == nil {
			s.options.Logger.Warn("A DLNA device's search failed", "error", err)
		}
		return nil, &errActionFailed
	}
	all := objects(items, action.args["ContainerID"])
	return s.result(r, c, window(all, start, count), len(all)), nil
}

// didl writes objects as DIDL-Lite: containers with their posters, and
// items with the one resource the device is offered.
func (s *Service) didl(r *http.Request, c client, listed []object) string {
	base := "http://" + r.Host
	var b strings.Builder
	b.WriteString(`<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
		`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/" xmlns:sec="http://www.sec.co.kr/">`)
	for _, o := range listed {
		if o.container {
			fmt.Fprintf(&b, `<container id="%s" parentID="%s" restricted="1" searchable="0"`, attr(o.id), attr(o.parent))
			if o.children >= 0 {
				fmt.Fprintf(&b, ` childCount="%d"`, o.children)
			}
			b.WriteString(">")
			writeElement(&b, "dc:title", o.title)
			writeElement(&b, "upnp:class", o.class)
			s.writeArt(&b, base, o)
			b.WriteString("</container>")
			continue
		}
		resource, ok := s.options.Player.Resource(r.Context(), c.user, o.item, c.profile)
		if !ok {
			continue
		}
		fmt.Fprintf(&b, `<item id="%s" parentID="%s" restricted="1">`, attr(o.id), attr(o.parent))
		writeElement(&b, "dc:title", itemTitle(o))
		writeElement(&b, "upnp:class", o.class)
		item := o.item
		if item.Overview != "" {
			writeElement(&b, "dc:description", item.Overview)
		}
		if item.PremiereDate != nil {
			writeElement(&b, "dc:date", item.PremiereDate.Format("2006-01-02"))
		}
		for _, genre := range item.Genres {
			writeElement(&b, "upnp:genre", genre)
		}
		if item.Kind == library.KindEpisode {
			writeElement(&b, "upnp:seriesTitle", item.SeriesName)
			if item.IndexNumber > 0 {
				writeElement(&b, "upnp:episodeNumber", strconv.Itoa(item.IndexNumber))
			}
			if item.ParentIndexNumber > 0 {
				writeElement(&b, "upnp:episodeSeason", strconv.Itoa(item.ParentIndexNumber))
			}
		}
		if item.Kind == library.KindTrack {
			for _, artist := range item.Artists {
				writeElement(&b, "upnp:artist", artist.Name)
			}
			if item.Album != "" {
				writeElement(&b, "upnp:album", item.Album)
			}
			if item.IndexNumber > 0 {
				writeElement(&b, "upnp:originalTrackNumber", strconv.Itoa(item.IndexNumber))
			}
		}
		if item.Kind == library.KindChannel && item.Number != "" {
			writeElement(&b, "upnp:channelNr", item.Number)
		}
		s.writeArt(&b, base, o)
		address := mediaURL(base, "media", c.user.ID, item.ID, resource.Extension, s, resource.Converted)
		b.WriteString(`<res protocolInfo="` + attr("http-get:*:"+resource.MimeType+":"+ContentFeatures(resource)) + `"`)
		if resource.Size > 0 && !resource.Converted {
			fmt.Fprintf(&b, ` size="%d"`, resource.Size)
		}
		if resource.Duration > 0 {
			fmt.Fprintf(&b, ` duration="%s"`, NPT(resource.Duration))
		}
		if resource.Width > 0 && resource.Height > 0 {
			fmt.Fprintf(&b, ` resolution="%dx%d"`, resource.Width, resource.Height)
		}
		if resource.Bitrate > 0 {
			// In bytes per second, as UPnP says.
			fmt.Fprintf(&b, ` bitrate="%d"`, resource.Bitrate/8)
		}
		if resource.Channels > 0 {
			fmt.Fprintf(&b, ` nrAudioChannels="%d"`, resource.Channels)
		}
		if resource.Sampling > 0 {
			fmt.Fprintf(&b, ` sampleFrequency="%d"`, resource.Sampling)
		}
		b.WriteString(">")
		_ = xmlEscape(&b, address)
		b.WriteString("</res>")
		if resource.Captions && c.profile.TakesCaptions() {
			captions := mediaURL(base, "captions", c.user.ID, item.ID, "srt", s, false)
			b.WriteString(`<res protocolInfo="http-get:*:text/srt:*">`)
			_ = xmlEscape(&b, captions)
			b.WriteString(`</res><sec:CaptionInfoEx sec:type="srt">`)
			_ = xmlEscape(&b, captions)
			b.WriteString(`</sec:CaptionInfoEx>`)
		}
		b.WriteString("</item>")
	}
	b.WriteString("</DIDL-Lite>")
	return b.String()
}

// itemTitle is the title an item shows: an episode outside its season
// names its series, season and number.
func itemTitle(o object) string {
	item := o.item
	if item.Kind == library.KindEpisode && !strings.HasPrefix(o.parent, seasonPrefix) {
		return fmt.Sprintf("%s · S%02dE%02d · %s", item.SeriesName, item.ParentIndexNumber, item.IndexNumber, item.Name)
	}
	if item.Kind == library.KindChannel && item.Number != "" {
		return item.Number + " " + item.Name
	}
	return o.title
}

// writeArt writes an object's poster, for those that have one.
func (s *Service) writeArt(b *strings.Builder, base string, o object) {
	if !o.hasItem || o.item.Images.Primary == "" && o.item.SeriesPoster == "" && o.item.AlbumPoster == "" {
		return
	}
	b.WriteString(`<upnp:albumArtURI dlna:profileID="JPEG_TN">`)
	_ = xmlEscape(b, base+Prefix+"art/"+o.item.ID.String()+".jpg")
	b.WriteString(`</upnp:albumArtURI>`)
}

// mediaURL is the address of an item's bytes or captions, signed for user.
func mediaURL(base, route string, user, item accounts.ID, extension string, s *Service, converted bool) string {
	query := url.Values{"g": {s.options.Signer.Sign(grantFor(user, item))}}
	if converted {
		query.Set("c", "1")
	}
	return base + Prefix + route + "/" + item.String() + "." + extension + "?" + query.Encode()
}

func attr(text string) string {
	var b strings.Builder
	_ = xmlEscape(&b, text)
	return b.String()
}
