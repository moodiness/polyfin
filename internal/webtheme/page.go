package webtheme

import (
	"crypto/sha256"
	"encoding/hex"
	"sync/atomic"
)

// The web client's page picks each user's theme itself: Polyfin's script
// loads the theme's stylesheet and script as the page loads, and tells the
// server the theme it shows in a cookie, so that Jellyfin's branding gives
// jellyfin-web the CSS of that very theme.

// Loader is the name of the script that loads LumaaGlaass, which Polyfin
// writes (accounts.LumaaGlaassJs), served beside the theme's files:
// Address(Loader).
const Loader = "loader.js"

// CustomCss and CustomJs are the names the web client serves the Custom
// theme's texts under, in /web/, with their Hash as the version:
// CustomCss + "?v=" + Hash(css).
const (
	CustomCss = "custom.css"
	CustomJs  = "custom.js"
)

// Cookie is the name of the cookie in which Polyfin's script tells the
// server whose ID is serverID the theme its page shows: one of
// accounts.WebThemes.
func Cookie(serverID string) string {
	return "polyfinWebTheme_" + serverID
}

// Import is CSS that imports the stylesheet at address, as Jellyfin's
// branding gives a theme's CSS to the web client.
func Import(address string) string {
	return "@import url('" + address + "');"
}

// Hash names the content of text in the address it is served at, so that
// the address can be cached for good.
func Hash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}

// Hasher hashes a text, again only when it changed: the texts are the
// settings', which change seldom and run up to megabytes.
type Hasher struct {
	last atomic.Pointer[hashed]
}

type hashed struct {
	text, hash string
}

// Hash is Hash(text), remembered for the last text.
func (h *Hasher) Hash(text string) string {
	if last := h.last.Load(); last != nil && last.text == text {
		return last.hash
	}
	hash := Hash(text)
	h.last.Store(&hashed{text: text, hash: hash})
	return hash
}
