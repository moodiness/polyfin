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
# Playback is recorded last, on a separate library of short clips covering
# the codecs Polyfin decides on, plus a .strm movie whose URL points at a
# static file server container (Polyfin's remote sources). Besides shape
# fixtures, it writes unscrubbed decision data to playback/: the upstream
# ffprobe output of every clip (probes/) and Jellyfin's PlaybackInfo answer
# for every clip and client DeviceProfile (decisions.json). The profiles in
# playback/profiles/ are inputs: jellyfin-web-chrome was captured from the
# real web client, the other clients' were reconstructed from their sources.
#
# Requirements: Docker, curl, jq, ffmpeg, ffprobe. Usage: scripts/jellyfin-fixtures.sh
set -euo pipefail

for tool in docker curl jq ffmpeg ffprobe; do
	command -v "$tool" >/dev/null || { echo "$tool is required" >&2; exit 1; }
done

image='jellyfin/jellyfin:12.1@sha256:78d3ea1207d1322471fcac39a614f004f2ccf7e878f95ab2977d752f07e4dd7e'
out="$(cd "$(dirname "$0")/.." && pwd)/internal/jellyfin/testdata/jellyfin-12.1"
container="polyfin-jellyfin-fixtures-$$"
# The .strm movie's file server, reachable from Jellyfin by name on a network
# of their own.
files="$container-files"
network="$container"
media=$(mktemp -d)
remote=$(mktemp -d)

