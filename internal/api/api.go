package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/bank"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/handlers"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/sync/errgroup"
)

type Api struct {
	router   *chi.Mux
	payments *handlers.PaymentsHandler
}

func New() (*Api, error) {
	config, err := loadConfig()
	if err != nil {
		return nil, err
	}
	store := repository.NewPaymentsRepository()
	authorizer := bank.NewClient(config.bankURL, config.bankTimeout)
	api := &Api{payments: handlers.NewPaymentsHandler(store, authorizer, func() time.Time { return time.Now().UTC() }, newPaymentID)}
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
	a.router = chi.NewRouter()
	a.router.Use(middleware.Logger)
	a.router.Get("/ping", a.PingHandler())
	a.router.Get("/swagger/*", a.SwaggerHandler())
	a.router.Post("/api/payments", a.payments.PostHandler())
	a.router.Get("/api/payments/{id}", a.payments.GetHandler())
}

func newPaymentID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
