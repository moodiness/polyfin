// Package themecopy keeps a copy of the files the web player's Custom CSS
// and Custom JavaScript load from elsewhere, such as a theme on a CDN, in
// the data folder's themes folder, and rewrites the texts to the addresses
// the web client serves the copy at (see Path), so that the theme works
// without reaching its CDN. It follows the stylesheets' @import and url()
// and the addresses scripts name, to a limited depth, and keeps the
// addresses the texts named, so that the copy can be made again from them,
// or removed, which puts them back.
package themecopy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Path is where the web client serves a copy's files, under /web/: Path,
// the copy's id, a slash and the file's name. The texts name them so,
// relative to the web client's address, as the bundled theme's are.
const Path = "polyfin/themes/saved/"

// The limits of a copy. The largest theme files seen are an icon font of
// 5.3 MB, a script of 700 KB and a stylesheet of 540 KB, the largest theme
// about 6 MB in 31 files, and the deepest chains of @import three levels:
// each limit leaves room above them, and keeps a copy far from filling the
// data folder.
const (
	MaxFileBytes  = 10_000_000
	MaxTotalBytes = 32_000_000
	MaxFiles      = 100
	MaxDepth      = 4
	// FileTimeout bounds each file, from asking to its last byte, and
	// CopyTimeout the whole copy, which the admin app waits for.
	FileTimeout = 30 * time.Second
	CopyTimeout = 2 * time.Minute
	// maxRedirects bounds the redirects of one file, each checked.
	maxRedirects = 5
)

// Why an address was not saved, as the admin API tells it.
const (
	// ReasonPrivate is an address on a local network, or redirected to
	// one: only addresses on the internet are asked.
	ReasonPrivate = "private_address"
	// ReasonNotHTTP is a redirect to an address neither http nor https.
	ReasonNotHTTP = "not_http"
	// ReasonUnavailable is an address that answered no file: an error
	// status, or no answer; Detail tells which.
	ReasonUnavailable = "unavailable"
	ReasonTimeout     = "timeout"
	// ReasonWrongType is a file that is not a stylesheet, a script, a
	// font or an image, or not the one its place asks for, such as a page
	// where a stylesheet is imported; Detail is the type it was sent as.
	ReasonWrongType = "wrong_type"
	ReasonTooLarge  = "too_large"
	// ReasonTotalTooLarge and ReasonTooManyFiles are files past the
	// copy's limits, ReasonTooDeep one past MaxDepth.
	ReasonTotalTooLarge = "total_too_large"
	ReasonTooManyFiles  = "too_many_files"
	ReasonTooDeep       = "too_deep"
	// ReasonBuiltAtRunTime is an address a script completes when it runs,
	// such as a folder it adds file names to: no copy can follow it.
	ReasonBuiltAtRunTime = "built_at_run_time"
)

// The kinds of files a copy keeps.
const (
	KindStylesheet = "stylesheet"
	KindScript     = "script"
	KindFont       = "font"
	KindImage      = "image"
)

var (
	// ErrRunning reports a copy being made or removed already.
	ErrRunning = errors.New("a copy of the theme is being made")
	// ErrNothingToSave reports Custom texts that load nothing from
	// elsewhere.
	ErrNothingToSave = errors.New("the custom CSS and JavaScript load nothing from elsewhere")
	// ErrNoCopy reports there is no copy to remove.
	ErrNoCopy = errors.New("no copy of the theme is kept")
)

// NothingSavedError reports a copy that could save none of the files the
// texts load, with why for each: no copy is made.
type NothingSavedError struct {
	Unsaved []Unsaved
}

func (e *NothingSavedError) Error() string {
	return fmt.Sprintf("none of the %d files could be saved", len(e.Unsaved))
}

