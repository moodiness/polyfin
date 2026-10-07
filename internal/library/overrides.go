package library

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Administrators' edits.
//
// Jellyfin apps let administrators edit an item's metadata and upload its
// artwork. Polyfin keeps what they change apart from what addons say: the
// fields set (Overrides) and the images uploaded, by item. They win over
// the addons' values for every user, wherever the item shows (see
// overridden), and parental control judges titles by the ratings and
// genres set (see overriddenTraits). A field cleared, or set back to the
// addon's value, follows the addon again. People and provider identifiers
// are the addons' only.

// UploadedImageTypes are the image types an administrator may upload for
// an item.
var UploadedImageTypes = []string{"Primary", "Backdrop", "Logo", "Thumb", "Banner"}

const (
	// MaxUploadedImageBytes bounds uploaded artwork, as sent and as kept.
	MaxUploadedImageBytes = 10 << 20
	// MaxUploadedImageSide is the longest side uploaded artwork is kept at:
	// a backdrop's on a 4K screen.
	MaxUploadedImageSide = 3840
	// uploadedScheme starts the URL standing for uploaded artwork in
	// Images, which images are served and tagged by (see UploadedImage).
	uploadedScheme = "polyfin-upload:"
)

// Overrides are the fields of an item an administrator set; nil fields
// follow the addon. Lists are never empty: clearing one follows the addon
// again.
type Overrides struct {
	Name            *string    `json:"name,omitempty"`
	OriginalTitle   *string    `json:"originalTitle,omitempty"`
	SortName        *string    `json:"sortName,omitempty"`
	Overview        *string    `json:"overview,omitempty"`
	Taglines        []string   `json:"taglines,omitempty"`
	Genres          []string   `json:"genres,omitempty"`
	Tags            []string   `json:"tags,omitempty"`
	Studios         []string   `json:"studios,omitempty"`
	OfficialRating  *string    `json:"officialRating,omitempty"`
	CustomRating    *string    `json:"customRating,omitempty"`
	CommunityRating *float64   `json:"communityRating,omitempty"`
	CriticRating    *float64   `json:"criticRating,omitempty"`
	PremiereDate    *time.Time `json:"premiereDate,omitempty"`
	EndDate         *time.Time `json:"endDate,omitempty"`
	ProductionYear  *int       `json:"productionYear,omitempty"`
}

func (o Overrides) empty() bool {
	return o.Name == nil && o.OriginalTitle == nil && o.SortName == nil && o.Overview == nil && o.Taglines == nil &&
		o.Genres == nil && o.Tags == nil && o.Studios == nil && o.OfficialRating == nil && o.CustomRating == nil &&
		o.CommunityRating == nil && o.CriticRating == nil && o.PremiereDate == nil && o.EndDate == nil && o.ProductionYear == nil
}

// apply sets the fields overridden on item.
func (o Overrides) apply(item *Item) {
	text := func(field *string, value *string) {
		if value != nil {
			*field = *value
		}
	}
	list := func(field *[]string, value []string) {
		if value != nil {
			*field = slices.Clone(value)
		}
	}
	text(&item.Name, o.Name)
	text(&item.OriginalTitle, o.OriginalTitle)
	text(&item.SortName, o.SortName)
	text(&item.Overview, o.Overview)
	text(&item.OfficialRating, o.OfficialRating)
	text(&item.CustomRating, o.CustomRating)
	list(&item.Taglines, o.Taglines)
	list(&item.Genres, o.Genres)
	list(&item.Tags, o.Tags)
	list(&item.Studios, o.Studios)
	if o.CommunityRating != nil {
		item.CommunityRating = *o.CommunityRating
	}
	if o.CriticRating != nil {
		item.CriticRating = new(*o.CriticRating)
	}
	if o.PremiereDate != nil {
		item.PremiereDate = new(*o.PremiereDate)
	}
	if o.EndDate != nil {
		item.EndDate = new(*o.EndDate)
	}
	if o.ProductionYear != nil {
		item.ProductionYear = *o.ProductionYear
	}
}

