// Package localfiles serves the files of folders as versions of the titles
// they are: folders mounted in Polyfin's container, and network shares
// Polyfin reads itself, SMB shares and WebDAV folders. An administrator
// declares a folder of movies or of shows; it is an addon of its own kind
// (see addons.KindLocal), as IPTV sources are, which answers as an addon:
// one catalog, a library apps see, of the titles its files were matched
// to, and the streams of those titles, its files, which play beside the
// other addons' streams.
//
// Folders are scanned at startup, every Settings.LocalScanHours and when an
// administrator asks. The folders in the container are also watched for
// changes, unless Settings.WatchLocalFolders is off, each scanned again a
// few seconds after its files stop changing; network shares are not
// watched. A scan reads only what changed since
// the last one, by path, size and modification time; each new file is
// matched to a title by the identifiers its name gives, else by its name
// and year through the metadata addons' searches, kept only when they find
// one title. An administrator links the files left unmatched to an IMDb
// identifier by hand.
//
// A share's files are read from the share at each request, with byte
// ranges, and never copied; its connections are kept between requests.
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
	"github.com/moodiness/polyfin/internal/secrets"
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
	// box seals the shares' passwords.
	box *secrets.Box
	now func() time.Time
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

	// shares are the readers of the folders that are network shares, by
	// folder, kept between requests.
	sharesMu sync.Mutex
	shares   map[accounts.ID]*pooledRemote

	// watch watches the folders in the container for changes, nil until
	// Watch.
	watch *watcher
}

// matchWorkers bounds the units a scan matches at once, each asking the
// metadata addons.
const matchWorkers = 4

// New returns the service. settings gives LocalScanHours, read whenever
// folders due are looked for, and WatchLocalFolders, read whenever the
// folders to watch are; box seals the shares' passwords.
func New(db *pgxpool.Pool, store *addons.Store, titles Titles, logger *slog.Logger, settings func() accounts.Settings,
	box *secrets.Box) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{db: db, addons: store, titles: titles, logger: logger, settings: settings, box: box, now: time.Now,
		workers: matchWorkers, ctx: ctx, cancel: cancel, running: map[accounts.ID]chan struct{}{}, scanned: map[accounts.ID]bool{},
		shares: map[accounts.ID]*pooledRemote{}}
}

// UseFiles sets the address the folders' files are served at, a file's
// key appended (see playback.Service.LocalFiles); it is called before the
// service is used.
func (s *Service) UseFiles(base string) { s.files = base }

// Watch watches the folders in the container for changes until Close; it
// is called once, before the service is used.
func (s *Service) Watch() {
	s.watch = newWatcher(s.ctx, s.logger, s.watchable, s.watchScan, openNotifyWatcher, s.wg.Go)
	s.wg.Go(s.watch.run)
}

// watchable lists the folders to watch.
func (s *Service) watchable(ctx context.Context) ([]watchFolder, error) {
	settings := s.settings()
	if !settings.WatchLocalFolders {
		return nil, nil
	}
	installed, err := s.addons.Addons(ctx, addons.Shared())
	if err != nil {
		return nil, err
	}
	return watchable(settings, installed), nil
}

// watchScan scans a folder that changed, once a scan of it under way
// ended.
func (s *Service) watchScan(ctx context.Context, id accounts.ID) {
	if err := s.Scan(ctx, id); err != nil && ctx.Err() == nil {
		s.logger.Warn("A local folder could not be scanned", "folder", id, "error", err)
	}
}

// Close stops the watch and the scans under way, waits for them, and
// closes the shares' connections.
func (s *Service) Close() {
	s.cancel()
	s.wg.Wait()
	s.forgetRemotes(func(accounts.ID) bool { return false })
}

// NewFolder is a folder to add: its name, its path in the container or a
// share's address, what it holds, KindMovies or KindShows, and a share's
// user and password.
type NewFolder struct {
	Name     string
	Path     string
	Kind     string
	User     string
	Password string
}