// File is a file of a copy: the address it came from, the text naming it
// in the Custom texts when they do, its name in the copy's folder, its
// kind and its size in bytes.
type File struct {
	Address string `json:"address"`
	Text    string `json:"text,omitempty"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Size    int64  `json:"size"`
}

// Unsaved is an address a copy could not save, and why (the Reason
// constants), with a detail for some.
type Unsaved struct {
	Address string `json:"address"`
	Reason  string `json:"reason"`
	Detail  string `json:"detail,omitempty"`
}

// Copy is the copy kept: its id, which names its folder and changes with
// each copy made, so that browsers keep its files for good, when it was
// made, its files and the addresses it could not save.
type Copy struct {
	ID      string
	SavedAt time.Time
	Files   []File
	Unsaved []Unsaved
}

// Size is the size of the copy's files, in bytes.
func (c Copy) Size() int64 {
	var size int64
	for _, file := range c.Files {
		size += file.Size
	}
	return size
}

// Status is the copy kept, nil for none, the names of its files missing
// from its folder, such as after a restore on another system, whether the
// Custom texts load the copy's files, and the addresses they load from
// elsewhere.
type Status struct {
	Copy    *Copy
	Missing []string
	InUse   bool
	Outside []string
}

// Options are what a Copier needs: the database, the settings, the folder
// copies are kept in (the data folder's themes), and the client asking
// for the files, nil for one reaching addresses on the internet only.
type Options struct {
	DB        *pgxpool.Pool
	Accounts  *accounts.Store
	Dir       string
	Client    *http.Client
	UserAgent string
	Logger    *slog.Logger
}

// Copier makes, remakes and removes the copy, one at a time.
type Copier struct {
	options Options
	running sync.Mutex
	// current is the copy kept, nil for none, which Original answers from
	// without asking the database: it changes with Keep and Remove only,
	// and a restored backup starts Polyfin again.
	current atomic.Pointer[Copy]
}

// New returns a Copier, after removing the folders of copies no longer
// kept, which a stop in the middle of a copy can leave.
func New(ctx context.Context, options Options) (*Copier, error) {
	if options.Client == nil {
		options.Client = newClient(func(address netip.AddrPort) bool { return stremio.Public(address.Addr()) })
	}
	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}
	c := &Copier{options: options}
	current, err := c.load(ctx, c.options.DB)
	if err != nil {
		return nil, err
	}
	c.current.Store(current)
	entries, err := os.ReadDir(options.Dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, entry := range entries {
		if validID.MatchString(entry.Name()) && (current == nil || entry.Name() != current.ID) {
			if err := os.RemoveAll(filepath.Join(options.Dir, entry.Name())); err != nil {
				options.Logger.Warn("A former copy of the web player theme could not be removed", "folder", entry.Name(), "error", err)
			}
		}
	}
	return c, nil
}

// newClient returns a client that connects only to the addresses allowed
// accepts, checked at each connection, so that neither a DNS answer nor a
// redirect leads it elsewhere, follows http and https redirects only, and
// goes through no proxy, which would be the address checked instead.
func newClient(allowed func(netip.AddrPort) bool) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	dialer.Control = func(_, address string, _ syscall.RawConn) error {
		parsed, err := netip.ParseAddrPort(address)
		if err != nil || !allowed(parsed) {
			return stremio.ErrPrivateNetwork
		}
		return nil
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = dialer.DialContext
	transport.ResponseHeaderTimeout = FileTimeout
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if request.URL.Scheme != "http" && request.URL.Scheme != "https" {
				return errNotHTTP
			}
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
}

var errNotHTTP = errors.New("a redirect to an address neither http nor https")

var (
	validID   = regexp.MustCompile(`^[0-9a-f]{16}$`)
	validName = regexp.MustCompile(`^[0-9]{3}-[A-Za-z0-9_-]{1,40}\.(css|js|woff2|woff|ttf|otf|eot|png|jpg|gif|webp|avif|svg|ico)$`)
	// savedAddress matches an address of a copy's file in a text.
	savedAddress = regexp.MustCompile(regexp.QuoteMeta(Path) + `([0-9a-f]{16})/([0-9]{3}-[A-Za-z0-9_-]{1,40}\.[a-z0-9]+)`)
)

// OpenFile opens the file rest names under dir, rest being a copy's id, a
// slash and a file's name, as Path serves them; any other name, or one
// leading out of dir, answers os.ErrNotExist.
func OpenFile(dir, rest string) (*os.File, error) {
	id, name, ok := strings.Cut(rest, "/")
	if dir == "" || !ok || !validID.MatchString(id) || !validName.MatchString(name) {
		return nil, os.ErrNotExist
	}
	file, err := os.OpenInRoot(dir, id+"/"+name)
	if err != nil {
		return nil, os.ErrNotExist
	}
	return file, nil
}

// ContentTypes are the types the files of a copy are served as, by their
// extension.
var ContentTypes = map[string]string{
	".css":   "text/css; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".woff2": "font/woff2",
	".woff":  "font/woff",
	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".eot":   "application/vnd.ms-fontobject",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
	".avif":  "image/avif",
	".svg":   "image/svg+xml",
	".ico":   "image/x-icon",
}

// Status returns the copy kept, the files it misses and what the Custom
// texts load from elsewhere.
func (c *Copier) Status(ctx context.Context) (Status, error) {
	current, err := c.load(ctx, c.options.DB)
	if err != nil {
		return Status{}, err
	}
	settings := c.options.Accounts.Settings()
	status := Status{Copy: current, Missing: []string{}, Outside: outside(settings.CustomCss, settings.CustomJs)}
	if current != nil {
		named := Path + current.ID + "/"
		status.InUse = strings.Contains(settings.CustomCss, named) || strings.Contains(settings.CustomJs, named)
		for _, file := range current.Files {
			if _, err := os.Stat(filepath.Join(c.options.Dir, current.ID, file.Name)); err != nil {
				status.Missing = append(status.Missing, file.Name)
			}
		}
	}
	return status, nil
}

// Original returns the address the file name of the copy id came from,
// for the copy kept only, and false for any other file. While a file is
// missing from the copy's folder, as after a backup restored on another
// system, the web client sends browsers there, so that the theme keeps
// working while the internet is reachable.
func (c *Copier) Original(id, name string) (string, bool) {
	kept := c.current.Load()
	if kept == nil || kept.ID != id {
		return "", false
	}
	for _, file := range kept.Files {
		if file.Name == name {
			parsed, err := url.Parse(file.Address)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return "", false
			}
			return file.Address, true
		}
	}
	return "", false
}

// outside returns the addresses css and js load from elsewhere, each once,
// in the order they name them.
func outside(css, js string) []string {
	seen := map[string]bool{}
	addresses := []string{}
	for _, ref := range append(cssReferences(css, nil), scriptReferences(js)...) {
		if !seen[ref.address] {
			seen[ref.address] = true
			addresses = append(addresses, ref.address)
		}
	}
	return addresses
}

// Keep makes a copy of the files the Custom texts load from elsewhere, or
// makes the copy kept again, from the addresses it came from, as a new
// one: the texts then name the new copy's files, and the former copy's
// folder is removed. A copy saving no file is not made; the texts are
// left as they were.
func (c *Copier) Keep(ctx context.Context) (Status, error) {
	if !c.running.TryLock() {
		return Status{}, ErrRunning
	}
	defer c.running.Unlock()
	ctx, cancel := context.WithTimeout(ctx, CopyTimeout)
	defer cancel()
	former, err := c.load(ctx, c.options.DB)
	if err != nil {
		return Status{}, err
	}
	settings := c.options.Accounts.Settings()
	wasCss, wasJs := settings.CustomCss, settings.CustomJs
	css, js := restore(wasCss, former), restore(wasJs, former)
	cssRefs, jsRefs := cssReferences(css, nil), scriptReferences(js)
	if len(cssRefs)+len(jsRefs) == 0 {
		return Status{}, ErrNothingToSave
	}
	id, err := newID()
	if err != nil {
		return Status{}, err
	}
	run := &copyRun{copier: c, id: id, saved: map[string]*saved{}, decided: map[string]bool{}}
	for _, ref := range append(append([]reference{}, cssRefs...), jsRefs...) {
		run.ask(ref, 0)
	}
	run.fetchAll(ctx)
	if len(run.files) == 0 {
		return Status{}, &NothingSavedError{Unsaved: run.unsaved}
	}
	dir := filepath.Join(c.options.Dir, id)
	if err := run.write(dir); err != nil {
		_ = os.RemoveAll(dir)
		return Status{}, err
	}
	texts := map[string]string{}
	addressOf := func(ref reference) string {
		file := run.saved[ref.address]
		if file == nil {
			return ""
		}
		if _, ok := texts[ref.address]; !ok {
			texts[ref.address] = ref.raw
		}
		return Path + id + "/" + file.name + fragment(ref.raw)
	}
	newCss, newJs := rewrite(css, cssRefs, addressOf), rewrite(js, jsRefs, addressOf)
	made := Copy{ID: id, SavedAt: time.Now().UTC().Truncate(time.Millisecond), Unsaved: run.unsaved}
	for _, file := range run.files {
		made.Files = append(made.Files, File{Address: file.address, Text: texts[file.address], Name: file.name, Kind: file.kind, Size: file.size})
	}
	_, err = c.options.Accounts.ReplaceCustomTexts(ctx, wasCss, wasJs, newCss, newJs, func(tx pgx.Tx) error {
		return store(ctx, tx, made)
	})
	if err != nil {
		_ = os.RemoveAll(dir)
		return Status{}, err
	}
	c.current.Store(&made)
	if former != nil {
		c.removeFolder(former.ID)
	}
	c.options.Logger.Info("A copy of the web player theme was kept", "files", len(made.Files), "bytes", made.Size(), "unsaved", len(made.Unsaved))
	return c.Status(ctx)
}

// Remove removes the copy kept: the texts name again the addresses its
// files came from, and its folder is removed.
func (c *Copier) Remove(ctx context.Context) (Status, error) {
	if !c.running.TryLock() {
		return Status{}, ErrRunning
	}
	defer c.running.Unlock()
	current, err := c.load(ctx, c.options.DB)
	if err != nil {
		return Status{}, err
	}
	if current == nil {
		return Status{}, ErrNoCopy
	}
	settings := c.options.Accounts.Settings()
	_, err = c.options.Accounts.ReplaceCustomTexts(ctx, settings.CustomCss, settings.CustomJs,
		restore(settings.CustomCss, current), restore(settings.CustomJs, current), func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "DELETE FROM theme_copy")
			return err
		})
	if err != nil {
		return Status{}, err
	}
	c.current.Store(nil)
	c.removeFolder(current.ID)
	c.options.Logger.Info("The copy of the web player theme was removed", "files", len(current.Files))
	return c.Status(ctx)
}

func (c *Copier) removeFolder(id string) {
	if err := os.RemoveAll(filepath.Join(c.options.Dir, id)); err != nil {
		c.options.Logger.Warn("The former copy of the web player theme could not be removed", "folder", id, "error", err)
	}
}

// restore returns text with the addresses of the files of copy it names
// put back to what the Custom texts named them by.
func restore(text string, kept *Copy) string {
	if kept == nil {
		return text
	}
	byName := map[string]string{}
	for _, file := range kept.Files {
		if file.Text != "" {
			byName[file.Name] = file.Text
		}
	}
	return savedAddress.ReplaceAllStringFunc(text, func(match string) string {
		parts := savedAddress.FindStringSubmatch(match)
		if original, ok := byName[parts[2]]; ok && parts[1] == kept.ID {
			return original
		}
		return match
	})
}

func (c *Copier) load(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (*Copy, error) {
	var kept Copy
	var files, unsaved []byte
	err := db.QueryRow(ctx, "SELECT id, saved_at, files, unsaved FROM theme_copy").Scan(&kept.ID, &kept.SavedAt, &files, &unsaved)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(files, &kept.Files); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(unsaved, &kept.Unsaved); err != nil {
		return nil, err
	}
	return &kept, nil
}

func store(ctx context.Context, tx pgx.Tx, kept Copy) error {
	if kept.Unsaved == nil {
		kept.Unsaved = []Unsaved{}
	}
	files, err := json.Marshal(kept.Files)
	if err != nil {
		return err
	}
	unsaved, err := json.Marshal(kept.Unsaved)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO theme_copy (id, saved_at, files, unsaved) VALUES ($1, $2, $3, $4)
		ON CONFLICT (singleton) DO UPDATE SET id = EXCLUDED.id, saved_at = EXCLUDED.saved_at, files = EXCLUDED.files, unsaved = EXCLUDED.unsaved`,
		kept.ID, kept.SavedAt, files, unsaved)
	return err
}

