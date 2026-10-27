#!/usr/bin/env bash
# Opens the issue that reports what changed in Silo since the commits
# Polyfin follows, with the report of scripts/silofollow as its body, or
# updates the open one: the open issue labelled clients with this exact
# title. With DRY_RUN=1, it prints what it would do instead.
#
#   scripts/silofollow/issue.sh REPORT
#
# gh reads the repository from GH_REPO and the token from GH_TOKEN. RUN_URL,
# when set, is where the whole report can be read.
set -euo pipefail

report=$1
title="Silo changed since the commits Polyfin follows"
label=clients
# An issue's body holds 65536 characters: a longer report is cut at a
# line, below that with room for a note.
limit=60000

body=$(mktemp)
trap 'rm -f "$body"' EXIT
if [ "$(wc -c <"$report")" -gt "$limit" ]; then
	head -c "$limit" "$report" | sed '$d' >"$body"
	printf '\nThe report is longer than an issue holds: read it whole in %s.\n' "${RUN_URL:-the summary of the workflow run}" >>"$body"
else
	cp "$report" "$body"
fi

number=$(gh issue list --state open --label "$label" --limit 1000 --json number,title \
	--jq "map(select(.title == \"$title\")) | first | .number // empty")

if [ "${DRY_RUN:-}" = 1 ]; then
	if [ -n "$number" ]; then
		echo "Would update issue #$number, \"$title\", with this body:"
	else
		echo "Would open an issue \"$title\", labelled $label, with this body:"
	fi
	echo
	cat "$body"
elif [ -n "$number" ]; then
	gh issue edit "$number" --body-file "$body"
else
	gh issue create --title "$title" --label "$label" --body-file "$body"
fi
