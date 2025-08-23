package server

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mailez_http_requests_total",
			Help: "HTTP requests processed, by method, route and status.",
		},
		[]string{"method", "route", "status"},
	)
	httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mailez_http_request_duration_seconds",
			Help:    "HTTP request latency.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)
)

func init() {
	prometheus.MustRegister(httpRequests, httpDuration)
}

// metricsMiddleware records request counts and latency. The route is the
// matched Fiber route pattern (e.g. /api/v1/users/:email), keeping label
// cardinality bounded.
func metricsMiddleware(c *fiber.Ctx) error {
	start := time.Now()
	err := c.Next()
	route := c.Route().Path
	status := strconv.Itoa(c.Response().StatusCode())
	httpRequests.WithLabelValues(c.Method(), route, status).Inc()
	httpDuration.WithLabelValues(c.Method(), route).Observe(time.Since(start).Seconds())
	return err
}
