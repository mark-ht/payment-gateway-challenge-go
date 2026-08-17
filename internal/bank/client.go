package bank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/models"
)

const maxBankResponseBytes = 64 * 1024

type Client struct {
	url        string
	httpClient *http.Client
}

func NewClient(url string, timeout time.Duration) *Client {
	return &Client{
		url: url,
		httpClient: &http.Client{
			Timeout: timeout,
			// Refuse redirects so a 307 or 308 cannot replay payment data to another host.
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *Client) Authorize(ctx context.Context, payment models.PaymentRequest) (bool, error) {
	body, err := json.Marshal(struct {
		CardNumber string `json:"card_number"`
		ExpiryDate string `json:"expiry_date"`
		Currency   string `json:"currency"`
		Amount     int    `json:"amount"`
		CVV        string `json:"cvv"`
	}{
		CardNumber: payment.CardNumber,
		ExpiryDate: fmt.Sprintf("%02d/%04d", payment.ExpiryMonth, payment.ExpiryYear),
		Currency:   payment.Currency,
		Amount:     payment.Amount,
		CVV:        payment.CVV,
	})
	if err != nil {
		return false, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected bank status: %d", response.StatusCode)
	}

	// Read only one byte beyond the limit so a bank response cannot exhaust gateway resources.
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxBankResponseBytes+1))
	if err != nil {
		return false, err
	}
	if len(responseBody) > maxBankResponseBytes {
		return false, fmt.Errorf("bank response exceeds size limit")
	}

	var result struct {
		Authorized *bool `json:"authorized"`
	}
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	if err := decoder.Decode(&result); err != nil || result.Authorized == nil {
		return false, fmt.Errorf("invalid bank response")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return false, fmt.Errorf("invalid bank response")
	}
	return *result.Authorized, nil
}
