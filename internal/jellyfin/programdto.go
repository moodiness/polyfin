package jellyfin

import (
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// writeLeanPrograms answers a listing of programmes asked for without
// images and without user data, as jellyfin-web's guide asks for hundreds
// of channels a day at a time: each programme as Jellyfin 12.4 describes
// it for that request, streamed as it is encoded (see jsonStream), in
// place of the full description of other listings. page is the page of
// programmes, of total listed from start.
func (h *Handler) writeLeanPrograms(w http.ResponseWriter, r *http.Request, page []library.Item, total, start int, fields fieldSet) {
	page = h.Library.Overridden(page...)
	var marks []timerMarks
	h.markTimers(r.Context(), len(page), func(i int) (timedProgramme, bool) {
		return timedProgramme{id: page[i].ID, name: page[i].Name, channel: page[i].Channel.ID, onChannel: true}, true
	}, func(i int, m timerMarks) {
		if marks == nil {
			marks = make([]timerMarks, len(page))
		}
		marks[i] = m
	})
	asked := askedProgramFields(fields)
	s := newJSONStream(w, http.StatusOK)
	defer s.close()
	s.buf = append(s.buf, `{"Items":[`...)
	for i := range page {
		if i > 0 {
			s.buf = append(s.buf, ',')
		}
		var m timerMarks
		if marks != nil {
			m = marks[i]
		}
		s.buf = h.appendLeanProgram(s.buf, page[i], asked, m)
		if !s.spill() {
			return
		}
	}
	s.buf = append(s.buf, `],"TotalRecordCount":`...)
	s.buf = strconv.AppendInt(s.buf, int64(total), 10)
	s.buf = append(s.buf, `,"StartIndex":`...)
	s.buf = strconv.AppendInt(s.buf, int64(start), 10)
	s.buf = append(s.buf, '}')
}

// programFields are the fields a lean programme gives on request, those
// of Jellyfin's ItemFields it sets on a programme. Jellyfin's
// LiveTvManager.GetPrograms removes CanDelete, CanDownload,
// DisplayPreferencesId and Etag (RemoveFields), so they are not here.
type programFields struct {
	channelInfo, channelImage, overview, genres, people, studios, providerIDs, externalURLs, remoteTrailers, tags,
	taglines, sortName, settings, dateCreated, originalTitle, customRating, parentID, playAccess, mediaSourceDisplay,
	chapters, localTrailerCount, specialFeatureCount, aspectRatio bool
}

func askedProgramFields(fields fieldSet) programFields {
	return programFields{
		channelInfo: fields.has("ChannelInfo"), channelImage: fields.has("ChannelImage"), overview: fields.has("Overview"),
		genres: fields.has("Genres"), people: fields.has("People"), studios: fields.has("Studios"),
		providerIDs: fields.has("ProviderIds"), externalURLs: fields.has("ExternalUrls"),
		remoteTrailers: fields.has("RemoteTrailers"), tags: fields.has("Tags"), taglines: fields.has("Taglines"),
		sortName: fields.has("SortName"), settings: fields.has("Settings"), dateCreated: fields.has("DateCreated"),
		originalTitle: fields.has("OriginalTitle"), customRating: fields.has("CustomRating"),
		parentID: fields.has("ParentId"), playAccess: fields.has("PlayAccess"),
		mediaSourceDisplay: fields.has("EnableMediaSourceDisplay"), chapters: fields.has("Chapters"),
		localTrailerCount: fields.has("LocalTrailerCount"), specialFeatureCount: fields.has("SpecialFeatureCount"),
		aspectRatio: fields.has("PrimaryImageAspectRatio"),
	}
}

// appendLeanProgram appends a programme's lean description: the members
// Jellyfin 12.4 sets on a programme listed with EnableImages and
// EnableUserData false, those it leaves null being left out as its JSON
// options leave out nulls (JsonDefaults: WhenWritingNull). They come in
// BaseItemDto's order, with the values the full description gives
// (see newItemDto and describeLive).
//
// Jellyfin's DtoService.GetBaseItemDtoInternal and AttachBasicFields set,
// on any item: ServerId, Id, Name, Type, MediaType, ChannelId, EndDate,
// ImageBlurHashes (empty without images), and when known IndexNumber,
// ParentIndexNumber, OfficialRating, CriticRating, CommunityRating,
// PremiereDate, ProductionYear and RunTimeTicks; no LocationType for a
// programme, and no IsFolder, which only folders and items with media
// sources get. ImageTags only with EnableImages, BackdropImageTags only
// within the image limit, 0 without images; UserData only with
// EnableUserData. LiveTvManager.AddInfoToProgramDto adds StartDate,
// EpisodeTitle and the categories (IsMovie and the like, as `|=` on a
// null only sets them true), with ChannelInfo or ChannelImage the
// channel's name and number, with ChannelImage its image's tag, then
// AddRecordingInfo the timers (TimerId, Status, SeriesTimerId). The
// other fields come with the ItemFields that ask for them.
//
// Polyfin keeps its own value where Jellyfin's differs by origin: the
// media type is Video, as the full description says, where Jellyfin says
// the channel's with ChannelInfo and Unknown without.
func (h *Handler) appendLeanProgram(b []byte, item library.Item, f programFields, marks timerMarks) []byte {
	channel := item.Channel
	b = append(b, '{')
	b = appendJSONKey(b, "Name", true)
	b = appendJSONString(b, item.Name)
	if f.originalTitle && item.OriginalTitle != "" {
		b = appendJSONKey(b, "OriginalTitle", false)
		b = appendJSONString(b, item.OriginalTitle)
	}
	b = appendJSONKey(b, "ServerId", false)
	b = appendJSONString(b, h.ServerID)
	b = appendJSONKey(b, "Id", false)
	b = appendJSONID(b, item.ID)
	if f.dateCreated {
		b = appendJSONKey(b, "DateCreated", false)
		b = appendJSONTime(b, time.Unix(0, 0))
	}
	if f.sortName {
		if name := sortName(item); name != "" {
			b = appendJSONKey(b, "SortName", false)
			b = appendJSONString(b, name)
		}
	}
	if f.settings && item.SortName != "" {
		b = appendJSONKey(b, "ForcedSortName", false)
		b = appendJSONString(b, item.SortName)
	}
	if item.PremiereDate != nil {
		b = appendJSONKey(b, "PremiereDate", false)
		b = appendJSONTime(b, *item.PremiereDate)
	}
	if f.externalURLs {
		b = appendJSONKey(b, "ExternalUrls", false)
		b = appendJSONValue(b, externalURLs(item.ProviderIDs, item.Kind))
	}
	if f.mediaSourceDisplay {
		b = appendJSONKey(b, "EnableMediaSourceDisplay", false)
		b = append(b, "true"...)
	}
	if item.OfficialRating != "" {
		b = appendJSONKey(b, "OfficialRating", false)
		b = appendJSONString(b, item.OfficialRating)
	}
	b = appendJSONKey(b, "ChannelId", false)
	b = appendJSONID(b, channel.ID)
	withChannel := f.channelInfo || f.channelImage
	if withChannel && channel.Name != "" {
		b = appendJSONKey(b, "ChannelName", false)
		b = appendJSONString(b, channel.Name)
	}
	if f.overview && item.Overview != "" {
		b = appendJSONKey(b, "Overview", false)
		b = appendJSONString(b, item.Overview)
	}
	if f.taglines {
		b = appendJSONKey(b, "Taglines", false)
		b = appendJSONValue(b, nonNil(item.Taglines))
	}
	if f.genres {
		b = appendJSONKey(b, "Genres", false)
		b = appendJSONValue(b, nonNil(item.Genres))
	}
	if item.CommunityRating > 0 {
		b = appendJSONKey(b, "CommunityRating", false)
		b = appendJSONFloat(b, item.CommunityRating)
	}
	if item.CriticRating != nil {
		b = appendJSONKey(b, "CriticRating", false)
		b = appendJSONFloat(b, *item.CriticRating)
	}
	if f.customRating && item.CustomRating != "" {
		b = appendJSONKey(b, "CustomRating", false)
		b = appendJSONString(b, item.CustomRating)
	}
	if item.Runtime > 0 {
		b = appendJSONKey(b, "RunTimeTicks", false)
		b = strconv.AppendInt(b, int64(item.Runtime/100), 10)
	}
	if f.playAccess {
		b = appendJSONKey(b, "PlayAccess", false)
		b = append(b, `"Full"`...)
	}
	if item.ProductionYear > 0 {
		b = appendJSONKey(b, "ProductionYear", false)
		b = strconv.AppendInt(b, int64(item.ProductionYear), 10)
	}
	if item.IndexNumber > 0 {
		b = appendJSONKey(b, "IndexNumber", false)
		b = strconv.AppendInt(b, int64(item.IndexNumber), 10)
	}
	if item.ParentIndexNumber > 0 {
		b = appendJSONKey(b, "ParentIndexNumber", false)
		b = strconv.AppendInt(b, int64(item.ParentIndexNumber), 10)
	}
	if f.remoteTrailers {
		b = appendJSONKey(b, "RemoteTrailers", false)
		b = appendJSONValue(b, trailers(item.Trailers))
	}
	if f.providerIDs {
		providers := item.ProviderIDs
		if providers == nil {
			providers = map[string]string{}
		}
		b = appendJSONKey(b, "ProviderIds", false)
		b = appendJSONValue(b, providers)
	}
	if f.parentID && item.ParentID != (accounts.ID{}) {
		parent := item.ParentID
		if item.Shown != nil {
			parent = item.Shown.SeasonID
		}
		b = appendJSONKey(b, "ParentId", false)
		b = appendJSONID(b, parent)
	}
	b = appendJSONKey(b, "Type", false)
	b = append(b, `"Program"`...)
	if f.people {
		b = appendJSONKey(b, "People", false)
		b = appendJSONValue(b, h.people(item.People))
	}
	if f.studios {
		b = appendJSONKey(b, "Studios", false)
		b = appendJSONValue(b, h.namedPairs(studioNamed, item.Studios))
	}
	if f.genres {
		b = appendJSONKey(b, "GenreItems", false)
		b = appendJSONValue(b, h.namedPairs(genresOf(item.Kind), item.Genres))
	}
	if f.localTrailerCount {
		b = appendJSONKey(b, "LocalTrailerCount", false)
		b = append(b, '0')
	}
	if f.specialFeatureCount {
		b = appendJSONKey(b, "SpecialFeatureCount", false)
		b = append(b, '0')
	}
	if marks.status != "" {
		b = appendJSONKey(b, "Status", false)
		b = appendJSONString(b, marks.status)
	}
	if f.tags {
		b = appendJSONKey(b, "Tags", false)
		b = appendJSONValue(b, nonNil(item.Tags))
	}
	if f.aspectRatio {
		if ratio, ok := h.programAspectRatio(item); ok {
			b = appendJSONKey(b, "PrimaryImageAspectRatio", false)
			b = appendJSONFloat(b, ratio)
		}
	}
	b = appendJSONKey(b, "ImageBlurHashes", false)
	b = append(b, '{', '}')
	if f.chapters {
		b = appendJSONKey(b, "Chapters", false)
		b = append(b, '[', ']')
	}
	b = appendJSONKey(b, "MediaType", false)
	b = append(b, `"Video"`...)
	b = appendJSONKey(b, "EndDate", false)
	b = appendJSONTime(b, *item.EndDate)
	if f.settings {
		b = appendJSONKey(b, "LockedFields", false)
		b = append(b, '[', ']')
		b = appendJSONKey(b, "LockData", false)
		b = append(b, "false"...)
	}
	if withChannel && channel.Number != "" {
		b = appendJSONKey(b, "ChannelNumber", false)
		b = appendJSONString(b, channel.Number)
	}
	if f.channelImage && channel.Images.Primary != "" {
		b = appendJSONKey(b, "ChannelPrimaryImageTag", false)
		b = appendJSONString(b, library.ImageTag(channel.Images.Primary))
	}
	b = appendJSONKey(b, "StartDate", false)
	b = appendJSONTime(b, *item.StartDate)
	categories := programCategoriesOf(item)
	for _, category := range [...]struct {
		key string
		set bool
	}{{"IsMovie", categories.movie}, {"IsSeries", categories.series}, {"IsNews", categories.news},
		{"IsKids", categories.kids}, {"IsSports", categories.sports}, {"IsRepeat", item.Repeat}} {
		if category.set {
			b = appendJSONKey(b, category.key, false)
			b = append(b, "true"...)
		}
	}
	if item.EpisodeTitle != "" {
		b = appendJSONKey(b, "EpisodeTitle", false)
		b = appendJSONString(b, item.EpisodeTitle)
	}
	if marks.timer != "" {
		b = appendJSONKey(b, "TimerId", false)
		b = appendJSONString(b, marks.timer)
	}
	if marks.seriesTimer != "" {
		b = appendJSONKey(b, "SeriesTimerId", false)
		b = appendJSONString(b, marks.seriesTimer)
	}
	return append(b, '}')
}

// programAspectRatio is the aspect ratio of a programme's primary image,
// as setImages gives it, Jellyfin giving one only for an item with a
// primary image (DtoService.GetPrimaryImageAspectRatio).
func (h *Handler) programAspectRatio(item library.Item) (float64, bool) {
	images := h.withoutDead(item.Images)
	switch {
	case images.Primary != "":
		return 16.0 / 9.0, true
	case item.TMDBPoster != "":
		return 2.0 / 3.0, true
	}
	return 0, false
}

// appendJSONID appends an identifier as its String writes it, quoted.
func appendJSONID(b []byte, id accounts.ID) []byte {
	b = append(b, '"')
	b = hex.AppendEncode(b, id[:])
	return append(b, '"')
}
