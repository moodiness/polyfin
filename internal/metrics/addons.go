package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Requests to addons are counted by the address of the addon asked, which
// may hold credentials and is never shown: when Prometheus asks, each
// addon is named as it is listed (see AddonNames), and the addons that
// share a name are counted together.

// addonBuckets are the bounds of the durations of addon requests, in
// seconds; a request is given 15 seconds at most.
var addonBuckets = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 15}

// maxAddonSeries bounds the series kept: past it, the requests of addons
// not counted yet are not.
const maxAddonSeries = 10000

var addonDesc = prometheus.NewDesc("polyfin_addon_request_duration_seconds",
	"Requests to addons, by addon as listed, resource (manifest, catalog, meta, stream, subtitles, other) and outcome, and how long their answers took.",
	[]string{"addon", "resource", "outcome"}, nil)

// addonSeries is one series of the addon requests: by addon, a key, or
// its name once named.
type addonSeries struct {
	addon, resource, outcome string
}

// histogram counts observations in addonBuckets, each in the first bucket
// that holds it, the last one past them all.
type histogram struct {
	count   uint64
	sum     float64
	buckets [9]uint64
}

func (h *histogram) observe(seconds float64) {
	h.count++
	h.sum += seconds
	i := 0
	for i < len(addonBuckets) && seconds > addonBuckets[i] {
		i++
	}
	h.buckets[i]++
}

func (h *histogram) add(other *histogram) {
	h.count += other.count
	h.sum += other.sum
	for i := range h.buckets {
		h.buckets[i] += other.buckets[i]
	}
}

// cumulative is the count of observations up to each bound, as Prometheus
// counts them.
func (h *histogram) cumulative() map[float64]uint64 {
	counts := make(map[float64]uint64, len(addonBuckets))
	var total uint64
	for i, bound := range addonBuckets {
		total += h.buckets[i]
		counts[bound] = total
	}
	return counts
}

// addonCollector counts the addon requests, and names their addons when
// Prometheus asks.
type addonCollector struct {
	mu     sync.Mutex
	series map[addonSeries]*histogram
	names  func(ctx context.Context) (map[string]string, error)
}

var addonRequests = &addonCollector{series: map[addonSeries]*histogram{}}

// AddonRequest counts a request for resource to the addon whose key is
// addon, its address, which took took, ok unless it failed (see
// stremio.Health).
func AddonRequest(addon, resource string, took time.Duration, ok bool) {
	c := addonRequests
	key := addonSeries{addon: addon, resource: resource, outcome: outcome(ok)}
	c.mu.Lock()
	defer c.mu.Unlock()
	h := c.series[key]
	if h == nil {
		if len(c.series) >= maxAddonSeries {
			return
		}
		h = &histogram{}
		c.series[key] = h
	}
	h.observe(took.Seconds())
}

// AddonNames names the addons when Prometheus asks: names maps the key of
// each addon installed to the name it is listed under. The requests of
// addons it does not name, removed since, are no longer reported.
func AddonNames(names func(ctx context.Context) (map[string]string, error)) {
	c := addonRequests
	c.mu.Lock()
	c.names = names
	c.mu.Unlock()
}

func (c *addonCollector) Describe(descs chan<- *prometheus.Desc) { descs <- addonDesc }

func (c *addonCollector) Collect(metrics chan<- prometheus.Metric) {
	c.mu.Lock()
	read := c.names
	c.mu.Unlock()
	if read == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	names, err := read(ctx)
	if err != nil {
		return
	}
	named := map[addonSeries]*histogram{}
	c.mu.Lock()
	for key, h := range c.series {
		name, ok := names[key.addon]
		if !ok {
			delete(c.series, key)
			continue
		}
		key.addon = name
		if named[key] == nil {
			named[key] = &histogram{}
		}
		named[key].add(h)
	}
	c.mu.Unlock()
	for key, h := range named {
		metrics <- prometheus.MustNewConstHistogram(addonDesc, h.count, h.sum, h.cumulative(), key.addon, key.resource, key.outcome)
	}
}