func newID() (string, error) {
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}

// saved is a file a copy saved: the address asked, the address it was
// served from after redirects, which its relative addresses resolve
// against, its kind, name and content, and the references it holds.
type saved struct {
	address string
	final   *url.URL
	kind    string
	ext     string
	name    string
	body    []byte
	size    int64
	refs    []reference
}

// copyRun is one copy being made: the files saved, in the order they were
// asked, the addresses decided, saved or not, and those not saved.
type copyRun struct {
	copier  *Copier
	id      string
	queue   []queued
	files   []*saved
	saved   map[string]*saved
	decided map[string]bool
	unsaved []Unsaved
	total   int64
}

type queued struct {
	ref   reference
	depth int
}

func (r *copyRun) ask(ref reference, depth int) {
	if r.decided[ref.address] {
		return
	}
	r.decided[ref.address] = true
	switch {
	case ref.builtAtRunTime:
		r.refuse(ref.address, ReasonBuiltAtRunTime, "")
	case depth > MaxDepth:
		r.refuse(ref.address, ReasonTooDeep, "")
	default:
		r.queue = append(r.queue, queued{ref: ref, depth: depth})
	}
}

func (r *copyRun) refuse(address, reason, detail string) {
	r.unsaved = append(r.unsaved, Unsaved{Address: address, Reason: reason, Detail: detail})
}

