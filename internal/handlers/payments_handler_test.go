package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/bank"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/models"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/repository"
	"github.com/go-chi/chi/v5"
)

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

type fakeAuthorizer struct {
	authorized bool
	err        error
	calls      int
}

func (f *fakeAuthorizer) Authorize(_ context.Context, _ models.PaymentRequest) (bool, error) {
	f.calls++
	return f.authorized, f.err
}

type authorizerFunc func(context.Context, models.PaymentRequest) (bool, error)

func (f authorizerFunc) Authorize(ctx context.Context, request models.PaymentRequest) (bool, error) {
	return f(ctx, request)
}

func TestPostPaymentHandlerGeneratesIDBeforeAuthorization(t *testing.T) {
	store := repository.NewPaymentsRepository()
	var calls []string
	bank := authorizerFunc(func(context.Context, models.PaymentRequest) (bool, error) {
		calls = append(calls, "authorize")
		return true, nil
	})
	handler := NewPaymentsHandler(store, bank, func() time.Time { return time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC) }, func() string {
		calls = append(calls, "id")
		return "payment-id"
	})

	recorder := httptest.NewRecorder()
	handler.PostHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(`{"card_number":"00000000000001","expiry_month":5,"expiry_year":2026,"currency":"GBP","amount":100,"cvv":"123"}`)))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got, want := strings.Join(calls, ","), "id,authorize"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
}

func TestPostPaymentHandlerRetriesIDCollisionWithoutOverwrite(t *testing.T) {
	store := repository.NewPaymentsRepository()
	existing := models.Payment{ID: "collision", Status: "Declined", Amount: 200}
	if !store.Create(existing) {
		t.Fatal("Create(existing) = false, want true")
	}
	ids := []string{"collision", "replacement"}
	nextID := 0
	handler := NewPaymentsHandler(store, &fakeAuthorizer{authorized: true}, func() time.Time { return time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC) }, func() string {
		id := ids[nextID]
		nextID++
		return id
	})

	recorder := httptest.NewRecorder()
	handler.PostHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(`{"card_number":"00000000000001","expiry_month":5,"expiry_year":2026,"currency":"GBP","amount":100,"cvv":"123"}`)))

	var payment models.Payment
	if err := json.NewDecoder(recorder.Body).Decode(&payment); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || payment.ID != "replacement" || nextID != 2 {
		t.Fatalf("status/payment/id calls = %d/%+v/%d, want 200/replacement/2", recorder.Code, payment, nextID)
	}
	if got, found := store.Get(existing.ID); !found || got != existing {
		t.Fatalf("existing payment = (%+v, %t), want (%+v, true)", got, found, existing)
	}
	if got, found := store.Get(payment.ID); !found || got != payment {
		t.Fatalf("replacement payment = (%+v, %t), want (%+v, true)", got, found, payment)
	}
}

func TestPostPaymentHandlerAuthorizesAndStoresOnlySafeFields(t *testing.T) {
	store := repository.NewPaymentsRepository()
	bank := &fakeAuthorizer{authorized: true}
	handler := NewPaymentsHandler(store, bank, func() time.Time { return time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC) }, func() string { return "payment-id" })

	recorder := httptest.NewRecorder()
	handler.PostHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(`{"card_number":"00000000000001","expiry_month":5,"expiry_year":2026,"currency":"gbp","amount":100,"cvv":"123"}`)))

	if recorder.Code != http.StatusOK || bank.calls != 1 {
		t.Fatalf("status/calls = %d/%d, want 200/1", recorder.Code, bank.calls)
	}
	var response map[string]json.RawMessage
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response) != 7 {
		t.Fatalf("response fields = %v, want exactly seven safe fields", response)
	}
	var payment models.Payment
	if err := json.Unmarshal(mustJSON(t, response), &payment); err != nil {
		t.Fatal(err)
	}
	if payment != (models.Payment{ID: "payment-id", Status: "Authorized", CardNumberLastFour: "0001", ExpiryMonth: 5, ExpiryYear: 2026, Currency: "GBP", Amount: 100}) {
		t.Fatalf("payment = %+v", payment)
	}
	if strings.Contains(recorder.Body.String(), "00000000000001") || strings.Contains(recorder.Body.String(), "123") {
		t.Fatal("response includes sensitive card data")
	}
	if _, found := store.Get("payment-id"); !found {
		t.Fatal("payment was not stored")
	}
}

