.PHONY: help test e2e

help:
	@printf '%s\n' 'Targets:' '  test  Run Go tests with aggregate statement coverage.' '  e2e   Run the Compose end-to-end smoke check with cleanup.'

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
