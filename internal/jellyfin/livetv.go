package jellyfin

import (
	"cmp"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// liveTvViewID identifies the Live TV view, the same on every server.
var liveTvViewID, _ = accounts.ParseID(nameID("view", "livetv"))

// guideDays is how far the guide reaches, Jellyfin's default.
const guideDays = 7

// liveTvRoutes registers Jellyfin's Live TV API: the channels of the
// users' live TV catalogs and the programmes of their addons' guides.
func (h *Handler) liveTvRoutes(rt *router) {
	// Every route but the players' requires Live TV access, as Jellyfin's
	// LiveTvAccess policy does.
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(liveTvAccess(handler)))
	}
	signedIn(http.MethodGet, "/LiveTv/Info", h.liveTvInfo)
	signedIn(http.MethodGet, "/LiveTv/GuideInfo", h.guideInfo)
	signedIn(http.MethodGet, "/LiveTv/Channels", h.liveChannels)
	signedIn(http.MethodGet, "/LiveTv/Channels/{channelId}", h.liveChannel)
	signedIn(http.MethodGet, "/LiveTv/Programs", h.programs(false))
	signedIn(http.MethodPost, "/LiveTv/Programs", h.programs(false))
	signedIn(http.MethodGet, "/LiveTv/Programs/Recommended", h.programs(true))
	signedIn(http.MethodGet, "/LiveTv/Programs/{programId}", h.program)
	// The DVR: timers, series timers and recordings.
	h.recordingRoutes(rt, signedIn)
	// Players fetch these without credentials: see liveFile and
	// liveSegment.
	rt.handle(http.MethodGet, "/Videos/{itemId}/live/{file}", http.HandlerFunc(h.liveFile))
	rt.handle(http.MethodGet, "/Videos/{itemId}/hls/{playlistId}/{file}", http.HandlerFunc(h.liveSegment))
}

type LiveTvServiceInfo struct {
	Name               string
	Status             string
	HasUpdateAvailable bool
	IsVisible          bool
	Tuners             []string
}

type LiveTvInfo struct {
	Services     []LiveTvServiceInfo
	IsEnabled    bool
	EnabledUsers []string
}

