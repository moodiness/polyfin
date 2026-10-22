package accounts

import (
	"encoding/base64"
	"strings"
	"testing"
)

// A logo is an image of BrandLogoTypes as a base64 data URL, of at most
// MaxBrandLogoBytes.
func TestBrandLogoImage(t *testing.T) {
	t.Parallel()
	png := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("\x89PNG..."))
	if kind, image, ok := BrandLogoImage(png); !ok || kind != "image/png" || string(image) != "\x89PNG..." {
		t.Errorf("a PNG: %q %q %v", kind, image, ok)
	}
	big := "data:image/png;base64," + base64.StdEncoding.EncodeToString(make([]byte, MaxBrandLogoBytes+1))
	for name, logo := range map[string]string{
		"empty":      "",
		"no data":    "image/png;base64,AAAA",
		"html":       "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte("<script>")),
		"not base64": "data:image/png,%89PNG",
		"bad base64": "data:image/png;base64,***",
		"too large":  big,
		"nothing":    "data:image/png;base64,",
	} {
		if _, _, ok := BrandLogoImage(logo); ok {
			t.Errorf("%s: accepted %q", name, logo[:min(len(logo), 40)]+strings.Repeat(".", 0))
		}
	}
}
