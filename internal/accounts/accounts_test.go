package accounts

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/testdb"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func mustCreate(t *testing.T, store *Store, user NewUser) User {
	t.Helper()
	created, err := store.CreateUser(t.Context(), user)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func device(id string) DeviceInfo {
	return DeviceInfo{DeviceID: id, DeviceName: "TV", Client: "Test", ClientVersion: "1.0", RemoteAddress: "192.0.2.1"}
}

func signedIn(ctx context.Context, store *Store, token string) bool {
	_, _, err := store.DeviceByToken(ctx, token, "192.0.2.1")
	return err == nil
}

func TestAuthenticateMatchesNamesWithoutCaseAndHidesDisabledAccounts(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "Alice", Password: "correct horse"})

	if user, err := store.Authenticate(ctx, "  aLiCe ", "correct horse"); err != nil || user.ID != alice.ID {
		t.Fatalf("case-insensitive sign-in failed: %v", err)
	}
	for name, password := range map[string]string{"alice": "wrong password", "nobody": "correct horse"} {
		if _, err := store.Authenticate(ctx, name, password); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("%s/%s: got %v, want invalid credentials", name, password, err)
		}
	}

	disabled := true
	if _, err := store.UpdateUser(ctx, alice.ID, UserChanges{IsDisabled: &disabled}, nil); err != nil {
		t.Fatal(err)
	}
	// A wrong password must not reveal that the account exists but is disabled.
	if _, err := store.Authenticate(ctx, "alice", "wrong password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("disabled account with wrong password: got %v", err)
	}
	if _, err := store.Authenticate(ctx, "alice", "correct horse"); !errors.Is(err, ErrDisabled) {
		t.Errorf("disabled account with right password: got %v", err)
	}
}

func TestUserNamesAreUniqueWithoutCaseAndValidated(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	mustCreate(t, store, NewUser{Name: "Alice", Password: "correct horse"})
	if _, err := store.CreateUser(ctx, NewUser{Name: "ALICE", Password: "correct horse"}); !errors.Is(err, ErrNameTaken) {
		t.Errorf("duplicate name: got %v", err)
	}
	for _, name := range []string{"", "   ", "a/b", "line\nbreak", string(make([]byte, 65))} {
		if _, err := store.CreateUser(ctx, NewUser{Name: name, Password: "correct horse"}); !errors.Is(err, ErrInvalidName) {
			t.Errorf("name %q: got %v", name, err)
		}
	}
	if _, err := store.CreateUser(ctx, NewUser{Name: "Bob", Password: "short"}); !errors.Is(err, ErrInvalidPassword) {
		t.Errorf("short password: got %v", err)
	}
	if _, err := store.CreateUser(ctx, NewUser{Name: "Zoé O'Neil-Ü.2", Password: "correct horse"}); err != nil {
		t.Errorf("valid international name refused: %v", err)
	}
}

func TestSettingsRoundTripAndRefuseUnknownLanguages(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	if got := store.Settings(); got.Language != "en" || !got.Chapters || got.PrepareAhead ||
		got.CatalogLimit != DefaultCatalogLimit || got.ChannelLimit != DefaultChannelLimit {
		t.Errorf("defaults: language %q, chapters %v, prepare ahead %v, catalog limit %d, channel limit %d",
			got.Language, got.Chapters, got.PrepareAhead, got.CatalogLimit, got.ChannelLimit)
	}
	if DefaultCatalogLimit != 2000 || DefaultChannelLimit != 10000 {
		t.Errorf("default limits: %d and %d", DefaultCatalogLimit, DefaultChannelLimit)
	}
	want := Settings{ServerName: "Maison", QuickConnectEnabled: false, LegacyAuthorization: true, Language: "fr", Chapters: false, PrepareAhead: true,
		CatalogLimit: 5000, ChannelLimit: 30000}
	want.PlayedPercent, want.ResumePercent = DefaultPlayedPercent, DefaultResumePercent
	want.VersionListMinutes, want.CatalogRefreshMinutes = DefaultVersionListMinutes, DefaultCatalogRefreshMinutes
	want.AnalysisTimeout, want.VersionAttempts, want.PreferDirectPlay, want.MaxConversions, want.MaxConversionHeight = 30, 5, true, 4, 720
	want.Trickplay, want.TrickplayInterval, want.TrickplayWidth, want.ChapterImages, want.ThumbnailStorageGB = true, 20, 480, true, 7
	want.LiveTvRefreshHours = 36
	if _, err := store.UpdateSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, store.db)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Settings(); got != want {
		t.Errorf("settings after reopening: %+v, want %+v", got, want)
	}
	for _, language := range []string{"", "de", "FR", "fr-FR"} {
		changed := want
		changed.Language = language
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidLanguage) {
			t.Errorf("language %q: got %v", language, err)
		}
	}
	if got := store.Settings(); got != want {
		t.Errorf("a refused update changed the settings: %+v", got)
	}
}

