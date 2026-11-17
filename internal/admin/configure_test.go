package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/playback"
)

// configureAPI is the admin API with configure links signed by signer,
// logging everything to log, and the addons a test configures: a
// configurable server addon, one that is not, and the member's own.
type configureAPI struct {
	testAPI
	signer      playback.Signer
	log         *syncBuffer
	addonStore  *addons.Store
	addonServer *httptest.Server
	// hinting has the addon server's /secret/ manifests declare a
	// configure page; true at first.
	hinting       *atomic.Bool
	administrator browser
	member        browser
	memberUser    accounts.User
	adminUser     accounts.User
	configurable  string
	plain         string
	mine          string
}

func newConfigureAPI(t *testing.T) configureAPI {
	t.Helper()
	hinting := new(atomic.Bool)
	hinting.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hints := ""
		if strings.HasPrefix(r.URL.Path, "/secret/") && hinting.Load() {
			hints = `, "behaviorHints": {"configurable": true}`
		}
		_, _ = w.Write([]byte(`{"id": "top` + r.URL.Path + `", "name": "Top", "version": "1.0.0", "resources": ["stream"]` + hints + `}`))
	}))
	t.Cleanup(server.Close)
	c := configureAPI{signer: playback.NewSigner([]byte("configure test secret")), log: new(syncBuffer), addonServer: server, hinting: hinting}
	c.testAPI = newTestAPI(t, 10, func(o *Options, deps testDeps) {
		o.Signer = &c.signer
		o.Logger = slog.New(slog.NewTextHandler(c.log, &slog.HandlerOptions{Level: slog.LevelDebug}))
		c.addonStore = deps.addons
	})
	c.administrator = c.signedIn("administrator", true)
	c.member = c.signedIn("member", false)
	users, err := c.testAPI.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		switch u.Name {
		case "member":
			c.memberUser = u
		case "administrator":
			c.adminUser = u
		}
	}
	install := func(path string) string {
		t.Helper()
		status, body, _ := c.administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": server.URL + path})
		if status != http.StatusCreated {
			t.Fatalf("installing %s: %d %v", path, status, body)
		}
		return body["id"].(string)
	}
	c.configurable = install("/secret/manifest.json?token=k")
	c.plain = install("/plain/manifest.json")
	// The member's own addon, installed as an administrator would reach it.
	own, err := c.addonStore.Install(t.Context(), addons.Personal(c.memberUser.ID), server.URL+"/secret/mine/manifest.json?token=m", false)
	if err != nil {
		t.Fatal(err)
	}
	c.mine = own.ID.String()
	return c
}

// links lists the addons of scope as b sees them, failing on an answer
// that holds an addon's address: each addon's configure link, by ID.
func (c configureAPI) links(b browser, path string) map[string]string {
	c.t.Helper()
	response, data := b.fetch(http.MethodGet, path, "", nil)
	if response.StatusCode != http.StatusOK {
		c.t.Fatalf("%s: %d", path, response.StatusCode)
	}
	for _, secret := range []string{"token=k", "token=m", "/secret/", "/plain/"} {
		if strings.Contains(string(data), secret) {
			c.t.Errorf("%s holds an address: %s", path, data)
		}
	}
	var listed []struct {
		ID            string `json:"id"`
		Configurable  bool   `json:"configurable"`
		ConfigureLink string `json:"configureLink"`
	}
	if strings.HasPrefix(string(data), "{") {
		var sources struct {
			Addons json.RawMessage `json:"addons"`
		}
		if err := json.Unmarshal(data, &sources); err != nil {
			c.t.Fatal(err)
		}
		data = sources.Addons
	}
	if err := json.Unmarshal(data, &listed); err != nil {
		c.t.Fatal(err)
	}
	result := map[string]string{}
	for _, l := range listed {
		if l.ConfigureLink != "" && !l.Configurable {
			c.t.Errorf("%s: a link for an addon without a configure page", l.ID)
		}
		result[l.ID] = l.ConfigureLink
	}
	return result
}

// open opens link as a browser without any cookie, following no
// redirect, in language: the status, the redirect, and the page.
func (c configureAPI) open(link, language string) (int, string, string, http.Header) {
	c.t.Helper()
	header := map[string]string{}
	if language != "" {
		header["Accept-Language"] = language
	}
	return c.send(http.MethodGet, link, header)
}