// parentalRating is the rating set that parental control judges a title
// by, as Jellyfin does: the custom rating, else the official one.
func (o Overrides) parentalRating() (string, bool) {
	if o.CustomRating != nil {
		return *o.CustomRating, true
	}
	if o.OfficialRating != nil {
		return *o.OfficialRating, true
	}
	return "", false
}

// against returns the overrides an edit makes of an item whose addon gives
// base, current being those set before. Text left empty, lists emptied and
// values equal to the addon's follow the addon; lists not sent (nil) stay
// as they were.
func (o Overrides) against(base Item, current Overrides) Overrides {
	text := func(value *string, addon string) *string {
		if value == nil {
			return nil
		}
		trimmed := strings.TrimSpace(*value)
		if trimmed == "" || trimmed == addon {
			return nil
		}
		return &trimmed
	}
	list := func(value, addon, kept []string) []string {
		if value == nil {
			return kept
		}
		var result []string
		for _, entry := range value {
			entry = strings.TrimSpace(entry)
			if entry != "" && !slices.ContainsFunc(result, func(e string) bool { return strings.EqualFold(e, entry) }) {
				result = append(result, entry)
			}
		}
		if len(result) == 0 || slices.Equal(result, addon) {
			return nil
		}
		return result
	}
	number := func(value *float64, addon *float64) *float64 {
		if value == nil || addon != nil && *addon == *value {
			return nil
		}
		return new(*value)
	}
	// The editor edits days: a date on the addon's day is the addon's.
	date := func(value *time.Time, addon *time.Time) *time.Time {
		if value == nil {
			return nil
		}
		v := value.UTC()
		if addon != nil {
			a := addon.UTC()
			if v.Year() == a.Year() && v.YearDay() == a.YearDay() {
				return nil
			}
		}
		return &v
	}
	var community *float64
	if base.CommunityRating > 0 {
		community = new(base.CommunityRating)
	}
	result := Overrides{
		Name:            text(o.Name, base.Name),
		OriginalTitle:   text(o.OriginalTitle, base.OriginalTitle),
		SortName:        text(o.SortName, base.SortName),
		Overview:        text(o.Overview, base.Overview),
		OfficialRating:  text(o.OfficialRating, base.OfficialRating),
		CustomRating:    text(o.CustomRating, base.CustomRating),
		Taglines:        list(o.Taglines, base.Taglines, current.Taglines),
		Genres:          list(o.Genres, base.Genres, current.Genres),
		Tags:            list(o.Tags, base.Tags, current.Tags),
		Studios:         list(o.Studios, base.Studios, current.Studios),
		CommunityRating: number(o.CommunityRating, community),
		CriticRating:    number(o.CriticRating, base.CriticRating),
		PremiereDate:    date(o.PremiereDate, base.PremiereDate),
		EndDate:         date(o.EndDate, base.EndDate),
	}
	if o.ProductionYear != nil && *o.ProductionYear > 0 && *o.ProductionYear != base.ProductionYear {
		result.ProductionYear = new(*o.ProductionYear)
	}
	return result
}

// overrideSet is every item's overrides and uploaded images, by image type
// with their tags. Polyfin keeps them all in memory: listings apply them to
// every item they show, and administrators edit few items.
type overrideSet struct {
	fields map[accounts.ID]Overrides
	images map[accounts.ID]map[string]string
}

// overridesNow returns the overrides, loaded on first use. If they cannot
// be read, items show as their addons describe them until they can.
func (s *Service) overridesNow() *overrideSet {
	if set := s.overrides.Load(); set != nil {
		return set
	}
	s.overridesMu.Lock()
	defer s.overridesMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	set, err := s.loadOverrides(ctx)
	if err != nil {
		s.logger.Error("Administrators' edits of items could not be read", "error", err)
		return &overrideSet{}
	}
	return set
}

