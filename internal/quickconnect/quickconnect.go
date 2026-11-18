// Package quickconnect holds Quick Connect requests: a TV or phone app shows
// a code, a signed-in user approves it, and the app then signs in as that
// user with the secret only it knows. Jellyfin apps show a 6-digit code;
// Silo's apps, which sign a TV in through the same requests (see package
// silo), take an 8-digit one.
package quickconnect

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/netip"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Lifetime is how long a request stays usable, approved or not.
const Lifetime = 10 * time.Minute

// maxPending bounds memory if requests are initiated faster than they
// expire: past it, the oldest request gives way to a new one.
const maxPending = 1000

// maxPerClient is how many requests one client address holds at a time,
// whatever their state: past it, its oldest gives way to its new one, so
// that one client cannot take the place of the others' requests.
const maxPerClient = 5

var (
	ErrUnknown = errors.New("unknown or expired Quick Connect request")
	// ErrDecided is a request already approved, denied, withdrawn or used:
	// it is decided once, as Jellyfin refuses to authorize a request twice.
	ErrDecided = errors.New("already decided Quick Connect request")
)

// Kind is the apps a request comes from, which decides its code and how
// often it signs in.
type Kind int

const (
	// Jellyfin apps show a 6-digit code, and sign in with their secret
	// until the request expires.
	Jellyfin Kind = iota
	// Silo's apps take an 8-digit code, and collect their sign-in once.
	Silo
)

// digits is the length of the code of each kind.
var digits = map[Kind]int{Jellyfin: 6, Silo: 8}

// State is where a request stands.
type State int

const (
	Pending State = iota
	// Approved by a signed-in user, for themselves.
	Approved
	// Denied by a signed-in user.
	Denied
	// Canceled by the app that asked.
	Canceled
	// Collected: one of Silo's apps signed in with it.
	Collected
)

// Origin is where a request was opened from.
type Origin struct {
	// Address is the client's address, as forwarded.Client tells it.
	Address string
	// Local reports whether that address is on the local network, as
	// forwarded.InLocalNetwork tells it.
	Local bool
}

// Request is a pending or decided sign-in.
type Request struct {
	Kind       Kind
	State      State
	Secret     string
	Code       string
	DeviceID   string
	DeviceName string
	// DevicePlatform is the platform Silo's apps name, as tvos.
	DevicePlatform string
	AppName        string
	AppVersion     string
	CreatedAt      time.Time
	// Origin is where the app asked from. The names above are the app's
	// own words; the address is what the server saw.
	Origin Origin
	// User is set once a signed-in user approved the code.
	User *accounts.ID

	// client is the client the request counts toward (see clientOf), and
	// order its rank among the requests opened: the lowest is the oldest.
	client string
	order  uint64
}

// ExpiresAt is when the request ends, decided or not.
func (r Request) ExpiresAt() time.Time {
	return r.CreatedAt.Add(Lifetime)
}

// Store keeps requests in memory: a restart cancels pending sign-ins, which
// apps then simply start again.
type Store struct {
	now func() time.Time

	mu       sync.Mutex
	bySecret map[string]*Request
	byCode   map[string]*Request
	// opened counts the requests ever opened, which orders them.
	opened uint64
}

func New() *Store {
	return NewAt(time.Now)
}

// NewAt is a store whose requests expire by the clock now.
func NewAt(now func() time.Time) *Store {
	return &Store{now: now, bySecret: map[string]*Request{}, byCode: map[string]*Request{}}
}

// clientOf is what the requests of one client are counted by: its address,
// or the /64 of an IPv6 address, which one client holds whole.
func clientOf(address string) string {
	parsed, err := netip.ParseAddr(address)
	if err != nil {
		return address
	}
	parsed = parsed.Unmap().WithZone("")
	if parsed.Is6() {
		if prefix, err := parsed.Prefix(64); err == nil {
			return prefix.String()
		}
	}
	return parsed.String()
}

// SameClient reports whether two addresses are one client's, as the store
// counts clients. An address not known is no one's.
func SameClient(a, b string) bool {
	return a != "" && b != "" && clientOf(a) == clientOf(b)
}

// AttemptKey is the key the codes a user tries are counted by in the
// sign-in throttle, beside their client's address: the Jellyfin API, the
// admin app and Silo's apps look codes up in this one store, and count the
// unknown ones together.
func AttemptKey(user accounts.ID) string {
	return "quick-connect:" + user.String()
}

// SameCode reports whether two codes, as typed, are one code: what an
// approval that names its code twice is checked by.
func SameCode(a, b string) bool {
	code := normalized(a)
	return code != "" && code == normalized(b)
}

// Initiate registers a Jellyfin app's request for the device and app
// described, asked from from.
func (s *Store) Initiate(from Origin, deviceID, deviceName, appName, appVersion string) (Request, error) {
	return s.initiate(Request{Kind: Jellyfin, Origin: from, DeviceID: deviceID, DeviceName: deviceName, AppName: appName, AppVersion: appVersion})
}