func TestSettingsKeepCatalogLimitsInRange(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	for _, tc := range []struct {
		catalog, channel int
		err              error
	}{
		{MinCatalogLimit - 1, DefaultChannelLimit, ErrInvalidCatalogLimit},
		{MaxCatalogLimit + 1, DefaultChannelLimit, ErrInvalidCatalogLimit},
		{0, DefaultChannelLimit, ErrInvalidCatalogLimit},
		{DefaultCatalogLimit, MinChannelLimit - 1, ErrInvalidChannelLimit},
		{DefaultCatalogLimit, MaxChannelLimit + 1, ErrInvalidChannelLimit},
		{DefaultCatalogLimit, -1, ErrInvalidChannelLimit},
	} {
		changed := store.Settings()
		changed.CatalogLimit, changed.ChannelLimit = tc.catalog, tc.channel
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, tc.err) {
			t.Errorf("limits %d and %d: got %v, want %v", tc.catalog, tc.channel, err, tc.err)
		}
	}
	if got := store.Settings(); got.CatalogLimit != DefaultCatalogLimit || got.ChannelLimit != DefaultChannelLimit {
		t.Errorf("a refused update changed the limits: %d and %d", got.CatalogLimit, got.ChannelLimit)
	}
	for _, limits := range [][2]int{{MinCatalogLimit, MinChannelLimit}, {MaxCatalogLimit, MaxChannelLimit}} {
		changed := store.Settings()
		changed.CatalogLimit, changed.ChannelLimit = limits[0], limits[1]
		if _, err := store.UpdateSettings(ctx, changed); err != nil {
			t.Fatalf("limits %v: %v", limits, err)
		}
		reopened, err := Open(ctx, store.db)
		if err != nil {
			t.Fatal(err)
		}
		if got := reopened.Settings(); got.CatalogLimit != limits[0] || got.ChannelLimit != limits[1] {
			t.Errorf("limits %v after reopening: %d and %d", limits, got.CatalogLimit, got.ChannelLimit)
		}
	}
}