// liveTvInfo describes the live TV service: the users' addons, enabled for
// the users who have live TV catalogs, as Jellyfin enables users once it
// has a tuner.
func (h *Handler) liveTvInfo(w http.ResponseWriter, r *http.Request) {
	users, err := h.Accounts.Users(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	enabled := []string{}
	for _, user := range users {
		has, err := h.Library.HasChannels(r.Context(), user)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		if has && !user.IsDisabled {
			enabled = append(enabled, user.ID.String())
		}
	}
	writeJSON(w, http.StatusOK, LiveTvInfo{
		Services:     []LiveTvServiceInfo{{Name: "Polyfin", Status: "Ok", Tuners: []string{}}},
		IsEnabled:    true,
		EnabledUsers: enabled,
	})
}

type GuideInfo struct {
	StartDate Time
	EndDate   Time
}

// guideInfo answers how far the guide reaches, as Jellyfin does whether it
// has programmes or not.
func (h *Handler) guideInfo(w http.ResponseWriter, _ *http.Request) {
	now := time.Now().UTC()
	writeJSON(w, http.StatusOK, GuideInfo{StartDate: Time(now), EndDate: Time(now.AddDate(0, 0, guideDays))})
}

// describeLive adds to a DTO what describes a channel or a programme, a
// replay included: its channel and its times.
func describeLive(dto *BaseItemDto, item library.Item, fields fieldSet, detail bool) {
	switch item.Kind {
	case library.KindChannel:
		dto.LocationType = "Remote"
		dto.Number, dto.ChannelNumber, dto.ChannelType = item.Number, item.Number, "TV"
		if detail {
			dto.ParentId = liveTvViewID.String()
		}
	case library.KindProgram, library.KindReplay:
		channel := item.Channel
		dto.ChannelId = new(channel.ID.String())
		dto.StartDate, dto.EndDate = new(Time(*item.StartDate)), new(Time(*item.EndDate))
		if detail || fields.has("ChannelInfo") {
			dto.ChannelName, dto.ChannelNumber = channel.Name, channel.Number
		}
		if detail {
			dto.ChannelPrimaryImageTag = library.ImageTag(channel.Images.Primary)
			dto.Tags = new(nonNil(item.Genres))
		}
		// Like Jellyfin's guide programmes, a programme gives the episode it
		// airs, when its guide numbers or names it.
		dto.EpisodeTitle = item.EpisodeTitle
		if item.IndexNumber > 0 {
			dto.IndexNumber = new(item.IndexNumber)
		}
		if item.ParentIndexNumber > 0 {
			dto.ParentIndexNumber = new(item.ParentIndexNumber)
		}
		flags := programFlags(item)
		for flag, field := range map[string]**bool{"movie": &dto.IsMovie, "series": &dto.IsSeries, "news": &dto.IsNews,
			"kids": &dto.IsKids, "sports": &dto.IsSports} {
			if flags[flag] {
				*field = new(true)
			}
		}
	}
}

// programFlags classifies a programme by its categories, as Jellyfin
// classifies guide programmes: movie, series, news, kids or sports.
func programFlags(item library.Item) map[string]bool {
	flags := map[string]bool{}
	for _, genre := range item.Genres {
		switch strings.ToLower(strings.TrimSpace(genre)) {
		case "movie", "movies", "film":
			flags["movie"] = true
		case "series", "show":
			flags["series"] = true
		case "news":
			flags["news"] = true
		case "kids", "children", "family":
			flags["kids"] = true
		case "sports", "sport":
			flags["sports"] = true
		}
	}
	return flags
}

// channelPlaceholder stands for a channel's streams in its details.
func channelPlaceholder(item library.Item) MediaSourceInfo {
	return MediaSourceInfo{Protocol: "Http", Id: item.ID.String(), Type: "Placeholder", Name: item.Name, IsRemote: true,
		SupportsTranscoding: true, SupportsDirectStream: true, SupportsDirectPlay: true, IsInfiniteStream: true, SupportsProbing: true,
		MediaStreams: []playback.MediaStream{}, MediaAttachments: []MediaAttachment{}, Formats: []string{},
		RequiredHttpHeaders: map[string]string{}, TranscodingSubProtocol: "http"}
}

// liveTvView describes the Live TV view, a view of Jellyfin's own.
func (h *Handler) liveTvView(r *http.Request, user accounts.User) (BaseItemDto, error) {
	view := library.Item{ID: liveTvViewID, Kind: library.KindLibrary, Name: "Live TV", CollectionType: "livetv"}
	dtos, err := h.folderDtos(r, user, []library.Item{view})
	if err != nil {
		return BaseItemDto{}, err
	}
	dtos[0].Type = "UserView"
	dtos[0].ChildCount, dtos[0].DateLastMediaAdded = new(0), nil
	return dtos[0], nil
}

// addLiveTvView adds the Live TV view to views when user has live TV
// catalogs, as Jellyfin shows it to users of a server with a tuner.
func (h *Handler) addLiveTvView(r *http.Request, user accounts.User, views []BaseItemDto) ([]BaseItemDto, error) {
	has, err := h.Library.HasChannels(r.Context(), user)
	if err != nil || !has {
		return views, err
	}
	view, err := h.liveTvView(r, user)
	if err != nil {
		return nil, err
	}
	return append(views, view), nil
}

// describeLiveTvView answers the description of the Live TV view. It
// reports whether user has it.
func (h *Handler) describeLiveTvView(w http.ResponseWriter, r *http.Request, user accounts.User) bool {
	if has, err := h.Library.HasChannels(r.Context(), user); err != nil || !has {
		return false
	}
	view, err := h.liveTvView(r, user)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	writeJSON(w, http.StatusOK, view)
	return true
}

// channelListing answers listings of channels: the Live TV view's, which
// Jellyfin leaves empty, and listings of TvChannel items across the
// server. It reports whether it answered.
func (h *Handler) channelListing(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID, hasParent bool, start, limit int) bool {
	if hasParent && parent == liveTvViewID {
		writeJSON(w, http.StatusOK, QueryResult{Items: []BaseItemDto{}, StartIndex: start})
		return true
	}
	include := listQuery(r, "includeItemTypes")
	if hasParent || !slices.ContainsFunc(include, func(t string) bool { return strings.EqualFold(t, "TvChannel") }) {
		return false
	}
	channels, err := h.Library.Channels(r.Context(), user)
	if err != nil {
		h.browseError(w, r, err)
		return true
	}
	from, to := bounds(len(channels), start, limit)
	dtos, err := h.dtos(r, user, channels[from:to], requestedFields(r), nil)
	if err == nil {
		h.addCurrentPrograms(r, user, dtos, requestedFields(r))
	}
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: dtos, TotalRecordCount: len(channels), StartIndex: start})
	return true
}

