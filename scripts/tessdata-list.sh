#!/usr/bin/env bash
# Generates the list of Tesseract language data internal/ocr embeds: the
# .traineddata files of tesseract-ocr/tessdata_fast at a pinned commit,
# each with its size and SHA-256, which Polyfin checks a download against
# (see internal/ocr/download.go). Only the files at the top of the
# repository are listed, one per language code; the script models of its
# folder script are not languages a track is read in. Each file is
# downloaded once, about 360 MB in all, into a temporary folder removed
# at the end.
#
# The commit is pinned in internal/ocr/download.go too: change both
# together, then run this script again.
#
# Requirements: curl, jq, sha256sum or shasum. Usage: scripts/tessdata-list.sh
set -euo pipefail

commit=87416418657359cb625c412a48b6e1d6d41c29bd
repository=tesseract-ocr/tessdata_fast

for tool in curl jq; do
	command -v "$tool" >/dev/null || { echo "$tool is required" >&2; exit 1; }
done
if command -v sha256sum >/dev/null; then
	sha256() { sha256sum "$1" | cut -d' ' -f1; }
else
	sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
fi

out="$(cd "$(dirname "$0")/.." && pwd)/internal/ocr/tessdata.json"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

curl --silent --show-error --fail "https://api.github.com/repos/$repository/git/trees/$commit" |
	jq --raw-output '.tree[] | select(.type == "blob" and (.path | endswith(".traineddata"))) | .path' | sort >"$work/files"

: >"$work/list"
while read -r file; do
	curl --silent --show-error --fail --location --output "$work/data" "https://raw.githubusercontent.com/$repository/$commit/$file"
	jq --null-input --compact-output --arg language "${file%.traineddata}" --arg sum "$(sha256 "$work/data")" \
		--argjson size "$(wc -c <"$work/data" | tr -d ' ')" \
		'{Language: $language, Size: $size, SHA256: $sum}' >>"$work/list"
done <"$work/files"

# One JSON object per line keeps the generated file readable in diffs.
jq --slurp --raw-output '"[", (.[:-1][] | tojson + ","), (last | tojson), "]"' "$work/list" >"$out"
