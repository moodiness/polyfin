package jellyfinimport

import (
	"cmp"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A user imports their own watch data from a Plex Media Server by signing
// in to Plex as Plex's apps do, with plex.tv's PIN flow (Plex's API
// reference, "Authenticating with Plex"): Polyfin asks plex.tv for a PIN,
// the user's browser opens Plex's own sign-in page with its code, and
// Polyfin asks plex.tv once a second at most whether the PIN was claimed,
// until it gives the account's token or expires. The token lists the Plex
// Home's profiles, when the account is in a Home, and switches to the one
// chosen, with its PIN when it has one, as Plex's apps do: a managed
// profile cannot sign in by itself. The profile's token lists the servers
// it reaches (/api/v2/resources), each with the profile's own token for
// it, which reads its own watched state, resume points and ratings from
// the server's libraries as the owner's token reads the owner's, and
// reads its Watchlist, which plex.tv keeps, from Plex's Discover service.
//
// An administrator importing a server signs in the same way as its owner:
// plex.tv then gives the owner's account, once it shows the account owns
// the server, the token each account the server is shared with has for it
// (/api/servers/{machine}/shared_servers), and switches it to the
// profiles of its Plex Home. Each account is read with its own token, as
// a user's own import reads theirs.
//
// The tokens are held in memory only, for the user who signed in, until
// the import ends or the sign-in is dropped, then signed out of Plex; the
// tokens of shared accounts, which are not sign-ins, are just dropped.
// None is ever stored, logged nor answered.

// PlexTV are the addresses of Plex's online services: plex.tv's API, the
// page a browser signs in to Plex at, and the Discover service, which
// keeps each account's Watchlist.
type PlexTV struct {
	API, SignIn, Discover string
}

// plexTVServices are Plex's own.
var plexTVServices = PlexTV{API: "https://plex.tv", SignIn: "https://app.plex.tv/auth", Discover: "https://discover.provider.plex.tv"}

// or is p, with defaults' address for each one p leaves empty.
func (p PlexTV) or(defaults PlexTV) PlexTV {
	if p.API == "" {
		p.API = defaults.API
	}
	if p.SignIn == "" {
		p.SignIn = defaults.SignIn
	}
	if p.Discover == "" {
		p.Discover = defaults.Discover
	}
	return p
}

// The ways signing in to Plex fails.
var (
	// ErrPlexSignInEnded reports a sign-in to Plex the user does not hold:
	// none was started, its PIN expired, it was dropped after
	// plexSignInHold, replaced by another, or taken by an import.
	ErrPlexSignInEnded = errors.New("the Plex sign-in ended: sign in to Plex again")
	// ErrPlexSignInPending reports a sign-in Plex has not completed yet.
	ErrPlexSignInPending = errors.New("Plex has not signed the user in yet")
	// ErrPlexTV reports an answer of plex.tv that is not what it answers.
	ErrPlexTV = errors.New("plex.tv did not answer as expected")
	// ErrPlexUnknownProfile reports a profile the Plex Home does not have.
	ErrPlexUnknownProfile = errors.New("the Plex Home has no such profile")
	// ErrPlexProfilePIN reports a Home profile's PIN Plex refused.
	ErrPlexProfilePIN = errors.New("Plex refused the profile's PIN")
	// ErrPlexUnknownServer reports a server the Plex account does not
	// reach.
	ErrPlexUnknownServer = errors.New("the Plex account reaches no such server")
	// ErrPlexServerUnreachable reports a server none of whose addresses
	// answered as that server: its machine identifier must match.
	ErrPlexServerUnreachable = errors.New("the Plex server answered at none of its addresses")
	// ErrPlexNotOwner reports a Plex account signed in for an
	// administrator's import that does not own the server imported from.
	ErrPlexNotOwner = errors.New("the Plex account signed in does not own the server")
	// ErrPlexTVUnreachable reports one of Plex's online services that did
	// not answer; it is an ErrUnreachable.
	ErrPlexTVUnreachable = fmt.Errorf("Plex's online services did not answer: %w", ErrUnreachable)
)

// PlexAccess is what an administrator's import reads of a Plex account
// when the server's owner signed in to Plex (see PlexOwnerAccess).
type PlexAccess string

const (
	// PlexAccessAll is an account Plex gives the owner a token for: its
	// played, marked and synced titles, resume points and ratings.
	PlexAccessAll PlexAccess = "all"
	// PlexAccessPIN is a Plex Home profile with a PIN: all of it once the
	// administrator gives the PIN, its plays on the server otherwise.
	PlexAccessPIN PlexAccess = "pin"
	// PlexAccessPlays is an account Plex gives no token for: its plays on
	// the server only, from the server's playback history.
	PlexAccessPlays PlexAccess = "plays"
)

const (
	// plexSignInHold is how long a sign-in is held once Plex signed the
	// user in, while they choose what to import, from their last choice.
	plexSignInHold = 10 * time.Minute
	// plexPINLife is a PIN's life when plex.tv does not tell it.
	plexPINLife = 15 * time.Minute
	// plexPINCheck is the least time between two checks of a PIN, as
	// Plex asks apps to check once a second.
	plexPINCheck = time.Second
	// plexProbeWait bounds the wait for each address of a server.
	plexProbeWait = 10 * time.Second
	// plexWatchlistPage is how many items a page of the Watchlist holds:
	// the Discover service refuses larger pages than Plex Media Server's.
	plexWatchlistPage = 100
	// plexWatchlistLookups bounds the items of a Watchlist whose
	// identifiers are asked one by one, and plexWatchlistFailures the
	// failed lookups in a row that stop asking.
	plexWatchlistLookups  = 500
	plexWatchlistFailures = 3
)

// PlexSignInLink is where a user signs in to Plex: URL opens Plex's own
// sign-in page, which claims the PIN, until Expires.
type PlexSignInLink struct {
	URL     string
	Expires time.Time
}

// PlexAccount is the Plex account a user signed in to: its Name, the
// profiles of its Plex Home, empty outside one, the one read, Profile,
// and the servers that profile reaches.
type PlexAccount struct {
	Name     string
	Profiles []PlexProfile
	Profile  string
	Servers  []PlexServer
}

// PlexProfile is a profile of a Plex Home: Managed for a managed profile,
// which has no sign-in of its own, Protected for one with a PIN.
type PlexProfile struct {
	ID, Name           string
	Managed, Protected bool
}

// PlexServer is a Plex Media Server an account reaches, by its machine
// identifier: Owned when the account owns it, Preselected when
// administrators last imported from it.
type PlexServer struct {
	ID, Name           string
	Owned, Preselected bool
}

// plexSignIn is a user's sign-in to Plex, held while they choose what to
// import: its PIN until Plex claims it, then the account's token, and the
// token of the Home profile switched to, if any. mu guards it while
// plex.tv is asked.
type plexSignIn struct {
	mu      sync.Mutex
	pin     int64
	code    string
	expires time.Time
	checked time.Time
	// account is the token of the account signed in, profileToken the
	// Home profile's switched to; own is the account's profile.
	account, profileToken string
	name, own             string
	profile, profileName  string
	profiles              []PlexProfile
	servers               []plexResource
}

// token is the token of the profile read.
func (p *plexSignIn) token() string {
	if p.profileToken != "" {
		return p.profileToken
	}
	return p.account
}

// plexResource is a server an account reaches, as plex.tv lists it.
type plexResource struct {
	Name             string           `json:"name"`
	ClientIdentifier string           `json:"clientIdentifier"`
	Provides         string           `json:"provides"`
	Owned            bool             `json:"owned"`
	AccessToken      string           `json:"accessToken"`
	Connections      []plexConnection `json:"connections"`
}

// plexConnection is an address of a server: on the account's local
// network, or remote, or relayed by Plex.
type plexConnection struct {
	URI   string `json:"uri"`
	Local bool   `json:"local"`
	Relay bool   `json:"relay"`
}

// rank orders the connections tried: local, remote, then relayed.
func (c plexConnection) rank() int {
	switch {
	case c.Relay:
		return 2
	case c.Local:
		return 0
	}
	return 1
}

// PlexSignIn starts user's sign-in to Plex: it asks plex.tv for a PIN,
// and returns the address of Plex's sign-in page that claims it. It drops
// the user's sign-in held before, if any. It fails with ErrUnreachable or
// ErrPlexTV.
func (s *Service) PlexSignIn(ctx context.Context, user accounts.ID) (PlexSignInLink, error) {
	var pin struct {
		ID        int64  `json:"id"`
		Code      string `json:"code"`
		ExpiresIn int    `json:"expiresIn"`
	}
	status, err := s.plexTVDo(ctx, http.MethodPost, s.plexTV.API+"/api/v2/pins", url.Values{"strong": {"true"}}, "", &pin)
	if err != nil {
		return PlexSignInLink{}, err
	}
	if status/100 != 2 || pin.ID == 0 || pin.Code == "" {
		return PlexSignInLink{}, ErrPlexTV
	}
	life := time.Duration(pin.ExpiresIn) * time.Second
	if life <= 0 {
		life = plexPINLife
	}
	held := &plexSignIn{pin: pin.ID, code: pin.Code, expires: s.now().Add(life)}
	s.mu.Lock()
	s.dropExpiredPlex()
	if before := s.plexSignIns[user]; before != nil {
		s.wg.Go(func() { s.endPlexSignIn(before) })
	}
	s.plexSignIns[user] = held
	s.mu.Unlock()
	link := s.plexTV.SignIn + "#?" + url.Values{"clientID": {s.device}, "code": {pin.Code},
		"context[device][product]": {"Polyfin"}}.Encode()
	return PlexSignInLink{URL: link, Expires: held.expires}, nil
}

// PlexAccount answers user's sign-in to Plex: until Plex signs them in, it
// asks plex.tv whether the PIN was claimed, once a second at most, and
// fails with ErrPlexSignInPending; once it is, it reads who signed in,
// their Plex Home and the servers they reach, and answers them. It fails
// with ErrPlexSignInEnded once the PIN expires, or as PlexSignIn does.
func (s *Service) PlexAccount(ctx context.Context, user accounts.ID) (PlexAccount, error) {
	held := s.heldPlex(user)
	if held == nil {
		return PlexAccount{}, ErrPlexSignInEnded
	}
	held.mu.Lock()
	defer held.mu.Unlock()
	if held.account != "" {
		return s.plexAccount(held), nil
	}
	now := s.now()
	if now.Sub(held.checked) < plexPINCheck {
		return PlexAccount{}, ErrPlexSignInPending
	}
	held.checked = now
	var pin struct {
		AuthToken *string `json:"authToken"`
	}
	status, err := s.plexTVDo(ctx, http.MethodGet, s.plexTV.API+"/api/v2/pins/"+strconv.FormatInt(held.pin, 10), nil, "", &pin)
	switch {
	case err != nil:
		return PlexAccount{}, err
	case status == http.StatusNotFound:
		// plex.tv forgets an expired PIN.
		s.dropPlex(user, held)
		return PlexAccount{}, ErrPlexSignInEnded
	case status != http.StatusOK:
		return PlexAccount{}, ErrPlexTV
	case pin.AuthToken == nil || *pin.AuthToken == "":
		if now.After(held.expires) {
			s.dropPlex(user, held)
			return PlexAccount{}, ErrPlexSignInEnded
		}
		return PlexAccount{}, ErrPlexSignInPending
	}
	held.account = *pin.AuthToken
	held.expires = now.Add(plexSignInHold)
	if err := s.readPlexAccount(ctx, held); err != nil {
		return PlexAccount{}, err
	}
	return s.plexAccount(held), nil
}

// readPlexAccount reads, with held's account token, who signed in, the
// profiles of their Plex Home, and the servers they reach.
func (s *Service) readPlexAccount(ctx context.Context, held *plexSignIn) error {
	var me struct {
		UUID     string `json:"uuid"`
		Username string `json:"username"`
		Title    string `json:"title"`
		Home     bool   `json:"home"`
	}
	status, err := s.plexTVDo(ctx, http.MethodGet, s.plexTV.API+"/api/v2/user", nil, held.account, &me)
	if err != nil {
		return err
	}
	if status != http.StatusOK || me.UUID == "" {
		return ErrPlexTV
	}
	held.name = cmp.Or(me.Title, me.Username)
	held.own, held.profile, held.profileName = me.UUID, me.UUID, held.name
	if me.Home {
		var home struct {
			Users []struct {
				UUID       string `json:"uuid"`
				Title      string `json:"title"`
				Username   string `json:"username"`
				Restricted bool   `json:"restricted"`
				Protected  bool   `json:"protected"`
			} `json:"users"`
		}
		status, err := s.plexTVDo(ctx, http.MethodGet, s.plexTV.API+"/api/v2/home/users", nil, held.account, &home)
		switch {
		case err != nil:
			return err
		case status == http.StatusOK:
			for _, u := range home.Users {
				profile := PlexProfile{ID: u.UUID, Name: cmp.Or(u.Title, u.Username), Managed: u.Restricted, Protected: u.Protected}
				if u.UUID == me.UUID {
					// The account signed in comes first.
					held.profiles = append([]PlexProfile{profile}, held.profiles...)
				} else if u.UUID != "" {
					held.profiles = append(held.profiles, profile)
				}
			}
		case status != http.StatusForbidden:
			// plex.tv answers 403 to an account outside any Home.
			return ErrPlexTV
		}
		if len(held.profiles) < 2 {
			held.profiles = nil
		}
	}
	return s.readPlexServers(ctx, held)
}

// readPlexServers reads the servers the profile held reads reaches, with
// its own token for each.
func (s *Service) readPlexServers(ctx context.Context, held *plexSignIn) error {
	servers, err := s.plexServers(ctx, held.token())
	if err != nil {
		return err
	}
	held.servers = servers
	return nil
}

// plexServers are the Plex Media Servers the account or profile token
// names reaches, with its own token for each.
func (s *Service) plexServers(ctx context.Context, token string) ([]plexResource, error) {
	var resources []plexResource
	status, err := s.plexTVDo(ctx, http.MethodGet, s.plexTV.API+"/api/v2/resources",
		url.Values{"includeHttps": {"1"}, "includeRelay": {"1"}}, token, &resources)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, ErrPlexTV
	}
	return slices.DeleteFunc(resources, func(r plexResource) bool {
		return !slices.Contains(strings.Split(r.Provides, ","), "server") || r.ClientIdentifier == "" || r.AccessToken == ""
	}), nil
}

