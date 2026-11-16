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
// for any term cannot tell a rare one, and reads the whole guide. It finds
// the programmes first, then their channels (see searchedProgrammes). The
// column is folded by Polyfin, not by PostgreSQL, whose lower case and
// accents differ from Go's: the programmes stored without it, before
// Polyfin kept it, and those an archive keeps, which are copied without,
// are folded by foldGuideTitles after each guide's download and before any
// search reads them, never at an idle guide check, which sends its checks
// only. The search's own test still decides which programmes are kept, as
// for the Native EPG's programmes and those of the channels Polyfin airs.

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

// searchedProgrammes reads the programmes whose folded title holds $7 of the
// channels of $1 to $4 (identifier, guide, guide channel and rank, as
// guidePrograms gives them) that overlap [$5, $6), as guidePrograms reads
// them. It finds the programmes first, in the current downloads of the
// channels' guides, then joins them to the channels: planned with the
// channels first, the read misjudges how many channels the guides hold, and
// for some of the planner's samples of the guide (ANALYZE) probes each
// channel's programmes rather than the trigram index. That took 43 to 47 ms
// instead of 1.2 for a rare word on the bench's guide, in 4 of 24 samples,
// and 1.5 to 8 s for a common one before any sample.
const searchedProgrammes = `WITH found AS MATERIALIZED (
		SELECT p.guide_id, p.xmltv_id, p.starts_at, p.ends_at, p.title, p.subtitle, p.description, p.categories, p.season, p.episode,
			p.icon, p.is_new, p.previously_shown
		FROM live_guide_programmes p JOIN live_guides g ON g.id = p.guide_id AND g.generation = p.generation
		WHERE p.guide_id = ANY($2::uuid[]) AND p.search LIKE $7 AND p.ends_at > $5 AND p.starts_at < $6)
	SELECT c.channel_id, p.guide_id, p.xmltv_id, p.starts_at, p.ends_at, p.title, p.subtitle, p.description,
		p.categories, coalesce(p.season, 0), coalesce(p.episode, 0), p.icon, p.is_new, p.previously_shown
	FROM unnest($1::uuid[], $2::uuid[], $3::text[], $4::int[]) AS c (channel_id, guide_id, xmltv_id, rank)
	JOIN found p ON p.guide_id = c.guide_id AND p.xmltv_id = c.xmltv_id
	ORDER BY p.starts_at, c.rank`

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
