# Payment Gateway Challenge

A Go 1.21 payment-gateway assessment. The API is documented by Swagger UI at `http://localhost:8090/swagger/index.html` when the gateway is run locally.

## Local development

Start only the simulator for a local gateway process:

```bash
docker compose up -d bank_simulator
go run .
```

The simulator payment port is loopback-only at `localhost:8080`; its Mountebank admin port is not published. Stop the local simulator when finished:

```bash
docker compose down
```

## Build metadata

At startup the gateway logs `APP_VERSION`, `APP_COMMIT`, and `APP_DATE`, and uses `APP_VERSION` for the Swagger version. Unset or empty values use the non-sensitive defaults `dev`, `none`, and `unknown` respectively. The gateway image accepts matching build arguments and makes them available at runtime:

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

The E2E runner waits for the gateway's internal `http://gateway:8090/readyz` endpoint with a bounded retry. It does not publish the gateway or Mountebank admin ports to the host.
