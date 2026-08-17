package api

import (
	"fmt"
	"os"
	"time"
)

const (
	defaultBankURL     = "http://localhost:8080/payments"
	defaultBankTimeout = 5 * time.Second
)

type config struct {
	bankURL     string
	bankTimeout time.Duration
}

func loadConfig() (config, error) {
	result := config{bankURL: os.Getenv("BANK_SIMULATOR_URL"), bankTimeout: defaultBankTimeout}
	if result.bankURL == "" {
		result.bankURL = defaultBankURL
	}
	if value := os.Getenv("BANK_SIMULATOR_TIMEOUT"); value != "" {
		timeout, err := time.ParseDuration(value)
		if err != nil || timeout <= 0 {
			return config{}, fmt.Errorf("invalid BANK_SIMULATOR_TIMEOUT")
		}
		result.bankTimeout = timeout
	}
	return result, nil
}
