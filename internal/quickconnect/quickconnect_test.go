package quickconnect

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// clock is a store's clock the test moves.
type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

// home is where the tests' devices ask from.
var home = Origin{Address: "192.168.1.20", Local: true}

func TestCodesOfEachKind(t *testing.T) {
	t.Parallel()
	store := New()
	jellyfin, err := store.Initiate(home, "phone", "Phone", "Jellyfin", "1")
	if err != nil {
		t.Fatal(err)
	}
	silo, err := store.InitiateSilo(home, "tv", "TV", "tvos", "Silo", "2")
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
	t.Parallel()
	store := New()
	alice, bob := accounts.ID{1}, accounts.ID{2}
	approved, _ := store.InitiateSilo(home, "tv", "TV", "tvos", "Silo", "2")
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

	denied, _ := store.Initiate(home, "phone", "Phone", "Jellyfin", "1")
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
	t.Parallel()
	store := New()
	user := accounts.ID{1}
	jellyfin, _ := store.Initiate(home, "phone", "Phone", "Jellyfin", "1")
	silo, _ := store.InitiateSilo(home, "tv", "TV", "tvos", "Silo", "2")
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
	t.Parallel()
	store := New()
	pending, _ := store.InitiateSilo(home, "tv", "TV", "tvos", "Silo", "2")
	if state, err := store.Cancel(pending.Secret); err != nil || state != Canceled {
		t.Fatalf("cancel: %v %v", state, err)
	}
	if err := store.Authorize(pending.Code, accounts.ID{1}); !errors.Is(err, ErrDecided) {
		t.Errorf("approving a withdrawn code: %v", err)
	}

	approved, _ := store.InitiateSilo(home, "tv", "TV", "tvos", "Silo", "2")
	_ = store.Authorize(approved.Code, accounts.ID{1})
	if state, _ := store.Cancel(approved.Secret); state != Canceled {
		t.Errorf("an approval not used yet: %v", state)
	}
	if _, err := store.Redeem(approved.Secret); !errors.Is(err, ErrUnknown) {
		t.Errorf("signing in once withdrawn: %v", err)
	}

	used, _ := store.InitiateSilo(home, "tv", "TV", "tvos", "Silo", "2")
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
	t.Parallel()
	c := &clock{now: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)}
	store := NewAt(c.Now)
	pending, _ := store.InitiateSilo(home, "tv", "TV", "tvos", "Silo", "2")
	approved, _ := store.InitiateSilo(home, "tv2", "TV", "tvos", "Silo", "2")
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

// waiting reports whether a request is still in the store.
func waiting(store *Store, request Request) bool {
	_, err := store.BySecret(request.Secret)
	return err == nil
}

// A request keeps where it was opened from. One client address holds
// maxPerClient requests: its next one takes the place of its oldest, and
// never of another client's, who still gets a code. The addresses of one
// IPv6 /64 are one client.
func TestAClientHoldsItsShareOfRequests(t *testing.T) {
	t.Parallel()
	store := New()
	tv, err := store.Initiate(home, "tv", "Living room TV", "Jellyfin", "1")
	if err != nil {
		t.Fatal(err)
	}
	if found, _ := store.ByCode(tv.Code); found.Origin != home {
		t.Errorf("the request was opened from %+v, want %+v", found.Origin, home)
	}

	// One address opens many requests, each as another device.
	busy := Origin{Address: "203.0.113.9"}
	var opened []Request
	for n := range 3 * maxPerClient {
		request, err := store.Initiate(busy, fmt.Sprintf("made-up-%d", n), "TV", "Jellyfin", "1")
		if err != nil {
			t.Fatalf("request %d of one address: %v", n+1, err)
		}
		opened = append(opened, request)
	}
	for n, request := range opened {
		if kept := n >= len(opened)-maxPerClient; waiting(store, request) != kept {
			t.Errorf("request %d of %d of one address: waiting %v, want %v", n+1, len(opened), !kept, kept)
		}
	}
	if !waiting(store, tv) {
		t.Error("another address's request gave way to them")
	}
	// Another address, and another device, still get a code.
	if phone, err := store.Initiate(Origin{Address: "198.51.100.7"}, "phone", "Phone", "Jellyfin", "1"); err != nil || len(phone.Code) != 6 {
		t.Errorf("another address's request: %v", err)
	}

	// The addresses of one /64 share their requests; the next /64 has its own.
	var first Request
	for n := range maxPerClient + 1 {
		request, err := store.Initiate(Origin{Address: fmt.Sprintf("2001:db8:0:1::%x", n+1)}, fmt.Sprintf("v6-%d", n), "TV", "Jellyfin", "1")
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			first = request
		}
	}
	if waiting(store, first) {
		t.Errorf("one /64 holds more than %d requests", maxPerClient)
	}
	neighbour, _ := store.Initiate(Origin{Address: "2001:db8:0:2::1"}, "v6-neighbour", "TV", "Jellyfin", "1")
	if _, err := store.Initiate(Origin{Address: "2001:db8:0:1::ffff"}, "v6-last", "TV", "Jellyfin", "1"); err != nil || !waiting(store, neighbour) {
		t.Errorf("the next /64's request gave way: %v", err)
	}
	if !SameClient("2001:db8:0:1::1", "2001:db8:0:1::2") || SameClient("2001:db8:0:1::1", "2001:db8:0:2::1") ||
		!SameClient("::ffff:192.168.1.20", home.Address) || SameClient("192.168.1.20", "192.168.1.21") || SameClient("", "") {
		t.Error("SameClient does not count clients as the store does")
	}
}