// loadOverrides reads the overrides unless they are loaded; overridesMu
// is held.
func (s *Service) loadOverrides(ctx context.Context) (*overrideSet, error) {
	if set := s.overrides.Load(); set != nil {
		return set, nil
	}
	set := &overrideSet{fields: map[accounts.ID]Overrides{}, images: map[accounts.ID]map[string]string{}}
	rows, err := s.db.Query(ctx, "SELECT item_id, fields FROM item_overrides")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id accounts.ID
		var raw []byte
		var o Overrides
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal(raw, &o); err != nil {
			rows.Close()
			return nil, err
		}
		set.fields[id] = o
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = s.db.Query(ctx, "SELECT item_id, image_type, tag FROM item_images")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id accounts.ID
		var imageType, tag string
		if err := rows.Scan(&id, &imageType, &tag); err != nil {
			return nil, err
		}
		if set.images[id] == nil {
			set.images[id] = map[string]string{}
		}
		set.images[id][imageType] = tag
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.overrides.Store(set)
	return set, nil
}

// changeOverrides stores a change made in the database by write, then
// applies it to a copy of the overrides kept, under overridesMu.
func (s *Service) changeOverrides(ctx context.Context, write func(*overrideSet) error, change func(*overrideSet)) error {
	s.overridesMu.Lock()
	defer s.overridesMu.Unlock()
	current, err := s.loadOverrides(ctx)
	if err != nil {
		return err
	}
	if err := write(current); err != nil {
		return err
	}
	next := &overrideSet{fields: make(map[accounts.ID]Overrides, len(current.fields)), images: make(map[accounts.ID]map[string]string, len(current.images))}
	for id, o := range current.fields {
		next.fields[id] = o
	}
	for id, images := range current.images {
		next.images[id] = images
	}
	change(next)
	s.overrides.Store(next)
	return nil
}

// Overrides returns the fields an administrator set for an item.
func (s *Service) Overrides(id accounts.ID) Overrides {
	return s.overridesNow().fields[id]
}

// SaveOverrides keeps an administrator's edit of an item whose addon
// describes it as base (see Original): the fields of requested that differ
// from the addon's, the others following the addon again.
func (s *Service) SaveOverrides(ctx context.Context, base Item, requested Overrides) error {
	var result Overrides
	return s.changeOverrides(ctx, func(current *overrideSet) error {
		result = requested.against(base, current.fields[base.ID])
		if result.empty() {
			_, err := s.db.Exec(ctx, "DELETE FROM item_overrides WHERE item_id = $1", base.ID)
			return err
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return err
		}
		_, err = s.db.Exec(ctx, `INSERT INTO item_overrides (item_id, fields) VALUES ($1, $2)
			ON CONFLICT (item_id) DO UPDATE SET fields = excluded.fields, updated_at = now()`, base.ID, encoded)
		return err
	}, func(next *overrideSet) {
		if result.empty() {
			delete(next.fields, base.ID)
		} else {
			next.fields[base.ID] = result
		}
	})
}

// uploadedType returns the image type of an upload as Polyfin names it, and
// whether it is one administrators may upload.
func uploadedType(imageType string) (string, bool) {
	i := slices.IndexFunc(UploadedImageTypes, func(t string) bool { return strings.EqualFold(t, imageType) })
	if i < 0 {
		return "", false
	}
	return UploadedImageTypes[i], true
}

// ErrUnsupportedImageType reports an image type administrators may not
// upload (see UploadedImageTypes).
var ErrUnsupportedImageType = errors.New("unsupported image type")

