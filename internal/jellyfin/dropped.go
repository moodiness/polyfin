package jellyfin

import (
	"context"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/trackers"
)

// A series the user dropped on a tracking service whose history they
// import (see package trackers) leaves their Next Up, Upcoming and
// Continue Watching, as the series they hid do, until no service lists it
// any more or one of its episodes is played after the drop (see
// userdata.Store.Dropped). What they played stays played. Next Up asked
// for the series alone still answers.

// DroppedSeries is a series a user dropped on tracking services: the
// services listing it as dropped, in trackers.Services' order, as the
// Tracking page lists them, and At, the latest of their dates, which a
// play must follow to bring it back.
type DroppedSeries struct {
	Item     library.Item
	Services []string
	At       time.Time
}

// NextUpDropped lists the series user dropped on a tracking service that
// still keep them out of Next Up and that Polyfin can still describe,
// once each, the series dropped last first.
func (h *Handler) NextUpDropped(ctx context.Context, user accounts.User) ([]DroppedSeries, error) {
	dropped, err := h.UserData.Dropped(ctx, user.ID)
	if err != nil || len(dropped) == 0 {
		return nil, err
	}
	// The rows of a series come together, the series in order.
	var series []DroppedSeries
	var ids []accounts.ID
	index := map[accounts.ID]int{}
	for _, d := range dropped {
		i, ok := index[d.Series]
		if !ok {
			i = len(series)
			index[d.Series] = i
			ids = append(ids, d.Series)
			series = append(series, DroppedSeries{Item: library.Item{ID: d.Series}})
		}
		series[i].Services = append(series[i].Services, d.Service)
		if d.At.After(series[i].At) {
			series[i].At = d.At
		}
	}
	items, err := h.Library.Items(ctx, user, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[accounts.ID]library.Item, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	result := make([]DroppedSeries, 0, len(series))
	for _, s := range series {
		item, ok := byID[s.Item.ID]
		if !ok || item.Kind != library.KindSeries {
			continue
		}
		s.Item = item
		slices.SortFunc(s.Services, func(a, b string) int {
			return slices.Index(trackers.Services, a) - slices.Index(trackers.Services, b)
		})
		result = append(result, s)
	}
	return result, nil
}
