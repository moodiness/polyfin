package library

import (
	"context"
	"testing"
)

// The work a request leaves shares the views the request built, not its
// end: a home row's queue lists its titles after the row is answered.
func TestSharedViewsOutliveTheirRequest(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	request := PerRequest(ctx)
	shared := SharedViews(request)
	cancel()
	if shared.Err() != nil || shared.Done() != nil {
		t.Error("the shared views end with their request")
	}
	if got, want := shared.Value(requestViewsKey{}), request.Value(requestViewsKey{}); got != want {
		t.Errorf("shared views %p, the request's %p", got, want)
	}
	// Without a request's views, the work has views of its own.
	if views, _ := SharedViews(t.Context()).Value(requestViewsKey{}).(*requestViews); views == nil {
		t.Error("no views without a request's")
	}
}
