package api

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/models"
	"github.com/go-chi/chi/v5/middleware"
)

func paymentRequestJSON(t *testing.T, cardNumber string) string {
	t.Helper()
	expiry := time.Now().UTC().AddDate(0, 1, 0)
	body, err := json.Marshal(models.PaymentRequest{CardNumber: cardNumber, ExpiryMonth: int(expiry.Month()), ExpiryYear: expiry.Year(), Currency: "GBP", Amount: 100, CVV: "123"})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestAPIAccessLogIncludesOnlySafeOperationalFields(t *testing.T) {
	var logs bytes.Buffer
	gateway := &Api{accessLogger: log.New(&logs, "", 0)}
	gateway.setupRouter()

	request := httptest.NewRequest(http.MethodGet, "/ping?query-sentinel", strings.NewReader("body-sentinel"))
	request.Header.Set("X-Sentinel", "header-sentinel")
	request.Header.Set("X-Correlation-ID", "client-correlation-sentinel")
	gateway.router.ServeHTTP(httptest.NewRecorder(), request)

	entry := logs.String()
	for _, sentinel := range []string{"query-sentinel", "body-sentinel", "header-sentinel", "client-correlation-sentinel"} {
		if strings.Contains(entry, sentinel) {
			t.Errorf("access log contains prohibited value %q: %q", sentinel, entry)
		}
	}
	if !regexp.MustCompile(`^method=GET route=/ping status=200 duration=[0-9.]+(?:ns|µs|ms|s) correlation_id=[0-9a-f]{32}\n$`).MatchString(entry) {
		t.Errorf("access log = %q, want only safe operational fields", entry)
	}
}

func TestAPIAccessLogDoesNotIncludeUnsupportedMethodToken(t *testing.T) {
	const methodSentinel = "4242424242424242"

	var logs bytes.Buffer
	gateway := &Api{accessLogger: log.New(&logs, "", 0)}
	gateway.setupRouter()
	gateway.router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(methodSentinel, "/ping", nil))

	entry := logs.String()
	if strings.Contains(entry, methodSentinel) {
		t.Fatalf("access log contains unsupported method token: %q", entry)
	}
	if !strings.Contains(entry, "method=OTHER ") {
		t.Fatalf("access log = %q, want fixed unsupported-method marker", entry)
	}
}

func TestAPIDoesNotInstallRequestLogger(t *testing.T) {
	originalLogger := middleware.DefaultLogger
	defer func() { middleware.DefaultLogger = originalLogger }()

	var loggedURI string
	middleware.DefaultLogger = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			loggedURI = r.RequestURI
			next.ServeHTTP(w, r)
		})
	}

	gateway := &Api{}
	gateway.setupRouter()
	gateway.router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ping?untrusted=value", nil))

	if loggedURI != "" {
		t.Fatalf("request logger received URI %q", loggedURI)
	}
}

func TestAPIProcessesAndRetrievesPayment(t *testing.T) {
	bank := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/payments" {
			t.Fatalf("bank request = %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"authorized":true}`))
	}))
	defer bank.Close()
	t.Setenv("BANK_SIMULATOR_URL", bank.URL+"/payments")
	t.Setenv("BANK_SIMULATOR_TIMEOUT", "1s")

	gateway, err := New()
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().UTC().AddDate(0, 1, 0)
	body, err := json.Marshal(struct {
		models.PaymentRequest
		Ignored bool `json:"ignored"`
	}{
		PaymentRequest: models.PaymentRequest{CardNumber: "00000000000001", ExpiryMonth: int(expiry.Month()), ExpiryYear: expiry.Year(), Currency: "gbp", Amount: 100, CVV: "123"},
		Ignored:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	post := httptest.NewRecorder()
	gateway.router.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(string(body))))
	if post.Code != http.StatusOK {
		t.Fatalf("POST status = %d", post.Code)
	}
	var created models.Payment
	if err := json.NewDecoder(post.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRecorder()
	gateway.router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/payments/"+created.ID, nil))
	if get.Code != http.StatusOK {
		t.Fatalf("GET status = %d", get.Code)
	}
	var retrieved models.Payment
	if err := json.NewDecoder(get.Body).Decode(&retrieved); err != nil || retrieved != created {
		t.Fatalf("GET payment/error = %+v/%v", retrieved, err)
	}
}

func TestAPIMapsDeclinesAndUnavailableBank(t *testing.T) {
	tests := []struct {
		name       string
		bankStatus int
		bankBody   string
		wantStatus int
		wantResult string
	}{
		{"declined", http.StatusOK, `{"authorized":false}`, http.StatusOK, "Declined"},
		{"unavailable", http.StatusServiceUnavailable, ``, http.StatusServiceUnavailable, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bank := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.bankStatus)
				_, _ = w.Write([]byte(test.bankBody))
			}))
			defer bank.Close()
			t.Setenv("BANK_SIMULATOR_URL", bank.URL+"/payments")
			t.Setenv("BANK_SIMULATOR_TIMEOUT", "1s")
			gateway, err := New()
			if err != nil {
				t.Fatal(err)
			}

			recorder := httptest.NewRecorder()
			gateway.router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(paymentRequestJSON(t, "00000000000002"))))
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			if test.wantResult != "" {
				var payment models.Payment
				if err := json.NewDecoder(recorder.Body).Decode(&payment); err != nil {
					t.Fatal(err)
				}
				if payment.Status != test.wantResult {
					t.Fatalf("status = %q, want %q", payment.Status, test.wantResult)
				}
			}
		})
	}
}

func TestAPIRejectsNonObjectAndTrailingJSONWithoutBankCall(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{"non-object", `[]`},
		{"trailing", paymentRequestJSON(t, "00000000000001") + `{}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			bank := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				_, _ = w.Write([]byte(`{"authorized":true}`))
			}))
			defer bank.Close()
			t.Setenv("BANK_SIMULATOR_URL", bank.URL+"/payments")
			gateway, err := New()
			if err != nil {
				t.Fatal(err)
			}

			recorder := httptest.NewRecorder()
			gateway.router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(test.body)))
			if recorder.Code != http.StatusBadRequest || calls != 0 {
				t.Fatalf("status/calls = %d/%d, want 400/0", recorder.Code, calls)
			}
		})
	}
}

func TestAPICompletedPaymentsReceiveDistinctIDs(t *testing.T) {
	bank := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"authorized":true}`))
	}))
	defer bank.Close()
	t.Setenv("BANK_SIMULATOR_URL", bank.URL+"/payments")
	gateway, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ids := make(map[string]struct{}, 2)
	for _, cardNumber := range []string{"00000000000001", "00000000000003"} {
		recorder := httptest.NewRecorder()
		gateway.router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(paymentRequestJSON(t, cardNumber))))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", recorder.Code)
		}
		var payment models.Payment
		if err := json.NewDecoder(recorder.Body).Decode(&payment); err != nil {
			t.Fatal(err)
		}
		if payment.ID == "" {
			t.Fatal("completed payment ID is empty")
		}
		if _, exists := ids[payment.ID]; exists {
			t.Fatal("completed payment IDs are not distinct")
		}
		ids[payment.ID] = struct{}{}
	}
}
