package addons

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/serverfile"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Network is where a source is: on a local network, or on the internet.
// It is what Polyfin connected to when it last read the source's manifest
// or its list, never what a name resolved to at another moment: a name's
// answers belong to whoever runs its DNS. Empty, it is not told yet: the
// source was installed before Polyfin kept it, and could not be read
// since. Such a source counts as one on the internet until it is told.
type Network string

const (
	// NetworkLocal is a source read from a local address (loopback,
	// private, link-local, unique local, carrier-grade NAT: what
	// stremio.Public refuses), a file of the server, or a local folder.
	NetworkLocal Network = "local"
	// NetworkPublic is a source read from an address on the internet.
	NetworkPublic Network = "public"
)

// Member reports whether the sources of scope are a member's: a user's
// own, when that user is no administrator. administrator tells whether
// the scope's owner is one; the server's sources have no owner.
func Member(scope Scope, administrator bool) bool {
	return scope.Owner != nil && !administrator
}

// Confined reports whether the requests made to the source, and those for
// every address its answers name (streams, subtitles, artwork, logos),
// stay on public addresses. member tells a member's own source (see
// Member), confined wherever it is. Any other, the server's or an
// administrator's own, is confined unless it is on a local network: only
// a source Polyfin reads from the local network may name addresses there.
// This is the one rule: every request made for a source follows it, or
// ConfinedSQL, which says the same in SQL.
func (a Addon) Confined(member bool) bool {
	return member || a.Network != NetworkLocal
}

// MemberSQL is Member in SQL, for the source the query names a: true for
// a user's own source whose owner is no administrator, or is gone.
func MemberSQL(a string) string {
	return "(" + a + ".owner_id IS NOT NULL AND NOT coalesce((SELECT owner.is_administrator FROM users owner WHERE owner.id = " + a + ".owner_id), false))"
}

// ConfinedSQL is Addon.Confined in SQL, for the source the query names a.
func ConfinedSQL(a string) string {
	return "(" + MemberSQL(a) + " OR " + a + ".network IS DISTINCT FROM '" + string(NetworkLocal) + "')"
}

// NetworkOf tells where the source at address is when the address alone
// tells it, without any request: a file of the server is local, and an
// address literal is where its address is. A host name tells nothing:
// false.
func NetworkOf(address string) (Network, bool) {
	if _, file := serverfile.Path(address); file {
		return NetworkLocal, true
	}
	parsed, err := url.Parse(address)
	if err != nil {
		return "", false
	}
	ip, err := netip.ParseAddr(parsed.Hostname())
	if err != nil {
		return "", false
	}
	if stremio.Public(ip) {
		return NetworkPublic, true
	}
	return NetworkLocal, true
}

// Locate reads the source at address with read, where the source is, and
// tells where that is. read reads it confined to public addresses, or
// else from local addresses only (see stremio.Client.OpenSource): the
// network of the read that answers is the source's, so that a name which
// resolves to several addresses, or to another one later, is where
// Polyfin connected to. confined tells a member's source, read from
// public addresses only. An address that tells where it is (see
// NetworkOf) is read once, there. A host name is read from the network
// known for it first, the local one when none is, then from the other.
// When neither answers, the error is that of the network the source
// seems to be on, and no network is told: the source keeps what was
// known of it.
func Locate[T any](ctx context.Context, address string, confined bool, known Network, read func(confined bool) (T, error)) (T, Network, error) {
	try := func(network Network) (T, Network, error) {
		value, err := read(network == NetworkPublic)
		if err != nil {
			network = ""
		}
		return value, network, err
	}
	if confined {
		return try(NetworkPublic)
	}
	if network, ok := NetworkOf(address); ok {
		return try(network)
	}
	first, second := NetworkLocal, NetworkPublic
	if known == NetworkPublic {
		first, second = second, first
	}
	value, network, err := try(first)
	if err == nil || ctx.Err() != nil {
		return value, network, err
	}
	other, network, otherErr := try(second)
	if otherErr != nil && offNetwork(otherErr) && !offNetwork(err) {
		// The second read only told the source is not on its network.
		return value, "", err
	}
	return other, network, otherErr
}

// offNetwork reports the refusal of a read by the client of the network
// the source is not on.
func offNetwork(err error) bool {
	return errors.Is(err, stremio.ErrPrivateNetwork) || errors.Is(err, stremio.ErrPublicNetwork)
}

// stored is network as the addons table keeps it: NULL when it is not
// told.
func (n Network) stored() *string {
	if n == "" {
		return nil
	}
	text := string(n)
	return &text
}

// Reconfine rewrites, in tx, the confinement kept with what a source
// brought to the library, as Addon.Confined tells it now: that of source,
// or of every source when it is nil (see SetReconfine).
type Reconfine func(ctx context.Context, tx pgx.Tx, source *accounts.ID) error

// SetReconfine has the store run reconfine, in its transaction, whenever
// where a source is changes, so that what was kept of the source before
// follows the rule at once rather than when it is next listed.
func (s *Store) SetReconfine(reconfine Reconfine) { s.reconfine = reconfine }

// reconfined runs the Reconfine set, if any.
func (s *Store) reconfined(ctx context.Context, tx pgx.Tx, source *accounts.ID) error {
	if s.reconfine == nil {
		return nil
	}
	return s.reconfine(ctx, tx, source)
}

