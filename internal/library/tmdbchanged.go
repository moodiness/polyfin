package library

import (
	"context"
	"errors"
)

// titleTMDBChanged is the one place to call once the TMDB identifier a
// movie or series shows changes, whatever changed it: an administrator's
// edit (see SaveOverrides), or a correction of its source's. old and now
// are its provider identifiers before and after (see providerIDs), and
// title names it by its ID, Kind and StremioID. What was learnt by the
// former identifier is dropped, or asked again by the new one:
//   - a series' alignment of its episodes on TMDB's numbering (see
//     episodealign.go), which the next numbering makes again, and which
//     a series left without a TMDB identifier would otherwise keep;
//   - the segment databases' answers about the movie, or about each of
//     the series' episodes, asked with the former identifier (see package
//     mediasegments), so that they are asked again with the new one;
//   - the title among those its TMDB credits were saved on, under the key
//     of its former identifiers, and its credits are looked up under the
//     new one (see creditsMoved).
//
// Everything else keyed by a TMDB identifier, artwork, ratings, box sets
// and release dates, is found by the new one. The tracking services'
// events already queued, and what imports learnt of a title for a day,
// keep the former identifier.
func (s *Service) titleTMDBChanged(ctx context.Context, title Item, old, now map[string]string) error {
	var errs []error
	switch title.Kind {
	case KindMovie:
		_, err := s.db.Exec(ctx, "DELETE FROM media_segments WHERE item_id = $1", title.ID)
		errs = append(errs, err)
	case KindSeries:
		_, err := s.db.Exec(ctx, "DELETE FROM episode_alignments WHERE series = $1", title.StremioID)
		errs = append(errs, err)
		_, err = s.db.Exec(ctx, `DELETE FROM media_segments WHERE item_id IN
			(SELECT id FROM items WHERE kind = 'episode' AND data->>'seriesId' = $1)`, title.StremioID)
		errs = append(errs, err)
	default:
		return nil
	}
	errs = append(errs, s.creditsMoved(ctx, title.ID, title.Kind, old, now))
	return errors.Join(errs...)
}
