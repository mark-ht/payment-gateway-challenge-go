package repository

import (
	"fmt"
	"testing"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/models"
)

func TestPaymentsRepositorySupportsConcurrentAccess(t *testing.T) {
	repository := NewPaymentsRepository()
	const count = 100
	done := make(chan struct{}, count)
	for i := 0; i < count; i++ {
		go func(i int) {
			payment := models.Payment{ID: fmt.Sprintf("payment-%d", i)}
			repository.Add(payment)
			if got, found := repository.Get(payment.ID); !found || got != payment {
				t.Errorf("Get(%q) = (%+v, %t)", payment.ID, got, found)
			}
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < count; i++ {
		<-done
	}
}

func TestPaymentsRepositoryStoresAndReturnsSafePayment(t *testing.T) {
	repository := NewPaymentsRepository()
	payment := models.Payment{
		ID:                 "payment-id",
		Status:             "Authorized",
		CardNumberLastFour: "0001",
		ExpiryMonth:        4,
		ExpiryYear:         2030,
		Currency:           "GBP",
		Amount:             100,
	}

	repository.Add(payment)
	got, found := repository.Get(payment.ID)
	if !found || got != payment {
		t.Fatalf("Get() = (%+v, %t), want (%+v, true)", got, found, payment)
	}
}