// liveChannels lists the user's channels, with the programme each airs
// now when the guide has it.
func (h *Handler) liveChannels(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, limit := b.paging(r, -1)
	favorite, favoriteSet := b.bool(r, "isFavorite")
	current, currentSet := b.bool(r, "addCurrentProgram")
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return
	}
	channels, err := h.Library.Channels(r.Context(), user)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	state, err := h.userState(r.Context(), user, channels)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if favoriteSet {
		channels = slices.DeleteFunc(channels, func(c library.Item) bool { return state.of(c).IsFavorite != favorite })
	}
	from, to := bounds(len(channels), start, limit)
	fields := requestedFields(r)
	var airing map[accounts.ID]library.Item
	if current || !currentSet {
		page := make([]accounts.ID, 0, to-from)
		for _, channel := range channels[from:to] {
			page = append(page, channel.ID)
		}
		if airing, err = h.airing(r, user, page); err != nil {
			h.browseError(w, r, err)
			return
		}
	}
	dtos := make([]BaseItemDto, 0, to-from)
	for _, channel := range channels[from:to] {
		dto := h.newItemDto(channel, fields, false, state)
		if program, ok := airing[channel.ID]; ok {
			dto.CurrentProgram = new(h.newItemDto(program, fields, false, userState{}))
		}
		dtos = append(dtos, dto)
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: dtos, TotalRecordCount: len(channels), StartIndex: start})
}

// addCurrentPrograms adds to the DTOs of channels the programme each airs
// now, as Jellyfin describes every channel.
func (h *Handler) addCurrentPrograms(r *http.Request, user accounts.User, dtos []BaseItemDto, fields fieldSet) {
	if !slices.ContainsFunc(dtos, func(dto BaseItemDto) bool { return dto.Type == "TvChannel" }) {
		return
	}
	var ids []accounts.ID
	for _, dto := range dtos {
		if id, ok := parseGUID(dto.Id); ok && dto.Type == "TvChannel" {
			ids = append(ids, id)
		}
	}
	airing, err := h.airing(r, user, ids)
	if err != nil && r.Context().Err() == nil {
		h.Logger.Warn("The programmes airing now could not be listed", "error", err)
	}
	for i := range dtos {
		if id, ok := parseGUID(dtos[i].Id); ok && dtos[i].Type == "TvChannel" {
			if program, ok := airing[id]; ok {
				dtos[i].CurrentProgram = new(h.newItemDto(program, fields, false, userState{}))
			}
		}
	}
}

// airing maps each of channels to the programme it airs now; nil asks
// for every channel.
func (h *Handler) airing(r *http.Request, user accounts.User, channels []accounts.ID) (map[accounts.ID]library.Item, error) {
	now := time.Now()
	programs, err := h.Library.Guide(r.Context(), user, library.GuideQuery{From: now, To: now.Add(time.Second), Channels: channels})
	airing := map[accounts.ID]library.Item{}
	for _, program := range programs {
		if !program.StartDate.After(now) {
			airing[program.Channel.ID] = program
		}
	}
	return airing, err
}

// liveChannel describes one of the user's channels.
func (h *Handler) liveChannel(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "channelId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	item, err := h.Library.Item(r.Context(), user, id)
	if err == nil && item.Kind != library.KindChannel {
		err = library.ErrNotFound
	}
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	state, err := h.userState(r.Context(), user, []library.Item{item})
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	dto := h.newItemDto(item, requestedFields(r), true, state)
	h.addMediaSources(r, user, &dto, item, id, true)
	if airing, err := h.airing(r, user, []accounts.ID{item.ID}); err == nil {
		if program, ok := airing[item.ID]; ok {
			dto.CurrentProgram = new(h.newItemDto(program, nil, true, userState{}))
		}
	}
	writeJSON(w, http.StatusOK, dto)
}

// programQuery is what a programme listing asks: Jellyfin's query
// parameters, or the GetProgramsDto posted.
type programQuery struct {
	UserId                                             string
	ChannelIds, SortBy, SortOrder, Fields              commaList
	Genres                                             pipeList
	MinStartDate, MaxStartDate, MinEndDate, MaxEndDate *Time
	HasAired, IsAiring                                 *bool
	IsMovie, IsSeries, IsNews, IsKids, IsSports        *bool
	EnableUserData, EnableTotalRecordCount             *bool
	StartIndex, Limit                                  *int
	// channels are the ChannelIds that are identifiers, read once.
	channels map[accounts.ID]bool
}

