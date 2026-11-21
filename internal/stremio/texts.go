package stremio

import (
	"maps"
	"slices"
	"strings"
	"unicode/utf8"
)

// What Polyfin keeps of the texts an addon gives. An answer is bounded in
// bytes, not its texts, and one name is shown in every listing, recorded
// and indexed for search, so each text is cut, on a character, well past
// what any real one takes.
const (
	// MaxName bounds a name: a title's, an episode's, a catalog's, a
	// person's, a genre's, a trailer's, and the other short texts of a
	// description, such as its status, language or dates.
	MaxName = 1 << 10
	// MaxOverview bounds an overview: a title's, an episode's, a
	// stream's or an addon's.
	MaxOverview = 64 << 10
	// MaxURL bounds an address: an image's, a trailer's, a stream's. It is
	// past what web servers take in a request line, so no longer one could
	// be asked for. An address is never cut, as a cut one would lead
	// elsewhere: a longer one is left out (see address).
	MaxURL = 16 << 10
)

// address is link as an addon gives it, or empty past MaxURL.
func address(link string) string {
	if len(link) > MaxURL {
		return ""
	}
	return link
}

// Cut is text cut to at most most bytes, at the start of a character, so
// valid UTF-8 stays valid. A text cut is copied, so that it does not keep
// the whole answer it was read from.
func Cut(text string, most int) string {
	if len(text) <= most {
		return text
	}
	end := most
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return strings.Clone(text[:end])
}

// bound cuts the texts of a meta as an addon gives them (see MaxName and
// MaxOverview), and leaves out its addresses past MaxURL. Its lists of
// names are cut as they are read (see Names); its links' names when they
// are read into them (see links.credit); its videos' streams as they are
// read (see decodeStream).
func (m *Meta) bound() {
	for _, name := range []*string{&m.Name, &m.Released, &m.Status, &m.Language, &m.PosterShape,
		(*string)(&m.ReleaseInfo), (*string)(&m.Year), (*string)(&m.Runtime), (*string)(&m.ImdbRating)} {
		*name = Cut(*name, MaxName)
	}
	m.Description = Cut(m.Description, MaxOverview)
	for _, link := range []*string{&m.Poster, &m.Background, &m.Logo, &m.LandscapePoster} {
		*link = address(*link)
	}
	for i := range m.Videos {
		video := &m.Videos[i]
		for _, name := range []*string{&video.Title, &video.Name, &video.Released, (*string)(&video.Runtime)} {
			*name = Cut(*name, MaxName)
		}
		video.Overview = Cut(video.Overview, MaxOverview)
		video.Thumbnail = address(video.Thumbnail)
	}
	for i := range m.Trailers {
		m.Trailers[i].Name = Cut(m.Trailers[i].Name, MaxName)
		m.Trailers[i].Type = Cut(m.Trailers[i].Type, MaxName)
		m.Trailers[i].Source = address(m.Trailers[i].Source)
	}
	if m.Extras != nil {
		for i := range m.Extras.Cast {
			m.Extras.Cast[i].Name = Cut(m.Extras.Cast[i].Name, MaxName)
			m.Extras.Cast[i].Character = Cut(m.Extras.Cast[i].Character, MaxName)
			m.Extras.Cast[i].Photo = address(m.Extras.Cast[i].Photo)
		}
		m.Extras.Certification = Cut(m.Extras.Certification, MaxName)
		m.Extras.CertificationLocal = Cut(m.Extras.CertificationLocal, MaxName)
		// A season poster keeps its place, as one missing does.
		for i, poster := range m.Extras.SeasonPosters {
			m.Extras.SeasonPosters[i] = address(poster)
		}
		maps.DeleteFunc(m.Extras.SeasonPosterByNumber, func(_, poster string) bool { return len(poster) > MaxURL })
	}
}

// bound cuts the texts of a stream as an addon gives them, and leaves out
// its addresses past MaxURL, and its trackers and subtitles with one. A
// stream left with no address and no text tells nothing (see tells).
func (s *Stream) bound() {
	for _, name := range []*string{&s.Name, &s.Title, &s.BehaviorHints.BingeGroup, &s.BehaviorHints.Filename, &s.Data.Service.ID} {
		*name = Cut(*name, MaxName)
	}
	s.Description = Cut(s.Description, MaxOverview)
	for _, link := range []*string{&s.URL, &s.ExternalURL, &s.YouTubeID, &s.InfoHash, &s.Usenet} {
		*link = address(*link)
	}
	s.Sources = slices.DeleteFunc(s.Sources, func(source string) bool { return len(source) > MaxURL })
	s.Subtitles = slices.DeleteFunc(s.Subtitles, func(subtitle Subtitle) bool { return len(subtitle.URL) > MaxURL })
}
