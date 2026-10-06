package accounts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/jpeg"
	"image/png"

	"github.com/jackc/pgx/v5"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers WebP with image.Decode
)

const (
	// MaxImageBytes bounds a profile picture as sent, and as kept.
	MaxImageBytes = 5 << 20
	// MaxImageSide is the longest side a profile picture is kept at: apps
	// show it small, and a larger one would only cost space and transfer.
	MaxImageSide = 512
	// maxImagePixels bounds the pictures decoded, which take memory by
	// their pixels rather than their bytes.
	maxImagePixels = 50_000_000
)

// ErrInvalidImage is returned for a picture that is not a JPEG, PNG or
// WebP image within the size allowed: MaxImageBytes for a profile picture.
var ErrInvalidImage = errors.New("invalid picture")

// ImageTypes are the formats a picture may be sent in.
var ImageTypes = []string{"image/jpeg", "image/png", "image/webp"}

// UserImage is a picture as kept, a user's profile picture or an item's
// uploaded artwork: a JPEG or PNG image, tagged by its content.
type UserImage struct {
	Data        []byte
	ContentType string
	Tag         string
}

// NormalizeImage decodes a JPEG, PNG or WebP picture of at most maxBytes
// and encodes it again within maxSide: as PNG when it has transparency,
// which JPEG would lose, else as JPEG, of at most maxBytes too. Profile
// pictures and the artwork administrators upload for items are kept so.
func NormalizeImage(data []byte, maxBytes, maxSide int) (UserImage, error) {
	if len(data) == 0 || len(data) > maxBytes {
		return UserImage{}, ErrInvalidImage
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "jpeg" && format != "png" && format != "webp" ||
		config.Width <= 0 || config.Height <= 0 || config.Width*config.Height > maxImagePixels {
		return UserImage{}, ErrInvalidImage
	}
	picture, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return UserImage{}, ErrInvalidImage
	}
	bounds := picture.Bounds()
	if width, height := bounds.Dx(), bounds.Dy(); width > maxSide || height > maxSide {
		scale := float64(maxSide) / float64(max(width, height))
		target := image.Rect(0, 0, max(1, int(float64(width)*scale+0.5)), max(1, int(float64(height)*scale+0.5)))
		scaled := image.NewRGBA(target)
		xdraw.CatmullRom.Scale(scaled, target, picture, bounds, xdraw.Src, nil)
		picture = scaled
	}
	var out bytes.Buffer
	result := UserImage{ContentType: "image/jpeg"}
	if opaque(picture) {
		err = jpeg.Encode(&out, picture, &jpeg.Options{Quality: 90})
	} else {
		result.ContentType = "image/png"
		err = png.Encode(&out, picture)
	}
	if err != nil || out.Len() > maxBytes {
		return UserImage{}, ErrInvalidImage
	}
	result.Data = out.Bytes()
	sum := sha256.Sum256(result.Data)
	result.Tag = hex.EncodeToString(sum[:16])
	return result, nil
}

// opaque reports whether every pixel of picture is opaque.
func opaque(picture image.Image) bool {
	if o, ok := picture.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	bounds := picture.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, a := picture.At(x, y).RGBA(); a != 0xffff {
				return false
			}
		}
	}
	return true
}

// SetImage replaces a user's profile picture with data, a JPEG, PNG or
// WebP image, normalized (see normalizeImage). It returns the user with
// their new ImageTag.
func (s *Store) SetImage(ctx context.Context, id ID, data []byte) (User, error) {
	// Apps show the picture by the user's ImageTag.
	defer s.forgetSignIns()
	normalized, err := NormalizeImage(data, MaxImageBytes, MaxImageSide)
	if err != nil {
		return User{}, err
	}
	return scanUser(s.db.QueryRow(ctx, "UPDATE users SET image = $2, image_type = $3, image_tag = $4 WHERE id = $1 RETURNING "+userColumns,
		id, normalized.Data, normalized.ContentType, normalized.Tag))
}

// DeleteImage removes a user's profile picture, if they have one.
func (s *Store) DeleteImage(ctx context.Context, id ID) (User, error) {
	defer s.forgetSignIns()
	return scanUser(s.db.QueryRow(ctx, "UPDATE users SET image = NULL, image_type = NULL, image_tag = '' WHERE id = $1 RETURNING "+userColumns, id))
}

// Image returns a user's profile picture; ErrNotFound when the user does
// not exist or has none.
func (s *Store) Image(ctx context.Context, id ID) (UserImage, error) {
	var picture UserImage
	err := s.db.QueryRow(ctx, "SELECT image, image_type, image_tag FROM users WHERE id = $1 AND image IS NOT NULL", id).
		Scan(&picture.Data, &picture.ContentType, &picture.Tag)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserImage{}, ErrNotFound
	}
	return picture, err
}
