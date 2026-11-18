package throttle

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// clocked returns failures on a clock the test moves.
func clocked(limit int) (*Failures, *time.Time) {
	f := New(limit, time.Minute)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	f.now = func() time.Time { return now }
	return f, &now
}

// keys returns how many keys f keeps.
func keys(f *Failures) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.failures)
}

// looksAt reserves an attempt of the keys given, takes it back, and
// returns how many of the keys kept it looked at beyond its own.
func looksAt(t *testing.T, f *Failures, reserved ...string) int {
	t.Helper()
	f.mu.Lock()
	before := f.visited
	f.mu.Unlock()
	attempt, wait := f.Reserve(reserved...)
	if attempt == nil {
		t.Fatalf("an attempt of %v was refused for %s", reserved, wait)
	}
	f.mu.Lock()
	looked := f.visited - before
	f.mu.Unlock()
	attempt.Cancel()
	return looked
}

// Attempts made at once, none of them settled yet, are refused past the
// limit.
func TestReserveCountsAttemptsAtOnce(t *testing.T) {
	t.Parallel()
	f := New(3, time.Minute)
	start := make(chan struct{})
	var mu sync.Mutex
	reserved := 0
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			<-start
			if attempt, _ := f.Reserve("client"); attempt != nil {
				mu.Lock()
				reserved++
				mu.Unlock()
			}
		})
	}
	close(start)
	wg.Wait()
	if reserved != 3 {
		t.Errorf("%d attempts reserved at once with a limit of 3", reserved)
	}
}

func TestSettlingAnAttempt(t *testing.T) {
	t.Parallel()
	f, now := clocked(2)
	failed, _ := f.Reserve("client")
	failed.Fail()
	// Cancel after Fail changes nothing: a deferred Cancel only takes back
	// an attempt left unsettled.
	failed.Cancel()
	cancelled, _ := f.Reserve("client")
	cancelled.Cancel()
	last, _ := f.Reserve("client")
	if last == nil {
		t.Fatal("refused with one failure and an attempt taken back")
	}
	if attempt, wait := f.Reserve("client"); attempt != nil || wait != time.Minute {
		t.Fatalf("with one failure and one attempt under way: %v, wait %v", attempt, wait)
	}
	// A success takes back its own attempt, and leaves the failure before
	// it counted.
	last.Succeed()
	next, _ := f.Reserve("client")
	if next == nil {
		t.Fatal("refused after a success, with one failure")
	}
	next.Fail()
	if attempt, _ := f.Reserve("client"); attempt != nil {
		t.Fatal("a success forgot the failure before it")
	}

	// A failure expires after the window.
	other, _ := f.Reserve("other")
	other.Fail()
	*now = now.Add(30 * time.Second)
	again, _ := f.Reserve("other")
	again.Fail()
	if attempt, wait := f.Reserve("other"); attempt != nil || wait != 30*time.Second {
		t.Fatalf("at the limit: %v, wait %v", attempt, wait)
	}
	*now = now.Add(30 * time.Second)
	if attempt, _ := f.Reserve("other"); attempt == nil {
		t.Error("refused once the first failure expired")
	}
}

// An attempt of several keys is counted for all of them, or for none when
// one of them is at its limit.
func TestReserveSeveralKeys(t *testing.T) {
	t.Parallel()
	f, _ := clocked(1)
	user, _ := f.Reserve("user")
	user.Fail()
	if attempt, _ := f.Reserve("client", "user"); attempt != nil {
		t.Fatal("reserved past the user's limit")
	}
	if attempt, _ := f.Reserve("client"); attempt == nil {
		t.Fatal("a refused attempt counted toward the client")
	}
}

// A success between failures changes nothing to their count, for any key
// of the attempts: the client and the account are refused at the limit.
func TestASuccessKeepsTheOtherFailuresOfItsKeys(t *testing.T) {
	t.Parallel()
	f, _ := clocked(3)
	for i := range 3 {
		wrong, _ := f.Reserve("192.0.2.1", "two-step:alice")
		if wrong == nil {
			t.Fatalf("failure %d was refused", i+1)
		}
		wrong.Fail()
		if i == 2 {
			break
		}
		// The client's own sign-in, to another account.
		right, _ := f.Reserve("192.0.2.1", "two-step:bob")
		if right == nil {
			t.Fatalf("a sign-in was refused after %d failures", i+1)
		}
		right.Succeed()
	}
	if attempt, wait := f.Reserve("192.0.2.1"); attempt != nil || wait != time.Minute {
		t.Errorf("the client after 3 failures and 2 successes between them: %v, wait %v", attempt, wait)
	}
	if attempt, _ := f.Reserve("two-step:alice"); attempt != nil {
		t.Error("the account after 3 failures was not refused")
	}
	// The successes left nothing counted.
	if attempt, _ := f.Reserve("two-step:bob"); attempt == nil {
		t.Error("the account of the successes was refused")
	}
	if kept := keys(f); kept != 3 {
		t.Errorf("%d keys kept, want the client, the account that failed and the attempt under way", kept)
	}
}

