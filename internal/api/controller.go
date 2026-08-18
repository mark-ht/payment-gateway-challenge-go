package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cko-recruitment/payment-gateway-challenge-go/docs"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	httpSwagger "github.com/swaggo/http-swagger"
)

type pong struct {
	Message string `json:"message"`
}

type probeStatus struct {
	Status string `json:"status"`
}

// PingHandler returns an http.HandlerFunc that handles HTTP Ping GET requests.
func (a *Api) PingHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeProbeJSON(w, http.StatusOK, pong{Message: "pong"})
	}
}

// HealthHandler reports local process and router availability.
//
//	@Summary	Gateway health probe
//	@Description	Makes local dependency checks only and never calls the bank. This process serves it unauthenticated; production ingress and network policy must prevent public access and permit only Kubernetes probe traffic.
//	@Produce	json
//	@Success	200	{object}	probeStatus
//	@Router		/healthz [get]
func (a *Api) HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeProbeJSON(w, http.StatusOK, probeStatus{Status: "ok"})
	}
}

// LivenessHandler reports whether this process should remain running.
//
//	@Summary	Gateway liveness probe
//	@Description	Makes local dependency checks only and never calls the bank. This process serves it unauthenticated; production ingress and network policy must prevent public access and permit only Kubernetes probe traffic.
//	@Produce	json
//	@Success	200	{object}	probeStatus
//	@Router		/livez [get]
func (a *Api) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Probes must stay local so bank outages do not trigger Kubernetes restart loops.
		writeProbeJSON(w, http.StatusOK, probeStatus{Status: "ok"})
	}
}

// ReadinessHandler reports whether local API construction completed.
//
//	@Summary	Gateway readiness probe
//	@Description	Makes local dependency checks only and never calls the bank. This process serves it unauthenticated; production ingress and network policy must prevent public access and permit only Kubernetes probe traffic.
//	@Produce	json
//	@Success	200	{object}	probeStatus
//	@Failure	503	"Gateway not ready"
//	@Router		/readyz [get]
func (a *Api) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Readiness must stay local because the simulator has no card-free health check.
		if !a.ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeProbeJSON(w, http.StatusOK, probeStatus{Status: "ready"})
	}
}

func writeProbeJSON(w http.ResponseWriter, status int, response any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

// MetricsHandler exposes the gateway's Prometheus metrics registry.
//
//	@Summary	Gateway Prometheus metrics
//	@Description	Prometheus text exposition served unauthenticated by this process. Production ingress and network policy must enforce private Prometheus-only scraping rather than public access. It excludes /metrics from HTTP request metrics.
//	@Produce	plain
//	@Success	200	{string}	string
//	@Router		/metrics [get]
func (a *Api) MetricsHandler() http.HandlerFunc {
	return promhttp.HandlerFor(a.metrics.registry, promhttp.HandlerOpts{}).ServeHTTP
}

// SwaggerHandler returns an http.HandlerFunc that handles HTTP Swagger related requests.
func (a *Api) SwaggerHandler() http.HandlerFunc {
	return httpSwagger.Handler(
		httpSwagger.URL(fmt.Sprintf("http://%s/swagger/doc.json", docs.SwaggerInfo.Host)),
	)
}
