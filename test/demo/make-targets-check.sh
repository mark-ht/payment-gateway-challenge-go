#!/bin/sh
set -eu

make -n dev-up | grep -F -x 'docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build gateway' >/dev/null
make -n dev-down | grep -F -x 'docker compose -f docker-compose.yml -f docker-compose.dev.yml down --remove-orphans' >/dev/null
make -n e2e | grep -F 'docker compose -f docker-compose.yml --profile e2e up --build --abort-on-container-exit --exit-code-from e2e' >/dev/null
./test/demo/compose-port-split-check.sh

printf '%s\n' 'local Compose Make targets verified'
