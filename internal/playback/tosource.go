package playback

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/source"
)

// SendsToSource reports whether an app of user may be told where a
// version comes from: sent to its source by Serve or ServeChannel, or
// relayed a manifest whose lines name the source's addresses. A source's
// address can hold what its owner configured it with, an addon's key or
// an IPTV account's name and password, so it is told only to who owns
// it:
//
//   - never for a version of an IPTV source, whose addresses Polyfin
//     builds from the stored account, as a replay programme's is never
//     told;
//   - for a version of one of the server's addons, to an administrator
//     only;
//   - for a version of a user's own addon, to that user.
//
// That is the default choice of Send apps to the source
// (accounts.Settings.SendToSource), owners. With everyone, every app may
// be sent, as before the setting existed; with no one, none is, an
// administrator or an addon's owner included, so that a source sees only
// the server's address. A version that names no addon, a recording or a
// channel Polyfin airs, has no source to keep from anyone; one whose addon
// cannot be read is kept.
//
// Serve and ServeChannel take no user: every caller of theirs that does
// not relay already asks here, for the user the request plays for, and
// relays when the answer is no.
func (s *Service) SendsToSource(ctx context.Context, user accounts.User, version library.Version) bool {
	if version.Origin.Addon == (accounts.ID{}) {
		return true
	}
	switch s.settings().SendToSource {
	case accounts.SendToSourceEveryone:
		return true
	case accounts.SendToSourceNone:
		return false
	}
	var kind string
	var owner *accounts.ID
	if err := s.db.QueryRow(ctx, "SELECT kind, owner_id FROM addons WHERE id = $1", version.Origin.Addon).Scan(&kind, &owner); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
			s.logger.Warn("A version's source could not be read: its apps are relayed it", "error", err)
		}
		return false
	}
	switch {
	case addons.Addon{Kind: kind}.IPTV():
		return false
	case owner == nil:
		return user.IsAdministrator
	}
	return *owner == user.ID
}

// NamesSource reports whether a version relayed as it is would tell an
// app its source's addresses: it is a manifest, whose lines name its
// files, and they would not be named at Polyfin's own addresses. A
// version analyzed as a DASH manifest is one; so is one analyzed as an
// HLS playlist unless the delivery names its files (linked, see
// Delivery.Link). Until a version is analyzed, nothing but its first
// bytes tells a manifest from a file: it is taken for one when they are
// text, where media is binary. They are asked of the source as relay
// asks it for the version, not of the source cache, which gives nothing
// of a file whose analysis failed: with the headers the source needs,
// its link renewed once if it expired, and the connection the version
// holds (see Hold). A source that cannot be reached is an error; one
// that answers no content, or none of its bytes, has nothing to tell,
// and relay answers for it. A torrent's file and a local folder's are
// never manifests relayed.
func (s *Service) NamesSource(ctx context.Context, version library.Version, linked bool) (bool, error) {
	if version.Torrent != nil || version.LocalFolder || s.ReadsInput(version) {
		return false, nil
	}
	if analysis, ok := s.Analyzed(ctx, version.ID); ok {
		return DASH(analysis) || HLS(analysis) && !linked, nil
	}
	release, err := s.Hold(ctx, version)
	if err != nil {
		return false, err
	}
	defer release()
	head, err := s.firstBytes(ctx, version)
	if err != nil {
		return false, err
	}
	return len(head) > 0 && source.Reply{Head: head}.Text(), nil
}

// firstBytes asks a source for the first source.ReplyHead bytes of a
// version, renewing its link once when it expired, and returns those it
// answered: none when it answers no content.
func (s *Service) firstBytes(ctx context.Context, version library.Version) ([]byte, error) {
	open := func(version library.Version) (*http.Response, error) {
		header := http.Header{"Range": {"bytes=0-" + strconv.Itoa(source.ReplyHead-1)}}
		for name, value := range version.RequestHeaders() {
			header.Set(name, value)
		}
		return s.opener.Open(ctx, http.MethodGet, version.URL, header, version.Confined)
	}
	response, err := open(version)
	if err == nil && expired(response.StatusCode) && s.renew != nil {
		response.Body.Close()
		fresh, renewErr := s.renew(ctx, version)
		if renewErr != nil {
			return nil, fmt.Errorf("%w: HTTP %d, and the link could not be renewed: %v", ErrSourceUnavailable, response.StatusCode, renewErr)
		}
		response, err = open(fresh)
	}
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		return nil, nil
	}
	head := make([]byte, source.ReplyHead)
	n, err := io.ReadFull(response.Body, head)
	if n == 0 && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return head[:n], nil
}
