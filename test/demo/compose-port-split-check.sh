#!/bin/sh
set -eu

if grep -F -x '      - "127.0.0.1:8090:8090"' docker-compose.yml >/dev/null; then
	printf '%s\n' 'gateway port 8090 must not be published by the base Compose file' >&2
	exit 1
fi

grep -F -x '      - "127.0.0.1:8090:8090"' docker-compose.dev.yml >/dev/null

printf '%s\n' 'Compose gateway port split verified'