func TestContentSettingsDefaultRoundTripAndStayInRange(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	defaults := store.Settings()
	if !defaults.SkipButtons || !defaults.SimilarTitles || defaults.PlayedPercent != 90 || defaults.ResumePercent != 5 ||
		defaults.VersionListMinutes != 10 || defaults.CatalogRefreshMinutes != 10 {
		t.Errorf("defaults: %+v", defaults)
	}
	if DefaultPlayedPercent != 90 || DefaultResumePercent != 5 || DefaultVersionListMinutes != 10 || DefaultCatalogRefreshMinutes != 10 {
		t.Error("the default constants are not today's behavior")
	}
	want := defaults
	want.SkipButtons, want.SimilarTitles = false, false
	want.PlayedPercent, want.ResumePercent = 95, 20
	want.VersionListMinutes, want.CatalogRefreshMinutes = 60, 720
	if _, err := store.UpdateSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, store.db)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Settings(); got != want {
		t.Errorf("after reopening: %+v, want %+v", got, want)
	}

	for _, tc := range []struct {
		name                    string
		played, resume          int
		versionList, catalogRef int
		err                     error
	}{
		{"played below its range", MinPlayedPercent - 1, 5, 10, 10, ErrInvalidPlayedPercent},
		{"played above its range", MaxPlayedPercent + 1, 5, 10, 10, ErrInvalidPlayedPercent},
		{"resume below its range", 90, MinResumePercent - 1, 10, 10, ErrInvalidResumePercent},
		{"resume above its range", 90, MaxResumePercent + 1, 10, 10, ErrInvalidResumePercent},
		{"resume equal to played", 50, 50, 10, 10, ErrResumeNotBelowPlayed},
		{"version lists below their range", 90, 5, MinVersionListMinutes - 1, 10, ErrInvalidVersionListMinutes},
		{"version lists above their range", 90, 5, MaxVersionListMinutes + 1, 10, ErrInvalidVersionListMinutes},
		{"catalogs below their range", 90, 5, 10, MinCatalogRefreshMinutes - 1, ErrInvalidCatalogRefreshMinutes},
		{"catalogs above their range", 90, 5, 10, MaxCatalogRefreshMinutes + 1, ErrInvalidCatalogRefreshMinutes},
	} {
		changed := store.Settings()
		changed.PlayedPercent, changed.ResumePercent = tc.played, tc.resume
		changed.VersionListMinutes, changed.CatalogRefreshMinutes = tc.versionList, tc.catalogRef
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, tc.err) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.err)
		}
	}
	if got := store.Settings(); got != want {
		t.Errorf("a refused update changed the settings: %+v", got)
	}
	for _, bounds := range [][4]int{
		{MinPlayedPercent, MinResumePercent, MinVersionListMinutes, MinCatalogRefreshMinutes},
		{MaxPlayedPercent, MaxResumePercent, MaxVersionListMinutes, MaxCatalogRefreshMinutes},
		{MinPlayedPercent, MinPlayedPercent - 1, 10, 10},
	} {
		changed := store.Settings()
		changed.PlayedPercent, changed.ResumePercent, changed.VersionListMinutes, changed.CatalogRefreshMinutes = bounds[0], bounds[1], bounds[2], bounds[3]
		if _, err := store.UpdateSettings(ctx, changed); err != nil {
			t.Errorf("bounds %v: %v", bounds, err)
		}
	}
	// The database refuses what the store would, should anything else
	// write the settings.
	if _, err := store.db.Exec(ctx, "UPDATE settings SET resume_percent = 50, played_percent = 50"); err == nil {
		t.Error("the database took a resume percent equal to the played percent")
	}
}

func TestSetupStoresTheAdministratorsLanguage(t *testing.T) {
	for _, tc := range []struct{ given, want string }{{"fr", "fr"}, {"en", "en"}, {"de", "en"}, {"", "en"}} {
		store := newStore(t)
		if _, err := store.CreateFirstAdministrator(t.Context(), "admin", "correct horse", tc.given); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(t.Context(), store.db)
		if err != nil {
			t.Fatal(err)
		}
		if cached, stored := store.Settings().Language, reopened.Settings().Language; cached != tc.want || stored != tc.want {
			t.Errorf("setup in %q: language %q (stored %q), want %q", tc.given, cached, stored, tc.want)
		}
		if cached, stored := store.Settings(), reopened.Settings(); cached != stored || cached.CatalogLimit != DefaultCatalogLimit {
			t.Errorf("setup in %q: settings %+v, stored %+v", tc.given, cached, stored)
		}
	}
	// A refused setup changes nothing.
	store := newStore(t)
	if _, err := store.CreateFirstAdministrator(t.Context(), "admin", "short", "fr"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("setup with a short password: %v", err)
	}
	if got := store.Settings().Language; got != "en" {
		t.Errorf("a refused setup set the language to %q", got)
	}
}

