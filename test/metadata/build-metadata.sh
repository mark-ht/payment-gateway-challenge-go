#!/bin/sh
set -eu

: "${APP_VERSION:?APP_VERSION must be set}"
: "${APP_COMMIT:?APP_COMMIT must be set}"
: "${APP_DATE:?APP_DATE must be set}"

project="payment-gateway-metadata-check"
config="$(docker compose -p "$project" config)"
for metadata in "APP_VERSION: $APP_VERSION" "APP_COMMIT: $APP_COMMIT"; do
	printf '%s\n' "$config" | grep -F -x "        $metadata" >/dev/null
done
if ! printf '%s\n' "$config" | grep -F -x "        APP_DATE: $APP_DATE" >/dev/null && \
	! printf '%s\n' "$config" | grep -F -x "        APP_DATE: \"$APP_DATE\"" >/dev/null; then
	exit 1
fi

image="$(docker compose -p "$project" config --images | grep -F -x "${project}-gateway")"
docker compose -p "$project" build gateway
image_env="$(docker image inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$image")"
for metadata in "APP_VERSION=$APP_VERSION" "APP_COMMIT=$APP_COMMIT" "APP_DATE=$APP_DATE"; do
	printf '%s\n' "$image_env" | grep -F -x "$metadata" >/dev/null
done

printf '%s\n' "gateway image metadata verified"
