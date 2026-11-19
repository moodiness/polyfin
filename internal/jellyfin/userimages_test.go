package jellyfin

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// picture is a PNG image of width by height, opaque unless alpha is below
// 255.
func picture(t *testing.T, width, height int, alpha uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 90, A: alpha})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// send sends body as is, with its content type.
func (s testServer) send(method, path, authorization, contentType, body string) (int, http.Header, []byte) {
	s.t.Helper()
	request, _ := http.NewRequestWithContext(s.t.Context(), method, s.url+path, strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := testClient.Do(request)
	if err != nil {
		s.t.Fatal(err)
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	return response.StatusCode, response.Header, payload
}

func TestUsersChangeTheirOwnPicturesAndAdministratorsAnyones(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	shown := false
	member := s.user("member", func(c *accounts.UserChanges) { c.IsHidden = &shown })
	s.user("other", nil)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	memberToken, otherToken, adminToken := s.signIn("member", "tv"), s.signIn("other", "tv"), s.signIn("admin", "tv")
	photo := base64.StdEncoding.EncodeToString(picture(t, 1000, 600, 255))

	// A member changes their own picture, which is kept smaller, as JPEG.
	if status, _, body := s.send(http.MethodPost, "/UserImage", app("tv", memberToken), "image/png", photo); status != http.StatusNoContent {
		t.Fatalf("upload: %d %s", status, body)
	}
	me := func(token string) UserDto {
		t.Helper()
		var dto UserDto
		s.get(t, "/Users/Me", token, &dto)
		return dto
	}
	tag := me(memberToken).PrimaryImageTag
	if tag == "" {
		t.Fatal("no PrimaryImageTag after the upload")
	}
	status, header, data := s.send(http.MethodGet, "/UserImage?userId="+member.ID.String()+"&tag="+tag, "", "", "")
	kept, format, err := image.DecodeConfig(bytes.NewReader(data))
	if status != http.StatusOK || header.Get("Content-Type") != "image/jpeg" || err != nil || format != "jpeg" ||
		kept.Width != 512 || kept.Height != 307 || header.Get("ETag") != `"`+tag+`"` {
		t.Fatalf("picture: %d %v %s %+v %v", status, header, format, kept, err)
	}
	// The tag shows wherever apps show the user.
	var public []UserDto
	s.get(t, "/Users/Public", "", &public)
	if len(public) != 1 || public[0].PrimaryImageTag != tag {
		t.Errorf("public users: %+v", public)
	}
	var sessions []SessionInfo
	s.get(t, "/Sessions", memberToken, &sessions)
	if len(sessions) == 0 || sessions[0].UserPrimaryImageTag != tag {
		t.Errorf("sessions: %+v", sessions)
	}
	if me(otherToken).PrimaryImageTag != "" {
		t.Error("another user got the picture")
	}
	// Without a user, the caller's; without either, refused.
	if status, _, _ := s.send(http.MethodGet, "/UserImage", app("tv", memberToken), "", ""); status != http.StatusOK {
		t.Errorf("own picture: %d", status)
	}
	if status, _, body := s.send(http.MethodGet, "/UserImage", "", "", ""); status != http.StatusBadRequest ||
		string(body) != "\"UserId is required if unauthenticated\"\n" {
		t.Errorf("anonymous without a user: %d %s", status, body)
	}

	// Another member may not change it; an administrator may, by either
	// route.
	for _, path := range []string{"/UserImage?userId=" + member.ID.String(), "/Users/" + member.ID.String() + "/Images/Primary"} {
		if status, _, _ := s.send(http.MethodPost, path, app("tv", otherToken), "image/png", photo); status != http.StatusForbidden {
			t.Errorf("other user on %s: %d", path, status)
		}
		if status, _, _ := s.send(http.MethodDelete, path, app("tv", otherToken), "", ""); status != http.StatusForbidden {
			t.Errorf("other user deleting on %s: %d", path, status)
		}
	}
	if me(memberToken).PrimaryImageTag != tag {
		t.Fatal("a refused change changed the picture")
	}
	// A transparent picture stays PNG, under a new tag.
	clear := base64.StdEncoding.EncodeToString(picture(t, 40, 30, 128))
	if status, _, body := s.send(http.MethodPost, "/Users/"+member.ID.String()+"/Images/Primary/0", app("tv", adminToken), "image/png", clear); status != http.StatusNoContent {
		t.Fatalf("administrator's upload: %d %s", status, body)
	}
	replaced := me(memberToken).PrimaryImageTag
	if status, header, _ := s.send(http.MethodGet, "/Users/"+member.ID.String()+"/Images/Primary", "", "", ""); replaced == tag || status != http.StatusOK || header.Get("Content-Type") != "image/png" {
		t.Errorf("replaced picture: %s, %d %s", replaced, status, header.Get("Content-Type"))
	}

	// What is not a picture is refused, and changes nothing.
	for name, request := range map[string][2]string{
		"another type":       {"image/gif", photo},
		"not base64":         {"image/png", "%%%"},
		"not a picture":      {"image/png", base64.StdEncoding.EncodeToString([]byte("text"))},
		"not an image type":  {"text/plain", photo},
		"without a type set": {"", photo},
	} {
		if status, _, body := s.send(http.MethodPost, "/UserImage", app("tv", memberToken), request[0], request[1]); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, status, body)
		}
	}
	if me(memberToken).PrimaryImageTag != replaced {
		t.Error("a refused upload changed the picture")
	}
	unknown := "/UserImage?userId=" + strings.Repeat("ab", 16)
	if status, _, _ := s.send(http.MethodPost, unknown, app("tv", adminToken), "image/png", photo); status != http.StatusNotFound {
		t.Errorf("unknown user: %d", status)
	}

	// Deleting removes it everywhere.
	if status, _, _ := s.send(http.MethodDelete, "/UserImage", app("tv", memberToken), "", ""); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	if status, _, _ := s.send(http.MethodGet, "/UserImage?userId="+member.ID.String(), "", "", ""); status != http.StatusNotFound || me(memberToken).PrimaryImageTag != "" {
		t.Errorf("after deleting: %d", status)
	}
	var raw map[string]any
	_, body := s.call(http.MethodGet, "/Users/Me", app("tv", memberToken), nil)
	_ = json.Unmarshal(body, &raw)
	if _, ok := raw["PrimaryImageTag"]; ok {
		t.Error("PrimaryImageTag is sent without a picture, which Jellyfin leaves out")
	}
}

