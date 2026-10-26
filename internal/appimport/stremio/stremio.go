// Package stremio reads a Stremio account, for an import into Polyfin (see
// package appimport).
//
// Stremio keeps its accounts on its account API, which its apps call with
// an auth key. Polyfin gets one of three ways: by a link code, as
// Stremio's TV apps sign in, which the user approves in a browser signed
// in to Stremio, whatever way their account signs in; by the account's
// email and password; or as the user pastes it, read from Stremio's web
// app. With it, Polyfin reads the account's library and addons once, then
// signs out, but for a key the user pasted: that would sign their own
// browser out. The key serves that only: it is never logged, nor answered
// to the browser, and kept only when the user chooses to stay connected,
// for the connection to read the account again (see Reader.Reread) until
// the user disconnects, when Polyfin signs out the session it opened.
//
// A supporter's account has profiles, up to six, each with its own
// library and addons, which Stremio's apps read with a session of the
// profile's own, opened with the account's key and the profile's PIN when
// it has one. Polyfin reads each profile so, signing out each session it
// opened; a profile with a PIN is read once the user gives it (see
// Reader.Unlock). The PIN goes to Stremio only: Polyfin never keeps it,
// and keeps, for a connection, the profile's session in its place. An
// account without profiles is read whole, with its own key.
//
// The library holds, for each title, whether it was watched, the resume
// point of the video played last, and whether it is in the user's library.
// A series tells its episodes watched by their places in its list of
// episodes, which Polyfin reads as its metadata addons list them (see
// watched).
//
// The account API is not documented: Polyfin calls it as Stremio's own
// apps do, its profiles as Stremio's web app does.
package stremio

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/anime"
	"github.com/moodiness/polyfin/internal/appimport"
	addon "github.com/moodiness/polyfin/internal/stremio"
)

// App names Stremio among the apps imported from.
const App = "stremio"

// The addresses of Stremio's account API and of its link service.
const (
	DefaultAPI  = "https://api.strem.io"
	DefaultLink = "https://link.stremio.com"
)

// The ways reading an account fails.
var (
	// ErrUnreachable reports Stremio not answering, or failing.
	ErrUnreachable = errors.New("Stremio does not answer")
	// ErrUnexpected reports an answer Polyfin does not read.
	ErrUnexpected = errors.New("Stremio answers in a way Polyfin does not read")
	// ErrSignInRefused reports an email and password Stremio refuses.
	ErrSignInRefused = errors.New("Stremio refuses this email and password")
	// ErrKeyRefused reports an auth key Stremio refuses.
	ErrKeyRefused = errors.New("Stremio refuses this auth key")
	// ErrPinRefused reports a profile's PIN Stremio refuses.
	ErrPinRefused = errors.New("Stremio refuses this PIN")
	// ErrLinkPending reports a link code not approved yet.
	ErrLinkPending = errors.New("the link code is not approved yet")
	// ErrLinkExpired reports a link code asked for too long ago, or none
	// asked for.
	ErrLinkExpired = errors.New("the link code expired: ask for another")
)

// maxReply bounds an answer read: a library is a few megabytes at most.
const maxReply = 64 << 20

// timing is how the reader paces its requests. Tests shorten it.
type timing struct {
	// request bounds each request, and signOut the wait for the session
	// to end.
	request, signOut time.Duration
	// linkFor is how long a link code is waited for.
	linkFor time.Duration
}

var defaultTiming = timing{request: time.Minute, signOut: 5 * time.Second, linkFor: 5 * time.Minute}

// Metas describes titles as the server's addons do (see
// library.Service.TitleMeta).
type Metas interface {
	TitleMeta(ctx context.Context, metaType, id string) (addon.Meta, bool)
}

// Options are the dependencies of the reader.
type Options struct {
	// Anime maps the Kitsu identifiers of anime to Polyfin's titles; nil
	// finds them as anime catalogs list them only.
	Anime *anime.Service
	// Metas gives the episodes of series; nil reads no episode watched.
	Metas Metas
	// Version is Polyfin's, which Stremio is told.
	Version string
	// API and Link are the addresses of Stremio's account API and link
	// service, empty for Stremio's own.
	API, Link string
}

