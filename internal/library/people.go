package library

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// PersonID identifies someone credited in titles by their name, the way
// titles' credits do.
func PersonID(name string) accounts.ID { return itemID(personKey(name)) }

// mergedPerson is the record a person's upsert stores: the new one, with the
// credits of both, and the stored photo when the new one has none.
const mergedPerson = `excluded.data
	|| jsonb_build_object('credits', COALESCE(items.data->'credits', '{}'::jsonb) || COALESCE(excluded.data->'credits', '{}'::jsonb))
	|| CASE WHEN excluded.data->'person' ? 'image' OR NOT COALESCE(items.data->'person' ? 'image', false) THEN '{}'::jsonb
		ELSE jsonb_build_object('person', items.data->'person', 'confined', COALESCE(items.data->'confined', 'false'::jsonb)) END`

// saveCredits records the people a title credits, so that apps can open
// them by identifier, and so that a person lists the titles Polyfin knows
// them in. Credits accumulate across titles; the merge happens in the
// upsert, so that titles described at once do not lose each other's.
func (s *Service) saveCredits(ctx context.Context, title accounts.ID, credits []Person, confined bool) error {
	if len(credits) == 0 {
		return nil
	}
	byID := map[accounts.ID]*record{}
	var people []*record
	for _, person := range credits {
		r, ok := byID[person.ID]
		if !ok {
			r = &record{ID: person.ID, Key: personKey(person.Name), Kind: KindPerson, Person: &Person{Name: person.Name},
				Credits: map[string][]string{}, Confined: confined}
			byID[person.ID] = r
			people = append(people, r)
		}
		if r.Person.Image == "" {
			r.Person.Image = person.Image
		}
		if types := r.Credits[title.String()]; !slices.Contains(types, person.Type) {
			r.Credits[title.String()] = append(types, person.Type)
		}
	}
	ids := make([]accounts.ID, 0, len(people))
	keys := make([]string, 0, len(people))
	kinds := make([]string, 0, len(people))
	data := make([]string, 0, len(people))
	for _, r := range people {
		encoded, err := json.Marshal(r)
		if err != nil {
			return err
		}
		ids, keys, kinds, data = append(ids, r.ID), append(keys, r.Key), append(kinds, string(r.Kind)), append(data, string(encoded))
	}
	_, err := s.db.Exec(ctx, `INSERT INTO items (id, key, kind, data)
		SELECT * FROM unnest($1::uuid[], $2::text[], $3::text[], $4::jsonb[])
		ON CONFLICT (id) DO UPDATE SET data = `+mergedPerson+`, updated_at = now()
		WHERE items.data IS DISTINCT FROM `+mergedPerson, ids, keys, kinds, data)
	return err
}

// searchable reports whether a catalog looks a term up: it takes a search
// property, and any other property it requires offers options.
func searchable(catalog stremio.Catalog) bool {
	search := false
	for _, extra := range catalog.Extra {
		if extra.Name == "search" {
			search = true
		} else if extra.IsRequired && len(extra.Options) == 0 {
			return false
		}
	}
	return search
}

// searchesPeople reports whether a catalog looks titles up by the name of
// someone credited in them. The Stremio protocol has no word for this:
// metadata addons name such catalogs after people, as AIOMetadata's
// "People Search" catalogs (people_search.people_search_movie and
// people_search.people_search_series).
func searchesPeople(catalog stremio.Catalog) bool {
	if !searchable(catalog) {
		return false
	}
	for _, text := range []string{catalog.ID, catalog.Name} {
		text = strings.ToLower(text)
		if strings.Contains(text, "people") || strings.Contains(text, "person") {
			return true
		}
	}
	return false
}

// PersonTitles lists titles of the given kinds a person is credited in:
// first those the people-search catalogs of the user's addons find for the
// person's name, at least count of them when the catalogs have that many,
// then the other titles Polyfin knows the person in. A people search can
// miss a title, or find someone else of the same name: the known titles
// keep the credits the user saw. Page.More tells that the catalogs have
// more titles. A person whose people search fails still lists their known
// titles.
func (s *Service) PersonTitles(ctx context.Context, user accounts.User, person accounts.ID, kinds []Kind, count int) (Page, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return Page{}, err
	}
	r, err := s.load(ctx, person)
	if err != nil {
		return Page{}, err
	}
	if r.Kind != KindPerson || r.Person == nil {
		return Page{}, ErrNotFound
	}
	var sources []source
	for _, entry := range v.addons {
		for _, catalog := range entry.addon.Manifest.Catalogs {
			if kind, ok := titleKind(catalog.Type); ok && slices.Contains(kinds, kind) && searchesPeople(catalog) {
				sources = append(sources, source{addon: entry, catalog: catalog, search: r.Person.Name})
			}
		}
	}
	var items []Item
	var records []record
	seen := map[accounts.ID]bool{}
	more := false
	if len(sources) > 0 && count > 0 {
		metas, total, err := s.merged(ctx, sources, 0, count)
		if ctx.Err() != nil {
			return Page{}, ctx.Err()
		}
		if err != nil {
			s.logger.Warn("A people search failed", "catalog", sources[0].catalog.ID, "error", err)
		}
		more = total > len(metas)
		for _, meta := range metas {
			src := sources[0]
			for _, candidate := range sources {
				if candidate.catalog.Type == meta.Type {
					src = candidate
					break
				}
			}
			item, rec, err := titleItem(src.addon.addon.ID, src.catalog, meta, accounts.ID{}, src.addon.confined)
			if err != nil || seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			// Like a search result, the title keeps the folder it was last
			// listed in.
			rec.Parent = nil
			items, records = append(items, item), append(records, rec)
		}
	}
	known, err := s.creditedTitles(ctx, r, kinds)
	if err != nil {
		return Page{}, err
	}
	for _, item := range known {
		if !seen[item.ID] {
			items = append(items, item)
		}
	}
	total := len(items)
	if more {
		total++
	}
	return Page{Items: items, Total: total, More: more}, s.save(ctx, records)
}

