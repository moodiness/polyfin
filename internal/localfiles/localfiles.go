// Package localfiles serves the files of folders mounted in Polyfin's
// container as versions of the titles they are. An administrator declares
// a folder of movies or of shows; it is an addon of its own kind (see
// addons.KindLocal), as IPTV sources are, which answers as an addon: one
// catalog, a library apps see, of the titles its files were matched to, and
// the streams of those titles, its files, which play beside the other
// addons' streams.
//
// Folders are scanned at startup, every Settings.LocalScanHours and when an
// administrator asks, never watched. A scan reads only what changed since
// the last one, by path, size and modification time; each new file is
// matched to a title by the identifiers its name gives, else by its name
// and year through the metadata addons' searches, kept only when they find
// one title. An administrator links the files left unmatched to an IMDb
// identifier by hand.
package localfiles

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Kinds of folders: a folder holds movies, or shows with their episodes.
const (
	KindMovies = "movies"
	KindShows  = "shows"
)

// catalogID identifies a folder's one catalog.
const catalogID = "local"

var (
	ErrInvalidName = errors.New("folder names are 1 to 64 printable characters")
	ErrInvalidPath = errors.New("a folder is an absolute path in the container, other than /")
	ErrInvalidKind = errors.New("a folder holds movies or shows")
	ErrInvalidIMDb = errors.New("an IMDb identifier is tt followed by digits")
)

// Titles finds titles in the server's metadata addons: library.Service.
type Titles interface {
	RemoteSearch(ctx context.Context, kind library.Kind, name string, limit int) ([]library.RemoteResult, error)
	TitleMeta(ctx context.Context, metaType, id string) (stremio.Meta, bool)
}

// Service stores the local folders, scans them and answers for them as
// addons.
type Service struct {
	db       *pgxpool.Pool
	addons   *addons.Store
	titles   Titles
	logger   *slog.Logger
	settings func() accounts.Settings
	now      func() time.Time
	// files is the address the loopback interface serves the folders'
	// files at, a file's key appended (see UseFiles); empty, folders list
	// no stream.
	files string
	// workers bounds the units matched at once in a scan.
	workers int

	// ctx ends the scans started in the background, which wg waits for.
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu sync.Mutex
	// running are the scans under way, by folder, each closed when it
	// ends; scanned the folders scanned since the service started.
	running map[accounts.ID]chan struct{}
	scanned map[accounts.ID]bool
}

// matchWorkers bounds the units a scan matches at once, each asking the
// metadata addons.
const matchWorkers = 4

// New returns the service. settings gives LocalScanHours, read whenever
// folders due are looked for.
func New(db *pgxpool.Pool, store *addons.Store, titles Titles, logger *slog.Logger, settings func() accounts.Settings) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{db: db, addons: store, titles: titles, logger: logger, settings: settings, now: time.Now, workers: matchWorkers,
		ctx: ctx, cancel: cancel, running: map[accounts.ID]chan struct{}{}, scanned: map[accounts.ID]bool{}}
}

// UseFiles sets the address the folders' files are served at, a file's
// key appended (see playback.Service.LocalFiles); it is called before the
// service is used.
func (s *Service) UseFiles(base string) { s.files = base }

// Close stops the scans under way and waits for them.
func (s *Service) Close() {
	s.cancel()
	s.wg.Wait()
}

// NewFolder is a folder to add: its name, its path in the container and
// what it holds, KindMovies or KindShows.
type NewFolder struct {
	Name string
	Path string
	Kind string
}

// Folder describes a local folder: its addon, what it holds, how its last
// scan went (CheckedAt is the last attempt, ScannedAt the last scan that
// could read the folder, Error the code of the last failure: "missing",
// "unreadable" or "not_folder"), whether a scan is under way, its files,
// those matched to a title and the others, and the links made by hand.
type Folder struct {
	Addon     addons.Addon
	Kind      string
	CheckedAt *time.Time
	ScannedAt *time.Time
	Error     string
	Scanning  bool
	Files     int
	Matched   int
	Unmatched int
	Links     []Link
}

// Link is a unit of a folder linked by hand to an IMDb identifier: a
// file, or a show's folder.
type Link struct {
	Unit string
	IMDb string
}

// Add adds a folder to the server's addons, its catalog enabled as a
// library, and scans it in the background. A folder Polyfin cannot read
// is added all the same, its failure reported: a share mounted later is
// read at the next scan.
func (s *Service) Add(ctx context.Context, folder NewFolder) (addons.Addon, error) {
	name, err := validName(folder.Name)
	if err != nil {
		return addons.Addon{}, err
	}
	path, err := validPath(folder.Path)
	if err != nil {
		return addons.Addon{}, err
	}
	if folder.Kind != KindMovies && folder.Kind != KindShows {
		return addons.Addon{}, ErrInvalidKind
	}
	addon, err := s.addons.Create(ctx, addons.Shared(), addons.KindLocal, path,
		func(id accounts.ID) stremio.Manifest { return manifest(id, name, folder.Kind) },
		func(tx pgx.Tx, addon addons.Addon) error {
			_, err := tx.Exec(ctx, "INSERT INTO local_folders (addon_id, kind, checked_at, error) VALUES ($1, $2, $3, $4)",
				addon.ID, folder.Kind, s.now(), readable(path))
			return err
		})
	if err != nil {
		return addons.Addon{}, err
	}
	s.StartScan(addon.ID)
	return addon, nil
}

