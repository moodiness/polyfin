// Package dlna shares Polyfin's libraries with the UPnP/DLNA devices of
// the local network, TVs, receivers, consoles and players that have no
// Jellyfin app: it announces a MediaServer over SSDP, describes it, and
// answers its ContentDirectory and ConnectionManager under /dlna/ on the
// HTTP port. A device cannot sign in: each shows the libraries of the user
// an administrator chose for it, none until then, and browses and plays as
// that user, under their visible libraries, parental control and limits.
package dlna

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/forwarded"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
)

// Library is what browsing reads: library.Service, which applies the
// user's visible libraries, parental control, blocked genres and hidden
// titles.
type Library interface {
	Libraries(ctx context.Context, user accounts.User) ([]library.Item, error)
	Children(ctx context.Context, user accounts.User, parent accounts.ID, start, count int, genre string) (library.Page, error)
	Seasons(ctx context.Context, user accounts.User, seriesID accounts.ID) ([]library.Item, error)
	Episodes(ctx context.Context, user accounts.User, seriesID accounts.ID, seasonID *accounts.ID) ([]library.Item, error)
	Search(ctx context.Context, user accounts.User, term string, kinds []library.Kind, limit int) ([]library.Item, error)
	SearchMusic(ctx context.Context, user accounts.User, term string, kinds []library.Kind, limit int) ([]library.Item, error)
	Item(ctx context.Context, user accounts.User, id accounts.ID) (library.Item, error)
	Music(ctx context.Context, user accounts.User, q library.MusicQuery) ([]library.Item, error)
	HasChannels(ctx context.Context, user accounts.User) (bool, error)
	Channels(ctx context.Context, user accounts.User) ([]library.Item, error)
}

// Player is what the Jellyfin API's handler does for DLNA devices, as it
// does for its apps: the lists it keeps for users, the version it would
// pick and how it plays, the playbacks under way, and the artwork.
type Player interface {
	// Allowed reports whether user may browse and play now: not disabled,
	// within their allowed hours.
	Allowed(user accounts.User) bool
	// ContinueWatching and NextUp list what the Jellyfin API's lists of
	// those names give user.
	ContinueWatching(ctx context.Context, user accounts.User) ([]library.Item, error)
	NextUp(ctx context.Context, user accounts.User) ([]library.Item, error)
	// Resource describes what a device with profile is offered for item,
	// from what is known now, reading nothing from the sources.
	Resource(ctx context.Context, user accounts.User, item library.Item, profile Profile) (Resource, bool)
	// Play serves a device's request for the bytes of a play.
	Play(w http.ResponseWriter, r *http.Request, play Play)
	// Captions serves the text subtitle offered beside a play's video, as
	// SubRip.
	Captions(w http.ResponseWriter, r *http.Request, play Play)
	// Art serves an item's poster as a JPEG at most width wide.
	Art(w http.ResponseWriter, r *http.Request, id accounts.ID, width int)
}

// Resource is what a device is offered for a movie, an episode, a song or
// a channel: the file as it is, or converted, to MPEG-TS for video and to
// MP3 for a song.
type Resource struct {
	Converted bool
	// Extension is the file extension of the address.
	Extension string
	MimeType  string
	// OrgPn is the DLNA profile name, if known.
	OrgPn    string
	Size     int64
	Duration time.Duration
	// Bitrate is in bits per second.
	Bitrate                           int64
	Width, Height, Channels, Sampling int
	// Captions tells that a text subtitle is offered beside the video.
	Captions bool
}

// Play is a device's request for the bytes of an item.
type Play struct {
	Device  Device
	User    accounts.User
	Item    library.Item
	Profile Profile
	// Converted tells that the address asked for the conversion.
	Converted bool
	// Start is where the device asked to start, by TimeSeekRange.dlna.org;
	// Seek tells it asked so.
	Start time.Duration
	Seek  bool
}

// DisplayName is how sessions and statistics name the device.
func (d Device) DisplayName() string {
	switch {
	case d.Name != "":
		return d.Name
	case d.Model != "":
		return d.Model
	}
	return d.Address
}