// plexAccount answers held.
func (s *Service) plexAccount(held *plexSignIn) PlexAccount {
	s.mu.Lock()
	preselected := s.plexServer
	s.mu.Unlock()
	account := PlexAccount{Name: held.name, Profiles: held.profiles, Profile: held.profile}
	for _, r := range held.servers {
		account.Servers = append(account.Servers, PlexServer{ID: r.ClientIdentifier, Name: r.Name, Owned: r.Owned,
			Preselected: r.ClientIdentifier == preselected})
	}
	return account
}

// PlexProfile switches user's sign-in to Plex to the profile of their
// Plex Home, with pin when it has one, as Plex's apps do, and answers the
// account with the servers that profile reaches. Choosing the account
// signed in goes back to it. It fails with ErrPlexSignInEnded,
// ErrPlexSignInPending, ErrPlexUnknownProfile, ErrPlexProfilePIN,
// ErrForbidden, or as PlexSignIn does.
func (s *Service) PlexProfile(ctx context.Context, user accounts.ID, profile, pin string) (PlexAccount, error) {
	held := s.heldPlex(user)
	if held == nil {
		return PlexAccount{}, ErrPlexSignInEnded
	}
	held.mu.Lock()
	defer held.mu.Unlock()
	if held.account == "" {
		return PlexAccount{}, ErrPlexSignInPending
	}
	i := slices.IndexFunc(held.profiles, func(p PlexProfile) bool { return p.ID == profile })
	if i < 0 {
		return PlexAccount{}, ErrPlexUnknownProfile
	}
	token := ""
	if profile != held.own {
		var query url.Values
		if pin != "" {
			query = url.Values{"pin": {pin}}
		}
		var switched struct {
			AuthToken           string `json:"authToken"`
			AuthenticationToken string `json:"authenticationToken"`
		}
		status, err := s.plexTVDo(ctx, http.MethodPost, s.plexTV.API+"/api/v2/home/users/"+url.PathEscape(profile)+"/switch", query,
			held.account, &switched)
		switch {
		case err != nil:
			return PlexAccount{}, err
		case status == http.StatusUnauthorized:
			return PlexAccount{}, ErrPlexProfilePIN
		case status == http.StatusForbidden:
			return PlexAccount{}, ErrForbidden
		case status/100 != 2:
			return PlexAccount{}, ErrPlexTV
		}
		if token = cmp.Or(switched.AuthToken, switched.AuthenticationToken); token == "" {
			return PlexAccount{}, ErrPlexTV
		}
	}
	if before := held.profileToken; before != "" {
		s.wg.Go(func() { s.plexSignOut(before) })
	}
	held.profileToken, held.profile, held.profileName = token, profile, held.profiles[i].Name
	held.expires = s.now().Add(plexSignInHold)
	if err := s.readPlexServers(ctx, held); err != nil {
		return PlexAccount{}, err
	}
	return s.plexAccount(held), nil
}

