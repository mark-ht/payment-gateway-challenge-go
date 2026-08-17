package bank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/models"
)

func TestClientRejectsMalformedBankResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"authorized":true} trailing`))
	}))
	defer server.Close()

	client := NewClient(server.URL, time.Second)
	_, err := client.Authorize(context.Background(), models.PaymentRequest{CardNumber: "00000000000001", ExpiryMonth: 4, ExpiryYear: 2030, Currency: "GBP", Amount: 100, CVV: "123"})
	if err == nil {
		t.Fatal("Authorize() error = nil, want malformed response error")
	}
}

func TestClientMapsDeclinesAndBankFailuresToErrorsOrDecisions(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantResult bool
		wantErr    bool
	}{
		{"declined", http.StatusOK, `{"authorized":false}`, false, false},
		{"unavailable", http.StatusServiceUnavailable, ``, false, true},
		{"unexpected status", http.StatusBadRequest, ``, false, true},
		{"missing decision", http.StatusOK, `{}`, false, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			client := NewClient(server.URL, time.Second)
			got, err := client.Authorize(context.Background(), models.PaymentRequest{CardNumber: "00000000000002", ExpiryMonth: 4, ExpiryYear: 2030, Currency: "GBP", Amount: 100, CVV: "123"})
			if (err != nil) != test.wantErr || got != test.wantResult {
				t.Fatalf("Authorize() = (%t, %v), want (%t, error=%t)", got, err, test.wantResult, test.wantErr)
			}
		})
	}
}

func TestClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()
	client := NewClient(server.URL, time.Millisecond)
	_, err := client.Authorize(context.Background(), models.PaymentRequest{CardNumber: "00000000000001", ExpiryMonth: 4, ExpiryYear: 2030, Currency: "GBP", Amount: 100, CVV: "123"})
	if err == nil {
		t.Fatal("Authorize() error = nil, want timeout error")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestClientPropagatesRequestCancellation(t *testing.T) {
	transportStarted := make(chan struct{})
	client := NewClient("http://bank.test/payments", time.Second)
	client.httpClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		close(transportStarted)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.Authorize(ctx, models.PaymentRequest{CardNumber: "00000000000001", ExpiryMonth: 4, ExpiryYear: 2030, Currency: "GBP", Amount: 100, CVV: "123"})
		result <- err
	}()

	select {
	case <-transportStarted:
	case <-time.After(time.Second):
		t.Fatal("HTTP transport did not receive request")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("Authorize() error = nil after context cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("Authorize() did not return after context cancellation")
	}
}

func TestClientReturnsNetworkFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close()

	client := NewClient(url, time.Second)
	_, err := client.Authorize(context.Background(), models.PaymentRequest{CardNumber: "00000000000001", ExpiryMonth: 4, ExpiryYear: 2030, Currency: "GBP", Amount: 100, CVV: "123"})
	if err == nil {
		t.Fatal("Authorize() error = nil, want network failure")
	}
}

func TestClientAuthorizesUsingSimulatorContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/payments" {
			t.Fatalf("request = %s %s, want POST /payments", r.Method, r.URL.Path)
		}
		if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", contentType)
		}
		var body struct {
			CardNumber string `json:"card_number"`
			ExpiryDate string `json:"expiry_date"`
			Currency   string `json:"currency"`
			Amount     int    `json:"amount"`
			CVV        string `json:"cvv"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.CardNumber != "00000000000001" || body.ExpiryDate != "04/2030" || body.Currency != "GBP" || body.Amount != 100 || body.CVV != "123" {
			t.Fatal("bank request did not preserve the required simulator fields")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authorized":true}`))
	}))
	defer server.Close()

	client := NewClient(server.URL+"/payments", time.Second)
	authorized, err := client.Authorize(context.Background(), models.PaymentRequest{CardNumber: "00000000000001", ExpiryMonth: 4, ExpiryYear: 2030, Currency: "GBP", Amount: 100, CVV: "123"})
	if err != nil || !authorized {
		t.Fatalf("Authorize() = (%t, %v), want (true, nil)", authorized, err)
	}
}
