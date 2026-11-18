package accounts

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

// At most as many password hashes run at once as there are turns,
// whatever asks for them: sign-ins of an unknown name, sign-ins of a
// known one and password changes, more of each than there are turns, are
// in flight together, and the hashes under way are counted where Argon2id
// runs. The others wait for a turn, every one of them ends, and the
// right passwords sign in, the new ones after a change.
//
//nolint:paralleltest // replaces the package's Argon2id, which every test hashes with
func TestPasswordHashesAtOnceAreBounded(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	bound := cap(hashTurns)
	if bound < 2 {
		t.Fatalf("%d password hashes at once, want two at least", bound)
	}
	each := bound + 1
	mustCreate(t, store, NewUser{Name: "known", Password: "correct horse"})
	changing := make([]User, each)
	for i := range changing {
		changing[i] = mustCreate(t, store, NewUser{Name: fmt.Sprintf("changing-%d", i), Password: "correct horse"})
	}

	// Each hash is counted from its start to its end. Until the test lets
	// the hashes go, each tells that it started, and is held there.
	checks := 3 * each
	var mu sync.Mutex
	running, peak := 0, 0
	started := make(chan struct{}, checks)
	release := make(chan struct{})
	hash := argonKey
	argonKey = func(password, salt []byte, passes, memory uint32, threads uint8, keyLen uint32) []byte {
		mu.Lock()
		running++
		peak = max(peak, running)
		mu.Unlock()
		defer func() {
			mu.Lock()
			running--
			mu.Unlock()
		}()
		select {
		case started <- struct{}{}:
			<-release
		case <-release:
		}
		return hash(password, salt, passes, memory, threads, keyLen)
	}
	t.Cleanup(func() { argonKey = hash })

	var wg sync.WaitGroup
	for i := range each {
		wg.Go(func() {
			if _, err := store.Authenticate(ctx, "nobody-here", "a guess"); !errors.Is(err, ErrInvalidCredentials) {
				t.Errorf("sign-in of an unknown name: %v", err)
			}
		})
		wg.Go(func() {
			if user, err := store.Authenticate(ctx, "known", "correct horse"); err != nil || user.Name != "known" {
				t.Errorf("sign-in with the right password: %q, %v", user.Name, err)
			}
		})
		wg.Go(func() {
			if err := store.ChangePassword(ctx, changing[i].ID, "correct horse", "battery staple", nil); err != nil {
				t.Errorf("password change %d: %v", i, err)
			}
		})
	}
	for range bound {
		<-started
	}
	// The hashes under way hold every turn, so the other checks wait for
	// one. Were a turn left, nothing would hold them: the test waits for
	// their hashes to start too, and the count below tells.
	if len(hashTurns) < bound {
		for range checks - bound {
			<-started
		}
	}
	close(release)
	wg.Wait()

	mu.Lock()
	atOnce, left := peak, running
	mu.Unlock()
	if atOnce != bound {
		t.Errorf("%d password hashes ran at once, want %d: one for each turn, the others waiting", atOnce, bound)
	}
	if left != 0 || len(hashTurns) != 0 {
		t.Errorf("after the checks: %d hashes under way holding %d turns, want none", left, len(hashTurns))
	}
	if _, err := store.Authenticate(ctx, "known", "correct horse"); err != nil {
		t.Errorf("the right password after the checks: %v", err)
	}
	for _, user := range changing {
		if _, err := store.Authenticate(ctx, user.Name, "battery staple"); err != nil {
			t.Errorf("%s: the new password: %v", user.Name, err)
		}
		if _, err := store.Authenticate(ctx, user.Name, "correct horse"); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("%s: the old password: %v", user.Name, err)
		}
	}
}
