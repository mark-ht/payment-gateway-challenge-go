package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cko-recruitment/payment-gateway-challenge-go/docs"
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
//	@Description	Reports local process and router availability; it never calls the bank.
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
//	@Description	Reports local process availability; it never calls the bank.
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
//	@Description	Reports completed local API construction only; it never calls the bank.
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

// SwaggerHandler returns an http.HandlerFunc that handles HTTP Swagger related requests.
func (a *Api) SwaggerHandler() http.HandlerFunc {
	return httpSwagger.Handler(
		httpSwagger.URL(fmt.Sprintf("http://%s/swagger/doc.json", docs.SwaggerInfo.Host)),
	)
}