cleanup() {
	docker stop "$container" "$files" >/dev/null 2>&1 || true
	docker network rm "$network" >/dev/null 2>&1 || true
	rm -rf "$media" "$remote"
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

# Ten-second playback clips, one folder each, named after their content:
# probes and decisions refer to them by these names.
playback="$media/playback"
pattern='testsrc2=size=640x360:rate=24:duration=10'
tone() { printf 'sine=frequency=%s:duration=10' "$1"; }
encode() { SVT_LOG=1 ffmpeg -nostdin -loglevel error -y "$@"; }
clip() { mkdir -p "$playback/$1" && printf '%s/%s/%s.%s' "$playback" "$1" "$1" "$2"; }
printf '1\n00:00:01,000 --> 00:00:04,000\nEmbedded English cue.\n\n2\n00:00:05,000 --> 00:00:08,000\n<i>Second</i> embedded cue.\n' >"$media/en.srt"
encode -f lavfi -i "$pattern" -f lavfi -i "$(tone 440)" \
	-c:v libx264 -b:v 1M -pix_fmt yuv420p -c:a aac -b:a 128k -movflags +faststart \
	-metadata:s:a:0 language=eng "$(clip h264-aac-mp4 mp4)"
# Two audio tracks (the second one is secondary), an embedded subtitle, a
# forced one, and an external French sidecar.
encode -f lavfi -i "$pattern" -f lavfi -i "$(tone 330)" -f lavfi -i "$(tone 550)" -i "$media/en.srt" -i "$media/en.srt" \
	-map 0:v -map 1:a -map 2:a -map 3:s -map 4:s -c:v libx264 -b:v 1M -pix_fmt yuv420p \
	-c:a:0 ac3 -b:a:0 384k -ac:a:0 6 -c:a:1 aac -b:a:1 128k -ac:a:1 2 -c:s srt \
	-metadata:s:a:0 language=eng -metadata:s:a:0 title='Surround 5.1' \
	-metadata:s:a:1 language=fre -metadata:s:a:1 title='Commentaire' \
	-metadata:s:s:0 language=eng -metadata:s:s:1 language=eng -metadata:s:s:1 title='Forced' \
	-disposition:a:0 default -disposition:a:1 0 -disposition:s:0 0 -disposition:s:1 forced \
	"$(clip h264-ac3-srt-mkv mkv)"
printf '1\n00:00:01,000 --> 00:00:04,000\nR\303\251plique externe.\n' >"$playback/h264-ac3-srt-mkv/h264-ac3-srt-mkv.fr.srt"
encode -f lavfi -i "$pattern" -f lavfi -i "$(tone 660)" \
	-vf 'format=yuv420p10le,setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc' \
	-c:v libx265 -b:v 1M -profile:v main10 \
	-x265-params 'log-level=error:hdr10=1:repeat-headers=1:master-display=G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,1):max-cll=1000,400' \
	-c:a eac3 -b:a 384k -ac 6 "$(clip hevc10-hdr10-eac3-mkv mkv)"
encode -f lavfi -i "$pattern" -f lavfi -i "$(tone 220)" -f lavfi -i "$(tone 880)" -map 0:v -map 1:a -map 2:a \
	-c:v libx265 -b:v 1M -pix_fmt yuv420p -x265-params log-level=error \
	-c:a:0 dca -ac:a:0 6 -c:a:1 truehd -ac:a:1 6 -strict -2 "$(clip hevc-dts-truehd-mkv mkv)"
encode -f lavfi -i "$pattern" -f lavfi -i "$(tone 500)" \
	-c:v libsvtav1 -preset 12 -b:v 1M -pix_fmt yuv420p -c:a libopus -b:a 96k "$(clip av1-opus-webm webm)"
encode -f lavfi -i "$pattern" -f lavfi -i "$(tone 770)" \
	-c:v libx264 -b:v 1M -pix_fmt yuv420p -c:a aac -b:a 128k "$remote/remote-h264-aac-mkv.mkv"
printf 'http://%s:8000/remote-h264-aac-mkv.mkv\n' "$files" >"$(clip remote-h264-aac-mkv strm)"

docker network create "$network" >/dev/null
docker run --detach --rm --name "$files" --network "$network" --volume "$remote:/srv:ro" --workdir /srv \
	python:3.13-alpine python3 -m http.server 8000 >/dev/null

docker run --detach --rm --name "$container" --network "$network" --publish 127.0.0.1::8096 \
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

# Playback, on its own library so that the browsing fixtures above stay as
# they were.
post "$base/Library/VirtualFolders?name=Playback&collectionType=movies&paths=%2Fmedia%2Fplayback&refreshLibrary=true" \
	--header "Authorization: $signed" --data '{"LibraryOptions":{}}'
playback_scanned() {
	indexed Movie 9 && return 0
	idle && post "$base/Library/Refresh" --header "Authorization: $signed" && sleep 10
	return 1
}
await 'the six playback clips' playback_scanned
await 'the library scan' idle
clips=$(get "/Items?userId=$user&recursive=true&includeItemTypes=Movie&fields=Path" |
	jq '[.Items[] | select(.Path | startswith("/media/playback/")) | {key: (.Path | split("/")[3]), value: .Id}] | from_entries')
clip_id() { jq --exit-status --raw-output --arg clip "$1" '.[$clip]' <<<"$clips"; }
mkv=$(clip_id h264-ac3-srt-mkv)
strm=$(clip_id remote-h264-aac-mkv)
playback_out="$out/playback"
mkdir -p "$playback_out/probes"

# Upstream ffprobe's view of each clip, which is what Polyfin analyzes with.
for file in "$playback"/*/*.* "$remote/remote-h264-aac-mkv.mkv"; do
	case $file in *.srt | *.strm) continue ;; esac
	name=$(basename "${file%.*}")
	ffprobe -v error -print_format json -show_format -show_streams "$file" |
		jq --arg name "$(basename "$file")" '.format.filename = $name' >"$playback_out/probes/$name.json"
done

# Asks for a PlaybackInfo decision the way clients do (UserId, MediaSourceId
# and DeviceProfile, plus the JSON options given as third argument), and
# keeps what Polyfin compares: the decision, the TranscodeReasons of the
# TranscodingUrl, and the streams.
playback_info() {
	local id options=${3:-'{}'}
	id=$(clip_id "$1")
	post "$base/Items/$id/PlaybackInfo" --header "Authorization: $signed" \
		--data "$(jq --null-input --arg user "$user" --arg id "$id" --argjson options "$options" \
			--slurpfile profile "$playback_out/profiles/$2.json" \
			'$options + {UserId: $user, MediaSourceId: $id, DeviceProfile: $profile[0]}')"
}
decide() {
	local options=${3:-'{}'}
	playback_info "$@" | jq --arg media "$1" --arg profile "$2" --argjson options "$options" '
		.MediaSources[0] as $source
		| {media: $media, profile: $profile, options: $options,
			result: ($source | {SupportsDirectPlay, SupportsDirectStream, SupportsTranscoding,
				TranscodeReasons: ((.TranscodingUrl // "") | [capture("[?&]TranscodeReasons=(?<r>[^&]*)").r] | first // ""
					| if . == "" then [] else split(",") end),
				TranscodingSubProtocol, TranscodingContainer, Container, Bitrate,
				DefaultAudioStreamIndex, DefaultSubtitleStreamIndex}),
			MediaStreams: ($source.MediaStreams
				| walk(if type == "string" then sub("ApiKey=[^&]*"; "ApiKey=<redacted>") else . end))}'
}
streams=$(get "/Users/$user/Items/$mkv" | jq '.MediaStreams')
stream_index() { jq --exit-status --raw-output "first(.[] | select($1)) | .Index" <<<"$streams"; }
external=$(stream_index '.Type == "Subtitle" and .IsExternal')
embedded=$(stream_index '.Type == "Subtitle" and (.IsExternal | not) and (.IsForced | not)')
secondary=$(stream_index '.Type == "Audio" and (.IsDefault | not)')
{
	for media in h264-aac-mp4 h264-ac3-srt-mkv hevc10-hdr10-eac3-mkv hevc-dts-truehd-mkv av1-opus-webm remote-h264-aac-mkv; do
		for profile in jellyfin-web-chrome swiftfin swiftfin-native findroid androidtv minimal; do
			decide "$media" "$profile"
		done
	done
	decide h264-aac-mp4 jellyfin-web-chrome '{"MaxStreamingBitrate":500000}'
	decide hevc10-hdr10-eac3-mkv jellyfin-web-chrome '{"MaxStreamingBitrate":500000}'
	decide h264-aac-mp4 minimal-no-aac
	for profile in jellyfin-web-chrome androidtv swiftfin-native; do
		decide h264-ac3-srt-mkv "$profile" "{\"AudioStreamIndex\":$secondary}"
	done
	for profile in jellyfin-web-chrome swiftfin swiftfin-native androidtv minimal; do
		decide h264-ac3-srt-mkv "$profile" "{\"SubtitleStreamIndex\":$embedded}"
		decide h264-ac3-srt-mkv "$profile" "{\"SubtitleStreamIndex\":$external}"
	done
} | jq --slurp . >"$playback_out/decisions.json"

get "/Users/$user/Items/$mkv" | save item-media-sources
playback_info h264-ac3-srt-mkv jellyfin-web-chrome | save playback-info
playback_info remote-h264-aac-mkv jellyfin-web-chrome | save playback-info-remote
# Session reports answer 204 without a body; the session list shows them.
report() { post "$base/Sessions/Playing$1" --header "Authorization: $signed" --data "$2"; }
report '' "{\"ItemId\":\"$strm\",\"MediaSourceId\":\"$strm\",\"PlayMethod\":\"DirectPlay\",\"PositionTicks\":0}"
report /Progress "{\"ItemId\":\"$strm\",\"MediaSourceId\":\"$strm\",\"PositionTicks\":20000000,\"IsPaused\":true}"
get /Sessions | jq '[.[] | select(.DeviceId == "polyfin-fixtures")]' | save sessions-now-playing
report /Stopped "{\"ItemId\":\"$strm\",\"MediaSourceId\":\"$strm\",\"PositionTicks\":20000000}"

echo "Fixtures written to $out"
