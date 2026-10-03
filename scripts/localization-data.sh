#!/usr/bin/env bash
# Generates the language list internal/localization embeds, from the ISO
# 639-2 code list the Library of Congress, its registration authority,
# publishes as UTF-8 text (bibliographic code, terminology code, ISO 639-1
# code, English name, French name), rather than from any Jellyfin file. The
# "qaa-qtz" line is a range reserved for local use, not a language, and is
# left out. English names are kept as listed ("Spanish; Castilian"), and
# sorted, as apps show them. Countries need no generated file: they come
# from the Unicode CLDR data in golang.org/x/text.
#
# Requirements: curl, jq. Usage: scripts/localization-data.sh
set -euo pipefail

for tool in curl jq; do
	command -v "$tool" >/dev/null || { echo "$tool is required" >&2; exit 1; }
done

out="$(cd "$(dirname "$0")/.." && pwd)/internal/localization"
languages='https://www.loc.gov/standards/iso639-2/ISO-639-2_utf-8.txt'

# One JSON object per line keeps the generated files readable in diffs.
lines() { jq --raw-output '"[", (.[:-1][] | tojson + ","), (last | tojson), "]"'; }

# The file starts with a byte order mark and ends lines with CRLF.
curl --silent --show-error --fail "$languages" |
	jq --raw-input --null-input '
		[inputs | ltrimstr("\ufeff") | rtrimstr("\r") | select(length > 0) | split("|") | select(.[0] != "qaa-qtz")
			| {Name: .[3], Bibliographic: .[0], Terminology: .[1], Alpha2: .[2]}]
		| sort_by(.Name)' | lines >"$out/languages.json"