// Reader reads Stremio accounts.
type Reader struct {
	anime     *anime.Service
	metas     Metas
	version   string
	api, link string
	client    *http.Client
	timing    timing
	now       func() time.Time

	mu sync.Mutex
	// links are the link codes asked for, by the user who asked: one
	// each.
	links map[accounts.ID]*pendingLink
}

// pendingLink is a link code waited for until until; busy while a poll
// reads it.
type pendingLink struct {
	code  string
	until time.Time
	busy  bool
}

// New returns a reader with options.
func New(options Options) *Reader {
	api, link := DefaultAPI, DefaultLink
	if options.API != "" {
		api = strings.TrimRight(options.API, "/")
	}
	if options.Link != "" {
		link = strings.TrimRight(options.Link, "/")
	}
	return &Reader{anime: options.Anime, metas: options.Metas, version: options.Version, api: api, link: link, client: &http.Client{},
		timing: defaultTiming, now: time.Now, links: map[accounts.ID]*pendingLink{}}
}

// Link is a link code asked for: the user opens URL in a browser signed
// in to Stremio, where Code shows, and approves it.
type Link struct {
	Code, URL string
}

// StartLink asks Stremio for a link code, which by waits for (see
// PollLink), in place of any they asked for before. It fails with
// ErrUnreachable or ErrUnexpected.
func (r *Reader) StartLink(ctx context.Context, by accounts.ID) (Link, error) {
	var created struct {
		Code string `json:"code"`
		Link string `json:"link"`
	}
	if err := r.call(ctx, http.MethodGet, r.link+"/api/v2/create?type=Create", nil, &created); err != nil {
		if errors.Is(err, errRefused) {
			return Link{}, ErrUnexpected
		}
		return Link{}, err
	}
	code := strings.TrimSpace(created.Code)
	address, err := url.Parse(strings.TrimSpace(created.Link))
	if code == "" || err != nil || address.Scheme != "https" && address.Scheme != "http" || address.Host == "" {
		return Link{}, ErrUnexpected
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	for other, l := range r.links {
		if !now.Before(l.until) {
			delete(r.links, other)
		}
	}
	r.links[by] = &pendingLink{code: code, until: now.Add(r.timing.linkFor)}
	return Link{Code: code, URL: address.String()}, nil
}

// PollLink asks Stremio whether the link code by waits for is approved:
// once it is, it reads the account with the auth key the approval gives,
// signs out unless keep asks to stay connected, and returns the account.
// It fails with ErrLinkPending until then, ErrLinkExpired past a few
// minutes, ErrUnreachable, ErrUnexpected, or ErrKeyRefused.
func (r *Reader) PollLink(ctx context.Context, by accounts.ID, keep bool) (appimport.Account, error) {
	r.mu.Lock()
	l := r.links[by]
	switch {
	case l == nil || !r.now().Before(l.until):
		delete(r.links, by)
		r.mu.Unlock()
		return appimport.Account{}, ErrLinkExpired
	case l.busy:
		r.mu.Unlock()
		return appimport.Account{}, ErrLinkPending
	}
	l.busy = true
	r.mu.Unlock()

	var approved struct {
		AuthKey string `json:"authKey"`
	}
	err := r.call(ctx, http.MethodGet, r.link+"/api/v2/read?type=Read&code="+url.QueryEscape(l.code), nil, &approved)
	key := strings.TrimSpace(approved.AuthKey)
	r.mu.Lock()
	l.busy = false
	if err == nil && key != "" && r.links[by] == l {
		delete(r.links, by)
	}
	r.mu.Unlock()
	switch {
	case errors.Is(err, ErrUnreachable):
		return appimport.Account{}, ErrUnreachable
	// Until the code is approved, Stremio answers an error, or no key.
	case err != nil || key == "":
		return appimport.Account{}, ErrLinkPending
	}
	return r.read(ctx, key, true, keep, "")
}

// SignIn signs in to Stremio with email and password, reads the account,
// signs out unless keep asks to stay connected, and returns the account.
// It fails with ErrSignInRefused, ErrUnreachable or ErrUnexpected.
func (r *Reader) SignIn(ctx context.Context, email, password string, keep bool) (appimport.Account, error) {
	var signedIn struct {
		AuthKey string `json:"authKey"`
		User    struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	err := r.call(ctx, http.MethodPost, r.api+"/api/login", struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Facebook bool   `json:"facebook"`
	}{strings.TrimSpace(email), password, false}, &signedIn)
	switch {
	case errors.Is(err, errRefused):
		return appimport.Account{}, ErrSignInRefused
	case err != nil:
		return appimport.Account{}, err
	case strings.TrimSpace(signedIn.AuthKey) == "":
		return appimport.Account{}, ErrUnexpected
	}
	return r.read(ctx, strings.TrimSpace(signedIn.AuthKey), true, keep, strings.TrimSpace(signedIn.User.Email))
}

// ReadKey reads the account with the auth key the user pasted, without
// signing out, which would sign the user's own browser out, and returns
// the account, keeping the key when keep asks to stay connected. It fails
// with ErrKeyRefused, ErrUnreachable or ErrUnexpected.
func (r *Reader) ReadKey(ctx context.Context, key string, keep bool) (appimport.Account, error) {
	// A key copied from the browser's console comes quoted.
	key = strings.Trim(strings.TrimSpace(key), `"'`)
	if key == "" {
		return appimport.Account{}, ErrKeyRefused
	}
	return r.read(ctx, key, false, keep, "")
}

// read reads the account key signs in to, and returns it. When keep asks
// to stay connected, the account holds the key, with the account's email,
// name when known; so it does, while the account is held, when one of its
// profiles asks for its PIN (see Unlock). Otherwise Polyfin signs out when
// it obtained the key itself (owned).
func (r *Reader) read(ctx context.Context, key string, owned, keep bool, name string) (appimport.Account, error) {
	account, err := r.readAccount(ctx, key)
	held := err == nil && (keep || slices.ContainsFunc(account.Profiles, func(p appimport.Profile) bool { return p.Locked }))
	if owned && !held {
		_ = r.signOut(context.Background(), key)
	}
	switch {
	case errors.Is(err, errRefused):
		return appimport.Account{}, ErrKeyRefused
	case err != nil:
		return appimport.Account{}, err
	}
	if name != "" {
		account.Name = name
	}
	if held {
		account.SignIn, account.StayConnected = &appimport.SignIn{Secret: key, Owned: owned}, keep
	}
	return account, nil
}

// wholeAccount is the ID of the one profile of an account without
// profiles, read with the account's session.
const wholeAccount = "1"

// readAccount reads the account key signs in to: its email, and its
// profiles, each read with its own session but those that ask for their
// PIN, which are listed locked; an account without profiles has one, the
// whole account. It fails with errRefused when Stremio refuses key.
func (r *Reader) readAccount(ctx context.Context, key string) (appimport.Account, error) {
	u, err := r.user(ctx, key)
	if err != nil {
		return appimport.Account{}, err
	}
	account := appimport.Account{App: App, Name: strings.TrimSpace(u.Email)}
	profiles := u.profiles()
	if len(profiles) == 0 {
		whole, err := r.readProfile(ctx, key)
		if err != nil {
			return appimport.Account{}, err
		}
		whole.ID = wholeAccount
		account.Profiles = []appimport.Profile{whole}
		return account, nil
	}
	for _, p := range profiles {
		if p.HasPin {
			account.Profiles = append(account.Profiles, appimport.Profile{ID: p.ID, Name: p.Name, Locked: true})
			continue
		}
		read, err := r.openProfile(ctx, key, p, "", false)
		if errors.Is(err, errOpenRefused) {
			// A profile without a PIN opens without one.
			err = ErrUnexpected
		}
		if err != nil {
			return appimport.Account{}, err
		}
		account.Profiles = append(account.Profiles, read)
	}
	return account, nil
}

// user is what Polyfin reads of the user getUser answers: the account's
// email, and its profiles, which Stremio's supporters have.
type user struct {
	Email        string          `json:"email"`
	PremiumPrefs json.RawMessage `json:"premiumPrefs"`
}

// profile is one of an account's profiles, as getUser lists them: the
// master is the account's own.
type profile struct {
	ID       string `json:"_id"`
	Name     string `json:"name"`
	IsMaster bool   `json:"isMaster"`
	HasPin   bool   `json:"hasPin"`
}

// profiles lists the account's profiles, in Stremio's order, those
// without an ID left out; none for an account without profiles, or when
// Stremio lists them in a way Polyfin does not read: the whole account is
// read then.
func (u user) profiles() []profile {
	var prefs struct {
		UserProfiles json.RawMessage `json:"userProfiles"`
	}
	if !present(u.PremiumPrefs) || json.Unmarshal(u.PremiumPrefs, &prefs) != nil || !present(prefs.UserProfiles) {
		return nil
	}
	var listed []profile
	if json.Unmarshal(prefs.UserProfiles, &listed) != nil {
		// A map of the profiles, by their IDs.
		var byID map[string]profile
		if json.Unmarshal(prefs.UserProfiles, &byID) != nil {
			return nil
		}
		for id, p := range byID {
			if p.ID == "" {
				p.ID = id
			}
			listed = append(listed, p)
		}
		slices.SortFunc(listed, func(a, b profile) int { return cmp.Compare(a.ID, b.ID) })
	}
	result := make([]profile, 0, len(listed))
	for _, p := range listed {
		p.ID, p.Name = strings.TrimSpace(p.ID), strings.TrimSpace(p.Name)
		if p.ID != "" {
			result = append(result, p)
		}
	}
	return result
}

// user reads the user key signs in to. It fails with errRefused when
// Stremio refuses key.
func (r *Reader) user(ctx context.Context, key string) (user, error) {
	var u user
	err := r.call(ctx, http.MethodPost, r.api+"/api/getUser", struct {
		AuthKey string `json:"authKey"`
	}{key}, &u)
	return u, err
}

// errOpenRefused is a profile Stremio refused to open: with the PIN
// given, or without one.
var errOpenRefused = errors.New("Stremio refuses to open this profile")

// openProfile reads the profile p of the account key signs in to: the
// master without a PIN with key, as the account's own; any other with a
// session of its own, opened with pin, empty for none, which is signed out
// unless keep asks to stay connected, when the profile holds it. It fails
// with errOpenRefused when Stremio refuses to open the profile.
func (r *Reader) openProfile(ctx context.Context, key string, p profile, pin string, keep bool) (appimport.Profile, error) {
	if p.IsMaster && !p.HasPin {
		read, err := r.readProfile(ctx, key)
		read.ID, read.Name = p.ID, p.Name
		return read, err
	}
	session, err := r.authenticate(ctx, key, p.ID, pin)
	if err != nil {
		return appimport.Profile{}, err
	}
	read, err := r.readProfile(ctx, session)
	if !keep || err != nil {
		_ = r.signOut(context.Background(), session)
	}
	switch {
	case errors.Is(err, errRefused):
		// The session Stremio just opened.
		return appimport.Profile{}, ErrUnexpected
	case err != nil:
		return appimport.Profile{}, err
	}
	read.ID, read.Name = p.ID, p.Name
	if keep {
		read.Session = session
	}
	return read, nil
}

// authenticate opens a session of the profile id of the account key signs
// in to, with pin, empty for none, as Stremio's apps switch profiles, and
// returns its auth key. It fails with errOpenRefused when Stremio refuses.
// The PIN goes in the body, which errors never hold.
func (r *Reader) authenticate(ctx context.Context, key, id, pin string) (string, error) {
	var pinGiven *string
	if pin != "" {
		pinGiven = &pin
	}
	var opened struct {
		AuthKey string `json:"authKey"`
	}
	err := r.call(ctx, http.MethodPost, r.api+"/api/premiumPrefs/authenticateProfile", struct {
		Type      string  `json:"type"`
		AuthKey   string  `json:"authKey"`
		ProfileID string  `json:"profileId"`
		Pin       *string `json:"pin"`
	}{"AuthenticateProfile", key, id, pinGiven}, &opened)
	switch {
	case errors.Is(err, errRefused):
		return "", errOpenRefused
	case err != nil:
		return "", err
	case strings.TrimSpace(opened.AuthKey) == "":
		return "", ErrUnexpected
	}
	return strings.TrimSpace(opened.AuthKey), nil
}

// readProfile reads what the session key reads: the library and the
// addons of an account, or of one of its profiles. It fails with
// errRefused when Stremio refuses key.
func (r *Reader) readProfile(ctx context.Context, key string) (appimport.Profile, error) {
	items, descriptors, err := r.fetch(ctx, key)
	if err != nil {
		return appimport.Profile{}, err
	}
	return r.profile(ctx, items, descriptors), nil
}

// Unlock opens, with pin, the profile id of the account signIn signs in
// to, which asks for its PIN, and returns it read. The PIN goes to Stremio
// only: it is never kept, nor logged. The session opened is signed out,
// unless keep asks to stay connected: the profile holds it then, for the
// connection to read the profile again without the PIN. It fails with
// ErrPinRefused, ErrKeyRefused once Stremio refuses the account's key,
// appimport.ErrUnknownProfile when the account no longer has the
// profile, ErrUnreachable or ErrUnexpected.
func (r *Reader) Unlock(ctx context.Context, signIn appimport.SignIn, id, pin string, keep bool) (appimport.Profile, error) {
	if pin = strings.TrimSpace(pin); pin == "" {
		return appimport.Profile{}, ErrPinRefused
	}
	u, err := r.user(ctx, signIn.Secret)
	if errors.Is(err, errRefused) {
		return appimport.Profile{}, ErrKeyRefused
	}
	if err != nil {
		return appimport.Profile{}, err
	}
	profiles := u.profiles()
	i := slices.IndexFunc(profiles, func(p profile) bool { return p.ID == id })
	if i < 0 {
		return appimport.Profile{}, appimport.ErrUnknownProfile
	}
	read, err := r.openProfile(ctx, signIn.Secret, profiles[i], pin, keep)
	if errors.Is(err, errOpenRefused) {
		return appimport.Profile{}, ErrPinRefused
	}
	return read, err
}

// Reread reads again, for a connection, the profile chosen of the account
// the auth key kept signs in to, and returns the account with that
// profile, and the key, which Stremio does not renew: the whole account
// read with the key, or a profile read with the session kept of it, opened
// with its PIN, or else a session opened without one, then signed out.
// The account without the profile is returned when Stremio no longer
// lists it. It fails with appimport.ErrSignInRefused once Stremio refuses
// the key, appimport.ErrProfileLocked when the profile asks for a PIN
// Polyfin does not have (one set since, or another, as Stremio refuses the
// session kept), appimport.ErrAppUnreachable, or ErrUnexpected.
func (r *Reader) Reread(ctx context.Context, signIn appimport.SignIn) (appimport.Account, appimport.SignIn, error) {
	account, err := r.reread(ctx, signIn)
	switch {
	case errors.Is(err, errRefused):
		return appimport.Account{}, signIn, appimport.ErrSignInRefused
	case errors.Is(err, errOpenRefused):
		return appimport.Account{}, signIn, appimport.ErrProfileLocked
	case errors.Is(err, ErrUnreachable):
		return appimport.Account{}, signIn, appimport.ErrAppUnreachable
	case err != nil:
		return appimport.Account{}, signIn, err
	}
	return account, signIn, nil
}

// reread reads the profile signIn chose, as Reread does. It fails with
// errRefused when Stremio refuses the account's key, and errOpenRefused
// when it refuses to open the profile.
func (r *Reader) reread(ctx context.Context, signIn appimport.SignIn) (appimport.Account, error) {
	u, err := r.user(ctx, signIn.Secret)
	if err != nil {
		return appimport.Account{}, err
	}
	account := appimport.Account{App: App, Name: strings.TrimSpace(u.Email)}
	if signIn.Profile == wholeAccount {
		whole, err := r.readProfile(ctx, signIn.Secret)
		whole.ID = wholeAccount
		account.Profiles = []appimport.Profile{whole}
		return account, err
	}
	profiles := u.profiles()
	i := slices.IndexFunc(profiles, func(p profile) bool { return p.ID == signIn.Profile })
	if i < 0 {
		return account, nil
	}
	p := profiles[i]
	var read appimport.Profile
	switch {
	case signIn.Session != "":
		read, err = r.readProfile(ctx, signIn.Session)
		if errors.Is(err, errRefused) {
			return appimport.Account{}, errOpenRefused
		}
		read.ID, read.Name = p.ID, p.Name
	case p.HasPin:
		// A PIN set since the profile was chosen.
		return appimport.Account{}, errOpenRefused
	default:
		read, err = r.openProfile(ctx, signIn.Secret, p, "", false)
	}
	if err != nil {
		return appimport.Account{}, err
	}
	account.Profiles = []appimport.Profile{read}
	return account, nil
}

// SignOut ends the sessions of a connection Polyfin opened: the auth key's
// when Polyfin obtained it, by a link code or a password, and the
// profile's it opened with its PIN. A key the user pasted is only
// forgotten, as signing it out would sign their own browser out.
func (r *Reader) SignOut(ctx context.Context, signIn appimport.SignIn) error {
	var errs []error
	if signIn.Session != "" {
		errs = append(errs, r.signOut(ctx, signIn.Session))
	}
	if signIn.Owned && signIn.Secret != "" {
		errs = append(errs, r.signOut(ctx, signIn.Secret))
	}
	return errors.Join(errs...)
}

// fetch reads the library items and addon descriptors the session key
// reads, as Stremio answers them.
func (r *Reader) fetch(ctx context.Context, key string) (items, descriptors []json.RawMessage, err error) {
	if err := r.call(ctx, http.MethodPost, r.api+"/api/datastoreGet", struct {
		AuthKey    string   `json:"authKey"`
		Collection string   `json:"collection"`
		IDs        []string `json:"ids"`
		All        bool     `json:"all"`
	}{key, "libraryItem", []string{}, true}, &items); err != nil {
		return nil, nil, err
	}
	var collection struct {
		Addons []json.RawMessage `json:"addons"`
	}
	if err := r.call(ctx, http.MethodPost, r.api+"/api/addonCollectionGet", struct {
		AuthKey string `json:"authKey"`
		Update  bool   `json:"update"`
	}{key, true}, &collection); err != nil {
		return nil, nil, err
	}
	return items, collection.Addons, nil
}

// signOut ends the session key signs in to, so that the account does not
// keep it, within ctx and a few seconds.
func (r *Reader) signOut(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, r.timing.signOut)
	defer cancel()
	var ignored json.RawMessage
	return r.call(ctx, http.MethodPost, r.api+"/api/logout", struct {
		AuthKey string `json:"authKey"`
	}{key}, &ignored)
}

// errRefused is a request Stremio answered with an error: what that means
// depends on the request.
var errRefused = errors.New("refused")

// call sends a request, a GET or a POST of body as JSON, and reads the
// result Stremio answers into into. Auth keys go in bodies, never in the
// address, which errors would hold.
func (r *Reader) call(ctx context.Context, method, address string, body, into any) error {
	ctx, cancel := context.WithTimeout(ctx, r.timing.request)
	defer cancel()
	var content io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		content = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, address, content)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "Polyfin/"+r.version)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := r.client.Do(request)
	if err != nil {
		return ErrUnreachable
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(response.Body, maxReply))
	if err != nil || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError {
		return ErrUnreachable
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if json.Unmarshal(answer, &envelope) != nil {
		return ErrUnexpected
	}
	if present(envelope.Error) {
		return errRefused
	}
	if response.StatusCode != http.StatusOK || !present(envelope.Result) || json.Unmarshal(envelope.Result, into) != nil {
		return ErrUnexpected
	}
	return nil
}

// present reports whether a JSON value was given, and is not null.
func present(value json.RawMessage) bool {
	return len(value) > 0 && !bytes.Equal(value, []byte("null"))
}