// InitiateSilo registers the request of one of Silo's apps.
func (s *Store) InitiateSilo(from Origin, deviceID, deviceName, devicePlatform, appName, appVersion string) (Request, error) {
	return s.initiate(Request{Kind: Silo, Origin: from, DeviceID: deviceID, DeviceName: deviceName, DevicePlatform: devicePlatform,
		AppName: appName, AppVersion: appVersion})
}

// initiate registers a request, and makes room for it first: a device has
// one request, its new one replacing its last; a client address has
// maxPerClient, and the store maxPending, the oldest giving way. A new
// request is never refused for the number of the others.
func (s *Store) initiate(request Request) (Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	request.client = clientOf(request.Origin.Address)
	held := 0
	for _, other := range s.bySecret {
		switch {
		case other.client != request.client:
		case request.DeviceID != "" && other.DeviceID == request.DeviceID:
			s.remove(other)
		default:
			held++
		}
	}
	for ; held >= maxPerClient; held-- {
		s.remove(s.oldest(func(other *Request) bool { return other.client == request.client }))
	}
	for len(s.bySecret) >= maxPending {
		s.remove(s.oldest(func(*Request) bool { return true }))
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	request.Secret = strings.ToUpper(hex.EncodeToString(raw))
	request.CreatedAt = s.now()
	s.opened++
	request.order = s.opened
	length := digits[request.Kind]
	limit := big.NewInt(1)
	for range length {
		limit.Mul(limit, big.NewInt(10))
	}
	for {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return Request{}, err
		}
		request.Code = fmt.Sprintf("%0*d", length, n.Int64())
		if _, taken := s.byCode[request.Code]; !taken {
			break
		}
	}
	s.bySecret[request.Secret] = &request
	s.byCode[request.Code] = &request
	return request, nil
}

// oldest is the request opened first among those counted. There is one.
func (s *Store) oldest(counted func(*Request) bool) *Request {
	var first *Request
	for _, request := range s.bySecret {
		if counted(request) && (first == nil || request.order < first.order) {
			first = request
		}
	}
	return first
}

func (s *Store) remove(request *Request) {
	delete(s.bySecret, request.Secret)
	delete(s.byCode, request.Code)
}

// BySecret returns the request an app polls with its secret.
func (s *Store) BySecret(secret string) (Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	request, ok := s.bySecret[strings.ToUpper(secret)]
	if !ok {
		return Request{}, ErrUnknown
	}
	return *request, nil
}

// normalized is a code as typed, without the spaces and dashes that group
// its digits.
func normalized(code string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || unicode.IsSpace(r) {
			return -1
		}
		return r
	}, code)
}

// ByCode returns the request behind a code, so the approving user can see
// which device asked, and where it stands, before deciding.
func (s *Store) ByCode(code string) (Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	request, ok := s.byCode[normalized(code)]
	if !ok {
		return Request{}, ErrUnknown
	}
	return *request, nil
}

// Authorize approves a pending code for a user.
func (s *Store) Authorize(code string, user accounts.ID) error {
	return s.decide(code, func(request *Request) {
		request.State, request.User = Approved, &user
	})
}

// Deny turns a pending code down: its app signs in with nothing.
func (s *Store) Deny(code string) error {
	return s.decide(code, func(request *Request) { request.State = Denied })
}

func (s *Store) decide(code string, decision func(*Request)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	request, ok := s.byCode[normalized(code)]
	switch {
	case !ok:
		return ErrUnknown
	case request.State != Pending:
		return ErrDecided
	}
	decision(request)
	return nil
}

// Redeem returns the approved request an app signs in with. A Jellyfin
// app's stays valid until it expires, as in Jellyfin, so that an app
// retrying after a network error is not refused; the request of one of
// Silo's apps signs in once, as their contract hands tokens over once.
func (s *Store) Redeem(secret string) (Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	request, ok := s.bySecret[strings.ToUpper(secret)]
	if !ok || request.State != Approved {
		return Request{}, ErrUnknown
	}
	if request.Kind == Silo {
		request.State = Collected
	}
	return *request, nil
}

// Cancel withdraws the request of an app that no longer waits for it, so
// that its code can no longer be approved, nor its approval used. A request
// already denied or used stays as it is. It returns the request's state.
func (s *Store) Cancel(secret string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	request, ok := s.bySecret[strings.ToUpper(secret)]
	if !ok {
		return 0, ErrUnknown
	}
	if request.State == Pending || request.State == Approved {
		request.State = Canceled
	}
	return request.State, nil
}

// Withdraw takes back every approval a user gave, used or not, when their
// account is signed out everywhere: an approval given before does not
// sign a device in after. The requests stand as denied, so that their
// apps are told, and no one approves them again.
func (s *Store) Withdraw(user accounts.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, request := range s.bySecret {
		if request.State == Approved && *request.User == user {
			request.State, request.User = Denied, nil
		}
	}
}

// Clear cancels every request, when Quick Connect is turned off.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.bySecret)
	clear(s.byCode)
}

func (s *Store) expire() {
	now := s.now()
	for _, request := range s.bySecret {
		if !now.Before(request.ExpiresAt()) {
			s.remove(request)
		}
	}
}
