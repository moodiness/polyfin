package episodename

import "testing"

// A range of episodes ends, as Jellyfin 12.4 reads one, on an episode of
// at most 3 digits, followed by no other digit nor a resolution's "p" or
// "i": a year or a resolution after the episode is no range's end.
func TestRangesEndAsJellyfinReadsThem(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]Episodes{
		"Show S01E01-2022.mkv":               {Season: 1, First: 1, Last: 1},
		"Show.S09E14-1080p.mkv":              {Season: 9, First: 14, Last: 14},
		"Show.S01E05-720i.mkv":               {Season: 1, First: 5, Last: 5},
		"Show S01E01-E02.mkv":                {Season: 1, First: 1, Last: 2},
		"Show S01E01-02.mkv":                 {Season: 1, First: 1, Last: 2},
		"Show S02E10E11.mkv":                 {Season: 2, First: 10, Last: 11},
		"Show S01E99-E101.mkv":               {Season: 1, First: 99, Last: 101},
		"Show S01E100-102 Title.mkv":         {Season: 1, First: 100, Last: 102},
		"Show S01E03-01.mkv":                 {Season: 1, First: 3, Last: 3},
		"Show S01E01-E02-1080p.mkv":          {Season: 1, First: 1, Last: 2},
		"Show S01E01 Pilot.mkv":              {Season: 1, First: 1, Last: 1},
		"Show.S01E01.1080p.WEB-DL.mkv":       {Season: 1, First: 1, Last: 1},
		"All.of.Us.Are.Dead.S01E02-2022.mkv": {Season: 1, First: 2, Last: 2},
	} {
		got, ok := Of("Show/Season 01/" + name)
		if !ok || got != want {
			t.Errorf("%s: %+v %v, want %+v", name, got, ok, want)
		}
	}
}

// An absolute episode number is read from the names Jellyfin 12.4 reads
// one in, but for years, resolutions and numbers told as another's.
func TestAbsoluteNumbersAsJellyfinReadsThem(t *testing.T) {
	t.Parallel()
	type read struct {
		number int
		title  string
	}
	for name, want := range map[string]read{
		"Show - 41":                            {41, "Show"},
		"Show 41":                              {41, "Show"},
		"[Group] Show - 041 [1080p]":           {41, "Show"},
		"[Group] Show - 041 [1080p][ABCD1234]": {41, "Show"},
		"[Group] Show-07 [720p]":               {7, "Show"},
		"Show - 101 (2020)":                    {101, "Show"},
		"Show - 05v2":                          {5, "Show"},
		"Show - 05 - The Title":                {5, "Show"},
		"Show_-_12_[1080p]":                    {12, "Show"},
		"Show (2019) - 05":                     {5, "Show (2019)"},
		"Show 2 - 05":                          {5, "Show 2"},
		"Spin-Off Show - 05":                   {5, "Spin-Off Show"},
		"Station 100 05":                       {5, "Station 100"},
		"Show 2019 05":                         {5, "Show 2019"},
		"Show 41 1080p":                        {41, "Show"},
		"Show - 1080":                          {1080, "Show"},
		"Long Show - 1100":                     {1100, "Long Show"},
		// Refused: years, resolutions, numbers told as another's.
		"Show 2019":              {},
		"Show - 2019":            {},
		"Show 1883":              {},
		"Show 1080":              {},
		"Show 720":               {},
		"Show 1080p":             {},
		"Show - 1080p":           {},
		"Show - 720i":            {},
		"Show 720p WEB":          {},
		"Show 1920x1080":         {},
		"Show S01E02":            {},
		"Show 1x02":              {},
		"Show S02 - 05":          {},
		"Show Season 2 - 05":     {},
		"Show Episode 5":         {},
		"Show Part 2":            {},
		"Show 41-42":             {},
		"Show - 41-42":           {},
		"Show - 41.5":            {},
		"Show - 00":              {},
		"41":                     {},
		"- 41":                   {},
		"Show.Name.41":           {},
		"Grey's Show 05":         {},
		"Show 2020-05-12":        {},
		"Movie Title (1968)":     {},
		"Show.2019.1080p.WEB-DL": {},
	} {
		number, title, ok := Absolute(name)
		if got := (read{number, title}); got != want || ok != (want.number > 0) {
			t.Errorf("%q: %+v %v, want %+v", name, got, ok, want)
		}
	}
}