// Changes are what Update changes of a folder: its name, its path.
type Changes struct {
	Name *string
	Path *string
}

// Update changes a folder's name or path. A new path forgets the files
// found in the former one, their links included, and is scanned at once.
func (s *Service) Update(ctx context.Context, id accounts.ID, changes Changes) (addons.Addon, error) {
	current, err := s.Folder(ctx, id)
	if err != nil {
		return addons.Addon{}, err
	}
	name, path := current.Addon.Manifest.Name, current.Addon.ManifestURL
	if changes.Name != nil {
		if name, err = validName(*changes.Name); err != nil {
			return addons.Addon{}, err
		}
	}
	if changes.Path != nil {
		if path, err = validPath(*changes.Path); err != nil {
			return addons.Addon{}, err
		}
	}
	moved := path != current.Addon.ManifestURL
	addon, err := s.addons.Update(ctx, addons.Shared(), id, addons.KindLocal, path, manifest(id, name, current.Kind), func(tx pgx.Tx) error {
		if !moved {
			return nil
		}
		if _, err := tx.Exec(ctx, "DELETE FROM local_files WHERE addon_id = $1", id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM local_links WHERE addon_id = $1", id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE local_folders SET checked_at = $2, scanned_at = NULL, error = $3 WHERE addon_id = $1", id, s.now(), readable(path))
		return err
	})
	if err != nil {
		return addons.Addon{}, err
	}
	if moved {
		s.StartScan(id)
	}
	return addon, nil
}

// Folder describes one of the server's local folders.
func (s *Service) Folder(ctx context.Context, id accounts.ID) (Folder, error) {
	addon, err := s.addons.Find(ctx, id)
	if err != nil {
		return Folder{}, err
	}
	if !addon.Local() {
		return Folder{}, addons.ErrNotFound
	}
	f := Folder{Addon: addon}
	err = s.db.QueryRow(ctx, `SELECT kind, checked_at, scanned_at, error,
			(SELECT count(*) FROM local_files WHERE addon_id = $1),
			(SELECT count(*) FROM local_files WHERE addon_id = $1 AND stremio_id IS NOT NULL)
		FROM local_folders WHERE addon_id = $1`, id).Scan(&f.Kind, &f.CheckedAt, &f.ScannedAt, &f.Error, &f.Files, &f.Matched)
	if errors.Is(err, pgx.ErrNoRows) {
		return Folder{}, addons.ErrNotFound
	}
	if err != nil {
		return Folder{}, err
	}
	f.Unmatched = f.Files - f.Matched
	rows, err := s.db.Query(ctx, "SELECT unit, imdb_id FROM local_links WHERE addon_id = $1 ORDER BY unit", id)
	if err != nil {
		return Folder{}, err
	}
	f.Links, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Link, error) {
		var l Link
		err := row.Scan(&l.Unit, &l.IMDb)
		return l, err
	})
	if err != nil {
		return Folder{}, err
	}
	s.mu.Lock()
	_, f.Scanning = s.running[id]
	s.mu.Unlock()
	return f, nil
}

// metaType is the Stremio type of a folder's titles.
func metaType(kind string) string {
	if kind == KindShows {
		return "series"
	}
	return "movie"
}

// idPrefixes are the identifiers a folder's titles have: IMDb's, or TMDB's
// for a title the metadata addons gave no IMDb identifier.
var idPrefixes = []string{"tt", "tmdb:"}

// manifest describes a folder as an addon: one catalog of its titles,
// searchable, and the descriptions and streams of their identifiers.
func manifest(id accounts.ID, name, kind string) stremio.Manifest {
	typ := metaType(kind)
	types := []string{typ}
	return stremio.Manifest{
		ID: "polyfin.local." + id.String(), Version: "1", Name: name, Description: "Local folder", Types: types,
		Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", Types: types, IDPrefixes: idPrefixes},
			{Name: "stream", Types: types, IDPrefixes: idPrefixes}},
		Catalogs: []stremio.Catalog{{Type: typ, ID: catalogID, Name: name, Extra: []stremio.Extra{{Name: "search"}, {Name: "skip"}}}},
	}
}

func validName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return "", ErrInvalidName
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return "", ErrInvalidName
		}
	}
	return name, nil
}

// validPath cleans a folder's path, which must be absolute, other than the
// root, and printable.
func validPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) || len(path) > 4096 || strings.ContainsFunc(path, func(r rune) bool { return !unicode.IsPrint(r) }) {
		return "", ErrInvalidPath
	}
	path = filepath.Clean(path)
	if path == string(filepath.Separator) {
		return "", ErrInvalidPath
	}
	return path, nil
}
