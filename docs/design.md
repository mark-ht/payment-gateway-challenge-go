# Payment Gateway Design

## Purpose and Status

This document describes the current intended design for the Go payment-gateway assessment.

## Scope

The gateway has two merchant-facing capabilities:

1. Process a card payment through the acquiring-bank simulator.
2. Retrieve the safe details of a completed payment by ID.

It deliberately does not implement durable database storage, merchant authentication/authorization, rate limiting, caching, refunds, capture, settlement, idempotency, or real acquiring-bank integration. The supplied in-memory repository is sufficient for this assessment.

## Considered Production Concerns Outside Scope

### Caller authentication and authorization

The assessment trusts callers and has no merchant identity or authorization model. A production gateway must authenticate and authorize each caller before payment processing. The mechanism depends on the client boundary: JWT/OAuth-style credentials are appropriate for delegated or browser-facing clients, while mTLS is a strong option for managed service-to-service or merchant connections. Credential rotation, issuer/audience validation, merchant-scoped authorization, and audit controls would be part of that design.

### Rate limiting

No request throttling is implemented. A production gateway should enforce limits at an API gateway and/or application layer using authenticated merchant identity, with carefully chosen per-merchant and abuse-protection limits. A shared distributed limiter is needed for horizontally scaled instances; an in-memory per-process limiter would be inconsistent and is not added here.

### Caching and durable storage

The in-memory repository is intentionally ephemeral: payment retrieval is limited to one process lifetime and instance. Production requires durable, highly available storage with appropriate transactional/concurrency semantics, encryption and access controls, retention policies, backups, and observability. Any cache should contain only safe payment representations or short-lived lookup metadata—never PAN or CVV—and must have clear invalidation and consistency rules. Caching is not needed for the assessment's in-memory workflow.

## Accepted Design Considerations and Assumptions

- The API uses `POST /api/payments` to create a payment and `GET /api/payments/{id}` to retrieve a completed one.
- A valid bank decision is a completed payment: it is persisted and has status `Authorized` or `Declined`. Invalid merchant input is `Rejected` before a bank request; bank unavailability is not a payment outcome and is not persisted.
- `400` represents invalid/malformed merchant input, `404` an unknown payment ID, and `503` an unavailable or unusable bank response.
- Currency input is normalized to uppercase and restricted to `GBP`, `USD`, and `EUR`; the amount is a positive integer in minor units.
- A payment ID is an opaque collision-resistant value generated before bank authorization. Completed payments are atomically created by ID; an existing ID is never overwritten and causes a fresh-ID retry. Only safe payment data is retained in the in-memory repository.
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

## Operational Probes

`/ping` remains the existing compatibility endpoint and returns `200 {"message":"pong"}`. The unauthenticated probe endpoints perform local dependency checks only and never contact the bank:

| Endpoint | Success response | Semantics |
|---|---|---|
| `GET /healthz` | `200 {"status":"ok"}` | Local process and router are available. |
| `GET /livez` | `200 {"status":"ok"}` | The process is live and should not be restarted because of downstream bank availability. |
| `GET /readyz` | `200 {"status":"ready"}` | Local API construction has completed. It does not test the bank, which has no card-free health operation. |

Here, “local” describes the dependency check, not network exposure: this application does not bind the endpoints to loopback or authorize callers. Production ingress and network policy must prevent public routing and permit only the Kubernetes probe traffic that needs them. A Kubernetes deployment can use the probes without making bank availability a restart or traffic-admission condition:

```yaml
livenessProbe:
  httpGet: { path: /livez, port: 8090 }
readinessProbe:
  httpGet: { path: /readyz, port: 8090 }
```

## Metrics

`GET /metrics` exposes a dedicated, non-global Prometheus registry. The application serves it without authentication; production ingress and network policy must enforce private Prometheus-only scraping rather than public routing. The assessment does not add that deployment enforcement, authentication, or deployment manifests. It provides this simple baseline for future production observability:

- `payment_gateway_http_requests_total` counter and `payment_gateway_http_request_duration_seconds` histogram, labelled only with normalized allow-listed `method`, server-known matched `route` template (or the fixed `unmatched` value), and final `status`.
- `payment_gateway_http_in_flight_requests`, an unlabeled gauge.

The `/metrics` scrape endpoint is excluded from the HTTP request counter, duration histogram, and in-flight gauge so scrapes do not distort service telemetry. Metrics share only the access logger's selected safe operational fields: normalized method, server-known route, and response status; they never include raw paths/query strings, headers, bodies, payment IDs, PAN, CVV, or correlation IDs.

