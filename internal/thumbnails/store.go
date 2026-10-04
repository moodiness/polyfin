package thumbnails

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// SaveTrickplay keeps a version's thumbnails at info.Width, replacing
// those it had at that width. item is the title the version belongs to.
func (s *Service) SaveTrickplay(ctx context.Context, version, item accounts.ID, info Info, tiles [][]byte) error {
	err := pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		if err := ensureVersion(ctx, tx, version, item); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM trickplay_sets WHERE version_id = $1 AND width = $2", version, info.Width); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO trickplay_sets (version_id, width, height, tile_width, tile_height, thumbnail_count, interval_ms, bandwidth)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			version, info.Width, info.Height, info.TileWidth, info.TileHeight, info.ThumbnailCount, info.Interval, info.Bandwidth); err != nil {
			return err
		}
		rows := make([][]any, len(tiles))
		for i, tile := range tiles {
			rows[i] = []any{version, info.Width, i, tile}
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"trickplay_tiles"}, []string{"version_id", "width", "tile", "data"}, pgx.CopyFromRows(rows)); err != nil {
			return err
		}
		return countBytes(ctx, tx, version)
	})
	if err != nil {
		return err
	}
	return s.evict(ctx)
}

// SaveChapterImages keeps the images of a version's chapters, in their
// order, replacing those it had.
func (s *Service) SaveChapterImages(ctx context.Context, version, item accounts.ID, images [][]byte) error {
	made := time.Now().UTC()
	err := pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		if err := ensureVersion(ctx, tx, version, item); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM chapter_images WHERE version_id = $1", version); err != nil {
			return err
		}
		rows := make([][]any, len(images))
		for i, image := range images {
			rows[i] = []any{version, i, imageTag(version, i, made), image, made}
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"chapter_images"}, []string{"version_id", "chapter", "tag", "data", "made_at"}, pgx.CopyFromRows(rows)); err != nil {
			return err
		}
		return countBytes(ctx, tx, version)
	})
	if err != nil {
		return err
	}
	return s.evict(ctx)
}

// imageTag names a chapter image, changing when it is made again.
func imageTag(version accounts.ID, chapter int, made time.Time) string {
	sum := sha256.Sum256([]byte("polyfin:chapter:" + version.String() + ":" + strconv.Itoa(chapter) + ":" + strconv.FormatInt(made.UnixNano(), 10)))
	return hex.EncodeToString(sum[:16])
}

func ensureVersion(ctx context.Context, tx pgx.Tx, version, item accounts.ID) error {
	_, err := tx.Exec(ctx, `INSERT INTO thumbnail_versions (version_id, item_id) VALUES ($1, $2)
		ON CONFLICT (version_id) DO UPDATE SET item_id = excluded.item_id, used_at = now()`, version, item)
	return err
}

// countBytes records the bytes a version's images take.
func countBytes(ctx context.Context, tx pgx.Tx, version accounts.ID) error {
	_, err := tx.Exec(ctx, `UPDATE thumbnail_versions SET bytes =
		(SELECT coalesce(sum(octet_length(data)), 0) FROM trickplay_tiles WHERE version_id = $1) +
		(SELECT coalesce(sum(octet_length(data)), 0) FROM chapter_images WHERE version_id = $1)
		WHERE version_id = $1`, version)
	return err
}

// evict drops the images of the versions used longest ago while all of
// them take more than the settings' ThumbnailStorageGB.
func (s *Service) evict(ctx context.Context) error {
	limit := int64(s.Settings().ThumbnailStorageGB) << 30
	tag, err := s.DB.Exec(ctx, `DELETE FROM thumbnail_versions WHERE version_id IN (
		SELECT version_id FROM (
			SELECT version_id, sum(bytes) OVER (ORDER BY used_at DESC, version_id) AS reached FROM thumbnail_versions
		) ranked WHERE reached > $1)`, limit)
	if err == nil && tag.RowsAffected() > 0 {
		s.Logger.Info("The images of the versions used longest ago were dropped to keep within their space", "versions", tag.RowsAffected())
	}
	return err
}

// touch records that a version's images were used, at most every
// touchEvery: the storage cap keeps those used last.
func (s *Service) touch(ctx context.Context, version accounts.ID) {
	if _, ok := s.touched.Get(version); ok {
		return
	}
	s.touched.Put(version, struct{}{})
	if _, err := s.DB.Exec(ctx, "UPDATE thumbnail_versions SET used_at = now() WHERE version_id = $1", version); err != nil && ctx.Err() == nil {
		s.Logger.Warn("Recording the use of a version's images failed", "error", err)
	}
}