// commaList is a list of a posted body that, like Jellyfin's
// JsonCommaDelimitedCollectionConverter, takes an array or a string of
// comma-separated values: jellyfin-web's guide posts its channels so.
type commaList []string

func (l *commaList) UnmarshalJSON(data []byte) error {
	return unmarshalDelimited(data, ",", (*[]string)(l))
}

// pipeList is a commaList separated by "|", as Jellyfin's
// JsonPipeDelimitedCollectionConverter reads genres, whose names hold
// commas.
type pipeList []string

func (l *pipeList) UnmarshalJSON(data []byte) error {
	return unmarshalDelimited(data, "|", (*[]string)(l))
}

// unmarshalDelimited reads a JSON array of strings, or a string of values
// separated by delimiter, leaving out empty values; null leaves the list
// unchanged.
func unmarshalDelimited(data []byte, delimiter string, into *[]string) error {
	if string(data) == "null" {
		return nil
	}
	var joined string
	if json.Unmarshal(data, &joined) != nil {
		return json.Unmarshal(data, into)
	}
	values := []string{}
	for value := range strings.SplitSeq(joined, delimiter) {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	*into = values
	return nil
}

// readProgramQuery reads a programme query from the URL, or from the body
// of a POST. ok is false once w was answered.
func readProgramQuery(w http.ResponseWriter, r *http.Request) (programQuery, bool) {
	var q programQuery
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
		if err != nil || json.Unmarshal(body, &q) != nil {
			validationProblem(w, map[string][]string{"$": {"The JSON value could not be converted."}})
			return programQuery{}, false
		}
		q.readChannels()
		return q, true
	}
	b := bindErrors{}
	q.UserId = query(r, "userId")
	q.ChannelIds, q.SortBy, q.SortOrder, q.Genres = listQuery(r, "channelIds"), listQuery(r, "sortBy"), listQuery(r, "sortOrder"), listQuery(r, "genres")
	for name, field := range map[string]**Time{"minStartDate": &q.MinStartDate, "maxStartDate": &q.MaxStartDate,
		"minEndDate": &q.MinEndDate, "maxEndDate": &q.MaxEndDate} {
		if raw := query(r, name); raw != "" {
			parsed, ok := parseTime(raw)
			if !ok {
				b.add(name, notValid(raw))
				continue
			}
			*field = new(Time(parsed))
		}
	}
	for name, field := range map[string]**bool{"hasAired": &q.HasAired, "isAiring": &q.IsAiring, "isMovie": &q.IsMovie,
		"isSeries": &q.IsSeries, "isNews": &q.IsNews, "isKids": &q.IsKids, "isSports": &q.IsSports, "enableUserData": &q.EnableUserData,
		"enableTotalRecordCount": &q.EnableTotalRecordCount} {
		if value, ok := b.bool(r, name); ok {
			*field = new(value)
		}
	}
	if value, ok := b.int32(r, "startIndex"); ok {
		q.StartIndex = new(value)
	}
	if value, ok := b.int32(r, "limit"); ok {
		q.Limit = new(value)
	}
	if len(b) > 0 {
		validationProblem(w, b)
		return programQuery{}, false
	}
	q.readChannels()
	return q, true
}

// readChannels parses the channels asked once, rather than for each
// programme: the guide asks a page of channels at a time, hundreds.
func (q *programQuery) readChannels() {
	q.channels = make(map[accounts.ID]bool, len(q.ChannelIds))
	for _, raw := range q.ChannelIds {
		if id, ok := parseGUID(raw); ok {
			q.channels[id] = true
		}
	}
}

