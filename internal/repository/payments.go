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

// Create stores payment only when its ID is not already present.
func (r *PaymentsRepository) Create(payment models.Payment) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Refuse collisions atomically so a retry cannot overwrite a completed payment.
	if _, exists := r.payments[payment.ID]; exists {
		return false
	}
	r.payments[payment.ID] = payment
	return true
}

// Add is retained for compatibility; new callers should use Create to detect collisions.
func (r *PaymentsRepository) Add(payment models.Payment) {
	r.Create(payment)
}