func TestPostPaymentHandlerRejectsInvalidInputBeforeBankCall(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{"empty", ``},
		{"malformed JSON", `{`},
		{"non-object JSON", `[]`},
		{"invalid field", `{"card_number":"short","expiry_month":5,"expiry_year":2026,"currency":"GBP","amount":100,"cvv":"123"}`},
		{"trailing JSON", `{"card_number":"00000000000001","expiry_month":5,"expiry_year":2026,"currency":"GBP","amount":100,"cvv":"123"} {}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := repository.NewPaymentsRepository()
			bank := &fakeAuthorizer{authorized: true}
			handler := NewPaymentsHandler(store, bank, func() time.Time { return time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC) }, func() string { return "payment-id" })
			recorder := httptest.NewRecorder()
			handler.PostHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(test.body)))
			if recorder.Code != http.StatusBadRequest || bank.calls != 0 || recorder.Body.String() != "{\"status\":\"Rejected\"}\n" {
				t.Fatalf("status/calls/body = %d/%d/%q", recorder.Code, bank.calls, recorder.Body.String())
			}
		})
	}
}

func TestPostPaymentHandlerRejectsOversizedBodyBeforeBankCall(t *testing.T) {
	store := repository.NewPaymentsRepository()
	bank := &fakeAuthorizer{authorized: true}
	handler := NewPaymentsHandler(store, bank, func() time.Time { return time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC) }, func() string { return "payment-id" })
	body := `{"card_number":"00000000000001","expiry_month":5,"expiry_year":2026,"currency":"GBP","amount":100,"cvv":"123","padding":"` + strings.Repeat("x", 64*1024) + `"}`

	recorder := httptest.NewRecorder()
	handler.PostHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(body)))

	if recorder.Code != http.StatusRequestEntityTooLarge || bank.calls != 0 || recorder.Body.String() != "{\"status\":\"Rejected\"}\n" {
		t.Fatalf("status/calls/body = %d/%d/%q, want 413/0/rejected", recorder.Code, bank.calls, recorder.Body.String())
	}
	if _, found := store.Get("payment-id"); found {
		t.Fatal("oversized request was stored")
	}
}

func TestPostPaymentHandlerDoesNotStoreOversizedBankResponse(t *testing.T) {
	bankServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"authorized":true,"padding":"` + strings.Repeat("x", 64*1024) + `"}`))
	}))
	defer bankServer.Close()

	store := repository.NewPaymentsRepository()
	handler := NewPaymentsHandler(store, bank.NewClient(bankServer.URL, time.Second), func() time.Time { return time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC) }, func() string { return "payment-id" })
	recorder := httptest.NewRecorder()
	handler.PostHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(`{"card_number":"00000000000001","expiry_month":5,"expiry_year":2026,"currency":"GBP","amount":100,"cvv":"123"}`)))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
	if _, found := store.Get("payment-id"); found {
		t.Fatal("oversized bank response was stored")
	}
}

func TestPostPaymentHandlerDoesNotStoreBankFailures(t *testing.T) {
	store := repository.NewPaymentsRepository()
	bank := &fakeAuthorizer{err: errors.New("bank unavailable")}
	handler := NewPaymentsHandler(store, bank, func() time.Time { return time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC) }, func() string { return "payment-id" })
	recorder := httptest.NewRecorder()
	handler.PostHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(`{"card_number":"00000000000001","expiry_month":5,"expiry_year":2026,"currency":"GBP","amount":100,"cvv":"123"}`)))
	if recorder.Code != http.StatusServiceUnavailable || recorder.Body.Len() != 0 {
		t.Fatalf("status/body length = %d/%d", recorder.Code, recorder.Body.Len())
	}
	if _, found := store.Get("payment-id"); found {
		t.Fatal("bank failure was stored")
	}
}

func TestPostPaymentHandlerReturnsDeclined(t *testing.T) {
	store := repository.NewPaymentsRepository()
	bank := &fakeAuthorizer{authorized: false}
	handler := NewPaymentsHandler(store, bank, func() time.Time { return time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC) }, func() string { return "payment-id" })
	recorder := httptest.NewRecorder()
	handler.PostHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(`{"card_number":"00000000000002","expiry_month":5,"expiry_year":2026,"currency":"GBP","amount":100,"cvv":"123"}`)))
	var payment models.Payment
	if err := json.NewDecoder(recorder.Body).Decode(&payment); err != nil || recorder.Code != http.StatusOK || payment.Status != "Declined" {
		t.Fatalf("status/payment/error = %d/%+v/%v", recorder.Code, payment, err)
	}
}

func TestGetPaymentHandler(t *testing.T) {
	store := repository.NewPaymentsRepository()
	store.Create(models.Payment{ID: "test-id", Status: "Authorized", CardNumberLastFour: "0001", ExpiryMonth: 10, ExpiryYear: 2035, Currency: "GBP", Amount: 100})
	handler := NewPaymentsHandler(store, nil, func() time.Time { return time.Now().UTC() }, func() string { return "id" })

	router := chi.NewRouter()
	router.Get("/api/payments/{id}", handler.GetHandler())

	for _, test := range []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"found", "/api/payments/test-id", http.StatusOK},
		{"not found", "/api/payments/missing", http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
		})
	}
}