// programs lists programmes of the user's guides that match the query,
// by start time; recommended lists, like Jellyfin, those airing or about
// to, by channel. Without a guide, the lists are empty, as Jellyfin's.
func (h *Handler) programs(recommended bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, ok := readProgramQuery(w, r)
		if !ok {
			return
		}
		b := bindErrors{}
		id, set := accounts.ID{}, false
		if q.UserId != "" {
			if id, set = parseGUID(q.UserId); !set {
				b.add("userId", notValid(q.UserId))
			}
		}
		if len(b) > 0 {
			validationProblem(w, b)
			return
		}
		user, ok := h.targetUser(w, r, id, set, unknownListingUser)
		if !ok {
			return
		}
		now := time.Now()
		from, to := now, now.AddDate(0, 0, guideDays)
		switch {
		case q.MinEndDate != nil:
			from = time.Time(*q.MinEndDate)
		case q.MinStartDate != nil:
			from = time.Time(*q.MinStartDate)
		case q.HasAired != nil && *q.HasAired:
			from = now.AddDate(0, 0, -1)
		}
		if q.MaxStartDate != nil {
			to = time.Time(*q.MaxStartDate).Add(time.Nanosecond)
		}
		// What airs now, or has aired, needs no guide of the days ahead.
		switch {
		case q.IsAiring != nil && *q.IsAiring:
			from, to = now, now.Add(time.Second)
		case q.HasAired != nil && *q.HasAired && to.After(now):
			to = now
		}
		start, limit := 0, -1
		if q.StartIndex != nil {
			start = *q.StartIndex
		}
		if q.Limit != nil && *q.Limit >= 0 {
			limit = *q.Limit
		}
		// The guide is read for the channels asked only, and, when the
		// programmes are listed by start time with no total asked, no
		// further than the page.
		airing := q.IsAiring != nil && *q.IsAiring
		byName := slices.ContainsFunc(q.SortBy, func(s string) bool { return strings.EqualFold(s, "SortName") })
		descending := slices.ContainsFunc(q.SortOrder, func(s string) bool { return strings.EqualFold(s, "Descending") })
		guide := library.GuideQuery{From: from, To: to, Keep: func(p library.Item) bool { return q.keeps(p, now) }}
		if len(q.ChannelIds) > 0 {
			guide.Channels = slices.Collect(maps.Keys(q.channels))
		}
		counted := q.EnableTotalRecordCount == nil || *q.EnableTotalRecordCount
		if limit >= 0 && !counted && !byName && !descending && !(recommended && airing) {
			guide.Limit = max(start, 0) + limit
		}
		programs, err := h.Library.Guide(r.Context(), user, guide)
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		// Recommended programmes airing now are listed by channel, those to
		// come by start time.
		if recommended && airing {
			slices.SortStableFunc(programs, func(a, b library.Item) int {
				x, _ := strconv.Atoi(a.Channel.Number)
				y, _ := strconv.Atoi(b.Channel.Number)
				return x - y
			})
		} else if byName {
			slices.SortStableFunc(programs, func(a, b library.Item) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
		}
		if descending {
			slices.Reverse(programs)
		}
		fields := requestedFields(r)
		for _, field := range q.Fields {
			fields[strings.ToLower(field)] = true
		}
		from2, to2 := bounds(len(programs), start, limit)
		dtos := make([]BaseItemDto, 0, to2-from2)
		state, err := h.userState(r.Context(), user, programs[from2:to2])
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		for _, program := range programs[from2:to2] {
			dto := h.newItemDto(program, fields, false, state)
			if q.EnableUserData != nil && !*q.EnableUserData {
				dto.UserData = UserItemData{}
			}
			dtos = append(dtos, dto)
		}
		h.addTimers(r.Context(), dtos)
		writeJSON(w, http.StatusOK, QueryResult{Items: dtos, TotalRecordCount: len(programs), StartIndex: start})
	}
}

// keeps reports whether a programme matches the query's filters.
func (q programQuery) keeps(p library.Item, now time.Time) bool {
	start, end := *p.StartDate, *p.EndDate
	if q.MinStartDate != nil && start.Before(time.Time(*q.MinStartDate)) || q.MaxStartDate != nil && start.After(time.Time(*q.MaxStartDate)) ||
		q.MinEndDate != nil && end.Before(time.Time(*q.MinEndDate)) || q.MaxEndDate != nil && end.After(time.Time(*q.MaxEndDate)) {
		return false
	}
	if q.HasAired != nil && *q.HasAired != !end.After(now) {
		return false
	}
	if q.IsAiring != nil && *q.IsAiring != (!start.After(now) && end.After(now)) {
		return false
	}
	if len(q.ChannelIds) > 0 && !q.channels[p.Channel.ID] {
		return false
	}
	if len(q.Genres) > 0 && !slices.ContainsFunc(p.Genres, func(g string) bool {
		return slices.ContainsFunc(q.Genres, func(want string) bool { return strings.EqualFold(g, want) })
	}) {
		return false
	}
	flags := programFlags(p)
	for flag, want := range map[string]*bool{"movie": q.IsMovie, "series": q.IsSeries, "news": q.IsNews, "kids": q.IsKids, "sports": q.IsSports} {
		if want != nil && *want != flags[flag] {
			return false
		}
	}
	return true
}