// post presses Continue on link's page, as a browser without any cookie
// posts the form from Polyfin's own origin, following no redirect.
func (c configureAPI) post(link string) (int, string, string, http.Header) {
	c.t.Helper()
	return c.send(http.MethodPost, link, map[string]string{"Origin": c.url, "Sec-Fetch-Site": "same-origin"})
}

// send requests link with header, without any cookie, following no
// redirect: the status, the redirect, and the page.
func (c configureAPI) send(method, link string, header map[string]string) (int, string, string, http.Header) {
	c.t.Helper()
	client := &http.Client{Transport: testTransport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, _ := http.NewRequestWithContext(c.t.Context(), method, c.url+link, nil)
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for name, value := range header {
		request.Header.Set(name, value)
	}
	response, err := client.Do(request)
	if err != nil {
		c.t.Fatal(err)
	}
	defer response.Body.Close()
	page, _ := io.ReadAll(response.Body)
	return response.StatusCode, response.Header.Get("Location"), string(page), response.Header
}

// token signs a configure token as Polyfin would, for the tests that
// forge one for another addon or user.
func (c configureAPI) token(shared bool, addon string, user accounts.ID, expires time.Time) string {
	c.t.Helper()
	id, err := accounts.ParseID(addon)
	if err != nil {
		c.t.Fatal(err)
	}
	h := &handler{Options: Options{Signer: &c.signer}}
	return configureLinkPath + h.signConfigure(configureGrant{shared: shared, addon: id, user: user, expires: expires})
}

// The addon JSON carries a configure link for whoever may replace the
// address. Opened from a browser with no session, it answers Polyfin's
// own page, naming the addon and the host of its configure page, never a
// redirect; Continue, posted from that page, leads to the configure page,
// once. The address is in no answer but that redirect, and nothing is
// logged.
func TestAConfigureLinkOpensAPageWhoseContinueLeadsOnce(t *testing.T) {
	t.Parallel()
	c := newConfigureAPI(t)
	shared := c.links(c.administrator, "/scopes/shared/addons")
	if !strings.HasPrefix(shared[c.configurable], configureLinkPath) || shared[c.plain] != "" {
		t.Fatalf("server addons' links: %v", shared)
	}
	own := c.links(c.member, "/scopes/me/addons")
	if !strings.HasPrefix(own[c.mine], configureLinkPath) {
		t.Fatalf("the member's own addon's link: %v", own)
	}
	// An administrator sees the member's own addon among every source,
	// without a link: they may not replace its address.
	if every := c.links(c.administrator, "/sources"); every[c.mine] != "" {
		t.Errorf("an administrator got a link for a user's own addon: %v", every)
	}

	host := strings.TrimPrefix(c.addonServer.URL, "http://")
	for link, want := range map[string]string{
		shared[c.configurable]: c.addonServer.URL + "/secret/configure?token=k",
		own[c.mine]:            c.addonServer.URL + "/secret/mine/configure?token=m",
	} {
		status, location, page, header := c.open(link, "")
		if status != http.StatusOK || location != "" {
			t.Errorf("opening a link: %d %q, want the page", status, location)
		}
		if !strings.Contains(page, "Polyfin will open the configure page of Top, on "+host+".") ||
			!strings.Contains(page, `<form method="post" action="`+link+`"><button type="submit">Continue</button></form>`) {
			t.Errorf("the page: %s", page)
		}
		if strings.Contains(page, "/secret/") || strings.Contains(page, "token=") || strings.Contains(page, "<script") {
			t.Errorf("the page holds the address or a script: %s", page)
		}
		policy := header.Get("Content-Security-Policy")
		if !strings.Contains(policy, "default-src 'none'") || !strings.Contains(policy, "form-action 'self' "+c.addonServer.URL+";") ||
			header.Get("Cache-Control") != "no-store" || header.Get("Referrer-Policy") != "same-origin" {
			t.Errorf("the page's headers: %v", header)
		}

		status, location, _, header = c.post(link)
		if status != http.StatusSeeOther || location != want {
			t.Errorf("Continue: %d %q, want %q", status, location, want)
		}
		if header.Get("Referrer-Policy") != "no-referrer" || header.Get("Cache-Control") != "no-store" {
			t.Errorf("Continue's headers: %v", header)
		}
		// The link is used up.
		if status, location, page, _ := c.post(link); status != http.StatusNotFound || location != "" || !strings.Contains(page, "This link isn’t valid.") {
			t.Errorf("Continue again: %d %q", status, location)
		}
		if status, _, _, _ := c.open(link, ""); status != http.StatusNotFound {
			t.Errorf("opening a used link: %d", status)
		}
	}
	// The old route that needed the session is gone.
	if status, _, _ := c.administrator.call(http.MethodGet, "/scopes/shared/addons/"+c.configurable+"/configure", nil); status != http.StatusNotFound {
		t.Errorf("the old route: %d", status)
	}

	logged := c.log.String()
	for _, secret := range []string{strings.TrimPrefix(shared[c.configurable], configureLinkPath), strings.TrimPrefix(own[c.mine], configureLinkPath),
		"token=k", "token=m", "/secret/"} {
		if strings.Contains(logged, secret) {
			t.Errorf("the log holds %q: %s", secret, logged)
		}
	}
}

// A link past its 10 minutes, or changed, opens a page that says so, in
// the browser's language, and nothing else.
func TestAnExpiredOrChangedConfigureLinkTellsSo(t *testing.T) {
	t.Parallel()
	c := newConfigureAPI(t)
	link := c.links(c.administrator, "/scopes/shared/addons")[c.configurable]

	encoded, mark, _ := strings.Cut(strings.TrimPrefix(link, configureLinkPath), ".")
	flipped := []byte(encoded)
	flipped[5] ^= 1
	for name, changed := range map[string]string{
		"payload":   configureLinkPath + string(flipped) + "." + mark,
		"signature": configureLinkPath + encoded + "." + strings.Repeat("A", len(mark)),
		"truncated": configureLinkPath + encoded,
		"garbage":   configureLinkPath + "not-a-token",
	} {
		status, location, page, _ := c.open(changed, "")
		if status != http.StatusNotFound || location != "" || !strings.Contains(page, "This link isn’t valid.") {
			t.Errorf("%s changed: %d %q %s", name, status, location, page)
		}
	}

	later := time.Now().Add(configureLinkLife + time.Minute)
	c.clock.Store(&later)
	for language, want := range map[string]string{
		"":                  "This link has expired.",
		"xx-YY, tlh":        "This link has expired.",
		"fr-CA,fr;q=0.9":    "Ce lien a expiré.",
		"de-AT, en;q=0.5":   "Dieser Link ist abgelaufen.",
		"pt;q=0.9, en;q=.5": "Este link expirou.",
	} {
		status, location, page, header := c.open(link, language)
		if status != http.StatusGone || location != "" || !strings.Contains(page, want) {
			t.Errorf("expired, in %q: %d %q %s", language, status, location, page)
		}
		if strings.Contains(page, "<script") || strings.Contains(page, "src=") || strings.Contains(page, "href=") {
			t.Errorf("the page loads or runs something: %s", page)
		}
		if !strings.HasPrefix(header.Get("Content-Type"), "text/html") || !strings.Contains(header.Get("Content-Security-Policy"), "default-src 'none'") {
			t.Errorf("headers: %v", header)
		}
	}
	// Continue on a page opened before the link expired is refused too.
	if status, location, page, _ := c.post(link); status != http.StatusGone || location != "" || !strings.Contains(page, "This link has expired.") {
		t.Errorf("Continue on an expired link: %d %q", status, location)
	}
	// An expired link that was also changed is only not valid.
	if status, _, _, _ := c.open(configureLinkPath+string(flipped)+"."+mark, ""); status != http.StatusNotFound {
		t.Errorf("an expired and changed link: %d", status)
	}
}

// Opening a link checks again who may replace the address, and that the
// addon still has a configure page; a link signed for another addon or
// user opens nothing.
func TestAConfigureLinkChecksTheRightsAgain(t *testing.T) {
	t.Parallel()
	c := newConfigureAPI(t)
	boss := c.signedIn("boss", true)
	sharedLink := c.links(c.administrator, "/scopes/shared/addons")[c.configurable]
	bossLink := c.links(boss, "/scopes/shared/addons")[c.configurable]
	ownLink := c.links(c.member, "/scopes/me/addons")[c.mine]
	invalid := func(name, link string) {
		t.Helper()
		status, location, page, _ := c.open(link, "")
		if status != http.StatusNotFound || location != "" || !strings.Contains(page, "This link isn’t valid.") {
			t.Errorf("%s: %d %q %s", name, status, location, page)
		}
		// Continue checks the link as the page does.
		if status, location, _, _ := c.post(link); status != http.StatusNotFound || location != "" {
			t.Errorf("%s, Continue: %d %q", name, status, location)
		}
	}
	soon := time.Now().Add(configureLinkLife)

	// Tokens Polyfin signed, but for another addon or user.
	invalid("the member, for a server addon", c.token(true, c.configurable, c.memberUser.ID, soon))
	invalid("a server addon as the member's own", c.token(false, c.configurable, c.memberUser.ID, soon))
	invalid("the administrator, for the member's addon", c.token(false, c.mine, c.adminUser.ID, soon))
	nobody := accounts.ID{0x0f, 0x0f}
	invalid("a user who does not exist", c.token(true, c.configurable, nobody, soon))
	invalid("an addon that does not exist", c.token(true, nobody.String(), c.adminUser.ID, soon))
	invalid("an addon without a configure page", c.token(true, c.plain, c.adminUser.ID, soon))

	// A disabled user, then the administrator demoted and the member
	// without their own addons.
	if _, err := c.testAPI.store.UpdateUser(t.Context(), c.memberUser.ID, accounts.UserChanges{IsDisabled: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	invalid("a disabled member", ownLink)
	if _, err := c.testAPI.store.UpdateUser(t.Context(), c.memberUser.ID, accounts.UserChanges{IsDisabled: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	if status, _, _, _ := c.open(ownLink, ""); status != http.StatusOK {
		t.Fatalf("the member enabled again: %d", status)
	}
	// An ended account, as a disabled one.
	today := accounts.Day(time.Now())
	if _, err := c.testAPI.store.UpdateUser(t.Context(), c.memberUser.ID, accounts.UserChanges{EndsOn: &today}, nil); err != nil {
		t.Fatal(err)
	}
	invalid("an ended member", ownLink)
	if _, err := c.testAPI.store.UpdateUser(t.Context(), c.memberUser.ID, accounts.UserChanges{EndsOn: new(time.Time{})}, nil); err != nil {
		t.Fatal(err)
	}
	if status, _, _, _ := c.open(ownLink, ""); status != http.StatusOK {
		t.Fatalf("the member's account no longer ending: %d", status)
	}
	if _, err := c.testAPI.store.UpdateUser(t.Context(), c.adminUser.ID, accounts.UserChanges{IsAdministrator: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	invalid("a demoted administrator", sharedLink)
	if _, err := c.testAPI.store.UpdateUser(t.Context(), c.memberUser.ID, accounts.UserChanges{PersonalAddons: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	invalid("a member without their own addons", ownLink)

	// The addon no longer has a configure page.
	if status, _, _, _ := c.open(bossLink, ""); status != http.StatusOK {
		t.Fatalf("the other administrator's link: %d", status)
	}
	c.hinting.Store(false)
	id, _ := accounts.ParseID(c.configurable)
	if _, _, err := c.addonStore.Refresh(t.Context(), addons.Shared(), id, false); err != nil {
		t.Fatal(err)
	}
	invalid("an addon no longer configurable", bossLink)
}

// Continue must come from the page Polyfin answered: a post from another
// origin, one the browser says is not same-origin, or one without an
// Origin is refused, and the link stays usable.
func TestContinueIsRefusedFromAnotherOrigin(t *testing.T) {
	t.Parallel()
	c := newConfigureAPI(t)
	link := c.links(c.administrator, "/scopes/shared/addons")[c.configurable]
	for name, header := range map[string]map[string]string{
		"another origin":        {"Origin": "https://elsewhere.example", "Sec-Fetch-Site": "cross-site"},
		"another origin alone":  {"Origin": "https://elsewhere.example"},
		"a null origin":         {"Origin": "null", "Sec-Fetch-Site": "cross-site"},
		"a same-site page":      {"Origin": c.url, "Sec-Fetch-Site": "same-site"},
		"no origin":             {},
		"no origin, same-site":  {"Sec-Fetch-Site": "same-site"},
		"no origin, user typed": {"Sec-Fetch-Site": "none"},
	} {
		status, location, _, _ := c.send(http.MethodPost, link, header)
		if status != http.StatusForbidden || location != "" {
			t.Errorf("%s: %d %q", name, status, location)
		}
	}
	if status, location, _, _ := c.post(link); status != http.StatusSeeOther || location == "" {
		t.Errorf("Continue from the page afterwards: %d %q", status, location)
	}
}
