package accounts

import (
	"errors"
	"testing"
)

// A source cache preset sets the cache's size, how long it keeps blocks,
// the credit of plays again and whole-file preloading; custom keeps the
// values set one by one.
func TestSourceCachePresetsSetTheLimits(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	settings := store.Settings()
	for preset, want := range SourceCachePresetLimits {
		changed := settings
		changed.SourceCachePreset = preset
		// Values set alongside give way to the preset's.
		changed.CacheSizeGB, changed.SourceCacheDays, changed.SourceCacheCreditDays, changed.PreloadWholeFiles = 1234, 99, 9, !want.WholeFiles
		saved, err := store.UpdateSettings(ctx, changed)
		if err != nil {
			t.Fatalf("%s: %v", preset, err)
		}
		got := SourceCacheLimits{SizeGB: saved.CacheSizeGB, Days: saved.SourceCacheDays, CreditDays: saved.SourceCacheCreditDays,
			WholeFiles: saved.PreloadWholeFiles}
		if got != want || store.Settings().CacheSizeGB != want.SizeGB {
			t.Errorf("%s: saved %+v, want %+v", preset, got, want)
		}
	}
	custom := store.Settings()
	custom.SourceCachePreset = SourceCacheCustom
	custom.CacheSizeGB, custom.SourceCacheDays, custom.SourceCacheCreditDays, custom.PreloadWholeFiles = 1234, 99, 9, true
	saved, err := store.UpdateSettings(ctx, custom)
	if err != nil || saved.CacheSizeGB != 1234 || saved.SourceCacheDays != 99 || saved.SourceCacheCreditDays != 9 || !saved.PreloadWholeFiles {
		t.Errorf("custom: %+v %v", saved, err)
	}
	for name, change := range map[string]func(*Settings){
		"an unknown preset":     func(s *Settings) { s.SourceCachePreset = "huge" },
		"366 days":              func(s *Settings) { s.SourceCacheDays = 366 },
		"a negative credit":     func(s *Settings) { s.SourceCacheCreditDays = -1 },
		"31 days of credit":     func(s *Settings) { s.SourceCacheCreditDays = 31 },
		"no episode preloaded":  func(s *Settings) { s.PreloadEpisodes = 0 },
		"4 episodes preloaded":  func(s *Settings) { s.PreloadEpisodes = 4 },
		"no preloading rate":    func(s *Settings) { s.PreloadMbps = 0 },
		"no free space to keep": func(s *Settings) { s.PreloadReserveGB = 0 },
	} {
		changed := custom
		change(&changed)
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidSourceCache) && !errors.Is(err, ErrInvalidPreload) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// The source cache is never emptied on a schedule by default, and
// otherwise at an hour from 0 to 23, whatever the preset; the database
// refuses what the store would.
func TestSourceCacheEmptyHourStaysInRange(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	if got := store.Settings().SourceCacheEmptyHour; got != DefaultSourceCacheEmptyHour || DefaultSourceCacheEmptyHour != -1 {
		t.Errorf("default: %d", got)
	}
	for _, hour := range []int{-2, 24} {
		changed := store.Settings()
		changed.SourceCacheEmptyHour = hour
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidSourceCacheEmptyHour) {
			t.Errorf("hour %d: %v", hour, err)
		}
	}
	for _, hour := range []int{0, 23, -1} {
		changed := store.Settings()
		changed.SourceCacheEmptyHour = hour
		saved, err := store.UpdateSettings(ctx, changed)
		if err != nil || saved.SourceCachePreset != DefaultSettings().SourceCachePreset {
			t.Fatalf("hour %d: %v, preset %q", hour, err, saved.SourceCachePreset)
		}
		reopened, err := Open(ctx, store.db)
		if err != nil {
			t.Fatal(err)
		}
		if got := reopened.Settings().SourceCacheEmptyHour; got != hour {
			t.Errorf("after reopening: %d, want %d", got, hour)
		}
	}
	for _, column := range []string{"source_cache_empty_hour = -2", "source_cache_empty_hour = 24"} {
		if _, err := store.db.Exec(ctx, "UPDATE settings SET "+column); err == nil {
			t.Errorf("the database took %s", column)
		}
	}
}