// Options are what Start needs.
type Options struct {
	// Port is the UDP port of SSDP: Port, or 0 for a free one, kept from
	// then on.
	Port int
	// HTTP is the address Polyfin's HTTP server listens on.
	HTTP     *net.TCPAddr
	ServerID string
	DB       *pgxpool.Pool
	// Settings returns the current settings; Changed a channel closed at
	// their next change.
	Settings func() accounts.Settings
	Changed  func() <-chan struct{}
	// User returns a user by ID.
	User    func(ctx context.Context, id accounts.ID) (accounts.User, error)
	Signer  playback.Signer
	Library Library
	Player  Player
	Version string
	Logger  *slog.Logger

	// route returns the local address the system reaches remote from;
	// routeTo by default.
	route func(remote netip.Addr) (netip.Addr, error)
	// now is the clock; time.Now by default.
	now func() time.Time
	// applied, when set, is told once a change of the settings is applied.
	applied func()
}

// Service shares the libraries over DLNA while the settings turn it on.
type Service struct {
	options Options
	store   *store
	ssdp    *ssdp
	// added are the added profiles, read once until they change.
	addedMu sync.Mutex
	added   []Profile
	loaded  bool
}

// Start shares the libraries while the settings turn it on, until ctx
// ends, following the settings without a restart.
func Start(ctx context.Context, options Options) *Service {
	if options.route == nil {
		options.route = routeTo
	}
	if options.now == nil {
		options.now = time.Now
	}
	s := &Service{options: options, store: newStore(options.DB)}
	s.ssdp = &ssdp{s: s, port: options.Port, described: map[netip.Addr]*described{}}
	changed := options.Changed()
	s.ssdp.apply(ctx)
	go func() {
		for {
			select {
			case <-ctx.Done():
				s.ssdp.mu.Lock()
				s.ssdp.stop()
				s.ssdp.mu.Unlock()
				return
			case <-changed:
				changed = s.options.Changed()
				s.ssdp.apply(ctx)
				if s.options.applied != nil {
					s.options.applied()
				}
			}
		}
	}()
	return s
}

func (s *Service) now() time.Time { return s.options.now() }

// udn is the server's unique device name, made of its ID.
func (s *Service) udn() string {
	id := s.options.ServerID
	if len(id) != 32 {
		return id
	}
	return id[0:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:32]
}

// serverHeader is the SERVER header of SSDP and of the HTTP answers.
func (s *Service) serverHeader() string {
	return "Polyfin/" + s.options.Version + " UPnP/1.0 DLNADOC/1.50"
}

// name is the name devices show for the server.
func (s *Service) name() string {
	settings := s.options.Settings()
	if settings.DLNAName != "" {
		return settings.DLNAName
	}
	return settings.ServerName
}

// client is a request's device, with the user it shows the libraries of
// and the profile it plays with.
type client struct {
	device  Device
	user    accounts.User
	hasUser bool
	profile Profile
}

// errRefused reports a request answered 403: from outside the local
// network, forwarded by a proxy missing from Known proxies or naming its
// client in Forwarded only, or from a blocked device.
var errRefused = errors.New("refused")

// clientOf finds who sends r, recording the device when it is new. A
// request with forwarded headers that does not come from a known proxy is
// refused: devices never send them, so it comes through a reverse proxy
// missing from Known proxies, whose own local address would stand for
// every client on the internet behind it. So is one that names its client
// in Forwarded alone, which Polyfin does not read, whatever proxy sends it.
func (s *Service) clientOf(r *http.Request) (client, error) {
	named := r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Real-IP") != ""
	if r.Header.Get("Forwarded") != "" && !named || named && !forwarded.ViaProxy(r) {
		return client{}, errRefused
	}
	address, err := netip.ParseAddr(forwarded.Client(r))
	if err != nil || !onLocalNetwork(address.Unmap()) {
		return client{}, errRefused
	}
	address = address.Unmap()
	sighting := Sighting{Address: address.String(), UserAgent: r.Header.Get("User-Agent"), ClientInfo: r.Header.Get("X-AV-Client-Info")}
	if announced, ok := s.ssdp.announcedAt(address); ok {
		sighting.Name, sighting.Manufacturer, sighting.Model = announced.Name, announced.Manufacturer, announced.Model
	}
	if manufacturer, model := clientInfo(sighting.ClientInfo); model != "" {
		sighting.Manufacturer = cmpOr(sighting.Manufacturer, manufacturer)
		sighting.Model = cmpOr(sighting.Model, model)
	}
	device, err := s.store.seen(r.Context(), sighting, s.options.Settings().DLNANewDevicesUser, s.now())
	if err != nil {
		return client{}, err
	}
	if device.Blocked {
		return client{}, errRefused
	}
	c := client{device: device}
	if device.User != nil {
		user, err := s.options.User(r.Context(), *device.User)
		if err == nil && s.options.Player.Allowed(user) {
			c.user, c.hasUser = user, true
		} else if err != nil && !errors.Is(err, accounts.ErrNotFound) {
			return client{}, err
		}
	}
	c.profile = s.profileFor(r, device)
	return c, nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// clientInfo reads the manufacturer and model X-AV-Client-Info names, as
// in av=5.0; cn="Sony Corporation"; mn="BRAVIA KDL-40EX720"; mv="1.7".
func clientInfo(header string) (manufacturer, model string) {
	for part := range strings.SplitSeq(header, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "cn":
			manufacturer = value
		case "mn":
			model = value
		}
	}
	return manufacturer, model
}

