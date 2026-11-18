package iptv

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// located downloads the parts of an account that want names, as fetch
// does, where the account is, and tells with the list where that is (see
// addons.Locate): a member's account, confined, is read from public
// addresses only; any other from the local network or from the internet,
// the network known for it first. A list that is a file of the server is
// on the server itself. What the list then names, its streams and logos,
// is reached as a source on that network may (see addons.Addon.Confined).
func (s *Service) located(ctx context.Context, account Account, confined bool, known addons.Network, want parts) (downloaded, error) {
	address := account.URL
	if account.Kind == addons.KindXtream {
		address = account.Server
	}
	list, network, err := addons.Locate(ctx, address, confined, known, func(confined bool) (downloaded, error) {
		return fetch(ctx, s.requester(), account, confined, want)
	})
	list.network = network
	return list, err
}

// stored stores, in tx, what a download of source brought (see
// storeDownload), with where the list was read, which the source keeps
// (see addons.Store.SetNetwork).
func (s *Service) stored(ctx context.Context, tx pgx.Tx, source accounts.ID, list downloaded, at time.Time) error {
	if err := s.addons.SetNetwork(ctx, tx, source, list.network); err != nil {
		return err
	}
	return storeDownload(ctx, tx, source, list, at)
}

// reach tells how the stored source id is reached: member, whether it is
// a member's own (see addons.Member), whose list is read from public
// addresses only, and confined, whether its requests and those for what
// its list names stay on public addresses (see addons.Addon.Confined).
func (s *Service) reach(ctx context.Context, id accounts.ID) (member, confined bool, err error) {
	err = s.db.QueryRow(ctx, "SELECT "+addons.MemberSQL("a")+", "+addons.ConfinedSQL("a")+" FROM addons a WHERE a.id = $1", id).Scan(&member, &confined)
	return member, confined, err
}
