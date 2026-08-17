package handlers

import (
	"testing"
	"time"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/models"
)

func TestValidatePaymentRequest(t *testing.T) {
	now := time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC)
	valid := models.PaymentRequest{CardNumber: "00000000000001", ExpiryMonth: 5, ExpiryYear: 2026, Currency: "gbp", Amount: 1, CVV: "123"}

	tests := []struct {
		name  string
		input models.PaymentRequest
		valid bool
	}{
		{"valid and normalized", valid, true},
		{"empty card", models.PaymentRequest{ExpiryMonth: 5, ExpiryYear: 2026, Currency: "GBP", Amount: 1, CVV: "123"}, false},
		{"short card", models.PaymentRequest{CardNumber: "123", ExpiryMonth: 5, ExpiryYear: 2026, Currency: "GBP", Amount: 1, CVV: "123"}, false},
		{"long card", models.PaymentRequest{CardNumber: "12345678901234567890", ExpiryMonth: 5, ExpiryYear: 2026, Currency: "GBP", Amount: 1, CVV: "123"}, false},
		{"non-numeric card", models.PaymentRequest{CardNumber: "1234567890123x", ExpiryMonth: 5, ExpiryYear: 2026, Currency: "GBP", Amount: 1, CVV: "123"}, false},
		{"non-ASCII card digit", models.PaymentRequest{CardNumber: "1234567890123١", ExpiryMonth: 5, ExpiryYear: 2026, Currency: "GBP", Amount: 1, CVV: "123"}, false},
		{"month zero", models.PaymentRequest{CardNumber: valid.CardNumber, ExpiryMonth: 0, ExpiryYear: 2026, Currency: "GBP", Amount: 1, CVV: "123"}, false},
		{"month above range", models.PaymentRequest{CardNumber: valid.CardNumber, ExpiryMonth: 13, ExpiryYear: 2026, Currency: "GBP", Amount: 1, CVV: "123"}, false},
		{"past expiry", models.PaymentRequest{CardNumber: valid.CardNumber, ExpiryMonth: 3, ExpiryYear: 2026, Currency: "GBP", Amount: 1, CVV: "123"}, false},
		{"current month", models.PaymentRequest{CardNumber: valid.CardNumber, ExpiryMonth: 4, ExpiryYear: 2026, Currency: "GBP", Amount: 1, CVV: "123"}, false},
		{"missing currency", models.PaymentRequest{CardNumber: valid.CardNumber, ExpiryMonth: 5, ExpiryYear: 2026, Amount: 1, CVV: "123"}, false},
		{"non-three-character currency", models.PaymentRequest{CardNumber: valid.CardNumber, ExpiryMonth: 5, ExpiryYear: 2026, Currency: "US", Amount: 1, CVV: "123"}, false},
		{"unsupported currency", models.PaymentRequest{CardNumber: valid.CardNumber, ExpiryMonth: 5, ExpiryYear: 2026, Currency: "CAD", Amount: 1, CVV: "123"}, false},
		{"zero amount", models.PaymentRequest{CardNumber: valid.CardNumber, ExpiryMonth: 5, ExpiryYear: 2026, Currency: "GBP", Amount: 0, CVV: "123"}, false},
		{"negative amount", models.PaymentRequest{CardNumber: valid.CardNumber, ExpiryMonth: 5, ExpiryYear: 2026, Currency: "GBP", Amount: -1, CVV: "123"}, false},
		{"missing CVV", models.PaymentRequest{CardNumber: valid.CardNumber, ExpiryMonth: 5, ExpiryYear: 2026, Currency: "GBP", Amount: 1}, false},
		{"short CVV", models.PaymentRequest{CardNumber: valid.CardNumber, ExpiryMonth: 5, ExpiryYear: 2026, Currency: "GBP", Amount: 1, CVV: "12"}, false},
		{"long CVV", models.PaymentRequest{CardNumber: valid.CardNumber, ExpiryMonth: 5, ExpiryYear: 2026, Currency: "GBP", Amount: 1, CVV: "12345"}, false},
		{"invalid CVV", models.PaymentRequest{CardNumber: valid.CardNumber, ExpiryMonth: 5, ExpiryYear: 2026, Currency: "GBP", Amount: 1, CVV: "12x"}, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := validatePaymentRequest(test.input, func() time.Time { return now })
			if ok != test.valid {
				t.Fatalf("valid = %t, want %t", ok, test.valid)
			}
			if ok && got.Currency != "GBP" {
				t.Fatalf("currency = %q, want GBP", got.Currency)
			}
		})
	}
}
