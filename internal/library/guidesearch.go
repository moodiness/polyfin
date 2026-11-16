package library

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/iptv"
)

// Programme searches.
//
// A search for programmes keeps those whose name, folded as searches fold
// text (iptv.Fold: lower case, without accents), holds the folded term. The
// programmes of XMLTV guides are many, a week of every channel: rather than
// reading them all and testing each name, the guide's programmes keep their
// folded title (live_guide_programmes.search), written as a guide is stored,
// which the read tests with LIKE, served by a trigram index where the server
// has pg_trgm. The read is planned for the term each time: a plan made once
// for any term cannot tell a rare one, and reads the whole guide. The column
// is folded by Polyfin, not by PostgreSQL, whose lower case and accents
// differ from Go's: the programmes stored without it, before Polyfin kept
// it, and those an archive keeps, which are copied without, are folded by
// foldGuideTitles after each guide's download and before any search reads
// them, never at an idle guide check, which sends its checks only. The
// search's own test still decides which programmes are kept, as for the
// Native EPG's programmes and those of the channels Polyfin airs.

// programmeNamesKey marks the context of a listing of programmes that keeps
// only those whose folded name holds a text (see WithProgrammeNames).
type programmeNamesKey struct{}

// WithProgrammeNames marks a listing of programmes (see Guide) whose Keep
// keeps no programme but those whose name, folded (see iptv.Fold), holds
// folded: the XMLTV guides are then read for those alone. An empty text
// marks nothing.
func WithProgrammeNames(ctx context.Context, folded string) context.Context {
	if folded == "" {
		return ctx
	}
	return context.WithValue(ctx, programmeNamesKey{}, folded)
}

// programmeNames returns the folded text a listing's programmes' names
// hold, if WithProgrammeNames marked it.
func programmeNames(ctx context.Context) (string, bool) {
	folded, ok := ctx.Value(programmeNamesKey{}).(string)
	return folded, ok && folded != ""
}

// containsPattern is the LIKE pattern of the texts that hold text.
func containsPattern(text string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text) + "%"
}

// foldTitlesBatch bounds the titles each step of foldGuideTitles folds.
const foldTitlesBatch = 5000

// foldGuideTitles writes the folded title of the guides' programmes that
// have none yet (see above), a batch of titles at a time; with none, it
// reads only the index of those (live_guide_programmes_unfolded).
func (s *Service) foldGuideTitles(ctx context.Context) error {
	_, err, _ := s.flight.Do("fold guide titles", func() (any, error) {
		for {
			rows, err := s.db.Query(ctx, `SELECT DISTINCT title FROM live_guide_programmes WHERE search IS NULL LIMIT $1`, foldTitlesBatch)
			if err != nil {
				return nil, err
			}
			titles, err := pgx.CollectRows(rows, pgx.RowTo[string])
			if err != nil || len(titles) == 0 {
				return nil, err
			}
			folded := make([]string, len(titles))
			for i, title := range titles {
				folded[i] = iptv.Fold(title)
			}
			if _, err := s.db.Exec(ctx, `UPDATE live_guide_programmes p SET search = f.search
				FROM unnest($1::text[], $2::text[]) AS f (title, search) WHERE p.search IS NULL AND p.title = f.title`, titles, folded); err != nil {
				return nil, err
			}
		}
	})
	return err
}
