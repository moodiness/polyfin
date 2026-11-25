package tmdb

import (
	"cmp"
	"context"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// The extras Moonfin's apps show (see the jellyfin package's
// moonfinextras.go): reviews of titles, people's links, the outline of a
// title the server does not hold, the votes of episodes and the next
// episode of series. Each is one request to TMDB, kept as every answer is.

// maxReviews is how many reviews Reviews returns: the first page of
// TMDB's.
const maxReviews = 20

// Review is a user review of a title on TMDB: its author, their rating out
// of 10 (nil when they gave none), its text as TMDB gives it, which may
// hold HTML, when it was written (RFC 3339, empty when TMDB gives none),
// the path of the author's picture (a path on the image server, or an
// address of another server when it starts with "/http"), and the address
// of the review on TMDB.
type Review struct {
	Author     string
	Rating     *float64
	Content    string
	Created    string
	AvatarPath string
	URL        string
}

// Reviews returns up to maxReviews user reviews of the movie or series
// (kind) TMDB identifies as id, those in language (an ISO 639-1 code, with
// a region or not), or in English when TMDB has none in it. Reviews of no
// text are left out.
func (c *Client) Reviews(ctx context.Context, kind Kind, id int, language string) ([]Review, error) {
	reviews, err := c.reviewsIn(ctx, kind, id, language)
	if err != nil || len(reviews) > 0 || language == "" || baseLanguage(language) == "en" {
		return reviews, err
	}
	return c.reviewsIn(ctx, kind, id, "en-US")
}

// reviewsIn returns the first page of the reviews of a title in language,
// TMDB's default when empty.
func (c *Client) reviewsIn(ctx context.Context, kind Kind, id int, language string) ([]Review, error) {
	query := url.Values{"page": {"1"}}
	if language != "" {
		query.Set("language", language)
	}
	var answer struct {
		Results []struct {
			Author  string `json:"author"`
			Details struct {
				Name       string   `json:"name"`
				Username   string   `json:"username"`
				AvatarPath string   `json:"avatar_path"`
				Rating     *float64 `json:"rating"`
			} `json:"author_details"`
			Content string `json:"content"`
			Created string `json:"created_at"`
			URL     string `json:"url"`
		} `json:"results"`
	}
	if err := c.get(ctx, "/"+string(kind)+"/"+strconv.Itoa(id)+"/reviews", query, &answer); err != nil {
		return nil, err
	}
	reviews := []Review{}
	for _, r := range answer.Results {
		if strings.TrimSpace(r.Content) == "" {
			continue
		}
		author := strings.TrimSpace(cmp.Or(r.Details.Name, r.Author, r.Details.Username))
		reviews = append(reviews, Review{Author: author, Rating: r.Details.Rating, Content: r.Content, Created: r.Created,
			AvatarPath: r.Details.AvatarPath, URL: r.URL})
		if len(reviews) == maxReviews {
			break
		}
	}
	return reviews, nil
}

// PersonLinks are a person's IMDb identifier, website, and accounts on
// Instagram, X, Facebook, TikTok and YouTube (their handles or
// identifiers), each empty when TMDB has none.
type PersonLinks struct {
	IMDb, Homepage, Instagram, Twitter, Facebook, TikTok, YouTube string
}

// PersonLinks returns the links of the person TMDB identifies as id, from
// their details and external identifiers, asked together.
func (c *Client) PersonLinks(ctx context.Context, id int) (PersonLinks, error) {
	var answer struct {
		Homepage string `json:"homepage"`
		IDs      struct {
			IMDb      string `json:"imdb_id"`
			Instagram string `json:"instagram_id"`
			Twitter   string `json:"twitter_id"`
			Facebook  string `json:"facebook_id"`
			TikTok    string `json:"tiktok_id"`
			YouTube   string `json:"youtube_id"`
		} `json:"external_ids"`
	}
	if err := c.get(ctx, "/person/"+strconv.Itoa(id), url.Values{"append_to_response": {"external_ids"}}, &answer); err != nil {
		return PersonLinks{}, err
	}
	ids := answer.IDs
	trim := strings.TrimSpace
	return PersonLinks{IMDb: trim(ids.IMDb), Homepage: trim(answer.Homepage), Instagram: trim(ids.Instagram), Twitter: trim(ids.Twitter),
		Facebook: trim(ids.Facebook), TikTok: trim(ids.TikTok), YouTube: trim(ids.YouTube)}, nil
}

// Outline is what an app shows of a title the server does not hold: its
// overview, TMDB's rating out of 10, its genres' names, its runtime in
// minutes (a series' first episode runtime, 0 when TMDB gives none), the
// paths on the image server of a backdrop without text, of a backdrop
// with the title on it and of a logo, and the YouTube key of its
// trailer, each empty for none.
type Outline struct {
	Overview       string
	Rating         float64
	Genres         []string
	Runtime        int
	Backdrop       string
	TitledBackdrop string
	Logo           string
	Trailer        string
}

// TitleOutline returns the outline of the movie or series (kind) TMDB
// identifies as id, in language (an ISO 639-1 code, with a region or not,
// TMDB's default when empty), its images and videos asked with its
// details. The backdrop without text is the best voted of those of no
// language, else TMDB's main one; the titled backdrop and the logo are
// the best voted in language, else in English, else TMDB's main backdrop
// and any logo. The trailer is a YouTube trailer, else a teaser, the
// official ones first, in language, else in English, else any.
func (c *Client) TitleOutline(ctx context.Context, kind Kind, id int, language string) (Outline, error) {
	lang := baseLanguage(language)
	wanted := []string{"en", "null"}
	if lang != "" && lang != "en" {
		wanted = []string{lang, "en", "null"}
	}
	query := url.Values{"append_to_response": {"images,videos"}, "include_image_language": {strings.Join(wanted, ",")},
		"include_video_language": {strings.Join(wanted, ",")}}
	if language != "" {
		query.Set("language", language)
	}
	var answer struct {
		Overview     string                  `json:"overview"`
		Rating       float64                 `json:"vote_average"`
		Genres       []struct{ Name string } `json:"genres"`
		Runtime      int                     `json:"runtime"`
		EpisodeTimes []int                   `json:"episode_run_time"`
		BackdropPath string                  `json:"backdrop_path"`
		Images       struct {
			Backdrops []Image `json:"backdrops"`
			Logos     []Image `json:"logos"`
		} `json:"images"`
		Videos struct {
			Results []video `json:"results"`
		} `json:"videos"`
	}
	if err := c.get(ctx, "/"+string(kind)+"/"+strconv.Itoa(id), query, &answer); err != nil {
		return Outline{}, err
	}
	outline := Outline{Overview: strings.TrimSpace(answer.Overview), Rating: answer.Rating, Genres: []string{}, Runtime: max(answer.Runtime, 0)}
	if outline.Runtime == 0 && len(answer.EpisodeTimes) > 0 {
		outline.Runtime = max(answer.EpisodeTimes[0], 0)
	}
	for _, genre := range answer.Genres {
		if name := strings.TrimSpace(genre.Name); name != "" {
			outline.Genres = append(outline.Genres, name)
		}
	}
	titled := []string{"en"}
	if lang != "" && lang != "en" {
		titled = []string{lang, "en"}
	}
	backdrop, _ := bestImage(answer.Images.Backdrops, "")
	outline.Backdrop = cmp.Or(backdrop, answer.BackdropPath)
	backdrop, _ = bestImage(answer.Images.Backdrops, titled...)
	outline.TitledBackdrop = cmp.Or(backdrop, answer.BackdropPath)
	outline.Logo, _ = bestImage(answer.Images.Logos, append(titled, "")...)
	if outline.Logo == "" && len(answer.Images.Logos) > 0 {
		outline.Logo = answer.Images.Logos[0].Path
	}
	// TMDB gives some logos as SVG files: its image server renders each as
	// a PNG under the same name ending in .png, which every app shows.
	if base, ok := strings.CutSuffix(strings.ToLower(outline.Logo), ".svg"); ok {
		outline.Logo = outline.Logo[:len(base)] + ".png"
	}
	outline.Trailer = trailerOf(answer.Videos.Results, lang)
	return outline, nil
}

// video is one of TMDB's videos of a title.
type video struct {
	Key      string `json:"key"`
	Site     string `json:"site"`
	Type     string `json:"type"`
	Official bool   `json:"official"`
	Language string `json:"iso_639_1"`
}

// trailerOf returns the YouTube key of the trailer of videos, else of the
// teaser, the official ones first, in lang, else in English, else any;
// empty for none.
func trailerOf(videos []video, lang string) string {
	rank := func(v video) int {
		switch v.Language {
		case lang:
			return 0
		case "en":
			return 1
		}
		return 2
	}
	kinds := map[string]int{"Trailer": 0, "Teaser": 1}
	candidates := slices.DeleteFunc(slices.Clone(videos), func(v video) bool {
		_, kind := kinds[v.Type]
		return v.Site != "YouTube" || strings.TrimSpace(v.Key) == "" || !kind
	})
	slices.SortStableFunc(candidates, func(a, b video) int {
		official := func(v video) int {
			if v.Official {
				return 0
			}
			return 1
		}
		return cmp.Or(cmp.Compare(kinds[a.Type], kinds[b.Type]), cmp.Compare(official(a), official(b)), cmp.Compare(rank(a), rank(b)))
	})
	if len(candidates) == 0 {
		return ""
	}
	return strings.TrimSpace(candidates[0].Key)
}

// baseLanguage returns the ISO 639-1 part of a language code, lowercased:
// "pt" of "pt-BR".
func baseLanguage(language string) string {
	base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(language)), "-")
	return base
}

