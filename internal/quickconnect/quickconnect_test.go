package quickconnect

import (
	"errors"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// clock is a store's clock the test moves.
type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func TestCodesOfEachKind(t *testing.T) {
	store := New()
	jellyfin, err := store.Initiate("phone", "Phone", "Jellyfin", "1")
	if err != nil {
		t.Fatal(err)
	}
	silo, err := store.InitiateSilo("tv", "TV", "tvos", "Silo", "2")
	if err != nil {
		t.Fatal(err)
	}
	if len(jellyfin.Code) != 6 || len(silo.Code) != 8 || silo.Kind != Silo || silo.DevicePlatform != "tvos" {
		t.Fatalf("codes %q and %q", jellyfin.Code, silo.Code)
	}
	// As the TV shows it, in two groups.
	if found, err := store.ByCode(" " + silo.Code[:4] + "-" + silo.Code[4:] + " "); err != nil || found.Secret != silo.Secret {
		t.Errorf("a grouped code: %v %v", found.Secret, err)
	}
}

// A code is decided once: approved for one user, another cannot take it,
// nor deny it; a denied one is approved by no one.
func TestDecidedOnce(t *testing.T) {
	store := New()
	alice, bob := accounts.ID{1}, accounts.ID{2}
	approved, _ := store.InitiateSilo("tv", "TV", "tvos", "Silo", "2")
	if err := store.Authorize(approved.Code, alice); err != nil {
		t.Fatal(err)
	}
	for name, err := range map[string]error{"approval": store.Authorize(approved.Code, bob), "denial": store.Deny(approved.Code)} {
		if !errors.Is(err, ErrDecided) {
			t.Errorf("another %s: %v", name, err)
		}
	}
	if found, _ := store.BySecret(approved.Secret); found.State != Approved || *found.User != alice {
		t.Errorf("approved for %v, %v", found.User, found.State)
	}

	denied, _ := store.Initiate("phone", "Phone", "Jellyfin", "1")
	if err := store.Deny(denied.Code); err != nil {
		t.Fatal(err)
	}
	if err := store.Authorize(denied.Code, alice); !errors.Is(err, ErrDecided) {
		t.Errorf("approving a denied code: %v", err)
	}
	if _, err := store.Redeem(denied.Secret); !errors.Is(err, ErrUnknown) {
		t.Errorf("signing in with a denied code: %v", err)
	}
	if err := store.Authorize("12345", alice); !errors.Is(err, ErrUnknown) {
		t.Errorf("an unknown code: %v", err)
	}
}

// A Jellyfin app signs in with its approved secret until the request
// expires, one of Silo's apps once.
func TestRedeem(t *testing.T) {
	store := New()
	user := accounts.ID{1}
	jellyfin, _ := store.Initiate("phone", "Phone", "Jellyfin", "1")
	silo, _ := store.InitiateSilo("tv", "TV", "tvos", "Silo", "2")
	for _, request := range []Request{jellyfin, silo} {
		if _, err := store.Redeem(request.Secret); !errors.Is(err, ErrUnknown) {
			t.Errorf("before approval: %v", err)
		}
		if err := store.Authorize(request.Code, user); err != nil {
			t.Fatal(err)
		}
		if redeemed, err := store.Redeem(request.Secret); err != nil || *redeemed.User != user {
			t.Errorf("after approval: %v", err)
		}
	}
	if _, err := store.Redeem(jellyfin.Secret); err != nil {
		t.Errorf("a Jellyfin app again: %v", err)
	}
	if _, err := store.Redeem(silo.Secret); !errors.Is(err, ErrUnknown) {
		t.Errorf("a Silo app again: %v", err)
	}
	if found, _ := store.BySecret(silo.Secret); found.State != Collected {
		t.Errorf("state %v", found.State)
	}
}

// A request withdrawn is approved by no one, nor signed in with; one
// already used stays used.
func TestCancel(t *testing.T) {
	store := New()
	pending, _ := store.InitiateSilo("tv", "TV", "tvos", "Silo", "2")
	if state, err := store.Cancel(pending.Secret); err != nil || state != Canceled {
		t.Fatalf("cancel: %v %v", state, err)
	}
	if err := store.Authorize(pending.Code, accounts.ID{1}); !errors.Is(err, ErrDecided) {
		t.Errorf("approving a withdrawn code: %v", err)
	}

	approved, _ := store.InitiateSilo("tv", "TV", "tvos", "Silo", "2")
	_ = store.Authorize(approved.Code, accounts.ID{1})
	if state, _ := store.Cancel(approved.Secret); state != Canceled {
		t.Errorf("an approval not used yet: %v", state)
	}
	if _, err := store.Redeem(approved.Secret); !errors.Is(err, ErrUnknown) {
		t.Errorf("signing in once withdrawn: %v", err)
	}

	used, _ := store.InitiateSilo("tv", "TV", "tvos", "Silo", "2")
	_ = store.Authorize(used.Code, accounts.ID{1})
	_, _ = store.Redeem(used.Secret)
	if state, _ := store.Cancel(used.Secret); state != Collected {
		t.Errorf("a used request: %v", state)
	}
	if _, err := store.Cancel("unknown"); !errors.Is(err, ErrUnknown) {
		t.Errorf("an unknown request: %v", err)
	}
}

// Requests end after their lifetime, approved or not.
func TestExpiry(t *testing.T) {
	c := &clock{now: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)}
	store := NewAt(c.Now)
	pending, _ := store.InitiateSilo("tv", "TV", "tvos", "Silo", "2")
	approved, _ := store.InitiateSilo("tv2", "TV", "tvos", "Silo", "2")
	_ = store.Authorize(approved.Code, accounts.ID{1})
	if pending.ExpiresAt() != c.now.Add(Lifetime) {
		t.Errorf("expires at %v", pending.ExpiresAt())
	}
	c.now = c.now.Add(Lifetime - time.Second)
	if _, err := store.BySecret(pending.Secret); err != nil {
		t.Errorf("before its end: %v", err)
	}
	c.now = c.now.Add(time.Second)
	if err := store.Authorize(pending.Code, accounts.ID{1}); !errors.Is(err, ErrUnknown) {
		t.Errorf("approving an expired code: %v", err)
	}
	if _, err := store.Redeem(approved.Secret); !errors.Is(err, ErrUnknown) {
		t.Errorf("signing in with an expired approval: %v", err)
	}
}
