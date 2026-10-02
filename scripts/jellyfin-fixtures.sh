#!/usr/bin/env bash
# Records the JSON shapes a real Jellyfin 12.1 server returns, as fixtures for
# internal/jellyfin tests. Values are replaced by their type ("" for strings,
# 0 for numbers, false for booleans; arrays keep one element), so fixtures
# hold no identifier, date or token from the disposable server.
#
# The server gets a tiny generated media tree (three movies, one series with
# two seasons) and a BoxSet, so browsing responses can be recorded too. It
# fetches metadata from TMDB, which needs network access.
#
# Requirements: Docker, curl, jq, ffmpeg. Usage: scripts/jellyfin-fixtures.sh
set -euo pipefail

for tool in docker curl jq ffmpeg; do
	command -v "$tool" >/dev/null || { echo "$tool is required" >&2; exit 1; }
done

image='jellyfin/jellyfin:12.1@sha256:78d3ea1207d1322471fcac39a614f004f2ccf7e878f95ab2977d752f07e4dd7e'
out="$(cd "$(dirname "$0")/.." && pwd)/internal/jellyfin/testdata/jellyfin-12.1"
container="polyfin-jellyfin-fixtures-$$"
media=$(mktemp -d)

cleanup() {
	docker stop "$container" >/dev/null 2>&1 || true
	rm -rf "$media"
}
trap cleanup EXIT

# Three-second test pattern and tone, H.264/AAC. Matroska carries no
# per-stream bitrate; Jellyfin derives the video one from the container's
# minus an assumed audio bitrate, so the video needs a realistic bitrate.
video() {
	mkdir -p "$(dirname "$1")"
	ffmpeg -nostdin -loglevel error -y \
		-f lavfi -i 'testsrc=size=640x360:rate=24:duration=3' \
		-f lavfi -i 'sine=frequency=440:duration=3' \
		-c:v libx264 -b:v 1M -pix_fmt yuv420p -c:a aac -shortest "$1"
}
for movie in 'Big Buck Bunny (2008)' 'Sintel (2010)' 'Tears of Steel (2012)'; do
	video "$media/movies/$movie/$movie.mkv"
done
series='Pioneer One (2010)'
video "$media/shows/$series/Season 01/Pioneer One S01E01.mkv"
video "$media/shows/$series/Season 01/Pioneer One S01E02.mkv"
video "$media/shows/$series/Season 02/Pioneer One S02E01.mkv"

docker run --detach --rm --name "$container" --publish 127.0.0.1::8096 \
	--volume "$media:/media:ro" "$image" >/dev/null
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
user=$(jq --raw-output .User.Id <<<"$authentication")

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

# Browsing: libraries, then a BoxSet once the movies are indexed.
get() { curl --silent --show-error --fail --header "Authorization: $signed" "$base$1"; }
# Retries a command once a second until it succeeds, for at most five minutes.
await() {
	local what=$1 last
	shift
	for _ in $(seq 1 300); do
		last=$("$@" 2>&1) && return 0
		sleep 1
	done
	echo "Timed out waiting for $what; last check: ${last//$'\n'/ }" >&2
	return 1
}
# Succeeds when a query matches the expected number of items.
total() {
	get "$1&limit=0" | jq --exit-status --argjson count "$2" '.TotalRecordCount | ., . == $count'
}
indexed() { total "/Items?userId=$user&recursive=true&includeItemTypes=$1" "$2"; }
# No library is being refreshed and no library scan is running.
idle() {
	get /Library/VirtualFolders | jq --exit-status 'length > 0 and all(.[]; .RefreshStatus == "Idle")' &&
		get /ScheduledTasks | jq --exit-status 'all(.[] | select(.Key == "RefreshLibrary"); .State == "Idle")'
}
# A scan requested while another one runs can be dropped, so ask again (at
# most every ten seconds) whenever the server is idle without having indexed
# everything.
scanned() {
	indexed Movie 3 && indexed Episode 3 && return 0
	idle && post "$base/Library/Refresh" --header "Authorization: $signed" && sleep 10
	return 1
}
children() { total "/Users/$user/Items?ParentId=$1" "$2"; }
view() { get "/UserViews?userId=$user" | jq --exit-status --raw-output --arg type "$1" '.Items[] | select(.CollectionType == $type) | .Id'; }

post "$base/Library/VirtualFolders?name=Movies&collectionType=movies&paths=%2Fmedia%2Fmovies&refreshLibrary=true" \
	--header "Authorization: $signed" --data '{"LibraryOptions":{}}'
