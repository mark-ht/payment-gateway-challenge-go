package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/models"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/repository"
	"github.com/go-chi/chi/v5"
)

// Authorizer submits a validated payment request to the acquiring bank.
type Authorizer interface {
	Authorize(context.Context, models.PaymentRequest) (bool, error)
}

type IDGenerator func() string
type Clock func() time.Time

type PaymentsHandler struct {
	storage    *repository.PaymentsRepository
	authorizer Authorizer
	clock      Clock
	newID      IDGenerator
}

func NewPaymentsHandler(storage *repository.PaymentsRepository, authorizer Authorizer, clock Clock, newID IDGenerator) *PaymentsHandler {
	return &PaymentsHandler{storage: storage, authorizer: authorizer, clock: clock, newID: newID}
}

// GetHandler retrieves a completed payment by ID.
//
//	@Summary	Retrieve a payment
//	@Description	A 404 response has no contractual error body.
//	@Produce	json
//	@Param		id	path		string	true	"Payment ID"
//	@Success	200	{object}	models.Payment
//	@Failure	404	"Payment not found"
//	@Router		/api/payments/{id} [get]
func (h *PaymentsHandler) GetHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		payment, found := h.storage.Get(chi.URLParam(r, "id"))
		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, payment)
	}
}

// PostHandler validates and submits a card payment. Content-Type is optional; unknown JSON properties are ignored.
//
//	@Summary	Process a payment
//	@Description	Content-Type is not required. Unknown JSON properties are ignored. A 503 response has no contractual error body.
//	@Accept		json
//	@Produce	json
//	@Param		payment	body		models.PaymentRequest	true	"Payment request"
//	@Success	200		{object}	models.Payment
//	@Failure	400		{object}	rejectedResponse
//	@Failure	503		"Bank unavailable"
//	@Router		/api/payments [post]
func (h *PaymentsHandler) PostHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request models.PaymentRequest
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&request); err != nil {
			writeRejected(w)
			return
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			writeRejected(w)
			return
		}
		request, valid := validatePaymentRequest(request, h.clock)
		if !valid {
			writeRejected(w)
			return
		}
		// Generate before authorization so every completed bank decision has a local ID.
		id := h.newID()
		authorized, err := h.authorizer.Authorize(r.Context(), request)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		status := "Declined"
		if authorized {
			status = "Authorized"
		}
		payment := models.Payment{
			ID:                 id,
			Status:             status,
			CardNumberLastFour: request.CardNumber[len(request.CardNumber)-4:],
			ExpiryMonth:        request.ExpiryMonth,
			ExpiryYear:         request.ExpiryYear,
			Currency:           request.Currency,
			Amount:             request.Amount,
		}
		for !h.storage.Create(payment) {
			// Retry a collision so an existing completed payment is never overwritten.
			payment.ID = h.newID()
		}
		writeJSON(w, http.StatusOK, payment)
	}
}

type rejectedResponse struct {
	Status string `json:"status"`
}

func writeRejected(w http.ResponseWriter) {
	writeJSON(w, http.StatusBadRequest, rejectedResponse{Status: "Rejected"})
}

func validatePaymentRequest(request models.PaymentRequest, clock Clock) (models.PaymentRequest, bool) {
	if !digits(request.CardNumber) || len(request.CardNumber) < 14 || len(request.CardNumber) > 19 ||
		request.ExpiryMonth < 1 || request.ExpiryMonth > 12 ||
		request.Amount <= 0 || !digits(request.CVV) || len(request.CVV) < 3 || len(request.CVV) > 4 {
		return models.PaymentRequest{}, false
	}

	request.Currency = strings.ToUpper(request.Currency)
	if len(request.Currency) != 3 || (request.Currency != "GBP" && request.Currency != "USD" && request.Currency != "EUR") {
		return models.PaymentRequest{}, false
	}

	now := clock().UTC()
	if request.ExpiryYear < now.Year() || (request.ExpiryYear == now.Year() && request.ExpiryMonth <= int(now.Month())) {
		return models.PaymentRequest{}, false
	}
	return request, true
}

func digits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
