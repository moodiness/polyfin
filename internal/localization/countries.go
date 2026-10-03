package localization

import (
	"slices"
	"sync"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// Country is an ISO 3166-1 country.
type Country struct {
	// Name is the short English name ("Bolivia").
	Name   string
	Alpha2 string
	Alpha3 string
}

// exceptionallyReserved are the two-letter codes the ISO 3166 Maintenance
// Agency reserves for other uses (Ascension Island, the European Union,
// the United Nations...) without assigning them to a country. CLDR
// describes some of them as countries.
var exceptionallyReserved = []string{"AC", "CP", "DG", "EA", "EU", "EZ", "FX", "IC", "SU", "TA", "UK", "UN"}

// userAssigned reports whether a two-letter code is in the ranges ISO 3166
// leaves to users: AA, QM to QZ, XA to XZ and ZZ. Kosovo's XK, which CLDR
// lists, is one of them and is left out with the others, as ISO 3166-1 has
// no code for Kosovo.
func userAssigned(code string) bool {
	return code == "AA" || code == "ZZ" || code[0] == 'X' || code[0] == 'Q' && code[1] >= 'M'
}

// countries are the ISO 3166-1 countries, built from the Unicode CLDR data
// golang.org/x/text carries: the two-letter codes CLDR describes as
// countries, without the reserved and user-assigned ones, the withdrawn
// ones (CLDR replaces them, gives them no alpha-3 code or no name), with
// their alpha-3 codes and English names.
var countries = sync.OnceValue(func() []Country {
	names := display.English.Regions()
	var list []Country
	for first := 'A'; first <= 'Z'; first++ {
		for second := 'A'; second <= 'Z'; second++ {
			code := string([]rune{first, second})
			if userAssigned(code) || slices.Contains(exceptionallyReserved, code) {
				continue
			}
			region, err := language.ParseRegion(code)
			if err != nil || region.String() != code || !region.IsCountry() || region.Canonicalize() != region {
				continue
			}
			alpha3, name := region.ISO3(), names.Name(region)
			if len(alpha3) != 3 || alpha3 == "ZZZ" || name == "" {
				continue
			}
			list = append(list, Country{Name: name, Alpha2: code, Alpha3: alpha3})
		}
	}
	// English collation puts Åland Islands among the A's.
	collator := collate.New(language.English)
	slices.SortFunc(list, func(a, b Country) int { return collator.CompareString(a.Name, b.Name) })
	return list
})