// profileFor returns the profile a device plays with: the one forced for
// it, else the first that matches what it sends and announces, added
// profiles first, else the generic one.
func (s *Service) profileFor(r *http.Request, device Device) Profile {
	if device.Profile != "" {
		if p, err := s.store.profileOf(r.Context(), device.Profile); err == nil {
			return p
		}
	}
	seen := Seen{Name: device.Name, Manufacturer: device.Manufacturer, Model: device.Model, Headers: r.Header}
	return Choose(append(s.addedProfiles(r.Context()), builtins[1:]...), seen)
}

// addedProfiles returns the added profiles, read once until they change.
func (s *Service) addedProfiles(ctx context.Context) []Profile {
	s.addedMu.Lock()
	defer s.addedMu.Unlock()
	if !s.loaded {
		stored, err := s.store.profiles(ctx)
		if err != nil {
			s.options.Logger.Warn("The added DLNA profiles could not be read", "error", err)
			return nil
		}
		s.added = s.added[:0]
		for _, p := range stored {
			s.added = append(s.added, p.Profile)
		}
		s.loaded = true
	}
	return slices.Clone(s.added)
}

// profilesChanged makes the added profiles read again.
func (s *Service) profilesChanged() {
	s.addedMu.Lock()
	defer s.addedMu.Unlock()
	s.loaded = false
}

// Devices lists the devices seen, the latest first.
func (s *Service) Devices(ctx context.Context) ([]Device, error) { return s.store.list(ctx) }

// UpdateDevice changes a device's user, blocking or profile.
func (s *Service) UpdateDevice(ctx context.Context, id accounts.ID, changes DeviceChanges) (Device, error) {
	if changes.Profile != nil && !strings.HasPrefix(*changes.Profile, BuiltinPrefix) && *changes.Profile != "" {
		parsed, err := accounts.ParseID(*changes.Profile)
		if err != nil {
			return Device{}, ErrUnknownProfile
		}
		changes.Profile = new(parsed.String())
	}
	return s.store.update(ctx, id, changes)
}

// RemoveDevice forgets a device: seen again, it is new.
func (s *Service) RemoveDevice(ctx context.Context, id accounts.ID) error {
	return s.store.remove(ctx, id)
}

// Profiles lists the added profiles.
func (s *Service) Profiles(ctx context.Context) ([]StoredProfile, error) {
	return s.store.profiles(ctx)
}

// Profile returns an added profile.
func (s *Service) Profile(ctx context.Context, id accounts.ID) (StoredProfile, error) {
	return s.store.profile(ctx, id)
}

// SaveProfile adds a profile from its XML, or replaces the one of id.
func (s *Service) SaveProfile(ctx context.Context, id *accounts.ID, data []byte) (StoredProfile, error) {
	p, err := s.store.saveProfile(ctx, id, data)
	s.profilesChanged()
	return p, err
}

// RemoveProfile deletes an added profile.
func (s *Service) RemoveProfile(ctx context.Context, id accounts.ID) error {
	err := s.store.removeProfile(ctx, id)
	s.profilesChanged()
	return err
}