// UploadImage keeps data, a JPEG, PNG or WebP picture, as an item's
// artwork of imageType in place of the addon's, normalized as profile
// pictures are (see accounts.NormalizeImage) within MaxUploadedImageSide.
func (s *Service) UploadImage(ctx context.Context, id accounts.ID, imageType string, data []byte) error {
	imageType, ok := uploadedType(imageType)
	if !ok {
		return ErrUnsupportedImageType
	}
	picture, err := accounts.NormalizeImage(data, MaxUploadedImageBytes, MaxUploadedImageSide)
	if err != nil {
		return err
	}
	return s.changeOverrides(ctx, func(*overrideSet) error {
		_, err := s.db.Exec(ctx, `INSERT INTO item_images (item_id, image_type, image, content_type, tag) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (item_id, image_type) DO UPDATE SET image = excluded.image, content_type = excluded.content_type,
			tag = excluded.tag, updated_at = now()`, id, imageType, picture.Data, picture.ContentType, picture.Tag)
		return err
	}, func(next *overrideSet) {
		images := map[string]string{imageType: picture.Tag}
		for t, tag := range next.images[id] {
			if t != imageType {
				images[t] = tag
			}
		}
		next.images[id] = images
	})
}

// DeleteUploadedImage drops an item's uploaded artwork of imageType, which
// shows the addon's again; there may be none.
func (s *Service) DeleteUploadedImage(ctx context.Context, id accounts.ID, imageType string) error {
	imageType, ok := uploadedType(imageType)
	if !ok {
		return nil
	}
	return s.changeOverrides(ctx, func(*overrideSet) error {
		_, err := s.db.Exec(ctx, "DELETE FROM item_images WHERE item_id = $1 AND image_type = $2", id, imageType)
		return err
	}, func(next *overrideSet) {
		images := map[string]string{}
		for t, tag := range next.images[id] {
			if t != imageType {
				images[t] = tag
			}
		}
		if len(images) == 0 {
			delete(next.images, id)
		} else {
			next.images[id] = images
		}
	})
}

// deleteUploadedImages drops every uploaded artwork of items, of any image
// type; there may be none.
func (s *Service) deleteUploadedImages(ctx context.Context, items []accounts.ID) error {
	if len(items) == 0 {
		return nil
	}
	return s.changeOverrides(ctx, func(*overrideSet) error {
		_, err := s.db.Exec(ctx, "DELETE FROM item_images WHERE item_id = ANY($1::uuid[])", items)
		return err
	}, func(next *overrideSet) {
		for _, id := range items {
			delete(next.images, id)
		}
	})
}

// uploadedURL stands for an item's uploaded artwork in Images: it changes
// with the upload, and so does its ImageTag.
func uploadedURL(id accounts.ID, imageType, tag string) string {
	return uploadedScheme + id.String() + "/" + imageType + "/" + tag
}

// Uploaded reports whether an artwork URL stands for uploaded artwork, and
// whose.
func Uploaded(url string) (id accounts.ID, imageType string, ok bool) {
	rest, ok := strings.CutPrefix(url, uploadedScheme)
	if !ok {
		return accounts.ID{}, "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 3 {
		return accounts.ID{}, "", false
	}
	id, err := accounts.ParseID(parts[0])
	return id, parts[1], err == nil
}

// UploadedImage returns an item's uploaded artwork of imageType;
// ErrNotFound when there is none.
func (s *Service) UploadedImage(ctx context.Context, id accounts.ID, imageType string) (accounts.UserImage, error) {
	var picture accounts.UserImage
	err := s.db.QueryRow(ctx, "SELECT image, content_type, tag FROM item_images WHERE item_id = $1 AND image_type = $2", id, imageType).
		Scan(&picture.Data, &picture.ContentType, &picture.Tag)
	if errors.Is(err, pgx.ErrNoRows) {
		return accounts.UserImage{}, ErrNotFound
	}
	return picture, err
}

// UploadedArtwork returns the URL standing for an item's artwork of
// imageType an administrator uploaded, if there is one (see Uploaded).
func (s *Service) UploadedArtwork(id accounts.ID, imageType string) (string, bool) {
	return s.overridesNow().uploadedArtwork(id, imageType)
}

