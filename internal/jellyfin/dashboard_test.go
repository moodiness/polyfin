package jellyfin

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestLiveSessionsShowWhatPlaysAndTakeCommands(t *testing.T) {
	p := playing(t)
	administrator := p.testServer.user("root", func(c *accounts.UserChanges) { c.IsAdministrator, c.RemoteControl = new(true), new(true) })
	stranger := p.testServer.user("bob", nil)
	if sessions, err := p.handler.LiveSessions(t.Context()); err != nil || len(sessions) != 0 {
		t.Fatalf("sessions before playback: %+v %v", sessions, err)
	}

	version := p.versions[1].ID.String()
	if status, body := p.call(http.MethodPost, "/Sessions/Playing", app("tv", p.token),
		map[string]any{"ItemId": p.movie, "MediaSourceId": version, "PositionTicks": 600_000_000, "PlayMethod": "DirectPlay"}); status != http.StatusNoContent {
		t.Fatalf("start: %d %s", status, body)
	}
	sessions, err := p.handler.LiveSessions(t.Context())
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions: %+v %v", sessions, err)
	}
	tv := sessions[0]
	if tv.User.Name != "member" || tv.Device.DeviceName != "tv" || tv.Device.Client != "Test App" || !tv.Found || tv.Item.Name != "Movie" ||
		tv.Playing.PlayMethod != "DirectPlay" || tv.Playing.Position < time.Minute || tv.Source == nil || len(tv.Encodings) != 0 || tv.Controllable {
		t.Errorf("tv session: %+v", tv)
	}
	if err := p.handler.StopPlayback(t.Context(), administrator, tv.Device.ID); !errors.Is(err, ErrNotControllable) {
		t.Errorf("stopping an app without remote control: %v", err)
	}

	tablet := p.remoteApp(t, "member", "tablet", queryMediaControl, true)
	tabletID, _ := parseGUID(tablet.session)
	if err := p.handler.SendMessage(t.Context(), administrator, tabletID, "Polyfin", "The server restarts soon", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	var message GeneralCommand
	tablet.socket.command(t, "GeneralCommand", &message)
	if message.Name != "DisplayMessage" || message.Arguments["Text"] != "The server restarts soon" || message.Arguments["Header"] != "Polyfin" ||
		message.Arguments["TimeoutMs"] != "5000" || message.ControllingUserId != administrator.ID.String() {
		t.Errorf("message: %+v", message)
	}
	if err := p.handler.StopPlayback(t.Context(), administrator, tabletID); err != nil {
		t.Fatal(err)
	}
	var stop PlaystateRequest
	tablet.socket.command(t, "Playstate", &stop)
	if stop.Command != "Stop" {
		t.Errorf("stop: %+v", stop)
	}
	if err := p.handler.StopPlayback(t.Context(), stranger, tabletID); !errors.Is(err, ErrControlRefused) {
		t.Errorf("another member stopping the tablet: %v", err)
	}
	tablet.socket.quiet(t)
	if err := p.handler.StopPlayback(t.Context(), administrator, randomID()); !errors.Is(err, accounts.ErrNotFound) {
		t.Errorf("unknown session: %v", err)
	}

	p.call(http.MethodPost, "/Sessions/Playing/Stopped", app("tv", p.token), map[string]any{"ItemId": p.movie, "PositionTicks": 600_000_000})
	if sessions, _ := p.handler.LiveSessions(t.Context()); len(sessions) != 0 {
		t.Errorf("sessions after stopping: %+v", sessions)
	}
}
