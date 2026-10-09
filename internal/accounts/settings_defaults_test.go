package accounts

import (
	"errors"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The Go defaults are those of the columns: a server set up anew, and a
// row the database fills with its defaults, both hold DefaultSettings.
func TestDefaultSettingsAreTheColumnDefaults(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	if got := store.Settings(); !reflect.DeepEqual(got, DefaultSettings()) {
		t.Errorf("a new server's settings:\n%+v\nwant\n%+v", got, DefaultSettings())
	}
	if _, err := store.db.Exec(ctx, "DELETE FROM settings"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(ctx, "INSERT INTO settings DEFAULT VALUES"); err != nil {
		t.Fatal(err)
	}
	var columns Settings
	if err := store.db.QueryRow(ctx, "SELECT "+settingsColumns+" FROM settings").Scan(columns.fields()...); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(columns, DefaultSettings()) {
		t.Errorf("the column defaults:\n%+v\nwant\n%+v", columns, DefaultSettings())
	}
}

// The web player's default CSS and script load the theme from one pinned
// commit: a branch would change every server's theme behind its back, and
// two commits would mix versions of its stylesheet and its script.
func TestDefaultThemeIsPinnedToOneCommit(t *testing.T) {
	pin := regexp.MustCompile(`/LumaaGlaass@([^/]+)/assets/`)
	css, js := pin.FindAllStringSubmatch(DefaultCustomCss, -1), pin.FindAllStringSubmatch(DefaultCustomJs, -1)
	commit := regexp.MustCompile(`^[0-9a-f]{40}$`)
	if len(css) != 1 || len(js) != 1 || css[0][1] != js[0][1] || !commit.MatchString(css[0][1]) {
		t.Errorf("the theme's pins: stylesheet %q, script %q", css, js)
	}
}

// boundField is the field of Settings named name in the admin API.
func boundField(settings *Settings, name string) (reflect.Value, bool) {
	value := reflect.ValueOf(settings).Elem()
	for i := range value.NumField() {
		if strings.EqualFold(value.Type().Field(i).Name, name) {
			return value.Field(i), true
		}
	}
	return reflect.Value{}, false
}

// The bounds served are those the settings check: each setting has an
// entry, its default is DefaultSettings', its bounds and choices are
// accepted, and what lies past them is refused.
func TestSettingsBoundsAreThoseChecked(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	base := store.Settings()
	bounds := SettingsBounds()
	named := map[string]bool{}
	for key := range bounds {
		named[strings.ToLower(key)] = true
	}
	fields := reflect.TypeFor[Settings]()
	for i := range fields.NumField() {
		if name := fields.Field(i).Name; !named[strings.ToLower(name)] {
			t.Errorf("no bounds for %s", name)
		}
	}
	try := func(name string, value any) error {
		t.Helper()
		changed := base
		field, _ := boundField(&changed, name)
		field.Set(reflect.ValueOf(value).Convert(field.Type()))
		_, err := store.UpdateSettings(ctx, changed)
		return err
	}
	defaults := DefaultSettings()
	for name, b := range bounds {
		field, ok := boundField(&defaults, name)
		if !ok {
			t.Errorf("%s: no such setting", name)
			continue
		}
		if !reflect.DeepEqual(field.Interface(), b.Default) {
			t.Errorf("%s: default %v, want %v", name, b.Default, field.Interface())
		}
		switch kind := field.Kind(); {
		case b.Bounded && (kind == reflect.Int || kind == reflect.Float64):
			step := 1.0
			if kind == reflect.Float64 {
				step = 0.01
			}
			for _, edge := range []float64{b.Min, b.Max} {
				if err := try(name, edge); err != nil {
					t.Errorf("%s: %v refused: %v", name, edge, err)
				}
			}
			if b.Zero {
				if err := try(name, 0); err != nil {
					t.Errorf("%s: 0 refused: %v", name, err)
				}
			}
			for _, past := range []float64{b.Min - step, b.Max + step} {
				if past == 0 && b.Zero {
					continue
				}
				if err := try(name, past); err == nil {
					t.Errorf("%s: %v accepted", name, past)
				}
			}
		case b.Bounded && kind == reflect.String:
			// An address is a URL: its scheme starts it, letters fill the
			// rest. A folder is an absolute path.
			letter, start := "a", ""
			switch name {
			case "remuxDbUrl", "publicAddress":
				start = "https://"
			case "recordingsFolder", "backupFolder":
				start = "/"
			}
			if err := try(name, start+strings.Repeat(letter, int(b.Max)-len(start))); err != nil {
				t.Errorf("%s: %v long refused: %v", name, b.Max, err)
			}
			if err := try(name, start+strings.Repeat(letter, int(b.Max)+1-len(start))); err == nil {
				t.Errorf("%s: %v long accepted", name, b.Max+1)
			}
			if b.Min > 0 {
				if err := try(name, strings.Repeat(letter, int(b.Min)-1)); err == nil {
					t.Errorf("%s: %v long accepted", name, b.Min-1)
				}
			}
		case b.Choices != nil && kind != reflect.Slice:
			for _, choice := range b.Choices {
				if err := try(name, choice); err != nil {
					t.Errorf("%s: %v refused: %v", name, choice, err)
				}
			}
			outside := any("other")
			if kind == reflect.Int {
				outside = 7
			}
			if err := try(name, outside); err == nil {
				t.Errorf("%s: %v accepted", name, outside)
			}
		case b.Choices != nil:
			var choices []string
			for _, choice := range b.Choices {
				choices = append(choices, choice.(string))
			}
			if err := try(name, []string{"other"}); err == nil {
				t.Errorf("%s: a value not among %v accepted", name, choices)
			}
		case b.Bounded || b.Zero:
			t.Errorf("%s: bounds on a %s", name, kind)
		}
	}
}

func TestEnvironmentIsAdoptedOnce(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	if err := store.AdoptEnvironment(ctx, Environment{Hardware: "nvenc", Segments: []string{"introdb", "theintrodb"}}); err != nil {
		t.Fatal(err)
	}
	check := func(when string, s *Store, hardware string, order, off []string) {
		t.Helper()
		got := s.Settings()
		if got.HardwareAcceleration != hardware || !slices.Equal(got.SegmentOrder, order) || !slices.Equal(got.SegmentSourcesOff, off) {
			t.Errorf("%s: GPU %q, order %v, off %v; want %q, %v, %v", when, got.HardwareAcceleration, got.SegmentOrder, got.SegmentSourcesOff,
				hardware, order, off)
		}
		reopened, err := Open(ctx, s.db)
		if err != nil {
			t.Fatal(err)
		}
		if reopened.Settings().HardwareAcceleration != got.HardwareAcceleration || !slices.Equal(reopened.Settings().SegmentOrder, got.SegmentOrder) ||
			!slices.Equal(reopened.Settings().SegmentSourcesOff, got.SegmentSourcesOff) {
			t.Errorf("%s: not saved: %+v", when, reopened.Settings())
		}
	}
	// The databases POLYFIN_SEGMENTS names come first, in its order, the
	// others after them, turned off.
	adopted := []string{"introdb", "theintrodb", "publicmetadb"}
	check("the first start", store, "nvenc", adopted, []string{"publicmetadb"})
	// A start with other values changes nothing.
	if err := store.AdoptEnvironment(ctx, Environment{Hardware: "vaapi", Segments: []string{"publicmetadb"}}); err != nil {
		t.Fatal(err)
	}
	check("a start with other values", store, "nvenc", adopted, []string{"publicmetadb"})
	// An administrator's choice stays chosen.
	chosen := store.Settings()
	chosen.HardwareAcceleration, chosen.SegmentOrder, chosen.SegmentSourcesOff = "none", []string{"publicmetadb", "theintrodb", "introdb"}, []string{}
	if _, err := store.UpdateSettings(ctx, chosen); err != nil {
		t.Fatal(err)
	}
	if err := store.AdoptEnvironment(ctx, Environment{Hardware: "auto"}); err != nil {
		t.Fatal(err)
	}
	check("after the administrator's choice", store, "none", chosen.SegmentOrder, []string{})
	// Wrong values are refused, and change nothing.
	if err := store.AdoptEnvironment(ctx, Environment{Hardware: "qsv"}); !errors.Is(err, ErrInvalidHardwareAcceleration) {
		t.Errorf("an unknown GPU: %v", err)
	}
	if err := store.AdoptEnvironment(ctx, Environment{Segments: []string{"introdb", "introdb"}}); !errors.Is(err, ErrInvalidSegmentSourcesOff) {
		t.Errorf("a database twice: %v", err)
	}
}

// A server that chose its GPU and order before only takes the databases
// turned off from the environment; POLYFIN_SEGMENTS=none turns them all
// off.
func TestEnvironmentFillsOnlyWhatFollowedIt(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	if _, err := store.db.Exec(ctx, "UPDATE settings SET hardware_acceleration = 'vaapi', "+
		"segment_order = '{publicmetadb,introdb,theintrodb}', environment_pending = '{segment_sources_off}'"); err != nil {
		t.Fatal(err)
	}
	if err := store.AdoptEnvironment(ctx, Environment{Hardware: "nvenc", Segments: []string{}}); err != nil {
		t.Fatal(err)
	}
	got := store.Settings()
	if got.HardwareAcceleration != "vaapi" || !slices.Equal(got.SegmentOrder, []string{"publicmetadb", "introdb", "theintrodb"}) ||
		!slices.Equal(got.SegmentSourcesOff, SegmentSources) {
		t.Errorf("adopted: %+v", got)
	}
}

func TestSegmentSourcesStayValid(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	for _, tc := range []struct {
		name       string
		order, off []string
		err        error
	}{
		{"no order", []string{}, []string{}, ErrInvalidSegmentOrder},
		{"a database missing", []string{"theintrodb", "introdb"}, []string{}, ErrInvalidSegmentOrder},
		{"a database twice", []string{"theintrodb", "introdb", "introdb"}, []string{}, ErrInvalidSegmentOrder},
		{"an unknown database turned off", SegmentSources, []string{"other"}, ErrInvalidSegmentSourcesOff},
		{"a database turned off twice", SegmentSources, []string{"introdb", "introdb"}, ErrInvalidSegmentSourcesOff},
	} {
		changed := store.Settings()
		changed.SegmentOrder, changed.SegmentSourcesOff = tc.order, tc.off
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, tc.err) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.err)
		}
	}
	// The databases turned off are kept in their usual order, and none is
	// an empty list.
	changed := store.Settings()
	changed.SegmentOrder, changed.SegmentSourcesOff = []string{"publicmetadb", "introdb", "theintrodb"}, []string{"publicmetadb", "theintrodb"}
	saved, err := store.UpdateSettings(ctx, changed)
	if err != nil || !slices.Equal(saved.SegmentSourcesOff, []string{"theintrodb", "publicmetadb"}) {
		t.Errorf("saved: %v %v", saved.SegmentSourcesOff, err)
	}
	changed.SegmentSourcesOff = nil
	if saved, err := store.UpdateSettings(ctx, changed); err != nil || saved.SegmentSourcesOff == nil {
		t.Errorf("none turned off: %#v %v", saved.SegmentSourcesOff, err)
	}
}
