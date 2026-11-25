package jellyfin

import (
	"testing"
)

// countsOf returns a DTO's ChildCount and RecursiveItemCount, -1 for each
// left out.
func countsOf(dto BaseItemDto) [2]int {
	counts := [2]int{-1, -1}
	if dto.ChildCount != nil {
		counts[0] = *dto.ChildCount
	}
	if dto.RecursiveItemCount != nil {
		counts[1] = *dto.RecursiveItemCount
	}
	return counts
}

// Series, seasons, albums and artists listed with
// Fields=ChildCount,RecursiveItemCount carry the counts their details give
// once Polyfin knows them, as Jellyfin's listings do; a series whose
// description was never read carries none, as a listing asks no addon.
func TestListingsCountKnownTitles(t *testing.T) {
	t.Parallel()
	const counted = "&Fields=ChildCount,RecursiveItemCount"
	s, token, views := browsing(t)
	listedSeries := func() BaseItemDto {
		t.Helper()
		var page QueryResult
		s.get(t, "/Items?ParentId="+views["Shows"]+counted, token, &page)
		if len(page.Items) != 1 {
			t.Fatalf("the shows: %+v", page.Items)
		}
		return page.Items[0]
	}
	series := listedSeries()
	if counts := countsOf(series); counts != [2]int{-1, -1} {
		t.Errorf("a series never described: counts %v", counts)
	}
	var details BaseItemDto
	s.get(t, "/Items/"+series.Id, token, &details)
	if counts := countsOf(listedSeries()); counts != countsOf(details) || counts[0] != 2 || counts[1] != 3 {
		t.Errorf("a series described: counts %v, its details %v", counts, countsOf(details))
	}
	var seasons QueryResult
	s.get(t, "/Items?ParentId="+series.Id+counted, token, &seasons)
	for _, season := range seasons.Items {
		var details BaseItemDto
		s.get(t, "/Items/"+season.Id, token, &details)
		if counts := countsOf(season); counts != countsOf(details) || counts[0] < 1 {
			t.Errorf("%s: counts %v, its details %v", season.Name, counts, countsOf(details))
		}
	}

	music := musicParity(t, false)
	user := music.users["allowed"]
	for _, kind := range []string{"album", "artist"} {
		id := music.items[kind]
		var details BaseItemDto
		music.get(t, "/Users/"+user.id+"/Items/"+id, user.token, &details)
		listed := music.listedByIdentifier(t, []string{id}, counted)[id]
		if counts := countsOf(listed); counts != countsOf(details) || counts[0] < 1 {
			t.Errorf("%s: counts %v, its details %v", kind, counts, countsOf(details))
		}
	}
}
