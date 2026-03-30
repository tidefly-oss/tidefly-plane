package metrics

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "tidefly-plane"

// Registry wraps a prometheus registry and all custom gauges/counters.
// Use New() to create — never instantiate directly.
type Registry struct {
	prom *prometheus.Registry
	snap snapshot

	// System resources (updated by Collector)
	cpuPercent  prometheus.Gauge
	memUsedMB   prometheus.Gauge
	memTotalMB  prometheus.Gauge
	memPercent  prometheus.Gauge
	diskUsedMB  prometheus.Gauge
	diskTotalMB prometheus.Gauge
	diskPercent prometheus.Gauge

	// Runtime / process
	goroutines prometheus.Gauge
	uptime     prometheus.Gauge
	startTime  time.Time

	// Container stats
	ContainersTotal   prometheus.Gauge
	ContainersRunning prometheus.Gauge
	ContainersStopped prometheus.Gauge

	// HTTP instrumentation (use with promhttp.InstrumentHandler*)
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPResponseSize    *prometheus.HistogramVec

	// Job metrics
	JobsTotal  *prometheus.CounterVec
	JobsFailed *prometheus.CounterVec

	// Webhook metrics
	WebhookDeliveriesTotal  *prometheus.CounterVec
	WebhookDeliveriesFailed *prometheus.CounterVec
}

// New creates a new isolated prometheus registry with all Tidefly metrics
// pre-registered. It also registers Go runtime and process collectors.
func New() *Registry {
	reg := prometheus.NewRegistry()

	factory := promauto.With(reg)

	r := &Registry{
		prom:      reg,
		startTime: time.Now(),

		// ── System ──────────────────────────────────────────────────────────
		cpuPercent: factory.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "system",
				Name:      "cpu_usage_percent",
				Help:      "Current CPU utilization in percent (0–100).",
			},
		),
		memUsedMB: factory.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "system",
				Name:      "memory_used_megabytes",
				Help:      "Currently used RAM in megabytes.",
			},
		),
		memTotalMB: factory.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "system",
				Name:      "memory_total_megabytes",
				Help:      "Total installed RAM in megabytes.",
			},
		),
		memPercent: factory.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "system",
				Name:      "memory_usage_percent",
				Help:      "Current memory utilization in percent (0–100).",
			},
		),
		diskUsedMB: factory.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "system",
				Name:      "disk_used_megabytes",
				Help:      "Used disk space on the root partition in megabytes.",
			},
		),
		diskTotalMB: factory.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "system",
				Name:      "disk_total_megabytes",
				Help:      "Total disk capacity of the root partition in megabytes.",
			},
		),
		diskPercent: factory.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "system",
				Name:      "disk_usage_percent",
				Help:      "Current disk utilization in percent (0–100).",
			},
		),

		// ── Runtime ─────────────────────────────────────────────────────────
		goroutines: factory.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "runtime",
				Name:      "goroutines",
				Help:      "Number of goroutines currently running.",
			},
		),
		uptime: factory.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "runtime",
				Name:      "uptime_seconds",
				Help:      "Seconds since the Tidefly backend process started.",
			},
		),

		// ── Containers ──────────────────────────────────────────────────────
		ContainersTotal: factory.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "containers",
				Name:      "total",
				Help:      "Total number of managed containers.",
			},
		),
		ContainersRunning: factory.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "containers",
				Name:      "running",
				Help:      "Number of currently running managed containers.",
			},
		),
		ContainersStopped: factory.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "containers",
				Name:      "stopped",
				Help:      "Number of stopped managed containers.",
			},
		),

		// ── HTTP ────────────────────────────────────────────────────────────
		HTTPRequestsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total number of HTTP requests by method, route and status code.",
			}, []string{"method", "route", "status"},
		),

		HTTPRequestDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "HTTP request latency distribution.",
				Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
			}, []string{"method", "route"},
		),

		HTTPResponseSize: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "http",
				Name:      "response_size_bytes",
				Help:      "HTTP response size distribution in bytes.",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 6),
			}, []string{"method", "route"},
		),

		// ── Jobs ────────────────────────────────────────────────────────────
		JobsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "jobs",
				Name:      "processed_total",
				Help:      "Total background jobs processed by type.",
			}, []string{"type"},
		),

		JobsFailed: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "jobs",
				Name:      "failed_total",
				Help:      "Total background jobs that failed by type.",
			}, []string{"type"},
		),

		// ── Webhooks ────────────────────────────────────────────────────────
		WebhookDeliveriesTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "webhooks",
				Name:      "deliveries_total",
				Help:      "Total incoming webhook deliveries by provider.",
			}, []string{"provider"},
		),

		WebhookDeliveriesFailed: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "webhooks",
				Name:      "deliveries_failed_total",
				Help:      "Total failed webhook deliveries by provider.",
			}, []string{"provider"},
		),
	}

	// Standard Go runtime + process metrics (allocations, GC, FDs, …)
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(
			collectors.ProcessCollectorOpts{
				Namespace: namespace,
			},
		),
	)

	return r
}

// Prometheus returns the underlying registry for use with promhttp.HandlerFor.
func (r *Registry) Prometheus() *prometheus.Registry {
	return r.prom
}

// SetContainers updates container count gauges.
func (r *Registry) SetContainers(total, running, stopped int) {
	r.ContainersTotal.Set(float64(total))
	r.ContainersRunning.Set(float64(running))
	r.ContainersStopped.Set(float64(stopped))
}

// ObserveHTTP records a completed HTTP request. status should be the numeric
// status code as a string (use strconv.Itoa).
func (r *Registry) ObserveHTTP(method, route, status string, duration time.Duration, responseBytes int) {
	r.HTTPRequestsTotal.WithLabelValues(method, route, status).Inc()
	r.HTTPRequestDuration.WithLabelValues(method, route).Observe(duration.Seconds())
	r.HTTPResponseSize.WithLabelValues(method, route).Observe(float64(responseBytes))
}

// IncJob increments the jobs_processed_total counter for the given job type.
func (r *Registry) IncJob(jobType string) {
	r.JobsTotal.WithLabelValues(jobType).Inc()
}

// IncJobFailed increments the jobs_failed_total counter for the given job type.
func (r *Registry) IncJobFailed(jobType string, err error) {
	r.JobsFailed.WithLabelValues(jobType).Inc()
	_ = fmt.Sprintf("job %s failed: %v", jobType, err) // surfaced via structured log, not metrics
}

// IncWebhook increments webhook delivery counters.
func (r *Registry) IncWebhook(provider string, failed bool) {
	r.WebhookDeliveriesTotal.WithLabelValues(provider).Inc()
	if failed {
		r.WebhookDeliveriesFailed.WithLabelValues(provider).Inc()
	}
}
