#!/usr/bin/env bash
# Records the JSON shapes a real Jellyfin 12.1 server returns, as fixtures for
# internal/jellyfin tests. Values are replaced by their type ("" for strings,
# 0 for numbers, false for booleans; arrays keep one element), so fixtures
# hold no identifier, date or token from the disposable server.
#
# Requirements: Docker, curl, jq. Usage: scripts/jellyfin-fixtures.sh
set -euo pipefail

image='jellyfin/jellyfin:12.1@sha256:78d3ea1207d1322471fcac39a614f004f2ccf7e878f95ab2977d752f07e4dd7e'
out="$(cd "$(dirname "$0")/.." && pwd)/internal/jellyfin/testdata/jellyfin-12.1"
container="polyfin-jellyfin-fixtures-$$"

docker run --detach --rm --name "$container" --publish 127.0.0.1::8096 "$image" >/dev/null
trap 'docker stop "$container" >/dev/null 2>&1 || true' EXIT
base="http://$(docker port "$container" 8096/tcp | head -n 1)"
# The server answers 503 until it has finished starting.
for _ in $(seq 1 120); do
	curl --silent --fail "$base/Startup/Configuration" >/dev/null && break
	sleep 1
done

client='MediaBrowser Client="Polyfin fixtures", Device="Fixtures", DeviceId="polyfin-fixtures", Version="1.0.0"'
post() { curl --silent --show-error --fail --request POST --header 'Content-Type: application/json' "$@"; }

post "$base/Startup/Configuration" --data '{"UICulture":"en-US","MetadataCountryCode":"US","PreferredMetadataLanguage":"en"}'
# Reading the startup user creates it; it can then be renamed.
curl --silent --fail "$base/Startup/User" >/dev/null
post "$base/Startup/User" --data '{"Name":"fixtures","Password":"fixtures-password"}'
post "$base/Startup/Complete"

authentication=$(post "$base/Users/AuthenticateByName" --header "Authorization: $client" \
	--data '{"Username":"fixtures","Pw":"fixtures-password"}')
signed="$client, Token=\"$(jq --raw-output .AccessToken <<<"$authentication")\""

shape='def shape:
	if type == "object" then with_entries(.value |= shape)
	elif type == "array" then (if length > 0 then [.[0] | shape] else [] end)
	elif type == "string" then ""
	elif type == "number" then 0
	elif type == "boolean" then false
	else null end;
shape'
mkdir -p "$out"
save() { jq --sort-keys "$shape" >"$out/$1.json"; }

save authentication-result <<<"$authentication"
curl --silent --fail "$base/System/Info/Public" | save public-system-info
curl --silent --fail --header "Authorization: $signed" "$base/System/Info" | save system-info
curl --silent --fail --header "Authorization: $signed" "$base/Users/Me" | save user
post "$base/QuickConnect/Initiate" --header "Authorization: $client" | save quick-connect-result
curl --silent --fail "$base/Branding/Configuration" | save branding-configuration

echo "Fixtures written to $out"
