package tmdb

import (
	"cmp"
	"context"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// The bounds of a title's credits, Jellyfin 12.4's defaults
// (Plugins/Tmdb/Configuration/PluginConfiguration.cs lines 86 and 91).
const (
	MaxCast = 15
	MaxCrew = 15
)

// The kinds of the people a title credits, as Jellyfin names them.
const (
	Actor    = "Actor"
	Director = "Director"
	Writer   = "Writer"
	Producer = "Producer"
)

// TitleCredit is someone a movie or series credits: their TMDB identifier,
// name, kind (Actor and the others), role (the character an actor plays,
// the job of the crew), and the path of their photo on the image server,
// empty when TMDB has none.
type TitleCredit struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Role        string `json:"role,omitempty"`
	ProfilePath string `json:"photo,omitempty"`
}

// castMember is an actor of TMDB's credits: of a movie, with the one
// character they play; of a series' aggregate credits, with their roles.
type castMember struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Character   string `json:"character"`
	Order       int    `json:"order"`
	ProfilePath string `json:"profile_path"`
	Roles       []role `json:"roles"`
}

// role is a character an actor of a series played, in how many episodes.
type role struct {
	Character string `json:"character"`
	Episodes  int    `json:"episode_count"`
}

// crewMember is a member of the crew of TMDB's credits.
type crewMember struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Job         string `json:"job"`
	Department  string `json:"department"`
	ProfilePath string `json:"profile_path"`
}

type credits struct {
	Cast []castMember `json:"cast"`
	Crew []crewMember `json:"crew"`
}

// TitleDetails is what TitleDetails reads of a movie or series: its
// people (see TitleDetails), the networks a series aired on and the
// companies that made the title, each once, with their TMDB identifier
// and logo (see Studio), and its tagline, empty when TMDB has none in the
// language asked.
type TitleDetails struct {
	Credits   []TitleCredit
	Networks  []Studio
	Companies []Studio
	Tagline   string
}

// TitleDetails returns the details of the movie or series (kind) TMDB
// identifies as id, in one request: its networks, companies and tagline,
// and its people, as Jellyfin 12.4's TMDB providers keep them: actors
// in billing order, at most MaxCast, then the directors, writers and
// producers, at most MaxCrew, in TMDB's order
// (Plugins/Tmdb/Movies/TmdbMovieProvider.cs lines 276-360). A series'
// actors are those of its aggregate credits, over its whole run, each
// role an actor played a credit of its own, the one they played the
// longest first; its crew is that of its credits, and its actors too when
// TMDB has no aggregate ones (Plugins/Tmdb/TV/TmdbSeriesProvider.cs lines
// 365-420, TmdbUtils.cs lines 292-329). Credits are asked with the
// details. Roles and the tagline are in language (an ISO 639-1 code, with
// a region or not), TMDB's default when empty.
func (c *Client) TitleDetails(ctx context.Context, kind Kind, id, language string) (TitleDetails, error) {
	query := url.Values{}
	if language != "" {
		query.Set("language", language)
	}
	var details struct {
		Networks  []Studio `json:"networks"`
		Companies []Studio `json:"production_companies"`
		Tagline   string   `json:"tagline"`
		Aggregate credits  `json:"aggregate_credits"`
		Credits   credits  `json:"credits"`
	}
	path := "/movie/" + url.PathEscape(id)
	query.Set("append_to_response", "credits")
	if kind == TV {
		path = "/tv/" + url.PathEscape(id)
		query.Set("append_to_response", "aggregate_credits,credits")
	}
	if err := c.get(ctx, path, query, &details); err != nil {
		return TitleDetails{}, err
	}
	cast, crew := details.Aggregate.Cast, details.Credits.Crew
	if len(cast) == 0 {
		cast = details.Credits.Cast
	}
	return TitleDetails{Credits: keptCredits(cast, crew), Networks: studiosOf(details.Networks),
		Companies: studiosOf(details.Companies), Tagline: strings.TrimSpace(details.Tagline)}, nil
}

