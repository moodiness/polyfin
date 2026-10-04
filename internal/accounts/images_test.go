package accounts

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func encodedPicture(t *testing.T, width, height int, alpha uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 40, A: alpha})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestProfilePicturesAreKeptSmall(t *testing.T) {
	store := newStore(t)
	user := mustCreate(t, store, NewUser{Name: "member", Password: "correct horse"})
	admin := mustCreate(t, store, NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})
	// Administrators manage subtitles unless that is taken away.
	if user.SubtitleManagement || !admin.SubtitleManagement {
		t.Errorf("subtitle management: member %v, administrator %v", user.SubtitleManagement, admin.SubtitleManagement)
	}
	updated, err := store.SetImage(t.Context(), user.ID, encodedPicture(t, 300, 900, 255))
	if err != nil {
		t.Fatal(err)
	}
	kept, err := store.Image(t.Context(), user.ID)
	if err != nil || kept.Tag != updated.ImageTag || kept.ContentType != "image/jpeg" {
		t.Fatalf("picture: %+v %v", kept, err)
	}
	if config, format, err := image.DecodeConfig(bytes.NewReader(kept.Data)); err != nil || format != "jpeg" || config.Width != 171 || config.Height != MaxImageSide {
		t.Errorf("kept picture: %s %+v %v", format, config, err)
	}
	// A small transparent picture keeps its size and its transparency.
	if _, err := store.SetImage(t.Context(), user.ID, encodedPicture(t, 20, 10, 0)); err != nil {
		t.Fatal(err)
	}
	if kept, _ := store.Image(t.Context(), user.ID); kept.ContentType != "image/png" {
		t.Errorf("transparent picture kept as %s", kept.ContentType)
	}
	for name, data := range map[string][]byte{
		"empty":         nil,
		"not a picture": []byte("hello"),
		"too large":     append(encodedPicture(t, 2, 2, 255), make([]byte, MaxImageBytes)...),
	} {
		if _, err := store.SetImage(t.Context(), user.ID, data); !errors.Is(err, ErrInvalidImage) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if updated, err = store.DeleteImage(t.Context(), user.ID); err != nil || updated.ImageTag != "" {
		t.Errorf("deleted: %q %v", updated.ImageTag, err)
	}
	if _, err := store.Image(t.Context(), user.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("after deleting: %v", err)
	}
	// The picture goes with its user.
	if _, err := store.SetImage(t.Context(), user.ID, encodedPicture(t, 4, 4, 255)); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteUser(t.Context(), user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Image(t.Context(), user.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("after deleting the user: %v", err)
	}
}