// A device has one request: its new one replaces its last, whatever
// became of it, and leaves the other devices of its address theirs. The
// same identifier from another address is another client's device.
func TestADevicesNewRequestReplacesItsLast(t *testing.T) {
	t.Parallel()
	store := New()
	phone, _ := store.Initiate(home, "phone", "Phone", "Jellyfin", "1")
	elsewhere, _ := store.Initiate(Origin{Address: "203.0.113.9"}, "tv", "TV", "Jellyfin", "1")
	first, _ := store.Initiate(home, "tv", "TV", "Jellyfin", "1")
	if err := store.Authorize(first.Code, accounts.ID{1}); err != nil {
		t.Fatal(err)
	}
	var last Request
	for range 2 * maxPerClient {
		previous := last
		var err error
		if last, err = store.Initiate(home, "tv", "TV", "Jellyfin", "1"); err != nil {
			t.Fatal(err)
		}
		if previous.Secret != "" && waiting(store, previous) {
			t.Fatal("a device's request outlived its next one")
		}
	}
	if waiting(store, first) {
		t.Error("a device's approved request outlived its next one")
	}
	if _, err := store.ByCode(first.Code); !errors.Is(err, ErrUnknown) {
		t.Errorf("the code of a replaced request: %v", err)
	}
	if !waiting(store, last) || !waiting(store, phone) || !waiting(store, elsewhere) {
		t.Error("a device's requests took the place of another device's")
	}
}

// A full store gives way to a new request: the oldest request ends, and
// no request is refused for the number of the others.
func TestAFullStoreGivesWayToANewRequest(t *testing.T) {
	t.Parallel()
	store := New()
	var opened []Request
	for n := range maxPending {
		request, err := store.Initiate(Origin{Address: fmt.Sprintf("10.%d.%d.1", n/250, n%250)}, "tv", "TV", "Jellyfin", "1")
		if err != nil {
			t.Fatalf("request %d: %v", n+1, err)
		}
		opened = append(opened, request)
	}
	for n, request := range opened {
		if !waiting(store, request) {
			t.Fatalf("request %d ended before the store was full", n+1)
		}
	}
	latest, err := store.InitiateSilo(home, "tv", "Living room TV", "tvos", "Silo", "2")
	if err != nil || !waiting(store, latest) {
		t.Fatalf("a request while the store is full: %v", err)
	}
	if waiting(store, opened[0]) {
		t.Error("the oldest request did not give way")
	}
	if _, err := store.ByCode(opened[0].Code); !errors.Is(err, ErrUnknown) {
		t.Errorf("the code of the request that gave way: %v", err)
	}
	for n, request := range opened[1:] {
		if !waiting(store, request) {
			t.Fatalf("request %d gave way too", n+2)
		}
	}
}