// Folder describes a local folder: its addon, what it holds, the kind of
// share it is (ShareSMB, ShareWebDAV, empty for a path in the container)
// with its user and whether a password is stored, how its last scan went
// (CheckedAt is the last attempt, ScannedAt the last scan that could read
// the folder, Error the code of the last failure: "missing",
// "unreadable", "not_folder", or for a share "unreachable" or "refused"),
// whether a scan is under way, its files, those matched to a title and the
// others, and the links made by hand. Unwatched tells why a folder in the
// container Polyfin should watch is not: "watch_limit" when the system's
// limit of watches is reached, "instance_limit" when its limit of watchers
// or of open files is, "failed" otherwise; empty when it is watched or
// need not be.
type Folder struct {
	Addon       addons.Addon
	Kind        string
	Share       string
	User        string
	PasswordSet bool
	CheckedAt   *time.Time
	ScannedAt   *time.Time
	Error       string
	Scanning    bool
	Files       int
	Matched     int
	Unmatched   int
	Links       []Link
	Unwatched   string
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
	location, err := validLocation(folder.Path)
	if err != nil {
		return addons.Addon{}, err
	}
	if folder.Kind != KindMovies && folder.Kind != KindShows {
		return addons.Addon{}, ErrInvalidKind
	}
	share := shareKind(location)
	user, password := "", ""
	if share != "" {
		if user, err = validCredentials(folder.User, folder.Password); err != nil {
			return addons.Addon{}, err
		}
		password = folder.Password
	}
	addon, err := s.addons.Create(ctx, addons.Shared(), addons.KindLocal, location,
		func(id accounts.ID) stremio.Manifest { return manifest(id, name, folder.Kind, share) },
		func(tx pgx.Tx, addon addons.Addon) error {
			// A share is first read by the scan that starts now.
			var checked *time.Time
			failure := ""
			if share == "" {
				at := s.now()
				checked, failure = &at, readable(location)
			}
			_, err := tx.Exec(ctx, `INSERT INTO local_folders (addon_id, kind, checked_at, error, share_user, share_password)
				VALUES ($1, $2, $3, $4, $5, $6)`, addon.ID, folder.Kind, checked, failure, user, s.box.Seal(password))
			return err
		})
	if err != nil {
		return addons.Addon{}, err
	}
	s.StartScan(addon.ID)
	s.watch.kick()
	return addon, nil
}

// Changes are what Update changes of a folder: its name, its path or
// address, a share's user, and its password, removed when empty.
type Changes struct {
	Name     *string
	Path     *string
	User     *string
	Password *string
}

// Update changes a folder's name, path or address, or a share's user or
// password. A new path or address forgets the files found in the former
// one, their links included, and is scanned at once, as a share whose
// user or password changed is. A path in the container keeps no user nor
// password.
func (s *Service) Update(ctx context.Context, id accounts.ID, changes Changes) (addons.Addon, error) {
	current, err := s.Folder(ctx, id)
	if err != nil {
		return addons.Addon{}, err
	}
	name, location := current.Addon.Manifest.Name, current.Addon.ManifestURL
	if changes.Name != nil {
		if name, err = validName(*changes.Name); err != nil {
			return addons.Addon{}, err
		}
	}
	if changes.Path != nil {
		if location, err = validLocation(*changes.Path); err != nil {
			return addons.Addon{}, err
		}
	}
	share := shareKind(location)
	user := current.User
	if changes.User != nil {
		user = *changes.User
	}
	password := ""
	if changes.Password != nil {
		password = *changes.Password
	}
	if user, err = validCredentials(user, password); err != nil {
		return addons.Addon{}, err
	}
	if share == "" {
		user, changes.Password = "", new("")
	}
	moved := location != current.Addon.ManifestURL
	credentials := user != current.User || changes.Password != nil
	addon, err := s.addons.Update(ctx, addons.Shared(), id, addons.KindLocal, location, manifest(id, name, current.Kind, share), func(tx pgx.Tx) error {
		if credentials {
			if _, err := tx.Exec(ctx, "UPDATE local_folders SET share_user = $2 WHERE addon_id = $1", id, user); err != nil {
				return err
			}
			if changes.Password != nil {
				if _, err := tx.Exec(ctx, "UPDATE local_folders SET share_password = $2 WHERE addon_id = $1", id, s.box.Seal(*changes.Password)); err != nil {
					return err
				}
			}
		}
		if !moved {
			return nil
		}
		if _, err := tx.Exec(ctx, "DELETE FROM local_files WHERE addon_id = $1", id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM local_links WHERE addon_id = $1", id); err != nil {
			return err
		}
		var checked *time.Time
		failure := ""
		if share == "" {
			at := s.now()
			checked, failure = &at, readable(location)
		}
		_, err := tx.Exec(ctx, "UPDATE local_folders SET checked_at = $2, scanned_at = NULL, error = $3 WHERE addon_id = $1", id, checked, failure)
		return err
	})
	if err != nil {
		return addons.Addon{}, err
	}
	if moved || share != "" && credentials {
		s.StartScan(id)
	}
	if moved {
		s.watch.kick()
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
	f := Folder{Addon: addon, Share: shareKind(addon.ManifestURL)}
	var password string
	err = s.db.QueryRow(ctx, `SELECT kind, checked_at, scanned_at, error, share_user, share_password,
			(SELECT count(*) FROM local_files WHERE addon_id = $1),
			(SELECT count(*) FROM local_files WHERE addon_id = $1 AND stremio_id IS NOT NULL)
		FROM local_folders WHERE addon_id = $1`, id).Scan(&f.Kind, &f.CheckedAt, &f.ScannedAt, &f.Error, &f.User, &password, &f.Files, &f.Matched)
	f.PasswordSet = password != ""
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
	f.Unwatched = s.watch.reason(id)
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
func manifest(id accounts.ID, name, kind, share string) stremio.Manifest {
	typ := metaType(kind)
	types := []string{typ}
	description := "Local folder"
	switch share {
	case ShareSMB:
		description = "SMB share"
	case ShareWebDAV:
		description = "WebDAV folder"
	}
	return stremio.Manifest{
		ID: "polyfin.local." + id.String(), Version: "1", Name: name, Description: description, Types: types,
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
