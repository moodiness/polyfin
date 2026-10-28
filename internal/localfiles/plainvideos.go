package localfiles

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
)

// The files a movies or shows folder leaves unmatched are listed in its
// library as plain videos, as Jellyfin lists the files it cannot identify,
// so that a folder whose files match no title does not look empty in
// apps. A plain video is a title of the folder's own, which no metadata
// addon is asked about (see library.PlainIDPrefix): it has no
// description, no rating and no artwork, and its files play as the
// folder's other files do.
//
// In a movies folder, each unit left unmatched, a file or a disc, is a
// movie named as its name was read for the search, with the year it gives,
// else after its file without extension. In a shows folder, each show
// folder left unmatched is a series named as its folder's name was read,
// whose seasons and episodes are those its files' names give; a file whose
// name gives no episode is left out, as it is of a matched show. The other
// movies of a disc, the titles of a show's disc without their episodes and
// the extras without their title are no plain videos: Jellyfin lists a
// disc as one movie and an extra under its title only.
//
// A plain video's identifier is made of its folder's and its unit's path
// (see plainID): it is the same through every scan and restart, and a
// file or show folder renamed or moved is another plain video. Once its
// unit is matched, by a scan or by hand, it is no longer listed, and what
// users did with it follows its files to the title they were matched to,
// as when what a folder holds changes: its items are kept as the files'
// former titles while it is listed (see store and follow).

// plainID is the identifier of the plain video a unit of a folder is while
// it is unmatched: the first half of a SHA-256 hash of the folder's
// identifier and the unit's path.
func plainID(folder accounts.ID, unit string) string {
	sum := sha256.Sum256([]byte(folder.String() + "/" + unit))
	return library.PlainIDPrefix + hex.EncodeToString(sum[:16])
}

// plainFile reports whether a file of a folder of kind, matched to
// stremioID or left unmatched for reason, is listed as a plain video:
// unmatched, but as another movie of a disc, a title of a show's disc
// without its episodes or an extra without its title, and in a shows
// folder an episode.
func plainFile(kind, stremioID, reason string, episodic bool) bool {
	if stremioID != "" || reason == reasonDiscTitle || reason == reasonDiscEpisode || reason == reasonLoneExtra {
		return false
	}
	return kind == KindMovies || kind == KindShows && episodic
}

// plainListed is the condition a file, of alias f in a folder of alias l
// (local_folders), meets while it is listed as a plain video (see
// plainFile): no file of its unit is matched either, as the files of a
// matched show whose names give no episode are not.
const plainListed = `f.stremio_id IS NULL AND f.plain_id IS NOT NULL AND f.reason NOT IN ('disc_title', 'disc_episode', 'lone_extra')
	AND (l.kind = 'movies' OR l.kind = 'shows' AND f.season IS NOT NULL AND f.episode IS NOT NULL)
	AND NOT EXISTS (SELECT 1 FROM local_files m WHERE m.addon_id = f.addon_id AND m.unit = f.unit AND m.stremio_id IS NOT NULL)`

// plainName is the name a plain video of a file, of alias f in a folder of
// alias l, shows: its unit's title as its name was read, else, in a movies
// folder, its file's name without extension, or its disc's folder's name,
// and in a shows folder its show folder's name.
const plainName = `CASE WHEN f.title <> '' THEN f.title
	WHEN l.kind = 'movies' AND f.unit = f.path THEN regexp_replace(regexp_replace(f.unit, '^.*/', ''), '\.[^.]*$', '')
	ELSE regexp_replace(f.unit, '^.*/', '') END`

// plainEpisodeName names an episode of a plain series after the file at
// rel, without its extension, as Jellyfin names an episode it cannot
// identify; a title of a show's disc, as other episodes of discs, by its
// number.
func plainEpisodeName(rel string, number int) string {
	if _, _, ok := showTitle(rel); ok {
		return "Episode " + strconv.Itoa(number)
	}
	return strings.TrimSuffix(path.Base(rel), path.Ext(rel))
}

// LinkPlain links the unit of the plain video plain, a movie or a series
// still listed, to an IMDb identifier, as Link does from the folder's page
// (Find the title): a movie's file or disc, or a show's whole folder. It
// answers addons.ErrNotFound when no folder lists that plain video, as
// once its unit is matched.
func (s *Service) LinkPlain(ctx context.Context, plain, imdb string) error {
	var folder accounts.ID
	var unit string
	err := s.db.QueryRow(ctx, `SELECT f.addon_id, f.unit FROM local_files f JOIN local_folders l ON l.addon_id = f.addon_id
		WHERE f.plain_id = $1 AND `+plainListed+` ORDER BY f.path LIMIT 1`, plain).Scan(&folder, &unit)
	if errors.Is(err, pgx.ErrNoRows) {
		return addons.ErrNotFound
	}
	if err != nil {
		return err
	}
	return s.Link(ctx, folder, unit, imdb)
}

// PlainMatch tells the title the unit of the plain video plain was matched
// to since, by a scan or by hand: its folder and the title's Stremio
// identifier, empty while the unit is unmatched or gone.
func (s *Service) PlainMatch(ctx context.Context, plain string) (accounts.ID, string, error) {
	var folder accounts.ID
	var stremioID string
	err := s.db.QueryRow(ctx, `SELECT addon_id, stremio_id FROM local_files WHERE plain_id = $1 AND stremio_id IS NOT NULL
		ORDER BY path LIMIT 1`, plain).Scan(&folder, &stremioID)
	if errors.Is(err, pgx.ErrNoRows) {
		return accounts.ID{}, "", nil
	}
	return folder, stremioID, err
}