Dashboards define the service error rate as `5xx` responses divided by all observed responses. `4xx` responses remain separately observable merchant/request rejection traffic rather than service errors.

## Payment Flow

1. The handler decodes exactly one JSON object, rejects malformed/non-object/trailing input, and ignores unknown fields without requiring a request `Content-Type` header. Merchant request bodies are limited to 64 KiB; an excess body is rejected before a bank call or persistence.
2. Validation normalizes the currency and validates every merchant field.
3. The gateway generates an opaque ID before submitting the valid request for bank authorization.
4. Only a valid request is translated to the simulator request shape and sent using the request context.
5. The bank client maps `authorized: true` to `Authorized` and `authorized: false` to `Declined`.
6. For either completed outcome, the gateway atomically creates a sanitized payment record by ID. If that ID already exists, it generates a replacement ID and retries without overwriting the existing record.
7. The handler returns the sanitized record. A later GET retrieves that same record.

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
- **Bank client:** an interface-backed adapter that formats `expiry_date` as `MM/YYYY`, calls the simulator using a bounded HTTP timeout, refuses redirects, and maps its response.
- **Repository:** concurrent-safe in-memory create-if-absent/get operations over sanitized completed-payment records. Creation atomically rejects an existing ID rather than overwriting its completed payment.

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
| `3xx` redirect | Do not follow the redirect; return gateway `503` without forwarding the payment request; do not persist. |
| Network error, timeout, malformed/unexpected response (including duplicate `authorized` fields) | Return gateway `503`; do not persist. |

### Bank Configuration

The bank client is configurable without source changes:

| Setting | Default | Format and behaviour |
|---|---|---|
| `BANK_SIMULATOR_URL` | `http://localhost:8080/payments` | Full simulator payment URL. |
| `BANK_SIMULATOR_TIMEOUT` | `5s` | Go duration, for example `3s`. An invalid value fails clearly during application startup rather than disabling the timeout. |

The configured timeout is applied to every bank request in addition to request-context cancellation.

### Compose Deployment and E2E

The multi-stage `Dockerfile` builds the gateway with Go 1.21 as a static binary, then runs it on port 8090 in the non-root distroless runtime image. Its `APP_VERSION`, `APP_COMMIT`, and `APP_DATE` build arguments are passed to that image as runtime environment variables. `make e2e` exports overridable metadata defaults of `dev`, the full current Git SHA (or `none`), and the current UTC RFC3339 timestamp (or `unknown`), while Compose supplies `dev`, `none`, and `unknown` when invoked directly without those variables. At startup the gateway logs those deployment metadata values, defaults empty or unset values to `dev`, `none`, and `unknown`, and uses the resolved version in Swagger. Compose configures its bank URL as `http://bank_simulator:8080/payments` and gates startup on an internal Mountebank-admin health check; Mountebank admin port 2525 is never published. A profile-gated, separately compiled standard-library E2E runner connects through service DNS and bounded-polls `/readyz` before exercising the composed API. It covers declined-payment retrieval, malformed/trailing and oversized-request rejection, health/liveness probes, and safe bounded Prometheus exposition without raw request values. It is not part of `go test ./...`.

The current Compose configuration publishes the gateway only as `127.0.0.1:8090:8090` for the assessment-only local demonstration and retains loopback simulator port `127.0.0.1:8080` for the local `go run .` workflow; Mountebank admin port 2525 is never published. `make dev-up` builds and starts the gateway plus simulator detached, and `make dev-down` removes the Compose services and orphans. `make fuzz` is a deterministic synthetic-payment demo smoke workflow—not randomized fuzzing—that bounded-polls `/readyz`, checks authorized, declined, and unavailable responses for odd, non-zero even, and zero final digits respectively, then prints status summaries and a bounded safe gateway-log snapshot. It is assessment-only and must never be used with real payment data.

### Resource Bounds

Merchant request bodies are limited to 64 KiB. An excess body is rejected with `413 Payload Too Large` and `status: "Rejected"` before any bank call or persistence. Bank response bodies are likewise limited to 64 KiB; an excess response is unusable and follows the generic `503` path. These limits bound resource use while remaining far above the small payment and simulator JSON payloads.

### Production Transport and Origin Policy

The assessment keeps its local HTTP simulator default. In production, the configured bank endpoint must require HTTPS and match an approved-host allowlist before PAN/CVV is forwarded. CORS is a separate browser-origin control for the public gateway; it does not enforce the server-to-bank endpoint, HTTPS, or host allowlist. If browser clients are supported, configure CORS with explicit approved origins in addition to—not instead of—bank-boundary transport controls.

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
