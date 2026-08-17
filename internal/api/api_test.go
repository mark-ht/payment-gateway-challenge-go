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

func TestAPIProbesAreLocalAndReturnAcceptedStatus(t *testing.T) {
	bankCalls := 0
	bankServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bankCalls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bankServer.Close()
	t.Setenv("BANK_SIMULATOR_URL", bankServer.URL+"/payments")

	gateway, err := New()
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		path string
		body string
	}{
		{path: "/healthz", body: `{"status":"ok"}` + "\n"},
		{path: "/livez", body: `{"status":"ok"}` + "\n"},
		{path: "/readyz", body: `{"status":"ready"}` + "\n"},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			gateway.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if response.Body.String() != test.body {
				t.Fatalf("body = %q, want %q", response.Body.String(), test.body)
			}
		})
	}
	if bankCalls != 0 {
		t.Fatalf("bank calls = %d, want 0", bankCalls)
	}
}

func TestAPIMetricsUseSafeBoundedLabelsAndExcludeMetricsEndpoint(t *testing.T) {
	bankServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer bankServer.Close()
	t.Setenv("BANK_SIMULATOR_URL", bankServer.URL+"/payments")

	gateway, err := New()
	if err != nil {
		t.Fatal(err)
	}

	pingRequest := httptest.NewRequest(http.MethodGet, "/ping?raw-query-sentinel", nil)
	pingRequest.Header.Set("X-Header-Sentinel", "header-sentinel")
	pingRequest.Header.Set("X-Correlation-ID", "correlation-sentinel")
	gateway.router.ServeHTTP(httptest.NewRecorder(), pingRequest)
	gateway.router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/payments/payment-id-sentinel", nil))

	expiry := time.Now().UTC().AddDate(0, 1, 0)
	paymentBody, err := json.Marshal(models.PaymentRequest{
		CardNumber: "00000000000001", ExpiryMonth: int(expiry.Month()), ExpiryYear: expiry.Year(), Currency: "GBP", Amount: 100, CVV: "987",
	})
	if err != nil {
		t.Fatal(err)
	}
	gateway.router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/payments", bytes.NewReader(paymentBody)))
	gateway.router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("method-sentinel", "/ping", nil))

	metricsResponse := httptest.NewRecorder()
	gateway.router.ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metricsResponse.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want %d", metricsResponse.Code, http.StatusOK)
	}
	metrics := metricsResponse.Body.String()

	for _, expected := range []string{
		`payment_gateway_http_requests_total{method="GET",route="/ping",status="200"} 1`,
		`payment_gateway_http_requests_total{method="GET",route="/api/payments/{id}",status="404"} 1`,
		`payment_gateway_http_requests_total{method="POST",route="/api/payments",status="503"} 1`,
		`payment_gateway_http_requests_total{method="OTHER",route="unmatched",status="405"} 1`,
	} {
		if !strings.Contains(metrics, expected) {
			t.Errorf("metrics missing %q:\n%s", expected, metrics)
		}
	}
	for _, sentinel := range []string{"payment-id-sentinel", "raw-query-sentinel", "00000000000001", "987", "header-sentinel", "correlation-sentinel", "method-sentinel"} {
		if strings.Contains(metrics, sentinel) {
			t.Errorf("metrics contain prohibited value %q:\n%s", sentinel, metrics)
		}
	}
	if strings.Contains(metrics, `route="/metrics"`) {
		t.Errorf("metrics endpoint must be excluded from HTTP metrics:\n%s", metrics)
	}
	assertMetricLabelsAreSafe(t, metrics)
}

func TestAPIMetricsTracksInFlightRequests(t *testing.T) {
	bankStarted := make(chan struct{})
	releaseBank := make(chan struct{})
	bankServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(bankStarted)
		<-releaseBank
		_, _ = w.Write([]byte(`{"authorized":true}`))
	}))
	defer bankServer.Close()
	t.Setenv("BANK_SIMULATOR_URL", bankServer.URL+"/payments")

	gateway, err := New()
	if err != nil {
		t.Fatal(err)
	}
	paymentRequest := paymentRequestJSON(t, "00000000000001")
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		gateway.router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(paymentRequest)))
	}()
	<-bankStarted

	metricsResponse := httptest.NewRecorder()
	gateway.router.ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(metricsResponse.Body.String(), "payment_gateway_http_in_flight_requests 1") {
		t.Fatalf("in-flight metric missing active request:\n%s", metricsResponse.Body.String())
	}
	close(releaseBank)
	<-requestDone
}

func assertMetricLabelsAreSafe(t *testing.T, metrics string) {
	t.Helper()
	allowedMethods := map[string]bool{"GET": true, "POST": true, "OTHER": true}
	allowedRoutes := map[string]bool{"/ping": true, "/api/payments": true, "/api/payments/{id}": true, "unmatched": true}
	allowedStatuses := map[string]bool{"200": true, "404": true, "405": true, "503": true}
	for _, line := range strings.Split(metrics, "\n") {
		if !strings.HasPrefix(line, "payment_gateway_http_requests_total{") && !strings.HasPrefix(line, "payment_gateway_http_request_duration_seconds_") {
			continue
		}
		start, end := strings.IndexByte(line, '{'), strings.LastIndexByte(line, '}')
		if start < 0 || end < start {
			continue
		}
		for _, label := range strings.Split(line[start+1:end], ",") {
			name, value, found := strings.Cut(label, "=")
			value = strings.Trim(value, "\"")
			if !found {
				t.Errorf("malformed metric label %q in %q", label, line)
				continue
			}
			switch name {
			case "method":
				if !allowedMethods[value] {
					t.Errorf("unsafe method label %q in %q", value, line)
				}
			case "route":
				if !allowedRoutes[value] {
					t.Errorf("unsafe route label %q in %q", value, line)
				}
			case "status":
				if !allowedStatuses[value] {
					t.Errorf("unexpected final status label %q in %q", value, line)
				}
			case "le":
			default:
				t.Errorf("unexpected metric label %q in %q", label, line)
			}
		}
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

func TestAPIRejectsBankRedirectsWithoutFollowingThem(t *testing.T) {
	for _, redirectStatus := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(redirectStatus), func(t *testing.T) {
			redirectTargetRequests := 0
			redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				redirectTargetRequests++
				_, _ = w.Write([]byte(`{"authorized":true}`))
			}))
			defer redirectTarget.Close()

			bank := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", redirectTarget.URL)
				w.WriteHeader(redirectStatus)
			}))
			defer bank.Close()
			t.Setenv("BANK_SIMULATOR_URL", bank.URL+"/payments")
			t.Setenv("BANK_SIMULATOR_TIMEOUT", "1s")
			gateway, err := New()
			if err != nil {
				t.Fatal(err)
			}

			recorder := httptest.NewRecorder()
			gateway.router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(paymentRequestJSON(t, "00000000000001"))))
			if recorder.Code != http.StatusServiceUnavailable {
				t.Errorf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
			}
			if redirectTargetRequests != 0 {
				t.Errorf("redirect target received %d requests, want 0", redirectTargetRequests)
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
