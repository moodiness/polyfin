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
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Lifetime is how long a request stays usable, approved or not.
const Lifetime = 10 * time.Minute

// maxPending bounds memory if requests are initiated faster than they expire.
const maxPending = 1000

var (
	ErrUnknown = errors.New("unknown or expired Quick Connect request")
	ErrFull    = errors.New("too many pending Quick Connect requests")
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
	// User is set once a signed-in user approved the code.
	User *accounts.ID
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
}

func New() *Store {
	return NewAt(time.Now)
}

// NewAt is a store whose requests expire by the clock now.
func NewAt(now func() time.Time) *Store {
	return &Store{now: now, bySecret: map[string]*Request{}, byCode: map[string]*Request{}}
}

// Initiate registers a Jellyfin app's request for the device and app
// described.
func (s *Store) Initiate(deviceID, deviceName, appName, appVersion string) (Request, error) {
	return s.initiate(Request{Kind: Jellyfin, DeviceID: deviceID, DeviceName: deviceName, AppName: appName, AppVersion: appVersion})
}

// InitiateSilo registers the request of one of Silo's apps.
func (s *Store) InitiateSilo(deviceID, deviceName, devicePlatform, appName, appVersion string) (Request, error) {
	return s.initiate(Request{Kind: Silo, DeviceID: deviceID, DeviceName: deviceName, DevicePlatform: devicePlatform,
		AppName: appName, AppVersion: appVersion})
}

func (s *Store) initiate(request Request) (Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	if len(s.bySecret) >= maxPending {
		return Request{}, ErrFull
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	request.Secret = strings.ToUpper(hex.EncodeToString(raw))
	request.CreatedAt = s.now()
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

// Clear cancels every request, when Quick Connect is turned off.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.bySecret)
	clear(s.byCode)
}

func (s *Store) expire() {
	now := s.now()
	for secret, request := range s.bySecret {
		if !now.Before(request.ExpiresAt()) {
			delete(s.bySecret, secret)
			delete(s.byCode, request.Code)
		}
	}
}