func TestTheLastEnabledAdministratorIsProtected(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	admin, err := store.CreateFirstAdministrator(ctx, "admin", "correct horse", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateFirstAdministrator(ctx, "second", "correct horse", ""); !errors.Is(err, ErrSetupComplete) {
		t.Fatalf("second setup: got %v", err)
	}
	no, yes := false, true
	for name, change := range map[string]UserChanges{
		"demote":  {IsAdministrator: &no},
		"disable": {IsDisabled: &yes},
	} {
		if _, err := store.UpdateUser(ctx, admin.ID, change, nil); !errors.Is(err, ErrLastAdministrator) {
			t.Errorf("%s the last administrator: got %v", name, err)
		}
	}
	if err := store.DeleteUser(ctx, admin.ID); !errors.Is(err, ErrLastAdministrator) {
		t.Errorf("delete the last administrator: got %v", err)
	}
	// A disabled administrator does not count as a remaining one.
	other := mustCreate(t, store, NewUser{Name: "other", Password: "correct horse", IsAdministrator: true})
	if _, err := store.UpdateUser(ctx, other.ID, UserChanges{IsDisabled: &yes}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteUser(ctx, admin.ID); !errors.Is(err, ErrLastAdministrator) {
		t.Errorf("delete with only a disabled administrator left: got %v", err)
	}
	if _, err := store.UpdateUser(ctx, other.ID, UserChanges{IsDisabled: &no}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteUser(ctx, admin.ID); err != nil {
		t.Errorf("delete with another enabled administrator: %v", err)
	}
}

func TestSigningInAgainFromADeviceReplacesOnlyItsToken(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	bob := mustCreate(t, store, NewUser{Name: "bob", Password: "correct horse"})

	first, firstDevice, _ := store.SignInDevice(ctx, alice.ID, device("tv"))
	phone, _, _ := store.SignInDevice(ctx, alice.ID, device("phone"))
	bobOnTV, _, _ := store.SignInDevice(ctx, bob.ID, device("tv"))
	second, secondDevice, err := store.SignInDevice(ctx, alice.ID, device("tv"))
	if err != nil {
		t.Fatal(err)
	}

	if signedIn(ctx, store, first) {
		t.Error("the previous token of the device still works")
	}
	if !signedIn(ctx, store, second) || !signedIn(ctx, store, phone) || !signedIn(ctx, store, bobOnTV) {
		t.Error("a token of another device or user was revoked")
	}
	if firstDevice.ID != secondDevice.ID {
		t.Error("the device lost its identity on a new sign-in")
	}
	// Apps may send the token in any letter case.
	if !signedIn(ctx, store, strings.ToUpper(second)) {
		t.Error("an upper-case token was refused")
	}
	if err := store.SignOutDevice(ctx, second); err != nil || signedIn(ctx, store, second) {
		t.Errorf("sign-out kept the token (%v)", err)
	}
}

func TestNewPasswordSignsOutEverywhereExceptTheCurrentSession(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	token, _, _ := store.SignInDevice(ctx, alice.ID, device("tv"))
	current, _, _ := store.CreateAdminSession(ctx, alice.ID)
	other, _, _ := store.CreateAdminSession(ctx, alice.ID)

	if err := store.ChangePassword(ctx, alice.ID, "wrong password", "battery staple", HashAdminToken(current)); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("wrong current password: got %v", err)
	}
	if err := store.ChangePassword(ctx, alice.ID, "correct horse", "battery staple", HashAdminToken(current)); err != nil {
		t.Fatal(err)
	}
	if signedIn(ctx, store, token) {
		t.Error("a device kept its token after the password change")
	}
	if _, err := store.AdminSession(ctx, other); !errors.Is(err, ErrNotFound) {
		t.Error("another admin session survived the password change")
	}
	if _, err := store.AdminSession(ctx, current); err != nil {
		t.Errorf("the session that changed the password was signed out: %v", err)
	}
	if _, err := store.Authenticate(ctx, "alice", "battery staple"); err != nil {
		t.Errorf("new password refused: %v", err)
	}
}

// Every way a device can be signed out tells of it, once, with the
// devices it signed out only.
func TestSignedOutDevicesAreToldOf(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	var told [][]ID
	store.OnSignOut(func(devices []ID) { told = append(told, devices) })
	expect := func(what string, want ...Device) {
		t.Helper()
		ids := make([]ID, 0, len(want))
		for _, d := range want {
			ids = append(ids, d.ID)
		}
		slices.SortFunc(ids, func(a, b ID) int { return bytes.Compare(a[:], b[:]) })
		if len(told) != 1 {
			t.Fatalf("%s: told %d times", what, len(told))
		}
		got := slices.SortedFunc(slices.Values(told[0]), func(a, b ID) int { return bytes.Compare(a[:], b[:]) })
		if !slices.Equal(got, ids) {
			t.Errorf("%s: told of %v, want %v", what, got, ids)
		}
		told = nil
	}
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	bob := mustCreate(t, store, NewUser{Name: "bob", Password: "correct horse"})
	tvToken, tv, _ := store.SignInDevice(ctx, alice.ID, device("tv"))
	_, phone, _ := store.SignInDevice(ctx, alice.ID, device("phone"))
	_, tablet, _ := store.SignInDevice(ctx, alice.ID, device("tablet"))
	_, bobTV, _ := store.SignInDevice(ctx, bob.ID, device("tv"))

	if err := store.SignOutDevice(ctx, tvToken); err != nil {
		t.Fatal(err)
	}
	expect("signing out", tv)
	if err := store.RevokeDevice(ctx, bob.ID, phone.ID); !errors.Is(err, ErrNotFound) || told != nil {
		t.Fatalf("revoking another user's device: %v, told of %v", err, told)
	}
	if err := store.RevokeDevice(ctx, alice.ID, phone.ID); err != nil {
		t.Fatal(err)
	}
	expect("revoking", phone)
	_, phone, _ = store.SignInDevice(ctx, alice.ID, device("phone"))
	if err := store.ChangePasswordFromDevice(ctx, alice.ID, "correct horse", "battery staple", phone.ID); err != nil {
		t.Fatal(err)
	}
	expect("changing the password from the phone", tablet)
	if _, err := store.UpdateUser(ctx, alice.ID, UserChanges{IsHidden: new(true)}, nil); err != nil || told != nil {
		t.Fatalf("hiding: %v, told of %v", err, told)
	}
	if _, err := store.UpdateUser(ctx, alice.ID, UserChanges{IsDisabled: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	expect("disabling", phone)
	if err := store.DeleteUser(ctx, bob.ID); err != nil {
		t.Fatal(err)
	}
	expect("deleting", bobTV)
}

func TestAdminSessionsExpireAndRenew(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	token, _, _ := store.CreateAdminSession(ctx, alice.ID)

	if session, err := store.AdminSession(ctx, token); err != nil || session.Renewed {
		t.Fatalf("fresh session: renewed=%v err=%v", session.Renewed, err)
	}
	if _, err := store.db.Exec(ctx, "UPDATE admin_sessions SET expires_at = now() + interval '1 day'"); err != nil {
		t.Fatal(err)
	}
	if session, err := store.AdminSession(ctx, token); err != nil || !session.Renewed {
		t.Fatalf("old session: renewed=%v err=%v", session.Renewed, err)
	}
	if _, err := store.db.Exec(ctx, "UPDATE admin_sessions SET expires_at = now() - interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AdminSession(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session: got %v", err)
	}
}

func TestPasswordHashes(t *testing.T) {
	hash := hashPassword("correct horse")
	if ok, err := verifyPassword("correct horse", hash); !ok || err != nil {
		t.Fatalf("right password refused: %v", err)
	}
	if ok, _ := verifyPassword("correct horsf", hash); ok {
		t.Fatal("wrong password accepted")
	}
	if hashPassword("correct horse") == hash {
		t.Fatal("two hashes of one password are identical: the salt is not random")
	}
	if _, err := verifyPassword("x", "$argon2id$v=19$m=1,t=1,p=1$bad"); err == nil {
		t.Fatal("malformed hash accepted")
	}
}
