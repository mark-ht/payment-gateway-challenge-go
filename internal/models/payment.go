package models

// PaymentRequest is the merchant-provided card payment request.
type PaymentRequest struct {
	CardNumber  string `json:"card_number"`
	ExpiryMonth int    `json:"expiry_month"`
	ExpiryYear  int    `json:"expiry_year"`
	Currency    string `json:"currency"`
	Amount      int    `json:"amount"`
	CVV         string `json:"cvv"`
}

// Payment contains only data that is safe to retain and return.
type Payment struct {
	ID                 string `json:"id"`
	Status             string `json:"status"`
	CardNumberLastFour string `json:"card_number_last_four"`
	ExpiryMonth        int    `json:"expiry_month"`
	ExpiryYear         int    `json:"expiry_year"`
	Currency           string `json:"currency"`
	Amount             int    `json:"amount"`
}