// creditedTitles describes the titles of the given kinds Polyfin knows a
// person in, by name, from what was stored when they were listed, with the
// complete description when it is still cached: describing each title
// anew would cost a request per title.
func (s *Service) creditedTitles(ctx context.Context, person record, kinds []Kind) ([]Item, error) {
	ids := make([]accounts.ID, 0, len(person.Credits))
	for raw := range person.Credits {
		if id, err := accounts.ParseID(raw); err == nil {
			ids = append(ids, id)
		}
	}
	records, err := s.loadAll(ctx, ids)
	if err != nil {
		return nil, err
	}
	var items []Item
	for _, r := range records {
		if r.Meta == nil || !slices.Contains(kinds, r.Kind) {
			continue
		}
		meta := *r.Meta
		if full, ok := s.cachedMeta(r); ok {
			meta = full
		}
		item := Item{ID: r.ID, Kind: r.Kind, ParentID: deref(r.Parent), Available: true}
		fromMeta(&item, meta)
		items = append(items, item)
	}
	slices.SortFunc(items, func(a, b Item) int { return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return items, nil
}

// PeopleQuery narrows the people Polyfin knows. Names compare without
// regard to case.
type PeopleQuery struct {
	// NameContains, NameStartsWith keep names containing, or starting
	// with, the text; NameBefore keeps names sorting before it, NameFrom
	// names sorting at or after it. Empty ones keep every name.
	NameContains, NameStartsWith, NameBefore, NameFrom string
	// A person is kept when one of their credits ("Actor", "Director",
	// "Writer") is one of Types, if set, and none of ExcludedTypes.
	Types, ExcludedTypes []string
	// Restricted keeps only the people of Only; Excluded leaves people out.
	Restricted     bool
	Only, Excluded []accounts.ID
	// Start skips people; a negative Limit lists all the others.
	Start, Limit int
}

// peopleFilter selects the people matching $1 to $9 (see People).
const peopleFilter = `FROM items WHERE kind = 'person'
	AND ($1 = '' OR strpos(lower(data->'person'->>'name'), lower($1)) > 0)
	AND ($2 = '' OR starts_with(lower(data->'person'->>'name'), lower($2)))
	AND ($3 = '' OR lower(data->'person'->>'name') < lower($3))
	AND ($4 = '' OR lower(data->'person'->>'name') >= lower($4))
	AND (cardinality($5::text[]) = 0 AND cardinality($6::text[]) = 0 OR EXISTS (
		SELECT 1 FROM jsonb_each(data->'credits') AS credit, jsonb_array_elements_text(credit.value) AS credited(kind)
		WHERE (cardinality($5::text[]) = 0 OR lower(credited.kind) = ANY($5::text[])) AND NOT lower(credited.kind) = ANY($6::text[])))
	AND (NOT $7 OR id = ANY($8::uuid[]))
	AND NOT id = ANY($9::uuid[])`

// People lists the people Polyfin knows from the titles it described, by
// name, and how many match the query. Like Jellyfin's, people belong to no
// library: anyone credited in a title some user opened is known.
func (s *Service) People(ctx context.Context, q PeopleQuery) ([]Item, int, error) {
	lower := func(values []string) []string {
		result := make([]string, 0, len(values))
		for _, value := range values {
			result = append(result, strings.ToLower(value))
		}
		return result
	}
	ids := func(values []accounts.ID) []accounts.ID {
		if values == nil {
			return []accounts.ID{}
		}
		return values
	}
	args := []any{q.NameContains, q.NameStartsWith, q.NameBefore, q.NameFrom, lower(q.Types), lower(q.ExcludedTypes),
		q.Restricted, ids(q.Only), ids(q.Excluded)}
	var total int
	if err := s.db.QueryRow(ctx, "SELECT count(*) "+peopleFilter, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	var limit *int
	if q.Limit >= 0 {
		limit = &q.Limit
	}
	rows, err := s.db.Query(ctx, "SELECT id, data "+peopleFilter+
		" ORDER BY lower(data->'person'->>'name'), id OFFSET $10 LIMIT $11", append(args, max(q.Start, 0), limit)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var people []Item
	for rows.Next() {
		var id accounts.ID
		var r record
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			return nil, 0, err
		}
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, 0, err
		}
		if r.Person != nil {
			people = append(people, Item{ID: id, Kind: KindPerson, Name: r.Person.Name, Images: Images{Primary: r.Person.Image}})
		}
	}
	return people, total, rows.Err()
}
