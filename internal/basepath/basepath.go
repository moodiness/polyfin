// Package basepath carries the base path a request came under, such as
// /jellyfin, from the route that strips it to the routes that answer, so
// that the addresses Polyfin sends back keep it: a reverse proxy that
// forwards only that path to Polyfin then receives their requests too.
package basepath

import (
	"context"
	"net/http"
)

type key struct{}

// With returns ctx telling that its request came under base, as the
// client wrote it.
func With(ctx context.Context, base string) context.Context {
	return context.WithValue(ctx, key{}, base)
}

// Of returns the base path r came under, or "" for the root.
func Of(r *http.Request) string {
	base, _ := r.Context().Value(key{}).(string)
	return base
}