// uploadedArtwork returns the URL of an item's uploaded artwork of
// imageType, if there is one.
func (set *overrideSet) uploadedArtwork(id accounts.ID, imageType string) (string, bool) {
	imageType, ok := uploadedType(imageType)
	if !ok {
		return "", false
	}
	tag, ok := set.images[id][imageType]
	if !ok {
		return "", false
	}
	return uploadedURL(id, imageType, tag), true
}

// setImage sets the artwork of an image type in images.
func setImage(images *Images, imageType, url string) {
	switch imageType {
	case "Primary":
		images.Primary = url
	case "Backdrop":
		images.Backdrop = url
	case "Logo":
		images.Logo = url
	case "Thumb":
		images.Thumb = url
	case "Banner":
		images.Banner = url
	}
}

// apply sets on item what administrators changed of it and of the items
// it shows: its series, season and album, its channel, the people it
// credits and its artists. Seasons and episodes follow the ratings set on
// their series, as Jellyfin's edits of a series rate its seasons and
// episodes; seasons without artwork of their own show their series'.
func (set *overrideSet) apply(item *Item) {
	own, hasOwn := set.fields[item.ID]
	if hasOwn {
		own.apply(item)
	}
	for imageType, tag := range set.images[item.ID] {
		setImage(&item.Images, imageType, uploadedURL(item.ID, imageType, tag))
	}
	if item.SeriesID != (accounts.ID{}) {
		if series, ok := set.fields[item.SeriesID]; ok {
			if series.Name != nil {
				item.SeriesName = *series.Name
			}
			if series.OfficialRating != nil && own.OfficialRating == nil {
				item.OfficialRating = *series.OfficialRating
			}
			if series.CustomRating != nil && own.CustomRating == nil {
				item.CustomRating = *series.CustomRating
			}
		}
		if url, ok := set.uploadedArtwork(item.SeriesID, "Primary"); ok {
			if item.Kind == KindSeason && item.Images.Primary == item.SeriesPoster {
				item.Images.Primary = url
			}
			item.SeriesPoster = url
		}
		if url, ok := set.uploadedArtwork(item.SeriesID, "Backdrop"); ok && set.images[item.ID]["Backdrop"] == "" {
			item.Images.Backdrop = url
		}
	}
	if item.SeasonID != (accounts.ID{}) {
		if season, ok := set.fields[item.SeasonID]; ok && season.Name != nil {
			item.SeasonName = *season.Name
		}
		if url, ok := set.uploadedArtwork(item.SeasonID, "Primary"); ok {
			item.SeasonPoster = url
		}
	}
	if item.AlbumID != (accounts.ID{}) {
		if album, ok := set.fields[item.AlbumID]; ok && album.Name != nil {
			item.Album = *album.Name
		}
		if url, ok := set.uploadedArtwork(item.AlbumID, "Primary"); ok {
			item.AlbumPoster = url
		}
	}
	if item.Channel != nil {
		channel := *item.Channel
		set.apply(&channel)
		item.Channel = &channel
	}
	if slices.ContainsFunc(item.People, func(p Person) bool { return set.touches(p.ID) }) {
		people := slices.Clone(item.People)
		for i := range people {
			if o, ok := set.fields[people[i].ID]; ok && o.Name != nil {
				people[i].Name = *o.Name
			}
			if url, ok := set.uploadedArtwork(people[i].ID, "Primary"); ok {
				people[i].Image = url
			}
		}
		item.People = people
	}
	if slices.ContainsFunc(item.Artists, func(c Credit) bool { return set.renames(c.ID) }) {
		artists := slices.Clone(item.Artists)
		for i := range artists {
			set.rename(&artists[i])
		}
		item.Artists = artists
	}
	if item.AlbumArtist != nil && set.renames(item.AlbumArtist.ID) {
		artist := *item.AlbumArtist
		set.rename(&artist)
		item.AlbumArtist = &artist
	}
}

