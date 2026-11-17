package library

import (
	"encoding/binary"
	"hash/maphash"

	"github.com/moodiness/polyfin/internal/stremio"
)

// stamp tells a series' list from another without making its records:
// it is a hash of all they are made from (see records), written into the
// hash as it is, field by field, where the fingerprint of a list encoded
// each of its records, every episode of the series, after every Next Up.
// Two lists of the same stamp make the same records: the series and its
// descriptions' identifiers; the seasons' numbers and posters; the
// episodes as shown, in the order they aired; every video of the
// description as recorded, its streams aside, which records leave out;
// and the seasons of the series' order.
func (l *outlinedEpisodes) stamp() uint64 {
	var h maphash.Hash
	h.SetSeed(recordedSeed)
	_, _ = h.Write(l.series.ID[:])
	writeText(&h, l.meta.ID)
	writeText(&h, l.shown.ID)
	for _, number := range seasonNumbers(l.meta) {
		writeNumber(&h, int64(number))
		writeText(&h, l.posters[number])
	}
	writeNumber(&h, int64(len(l.aired)))
	for place := range l.aired {
		video := l.video(place)
		writeText(&h, video.ID)
		writeNumber(&h, int64(video.Season))
	}
	writeNumber(&h, int64(len(l.meta.Videos)))
	for i := range l.meta.Videos {
		writeVideo(&h, &l.meta.Videos[i])
	}
	if l.layout != nil {
		writeText(&h, l.layout.group)
		writeNumber(&h, int64(len(l.layout.seasons)))
		for _, season := range l.layout.seasons {
			writeNumber(&h, int64(season.number))
		}
		if l.layout.hidden != nil {
			writeNumber(&h, int64(l.layout.hidden.number))
		}
	}
	return h.Sum64()
}

// writeVideo writes into h every field of a video a record keeps (see
// record.stored): all of them but its streams.
func writeVideo(h *maphash.Hash, video *stremio.Video) {
	for _, text := range []string{video.ID, video.Title, video.Name, video.Released, video.Thumbnail, video.Overview, string(video.Runtime),
		string(video.MoviedbID), string(video.TvdbID), video.StartTime, video.EndTime} {
		writeText(h, text)
	}
	for _, number := range []stremio.Number{video.Season, video.Episode, video.Number} {
		writeNumber(h, int64(number))
	}
	switch {
	case video.Available == nil:
		writeNumber(h, -1)
	case *video.Available:
		writeNumber(h, 1)
	default:
		writeNumber(h, 0)
	}
	writeNumber(h, int64(len(video.Genres)))
	for _, genre := range video.Genres {
		writeText(h, genre)
	}
}

// writeText writes text into h after its length, so that two texts never
// write as their concatenation does.
func writeText(h *maphash.Hash, text string) {
	writeNumber(h, int64(len(text)))
	_, _ = h.WriteString(text)
}

func writeNumber(h *maphash.Hash, n int64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(n))
	_, _ = h.Write(b[:])
}
