package accounts

import (
	"errors"
	"reflect"
	"testing"
)

// The playback choices start as Polyfin played before they were settings:
// 45 s per analysis, three versions tried, the first version that plays,
// conversions neither counted nor scaled down.
func TestPlaybackChoicesDefaultToWhatPolyfinDidBefore(t *testing.T) {
	store := newStore(t)
	got := store.Settings()
	if got.AnalysisTimeout != 45 || got.VersionAttempts != 3 || got.PreferDirectPlay || got.MaxConversions != 0 || got.MaxConversionHeight != 0 {
		t.Errorf("defaults: %+v", got)
	}
	if DefaultAnalysisTimeout != got.AnalysisTimeout || DefaultVersionAttempts != got.VersionAttempts || DefaultMaxConversions != got.MaxConversions {
		t.Errorf("default constants: %d, %d, %d", DefaultAnalysisTimeout, DefaultVersionAttempts, DefaultMaxConversions)
	}
	// The administrator's setup keeps them.
	if _, err := store.CreateFirstAdministrator(t.Context(), "admin", "correct horse", "fr"); err != nil {
		t.Fatal(err)
	}
	if after := store.Settings(); after.AnalysisTimeout != 45 || after.VersionAttempts != 3 || after.PreferDirectPlay ||
		after.MaxConversions != 0 || after.MaxConversionHeight != 0 {
		t.Errorf("after the setup: %+v", after)
	}
}

func TestPlaybackChoicesStayInRange(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	for _, tc := range []struct {
		name   string
		change func(*Settings)
		err    error
	}{
		{"analysis timeout too short", func(s *Settings) { s.AnalysisTimeout = MinAnalysisTimeout - 1 }, ErrInvalidAnalysisTimeout},
		{"analysis timeout too long", func(s *Settings) { s.AnalysisTimeout = MaxAnalysisTimeout + 1 }, ErrInvalidAnalysisTimeout},
		{"no version tried", func(s *Settings) { s.VersionAttempts = MinVersionAttempts - 1 }, ErrInvalidVersionAttempts},
		{"too many versions tried", func(s *Settings) { s.VersionAttempts = MaxVersionAttempts + 1 }, ErrInvalidVersionAttempts},
		{"negative conversions", func(s *Settings) { s.MaxConversions = MinMaxConversions - 1 }, ErrInvalidMaxConversions},
		{"too many conversions", func(s *Settings) { s.MaxConversions = MaxMaxConversions + 1 }, ErrInvalidMaxConversions},
		{"a height that is not offered", func(s *Settings) { s.MaxConversionHeight = 600 }, ErrInvalidMaxConversionHeight},
		{"a negative height", func(s *Settings) { s.MaxConversionHeight = -480 }, ErrInvalidMaxConversionHeight},
		{"a height above 2160", func(s *Settings) { s.MaxConversionHeight = 4320 }, ErrInvalidMaxConversionHeight},
	} {
		changed := store.Settings()
		tc.change(&changed)
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, tc.err) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.err)
		}
	}
	if got := store.Settings(); got.AnalysisTimeout != DefaultAnalysisTimeout || got.VersionAttempts != DefaultVersionAttempts ||
		got.MaxConversions != DefaultMaxConversions || got.MaxConversionHeight != 0 {
		t.Errorf("a refused update changed the settings: %+v", got)
	}
	// The bounds are accepted, and kept across a restart.
	for _, edge := range [][4]int{
		{MinAnalysisTimeout, MinVersionAttempts, MinMaxConversions, ConversionHeights[1]},
		{MaxAnalysisTimeout, MaxVersionAttempts, MaxMaxConversions, ConversionHeights[len(ConversionHeights)-1]},
	} {
		changed := store.Settings()
		changed.AnalysisTimeout, changed.VersionAttempts, changed.MaxConversions, changed.MaxConversionHeight = edge[0], edge[1], edge[2], edge[3]
		changed.PreferDirectPlay = !changed.PreferDirectPlay
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
	// The database refuses what the store refuses, whoever writes it.
	for _, column := range []string{"analysis_timeout = 4", "analysis_timeout = 121", "version_attempts = 0", "version_attempts = 11",
		"max_conversions = -1", "max_conversions = 33", "max_conversion_height = 600"} {
		if _, err := store.db.Exec(ctx, "UPDATE settings SET "+column); err == nil {
			t.Errorf("the database took %s", column)
		}
	}
}
