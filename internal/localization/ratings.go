package localization

// Rating is a parental rating of the United States, the country metadata
// is described for.
type Rating struct {
	Name string
	// Score is the youngest age the rating suits, by which apps compare
	// ratings; nil for the ratings that say a title was not rated.
	Score *int
}

// Ratings returns the ratings of the two US systems: the film ratings of the
// Motion Picture Association (filmratings.com) and the TV Parental
// Guidelines (tvguidelines.org). Each score is the age the system's own
// wording sets: R and TV-MA admit no one under 17 alone, NC-17 no one 17 or
// under; PG and TV-PG, which only suggest guidance, sit between 7 and 13.
// The unrated come first, then by score. The list is shared: callers must
// not change it.
func Ratings() []Rating {
	return ratings
}

var ratings = []Rating{
	{Name: "NR"},
	{Name: "Unrated"},
	{Name: "G", Score: new(0)},
	{Name: "TV-G", Score: new(0)},
	{Name: "TV-Y", Score: new(0)},
	{Name: "TV-Y7", Score: new(7)},
	{Name: "TV-Y7-FV", Score: new(7)},
	{Name: "PG", Score: new(10)},
	{Name: "TV-PG", Score: new(10)},
	{Name: "PG-13", Score: new(13)},
	{Name: "TV-14", Score: new(14)},
	{Name: "R", Score: new(17)},
	{Name: "TV-MA", Score: new(17)},
	{Name: "NC-17", Score: new(18)},
}
