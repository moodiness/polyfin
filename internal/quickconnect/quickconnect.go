// Package quickconnect holds Quick Connect requests: a TV or phone app shows
// a 6-digit code, a signed-in user approves it, and the app then signs in as
// that user with the secret only it knows.
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

	"github.com/moodiness/polyfin/internal/accounts"
)

// Lifetime is how long a request stays usable, approved or not.
const Lifetime = 10 * time.Minute

// maxPending bounds memory if requests are initiated faster than they expire.
const maxPending = 1000

var (
	ErrUnknown = errors.New("unknown or expired Quick Connect request")
	ErrFull    = errors.New("too many pending Quick Connect requests")
)

// Request is a pending or approved sign-in.
type Request struct {
	Secret     string
	Code       string
	DeviceID   string
	DeviceName string
	AppName    string
	AppVersion string
	CreatedAt  time.Time
	// User is set once a signed-in user approved the code.
	User *accounts.ID
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
	return &Store{now: time.Now, bySecret: map[string]*Request{}, byCode: map[string]*Request{}}
}

// Initiate registers a new request for the device and app described.
func (s *Store) Initiate(deviceID, deviceName, appName, appVersion string) (Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	if len(s.bySecret) >= maxPending {
		return Request{}, ErrFull
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	request := &Request{
		Secret:     strings.ToUpper(hex.EncodeToString(raw)),
		DeviceID:   deviceID,
		DeviceName: deviceName,
		AppName:    appName,
		AppVersion: appVersion,
		CreatedAt:  s.now(),
	}
	for {
		n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
		if err != nil {
			return Request{}, err
		}
		request.Code = fmt.Sprintf("%06d", n.Int64())
		if _, taken := s.byCode[request.Code]; !taken {
			break
		}
	}
	s.bySecret[request.Secret] = request
	s.byCode[request.Code] = request
	return *request, nil
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

// ByCode returns the request behind a code, so the approving user can see
// which device asked before approving it.
func (s *Store) ByCode(code string) (Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	request, ok := s.byCode[strings.TrimSpace(code)]
	if !ok {
		return Request{}, ErrUnknown
	}
	return *request, nil
}

// Authorize approves a code for a user.
func (s *Store) Authorize(code string, user accounts.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	request, ok := s.byCode[strings.TrimSpace(code)]
	if !ok {
		return ErrUnknown
	}
	request.User = &user
	return nil
}

// Clear cancels every request, when Quick Connect is turned off.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.bySecret)
	clear(s.byCode)
}

func (s *Store) expire() {
	cutoff := s.now().Add(-Lifetime)
	for secret, request := range s.bySecret {
		if request.CreatedAt.Before(cutoff) {
			delete(s.bySecret, secret)
			delete(s.byCode, request.Code)
		}
	}
}
