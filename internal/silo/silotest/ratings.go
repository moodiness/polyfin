package silotest

import "math"

// The user's own rating of a title. The Apple app offers it on every movie
// and series page it has loaded (ItemDetailViewModel.canRate,
// Screens/Detail/ItemDetailViewModel.swift), whatever the server, and
// writes it with PUT /api/v2/ratings/{item_id} and removes it with DELETE,
// either taking any answer but 204 as a failure (APIv2Client.setRating and
// deleteRating, Networking/APIv2/APIv2Client+Ratings.swift).

// CheckUserRating checks the user_rating of a title's page: absent when
// the user has not rated it, else a whole number from 1 to 10, the stars
// times two. Apple APIv2CatalogRead.CatalogItemDetail decodes it as an
// Int (Networking/APIv2/APIv2CatalogReadModels.swift), and ItemDetail
// (Networking/APIv2/APIv2CatalogReadProjection.swift) drops one outside 1
// to 10, which then shows as no rating. It reports the rating, 0 for none.
func CheckUserRating(data []byte) (int, error) {
	detail, err := decode(data)
	if err != nil {
		return 0, err
	}
	c := &checker{}
	value, sent := detail["user_rating"]
	if !sent {
		return 0, nil
	}
	n, number := value.(float64)
	if !number || n != math.Trunc(n) || n < 1 || n > 10 {
		c.fail("user_rating %v is not a whole number from 1 to 10", value)
		return 0, c.err()
	}
	return int(n), nil
}