// keptCredits keeps the people of TMDB's credits a title shows (see
// TitleDetails).
func keptCredits(cast []castMember, crew []crewMember) []TitleCredit {
	cast = slices.DeleteFunc(slices.Clone(cast), func(m castMember) bool { return strings.TrimSpace(m.Name) == "" })
	slices.SortStableFunc(cast, func(a, b castMember) int { return cmp.Compare(a.Order, b.Order) })
	var kept []TitleCredit
	for _, m := range cast[:min(len(cast), MaxCast)] {
		name := strings.TrimSpace(m.Name)
		var characters []string
		roles := slices.Clone(m.Roles)
		slices.SortStableFunc(roles, func(a, b role) int { return cmp.Compare(b.Episodes, a.Episodes) })
		for _, role := range roles {
			if character := strings.TrimSpace(role.Character); character != "" {
				characters = append(characters, character)
			}
		}
		if len(m.Roles) == 0 {
			characters = []string{strings.TrimSpace(m.Character)}
		}
		if len(characters) == 0 {
			characters = []string{""}
		}
		for _, character := range characters {
			kept = append(kept, TitleCredit{ID: max(m.ID, 0), Name: name, Kind: Actor, Role: character, ProfilePath: m.ProfilePath})
		}
	}
	crewKept := 0
	for _, m := range crew {
		if crewKept == MaxCrew {
			break
		}
		kind := crewKind(m)
		if kind == "" {
			continue
		}
		crewKept++
		if name := strings.TrimSpace(m.Name); name != "" {
			kept = append(kept, TitleCredit{ID: max(m.ID, 0), Name: name, Kind: kind, Role: strings.TrimSpace(m.Job), ProfilePath: m.ProfilePath})
		}
	}
	return kept
}

// crewKind is the kind of a member of the crew Jellyfin keeps, empty for
// another (Plugins/Tmdb/TmdbUtils.cs lines 59-74 and 262-282).
func crewKind(m crewMember) string {
	switch department, job := strings.ToLower(m.Department), strings.ToLower(strings.TrimSpace(m.Job)); {
	case department == "directing" && job == "director":
		return Director
	case department == "production" && job == "producer":
		return Producer
	case department == "writing" && (job == "writer" || job == "screenplay" || job == "novel"):
		return Writer
	}
	return ""
}

// PersonTitles returns the movies and series TMDB credits the person it
// identifies as id in, as an actor or in the crew, each once: as a role
// rather than an appearance (see Credit.Appearance) when it is both.
func (c *Client) PersonTitles(ctx context.Context, id int) ([]Credit, error) {
	credits, err := c.PersonCredits(ctx, id)
	if err != nil {
		return nil, err
	}
	var titles []Credit
	seen := map[string]int{}
	for _, credit := range credits {
		if credit.ID <= 0 || credit.MediaType != "movie" && credit.MediaType != "tv" {
			continue
		}
		key := credit.MediaType + "/" + strconv.Itoa(credit.ID)
		i, ok := seen[key]
		switch {
		case !ok:
			seen[key] = len(titles)
			titles = append(titles, credit)
		case titles[i].Appearance() && !credit.Appearance():
			titles[i] = credit
		}
	}
	return titles, nil
}

// TMDB's genres of talk shows and of the news.
const (
	genreNews = 10763
	genreTalk = 10767
)

// Appearance reports whether TMDB marks a credit as the person appearing
// as themselves rather than in a role: a talk show or the news, or a
// character TMDB names after the person's self ("Self", "Himself",
// "Herself", "Themselves", such as "Self - Host" or "Himself (archive
// footage)").
func (c Credit) Appearance() bool {
	if slices.ContainsFunc(c.GenreIDs, func(genre int) bool { return genre == genreNews || genre == genreTalk }) {
		return true
	}
	word := strings.ToLower(strings.TrimSpace(c.Character))
	if end := strings.IndexFunc(word, func(r rune) bool { return r < 'a' || r > 'z' }); end >= 0 {
		word = word[:end]
	}
	switch word {
	case "self", "himself", "herself", "themselves", "themself":
		return true
	}
	return false
}

// ForgetTitleDetails forgets the answers kept of the details of the
// movie or series (kind) TMDB identifies as id, so that TitleDetails asks
// TMDB again, as a refresh of the title asks.
func (c *Client) ForgetTitleDetails(kind Kind, id string) {
	if c == nil {
		return
	}
	path := "/" + string(kind) + "/" + url.PathEscape(id)
	c.answers.DeleteFunc(func(key string, _ answer) bool {
		kept, _, _ := strings.Cut(key, "?")
		return kept == path
	})
}
