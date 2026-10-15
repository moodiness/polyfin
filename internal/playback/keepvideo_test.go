package playback

import (
	"testing"

	"github.com/moodiness/polyfin/internal/hls"
)

// The key of a remux tells its converted video by its device and its
// conversion: another audio track or play session keeps it, another
// quality or burned subtitle changes it, and copied video has none.
func TestKeyTellsTheConvertedVideo(t *testing.T) {
	t.Parallel()
	conversion := &VideoConversion{Codec: "h264", Encoder: "libx264", Width: 1920, Height: 1080, Bitrate: 8_000_000}
	remux := Remux{Session: "one", Device: "tv-id", Audio: 1, Format: hls.FMP4, ConvertVideo: conversion}
	key := remux.key()
	if key.Device != "tv-id" || key.Video == "" || !key.Converts {
		t.Fatalf("key: %+v", key)
	}
	other := remux
	other.Session, other.Audio = "two", 2
	if got := other.key(); got.Video != key.Video {
		t.Error("another audio track changed the video's key")
	}
	lower := remux
	lower.ConvertVideo = &VideoConversion{Codec: "h264", Encoder: "libx264", Width: 1280, Height: 720, Bitrate: 4_000_000}
	burned := remux
	burned.Burn = new(3)
	file := remux
	file.BurnFile = "http://subtitles.example/movie.sup"
	toneMapped := remux
	toneMapped.ConvertVideo = &VideoConversion{Codec: "h264", Encoder: "libx264", Width: 1920, Height: 1080, Bitrate: 8_000_000, ToneMap: true}
	seen := map[string]string{key.Video: "first"}
	for name, r := range map[string]Remux{"lower": lower, "burned": burned, "burned file": file, "tone mapped": toneMapped} {
		video := r.key().Video
		if before, ok := seen[video]; ok {
			t.Errorf("%s has the video key of %s", name, before)
		}
		seen[video] = name
	}
	copied := remux
	copied.ConvertVideo = nil
	if got := copied.key(); got.Video != "" || got.Converts {
		t.Errorf("copied video: %+v", got)
	}
}