// The picture of a user kept off the sign-in screen, or of a disabled one,
// goes only to those told of them: for its tag, which comes with the
// user, or to the user themselves and administrators. Anyone else gets
// the answer of a picture that does not exist. A shown user's picture
// loads by identifier alone, as in Jellyfin.
func TestAHiddenUsersPictureGoesOnlyToThoseToldOfThem(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	hidden := s.user("hidden", nil)
	shown := s.user("shown", func(c *accounts.UserChanges) { c.IsHidden = new(false) })
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	hiddenToken, shownToken, adminToken := s.signIn("hidden", "tv"), s.signIn("shown", "tv"), s.signIn("admin", "tv")
	tags := map[accounts.ID]string{}
	for id, token := range map[accounts.ID]string{hidden.ID: hiddenToken, shown.ID: shownToken} {
		// Each its own picture, so that each has its own tag.
		photo := base64.StdEncoding.EncodeToString(picture(t, 40, 30+len(tags), 255))
		if status, _, body := s.send(http.MethodPost, "/UserImage", app("tv", token), "image/png", photo); status != http.StatusNoContent {
			t.Fatalf("upload: %d %s", status, body)
		}
		var me UserDto
		s.get(t, "/Users/Me", token, &me)
		tags[id] = me.PrimaryImageTag
	}
	// problem reads a problem answer.
	problem := func(body []byte) problemDetails {
		var answer problemDetails
		_ = json.Unmarshal(body, &answer)
		return answer
	}
	_, _, unknownBody := s.send(http.MethodGet, "/UserImage?userId="+strings.Repeat("ab", 16), "", "", "")
	unknown := problem(unknownBody)
	// load asks for id's picture by each route, with query added, as
	// authorization, and checks the status.
	load := func(name string, id accounts.ID, query, authorization string, want int) {
		t.Helper()
		for _, path := range []string{"/UserImage?userId=" + id.String() + "&", "/Users/" + id.String() + "/Images/Primary?", "/Users/" + id.String() + "/Images/Primary/0?"} {
			status, _, body := s.send(http.MethodGet, path+query, authorization, "", "")
			if got := problem(body); status != want || want == http.StatusNotFound && (got.Type != unknown.Type || got.Title != unknown.Title) {
				t.Errorf("%s, %s%s: %d %s, want %d", name, path, query, status, body, want)
			}
		}
	}
	load("a hidden user's picture, anonymous", hidden.ID, "", "", http.StatusNotFound)
	load("a hidden user's picture, to another member", hidden.ID, "", app("tv", shownToken), http.StatusNotFound)
	load("a hidden user's picture, by another tag", hidden.ID, "tag="+tags[shown.ID], "", http.StatusNotFound)
	load("a hidden user's picture, by its tag", hidden.ID, "tag="+tags[hidden.ID], "", http.StatusOK)
	load("a hidden user's picture, to themselves", hidden.ID, "", app("tv", hiddenToken), http.StatusOK)
	load("a hidden user's picture, to an administrator", hidden.ID, "", app("tv", adminToken), http.StatusOK)
	load("a shown user's picture, anonymous", shown.ID, "", "", http.StatusOK)
	if _, err := s.store.UpdateUser(t.Context(), shown.ID, accounts.UserChanges{IsDisabled: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	load("a disabled user's picture, anonymous", shown.ID, "", "", http.StatusNotFound)
	load("a disabled user's picture, by its tag", shown.ID, "tag="+tags[shown.ID], "", http.StatusOK)
}
