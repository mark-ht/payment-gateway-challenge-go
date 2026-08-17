package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
)

type httpMetrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
}

func newHTTPMetrics() *httpMetrics {
	metrics := &httpMetrics{
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "payment_gateway_http_requests_total",
			Help: "Total HTTP requests handled by the payment gateway.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "payment_gateway_http_request_duration_seconds",
			Help: "HTTP request duration handled by the payment gateway in seconds.",
		}, []string{"method", "route", "status"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "payment_gateway_http_in_flight_requests",
			Help: "HTTP requests currently handled by the payment gateway.",
		}),
	}
	metrics.registry.MustRegister(metrics.requests, metrics.duration, metrics.inFlight)
	return metrics
}

func (m *httpMetrics) instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Scrapes must not measure themselves, which would distort request-rate and latency telemetry.
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		response := &statusResponseWriter{ResponseWriter: w, status: http.StatusOK}
		m.inFlight.Inc()
		defer func() {
			m.inFlight.Dec()
			// Bound labels to prevent client input from creating sensitive, unbounded metric series.
			labels := prometheus.Labels{
				"method": safeMethod(r.Method),
				"route":  safeRoute(chi.RouteContext(r.Context()).RoutePattern()),
				"status": safeStatus(response.status),
			}
			m.requests.With(labels).Inc()
			m.duration.With(labels).Observe(time.Since(start).Seconds())
		}()
		next.ServeHTTP(response, r)
	})
}

func safeRoute(route string) string {
	if route == "" {
		// Unmatched paths are client-controlled, so use one fixed label rather than the raw URI.
		return "unmatched"
	}
	return route
}

func safeStatus(status int) string {
	if status < http.StatusContinue || status > 599 {
		return "unknown"
	}
	return strconv.Itoa(status)
}