// fetchAll downloads the files asked, breadth first, and asks for those
// their stylesheets and scripts name, one level deeper.
func (r *copyRun) fetchAll(ctx context.Context) {
	for len(r.queue) > 0 {
		next := r.queue[0]
		r.queue = r.queue[1:]
		if len(r.files) >= MaxFiles {
			r.refuse(next.ref.address, ReasonTooManyFiles, "")
			continue
		}
		file, reason, detail := r.fetch(ctx, next.ref)
		if file == nil {
			r.refuse(next.ref.address, reason, detail)
			continue
		}
		if r.total+int64(len(file.body)) > MaxTotalBytes {
			r.refuse(next.ref.address, ReasonTotalTooLarge, "")
			continue
		}
		r.total += int64(len(file.body))
		file.name = fmt.Sprintf("%03d-%s%s", len(r.files)+1, baseName(file.final.Path), file.ext)
		r.files = append(r.files, file)
		r.saved[file.address] = file
		switch file.kind {
		case KindStylesheet:
			file.refs = cssReferences(string(file.body), file.final)
		case KindScript:
			file.refs = scriptReferences(string(file.body))
		}
		for _, ref := range file.refs {
			r.ask(ref, next.depth+1)
		}
	}
}

// fetch downloads one file, and returns it, or why it was not saved.
func (r *copyRun) fetch(ctx context.Context, ref reference) (*saved, string, string) {
	ctx, cancel := context.WithTimeout(ctx, FileTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, ref.address, nil)
	if err != nil {
		return nil, ReasonUnavailable, "invalid address"
	}
	if r.copier.options.UserAgent != "" {
		request.Header.Set("User-Agent", r.copier.options.UserAgent)
	}
	response, err := r.copier.options.Client.Do(request)
	if err != nil {
		switch {
		case errors.Is(err, stremio.ErrPrivateNetwork):
			return nil, ReasonPrivate, ""
		case errors.Is(err, errNotHTTP):
			return nil, ReasonNotHTTP, ""
		case errors.Is(err, context.DeadlineExceeded):
			return nil, ReasonTimeout, ""
		}
		return nil, ReasonUnavailable, "no answer"
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, ReasonUnavailable, fmt.Sprintf("HTTP %d", response.StatusCode)
	}
	kind, ext := kindOf(response.Header.Get("Content-Type"), response.Request.URL.Path)
	if !accepts(ref.place, kind) {
		mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
		return nil, ReasonWrongType, mediaType
	}
	if response.ContentLength > MaxFileBytes {
		return nil, ReasonTooLarge, ""
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, MaxFileBytes+1))
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return nil, ReasonTimeout, ""
	case err != nil:
		return nil, ReasonUnavailable, "interrupted"
	case len(body) > MaxFileBytes:
		return nil, ReasonTooLarge, ""
	}
	return &saved{address: ref.address, final: response.Request.URL, kind: kind, ext: ext, body: body}, "", ""
}