// SetNetwork stores, in tx, where the source id was when its list was
// read (see Locate), and has what is kept of it follow when that changed
// (see SetReconfine). Polyfin's own sources call it from the transaction
// that stores their list. An empty network changes nothing.
func (s *Store) SetNetwork(ctx context.Context, tx pgx.Tx, id accounts.ID, network Network) error {
	if network == "" {
		return nil
	}
	tag, err := tx.Exec(ctx, "UPDATE addons SET network = $2 WHERE id = $1 AND network IS DISTINCT FROM $2", id, string(network))
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	return s.reconfined(ctx, tx, &id)
}

// locateDelays are the waits before the sources that could not be read
// are read again, the last one before every later read.
var locateDelays = []time.Duration{15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}

// unlocated is a source whose network is to be told by reading it.
type unlocated struct {
	id          accounts.ID
	kind        string
	manifestURL string
}

// LocateSources tells, at start, where the sources not told yet are (see
// Network): those installed before Polyfin kept it, without waiting for
// an administrator to refresh them.
//
//   - First, without any request: a member's source counts as on the
//     internet, as it is read from there only; a file of the server and an
//     address literal are where their address is. What is kept of every
//     source then follows the rule (see SetReconfine), a source not told
//     yet counting as on the internet.
//   - Then the Stremio and Eclipse addons named by a host name have their
//     manifest read, from the local network, then from the internet (see
//     Locate). Nothing else of the manifest is kept. Those that cannot be
//     read are read again after each of locateDelays, then every hour,
//     until all are told or ctx ends. The IPTV sources named by a host
//     name are told by their next list refresh, which takes them for due
//     (see iptv.Service.RefreshDue).
//
// One line at Info tells how the first pass went; the sources' addresses,
// which hold their settings and keys, are never logged.
func (s *Store) LocateSources(ctx context.Context, logger *slog.Logger) {
	pending, told, err := s.locateByAddress(ctx)
	if err != nil {
		if ctx.Err() == nil {
			logger.Warn("The sources whose network is not told could not be listed", "error", err)
		}
		return
	}
	first := len(pending)
	for attempt := 0; ; attempt++ {
		pending = s.locateByReading(ctx, pending)
		if attempt == 0 && told+first > 0 {
			logger.Info("Told where the sources installed before are", "by_address", told, "by_reading", first-len(pending), "unread", len(pending))
		}
		if len(pending) == 0 || ctx.Err() != nil {
			return
		}
		wait := s.locateWait
		if wait == nil {
			wait = sleep
		}
		if !wait(ctx, locateDelays[min(attempt, len(locateDelays)-1)]) {
			return
		}
	}
}

// sleep waits for d, and reports false when ctx ended first.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// locateByAddress tells, in one transaction, where the sources not told
// yet are when no request is needed (see LocateSources), then has what is
// kept of every source follow the rule. It returns the addons left to
// read, and how many sources it told.
func (s *Store) locateByAddress(ctx context.Context) ([]unlocated, int, error) {
	var pending []unlocated
	told := 0
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		pending, told = nil, 0
		rows, err := tx.Query(ctx, "SELECT a.id, a.kind, a.manifest_url, "+MemberSQL("a")+` FROM addons a
			WHERE a.network IS NULL ORDER BY a.id FOR UPDATE OF a`)
		if err != nil {
			return err
		}
		type source struct {
			unlocated
			member bool
		}
		sources, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (source, error) {
			var s source
			err := row.Scan(&s.id, &s.kind, &s.manifestURL, &s.member)
			return s, err
		})
		if err != nil || len(sources) == 0 {
			return err
		}
		for _, source := range sources {
			network, ok := NetworkOf(source.manifestURL)
			switch {
			case source.member:
				network = NetworkPublic
			case source.kind == KindLocal:
				network = NetworkLocal
			case !ok:
				if source.kind == KindStremio || source.kind == KindEclipse {
					pending = append(pending, source.unlocated)
				}
				continue
			}
			if _, err := tx.Exec(ctx, "UPDATE addons SET network = $2 WHERE id = $1", source.id, string(network)); err != nil {
				return err
			}
			told++
		}
		return s.reconfined(ctx, tx, nil)
	})
	if told > 0 {
		s.changed()
	}
	return pending, told, err
}

// locateByReading reads the manifest of each of sources to tell where it
// is, and returns those that could not be read. A source whose address
// changed meanwhile, or that was told meanwhile, is left as it is.
func (s *Store) locateByReading(ctx context.Context, sources []unlocated) []unlocated {
	var unread []unlocated
	for _, source := range sources {
		if ctx.Err() != nil {
			return append(unread, source)
		}
		f, err := s.fetch(ctx, source.manifestURL, false, "")
		if err != nil || f.kind != source.kind {
			unread = append(unread, source)
			continue
		}
		err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, "UPDATE addons SET network = $2 WHERE id = $1 AND network IS NULL AND manifest_url = $3",
				source.id, string(f.network), source.manifestURL)
			if err != nil || tag.RowsAffected() == 0 {
				return err
			}
			return s.reconfined(ctx, tx, &source.id)
		})
		if err != nil {
			unread = append(unread, source)
			continue
		}
		s.changed()
	}
	return unread
}
