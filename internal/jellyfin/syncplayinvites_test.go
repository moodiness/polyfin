package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// webPlayer signs name in to jellyfin-web on a device of its own, and
// opens its socket.
func (s testServer) webPlayer(t *testing.T, name, device string) syncPlayApp {
	t.Helper()
	header := fmt.Sprintf(`MediaBrowser Client="Jellyfin Web", Device="%s", DeviceId="%s-id", Version="10.11.0"`, device, device)
	status, body := s.call(http.MethodPost, "/Users/AuthenticateByName", header, map[string]string{"Username": name, "Pw": "correct horse"})
	var signedIn AuthenticationResult
	if status != http.StatusOK || json.Unmarshal(body, &signedIn) != nil || signedIn.AccessToken == "" {
		t.Fatalf("%s's web player: %d %s", name, status, body)
	}
	a := syncPlayApp{device: device, token: signedIn.AccessToken}
	var err error
	if a.socket, _, err = s.openSocket(t, a.token); err != nil {
		t.Fatal(err)
	}
	if message := a.socket.next(t); message.MessageType != "ForceKeepAlive" {
		t.Fatalf("first message of %s: %+v", device, message)
	}
	return a
}

// invite is the app's next PolyfinSyncPlayInvite message.
func (a syncPlayApp) invite(t *testing.T) InvitePush {
	t.Helper()
	for {
		if message := a.socket.next(t); message.MessageType == inviteMessage {
			return decodeData[InvitePush](t, message)
		}
	}
}

// noInvite checks that no PolyfinSyncPlayInvite message comes for a while.
func (a syncPlayApp) noInvite(t *testing.T) {
	t.Helper()
	deadline := time.After(300 * time.Millisecond)
	for {
		select {
		case message := <-a.socket.messages:
			if message.MessageType == inviteMessage {
				t.Errorf("unexpected invitation to %s: %+v", a.device, message.Data)
			}
		case <-deadline:
			return
		}
	}
}

// inviteGroup is a host's group, playing the first episode of the test
// catalog, which TV-14 rates, in the Drama genre and the Shows library.
type inviteGroup struct {
	tracking
	host  syncPlayApp
	group string
}

func newInviteGroup(t *testing.T) inviteGroup {
	t.Helper()
	tr := newTracking(t)
	tr.testServer.user("host", nil)
	g := inviteGroup{tracking: tr, host: tr.syncPlayApp(t, "host", "host-tv")}
	g.host.drain(t)
	status, answer := g.host.send(t, tr.testServer, http.MethodPost, "/SyncPlay/New", map[string]any{"GroupName": "Movie night"})
	var info GroupInfoDto
	if status != http.StatusOK || json.Unmarshal(answer, &info) != nil {
		t.Fatalf("new group: %d %s", status, answer)
	}
	g.group = info.GroupId
	g.host.post(t, tr.testServer, "SetNewQueue", map[string]any{"PlayingQueue": []string{tr.episodes[0]}})
	return g
}

// invitees lists the names the host's group may invite.
func (g inviteGroup) invitees(t *testing.T, a syncPlayApp) []string {
	t.Helper()
	status, body := a.send(t, g.testServer, http.MethodGet, "/Polyfin/SyncPlay/Invitees", nil)
	var answer SyncPlayInvitees
	if status != http.StatusOK || json.Unmarshal(body, &answer) != nil || answer.GroupId != g.group {
		t.Fatalf("invitees: %d %s", status, body)
	}
	names := []string{}
	for _, invitee := range answer.Items {
		names = append(names, invitee.Name)
	}
	return names
}

// invite has the host invite a user to group, and gives the answer's
// status and Retry-After.
func (g inviteGroup) invite(t *testing.T, a syncPlayApp, group string, user accounts.User) (int, string) {
	t.Helper()
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, g.url+"/Polyfin/SyncPlay/Invitations",
		strings.NewReader(encode(map[string]any{"GroupId": group, "UserId": user.ID.String()})))
	request.Header.Set("Authorization", app(a.device, a.token))
	request.Header.Set("Content-Type", "application/json")
	response, err := testClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	return response.StatusCode, response.Header.Get("Retry-After")
}

