APP_VERSION ?= dev
APP_COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || printf '%s' none)
APP_DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || printf '%s' unknown)

export APP_VERSION APP_COMMIT APP_DATE

.PHONY: help test e2e metadata-check

help:
	@printf '%s\n' 'Targets:' '  test            Run Go tests with aggregate statement coverage.' '  e2e             Run the Compose end-to-end smoke check with cleanup.' '  metadata-check  Verify explicit build metadata overrides in the gateway image.'

test:
	@set -e; \
	coverage_file=$$(mktemp "$${TMPDIR:-/tmp}/payment-gateway-coverage.XXXXXX"); \
	trap 'rm -f "$$coverage_file"' EXIT HUP INT TERM; \
	go test ./... -coverprofile="$$coverage_file"; \
	go tool cover -func="$$coverage_file"

e2e:
	@cleanup() { docker compose --profile e2e down --remove-orphans; }; \
	trap cleanup EXIT; \
	trap 'exit 129' HUP; \
	trap 'exit 130' INT; \
	trap 'exit 143' TERM; \
	docker compose --profile e2e up --build --abort-on-container-exit --exit-code-from e2e

metadata-check:
	@./test/metadata/build-metadata.sh
