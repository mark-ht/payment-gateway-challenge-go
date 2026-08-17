//go:build e2e

package e2e

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const defaultGatewayURL = "http://gateway:8090"

//go:embed testdata/payment-fixtures.json
var paymentFixturesJSON []byte

type paymentFixture struct {
	CardNumber  string `json:"card_number"`
	ExpiryMonth int    `json:"expiry_month"`
	ExpiryYear  int    `json:"expiry_year"`
	Currency    string `json:"currency"`
	Amount      int    `json:"amount"`
	CVV         string `json:"cvv"`
}

type fixtures struct {
	Authorized  paymentFixture `json:"authorized"`
	Declined    paymentFixture `json:"declined"`
	Unavailable paymentFixture `json:"unavailable"`
}

type paymentResponse struct {
	ID                 string `json:"id"`
	Status             string `json:"status"`
	CardNumberLastFour string `json:"card_number_last_four"`
	ExpiryMonth        int    `json:"expiry_month"`
	ExpiryYear         int    `json:"expiry_year"`
	Currency           string `json:"currency"`
	Amount             int    `json:"amount"`
}

func TestGatewayScenarios(t *testing.T) {
	gatewayURL := gatewayURL()
	client := &http.Client{Timeout: 2 * time.Second}
	waitForReady(t, client, gatewayURL)

	var fixtures fixtures
	if err := json.Unmarshal(paymentFixturesJSON, &fixtures); err != nil {
		t.Fatal("load payment fixtures")
	}

	authorized := postPayment(t, client, gatewayURL, fixtures.Authorized, http.StatusOK)
	assertSafePayment(t, authorized, "Authorized", fixtures.Authorized)

	retrieved := getPayment(t, client, gatewayURL, authorized.ID, http.StatusOK)
	assertSafePayment(t, retrieved, "Authorized", fixtures.Authorized)
	if retrieved != authorized {
		t.Fatal("retrieved payment differs from completed payment")
	}

	declined := postPayment(t, client, gatewayURL, fixtures.Declined, http.StatusOK)
	assertSafePayment(t, declined, "Declined", fixtures.Declined)

	postRaw(t, client, gatewayURL, []byte(`{}`), http.StatusBadRequest, true)
	postPayment(t, client, gatewayURL, fixtures.Unavailable, http.StatusServiceUnavailable)
	getPayment(t, client, gatewayURL, "unknown-payment-id", http.StatusNotFound)
}

func gatewayURL() string {
	if value := os.Getenv("E2E_GATEWAY_URL"); value != "" {
		return strings.TrimRight(value, "/")
	}
	return defaultGatewayURL
}

func waitForReady(t *testing.T, client *http.Client, gatewayURL string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(gatewayURL + "/readyz")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("gateway did not become ready before timeout")
}

func postPayment(t *testing.T, client *http.Client, gatewayURL string, fixture paymentFixture, wantStatus int) paymentResponse {
	t.Helper()
	body, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal("encode payment fixture")
	}
	return postRaw(t, client, gatewayURL, body, wantStatus, false)
}

func postRaw(t *testing.T, client *http.Client, gatewayURL string, body []byte, wantStatus int, wantRejected bool) paymentResponse {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, gatewayURL+"/api/payments", bytes.NewReader(body))
	if err != nil {
		t.Fatal("create payment request")
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("send payment request")
	}
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		t.Fatalf("unexpected payment status code: got %d want %d", response.StatusCode, wantStatus)
	}
	if wantRejected {
		var rejected struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(io.LimitReader(response.Body, 1024)).Decode(&rejected); err != nil {
			t.Fatal("decode rejected response")
		}
		if rejected.Status != "Rejected" {
			t.Fatal("invalid payment did not return rejected status")
		}
		return paymentResponse{}
	}
	if wantStatus != http.StatusOK {
		return paymentResponse{}
	}
	return decodeSafePayment(t, response.Body)
}

func getPayment(t *testing.T, client *http.Client, gatewayURL, id string, wantStatus int) paymentResponse {
	t.Helper()
	response, err := client.Get(gatewayURL + "/api/payments/" + id)
	if err != nil {
		t.Fatal("retrieve payment")
	}
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		t.Fatalf("unexpected retrieval status code: got %d want %d", response.StatusCode, wantStatus)
	}
	if wantStatus != http.StatusOK {
		return paymentResponse{}
	}
	return decodeSafePayment(t, response.Body)
}

func decodeSafePayment(t *testing.T, body io.Reader) paymentResponse {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.NewDecoder(io.LimitReader(body, 4096)).Decode(&fields); err != nil {
		t.Fatal("decode payment response")
	}
	allowed := map[string]struct{}{
		"id": {}, "status": {}, "card_number_last_four": {}, "expiry_month": {},
		"expiry_year": {}, "currency": {}, "amount": {},
	}
	if len(fields) != len(allowed) {
		t.Fatal("payment response has unexpected fields")
	}
	for field := range fields {
		if _, ok := allowed[field]; !ok {
			t.Fatal("payment response has an unsafe field")
		}
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		t.Fatal("encode safe payment response")
	}
	var payment paymentResponse
	if err := json.Unmarshal(encoded, &payment); err != nil {
		t.Fatal("decode safe payment fields")
	}
	return payment
}

func assertSafePayment(t *testing.T, payment paymentResponse, wantStatus string, fixture paymentFixture) {
	t.Helper()
	if payment.ID == "" {
		t.Fatal("payment response has no ID")
	}
	if payment.Status != wantStatus ||
		payment.CardNumberLastFour != fixture.CardNumber[len(fixture.CardNumber)-4:] ||
		payment.ExpiryMonth != fixture.ExpiryMonth ||
		payment.ExpiryYear != fixture.ExpiryYear ||
		payment.Currency != fixture.Currency ||
		payment.Amount != fixture.Amount {
		t.Fatal("payment response does not match safe expected fields")
	}
}