// The addresses of one IPv6 /64 share one count. Those of two /64 do not,
// nor do IPv4 addresses, IPv4-mapped ones or keys that are no address.
func TestIPv6AddressesCountByTheir64(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name          string
		first, second string
		together      bool
	}{
		{"one /64", "2001:db8:aaaa:bbbb::1", "2001:db8:aaaa:bbbb:ffff:ffff:ffff:ffff", true},
		{"one /64 written otherwise", "2001:db8::1", "2001:0DB8:0:0:1:0:0:0", true},
		{"one /64 with zones", "fe80::1%eth0", "fe80::2%eth1", true},
		{"two /64", "2001:db8:aaaa:bbbb::1", "2001:db8:aaaa:bbbc::1", false},
		{"IPv4", "192.0.2.1", "192.0.2.2", false},
		{"IPv4-mapped", "::ffff:192.0.2.1", "::ffff:192.0.2.2", false},
		{"IPv4 and the same IPv4-mapped", "192.0.2.1", "::ffff:192.0.2.1", false},
		{"no address", "two-step:1", "two-step:2", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f, _ := clocked(2)
			// An attempt taken back does not count for the others either.
			right, _ := f.Reserve(c.first)
			right.Succeed()
			for i := range 2 {
				wrong, _ := f.Reserve(c.first)
				if wrong == nil {
					t.Fatalf("failure %d of %s was refused", i+1, c.first)
				}
				wrong.Fail()
			}
			if attempt, _ := f.Reserve(c.first); attempt != nil {
				t.Fatalf("%s was not refused after 2 failures", c.first)
			}
			attempt, _ := f.Reserve(c.second)
			if refused := attempt == nil; refused != c.together {
				t.Errorf("%s after 2 failures of %s: refused %v, want %v", c.second, c.first, refused, c.together)
			}
		})
	}
}

// No more keys are kept than allowed, however many fail inside the window,
// and none of them is refused for it: the address whose last attempt is
// the oldest gives way to a new key, and no other key does while an
// address is kept.
func TestTheKeysKeptHaveABound(t *testing.T) {
	t.Parallel()
	const bound = 100
	f, now := clocked(1)
	f.maxKeys = bound
	fail := func(key string) {
		t.Helper()
		attempt, wait := f.Reserve(key)
		if attempt == nil {
			t.Fatalf("%s was refused for %s with %d keys kept", key, wait, keys(f))
		}
		attempt.Fail()
		if kept := keys(f); kept > bound {
			t.Fatalf("%d keys kept after %s, want %d at most", kept, key, bound)
		}
	}
	address := func(i int) string { return fmt.Sprintf("198.51.%d.%d", i/256, i%256) }
	fail("two-step:alice")
	// All inside the window of a minute.
	const addresses = 10 * bound
	for i := range addresses {
		*now = now.Add(time.Millisecond)
		fail(address(i))
	}
	if kept := keys(f); kept != bound {
		t.Errorf("%d keys kept, want %d", kept, bound)
	}
	if attempt, _ := f.Reserve("two-step:alice"); attempt != nil {
		t.Error("the account gave way to addresses")
	}
	// The last bound-1 addresses are kept beside the account.
	if attempt, _ := f.Reserve(address(addresses - bound + 1)); attempt != nil {
		t.Error("an address among the last ones gave way")
	}
	if attempt, _ := f.Reserve(address(addresses - bound)); attempt == nil {
		t.Error("the address before them is still counted")
	} else {
		attempt.Cancel()
	}

	// With no address kept, the oldest of the other keys gives way.
	names, _ := clocked(1)
	names.maxKeys = 3
	for _, name := range []string{"two-step:1", "two-step:2", "two-step:3", "two-step:4"} {
		attempt, _ := names.Reserve(name)
		if attempt == nil {
			t.Fatalf("%s was refused", name)
		}
		attempt.Fail()
	}
	if kept := keys(names); kept != 3 {
		t.Errorf("%d keys kept, want 3", kept)
	}
	if attempt, _ := names.Reserve("two-step:2"); attempt != nil {
		t.Error("the second key gave way")
	}
	if attempt, _ := names.Reserve("two-step:1"); attempt == nil {
		t.Error("the oldest key is still counted")
	}
}

// An attempt looks at a few of the keys kept, however many there are, and
// forgets a few of those whose failures have all expired: none walks them
// all, and all are forgotten in the end.
func TestAnAttemptLooksAtFewOfTheKeysKept(t *testing.T) {
	t.Parallel()
	// The oldest keys of each order, addresses and others, at most.
	const few = 2 * expiredAtOnce
	f, now := clocked(10)
	if f.maxKeys != keysKept {
		t.Fatalf("New keeps %d keys at most, want %d", f.maxKeys, keysKept)
	}
	const prefixes = 11_000
	for i := range prefixes {
		attempt, wait := f.Reserve(fmt.Sprintf("2001:db8:%x:%x::1", i>>16, i&0xffff))
		if attempt == nil {
			t.Fatalf("the /64 number %d was refused for %s", i, wait)
		}
		attempt.Fail()
		// A second later for each thousand, all inside the window.
		if i%1000 == 999 {
			*now = now.Add(time.Second)
		}
	}
	if kept := keys(f); kept != prefixes {
		t.Fatalf("%d keys kept, want %d", kept, prefixes)
	}
	for range 100 {
		if looked := looksAt(t, f, "192.0.2.1", "two-step:alice"); looked > few {
			t.Fatalf("an attempt looked at %d keys of %d that all count, want %d at most", looked, prefixes, few)
		}
	}
	if kept := keys(f); kept != prefixes {
		t.Fatalf("%d keys kept after attempts taken back, want %d", kept, prefixes)
	}

	// Once the window has passed, the attempts forget them few by few.
	*now = now.Add(time.Minute)
	for i := range prefixes / expiredAtOnce {
		if looked := looksAt(t, f, "192.0.2.1", "two-step:alice"); looked > few {
			t.Fatalf("attempt %d looked at %d keys of %d expired ones, want %d at most", i+1, looked, keys(f), few)
		}
	}
	if kept := keys(f); kept != 0 {
		t.Errorf("%d expired keys still kept after %d attempts", kept, prefixes/expiredAtOnce)
	}
}
