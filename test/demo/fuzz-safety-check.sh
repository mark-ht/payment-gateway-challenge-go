#!/bin/sh
set -eu

make -n fuzz | grep -F -x './test/demo/fuzz.sh' >/dev/null
script=test/demo/fuzz.sh
test -x "$script"
grep -F -- '--data-binary @-' "$script" >/dev/null
grep -F -- '/readyz' "$script" >/dev/null
grep -F -- 'logs --tail 20 gateway' "$script" >/dev/null
grep -F -x 'submit odd 200 Authorized <<'"'"'PAYMENT'"'"'' "$script" >/dev/null
grep -F -x 'submit even 200 Declined <<'"'"'PAYMENT'"'"'' "$script" >/dev/null
grep -F -x 'submit zero 503 unavailable <<'"'"'PAYMENT'"'"'' "$script" >/dev/null
! grep -E '^[[:space:]]*set[[:space:]]+-[^#]*x' "$script"
! grep -E -- '--data[[:space:]]' "$script"

printf '%s\n' 'deterministic demo safety checks verified'