// Manifest returns the thumbnails of a title's versions, by version and
// width, when the settings turn them on.
func (s *Service) Manifest(ctx context.Context, item accounts.ID) (map[accounts.ID]map[int]Info, error) {
	if !s.Settings().Trickplay {
		return nil, nil
	}
	rows, err := s.DB.Query(ctx, `SELECT t.version_id, t.width, t.height, t.tile_width, t.tile_height, t.thumbnail_count, t.interval_ms, t.bandwidth
		FROM trickplay_sets t JOIN thumbnail_versions v USING (version_id) WHERE v.item_id = $1`, item)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	manifest := map[accounts.ID]map[int]Info{}
	for rows.Next() {
		var version accounts.ID
		var info Info
		if err := rows.Scan(&version, &info.Width, &info.Height, &info.TileWidth, &info.TileHeight, &info.ThumbnailCount, &info.Interval, &info.Bandwidth); err != nil {
			return nil, err
		}
		if manifest[version] == nil {
			manifest[version] = map[int]Info{}
		}
		manifest[version][info.Width] = info
	}
	return manifest, rows.Err()
}

// ErrNotFound reports images that do not exist, or that the settings
// turned off.
var ErrNotFound = errors.New("no such images")

// Trickplay returns a title's thumbnails at width: those of version, or
// when it is zero, of the title's version used last.
func (s *Service) Trickplay(ctx context.Context, item, version accounts.ID, width int) (accounts.ID, Info, error) {
	if !s.Settings().Trickplay {
		return accounts.ID{}, Info{}, ErrNotFound
	}
	var info Info
	var found accounts.ID
	err := s.DB.QueryRow(ctx, `SELECT t.version_id, t.width, t.height, t.tile_width, t.tile_height, t.thumbnail_count, t.interval_ms, t.bandwidth
		FROM trickplay_sets t JOIN thumbnail_versions v USING (version_id)
		WHERE v.item_id = $1 AND t.width = $2 AND (t.version_id = $3 OR $3 = '00000000-0000-0000-0000-000000000000'::uuid)
		ORDER BY v.used_at DESC LIMIT 1`, item, width, version).
		Scan(&found, &info.Width, &info.Height, &info.TileWidth, &info.TileHeight, &info.ThumbnailCount, &info.Interval, &info.Bandwidth)
	if errors.Is(err, pgx.ErrNoRows) {
		return accounts.ID{}, Info{}, ErrNotFound
	}
	if err != nil {
		return accounts.ID{}, Info{}, err
	}
	s.touch(ctx, found)
	return found, info, nil
}

// Tile returns a tile of a version's thumbnails at width.
func (s *Service) Tile(ctx context.Context, version accounts.ID, width, index int) ([]byte, error) {
	if !s.Settings().Trickplay {
		return nil, ErrNotFound
	}
	var data []byte
	err := s.DB.QueryRow(ctx, "SELECT data FROM trickplay_tiles WHERE version_id = $1 AND width = $2 AND tile = $3", version, width, index).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s.touch(ctx, version)
	return data, nil
}

// ChapterImage is a chapter's image.
type ChapterImage struct {
	Tag  string
	Made time.Time
	// Data is the JPEG image, when asked for.
	Data []byte
}

// ChapterImages returns the images of a version's chapters, by chapter,
// when the settings turn them on.
func (s *Service) ChapterImages(ctx context.Context, version accounts.ID) (map[int]ChapterImage, error) {
	if !s.Settings().ChapterImages {
		return nil, nil
	}
	rows, err := s.DB.Query(ctx, "SELECT chapter, tag, made_at FROM chapter_images WHERE version_id = $1", version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	images := map[int]ChapterImage{}
	for rows.Next() {
		var chapter int
		var image ChapterImage
		if err := rows.Scan(&chapter, &image.Tag, &image.Made); err != nil {
			return nil, err
		}
		images[chapter] = image
	}
	return images, rows.Err()
}

// ChapterImageOf returns the image of a chapter of id, a version or a
// title. A title's is that of the version whose image has tag, else of its
// version used last.
func (s *Service) ChapterImageOf(ctx context.Context, id accounts.ID, chapter int, tag string) (ChapterImage, error) {
	if !s.Settings().ChapterImages {
		return ChapterImage{}, ErrNotFound
	}
	var image ChapterImage
	var version accounts.ID
	err := s.DB.QueryRow(ctx, `SELECT c.version_id, c.tag, c.made_at, c.data
		FROM chapter_images c JOIN thumbnail_versions v USING (version_id)
		WHERE (v.version_id = $1 OR v.item_id = $1) AND c.chapter = $2
		ORDER BY v.version_id = $1 DESC, c.tag = $3 DESC, v.used_at DESC LIMIT 1`, id, chapter, tag).
		Scan(&version, &image.Tag, &image.Made, &image.Data)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChapterImage{}, ErrNotFound
	}
	if err != nil {
		return ChapterImage{}, err
	}
	s.touch(ctx, version)
	return image, nil
}