// accepts tells whether a file of kind may be what a reference at a
// place loads.
func accepts(at place, kind string) bool {
	switch at {
	case inImport:
		return kind == KindStylesheet
	case inURL:
		return kind == KindFont || kind == KindImage
	}
	return kind != ""
}

// kindOf returns the kind of a file sent as contentType from path, and the
// extension it is saved with; "" for one a copy does not keep. A file sent
// as plain text or bytes, as some file hosts send any, is told by its
// extension.
func kindOf(contentType, filePath string) (kind, ext string) {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	switch mediaType {
	case "text/plain", "application/octet-stream", "binary/octet-stream", "":
		byExtension, ok := extensionKinds[strings.ToLower(path.Ext(filePath))]
		if !ok {
			return "", ""
		}
		return byExtension.kind, byExtension.ext
	}
	if found, ok := mediaKinds[mediaType]; ok {
		return found.kind, found.ext
	}
	return "", ""
}

type kindExt struct{ kind, ext string }

var mediaKinds = map[string]kindExt{
	"text/css":                      {KindStylesheet, ".css"},
	"text/javascript":               {KindScript, ".js"},
	"application/javascript":        {KindScript, ".js"},
	"application/x-javascript":      {KindScript, ".js"},
	"application/ecmascript":        {KindScript, ".js"},
	"text/ecmascript":               {KindScript, ".js"},
	"font/woff2":                    {KindFont, ".woff2"},
	"font/woff":                     {KindFont, ".woff"},
	"font/ttf":                      {KindFont, ".ttf"},
	"font/otf":                      {KindFont, ".otf"},
	"font/sfnt":                     {KindFont, ".ttf"},
	"application/font-woff2":        {KindFont, ".woff2"},
	"application/font-woff":         {KindFont, ".woff"},
	"application/x-font-woff":       {KindFont, ".woff"},
	"application/x-font-ttf":        {KindFont, ".ttf"},
	"application/x-font-truetype":   {KindFont, ".ttf"},
	"application/x-font-opentype":   {KindFont, ".otf"},
	"application/font-sfnt":         {KindFont, ".ttf"},
	"application/vnd.ms-fontobject": {KindFont, ".eot"},
	"image/png":                     {KindImage, ".png"},
	"image/jpeg":                    {KindImage, ".jpg"},
	"image/gif":                     {KindImage, ".gif"},
	"image/webp":                    {KindImage, ".webp"},
	"image/avif":                    {KindImage, ".avif"},
	"image/svg+xml":                 {KindImage, ".svg"},
	"image/x-icon":                  {KindImage, ".ico"},
	"image/vnd.microsoft.icon":      {KindImage, ".ico"},
}