// program describes a programme of the user's guides.
func (h *Handler) program(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "programId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	item, err := h.Library.Item(r.Context(), user, id)
	if err == nil && item.Kind != library.KindProgram {
		err = library.ErrNotFound
	}
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	state, err := h.userState(r.Context(), user, []library.Item{item})
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	dtos := []BaseItemDto{h.newItemDto(item, requestedFields(r), true, state)}
	h.addTimers(r.Context(), dtos)
	writeJSON(w, http.StatusOK, dtos[0])
}

// webApp reports whether the app is jellyfin-web, which plays HLS in the
// browser: a browser reads another site's playlist only with that site's
// consent, which live sources rarely give, so their playlists are relayed
// to it.
func webApp(r *http.Request) bool {
	return strings.EqualFold(callerFrom(r.Context()).Device.Client, "Jellyfin Web")
}

// livePlaybackInfo answers PlaybackInfo for a channel: its stream as a
// live media source, played as it is when the app takes it, else
// converted into HLS by FFmpeg as it comes. Unlike Jellyfin's tuners, a
// channel's source needs no opening (RequiresOpening is false): Polyfin
// keeps nothing open for it until a player asks for its stream.
func (h *Handler) livePlaybackInfo(w http.ResponseWriter, r *http.Request, user accounts.User, channel library.Item, opened accounts.ID, request playbackInfoRequest) {
	versions, err := h.Library.Versions(r.Context(), user, channel.ID)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	versions = h.inGroup(r.Context(), user, slices.DeleteFunc(versions, func(v library.Version) bool { return h.Playback.Failed(v.ID) }))
	requested, asked := parseGUID(request.MediaSourceId)
	allowed := h.Accounts.Conversions(user)
	attempts, tries := 0, h.Accounts.Settings().VersionAttempts
	at := openedIndex(opened, versions)
	for i, version := range versions {
		id := sourceID(opened, version, i == at)
		if asked && id != requested || attempts >= tries {
			continue
		}
		attempts++
		// A stream that answers no live stream fails within seconds: the
		// next one is tried at once.
		analysis, err := h.Playback.AnalyzeLive(playback.ForUser(r.Context(), user.ID), version)
		if err != nil {
			h.Logger.Info("A channel's stream could not be analyzed", "addon", version.Addon, "failure", playback.LiveFailure(err), "error", err)
			continue
		}
		manifest := playback.Manifest(analysis)
		relay := mustRelay(r, version) || manifest && (webApp(r) || !h.Playback.Redirectable(r.Context(), version))
		session := h.Playback.Signer().Sign(playback.Grant{Version: version.ID, User: user.ID, Relay: relay})
		permits := h.convertible(allowed, user, version)
		source, ok := h.liveSource(r, channel, version, id, analysis, request, session, relay, permits)
		if !ok {
			if permits != allowed {
				h.Logger.Info("A channel's stream would need its video converted while the server converts as many as it may", "addon", version.Addon)
			} else {
				h.Logger.Info("A channel's stream would need a conversion the user may not have, or is above their bitrate limit or quality group", "addon", version.Addon)
			}
			continue
		}
		writeJSON(w, http.StatusOK, playbackInfoResponse{MediaSources: []MediaSourceInfo{source}, PlaySessionId: session})
		return
	}
	writeJSON(w, http.StatusOK, noCompatibleStream{MediaSources: []MediaSourceInfo{}, ErrorCode: "NoCompatibleStream"})
}

