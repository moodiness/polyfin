package accounts

import (
	"errors"
	"testing"
)

// A source cache preset sets the cache's size, how long it keeps blocks,
// the credit of plays again and whole-file preloading; custom keeps the
// values set one by one.
func TestSourceCachePresetsSetTheLimits(t *testing.T) {
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