// DropPlexSignIn drops user's sign-in to Plex, if any, and signs it out.
func (s *Service) DropPlexSignIn(user accounts.ID) {
	if held := s.heldPlex(user); held != nil {
		s.dropPlex(user, held)
	}
}

// ImportOwnPlex imports, in the background, into user's account, who
// starts it, the watch data of the Plex profile they signed in to Plex
// with on server, by its machine identifier: what it played, marked or
// synced as played, its resume points and ratings, and, with watchlist,
// its Watchlist. It reaches the server by the addresses plex.tv gives,
// local first, then remote, then relayed, each an address ParseAddress
// takes, whose /identity names the server. The import takes the sign-in
// over and signs it out of Plex once it ends. It fails with ErrRunning,
// ErrPlexSignInEnded, ErrNotSignedIn before Plex signed the user in,
// ErrPlexUnknownServer or ErrPlexServerUnreachable, keeping the sign-in.
func (s *Service) ImportOwnPlex(ctx context.Context, user accounts.User, server string, watchlist bool) (*Status, error) {
	if !s.claim() {
		return nil, ErrRunning
	}
	started := false
	defer func() {
		if !started {
			s.release()
		}
	}()
	held := s.heldPlex(user.ID)
	if held == nil {
		return nil, ErrPlexSignInEnded
	}
	held.mu.Lock()
	defer held.mu.Unlock()
	if held.account == "" {
		return nil, ErrNotSignedIn
	}
	i := slices.IndexFunc(held.servers, func(r plexResource) bool { return r.ClientIdentifier == server })
	if i < 0 {
		return nil, ErrPlexUnknownServer
	}
	resource := held.servers[i]
	address, err := s.reachPlex(ctx, resource)
	if err != nil {
		return nil, err
	}
	// The import holds the tokens from now on.
	plex := &ownPlex{account: held.token(), watchlist: watchlist}
	signOut := s.takePlex(user.ID, held)
	status := s.newStatus(Server{Kind: Plex, Name: resource.Name, Address: address}, user, true)
	status.Users = []UserStatus{{JellyfinID: held.profile, JellyfinName: held.profileName, User: user.ID, UserName: user.Name,
		State: UserWaiting}}
	c := &client{s: s, kind: Plex, address: address, key: resource.AccessToken}
	snapshot, err := s.launch(status, c, nil, plex, signOut)
	if started = err == nil; !started {
		for _, token := range signOut {
			s.plexSignOut(token)
		}
	}
	return snapshot, err
}

