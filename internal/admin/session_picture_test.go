package admin

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The account button shows the user's profile picture: the session names
// it, and a new one once it changes in an app.
func TestTheSessionNamesTheProfilePicture(t *testing.T) {
	api := newTestAPI(t, 10)
	member := api.signedIn("member", false)
	imageTagOf := func() (string, any) {
		t.Helper()
		status, body, _ := member.call(http.MethodGet, "/session", nil)
		user, _ := body["user"].(map[string]any)
		if status != http.StatusOK || user == nil {
			t.Fatalf("session: %d %v", status, body)
		}
		return user["id"].(string), user["imageTag"]
	}
	id, tag := imageTagOf()
	if tag != nil {
		t.Fatalf("without a picture: %v", tag)
	}
	userID, err := accounts.ParseID(id)
	if err != nil {
		t.Fatal(err)
	}
	picture := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for x := range 16 {
		picture.Set(x, x, color.NRGBA{R: 200, G: 40, B: 90, A: 255})
	}
	var data bytes.Buffer
	if err := png.Encode(&data, picture); err != nil {
		t.Fatal(err)
	}
	user, err := api.store.SetImage(t.Context(), userID, data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, tag := imageTagOf(); user.ImageTag == "" || tag != user.ImageTag {
		t.Errorf("with a picture: %v, want %q", tag, user.ImageTag)
	}
}