var extensionKinds = map[string]kindExt{
	".css":   {KindStylesheet, ".css"},
	".js":    {KindScript, ".js"},
	".mjs":   {KindScript, ".js"},
	".woff2": {KindFont, ".woff2"},
	".woff":  {KindFont, ".woff"},
	".ttf":   {KindFont, ".ttf"},
	".otf":   {KindFont, ".otf"},
	".eot":   {KindFont, ".eot"},
	".png":   {KindImage, ".png"},
	".jpg":   {KindImage, ".jpg"},
	".jpeg":  {KindImage, ".jpg"},
	".gif":   {KindImage, ".gif"},
	".webp":  {KindImage, ".webp"},
	".avif":  {KindImage, ".avif"},
	".svg":   {KindImage, ".svg"},
	".ico":   {KindImage, ".ico"},
}

// baseName is the readable part of a file's name in a copy, from the last
// part of its address: letters, digits, dashes and underscores, without
// its extension.
func baseName(p string) string {
	base := strings.TrimSuffix(path.Base(p), path.Ext(p))
	var out strings.Builder
	for _, r := range base {
		if out.Len() >= 40 {
			break
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			out.WriteRune(r)
		}
	}
	if out.Len() == 0 {
		return "file"
	}
	return out.String()
}

// write writes the files of the run into dir, their stylesheets and
// scripts naming the copy's files instead of the addresses saved.
func (r *copyRun) write(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, file := range r.files {
		body := file.body
		switch file.kind {
		case KindStylesheet:
			body = []byte(rewrite(string(body), file.refs, func(ref reference) string {
				if found := r.saved[ref.address]; found != nil {
					return found.name + fragment(ref.raw)
				}
				return ""
			}))
		case KindScript:
			body = []byte(rewrite(string(body), file.refs, func(ref reference) string {
				if found := r.saved[ref.address]; found != nil {
					return Path + r.id + "/" + found.name + fragment(ref.raw)
				}
				return ""
			}))
		}
		if err := os.WriteFile(filepath.Join(dir, file.name), body, 0o644); err != nil {
			return err
		}
		file.size = int64(len(body))
	}
	return nil
}
