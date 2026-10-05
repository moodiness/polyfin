package accounts

import (
	"errors"
	"reflect"
	"testing"
)

// Thumbnails and chapter images read the sources, so both start off, with
// Jellyfin's interval and width, and 2 GB for them.
func TestThumbnailSettingsStartOff(t *testing.T) {
	store := newStore(t)
	got := store.Settings()
	if got.Trickplay || got.ChapterImages || got.TrickplayInterval != 10 || got.TrickplayWidth != 320 || got.ThumbnailStorageGB != 2 {
		t.Errorf("defaults: %+v", got)
	}
	if DefaultTrickplayInterval != got.TrickplayInterval || DefaultTrickplayWidth != got.TrickplayWidth || DefaultThumbnailStorageGB != got.ThumbnailStorageGB {
		t.Errorf("default constants: %d, %d, %d", DefaultTrickplayInterval, DefaultTrickplayWidth, DefaultThumbnailStorageGB)
	}
	if _, err := store.CreateFirstAdministrator(t.Context(), "admin", "correct horse", "en"); err != nil {
		t.Fatal(err)
	}
	if after := store.Settings(); after.Trickplay || after.ChapterImages || after.TrickplayInterval != 10 ||
		after.TrickplayWidth != 320 || after.ThumbnailStorageGB != 2 {
		t.Errorf("after the setup: %+v", after)
	}
}

func TestThumbnailSettingsStayInRange(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	for _, tc := range []struct {
		name   string
		change func(*Settings)
		err    error
	}{
		{"interval too short", func(s *Settings) { s.TrickplayInterval = MinTrickplayInterval - 1 }, ErrInvalidTrickplayInterval},
		{"interval too long", func(s *Settings) { s.TrickplayInterval = MaxTrickplayInterval + 1 }, ErrInvalidTrickplayInterval},
		{"a width not offered", func(s *Settings) { s.TrickplayWidth = 300 }, ErrInvalidTrickplayWidth},
		{"no width", func(s *Settings) { s.TrickplayWidth = 0 }, ErrInvalidTrickplayWidth},
		{"no storage", func(s *Settings) { s.ThumbnailStorageGB = MinThumbnailStorageGB - 1 }, ErrInvalidThumbnailStorage},
		{"too much storage", func(s *Settings) { s.ThumbnailStorageGB = MaxThumbnailStorageGB + 1 }, ErrInvalidThumbnailStorage},
	} {
		changed := store.Settings()
		tc.change(&changed)
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, tc.err) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.err)
		}
	}
	for _, edge := range [][3]int{
		{MinTrickplayInterval, TrickplayWidths[0], MinThumbnailStorageGB},
		{MaxTrickplayInterval, TrickplayWidths[len(TrickplayWidths)-1], MaxThumbnailStorageGB},
	} {
		changed := store.Settings()
		changed.TrickplayInterval, changed.TrickplayWidth, changed.ThumbnailStorageGB = edge[0], edge[1], edge[2]
		changed.Trickplay, changed.ChapterImages = !changed.Trickplay, !changed.ChapterImages
		if _, err := store.UpdateSettings(ctx, changed); err != nil {
			t.Fatalf("%v: %v", edge, err)
		}
		reopened, err := Open(ctx, store.db)
		if err != nil {
			t.Fatal(err)
		}
		if got := reopened.Settings(); !reflect.DeepEqual(got, changed) {
			t.Errorf("%v after reopening: %+v, want %+v", edge, got, changed)
		}
	}
	for _, column := range []string{"trickplay_interval = 4", "trickplay_interval = 61", "trickplay_width = 300",
		"thumbnail_storage_gb = 0", "thumbnail_storage_gb = 51"} {
		if _, err := store.db.Exec(ctx, "UPDATE settings SET "+column); err == nil {
			t.Errorf("the database took %s", column)
		}
	}
}
