package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/bank"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/handlers"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/repository"
	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/errgroup"
)

type Api struct {
	router       *chi.Mux
	payments     *handlers.PaymentsHandler
	accessLogger *log.Logger
}

var accessLogSequence atomic.Uint64

func New() (*Api, error) {
	config, err := loadConfig()
	if err != nil {
		return nil, err
	}
	store := repository.NewPaymentsRepository()
	authorizer := bank.NewClient(config.bankURL, config.bankTimeout)
	api := &Api{
		payments:     handlers.NewPaymentsHandler(store, authorizer, func() time.Time { return time.Now().UTC() }, newPaymentID),
		accessLogger: log.New(os.Stderr, "", 0),
	}
	api.setupRouter()
	return api, nil
}

func (a *Api) Run(ctx context.Context, addr string) error {
	httpServer := &http.Server{
		Addr:        addr,
		Handler:     a.router,
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		<-ctx.Done()
		return httpServer.Shutdown(ctx)
	})
	g.Go(func() error {
		fmt.Printf("starting HTTP server on %s\n", addr)
		err := httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
	return g.Wait()
}

func (a *Api) setupRouter() {
	if a.accessLogger == nil {
		a.accessLogger = log.New(os.Stderr, "", 0)
	}
	a.router = chi.NewRouter()
	a.router.Use(a.accessLog)
	a.router.Get("/ping", a.PingHandler())
	a.router.Get("/swagger/*", a.SwaggerHandler())
	a.router.Post("/api/payments", a.payments.PostHandler())
	a.router.Get("/api/payments/{id}", a.payments.GetHandler())
}

func (a *Api) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		correlationID := newCorrelationID()
		response := &statusResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(response, r)

		// Log only selected server-derived fields so request data cannot expose payment details.
		a.accessLogger.Printf("method=%s route=%s status=%d duration=%s correlation_id=%s", safeMethod(r.Method), chi.RouteContext(r.Context()).RoutePattern(), response.status, time.Since(start), correlationID)
	})
}

func safeMethod(method string) string {
	switch method {
	case http.MethodConnect, http.MethodDelete, http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPatch, http.MethodPost, http.MethodPut, http.MethodTrace:
		return method
	default:
		// Method tokens are client-controlled, so unknown values must not enter access logs.
		return "OTHER"
	}
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func newCorrelationID() string {
	id, err := newPaymentID()
	if err == nil {
		return id
	}
	return fmt.Sprintf("fallback-%d", accessLogSequence.Add(1))
}

func newPaymentID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
