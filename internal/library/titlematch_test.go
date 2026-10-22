package library

import "testing"

func TestReleaseTitles(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ file, title string }{
		{"Friends.S01E01.1080p.BluRay.x265.mkv", "friends"},
		{"Smiling.Friends.S01E01.1080p.WEB.mkv", "smiling friends"},
		{"[Group] Friends - S01E01 [1080p].mkv", "friends"},
		{"Spider-Man.No.Way.Home.2021.2160p.mkv", "spider man no way home"},
		{"The Office (US) S02E01.mkv", "the office us"},
		{"Friends (1994) S01E01.mkv", "friends"},
		{"movie.mkv", ""},
		{"Movie.English.1080p.mkv", "movie"},
		{"Friends.FRENCH.S01E01.mkv", "friends"},
		{"", ""},
	} {
		if got := releaseTitle(c.file); got != c.title {
			t.Errorf("%q: %q, want %q", c.file, got, c.title)
		}
	}
	if !sameReleaseTitle("friends", matchKey("Friends")) || sameReleaseTitle("smiling friends", matchKey("Friends")) {
		t.Error("Friends and Smiling Friends are told apart wrong")
	}
	if !otherTitleWith("smiling friends", "friends") || otherTitleWith("friends", "friends") || otherTitleWith("movie", "test movie") {
		t.Error("only longer titles holding the title are other titles")
	}
	if !sameReleaseTitle("spider man no way home", matchKey("Spider-Man: No Way Home")) || !sameReleaseTitle("office", matchKey("The Office")) {
		t.Error("punctuation or the leading the counts")
	}
}
