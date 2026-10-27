package silotest

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
)

// Filters: how the apps ask the values of a filter sheet, read them, and
// ask a page narrowed by the facets a user picks.

// filterLists are the lists of values the apps decode from GET
// /api/v2/catalog/filters, non-optional: Apple APIv2CatalogFilters
// (Networking/APIv2/APIv2CatalogModels.swift) and Android
// CatalogFiltersV2 (shared network/apiv2/CatalogV2Models.kt).
var filterLists = []string{"genres", "studios", "networks", "countries", "content_ratings", "original_languages", "authors",
	"narrators", "series"}

// technicalLists are those of the optional technical object: Apple
// APIv2CatalogTechnicalFilters, Android CatalogTechnicalFiltersV2.
var technicalLists = []string{"resolutions", "audio_languages", "subtitle_languages"}

// CheckCatalogFilters checks the values of a filter sheet as the apps
// offer them: each list decoded, of texts, and each value once, as the
// Apple app lists a facet's values by their value (FacetValuePicker's
// ForEach, Screens/Browse/FilterView.swift).
func CheckCatalogFilters(data []byte) error {
	var doc object
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	c := &checker{}
	values := func(o object, where string, lists []string) {
		kinds := map[string]string{}
		for _, list := range lists {
			kinds[list] = "a"
		}
		c.has(o, where, kinds)
		for _, list := range lists {
			entries, _ := o[list].([]any)
			seen := map[string]bool{}
			for n, entry := range entries {
				value, text := entry.(string)
				switch {
				case !text || value == "":
					c.fail("%s.%s[%d] is not a value: %v", where, list, n, entry)
				case seen[value]:
					c.fail("%s.%s lists %q twice", where, list, value)
				}
				seen[value] = true
			}
		}
	}
	values(doc, "filters", filterLists)
	if technical, ok := doc["technical"]; ok {
		if o, ok := technical.(object); ok {
			values(o, "filters.technical", technicalLists)
		} else {
			c.fail("filters.technical is not an object: %v", technical)
		}
	}
	return c.err()
}

// Facets are the facets a user picks in the apps' filter sheets.
type Facets struct {
	// Type is the Apple apps' Type facet of a library of movies and
	// series: movie or series.
	Type           string
	Genres         []string
	ContentRatings []string
	// Decades are the first years of the decades picked: 1990 for the
	// 1990s.
	Decades []int
	// WatchStatus is unwatched, in_progress, watched, favorited or
	// watchlist.
	WatchStatus string
	HDR         bool
	// Any asks titles that match any facet rather than all.
	Any bool
}

// watchRules are the rules of the watch statuses: Apple
// GroupAccumulator.addWatchStatus, Android buildGroups.
var watchRules = map[string][2]string{"unwatched": {"watched", "false"}, "in_progress": {"in_progress", "true"},
	"watched": {"watched", "true"}, "favorited": {"favorited", "true"}, "watchlist": {"in_watchlist", "true"}}

// groups are the rule groups of facets: one per facet, any of its values,
// the watch status alone, as the Apple apps' CatalogQueryBuilder.build
// (Screens/Browse/Filtering/CatalogQueryBuilder.swift) and the Android
// apps' CatalogFilterQueryBuilder.buildGroups (shared
// catalog/filter/CatalogFilterState.kt) build them. The Apple apps type
// their values, years as numbers and flags as booleans; the Android apps
// send each as a text (toV2Groups, shared network/api/CatalogApi.kt).
func (f Facets) groups(typed bool) []any {
	rule := func(field, op string, value any) map[string]any {
		return map[string]any{"field": field, "op": op, "value": value}
	}
	flag := func(value string) any {
		if typed {
			return value == "true"
		}
		return value
	}
	year := func(value int) any {
		if typed {
			return value
		}
		return strconv.Itoa(value)
	}
	var groups []any
	anyOf := func(field, op string, values []string) {
		if len(values) == 0 {
			return
		}
		var rules []any
		for _, value := range slices.Sorted(slices.Values(values)) {
			rules = append(rules, rule(field, op, value))
		}
		groups = append(groups, map[string]any{"match": "any", "rules": rules})
	}
	if f.Type != "" && typed {
		groups = append(groups, map[string]any{"match": "all", "rules": []any{rule("type", "is", f.Type)}})
	}
	anyOf("genre", "contains", f.Genres)
	anyOf("content_rating", "is", f.ContentRatings)
	if len(f.Decades) > 0 {
		var rules []any
		for _, start := range slices.Sorted(slices.Values(f.Decades)) {
			rules = append(rules, rule("year", "between", []any{year(start), year(start + 9)}))
		}
		groups = append(groups, map[string]any{"match": "any", "rules": rules})
	}
	if f.HDR {
		groups = append(groups, map[string]any{"match": "any", "rules": []any{rule("hdr", "is", flag("true"))}})
	}
	if watch, ok := watchRules[f.WatchStatus]; ok {
		groups = append(groups, map[string]any{"match": "all", "rules": []any{rule(watch[0], "is", flag(watch[1]))}})
	}
	return groups
}

