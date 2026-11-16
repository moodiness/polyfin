package silotest

import "fmt"

// CheckProfiles checks the profile list (GET /api/v2/profiles) of a
// server that creates no profile:
//
//   - Each profile has an id and requests_allowed, and the list
//     requests_allowed_supported, booleans the contract requires.
//   - max_profiles is no more than the profiles listed. With one profile,
//     the apps' profile menu offers Add Profile unless max_profiles is 1
//     or less, absent included: Apple ProfileMenuRow.row
//     (Components/ProfileMenuRow.swift), Android profileMenuRow (shared
//     model/server/ProfileMenuRow.kt). Add Profile opens the creation of a
//     profile, POST /api/v2/profiles, which such a server refuses: Apple
//     AddProfileFromMenu (Screens/Profiles/AddProfileFromMenu.swift),
//     Android AppNavigation's addProfile, then CreateProfile
//     (androidApp ui/navigation/AppNavigation.kt).
func CheckProfiles(data []byte) error {
	doc, err := decode(data)
	if err != nil {
		return err
	}
	c := &checker{}
	c.has(doc, "profiles", map[string]string{"items": "a", "requests_allowed_supported": "b", "max_profiles": "i"})
	items, _ := doc["items"].([]any)
	for n, entry := range items {
		profile, _ := entry.(object)
		c.has(profile, fmt.Sprintf("items[%d]", n), map[string]string{"id": "s", "requests_allowed": "b"})
	}
	if limit, _ := doc["max_profiles"].(float64); len(items) == 0 || limit > float64(len(items)) {
		c.fail("max_profiles %v for %d profiles: the apps offer a profile the server does not create", doc["max_profiles"], len(items))
	}
	return c.err()
}
