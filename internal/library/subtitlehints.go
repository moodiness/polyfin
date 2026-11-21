package library

import (
	"context"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// SubtitleHints describe the video a title's subtitles are for, as
// subtitle addons take them to find the files made for it: its file name,
// its size in bytes and its video hash, each when known. The zero
// value describes none: the title's subtitles are asked for.
type SubtitleHints struct {
	Filename string
	Size     int64
	Hash     string
}

// extra is the request properties hints send to an addon, those known.
func (h SubtitleHints) extra() []stremio.ExtraValue {
	var extra []stremio.ExtraValue
	if h.Hash != "" {
		extra = append(extra, stremio.ExtraValue{Name: "videoHash", Value: h.Hash})
	}
	if h.Size > 0 {
		extra = append(extra, stremio.ExtraValue{Name: "videoSize", Value: strconv.FormatInt(h.Size, 10)})
	}
	if h.Filename != "" {
		extra = append(extra, stremio.ExtraValue{Name: "filename", Value: h.Filename})
	}
	return extra
}

// hintsSeparator parts, in the identifier an addon's subtitle list is kept
// under, the title's identifier from the hints it was asked with: no
// identifier an addon gives holds it.
const hintsSeparator = "\x00"

// subtitlesKey names an addon's subtitle list for a title asked with
// hints: the title's own list without any, a list of its own with them,
// kept, shared and followed up as the title's is (see subtitleList).
func subtitlesKey(addon accounts.ID, t target, hints SubtitleHints) streamKey {
	key := streamKey{addon, t.metaType, t.id}
	if hints != (SubtitleHints{}) {
		key.id += hintsSeparator + hints.Hash + hintsSeparator + strconv.FormatInt(hints.Size, 10) + hintsSeparator + hints.Filename
	}
	return key
}

// subtitlesAsked returns the title's identifier a subtitle list's names
// (see subtitlesKey), and the request properties it was asked with.
func subtitlesAsked(id string) (string, []stremio.ExtraValue) {
	id, hinted, ok := strings.Cut(id, hintsSeparator)
	if !ok {
		return id, nil
	}
	parts := strings.SplitN(hinted, hintsSeparator, 3)
	if len(parts) != 3 {
		return id, nil
	}
	size, _ := strconv.ParseInt(parts[1], 10, 64)
	return id, SubtitleHints{Hash: parts[0], Size: size, Filename: parts[2]}.extra()
}

// hintedOf reports whether key names an addon's subtitle list for title,
// the key of its list without hints, asked with some.
func hintedOf(key, title streamKey) bool {
	return key.addon == title.addon && key.contentType == title.contentType &&
		strings.HasPrefix(key.id, title.id+hintsSeparator)
}

// forgetHintedSubtitles forgets, as forgetLists forgets the subtitle list
// of an addon for a title, under title, those it gave for the title asked
// with hints: their follow-ups stop, the requests asking the addon for
// them are not joined any more, and their answers are not stored.
func (s *Service) forgetHintedSubtitles(title streamKey) {
	var flights []string
	s.followUps.mu.Lock()
	for key := range s.followUps.asking {
		if key.list == listSubtitles && hintedOf(key.streamKey, title) {
			delete(s.followUps.asking, key)
			flights = append(flights, subtitlesFlight(key.streamKey))
		}
	}
	for key := range s.followUps.running {
		if key.list == listSubtitles && hintedOf(key.streamKey, title) {
			delete(s.followUps.running, key)
		}
	}
	s.followUps.mu.Unlock()
	for _, flight := range flights {
		s.flight.Forget(flight)
	}
	s.subtitleLists.DeleteFunc(func(key streamKey, _ list[stremio.Subtitle]) bool { return hintedOf(key, title) })
}

// SubtitlesFor lists, as Subtitles does, the subtitle files of a movie or
// an episode, asking the addons for those of the video hints describe, as
// they are asked when it plays. An addon that cannot answer so lists the
// title's.
func (s *Service) SubtitlesFor(ctx context.Context, user accounts.User, id accounts.ID, hints SubtitleHints) ([]ExternalSubtitle, error) {
	subtitles, _, err := s.subtitlesOf(ctx, user, id, hints, waitAll)
	return subtitles, err
}