// liveSource describes a channel's stream with the decision for the app's
// device profile. Subtitles in live streams are not offered. Only the
// conversions allowed are planned: it reports false when the stream would
// need another to play on the app.
func (h *Handler) liveSource(r *http.Request, channel library.Item, version library.Version, id accounts.ID, analysis media.Analysis,
	request playbackInfoRequest, session string, relay bool, allowed accounts.Conversions) (MediaSourceInfo, bool) {
	streams := slices.DeleteFunc(playback.MediaStreams(analysis, nil, h.Accounts.Settings().Language),
		func(s playback.MediaStream) bool { return s.Type == "Subtitle" })
	// Jellyfin 12.2 reports no codec tag of a live stream: MPEG-TS tags
	// are zero bytes, and HLS ones it does not read.
	for i := range streams {
		streams[i].CodecTag = ""
	}
	container := playback.Container(analysis)
	source := MediaSourceInfo{
		Protocol: "Http", Id: id.String(), Type: "Default", Container: container, Name: version.Name, IsRemote: true,
		ETag: version.ID.String(), IsInfiniteStream: true, SupportsProbing: true, VideoType: "VideoFile",
		MediaStreams: streams, MediaAttachments: []MediaAttachment{}, Formats: []string{},
		RequiredHttpHeaders: map[string]string{}, TranscodingSubProtocol: "http",
	}
	if analysis.Bitrate > 0 {
		source.Bitrate = new(analysis.Bitrate)
	}
	source.Path = h.streamURL(r, channel.ID, id, version, container, relay)
	options := playback.Options{
		MaxStreamingBitrate: request.MaxStreamingBitrate.value,
		SubtitleStreamIndex: new(-1),
		EnableDirectPlay:    request.EnableDirectPlay == nil || *request.EnableDirectPlay,
		EnableDirectStream:  request.EnableDirectStream == nil || *request.EnableDirectStream,
		ConvertAudio:        request.AllowAudioStreamCopy != nil && !*request.AllowAudioStreamCopy,
		ConvertVideo:        request.AllowVideoStreamCopy != nil && !*request.AllowVideoStreamCopy,
		MaxHeight:           request.group,
		ConversionHeight:    request.conversionHeight,
		Can:                 h.Playback.Capabilities(),
	}
	decision := playback.Decision{DirectPlay: true, Container: container, AudioStreamIndex: -1, SubtitleStreamIndex: -1}
	if request.DeviceProfile != nil {
		decision = playback.Decide(request.DeviceProfile, playback.MediaSource{Container: container, Bitrate: analysis.Bitrate, Streams: streams}, options)
	}
	if decision.HLS && !permitted(allowed, decision) {
		return MediaSourceInfo{}, false
	}
	// Above the user's bitrate limit, or taller than their quality group, a
	// stream plays only converted down to it.
	if beyondUserLimit(request, analysis.Bitrate, decision) || aboveGroup(request, analysis, decision) {
		return MediaSourceInfo{}, false
	}
	source.Container = cmp.Or(decision.Container, container)
	source.SupportsDirectPlay, source.SupportsDirectStream = decision.DirectPlay, decision.DirectPlay
	if _, video := playback.LiveVideo(analysis); decision.HLS && video {
		limit := request.MaxStreamingBitrate.value
		if limit <= 0 && request.DeviceProfile.MaxStreamingBitrate != nil {
			limit = *request.DeviceProfile.MaxStreamingBitrate
		}
		source.SupportsTranscoding = true
		source.TranscodingUrl = transcodingURL(r, channel.ID, id, version, analysis, streams, decision, limit, session)
		source.TranscodingSubProtocol = "hls"
		source.TranscodingContainer = decision.Transcoding.Container
		describeConverted(request, analysis, source.MediaStreams, decision.Video)
	}
	if decision.AudioStreamIndex >= 0 {
		source.DefaultAudioStreamIndex = new(decision.AudioStreamIndex)
	}
	return source, true
}