func visible(c *accounts.UserChanges) { c.IsHidden = new(false) }

// A group may invite the users who could join it and play its title now:
// another user, neither disabled nor kept off sign-in screens, whose
// SyncPlay access is Join only or Create and join, inside their allowed
// hours, with a web player open, who may play the title (parental limits,
// blocked genres, own sources only), and not in the group yet. The others
// are left out without a reason, and inviting them is not found. A hidden
// library hides its view only, its titles staying reachable: a user who
// hides the title's library may join, and is listed. Invitations go to
// the caller's own group only.
func TestSyncPlayInviteesAreThoseWhoMayJoin(t *testing.T) {
	t.Parallel()
	g := newInviteGroup(t)
	s := g.testServer
	join := accounts.SyncPlayJoin
	none := accounts.SyncPlayNone
	users := map[string]accounts.User{
		"sam":      s.user("sam", visible),
		"joinonly": s.user("joinonly", func(c *accounts.UserChanges) { visible(c); c.SyncPlay = &join }),
		"hidden":   s.user("hidden", nil),
		"nosync":   s.user("nosync", func(c *accounts.UserChanges) { visible(c); c.SyncPlay = &none }),
		"child": s.user("child", func(c *accounts.UserChanges) {
			visible(c)
			c.Parental = &accounts.ParentalControl{MaxRating: new(13)}
		}),
		"calm":     s.user("calm", func(c *accounts.UserChanges) { visible(c); c.BlockedGenres = &[]string{"drama"} }),
		"noshows":  s.user("noshows", visible),
		"ownonly":  s.user("ownonly", func(c *accounts.UserChanges) { visible(c); c.UseSharedAddons = new(false) }),
		"night":    s.user("night", visible),
		"disabled": s.user("disabled", visible),
		"offline":  s.user("offline", visible),
		"tvonly":   s.user("tvonly", visible),
		"guest":    s.user("guest", visible),
	}
	for _, name := range []string{"sam", "joinonly", "hidden", "nosync", "child", "calm", "noshows", "ownonly", "night", "disabled", "guest"} {
		s.webPlayer(t, name, name+"-web").drain(t)
	}
	shows, _ := accounts.ParseID(g.views["Shows"])
	away := time.Now().Add(72 * time.Hour).Weekday().String()
	for name, changes := range map[string]accounts.UserChanges{
		"noshows":  {HiddenLibraries: &[]accounts.ID{shows}},
		"night":    {AccessSchedules: &[]accounts.AccessSchedule{{Day: away, StartHour: 0, EndHour: 24}}},
		"disabled": {IsDisabled: new(true)},
	} {
		if _, err := s.store.UpdateUser(t.Context(), users[name].ID, changes, nil); err != nil {
			t.Fatal(err)
		}
	}
	// Another app than the web player, which would not show the prompt.
	s.syncPlayApp(t, "tvonly", "tvonly-tv").drain(t)
	// The guest is in the group already.
	guest := s.syncPlayApp(t, "guest", "guest-tv")
	guest.drain(t)
	guest.post(t, s, "Join", map[string]any{"GroupId": g.group})

	invitable := []string{"joinonly", "noshows", "sam"}
	if got := g.invitees(t, g.host); !slices.Equal(got, invitable) {
		t.Errorf("invitees %v, want %v", got, invitable)
	}
	for name, user := range users {
		status, _ := g.invite(t, g.host, g.group, user)
		if want := map[bool]int{true: http.StatusNoContent, false: http.StatusNotFound}[slices.Contains(invitable, name)]; status != want {
			t.Errorf("inviting %s: %d, want %d", name, status, want)
		}
	}

	// Only to the caller's own group, by a session in it.
	other := s.syncPlayApp(t, "member", "member-tv")
	other.drain(t)
	if status, _ := other.send(t, s, http.MethodGet, "/Polyfin/SyncPlay/Invitees", nil); status != http.StatusForbidden {
		t.Errorf("invitees out of any group: %d", status)
	}
	if status, _ := g.invite(t, other, g.group, users["sam"]); status != http.StatusForbidden {
		t.Errorf("inviting to another's group: %d", status)
	}
	status, answer := other.send(t, s, http.MethodPost, "/SyncPlay/New", map[string]any{"GroupName": "Other"})
	var info GroupInfoDto
	if status != http.StatusOK || json.Unmarshal(answer, &info) != nil {
		t.Fatalf("other group: %d %s", status, answer)
	}
	if status, _ := g.invite(t, other, g.group, users["joinonly"]); status != http.StatusForbidden {
		t.Errorf("inviting to a group the caller is not in: %d", status)
	}
	// The host's other session, out of the group.
	away2 := s.syncPlayApp(t, "host", "host-phone")
	away2.drain(t)
	if status, _ := away2.send(t, s, http.MethodGet, "/Polyfin/SyncPlay/Invitees", nil); status != http.StatusForbidden {
		t.Errorf("invitees from the host's session out of the group: %d", status)
	}
	if status, _ := g.invite(t, away2, g.group, users["joinonly"]); status != http.StatusForbidden {
		t.Errorf("inviting from the host's session out of the group: %d", status)
	}
}