func (f Facets) match() string {
	if f.Any {
		return "any"
	}
	return "all"
}

// AppleCatalog is the query of the page of a library (every library when
// library is empty) the Apple apps ask for facets, sorted by sort (its
// field, - first for the descending order): GET /api/v2/catalog with the
// groups as JSON with sorted keys, and match always
// (APIv2CatalogQuery.getParameters, Networking/APIv2/APIv2CatalogModels.swift).
func AppleCatalog(library string, f Facets, sort string, limit int) url.Values {
	q := url.Values{"source": {"query"}, "limit": {strconv.Itoa(limit)}, "match": {f.match()}}
	if library != "" {
		q.Set("library_id", library)
	}
	if sort != "" {
		q.Set("sort", sort)
	}
	if groups := f.groups(true); len(groups) > 0 {
		data, err := json.Marshal(groups)
		if err != nil {
			panic(fmt.Sprint("the groups of facets: ", err))
		}
		q.Set("groups", string(data))
	}
	return q
}

// AndroidCatalog is the body of the page of a library (every library
// when library is empty) the Android apps ask for facets, sorted by sort
// in order (asc or desc): POST /api/v2/catalog/query as soon as there are
// groups, with every field of the query but those without a value
// (CatalogV2Api.browse, shared network/apiv2/CatalogV2Api.kt, and
// CatalogQueryV2, encoded with its defaults). The cursor of a next page
// goes in the body too.
func AndroidCatalog(library string, f Facets, sort, order string, limit int) map[string]any {
	body := map[string]any{"source": "query", "match": f.match(), "groups": f.groups(false), "skip_total": false, "limit": limit}
	if library != "" {
		body["library_id"] = library
	}
	if sort != "" {
		body["sort"], body["order"] = sort, order
	}
	return body
}

// AndroidBrowse is the body of the page of every library narrowed to a
// genre and a content rating the Android TV app's Browse screen asks
// (TvBrowseViewModel.load, androidTvApp ui/screens/browse): each a group
// of its own, without match (CatalogApi.getCatalog, shared
// network/api/CatalogApi.kt).
func AndroidBrowse(genre, contentRating, sort, order string, limit int) map[string]any {
	var groups []any
	if genre != "" {
		groups = append(groups, map[string]any{"match": "all", "rules": []any{map[string]any{"field": "genre", "op": "contains", "value": genre}}})
	}
	if contentRating != "" {
		groups = append(groups, map[string]any{"match": "all",
			"rules": []any{map[string]any{"field": "content_rating", "op": "is", "value": contentRating}}})
	}
	body := map[string]any{"source": "query", "groups": groups, "skip_total": false, "limit": limit}
	if sort != "" {
		body["sort"], body["order"] = sort, order
	}
	return body
}

// AppleFilters is the query of the values of a library's filter sheet
// (every library's when library is empty) the Apple apps ask: GET
// /api/v2/catalog/filters with skip_technical only to leave those facets
// out (APIv2Client.catalogFilters, Networking/APIv2/APIv2Client.swift).
func AppleFilters(library string, technical bool) url.Values {
	q := url.Values{}
	if library != "" {
		q.Set("library_id", library)
	}
	if !technical {
		q.Set("skip_technical", "true")
	}
	return q
}

// AndroidFilters is the query of the values of a filter sheet the
// Android apps ask, of a library, of a list of the user (source) or of a
// collection, the facets of files always asked (CatalogV2Api.filters,
// shared network/apiv2/CatalogV2Api.kt; CatalogApi.getFilters).
func AndroidFilters(library, source, collection string) url.Values {
	q := url.Values{"skip_technical": {"false"}}
	for key, value := range map[string]string{"library_id": library, "source": source, "collection_id": collection} {
		if value != "" {
			q.Set(key, value)
		}
	}
	return q
}