// serveChannel serves a channel's stream to a player that plays it as it
// is: an HLS playlist is relayed, its files named through Polyfin, unless
// the player may be sent to the source; an MPEG-TS stream is relayed from
// its shared feed, or redirected to when that costs no connection (see
// playback.Service.ServeChannel).
func (h *Handler) serveChannel(w http.ResponseWriter, r *http.Request, user accounts.User, channel library.Item, version library.Version, relay bool) {
	r = r.WithContext(playback.ForUser(r.Context(), user.ID))
	analysis, err := h.Playback.AnalyzeLive(r.Context(), version)
	if err != nil {
		http.Error(w, "source unavailable", http.StatusBadGateway)
		return
	}
	// PlaybackInfo never offers a stream taller than the user's quality
	// group as it is: this guards URLs kept, or made up.
	if !user.FitsGroup(videoHeight(analysis)) {
		h.Logger.Info("A channel's stream taller than the user's quality group was refused", "addon", version.Addon)
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if !playback.Manifest(analysis) {
		err = h.Playback.ServeChannel(w, r, version, relay)
	} else if relay {
		grant := h.Playback.Signer().Sign(playback.Grant{Version: version.ID, User: user.ID, Relay: true})
		err = h.Playback.ServeLive(w, r, version, "", h.liveLink(r, channel.ID, grant))
	} else {
		err = h.Playback.Serve(w, r, version, playback.Delivery{})
	}
	if err != nil && r.Context().Err() == nil {
		h.Logger.Warn("A channel's stream could not be served", "addon", version.Addon, "error", err)
	}
}

// liveLink names the files a relayed playlist refers to: Polyfin's
// addresses, signed for the grant, so that they relay nothing else.
func (h *Handler) liveLink(r *http.Request, channel accounts.ID, grant string) func(string) string {
	base := baseURL(r) + "/Videos/" + channel.String() + "/live/"
	return func(target string) string {
		q := url.Values{grantParameter: {grant}, "target": {base64.RawURLEncoding.EncodeToString([]byte(target))},
			"sign": {h.Playback.Signer().Link(grant, target)}}
		return base + url.PathEscape(playback.FileName(target)) + "?" + q.Encode()
	}
}

// liveFile relays a file a relayed playlist names, for the grant it was
// signed for.
func (h *Handler) liveFile(w http.ResponseWriter, r *http.Request) {
	token := query(r, grantParameter)
	grant, err := h.Playback.Signer().Verify(token)
	target, decodeErr := base64.RawURLEncoding.DecodeString(query(r, "target"))
	if err != nil || decodeErr != nil || !h.Playback.Signer().VerifyLink(token, string(target), query(r, "sign")) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	opened, ok := parseGUID(r.PathValue("itemId"))
	user, userErr := h.Accounts.User(r.Context(), grant.User)
	if !ok || userErr != nil || user.IsDisabled || h.outsideHours(user) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	channel, err := h.played(r.Context(), user, opened)
	if err != nil || channel.Kind != library.KindChannel {
		processingError(w, http.StatusNotFound)
		return
	}
	version, err := h.Library.Version(r.Context(), user, channel.ID, grant.Version)
	if err != nil {
		processingError(w, http.StatusNotFound)
		return
	}
	if err := h.Playback.ServeLive(w, r, version, string(target), h.liveLink(r, channel.ID, token)); err != nil && r.Context().Err() == nil {
		h.Logger.Debug("A file of a channel's stream could not be relayed", "addon", version.Addon, "error", err)
	}
}

// livePlaylist serves the playlists of a channel converted by FFmpeg: the
// master playlist names live.m3u8, as Jellyfin's does for live TV, which
// lists the segments under hls/live.
func (h *Handler) livePlaylist(w http.ResponseWriter, r *http.Request, remux playback.Remux, name string) {
	var data []byte
	var err error
	if strings.EqualFold(name, "master") {
		var variant hls.Variant
		if variant, err = h.Playback.LiveVariant(r.Context(), remux); err == nil {
			var b strings.Builder
			err = hls.WriteMaster(&b, variant, "live.m3u8?"+r.URL.RawQuery)
			data = []byte(b.String())
		}
	} else {
		data, err = h.Playback.LivePlaylist(r.Context(), remux, func(file string) string {
			return "hls/live/" + file + "?" + r.URL.RawQuery
		})
	}
	if err != nil {
		h.remuxError(w, r, remux, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

// liveSegment serves a segment of a channel converted by FFmpeg.
func (h *Handler) liveSegment(w http.ResponseWriter, r *http.Request) {
	req, ok := h.remuxOf(w, r)
	if !ok {
		return
	}
	if !req.live || !strings.EqualFold(r.PathValue("playlistId"), "live") {
		processingError(w, http.StatusNotFound)
		return
	}
	file, err := h.Playback.LiveSegment(req.remux, r.PathValue("file"))
	if errors.Is(err, hls.ErrNotFound) || errors.Is(err, hls.ErrStopped) {
		processingError(w, http.StatusNotFound)
		return
	}
	if err != nil {
		h.remuxError(w, r, req.remux, err)
		return
	}
	defer file.Close()
	contentType := "video/mp2t"
	if strings.HasSuffix(file.Name(), ".mp4") {
		contentType = "video/mp4"
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, "", time.Time{}, file)
}
