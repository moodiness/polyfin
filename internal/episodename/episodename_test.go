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
