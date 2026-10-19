package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/desktop/app"
)

// The icon is a PNG of the size asked, wrapped for Windows in an icon
// file that points at it.
func TestIcon(t *testing.T) {
	for _, template := range []bool{true, false} {
		data := icon(44, template, false)
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil || img.Bounds().Dx() != 44 {
			t.Fatalf("icon: %v, %v", img, err)
		}
		if _, _, _, a := img.At(22, 30).RGBA(); a == 0 {
			t.Error("the logo's middle is transparent")
		}
	}
	data := icon(64, false, true)
	file := ico(data, 64)
	if binary.LittleEndian.Uint16(file[2:]) != 1 || binary.LittleEndian.Uint32(file[18:]) != 22 || !bytes.Equal(file[22:], data) {
		t.Error("the icon file does not hold the PNG")
	}
}

// Both languages have every line, French with its own punctuation.
func TestTexts(t *testing.T) {
	s := app.Status{Phase: app.Downloading, Percent: 42, Problems: []app.Problem{app.NoTools}}
	if got := english.phase(s); got != "Downloading FFmpeg: 42%" {
		t.Errorf("English = %q", got)
	}
	if got := french.phase(s); got != "Téléchargement de FFmpeg : 42 %" {
		t.Errorf("French = %q", got)
	}
	if got := french.problems(s); len(got) != 1 || !strings.Contains(got[0], "FFmpeg") {
		t.Errorf("French problems = %v", got)
	}
	if got := formatted(english.setupCode, "AB3D-7KQ9"); got != "Setup code: AB3D-7KQ9 (click to copy)" {
		t.Errorf("setup code = %q", got)
	}
	if english.copyKey != "Copy the secret key" || french.copyKey != "Copier la clé secrète" || !strings.Contains(french.keyCopied, "sauvegardes") {
		t.Error("the secret key's lines")
	}
	for _, phase := range []app.Phase{app.Starting, app.Running, app.Stopping, app.Stopped, app.Failed, app.Updating, app.Rebuilding} {
		if english.phase(app.Status{Phase: phase}) == "" || french.phase(app.Status{Phase: phase}) == "" {
			t.Errorf("no line for %s", phase)
		}
	}
	if english.phase(app.Status{Phase: app.Rebuilding}) != "Rebuilding the database's indexes" || french.phase(app.Status{Phase: app.Rebuilding}) != "Reconstruction des index de la base de données" {
		t.Error("the rebuilding line")
	}
	if strings.Contains(strings.Join([]string{french.failed, french.stopping, french.keyCopied, french.keyHint}, ""), "'") {
		t.Error("French uses a straight apostrophe")
	}
}
