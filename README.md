# Payment Gateway Challenge

A Go 1.21 payment-gateway assessment. The API is documented by Swagger UI at `http://localhost:8090/swagger/index.html` when the gateway is run locally.

## Local development

For the local Compose demonstration, `make dev-up` combines the base Compose file with the development-only `docker-compose.dev.yml` override, then builds and starts the gateway and simulator:

```bash
make dev-up
```

The development override publishes the gateway only on `127.0.0.1:8090` (including Swagger at `http://127.0.0.1:8090/swagger/index.html`); it is not publicly exposed. `make dev-up` requires local port 8090 to be free and does not stop an existing listener. The simulator payment port is also loopback-only at `127.0.0.1:8080`, and its Mountebank admin port is not published. Remove the demonstration services and any Compose orphans when finished:

```bash
make dev-down
```

`make fuzz` is a deterministic demo smoke workflow, **not randomized fuzzing**. It starts the local stack, bounded-polls readiness, and submits only assessment-only synthetic payments with odd, non-zero even, and zero final digits. It checks `200 Authorized`, `200 Declined`, and `503` respectively, prints only status summaries and a bounded safe gateway-log snapshot, and never prints request bodies, PANs, or CVVs:

```bash
make fuzz
make dev-down
```

Do not use this Compose demo or its synthetic fixture values with real payment data.

To run the gateway process directly instead, start only the simulator and then run Go:

```bash
docker compose up -d bank_simulator
go run .
```

Use `make dev-down` to clean up the simulator afterwards.

## Build metadata

At startup the gateway logs `APP_VERSION`, `APP_COMMIT`, and `APP_DATE`, and uses `APP_VERSION` for the Swagger version. Unset or empty values use the non-sensitive defaults `dev`, `none`, and `unknown` respectively. For `make e2e`, the exported defaults are `APP_VERSION=dev`, the full current Git SHA for `APP_COMMIT` (or `none` outside a Git checkout), and the current UTC RFC3339 timestamp for `APP_DATE` (or `unknown` if it cannot be generated). Explicit Make or CI values override those defaults and are passed through Compose as gateway Docker build arguments:

```bash
make e2e \
  APP_VERSION=v1.2.3 \
  APP_COMMIT=0123456789abcdef \
  APP_DATE=2026-04-02T00:00:00Z
```

Direct Compose builds use the same `dev`, `none`, and `unknown` defaults when these variables are omitted. The gateway image accepts matching build arguments and makes them available at runtime:

```bash
docker build \
  --build-arg APP_VERSION=v1.2.3 \
  --build-arg APP_COMMIT=abc123 \
  --build-arg APP_DATE=2026-04-02T00:00:00Z \
  -t payment-gateway .
```

## Tests

Normal Go tests are deterministic and Docker-free. Run them with aggregate statement coverage:

```bash
make test
```

For the additional submission checks, run:

```bash
go vet ./...
go test -race ./...
```

The optional composed E2E smoke check builds a non-root, static distroless gateway image, starts it with the simulator, and runs the tagged compiled E2E runner over the Compose network. It always removes the E2E containers and network afterwards, including after a failed run:

```bash
make e2e
```

Verify explicit metadata overrides reach the built gateway image without running the E2E suite:

```bash
make metadata-check \
  APP_VERSION=v1.2.3 \
  APP_COMMIT=0123456789abcdef \
  APP_DATE=2026-04-02T00:00:00Z
```

The E2E runner uses only the base Compose configuration and waits for the gateway's internal `http://gateway:8090/readyz` endpoint with a bounded retry. It covers authorized and declined payment retrieval, malformed/trailing and oversized-request rejection, unavailable and unknown-payment handling, exact health/liveness probe responses, and safe bounded Prometheus metrics. It does not publish the gateway or Mountebank admin ports to the host, so an unrelated local listener on port 8090 does not block `make e2e`.
