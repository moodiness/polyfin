package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"log"
	"log/slog"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/desktop/app"
	"github.com/moodiness/polyfin/internal/i18n"
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

// Every language has every line (internal/i18n's TestTexts checks their
// placeholders), French with its own punctuation; another language gets
// the English.
func TestTexts(t *testing.T) {
	english, french := trayTexts["en"], trayTexts["fr"]
	s := app.Status{Phase: app.Downloading, Percent: 42, Problems: []app.Problem{app.NoTools}}
	if got := english.phase(s); got != "Downloading FFmpeg: 42%" {
		t.Errorf("English = %q", got)
	}
	if got := french.phase(s); got != "Téléchargement de FFmpeg : 42 %" {
		t.Errorf("French = %q", got)
	}
	if got := formatted(english.setupCode, "AB3D-7KQ9"); got != "Setup code: AB3D-7KQ9 (click to copy)" {
		t.Errorf("setup code = %q", got)
	}
	if english.copyKey != "Copy the secret key" || french.copyKey != "Copier la clé secrète" || !strings.Contains(french.keyCopied, "sauvegardes") {
		t.Error("the secret key's lines")
	}
	if english.phase(app.Status{Phase: app.Rebuilding}) != "Rebuilding the database's indexes" || french.phase(app.Status{Phase: app.Rebuilding}) != "Reconstruction des index de la base de données" {
		t.Error("the rebuilding line")
	}
	if textsFor("nl") != english || textsFor("pt-BR") != trayTexts["pt-BR"] {
		t.Error("textsFor")
	}
	if len(trayTexts) != len(i18n.Languages) {
		t.Errorf("%d languages, want %d", len(trayTexts), len(i18n.Languages))
	}
	for _, language := range i18n.Languages {
		text, ok := trayTexts[language]
		if !ok {
			t.Errorf("no texts in %s", language)
			continue
		}
		if got := text.problems(s); len(got) != 1 || !strings.Contains(got[0], "FFmpeg") {
			t.Errorf("%s problems = %v", language, got)
		}
		if got := text.phase(s); !strings.Contains(got, "42") || strings.Contains(got, "%!") {
			t.Errorf("%s = %q", language, got)
		}
		for _, phase := range []app.Phase{app.Starting, app.Running, app.Stopping, app.Stopped, app.Failed, app.Updating, app.Rebuilding} {
			if text.phase(app.Status{Phase: phase}) == "" {
				t.Errorf("no line for %s in %s", phase, language)
			}
		}
		if language != "en" && strings.Contains(text.failed+text.stopping+text.keyCopied+text.keyHint+text.updating, "'") {
			t.Errorf("%s uses a straight apostrophe", language)
		}
	}
}

// What libraries write with the standard log package, such as the tray's
// errors, goes to the app's log as a warning.
func TestLogLibraries(t *testing.T) {
	previous, writer, flags := slog.Default(), log.Writer(), log.Flags()
	defer func() {
		slog.SetDefault(previous)
		slog.SetLogLoggerLevel(slog.LevelInfo)
		log.SetOutput(writer)
		log.SetFlags(flags)
	}()
	var out bytes.Buffer
	logLibraries(slog.New(slog.NewTextHandler(&out, nil)))
	log.Printf("systray error: unable to init instance: %s\n", "no taskbar")
	if got := out.String(); !strings.Contains(got, `level=WARN msg="systray error: unable to init instance: no taskbar"`) {
		t.Errorf("log = %q", got)
	}
}
