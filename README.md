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

## Tests

Normal Go tests are deterministic and Docker-free:

```bash
go test ./...
go vet ./...
go test -race ./...
```

The optional composed E2E smoke check builds a non-root, static distroless gateway image, starts it with the simulator, and runs the tagged compiled E2E runner over the Compose network:

```bash
docker compose --profile e2e up --build --abort-on-container-exit --exit-code-from e2e
```

Always remove the E2E containers and network afterwards, including after a failed run:

```bash
docker compose --profile e2e down --remove-orphans
```

The E2E runner waits for the gateway's internal `http://gateway:8090/readyz` endpoint with a bounded retry. It does not publish the gateway or Mountebank admin ports to the host.