post "$base/Library/VirtualFolders?name=Shows&collectionType=tvshows&paths=%2Fmedia%2Fshows&refreshLibrary=true" \
	--header "Authorization: $signed" --data '{"LibraryOptions":{}}'
await 'three movies and three episodes' scanned
await 'the library scan' idle

movie_ids=$(get "/Items?userId=$user&recursive=true&includeItemTypes=Movie" |
	jq --raw-output '[.Items[].Id] | join(",")')
boxset=$(post "$base/Collections?name=Blender%20Open%20Movies&ids=$movie_ids" --header "Authorization: $signed" |
	jq --raw-output .Id)
await 'the Collections view' view boxsets
await 'the library scan' idle
# Creating the first collection starts a scan that can drop the collection's
# movies; adding them again once it is over is harmless.
post "$base/Collections/$boxset/Items?ids=$movie_ids" --header "Authorization: $signed"
await 'the BoxSet movies' children "$boxset" 3
await 'the library scan' idle

movies=$(view movies)
shows=$(view tvshows)
collections=$(view boxsets)
movie=$(get "/Items?userId=$user&recursive=true&includeItemTypes=Movie&fields=Path" |
	jq --exit-status --raw-output '.Items[] | select(.Path | endswith("/Big Buck Bunny (2008).mkv")) | .Id')
series=$(get "/Items?userId=$user&recursive=true&includeItemTypes=Series" | jq --exit-status --raw-output '.Items[0].Id')
season=$(get "/Shows/$series/Seasons?userId=$user" | jq --exit-status --raw-output '.Items[] | select(.IndexNumber == 1) | .Id')
episode=$(get "/Shows/$series/Episodes?seasonId=$season&userId=$user" |
	jq --exit-status --raw-output '.Items[] | select(.IndexNumber == 1) | .Id')

library="/Items?userId=$user&startIndex=0&limit=100&recursive=true&sortOrder=Ascending&fields=MediaSourceCount&fields=PrimaryImageAspectRatio&sortBy=SortName&imageTypeLimit=1&enableImageTypes=Primary&enableImageTypes=Backdrop"
counts='Fields=ItemCounts,PrimaryImageAspectRatio,CanDelete,MediaSourceCount'
get "/UserViews?userId=$user" | save views
get "$library&parentId=$movies&includeItemTypes=Movie" | save library-movies
get "$library&parentId=$shows&includeItemTypes=Series" | save library-series
get "/Items/Latest?userId=$user&parentId=$movies&fields=PrimaryImageAspectRatio&fields=Path&imageTypeLimit=1&enableImageTypes=Primary&enableImageTypes=Backdrop&enableImageTypes=Thumb&limit=16" | save latest
get "/Users/$user/Items/$movie" | save movie
get "/Users/$user/Items/$series" | save series
get "/Users/$user/Items/$season" | save season
get "/Users/$user/Items/$episode" | save episode
get "/Shows/$series/Seasons?userId=$user&$counts" | save seasons
get "/Shows/$series/Episodes?seasonId=$season&userId=$user&$counts,Overview" | save episodes
get "/Users/$user/Items?ParentId=$collections&$counts" | save boxsets
get "/Users/$user/Items/$boxset" | save boxset
get "/Users/$user/Items?ParentId=$boxset&$counts" | save boxset-children
get "/Items/$episode/Ancestors" | save ancestors
get "/Items/Filters?userId=$user&parentId=$movies&includeItemTypes=Movie" | save filters
get "/Items/Filters2?userId=$user&parentId=$movies&includeItemTypes=Movie" | save filters2
get "/Items?userId=$user&limit=100&recursive=true&searchTerm=sintel&fields=PrimaryImageAspectRatio&fields=CanDelete&fields=MediaSourceCount&includeItemTypes=Movie&includeItemTypes=Series&imageTypeLimit=1&enableTotalRecordCount=false" | save search
get "/DisplayPreferences/usersettings?userId=$user&client=emby" | save display-preferences
get "/UserItems/Resume?userId=$user&limit=12&fields=PrimaryImageAspectRatio&mediaTypes=Video&imageTypeLimit=1&enableImageTypes=Primary&enableImageTypes=Backdrop&enableImageTypes=Thumb&enableTotalRecordCount=false" | save resume
get /System/Endpoint | save system-endpoint

echo "Fixtures written to $out"