// An invitation reaches the invitee's web players only, at most once a
// minute, naming the group, its host and its title. Joining from one of
// them closes the prompt on the others; Not now does too.
func TestSyncPlayInvitationsReachTheWebPlayers(t *testing.T) {
	t.Parallel()
	g := newInviteGroup(t)
	s := g.testServer
	var clock atomic.Pointer[time.Time]
	start := time.Now()
	clock.Store(&start)
	s.handler.now = func() time.Time { return *clock.Load() }
	sam := s.user("sam", visible)
	robin := s.user("robin", visible)
	laptop, phone := s.webPlayer(t, "sam", "sam-laptop"), s.webPlayer(t, "sam", "sam-phone")
	tv := s.syncPlayApp(t, "sam", "sam-tv")
	robinWeb := s.webPlayer(t, "robin", "robin-web")

	if status, _ := g.invite(t, g.host, g.group, sam); status != http.StatusNoContent {
		t.Fatalf("inviting sam: %d", status)
	}
	sent := laptop.invite(t)
	if sent.State != "Invited" || sent.GroupId != g.group || sent.GroupName != "Movie night" || sent.HostName != "host" ||
		sent.ItemId != g.episodes[0] || sent.ItemName != "Pilot" || sent.SeriesName != "Show" || sent.Id == "" || sent.ExpiresAt == nil {
		t.Errorf("invitation: %+v", sent)
	} else if lasts := time.Time(*sent.ExpiresAt).Sub(start); lasts < 5*time.Minute-time.Millisecond || lasts > 5*time.Minute+time.Millisecond {
		t.Errorf("the invitation lasts %v", lasts)
	}
	if other := phone.invite(t); encode(other) != encode(sent) {
		t.Errorf("the phone's invitation %+v, the laptop's %+v", other, sent)
	}
	tv.noInvite(t)
	robinWeb.noInvite(t)

	// A user is invited at most once a minute, by anyone.
	if status, wait := g.invite(t, g.host, g.group, sam); status != http.StatusTooManyRequests || wait != "60" {
		t.Errorf("inviting sam again: %d, Retry-After %q", status, wait)
	}
	later := start.Add(59 * time.Second)
	clock.Store(&later)
	if status, wait := g.invite(t, g.host, g.group, sam); status != http.StatusTooManyRequests || wait != "1" {
		t.Errorf("inviting sam 59 seconds later: %d, Retry-After %q", status, wait)
	}
	laptop.noInvite(t)
	later = start.Add(time.Minute)
	clock.Store(&later)
	if status, _ := g.invite(t, g.host, g.group, sam); status != http.StatusNoContent {
		t.Fatalf("inviting sam a minute later: %d", status)
	}
	// It replaces the invitation before.
	again := laptop.invite(t)
	if encode(phone.invite(t)) != encode(again) || again.Id == sent.Id || again.State != "Invited" {
		t.Errorf("the second invitation: %+v", again)
	}

	// Joining from the laptop, as Join does, closes the phone's prompt.
	laptop.post(t, s, "Join", map[string]any{"GroupId": g.group})
	for _, a := range []syncPlayApp{laptop, phone} {
		if closed := a.invite(t); closed != (InvitePush{Id: again.Id, State: "Joined", GroupId: g.group}) {
			t.Errorf("%s told %+v", a.device, closed)
		}
	}
	tv.noInvite(t)
	// Ended, it can no longer be declined.
	if status, _ := phone.send(t, s, http.MethodPost, "/Polyfin/SyncPlay/Invitations/"+again.Id+"/Decline", nil); status != http.StatusNotFound {
		t.Errorf("declining a used invitation: %d", status)
	}

	// Not now closes the invitee's other prompts; nobody else may decline.
	other := s.webPlayer(t, "robin", "robin-phone")
	if status, _ := g.invite(t, g.host, g.group, robin); status != http.StatusNoContent {
		t.Fatalf("inviting robin: %d", status)
	}
	invited := robinWeb.invite(t)
	other.invite(t)
	if status, _ := phone.send(t, s, http.MethodPost, "/Polyfin/SyncPlay/Invitations/"+invited.Id+"/Decline", nil); status != http.StatusNotFound {
		t.Errorf("sam declining robin's invitation: %d", status)
	}
	if status, _ := other.send(t, s, http.MethodPost, "/Polyfin/SyncPlay/Invitations/"+invited.Id+"/Decline", nil); status != http.StatusNoContent {
		t.Fatalf("declining: %d", status)
	}
	for _, a := range []syncPlayApp{robinWeb, other} {
		if closed := a.invite(t); closed != (InvitePush{Id: invited.Id, State: "Declined", GroupId: g.group}) {
			t.Errorf("%s told %+v", a.device, closed)
		}
	}
	if status, _ := other.send(t, s, http.MethodPost, "/Polyfin/SyncPlay/Invitations/"+invited.Id+"/Decline", nil); status != http.StatusNotFound {
		t.Errorf("declining twice: %d", status)
	}
}