// touches reports whether an administrator changed anything of an item.
func (set *overrideSet) touches(id accounts.ID) bool {
	_, edited := set.fields[id]
	return edited || len(set.images[id]) > 0
}

func (set *overrideSet) renames(id accounts.ID) bool {
	return set.fields[id].Name != nil
}

func (set *overrideSet) rename(c *Credit) {
	if name := set.fields[c.ID].Name; name != nil {
		c.Name = *name
	}
}

// overridden returns items with what administrators changed of them (see
// overrideSet.apply). Items may come from caches: they are copied, never
// changed in place.
func (s *Service) overridden(items []Item) []Item {
	set := s.overridesNow()
	if len(set.fields) == 0 && len(set.images) == 0 {
		return items
	}
	result := slices.Clone(items)
	for i := range result {
		set.apply(&result[i])
	}
	return result
}

// overriddenItem returns an item with what administrators changed of it.
func (s *Service) overriddenItem(item Item) Item {
	s.overridesNow().apply(&item)
	return item
}

// Overridden returns items made apart from the addons, Polyfin's own
// collections among them, with what administrators changed of them.
func (s *Service) Overridden(items ...Item) []Item {
	return s.overridden(items)
}

// overriddenTraits returns the traits parental control judges a title by
// once what administrators set of it applies: its rating and genres.
func (s *Service) overriddenTraits(id accounts.ID, t traits) traits {
	o, ok := s.overridesNow().fields[id]
	if !ok {
		return t
	}
	if rating, ok := o.parentalRating(); ok {
		t.rating, t.ratingKnown = rating, true
	}
	if o.Genres != nil {
		t.genres, t.genresKnown = o.Genres, true
	}
	return t
}

// renamedMatches lists the items an administrator named so that term
// matches their name, without regard to case.
func (s *Service) renamedMatches(term string) []accounts.ID {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return nil
	}
	var ids []accounts.ID
	for id, o := range s.overridesNow().fields {
		if o.Name != nil && strings.Contains(strings.ToLower(*o.Name), term) {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b accounts.ID) int { return strings.Compare(a.String(), b.String()) })
	return ids
}

// RemoteResult is a title an addon's search found, with the name of the
// addon.
type RemoteResult struct {
	Item     Item
	Provider string
}

// RemoteSearch looks a name up for titles of kind in the search catalogs of
// the server's addons that describe titles, as Jellyfin asks its metadata
// providers when an administrator identifies an item. Without such an
// addon, it finds nothing.
func (s *Service) RemoteSearch(ctx context.Context, kind Kind, name string, limit int) ([]RemoteResult, error) {
	name = strings.TrimSpace(name)
	if name == "" || limit <= 0 {
		return nil, nil
	}
	// A view without a user is the server's addons'.
	v, err := s.view(ctx, accounts.User{})
	if err != nil {
		return nil, err
	}
	var sources []source
	for _, entry := range v.addons {
		if !entry.addon.Manifest.HasResource("meta") {
			continue
		}
		for _, catalog := range entry.addon.Manifest.Catalogs {
			if catalogKind, ok := titleKind(catalog.Type); ok && catalogKind == kind && searchable(catalog) {
				sources = append(sources, source{addon: entry, catalog: catalog, search: name})
			}
		}
	}
	if len(sources) == 0 {
		return nil, nil
	}
	titles, _, err := s.merged(ctx, v, sources, 0, limit)
	if err != nil {
		return nil, err
	}
	var results []RemoteResult
	var records []record
	for _, title := range titles {
		item, r, err := title.title(accounts.ID{})
		if err != nil {
			continue
		}
		r.Parent = nil
		results = append(results, RemoteResult{Item: item, Provider: title.src.addon.addon.Manifest.Name})
		records = append(records, r)
	}
	return results, s.save(ctx, records)
}
