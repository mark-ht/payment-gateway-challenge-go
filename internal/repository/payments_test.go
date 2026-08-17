package repository

import (
	"fmt"
	"sync"
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
			if !repository.Create(payment) {
				t.Errorf("Create(%q) = false, want true", payment.ID)
			}
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

func TestPaymentsRepositoryCreateDoesNotOverwriteExistingPayment(t *testing.T) {
	repository := NewPaymentsRepository()
	first := models.Payment{ID: "payment-id", Status: "Authorized", Amount: 100}
	second := models.Payment{ID: first.ID, Status: "Declined", Amount: 200}

	if !repository.Create(first) {
		t.Fatal("Create(first) = false, want true")
	}
	if repository.Create(second) {
		t.Fatal("Create(second) = true, want false for a duplicate ID")
	}
	if got, found := repository.Get(first.ID); !found || got != first {
		t.Fatalf("Get(%q) = (%+v, %t), want (%+v, true)", first.ID, got, found, first)
	}
}

func TestPaymentsRepositoryCreateIsAtomicForConcurrentCollisions(t *testing.T) {
	repository := NewPaymentsRepository()
	const attempts = 100
	start := make(chan struct{})
	created := make(chan models.Payment, attempts)
	var workers sync.WaitGroup

	for i := 0; i < attempts; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			payment := models.Payment{ID: "collision", Amount: i}
			<-start
			if repository.Create(payment) {
				created <- payment
			}
		}(i)
	}
	close(start)
	workers.Wait()
	close(created)

	var winner models.Payment
	for payment := range created {
		if winner.ID != "" {
			t.Fatalf("Create succeeded more than once: %+v and %+v", winner, payment)
		}
		winner = payment
	}
	if winner.ID == "" {
		t.Fatal("Create never succeeded")
	}
	if got, found := repository.Get(winner.ID); !found || got != winner {
		t.Fatalf("Get(%q) = (%+v, %t), want (%+v, true)", winner.ID, got, found, winner)
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

	if !repository.Create(payment) {
		t.Fatal("Create() = false, want true")
	}
	got, found := repository.Get(payment.ID)
	if !found || got != payment {
		t.Fatalf("Get() = (%+v, %t), want (%+v, true)", got, found, payment)
	}
}