// An invitation lasts 5 minutes, or until its group ends: the invitee's
// web players are told, which closes the prompt.
func TestSyncPlayInvitationsExpireOrEndWithTheirGroup(t *testing.T) {
	t.Parallel()
	g := newInviteGroup(t)
	s := g.testServer
	s.handler.invites.mu.Lock()
	s.handler.invites.lasts = 300 * time.Millisecond
	s.handler.invites.mu.Unlock()
	sam := s.user("sam", visible)
	robin := s.user("robin", visible)
	samWeb, robinWeb := s.webPlayer(t, "sam", "sam-web"), s.webPlayer(t, "robin", "robin-web")

	if status, _ := g.invite(t, g.host, g.group, sam); status != http.StatusNoContent {
		t.Fatalf("inviting sam: %d", status)
	}
	sent := samWeb.invite(t)
	if expired := samWeb.invite(t); expired != (InvitePush{Id: sent.Id, State: "Expired", GroupId: g.group}) {
		t.Errorf("after 5 minutes: %+v", expired)
	}

	s.handler.invites.mu.Lock()
	s.handler.invites.lasts = inviteLasts
	s.handler.invites.mu.Unlock()
	if status, _ := g.invite(t, g.host, g.group, robin); status != http.StatusNoContent {
		t.Fatalf("inviting robin: %d", status)
	}
	sent = robinWeb.invite(t)
	g.host.post(t, s, "Leave", nil)
	if ended := robinWeb.invite(t); ended != (InvitePush{Id: sent.Id, State: "Ended", GroupId: g.group}) {
		t.Errorf("after the group ended: %+v", ended)
	}
	s.handler.invites.mu.Lock()
	defer s.handler.invites.mu.Unlock()
	if len(s.handler.invites.open) != 0 {
		t.Errorf("invitations left: %d", len(s.handler.invites.open))
	}
}
