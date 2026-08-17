# Payment Gateway Design

## Purpose and Status

This document describes the current intended design for the Go payment-gateway assessment.

## Scope

The gateway has two merchant-facing capabilities:

1. Process a card payment through the acquiring-bank simulator.
2. Retrieve the safe details of a completed payment by ID.

It deliberately does not implement a real database, merchant authentication, refunds, capture, settlement, idempotency, or real acquiring-bank integration. The supplied in-memory repository is sufficient for this assessment.

## Accepted Design Considerations and Assumptions

- The API uses `POST /api/payments` to create a payment and `GET /api/payments/{id}` to retrieve a completed one.
- A valid bank decision is a completed payment: it is persisted and has status `Authorized` or `Declined`. Invalid merchant input is `Rejected` before a bank request; bank unavailability is not a payment outcome and is not persisted.
- `400` represents invalid/malformed merchant input, `404` an unknown payment ID, and `503` an unavailable or unusable bank response.
- Currency input is normalized to uppercase and restricted to `GBP`, `USD`, and `EUR`; the amount is a positive integer in minor units.
- A payment ID is an opaque collision-resistant value. Only safe payment data is retained in the in-memory repository.
- The implementation remains deliberately small: handler, validation, bank-client, and repository responsibilities are separated only to make the required behaviour testable and maintainable.

## Public HTTP API

### Create payment

```text
POST /api/payments
Content-Type: application/json
```

Request:

```json
{
  "card_number": "2222405343248877",
  "expiry_month": 4,
  "expiry_year": 2030,
  "currency": "GBP",
  "amount": 100,
  "cvv": "123"
}
```

For an authorized or declined request, return `200 OK`:

```json
{
  "id": "opaque-payment-id",
  "status": "Authorized",
  "card_number_last_four": "8877",
  "expiry_month": 4,
  "expiry_year": 2030,
  "currency": "GBP",
  "amount": 100
}
```

`status` is exactly `Authorized` or `Declined` for a completed payment. The simulator authorization code is not exposed.

Invalid or malformed requests return `400 Bad Request` without contacting the bank or storing a payment. Its JSON representation contains `status: "Rejected"`.

An unavailable, timed-out, malformed, or unexpected bank response returns `503 Service Unavailable`. It creates no payment record and must not expose internal error details or introduce a new payment-status value.

### Retrieve payment

```text
GET /api/payments/{id}
```

A known ID returns `200 OK` and the same safe completed-payment representation. An unknown ID returns `404 Not Found`; it is not a payment-status value and has no additional public error-code contract.

## Payment Flow

1. The handler decodes exactly one JSON object, rejects malformed/non-object/trailing input, and ignores unknown fields without requiring a request `Content-Type` header. It does not impose an unquantified body-size limit.
2. Validation normalizes the currency and validates every merchant field.
3. Only a valid request is translated to the simulator request shape and sent using the request context.
4. The bank client maps `authorized: true` to `Authorized` and `authorized: false` to `Declined`.
5. For either completed outcome, the gateway generates an opaque ID and persists a sanitized payment record.
6. The handler returns the sanitized record. A later GET retrieves that same record.

## Validation Rules

| Field | Current accepted rule |
|---|---|
| Card number | String containing only digits, 14–19 characters. |
| Expiry month | Integer from 1 through 12. |
| Expiry date | Strictly after the current UTC month; a card expiring this month is invalid. Production uses `time.Now().UTC()` through an injected clock. |
| Currency | Normalize to uppercase; accept only `GBP`, `USD`, or `EUR`. |
| Amount | Integer greater than zero, in minor currency units. |
| CVV | String containing only digits, 3–4 characters. |

## Internal Boundaries

Keep the implementation small and testable:

- **HTTP handler:** HTTP decoding/encoding, status selection, and request-context propagation.
- **Validation:** merchant-request validation and normalization before any bank call.
- **Bank client:** an interface-backed adapter that formats `expiry_date` as `MM/YYYY`, calls the simulator, applies a bounded HTTP timeout, and maps its response.
- **Repository:** concurrent-safe in-memory add/get operations over sanitized completed-payment records.

A separate service layer is optional, not required. Introduce it only if it makes these boundaries clearer.

## Simulator Mapping

The internal bank request is:

```json
{
  "card_number": "2222405343248877",
  "expiry_date": "04/2030",
  "currency": "GBP",
  "amount": 100,
  "cvv": "123"
}
```

| Simulator outcome | Gateway behaviour |
|---|---|
| `200` with `authorized: true` | Persist and return `Authorized`. |
| `200` with `authorized: false` | Persist and return `Declined`. |
| `503` | Return gateway `503`; do not persist. |
| Network error, timeout, malformed/unexpected response | Return gateway `503`; do not persist. |

### Bank Configuration

The bank client is configurable without source changes:

| Setting | Default | Format and behaviour |
|---|---|---|
| `BANK_SIMULATOR_URL` | `http://localhost:8080/payments` | Full simulator payment URL. |
| `BANK_SIMULATOR_TIMEOUT` | `5s` | Go duration, for example `3s`. An invalid value fails clearly during application startup rather than disabling the timeout. |

The configured timeout is applied to every bank request in addition to request-context cancellation.

## Data Safety

- Full PAN and CVV are strings only while validating and sending the simulator request.
- Never persist, return, or log the full PAN or CVV.
- Per-request access logs contain only an allow-listed standard HTTP method name (or the fixed `OTHER` marker), server-known matched route pattern, response status, elapsed duration, and a server-generated correlation ID. They never include raw method tokens, URIs/query strings, headers, bodies, card data, remote addresses, user agents, or client-supplied correlation IDs.
- Persist only payment ID, status, last four digits as a string, expiry month/year, normalized currency, and amount.
- Do not return or persist the simulator authorization code without a new accepted decision.

## Testing Approach

Each changed acceptance criterion follows red-green-refactor TDD:

1. Write the smallest focused test first.
2. Run it and observe its expected failure.
3. Implement only enough to make it pass.
4. Refactor only while the suite is green.

Use a test pyramid: deterministic unit tests are the required default suite; in-process integration tests cover the assembled API and real HTTP bank-client boundary with `httptest.Server`; Docker-backed simulator checks are optional E2E smoke verification. The default suite never requires Docker or fixed ports.

The test coverage includes validation/no-bank-call behaviour, authorized and declined processing, bank failure, safe persistence, GET retrieval, unknown IDs, malformed JSON, configuration, and concurrent repository access.

## Documentation and API Description

The starter project exposes Swagger UI. The implemented routes, request/response schemas, and status codes must be reflected in its Swagger annotations and generated specification, or any generation limitation must be documented before submission. This complements this design document; it does not replace the API contract.