// Withdraw takes back what a user approved, used or not, and nothing
// else: the requests stand as denied.
func TestWithdrawEndsWhatAUserApproved(t *testing.T) {
	t.Parallel()
	store := New()
	alice, bob := accounts.ID{1}, accounts.ID{2}
	waits, _ := store.InitiateSilo(home, "tv", "TV", "tvos", "Silo", "2")
	used, _ := store.Initiate(home, "phone", "Phone", "Jellyfin", "1")
	others, _ := store.Initiate(home, "tablet", "Tablet", "Jellyfin", "1")
	pending, _ := store.Initiate(home, "console", "Console", "Jellyfin", "1")
	for request, user := range map[*Request]accounts.ID{&waits: alice, &used: alice, &others: bob} {
		if err := store.Authorize(request.Code, user); err != nil {
			t.Fatal(err)
		}
	}
	// A Jellyfin app's request stays approved once it signed in.
	if _, err := store.Redeem(used.Secret); err != nil {
		t.Fatal(err)
	}
	store.Withdraw(alice)
	for name, request := range map[string]Request{"an approval not used yet": waits, "an approval used": used} {
		if _, err := store.Redeem(request.Secret); !errors.Is(err, ErrUnknown) {
			t.Errorf("signing in with %s, once withdrawn: %v", name, err)
		}
		if found, err := store.BySecret(request.Secret); err != nil || found.State != Denied || found.User != nil {
			t.Errorf("%s, once withdrawn: %v for %v, %v, want denied", name, found.State, found.User, err)
		}
		if err := store.Authorize(request.Code, bob); !errors.Is(err, ErrDecided) {
			t.Errorf("approving %s again, once withdrawn: %v", name, err)
		}
	}
	if _, err := store.Redeem(others.Secret); err != nil {
		t.Errorf("another user's approval: %v", err)
	}
	if found, err := store.ByCode(pending.Code); err != nil || found.State != Pending {
		t.Errorf("a request no one approved: %v %v", found.State, err)
	}
}

// A code is decided once, and the secret of one of Silo's apps signs in
// once, when sixteen requests race for each; a Jellyfin app's signs in
// each time, until it expires.
func TestRequestsAreDecidedAndCollectedOnceAtOnce(t *testing.T) {
	t.Parallel()
	store := New()
	request, err := store.InitiateSilo(home, "tv-id", "TV", "tvos", "Silo", "1.0")
	if err != nil {
		t.Fatal(err)
	}
	const racers = 16
	race := func(try func(i int) bool) int {
		var wins atomic.Int32
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range racers {
			wg.Go(func() {
				<-start
				if try(i) {
					wins.Add(1)
				}
			})
		}
		close(start)
		wg.Wait()
		return int(wins.Load())
	}
	if won := race(func(i int) bool { return store.Authorize(request.Code, accounts.ID{byte(i + 1)}) == nil }); won != 1 {
		t.Errorf("one code was approved for %d users at once, want 1", won)
	}
	if won := race(func(int) bool { _, err := store.Redeem(request.Secret); return err == nil }); won != 1 {
		t.Errorf("one secret of Silo's apps signed in %d times at once, want 1", won)
	}
	jellyfin, err := store.Initiate(home, "phone-id", "Phone", "Jellyfin", "1.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Authorize(jellyfin.Code, accounts.ID{1}); err != nil {
		t.Fatal(err)
	}
	if won := race(func(int) bool { _, err := store.Redeem(jellyfin.Secret); return err == nil }); won != racers {
		t.Errorf("a Jellyfin app's secret signed in %d times of %d", won, racers)
	}
	// Requests opened at once, by one client, leave it its share.
	var opened sync.Map
	race(func(i int) bool {
		request, err := store.Initiate(Origin{Address: "203.0.113.9"}, fmt.Sprintf("racer-%d", i), "TV", "Jellyfin", "1")
		opened.Store(i, request)
		return err == nil
	})
	held := 0
	opened.Range(func(_, request any) bool {
		if waiting(store, request.(Request)) {
			held++
		}
		return true
	})
	if held != maxPerClient {
		t.Errorf("one client holds %d requests opened at once, want %d", held, maxPerClient)
	}
}
