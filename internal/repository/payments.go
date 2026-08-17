package repository

import (
	"sync"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/models"
)

// PaymentsRepository stores safe, completed payment records in memory.
type PaymentsRepository struct {
	mu       sync.RWMutex
	payments map[string]models.Payment
}

func NewPaymentsRepository() *PaymentsRepository {
	return &PaymentsRepository{payments: make(map[string]models.Payment)}
}

func (r *PaymentsRepository) Get(id string) (models.Payment, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	payment, found := r.payments[id]
	return payment, found
}

func (r *PaymentsRepository) Add(payment models.Payment) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.payments[payment.ID] = payment
}
