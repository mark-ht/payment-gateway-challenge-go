#!/bin/sh
set -eu

umask 077
tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/payment-gateway-demo.XXXXXX")"
cleanup() {
	rm -rf "$tmpdir"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

gateway_url=http://127.0.0.1:8090
response_file="$tmpdir/response"

attempt=0
while [ "$attempt" -lt 30 ]; do
	if curl --silent --connect-timeout 1 --max-time 2 --output "$tmpdir/readyz" --write-out '%{http_code}' "$gateway_url/readyz" 2>/dev/null | grep -qx '200'; then
		break
	fi
	attempt=$((attempt + 1))
	sleep 1
done
if [ "$attempt" -eq 30 ]; then
	printf '%s\n' 'gateway did not become ready'
	exit 1
fi

submit() {
	scenario=$1
	expected_code=$2
	expected_status=$3
	status_code="$(curl --silent --show-error --connect-timeout 1 --max-time 5 --output "$response_file" --write-out '%{http_code}' --request POST --header 'Content-Type: application/json' --data-binary @- "$gateway_url/api/payments")"
	if [ "$status_code" != "$expected_code" ]; then
		printf '%s\n' "scenario=$scenario status=$status_code expected=$expected_code"
		exit 1
	fi
	if [ "$expected_status" != unavailable ] && ! grep -F -q "\"status\":\"$expected_status\"" "$response_file"; then
		printf '%s\n' "scenario=$scenario status=$status_code response_status=unexpected"
		exit 1
	fi
	printf '%s\n' "scenario=$scenario status=$status_code response_status=$expected_status"
}

submit odd 200 Authorized <<'PAYMENT'
{"card_number":"2222405343248871","expiry_month":12,"expiry_year":2035,"currency":"GBP","amount":100,"cvv":"123"}
PAYMENT
submit even 200 Declined <<'PAYMENT'
{"card_number":"2222405343248872","expiry_month":12,"expiry_year":2035,"currency":"GBP","amount":100,"cvv":"123"}
PAYMENT
submit zero 503 unavailable <<'PAYMENT'
{"card_number":"2222405343248870","expiry_month":12,"expiry_year":2035,"currency":"GBP","amount":100,"cvv":"123"}
PAYMENT

printf '%s\n' 'gateway log snapshot (last 20 lines):'
docker compose logs --tail 20 gateway
