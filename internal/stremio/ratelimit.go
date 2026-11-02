package stremio

import (
	"errors"
	"strings"
)

// ErrRateLimited reports a request an addon refused for its rate with an
// answer of its own, such as a notice described as a title, rather than
// HTTP 429: it limits the requests of each address, and this one came
// beyond its limit.
var ErrRateLimited = errors.New("the addon refused the request: too many requests")

// rateLimitWords are what a notice of an addon refusing a request for its
// rate says, in lower case: the generic wording of request limiters.
var rateLimitWords = []string{"rate limit", "rate-limit", "ratelimit", "too many requests"}

// SaysRateLimited reports whether the texts of an addon's notice, a
// notice stream's or a notice described as a title, tell that it refused
// the request for its rate.
func SaysRateLimited(texts ...string) bool {
	for _, text := range texts {
		text = strings.ToLower(text)
		for _, words := range rateLimitWords {
			if strings.Contains(text, words) {
				return true
			}
		}
	}
	return false
}