// tokens are the tokens p holds, which end with it.
func (p *plexSignIn) tokens() []string {
	var tokens []string
	for _, token := range []string{p.profileToken, p.account} {
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

// plexShares are the tokens Plex gives the owner of a server for the
// accounts it reaches, by their Plex account identifier, which a server's
// /accounts lists them by: the owner's account token, the server's
// machine identifier, the accounts it is shared with and their access
// token for it, and the profiles of the owner's Plex Home.
type plexShares struct {
	owner, machine string
	shared         map[string]string
	home           map[string]plexHomeUser
}

// plexHomeUser is a profile of a Plex Home, Protected with a PIN.
type plexHomeUser struct {
	uuid      string
	protected bool
}

// plexSharedServers is plex.tv's list of the shares of a server, in XML.
type plexSharedServers struct {
	Shares []struct {
		UserID      string `xml:"userID,attr"`
		AccessToken string `xml:"accessToken,attr"`
	} `xml:"SharedServer"`
}

// PlexOwnerAccess answers what an administrator's import of the Plex
// server connection leads to reads of each account but the owner, by its
// identifier on the server, once by signed in to Plex as the server's
// owner (see PlexSignIn): it fails with ErrPlexSignInEnded,
// ErrNotSignedIn before Plex signed by in, ErrPlexNotOwner, or as Users
// does. The tokens it reads are dropped at once.
func (s *Service) PlexOwnerAccess(ctx context.Context, by accounts.ID, connection Connection) (map[string]PlexAccess, error) {
	server, users, err := s.Users(ctx, connection)
	if err != nil {
		return nil, err
	}
	if server.Kind != Plex {
		return nil, ErrPlexNotOwner
	}
	held := s.heldPlex(by)
	if held == nil {
		return nil, ErrPlexSignInEnded
	}
	held.mu.Lock()
	defer held.mu.Unlock()
	shares, err := s.plexOwnerShares(ctx, held, server.MachineID)
	if err != nil {
		return nil, err
	}
	access := make(map[string]PlexAccess, len(users))
	for _, u := range users {
		if u.ID != plexOwner {
			access[u.ID] = shares.access(u.ID)
		}
	}
	return access, nil
}

// access is what is read of the account id.
func (p *plexShares) access(id string) PlexAccess {
	if p.shared[id] != "" {
		return PlexAccessAll
	}
	if profile, ok := p.home[id]; ok {
		if profile.protected {
			return PlexAccessPIN
		}
		return PlexAccessAll
	}
	return PlexAccessPlays
}

// plexOwnerShares reads, with held's account token, the tokens Plex gives
// it for the accounts reaching the server machine names, once it checked
// the account owns that server. It is called with held.mu held.
func (s *Service) plexOwnerShares(ctx context.Context, held *plexSignIn, machine string) (*plexShares, error) {
	if held.account == "" {
		return nil, ErrNotSignedIn
	}
	servers, err := s.plexServers(ctx, held.account)
	if err != nil {
		return nil, err
	}
	if machine == "" || !slices.ContainsFunc(servers, func(r plexResource) bool { return r.Owned && r.ClientIdentifier == machine }) {
		return nil, ErrPlexNotOwner
	}
	shares := &plexShares{owner: held.account, machine: machine, shared: map[string]string{}, home: map[string]plexHomeUser{}}
	var listed plexSharedServers
	status, err := s.plexTVDo(ctx, http.MethodGet, s.plexTV.API+"/api/servers/"+url.PathEscape(machine)+"/shared_servers", nil,
		held.account, xmlAnswer{&listed})
	switch {
	case err != nil:
		return nil, err
	case status != http.StatusOK:
		return nil, ErrPlexTV
	}
	for _, share := range listed.Shares {
		if share.UserID != "" && share.AccessToken != "" {
			shares.shared[share.UserID] = share.AccessToken
		}
	}
	var home struct {
		Users []struct {
			ID        int64  `json:"id"`
			UUID      string `json:"uuid"`
			Admin     bool   `json:"admin"`
			Protected bool   `json:"protected"`
		} `json:"users"`
	}
	status, err = s.plexTVDo(ctx, http.MethodGet, s.plexTV.API+"/api/v2/home/users", nil, held.account, &home)
	switch {
	case err != nil:
		return nil, err
	case status == http.StatusOK:
		for _, u := range home.Users {
			if u.ID != 0 && u.UUID != "" && !u.Admin {
				shares.home[strconv.FormatInt(u.ID, 10)] = plexHomeUser{uuid: u.UUID, protected: u.Protected}
			}
		}
	case status != http.StatusForbidden:
		// plex.tv answers 403 to an account outside any Home.
		return nil, ErrPlexTV
	}
	return shares, nil
}

// token returns the token that reads the account id's own watch data on
// the server: the one Plex gives the owner for an account the server is
// shared with, else, for a profile of the owner's Plex Home, the one the
// profile has for the server once switched to, with pin when it has one.
// It returns "" for an account Plex gives no token for, or a protected
// profile without pin, and the profile's token, signed out of Plex once
// done with, as signOut. It fails with ErrPlexProfilePIN, or as
// PlexProfile does.
func (s *Service) plexToken(ctx context.Context, shares *plexShares, id, pin string) (token, signOut string, err error) {
	if token := shares.shared[id]; token != "" {
		return token, "", nil
	}
	profile, ok := shares.home[id]
	if !ok || profile.protected && pin == "" {
		return "", "", nil
	}
	var query url.Values
	if pin != "" {
		query = url.Values{"pin": {pin}}
	}
	var switched struct {
		AuthToken           string `json:"authToken"`
		AuthenticationToken string `json:"authenticationToken"`
	}
	status, err := s.plexTVDo(ctx, http.MethodPost, s.plexTV.API+"/api/v2/home/users/"+url.PathEscape(profile.uuid)+"/switch", query,
		shares.owner, &switched)
	switch {
	case err != nil:
		return "", "", err
	case status == http.StatusUnauthorized:
		return "", "", ErrPlexProfilePIN
	case status == http.StatusForbidden:
		return "", "", ErrForbidden
	case status/100 != 2:
		return "", "", ErrPlexTV
	}
	signOut = cmp.Or(switched.AuthToken, switched.AuthenticationToken)
	if signOut == "" {
		return "", "", ErrPlexTV
	}
	servers, err := s.plexServers(ctx, signOut)
	if err != nil {
		return "", signOut, err
	}
	for _, r := range servers {
		if r.ClientIdentifier == shares.machine {
			return r.AccessToken, signOut, nil
		}
	}
	return "", signOut, nil
}

// takePlex takes user's sign-in held over for an import, which signs its
// tokens out of Plex once it ends, and returns them. It is called with
// held.mu held.
func (s *Service) takePlex(user accounts.ID, held *plexSignIn) []string {
	s.mu.Lock()
	if s.plexSignIns[user] == held {
		delete(s.plexSignIns, user)
	}
	s.mu.Unlock()
	tokens := held.tokens()
	held.account, held.profileToken = "", ""
	return tokens
}

// xmlAnswer has plexTVDo read an answer of plex.tv's in XML into v, as
// plex.tv answers its older API.
type xmlAnswer struct {
	v any
}

// reachPlex returns the address of resource that answers as it: its local
// addresses first, then its remote ones, then those relayed by Plex, each
// an address ParseAddress takes, whose /identity names the server's
// machine identifier.
func (s *Service) reachPlex(ctx context.Context, resource plexResource) (string, error) {
	connections := slices.Clone(resource.Connections)
	slices.SortStableFunc(connections, func(a, b plexConnection) int { return a.rank() - b.rank() })
	for _, connection := range connections {
		address, err := ParseAddress(connection.URI)
		if err != nil {
			continue
		}
		probe := &client{s: s, kind: Plex, address: address}
		var identity struct {
			MediaContainer struct {
				MachineIdentifier string `json:"machineIdentifier"`
			}
		}
		probeCtx, cancel := context.WithTimeout(ctx, plexProbeWait)
		err = probe.get(probeCtx, "/identity", nil, &identity)
		cancel()
		if err == nil && identity.MediaContainer.MachineIdentifier == resource.ClientIdentifier {
			return address, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}
	return "", ErrPlexServerUnreachable
}

// plexWatchlist reads the Watchlist of the Plex account or profile token
// is, a page at a time, handing each movie or show read to found. The
// Discover service's list may leave the items' identifiers out: those are
// asked of it item by item, paced, plexWatchlistLookups at most, until
// plexWatchlistFailures lookups fail in a row.
func (s *Service) plexWatchlist(ctx context.Context, token string, found func()) ([]itemJSON, error) {
	var listed []plexItemJSON
	for start := 0; ; {
		var page plexContainerJSON
		status, err := s.plexTVDo(ctx, http.MethodGet, s.plexTV.Discover+"/library/sections/watchlist/all", url.Values{
			"includeGuids":           {"1"},
			"X-Plex-Container-Start": {strconv.Itoa(start)},
			"X-Plex-Container-Size":  {strconv.Itoa(plexWatchlistPage)},
		}, token, &page)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, ErrPlexTV
		}
		items := page.items()
		listed = append(listed, items...)
		start += len(items)
		if total := page.MediaContainer.TotalSize; len(items) == 0 || total == nil || start >= *total {
			break
		}
		if !sleep(ctx, s.timing.gap) {
			return nil, ctx.Err()
		}
	}
	var watchlist []itemJSON
	lookups, failures := 0, 0
	for _, p := range listed {
		if p.Type != plexMovie && p.Type != plexShow {
			continue
		}
		if len(p.Guid) == 0 && p.RatingKey != "" && lookups < plexWatchlistLookups && failures < plexWatchlistFailures {
			lookups++
			if !sleep(ctx, s.timing.gap) {
				return watchlist, ctx.Err()
			}
			var meta plexContainerJSON
			status, err := s.plexTVDo(ctx, http.MethodGet, s.plexTV.Discover+"/library/metadata/"+url.PathEscape(p.RatingKey),
				url.Values{"includeGuids": {"1"}}, token, &meta)
			switch {
			case err == nil && status == http.StatusOK && len(meta.items()) > 0:
				p.Guid, p.Year, failures = meta.items()[0].Guid, cmp.Or(p.Year, meta.items()[0].Year), 0
			case err == nil && status == http.StatusNotFound:
				// That item is gone; the others may not be.
				failures = 0
			default:
				failures++
			}
		}
		item := itemJSON{ID: "watchlist:" + p.RatingKey, Name: p.Title, Type: "Movie", ProductionYear: p.Year, ProviderIDs: p.providers()}
		if p.Type == plexShow {
			item.Type = "Series"
		}
		watchlist = append(watchlist, item)
		found()
	}
	return watchlist, nil
}

// heldPlex is user's sign-in to Plex held, nil when none is.
func (s *Service) heldPlex(user accounts.ID) *plexSignIn {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropExpiredPlex()
	return s.plexSignIns[user]
}

// dropPlex drops user's sign-in held, if still held, and signs it out.
func (s *Service) dropPlex(user accounts.ID, held *plexSignIn) {
	s.mu.Lock()
	if s.plexSignIns[user] == held {
		delete(s.plexSignIns, user)
	}
	s.mu.Unlock()
	tokens := held.tokens()
	held.account, held.profileToken = "", ""
	for _, token := range tokens {
		s.wg.Go(func() { s.plexSignOut(token) })
	}
}

// dropExpiredPlex drops the sign-ins held past their expiry, signing them
// out in the background. It is called with s.mu held.
func (s *Service) dropExpiredPlex() {
	now := s.now()
	for user, held := range s.plexSignIns {
		if now.After(held.expires) {
			delete(s.plexSignIns, user)
			s.wg.Go(func() { s.endPlexSignIn(held) })
		}
	}
}

// endPlexSignIn signs out of Plex the tokens held holds.
func (s *Service) endPlexSignIn(held *plexSignIn) {
	held.mu.Lock()
	tokens := held.tokens()
	held.account, held.profileToken = "", ""
	held.mu.Unlock()
	for _, token := range tokens {
		s.plexSignOut(token)
	}
}

// plexSignOut signs token out of Plex, which takes Polyfin off the
// account's authorized devices for it. It does not wait long: nothing
// depends on it.
func (s *Service) plexSignOut(token string) {
	ctx, cancel := context.WithTimeout(context.Background(), signOutWait)
	defer cancel()
	_, _ = s.plexTVDo(ctx, http.MethodDelete, s.plexTV.API+"/api/v2/users/signout", nil, token, nil)
}

// plexTVDo sends a request to one of Plex's online services at address,
// with query, as token when there is one, naming Polyfin as Plex's apps
// name themselves, and reads an answer of status 200 or 201 into into, in
// JSON, or in XML for an xmlAnswer. It returns the answer's status, and
// ErrPlexTVUnreachable when none came.
func (s *Service) plexTVDo(ctx context.Context, method, address string, query url.Values, token string, into any) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timing.request)
	defer cancel()
	if len(query) > 0 {
		address += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, method, address, nil)
	if err != nil {
		return 0, err
	}
	decode := json.Unmarshal
	if x, ok := into.(xmlAnswer); ok {
		into, decode = x.v, xml.Unmarshal
		request.Header.Set("Accept", "application/xml")
	} else {
		request.Header.Set("Accept", "application/json")
	}
	request.Header.Set("X-Plex-Product", "Polyfin")
	request.Header.Set("X-Plex-Version", s.version)
	request.Header.Set("X-Plex-Client-Identifier", s.device)
	request.Header.Set("X-Plex-Device-Name", "Polyfin")
	if token != "" {
		request.Header.Set("X-Plex-Token", token)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return 0, ErrPlexTVUnreachable
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(response.Body, maxReply))
	if err != nil {
		return 0, ErrPlexTVUnreachable
	}
	if into == nil || response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		return response.StatusCode, nil
	}
	if err := decode(answer, into); err != nil {
		// The path tells where, never the query or the token.
		s.logger.Warn("A Plex sign-in could not read an answer", "path", request.URL.Path, "error", err)
		return response.StatusCode, ErrPlexTV
	}
	return response.StatusCode, nil
}
