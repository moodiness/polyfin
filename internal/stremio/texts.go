package stremio

import (
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
	// MaxOverview bounds an overview: a title's or an episode's.
	MaxOverview = 64 << 10
)

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
// MaxOverview). Its lists of names are cut as they are read (see Names);
// its links' names when they are read into them (see links.credit).
func (m *Meta) bound() {
	for _, name := range []*string{&m.Name, &m.Released, &m.Status, &m.Language, &m.PosterShape,
		(*string)(&m.ReleaseInfo), (*string)(&m.Year), (*string)(&m.Runtime), (*string)(&m.ImdbRating)} {
		*name = Cut(*name, MaxName)
	}
	m.Description = Cut(m.Description, MaxOverview)
	for i := range m.Videos {
		video := &m.Videos[i]
		for _, name := range []*string{&video.Title, &video.Name, &video.Released, (*string)(&video.Runtime)} {
			*name = Cut(*name, MaxName)
		}
		video.Overview = Cut(video.Overview, MaxOverview)
	}
	for i := range m.Trailers {
		m.Trailers[i].Name = Cut(m.Trailers[i].Name, MaxName)
		m.Trailers[i].Type = Cut(m.Trailers[i].Type, MaxName)
	}
	if m.Extras != nil {
		for i := range m.Extras.Cast {
			m.Extras.Cast[i].Name = Cut(m.Extras.Cast[i].Name, MaxName)
			m.Extras.Cast[i].Character = Cut(m.Extras.Cast[i].Character, MaxName)
		}
		m.Extras.Certification = Cut(m.Extras.Certification, MaxName)
		m.Extras.CertificationLocal = Cut(m.Extras.CertificationLocal, MaxName)
	}
}
