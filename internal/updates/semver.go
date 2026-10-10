package updates

import (
	"cmp"
	"strconv"
	"strings"
)

// semver is a version as Semantic Versioning 2.0 writes it: MAJOR.MINOR.
// PATCH, then the identifiers of a pre-release, if any.
type semver struct {
	core [3]int
	pre  []string
}

// parseVersion reads a version such as "1.4.0" or "v1.4.0-rc.1", with
// or without a leading v; build metadata, after +, is left out, as it
// does not order versions. It reports false for anything else, such as
// "dev".
func parseVersion(text string) (semver, bool) {
	text = strings.TrimPrefix(text, "v")
	text, _, _ = strings.Cut(text, "+")
	core, pre, hasPre := strings.Cut(text, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	var v semver
	for i, part := range parts {
		n, ok := number(part)
		if !ok {
			return semver{}, false
		}
		v.core[i] = n
	}
	if hasPre {
		v.pre = strings.Split(pre, ".")
		for _, id := range v.pre {
			if !validIdentifier(id) {
				return semver{}, false
			}
		}
	}
	return v, true
}

// number reads a numeric part: digits, without a leading zero.
func number(text string) (int, bool) {
	if text == "" || len(text) > 1 && text[0] == '0' || strings.TrimLeft(text, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.Atoi(text)
	return n, err == nil
}

// validIdentifier reports whether id is a pre-release identifier: ASCII
// letters, digits and dashes, a number without a leading zero.
func validIdentifier(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-') {
			return false
		}
	}
	if strings.TrimLeft(id, "0123456789") == "" {
		_, ok := number(id)
		return ok
	}
	return true
}

// compare orders a and b as Semantic Versioning does: -1 when a comes
// first, 1 when b does, 0 when they are equal. A pre-release comes before
// its version.
func compare(a, b semver) int {
	for i := range a.core {
		if c := cmp.Compare(a.core[i], b.core[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if c := compareIdentifier(a.pre[i], b.pre[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(a.pre), len(b.pre))
}

// compareIdentifier orders two pre-release identifiers: numbers by value,
// before words, which are in ASCII order.
func compareIdentifier(a, b string) int {
	an, aNumber := number(a)
	bn, bNumber := number(b)
	switch {
	case aNumber && bNumber:
		return cmp.Compare(an, bn)
	case aNumber:
		return -1
	case bNumber:
		return 1
	}
	return strings.Compare(a, b)
}

// newer reports whether version comes after current, both versions
// parseVersion reads; anything else is never newer.
func newer(version, current string) bool {
	v, ok := parseVersion(version)
	if !ok {
		return false
	}
	c, ok := parseVersion(current)
	return ok && compare(v, c) > 0
}
