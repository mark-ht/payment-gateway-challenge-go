package api

import (
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		timeout string
		wantURL string
		want    time.Duration
		wantErr bool
	}{
		{"defaults", "", "", "http://localhost:8080/payments", 5 * time.Second, false},
		{"overrides", "http://bank.test/payments", "250ms", "http://bank.test/payments", 250 * time.Millisecond, false},
		{"invalid timeout", "", "bad", "", 0, true},
		{"non-positive timeout", "", "0s", "", 0, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("BANK_SIMULATOR_URL", test.url)
			t.Setenv("BANK_SIMULATOR_TIMEOUT", test.timeout)
			config, err := loadConfig()
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, want error: %t", err, test.wantErr)
			}
			if err == nil && (config.bankURL != test.wantURL || config.bankTimeout != test.want) {
				t.Fatalf("config = %+v", config)
			}
		})
	}
}
