#!/bin/sh
set -eu

make -n dev-up | grep -F -x 'docker compose up -d --build gateway' >/dev/null
make -n dev-down | grep -F -x 'docker compose down --remove-orphans' >/dev/null
grep -F -x '      - "127.0.0.1:8090:8090"' docker-compose.yml >/dev/null

printf '%s\n' 'local Compose Make targets verified'
