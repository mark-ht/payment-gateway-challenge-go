package models

// PaymentRequest is the merchant-provided card payment request.
type PaymentRequest struct {
	CardNumber string `json:"card_number" binding:"required" minLength:"14" maxLength:"19" extensions:"x-pattern=^[0-9]+$"`
	// ExpiryMonth must be from 1 through 12 and, with ExpiryYear, be strictly after the current UTC month.
	ExpiryMonth int `json:"expiry_month" binding:"required" minimum:"1" maximum:"12"`
	ExpiryYear  int `json:"expiry_year" binding:"required"`
	// Currency uses canonical supported values; input is normalized to uppercase before validation.
	Currency string `json:"currency" binding:"required" enums:"GBP,USD,EUR"`
	Amount   int    `json:"amount" binding:"required" minimum:"1"`
	CVV      string `json:"cvv" binding:"required" minLength:"3" maxLength:"4" extensions:"x-pattern=^[0-9]+$"`
}

// Payment contains only data that is safe to retain and return.
type Payment struct {
	// ID is a canonical UUIDv7 payment identifier. Its timestamp prefix reveals an approximate creation time.
	ID                 string `json:"id" format:"uuid"`
	Status             string `json:"status" enums:"Authorized,Declined"`
	CardNumberLastFour string `json:"card_number_last_four"`
	ExpiryMonth        int    `json:"expiry_month"`
	ExpiryYear         int    `json:"expiry_year"`
	Currency           string `json:"currency" enums:"GBP,USD,EUR"`
	Amount             int    `json:"amount"`
}
