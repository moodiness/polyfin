package jellyfin

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// People.
//
// Titles credit actors, directors and writers by name; each is a Person
// item, identified by their name (see library.PersonID), with the photo the
// metadata addon gives. A person's titles are those the people-search
// catalogs of the user's addons find for their name, plus those Polyfin
// already knows them in (see library.Service.PersonTitles).

func (h *Handler) personRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	signedIn(http.MethodGet, "/Persons", h.persons)
	signedIn(http.MethodGet, "/Persons/{name}", h.personByName)
}

// personListDto describes a person in a list, as Jellyfin does, in its key
// order.
type personListDto struct {
	Name                    string
	ServerId                string
	Id                      string
	ChannelId               *string
	Type                    string
	UserData                UserItemData
	PrimaryImageAspectRatio *float64 `json:",omitempty"`
	ImageTags               map[string]string
	BackdropImageTags       []string
	ImageBlurHashes         map[string]map[string]string
	LocationType            string
	MediaType               string
}

// persons lists the people credited in the titles the user reaches, by
// name, narrowed as Jellyfin narrows them. Jellyfin also narrows them to a
// library with parentId; a person is not listed by library in Polyfin, so
// parentId is ignored.
func (h *Handler) persons(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, limit := b.paging(r, -1)
	favorite, favoriteSet := b.bool(r, "isFavorite")
	appearsIn, appears := b.guid(r, "appearsInItemId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	if !favoriteSet && slices.ContainsFunc(listQuery(r, "filters"), func(f string) bool { return strings.EqualFold(f, "IsFavorite") }) {
		favorite, favoriteSet = true, true
	}
	q := library.PeopleQuery{
		NameContains: strings.TrimSpace(query(r, "searchTerm")), NameStartsWith: query(r, "nameStartsWith"),
		NameBefore: query(r, "nameLessThan"), NameFrom: query(r, "nameStartsWithOrGreater"),
		Types: listQuery(r, "personTypes"), ExcludedTypes: listQuery(r, "excludePersonTypes"),
		Start: max(start, 0), Limit: limit,
	}
	if favoriteSet {
		entries, err := h.UserData.Favorites(r.Context(), user.ID)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		if favorite {
			q.Restricted, q.Only = true, entryIDs(entries)
		} else {
			q.Excluded = entryIDs(entries)
		}
	}
	if appears {
		title, err := h.Library.Item(r.Context(), user, appearsIn)
		if err != nil && !errors.Is(err, library.ErrNotFound) {
			h.browseError(w, r, err)
			return
		}
		var credited []accounts.ID
		for _, person := range title.People {
			if !q.Restricted || slices.Contains(q.Only, person.ID) {
				credited = append(credited, person.ID)
			}
		}
		q.Restricted, q.Only = true, credited
	}
	people, total, err := h.Library.People(r.Context(), user, q)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	state, err := h.userState(r.Context(), user, people)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	fields := requestedFields(r)
	items := make([]personListDto, 0, len(people))
	for _, person := range people {
		dto := personListDto{
			Name: person.Name, ServerId: h.ServerID, Id: person.ID.String(), Type: "Person", UserData: state.of(person),
			ImageTags: map[string]string{}, BackdropImageTags: []string{}, ImageBlurHashes: map[string]map[string]string{},
			LocationType: "FileSystem", MediaType: "Unknown",
		}
		if person.Images.Primary != "" {
			dto.ImageTags["Primary"] = library.ImageTag(person.Images.Primary)
			if fields.has("PrimaryImageAspectRatio") {
				dto.PrimaryImageAspectRatio = new(2.0 / 3.0)
			}
		}
		items = append(items, dto)
	}
	writeJSON(w, http.StatusOK, struct {
		Items            []personListDto
		TotalRecordCount int
		StartIndex       int
	}{items, total, start})
}

// personByName describes the person of a name, as titles credit them.
func (h *Handler) personByName(w http.ResponseWriter, r *http.Request) {
	user, ok := h.viewer(w, r, bindErrors{}, notFoundProblem)
	if !ok {
		return
	}
	person, err := h.Library.Item(r.Context(), user, library.PersonID(r.PathValue("name")))
	if err == nil && person.Kind != library.KindPerson {
		err = library.ErrNotFound
	}
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	h.writePerson(w, r, user, person)
}

// writePerson answers a person's own description. Jellyfin describes people
// as it does genres and studios, with the number of titles of each kind
// they are credited in; jellyfin-web only shows the sections of a person's
// page whose count is not zero. As on genre pages, the counts are those of
// a first page of titles, plus one when more follow, and the listings that
// follow read the same catalog pages again from cache. Polyfin knows no
// biography, birth date or birthplace (Overview, PremiereDate,
// ProductionLocations).
func (h *Handler) writePerson(w http.ResponseWriter, r *http.Request, user accounts.User, person library.Item) {
	dto := h.namedItem(namedPage{itemType: "Person", name: person.Name, id: person.ID.String(), sortName: strings.ToLower(person.Name)})
	if person.Images.Primary != "" {
		dto.ImageTags["Primary"] = library.ImageTag(person.Images.Primary)
		dto.PrimaryImageAspectRatio = new(2.0 / 3.0)
	}
	for _, count := range []struct {
		kind  library.Kind
		total *int
	}{{library.KindMovie, &dto.MovieCount}, {library.KindSeries, &dto.SeriesCount}} {
		titles, err := h.Library.PersonTitles(r.Context(), user, person.ID, []library.Kind{count.kind}, defaultPageSize)
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		*count.total = titles.Total
	}
	state, err := h.userState(r.Context(), user, []library.Item{person})
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	dto.PlayAccess = "Full"
	dto.UserData = new(state.of(person))
	writeJSON(w, http.StatusOK, dto)
}

// personListing lists the titles of people (personIds), as apps do on a
// person's page, narrowed by type, excluded items (excludeItemIds) and what
// the user did with them, and sorted as asked. At least a first page of
// titles is read whatever the page asked, the one the person's own page
// counted, so that pages are cut from the same sorted titles; past what was
// read, the order is the catalogs'.
func (h *Handler) personListing(w http.ResponseWriter, r *http.Request, user accounts.User, people []accounts.ID, start, limit int) {
	keep := itemTypeFilter(r)
	var excluded []accounts.ID
	for _, raw := range listQuery(r, "excludeItemIds") {
		if id, ok := parseGUID(raw); ok {
			excluded = append(excluded, id)
		}
	}
	var items []library.Item
	seen := map[accounts.ID]bool{}
	more := false
	for _, person := range people {
		page, err := h.Library.PersonTitles(r.Context(), user, person, searchKinds(keep), max(max(start, 0)+limit, defaultPageSize))
		if errors.Is(err, library.ErrNotFound) {
			// Like Jellyfin, someone unknown is credited in nothing.
			continue
		}
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		more = more || page.More
		for _, item := range page.Items {
			if !seen[item.ID] && keep(item) && !slices.Contains(excluded, item.ID) {
				seen[item.ID] = true
				items = append(items, item)
			}
		}
	}
	state, err := h.userState(r.Context(), user, items)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if f, ok := stateFilterOf(r); ok {
		items = slices.DeleteFunc(items, func(item library.Item) bool { return !f.keeps(item, state) })
	}
	sortItems(r, items, state)
	total := len(items)
	if more {
		total++
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: h.listDtos(r, user, window(items, start, limit), requestedFields(r), state),
		TotalRecordCount: total, StartIndex: start})
}
