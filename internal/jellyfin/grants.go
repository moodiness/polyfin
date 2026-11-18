package jellyfin

import (
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
)

// grantLife is how long a grant stands after it was signed. The play
// session of a PlaybackInfo and the grant of a media source's Path are
// given for a play that starts now, and apps ask again for the next one:
// two days let a movie paused one evening go on the next, and the
// downloads an app queued be fetched, while an address read later, in a
// player's history or a proxy's log, opens nothing.
const grantLife = 48 * time.Hour

// grantIdle is how long a grant past its life still stands after the last
// request it was accepted for: a play that goes on past two days, as a
// channel left on, is not cut while its app keeps asking, and is over
// once the app asked nothing for an hour. Only a grant that names a
// device goes on so: its device's sign-out ends it.
const grantIdle = time.Hour

// grant signs a grant as of the handler's clock, which granted reads its
// age on. Every grant the handler gives out is signed here: one signed
// without a time is refused.
func (h *Handler) grant(grant playback.Grant) string {
	grant.Signed = h.now()
	return h.Playback.Signer().Sign(grant)
}

// filesGrant signs the grant a relayed playlist names its files with (see
// liveLink), in a request streamAccess accepted with the grant accepted.
// The files are the same play's: their grant names the device the request
// was accepted for, its grant's or its token's, and ends with that
// device's sign-in. A request opened by a grant that names no device
// gives its files that grant's own time rather than the clock's: no
// sign-out ends such grants, so none renews another, and whatever they
// name is over two days after the first was signed.
func (h *Handler) filesGrant(accepted playback.Grant, user accounts.User, version library.Version) string {
	files := playback.Grant{Version: version.ID, User: user.ID, Device: accepted.Device, Relay: true, Signed: h.now()}
	if files.Device == (accounts.ID{}) && !accepted.Signed.IsZero() {
		files.Signed = accepted.Signed
	}
	return h.Playback.Signer().Sign(files)
}

// granted reads the grant a media request carries as token, and the user
// it stands for. Every media route that takes a grant asks here, so that
// all refuse the same ones. A grant stands when Polyfin signed it, within
// its life, while the device it was given to is signed in, and while its
// user is enabled, within their allowed hours and network. Past its life,
// one that names a device stands while it is in use (see grantIdle); one
// that names none, as an API key's caller is given, is over, since
// nothing else would end it. A grant that carries no time, as those
// signed before grants carried one, stands no more: its app asks
// PlaybackInfo, or the item, again.
func (h *Handler) granted(r *http.Request, token string) (accounts.User, playback.Grant, bool) {
	grant, err := h.Playback.Signer().Verify(token)
	if err != nil || grant.Signed.IsZero() {
		return accounts.User{}, playback.Grant{}, false
	}
	// A grant signed further ahead of the clock than grants live comes
	// from a clock that was set wrong: it would stand until then.
	named := grant.Device != (accounts.ID{})
	age := h.now().Sub(grant.Signed)
	if age.Abs() > grantLife {
		if _, used := h.grantUses.Get(token); !named || !used {
			return accounts.User{}, playback.Grant{}, false
		}
	}
	// Signing a device out, removing it, or changing the password of its
	// account ends the grants given to it, as it ends its token. The
	// user's other devices play on.
	if named {
		if signedIn, err := h.Accounts.SignedIn(r.Context(), grant.Device); err != nil || !signedIn {
			return accounts.User{}, playback.Grant{}, false
		}
	}
	user, err := h.Accounts.User(r.Context(), grant.User)
	if err != nil || user.Refused(h.now()) || h.outsideHours(user) || remoteRefused(r, user) {
		return accounts.User{}, playback.Grant{}, false
	}
	// Only the grants about to end, or past their life, need their uses
	// remembered, and only those a sign-out can end.
	if named && age > grantLife-grantIdle {
		h.grantUses.Put(token, struct{}{})
	}
	return user, grant, true
}