// EpisodeVote is an episode of a season and TMDB's rating of it out of
// 10, 0 when it has no votes.
type EpisodeVote struct {
	Episode int
	Average float64
}

// SeasonVotes returns the ratings of the episodes of the season of the
// series TMDB identifies as id, in their order.
func (c *Client) SeasonVotes(ctx context.Context, id, season int) ([]EpisodeVote, error) {
	var answer struct {
		Episodes []struct {
			Number  int     `json:"episode_number"`
			Average float64 `json:"vote_average"`
		} `json:"episodes"`
	}
	if err := c.get(ctx, "/tv/"+strconv.Itoa(id)+"/season/"+strconv.Itoa(season), nil, &answer); err != nil {
		return nil, err
	}
	votes := make([]EpisodeVote, 0, len(answer.Episodes))
	for _, e := range answer.Episodes {
		votes = append(votes, EpisodeVote{Episode: e.Number, Average: e.Average})
	}
	return votes, nil
}

// NextEpisode is the next episode of a series to air: the day it airs
// ("2006-01-02"), its season and number, and its name.
type NextEpisode struct {
	AirDate string
	Season  int
	Episode int
	Name    string
}

// NextEpisode returns the next episode to air of the series TMDB
// identifies as id, nil when none is scheduled. It reads the series'
// details, which Studios reads too: one answer serves both.
func (c *Client) NextEpisode(ctx context.Context, id int) (*NextEpisode, error) {
	var answer struct {
		Next *struct {
			AirDate string `json:"air_date"`
			Season  int    `json:"season_number"`
			Episode int    `json:"episode_number"`
			Name    string `json:"name"`
		} `json:"next_episode_to_air"`
	}
	if err := c.get(ctx, "/tv/"+strconv.Itoa(id), nil, &answer); err != nil {
		return nil, err
	}
	if answer.Next == nil || strings.TrimSpace(answer.Next.AirDate) == "" {
		return nil, nil
	}
	return &NextEpisode{AirDate: strings.TrimSpace(answer.Next.AirDate), Season: answer.Next.Season, Episode: answer.Next.Episode,
		Name: strings.TrimSpace(answer.Next.Name)}, nil
}
