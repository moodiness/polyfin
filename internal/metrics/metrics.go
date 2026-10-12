// Package metrics measures how Polyfin runs, in Prometheus' text format,
// at /metrics (see Handler): the server's parts count what they do with a
// call here, and the rest is read from their state when Prometheus asks.
// Labels take their values from small fixed sets, and never name a user,
// a title, an address, a device or a token: addons are named as they are
// listed.
package metrics

import (
	"context"
	"crypto/subtle"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

// registry holds Polyfin's metrics, apart from the default registry of
// the Prometheus library, which other libraries may fill.
var registry = prometheus.NewRegistry()

// Outcomes of what is counted.
const (
	Success = "success"
	Failure = "failure"
)

// The APIs HTTP requests are counted by.
const (
	APIJellyfin = "jellyfin"
	APIAdmin    = "admin"
	APIWeb      = "web"
	APIOther    = "other"
)

var (
	httpRequests = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "polyfin_http_request_duration_seconds",
		Help:    "HTTP requests, by API (jellyfin, admin, web, other) and status class, and how long their answers took.",
		Buckets: []float64{0.005, 0.025, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
	}, []string{"api", "status"})
	segments = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "polyfin_segments_total",
		Help: "HLS segments made of files, by method: nvidia, vaapi or processor when the video is converted, copy when it is copied.",
	}, []string{"method"})
	encodingFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "polyfin_encoding_failures_total",
		Help: "Remuxes and conversions FFmpeg failed, of files and channels, by method: nvidia, vaapi, processor or copy.",
	}, []string{"method"})
	imports = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "polyfin_imports_total",
		Help: "Imports of app connections and tracking services, by service and outcome.",
	}, []string{"service", "outcome"})
	trackingSent = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "polyfin_tracking_sent_total",
		Help: "Changes sent to tracking services, such as what was watched, by service and outcome.",
	}, []string{"service", "outcome"})
	notifications = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "polyfin_notifications_total",
		Help: "Notifications sent to targets, each try counted, by kind of target and outcome.",
	}, []string{"target", "outcome"})
	scansRunning = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "polyfin_scans_running",
		Help: "Scans of local folders under way.",
	})
	scanDurations = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "polyfin_scan_duration_seconds",
		Help:    "Scans of local folders, and how long they took.",
		Buckets: []float64{1, 5, 15, 60, 300, 900, 3600},
	})
)

func init() {
	registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		httpRequests, segments, encodingFailures, imports, trackingSent, notifications, scansRunning, scanDurations, addonRequests)
}

// Register adds collectors that read the server's state when Prometheus
// asks, such as the playbacks under way (see NewGauges).
func Register(collectors ...prometheus.Collector) {
	registry.MustRegister(collectors...)
}

// outcome is Success when ok, else Failure.
func outcome(ok bool) string {
	if ok {
		return Success
	}
	return Failure
}

// HTTPRequest counts a request to api answered with status, which took
// took to answer.
func HTTPRequest(api string, status int, took time.Duration) {
	httpRequests.WithLabelValues(api, strconv.Itoa(status/100)+"xx").Observe(took.Seconds())
}

// SegmentMade counts a segment made by method (see the hls package).
func SegmentMade(method string) {
	segments.WithLabelValues(method).Inc()
}

// EncodingFailed counts a remux or conversion that failed, by method.
func EncodingFailed(method string) {
	encodingFailures.WithLabelValues(method).Inc()
}

// Imported counts an import of an app connection or a tracking service,
// by its service, ok when it read and imported everything.
func Imported(service string, ok bool) {
	imports.WithLabelValues(service, outcome(ok)).Inc()
}

// TrackingSent counts a change sent to a tracking service, ok when the
// service took it.
func TrackingSent(service string, ok bool) {
	trackingSent.WithLabelValues(service, outcome(ok)).Inc()
}

// NotificationSent counts a try at sending a notification to a target of
// a kind, ok when the target took it.
func NotificationSent(target string, ok bool) {
	notifications.WithLabelValues(target, outcome(ok)).Inc()
}

// ScanStarted counts a scan of a local folder under way, until finished
// is called once it ended.
func ScanStarted() (finished func()) {
	started := time.Now()
	scansRunning.Inc()
	return func() {
		scansRunning.Dec()
		scanDurations.Observe(time.Since(started).Seconds())
	}
}

// Sample is a value read when Prometheus asks, with its label values.
type Sample struct {
	Labels []string
	Value  float64
}

// gauges reports the samples read returns as one gauge.
type gauges struct {
	desc *prometheus.Desc
	read func(ctx context.Context) []Sample
}

// Value reads the series of the metric named name whose labels are
// labels: a counter's or a gauge's value, or a histogram's count; 0 when
// there is no such series. Tests read the metrics so.
func Value(name string, labels map[string]string) float64 {
	families, err := registry.Gather()
	if err != nil {
		return 0
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			pairs := metric.GetLabel()
			if len(pairs) != len(labels) || slices.ContainsFunc(pairs, func(pair *dto.LabelPair) bool {
				return labels[pair.GetName()] != pair.GetValue()
			}) {
				continue
			}
			switch {
			case metric.GetCounter() != nil:
				return metric.GetCounter().GetValue()
			case metric.GetGauge() != nil:
				return metric.GetGauge().GetValue()
			case metric.GetHistogram() != nil:
				return float64(metric.GetHistogram().GetSampleCount())
			}
		}
	}
	return 0
}

// readTimeout bounds what a gauge reads, such as the database.
const readTimeout = 5 * time.Second

// NewGauges returns a gauge named name, with labels, whose values read
// returns when Prometheus asks.
func NewGauges(name, help string, labels []string, read func(ctx context.Context) []Sample) prometheus.Collector {
	return gauges{desc: prometheus.NewDesc(name, help, labels, nil), read: read}
}

func (g gauges) Describe(descs chan<- *prometheus.Desc) { descs <- g.desc }

func (g gauges) Collect(metrics chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	for _, sample := range g.read(ctx) {
		metrics <- prometheus.MustNewConstMetric(g.desc, prometheus.GaugeValue, sample.Value, sample.Labels...)
	}
}

// Handler answers /metrics while settings tells it is on, to requests
// that send the token it gives as a bearer token, compared in constant
// time; 404 while off, 401 without the token or with another.
func Handler(settings func() (on bool, token string)) http.Handler {
	// The server compresses the answer as it does others.
	serve := promhttp.HandlerFor(registry, promhttp.HandlerOpts{ErrorHandling: promhttp.ContinueOnError, DisableCompression: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		on, token := settings()
		if !on {
			http.NotFound(w, r)
			return
		}
		scheme, given, _ := strings.Cut(r.Header.Get("Authorization"), " ")
		if token == "" || !strings.EqualFold(scheme, "Bearer") ||
			subtle.ConstantTimeCompare([]byte(strings.TrimSpace(given)), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		serve.ServeHTTP(w, r)
	})
}
