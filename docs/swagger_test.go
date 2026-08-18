package docs

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type swaggerDocument struct {
	Definitions map[string]swaggerDefinition `json:"definitions"`
}

type swaggerDefinition struct {
	Properties map[string]swaggerProperty `json:"properties"`
	Required   []string                   `json:"required"`
}

type swaggerProperty struct {
	Description string   `json:"description"`
	Enum        []string `json:"enum"`
	Format      string   `json:"format"`
	Maximum     *float64 `json:"maximum"`
	MaxLength   *int     `json:"maxLength"`
	Minimum     *float64 `json:"minimum"`
	MinLength   *int     `json:"minLength"`
	Type        string   `json:"type"`
	XPattern    string   `json:"x-pattern"`
}

func TestPaymentSchemasRetainSourceContractConstraints(t *testing.T) {
	var document swaggerDocument
	if err := json.Unmarshal([]byte(SwaggerInfo.ReadDoc()), &document); err != nil {
		t.Fatal(err)
	}

	request := document.Definitions["models.PaymentRequest"]
	assertStringsEqual(t, "required request properties", request.Required, []string{"amount", "card_number", "currency", "cvv", "expiry_month", "expiry_year"})
	assertProperty(t, request.Properties["card_number"], swaggerProperty{Type: "string", MinLength: intPointer(14), MaxLength: intPointer(19), XPattern: "^[0-9]+$"})
	assertProperty(t, request.Properties["cvv"], swaggerProperty{Type: "string", MinLength: intPointer(3), MaxLength: intPointer(4), XPattern: "^[0-9]+$"})
	assertProperty(t, request.Properties["expiry_month"], swaggerProperty{Type: "integer", Minimum: floatPointer(1), Maximum: floatPointer(12)})
	if !strings.Contains(request.Properties["expiry_month"].Description, "strictly after the current UTC month") {
		t.Fatalf("expiry_month description = %q, want future-expiry constraint", request.Properties["expiry_month"].Description)
	}
	assertStringsEqual(t, "supported currencies", request.Properties["currency"].Enum, []string{"GBP", "USD", "EUR"})
	assertProperty(t, request.Properties["amount"], swaggerProperty{Type: "integer", Minimum: floatPointer(1)})

	paymentID := document.Definitions["models.Payment"].Properties["id"]
	if paymentID.Type != "string" || paymentID.Format != "uuid" || !strings.Contains(paymentID.Description, "UUIDv7") {
		t.Fatalf("payment ID schema = %+v, want UUIDv7 string in uuid format", paymentID)
	}
	assertStringsEqual(t, "completed payment statuses", document.Definitions["models.Payment"].Properties["status"].Enum, []string{"Authorized", "Declined"})
	assertStringsEqual(t, "rejected payment statuses", document.Definitions["handlers.rejectedResponse"].Properties["status"].Enum, []string{"Rejected"})
}

func assertProperty(t *testing.T, got, want swaggerProperty) {
	t.Helper()
	if got.Type != want.Type || !reflect.DeepEqual(got.Minimum, want.Minimum) || !reflect.DeepEqual(got.Maximum, want.Maximum) || !reflect.DeepEqual(got.MinLength, want.MinLength) || !reflect.DeepEqual(got.MaxLength, want.MaxLength) || got.XPattern != want.XPattern {
		t.Fatalf("property = %+v, want constraints %+v", got, want)
	}
}

func assertStringsEqual(t *testing.T, name string, got, want []string) {
	t.Helper()
	got = append([]string(nil), got...)
	want = append([]string(nil), want...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func intPointer(value int) *int {
	return &value
}

func floatPointer(value float64) *float64 {
	return &value
}
