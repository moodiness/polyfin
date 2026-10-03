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
# User data (played, favorites, resume points, Next Up) is recorded next, by a
# separate user, so that the browsing fixtures keep an untouched user's shape.
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
# Subtitles come after, on a library of their own: a clip with embedded
# SubRip and ASS tracks, fonts and a cover, whose unscrubbed answers (media
# source, PlaybackInfo, attachments, tracks in each format, encoding
# options) go to ass/.
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
# Holds a WebSocket open for the fixtures' session, so that it can be
# remotely controlled.
socket="$container-socket"
network="$container"
media=$(mktemp -d)
remote=$(mktemp -d)

cleanup() {
	docker stop "$container" "$files" "$socket" >/dev/null 2>&1 || true
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
get /Library/VirtualFolders | save virtual-folders
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

# User data, as a second user whose plays and favorites no fixture above sees.
# The clips are shorter than MinResumeDurationSeconds, so playback reports
# would mark them played: resume points are set through the UserData endpoint.
post "$base/Users/New" --header "Authorization: $signed" --data '{"Name":"viewer","Password":"viewer-password"}' >/dev/null
viewer_client='MediaBrowser Client="Polyfin fixtures", Device="Fixtures", DeviceId="polyfin-fixtures-viewer", Version="1.0.0"'
viewer_authentication=$(post "$base/Users/AuthenticateByName" --header "Authorization: $viewer_client" \
	--data '{"Username":"viewer","Pw":"viewer-password"}')
viewer=$(jq --raw-output .User.Id <<<"$viewer_authentication")
viewer_signed="$viewer_client, Token=\"$(jq --raw-output .AccessToken <<<"$viewer_authentication")\""
vget() { curl --silent --show-error --fail --header "Authorization: $viewer_signed" "$base$1"; }
vpost() { post "$base$1" --header "Authorization: $viewer_signed" "${@:2}"; }
movie_named() {
	get "/Items?userId=$user&recursive=true&includeItemTypes=Movie&fields=Path" |
		jq --exit-status --raw-output --arg file "/$1.mkv" '.Items[] | select(.Path | endswith($file)) | .Id'
}
episode_numbered() {
	get "/Shows/$series/Episodes?userId=$user" |
		jq --exit-status --raw-output --argjson season "$1" --argjson number "$2" \
			'.Items[] | select(.ParentIndexNumber == $season and .IndexNumber == $number) | .Id'
}
# Sets a resume point at 40% of the item's runtime, last played the given
# number of seconds ago (Resume and Next Up order by it).
resume_at() {
	local runtime data
	runtime=$(vget "/Users/$viewer/Items/$1" | jq --exit-status '.RunTimeTicks')
	data=$(jq --null-input --compact-output --argjson ticks "$((runtime * 2 / 5))" --argjson ago "$2" \
		'{PlaybackPositionTicks: $ticks, LastPlayedDate: (now - $ago | todate)}')
	vpost "/UserItems/$1/UserData?userId=$viewer" --data "$data" >/dev/null
}
# Saves a list whose first item, the only one its shape keeps, must be the
# given one.
save_led_by() {
	local json
	json=$(cat)
	jq --exit-status --arg id "$2" '.Items[0].Id == $id' <<<"$json" >/dev/null || {
		echo "$1: expected $2 first, got $(jq --compact-output '[.Items[].Name]' <<<"$json")" >&2
		return 1
	}
	save "$1" <<<"$json"
}
sintel=$(movie_named 'Sintel (2010)')
tears=$(movie_named 'Tears of Steel (2012)')
s01e02=$(episode_numbered 1 2)
s02e01=$(episode_numbered 2 1)
vpost "/UserPlayedItems/$movie?userId=$viewer" | save user-item-data
vpost "/UserFavoriteItems/$sintel?userId=$viewer" | save favorite-user-data
# S01E01 watched, then rewatched to 40%; S02E01 started; S01E02 is Next Up.
vpost "/Users/$viewer/PlayedItems/$episode?DatePlayed=$(jq --null-input --raw-output 'now - 7200 | todate')" >/dev/null
resume_at "$tears" 3600
resume_at "$episode" 120
resume_at "$s02e01" 60
# The home page's rows and the Favorites tab, with jellyfin-web 12.1's
# parameters; its Next Up cutoff is a date, 365 days back by default.
vget "/UserItems/Resume?userId=$viewer&limit=12&fields=PrimaryImageAspectRatio&mediaTypes=Video&imageTypeLimit=1&enableImageTypes=Primary&enableImageTypes=Backdrop&enableImageTypes=Thumb&enableTotalRecordCount=false" |
	save_led_by resume-items "$s02e01"
vget "/Shows/NextUp?userId=$viewer&limit=24&fields=PrimaryImageAspectRatio&fields=DateCreated&fields=Path&fields=MediaSourceCount&imageTypeLimit=1&enableImageTypes=Primary&enableImageTypes=Backdrop&enableImageTypes=Thumb&nextUpDateCutoff=$(jq --null-input --raw-output 'now - 365 * 86400 | strftime("%Y-%m-%d")')&enableTotalRecordCount=false&enableResumable=false&enableRewatching=false" |
	save_led_by next-up "$s01e02"
vget "/Users/$viewer/Items?SortBy=SeriesSortName%2CSortName&SortOrder=Ascending&Filters=IsFavorite&Recursive=true&Fields=PrimaryImageAspectRatio&CollapseBoxSetItems=false&ExcludeLocationTypes=Virtual&EnableTotalRecordCount=false&Limit=20&IncludeItemTypes=Movie" |
	save_led_by favorites "$sintel"
vget "/Users/$viewer/Items/$series" | save series-in-progress
vget "/Shows/$series/Episodes?userId=$viewer&$counts,Overview" | save_led_by episodes-in-progress "$episode"

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

# Localization lists, which any signed-in user can read.
get /Localization/Cultures | save localization-cultures
get /Localization/Countries | save localization-countries
get /Localization/ParentalRatings | save localization-parental-ratings
get /Localization/Options | save localization-options

# Requests from here on are also recorded by HTTP status, under a readable
# key, in next-statuses.json; non-empty error bodies go to
# next-error-bodies.json. request leaves the status in $code and the answer
# in $body, which kept saves as a fixture when the request succeeded.
statuses='{}'
error_bodies='{}'
request() {
	local key=$1 method=$2 path=$3 authorization=$4 response
	response=$(curl --silent --show-error --request "$method" --header "Authorization: $authorization" \
		--header 'Content-Type: application/json' --write-out '\n%{http_code}' "${@:5}" "$base$path")
	code=${response##*$'\n'}
	body=${response%$'\n'*}
	statuses=$(jq --compact-output --arg key "$key" --argjson code "$code" '.[$key] = $code' <<<"$statuses")
	if ((code >= 400)) && [[ -n $body ]]; then
		error_bodies=$(jq --compact-output --arg key "$key" --arg body "$body" \
			'.[$key] = ($body | fromjson? // $body)' <<<"$error_bodies")
	fi
}
kept() {
	if [[ $code == 2* ]]; then
		save "$1" <<<"$body"
	else
		echo "$1 not recorded: HTTP $code" >&2
	fi
}

# The viewer's playback settings: the configuration GET /Users/Me returns,
# with the languages and subtitle behavior changed.
configuration=$(vget /Users/Me | jq --compact-output '.Configuration
	| .AudioLanguagePreference = "fre" | .SubtitleLanguagePreference = "eng" | .SubtitleMode = "Always"
	| .PlayDefaultAudioTrack = false | .RememberSubtitleSelections = false')
request UserConfigurationUpdate POST "/Users/Configuration?userId=$viewer" "$viewer_signed" --data "$configuration"
request UserConfigured GET /Users/Me "$viewer_signed"
kept user-configured

# A password change keeps the token that made it; a wrong current password
# is refused.
request PasswordChange POST "/Users/Password?userId=$viewer" "$viewer_signed" \
	--data '{"CurrentPw":"viewer-password","NewPw":"viewer-password-2"}'
request PasswordWrongCurrent POST "/Users/Password?userId=$viewer" "$viewer_signed" \
	--data '{"CurrentPw":"wrong-password","NewPw":"viewer-password-3"}'

# No segment or subtitle provider is installed: these give the envelopes.
request MediaSegments GET "/MediaSegments/$episode" "$signed"
kept media-segments
request RemoteSubtitleSearch GET "/Items/$movie/RemoteSearch/Subtitles/eng" "$signed"
kept remote-subtitles

# The pages of the test movie's first genre, first studio and production
# year, and the lists apps show on them.
details=$(get "/Users/$user/Items/$movie")
uri() { jq --raw-input --raw-output @uri <<<"$1"; }
genre=$(get "/Genres/$(uri "$(jq --exit-status --raw-output '.Genres[0]' <<<"$details")")?userId=$user")
studio=$(get "/Studios/$(uri "$(jq --exit-status --raw-output '.Studios[0].Name' <<<"$details")")?userId=$user")
year=$(jq --exit-status --raw-output .ProductionYear <<<"$details")
save genre <<<"$genre"
save studio <<<"$studio"
get "/Years/$year?userId=$user" | save year
listing="/Items?userId=$user&recursive=true&includeItemTypes=Movie,Series&fields=PrimaryImageAspectRatio&sortBy=SortName&sortOrder=Ascending"
get "$listing&genreIds=$(jq --exit-status --raw-output .Id <<<"$genre")" | save_led_by items-by-genre "$movie"
get "$listing&studioIds=$(jq --exit-status --raw-output .Id <<<"$studio")" | save_led_by items-by-studio "$movie"
get "$listing&years=$year" | save_led_by items-by-year "$movie"

# Remote control of the fixtures' own session.
session=$(get /Sessions | jq --exit-status --raw-output 'first(.[] | select(.DeviceId == "polyfin-fixtures")) | .Id')
request SessionPlay POST "/Sessions/$session/Playing?playCommand=PlayNow&itemIds=$movie" "$signed"
request SessionPause POST "/Sessions/$session/Playing/Pause" "$signed"
request SessionDisplayMessage POST "/Sessions/$session/Command/DisplayMessage" "$signed"
request SessionMessage POST "/Sessions/$session/Message" "$signed" --data '{"Header":"h","Text":"t"}'
request SessionViewing POST "/Sessions/$session/Viewing?itemType=Movie&itemId=$movie&itemName=x" "$signed"
request SessionGoHome POST "/Sessions/$session/System/GoHome" "$signed"
# A session is controllable once it declares media control and holds a
# WebSocket; a bare handshake kept open is enough.
request SessionCapabilities POST '/Sessions/Capabilities?playableMediaTypes=Video&supportsMediaControl=true' "$signed"
docker run --detach --rm --init --name "$socket" --network "$network" python:3.13-alpine python3 -c '
import base64, os, socket, sys, time
connection = socket.create_connection((sys.argv[1], 8096))
connection.sendall((
	"GET /socket HTTP/1.1\r\nHost: %s:8096\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"
	"Sec-WebSocket-Version: 13\r\nSec-WebSocket-Key: %s\r\nAuthorization: %s\r\n\r\n"
	% (sys.argv[1], base64.b64encode(os.urandom(16)).decode(), sys.argv[2])).encode())
time.sleep(3600)' "$container" "$signed" >/dev/null
controllable() { get "/Sessions?controllableByUserId=$user" | jq --exit-status 'length > 0'; }
await 'a controllable session' controllable
request SessionsControllable GET "/Sessions?controllableByUserId=$user" "$signed"
kept sessions-controllable

# Playlists last, since they add a view to the user's.
request PlaylistCreate POST /Playlists "$signed" \
	--data "$(jq --null-input --compact-output --arg user "$user" --arg movie "$movie" --arg episode "$episode" \
		'{Name: "Fixture playlist", Ids: [$movie, $episode], UserId: $user, MediaType: "Video"}')"
kept playlist-created
playlist=$(jq --exit-status --raw-output .Id <<<"$body")
request PlaylistGet GET "/Playlists/$playlist" "$signed"
kept playlist
request PlaylistItems GET "/Playlists/$playlist/Items?userId=$user" "$signed"
kept playlist-items
entry=$(jq --exit-status --raw-output '.Items[0].PlaylistItemId' <<<"$body")
request PlaylistItem GET "/Users/$user/Items/$playlist" "$signed"
kept playlist-item
await 'the Playlists view' view playlists
request ViewsWithPlaylists GET "/UserViews?userId=$user" "$signed"
kept views-with-playlists
request Playlists GET "/Items?userId=$user&includeItemTypes=Playlist&recursive=true" "$signed"
kept playlists
request PlaylistAddItem POST "/Playlists/$playlist/Items?ids=$sintel&userId=$user" "$signed"
request PlaylistMoveItem POST "/Playlists/$playlist/Items/$entry/Move/1" "$signed"
request PlaylistRemoveItem DELETE "/Playlists/$playlist/Items?entryIds=$entry" "$signed"
request PlaylistRename POST "/Playlists/$playlist" "$signed" --data '{"Name":"Renamed"}'
# Shared with the viewer, so that the playlist's users list has an entry.
request PlaylistShare POST "/Playlists/$playlist/Users/$viewer" "$signed" --data '{"CanEdit":true}'
request PlaylistUsers GET "/Playlists/$playlist/Users" "$signed"
kept playlist-users
request PlaylistUser GET "/Playlists/$playlist/Users/$user" "$signed"
kept playlist-user

# Subtitles and attachments, last and on a library of their own so that no
# fixture above sees the clip: an MKV with a French SubRip track, an English
# ASS track using what renderers must keep (two styles, italics, a \N break,
# a positioned sign, a \h hard space), three dummy fonts with the MIME types
# muxers write (Odd.ttf's is one jellyfin-web does not list) and a JPEG
# cover, which FFmpeg's demuxer turns into an attached picture. ass/ holds
# unscrubbed answers, with tokens redacted: the media source, the
# PlaybackInfo decisions, the attachment route, the tracks in each subtitle
# format, and the encoding options jellyfin-web reads its fallback font from.
ass_media="$media/subtitles/ass-srt-fonts-mkv"
sources="$media/sources"
mkdir -p "$ass_media" "$sources"
printf '1\n00:00:01,000 --> 00:00:04,000\nPremi\303\250re r\303\251plique.\n\n2\n00:00:05,000 --> 00:00:08,000\n<i>Seconde</i> r\303\251plique.\n' >"$sources/fr.srt"
cat >"$sources/en.ass" <<'EOF'
[Script Info]
ScriptType: v4.00+
PlayResX: 640
PlayResY: 360
WrapStyle: 0

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,Test,24,&H00FFFFFF,&H000000FF,&H00000000,&H80000000,0,0,0,0,100,100,0,0,1,2,1,2,20,20,20,1
Style: Sign,Odd,18,&H0000FFFF,&H000000FF,&H00000000,&H00000000,1,0,0,0,100,100,0,0,1,1,0,8,10,10,10,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:01.00,0:00:04.00,Default,,0,0,0,,First line\Nsecond line
Dialogue: 0,0:00:02.00,0:00:05.00,Sign,,0,0,0,,{\pos(320,40)}A SIGN
Dialogue: 0,0:00:05.00,0:00:08.00,Default,Bob,0,0,0,,{\i1}Italic{\i0} words\hwith a hard space
EOF
# Fonts with dummy bytes: Jellyfin only stores and serves them.
printf 'dummy TrueType font\n' >"$sources/Test.ttf"
printf 'dummy OpenType font\n' >"$sources/Test.otf"
printf 'dummy odd font\n' >"$sources/Odd.ttf"
encode -f lavfi -i 'testsrc2=size=64x64:rate=1:duration=1' -frames:v 1 "$sources/cover.jpg"
encode -f lavfi -i "$pattern" -f lavfi -i "$(tone 440)" -i "$sources/fr.srt" -i "$sources/en.ass" \
	-map 0:v -map 1:a -map 2:s -map 3:s -c:v libx264 -b:v 1M -pix_fmt yuv420p -c:a aac -b:a 128k \
	-c:s:0 srt -c:s:1 ass -metadata:s:a:0 language=eng \
	-metadata:s:s:0 language=fre -metadata:s:s:1 language=eng -metadata:s:s:1 title='Signs & Songs' \
	-attach "$sources/Test.ttf" -metadata:s:t:0 mimetype=application/x-truetype-font \
	-attach "$sources/Test.otf" -metadata:s:t:1 mimetype=application/vnd.ms-opentype \
	-attach "$sources/Odd.ttf" -metadata:s:t:2 mimetype=application/x-font-ttf \
	-attach "$sources/cover.jpg" -metadata:s:t:3 mimetype=image/jpeg \
	"$ass_media/ass-srt-fonts-mkv.mkv"
post "$base/Library/VirtualFolders?name=Subtitles&collectionType=movies&paths=%2Fmedia%2Fsubtitles&refreshLibrary=true" \
	--header "Authorization: $signed" --data '{"LibraryOptions":{}}'
subtitles_scanned() {
	indexed Movie 10 && return 0
	idle && post "$base/Library/Refresh" --header "Authorization: $signed" && sleep 10
	return 1
}
await 'the subtitles clip' subtitles_scanned
await 'the library scan' idle
ass_out="$out/ass"
mkdir -p "$ass_out"
ass=$(get "/Items?userId=$user&recursive=true&includeItemTypes=Movie&fields=Path" |
	jq --exit-status --raw-output '.Items[] | select(.Path | endswith("/ass-srt-fonts-mkv.mkv")) | .Id')
redact='walk(if type == "string" then gsub("(?<k>api_key|ApiKey)=[^&]*"; "\(.k)=<redacted>"; "i") else . end)'
ass_source=$(get "/Users/$user/Items/$ass" | jq '.MediaSources[0]')
jq --sort-keys "$redact" <<<"$ass_source" >"$ass_out/media-source.json"
ass_playback_info() {
	post "$base/Items/$ass/PlaybackInfo" --header "Authorization: $signed" \
		--data "$(jq --null-input --arg user "$user" --arg id "$ass" --argjson options "$1" \
			--slurpfile profile "$playback_out/profiles/jellyfin-web-chrome.json" \
			'$options + {UserId: $user, MediaSourceId: $id, DeviceProfile: $profile[0]}')" |
		jq --argjson options "$1" '{options: $options, MediaSource: .MediaSources[0]}'
}
ass_index=$(jq --exit-status '.MediaStreams[] | select(.Type == "Subtitle" and .Codec == "ass") | .Index' <<<"$ass_source")
srt_index=$(jq --exit-status '.MediaStreams[] | select(.Type == "Subtitle" and .Codec != "ass") | .Index' <<<"$ass_source")
{
	ass_playback_info '{}'
	ass_playback_info "{\"SubtitleStreamIndex\":$ass_index}"
	ass_playback_info "{\"SubtitleStreamIndex\":$srt_index}"
} | jq --slurp --sort-keys "$redact" >"$ass_out/playback-info.json"

# Answers recorded by key: status, Content-Type and length, plus whether the
# bytes equal the attached file when one is expected, else the body.
ass_answers='{}'
answer() {
	local key=$1 path=$2 file=$3 headers code same=null
	headers=$(mktemp)
	code=$(curl --silent --show-error --output "$headers.body" --dump-header "$headers" --write-out '%{http_code}' \
		"${@:4}" "$base$path")
	if [[ -n $file ]]; then
		same=false
		cmp --silent "$file" "$headers.body" && same=true
	fi
	ass_answers=$(jq --compact-output --arg key "$key" --argjson code "$code" --argjson same "$same" \
		--arg type "$(tr -d '\r' <"$headers" | sed -n 's/^[Cc]ontent-[Tt]ype: //p' | tail -n 1)" \
		--argjson length "$(wc -c <"$headers.body")" --rawfile body "$headers.body" \
		'.[$key] = {status: $code, contentType: $type, length: $length}
			+ (if $same != null then {equalsAttachedFile: $same} else {} end)
			+ (if $code >= 400 or $same == null then {body: ($body | fromjson? // $body)} else {} end)' <<<"$ass_answers")
	rm -f "$headers" "$headers.body"
}
attachment() { printf '/Videos/%s/%s/Attachments/%s' "$1" "$2" "$3"; }
while IFS=$'\t' read -r index name; do
	answer "Attachment $name anonymous" "$(attachment "$ass" "$ass" "$index")" "$sources/$name"
	answer "Attachment $name signed" "$(attachment "$ass" "$ass" "$index")" "$sources/$name" \
		--header "Authorization: $signed"
done < <(jq --raw-output '.MediaAttachments[] | [.Index, .FileName] | @tsv' <<<"$ass_source")
missing=0123456789abcdef0123456789abcdef
first=$(jq --exit-status '.MediaAttachments[0].Index' <<<"$ass_source")
answer 'Attachment wrong index' "$(attachment "$ass" "$ass" 99)" '' --header "Authorization: $signed"
answer 'Attachment wrong media source' "$(attachment "$ass" "$missing" "$first")" '' --header "Authorization: $signed"
answer 'Attachment wrong item' "$(attachment "$missing" "$ass" "$first")" '' --header "Authorization: $signed"
answer 'Encoding configuration as viewer' /System/Configuration/encoding '' --header "Authorization: $viewer_signed"
answer 'Encoding configuration as administrator' /System/Configuration/encoding '' --header "Authorization: $signed"
answer 'Unknown configuration key' /System/Configuration/unknown '' --header "Authorization: $signed"
answer 'Fallback fonts as viewer' /FallbackFont/Fonts '' --header "Authorization: $viewer_signed"
answer 'Fallback fonts as administrator' /FallbackFont/Fonts '' --header "Authorization: $signed"
jq --sort-keys . <<<"$ass_answers" >"$ass_out/answers.json"

# The tracks as Jellyfin serves them, from the start.
for format in ass js vtt srt; do
	get "/Videos/$ass/$ass/Subtitles/$ass_index/0/Stream.$format" >"$ass_out/ass-track.$format"
done
get "/Videos/$ass/$ass/Subtitles/$srt_index/0/Stream.ass" >"$ass_out/srt-track.ass"

jq --sort-keys . <<<"$statuses" >"$out/next-statuses.json"
save next-error-bodies <<<"$error_bodies"

echo "Fixtures written to $out"

# Chapters, on a library of their own: an MKV whose chapters are titled,
# untitled, titled with a time and titled with a number (the last two are
# names Jellyfin replaces), one starting off the millisecond. chapters/
# holds upstream ffprobe's view of the clip with its chapters, which is what
# Polyfin analyzes with, and the chapters Jellyfin returns in item details,
# in a listing that asks for them, and in the session playing the clip.
chapters_media="$media/chapters/chapters-mkv"
chapters_out="$out/chapters"
mkdir -p "$chapters_media" "$chapters_out"
cat >"$sources/chapters.txt" <<'EOF'
;FFMETADATA1
[CHAPTER]
TIMEBASE=1/1000000
START=0
END=2500400
title=Opening
[CHAPTER]
TIMEBASE=1/1000000
START=2500400
END=5000000
[CHAPTER]
TIMEBASE=1/1000000
START=5000000
END=7500000
title=00:05:00.000
[CHAPTER]
TIMEBASE=1/1000000
START=7500000
END=10000000
title=7
EOF
encode -f lavfi -i "$pattern" -f lavfi -i "$(tone 660)" -i "$sources/chapters.txt" -map 0:v -map 1:a -map_chapters 2 \
	-c:v libx264 -b:v 1M -pix_fmt yuv420p -c:a aac -b:a 128k "$chapters_media/chapters-mkv.mkv"
ffprobe -v error -print_format json -show_format -show_streams -show_chapters "$chapters_media/chapters-mkv.mkv" |
	jq --sort-keys '.format.filename = "chapters-mkv.mkv"' >"$chapters_out/probe.json"
post "$base/Library/VirtualFolders?name=Chapters&collectionType=movies&paths=%2Fmedia%2Fchapters&refreshLibrary=true" \
	--header "Authorization: $signed" --data '{"LibraryOptions":{}}'
chapters_scanned() {
	indexed Movie 11 && return 0
	idle && post "$base/Library/Refresh" --header "Authorization: $signed" && sleep 10
	return 1
}
await 'the chapters clip' chapters_scanned
await 'the library scan' idle
chaptered=$(get "/Items?userId=$user&recursive=true&includeItemTypes=Movie&fields=Path" |
	jq --exit-status --raw-output '.Items[] | select(.Path | endswith("/chapters-mkv.mkv")) | .Id')
chapters_detail=$(get "/Users/$user/Items/$chaptered")
chapters_listed=$(get "/Items?userId=$user&ids=$chaptered&fields=Chapters")
chapters_unasked=$(get "/Items?userId=$user&ids=$chaptered")
report '' "{\"ItemId\":\"$chaptered\",\"MediaSourceId\":\"$chaptered\",\"PlayMethod\":\"DirectPlay\",\"PositionTicks\":0}"
chapters_playing=$(get /Sessions | jq 'first(.[] | select(.DeviceId == "polyfin-fixtures")) | .NowPlayingItem')
report /Stopped "{\"ItemId\":\"$chaptered\",\"MediaSourceId\":\"$chaptered\",\"PositionTicks\":0}"
jq --null-input --sort-keys --argjson detail "$chapters_detail" --argjson listed "$chapters_listed" \
	--argjson unasked "$chapters_unasked" --argjson playing "$chapters_playing" \
	'{detail: $detail.Chapters, listing: $listed.Items[0].Chapters,
		listingWithoutTheField: ($unasked.Items[0] | has("Chapters")), nowPlaying: $playing.Chapters}' \
	>"$chapters_out/chapters.json"
# Downloads: GET /Items/{itemId}/Download, as jellyfin-web and Streamyfin
# call it, with the token in the ApiKey parameter or the Authorization
# header. downloads/answers.json holds, by key, the status and the headers
# that matter, and the body of errors; downloads/can-download.json holds
# CanDownload where apps read it.
download_answers='{}'
download_answer() {
	local key=$1 path=$2 headers code
	headers=$(mktemp)
	code=$(curl --silent --show-error --output "$headers.body" --dump-header "$headers" --write-out '%{http_code}' \
		"${@:3}" "$base$path")
	header() { tr -d '\r' <"$headers" | sed -n "s/^$1: //Ip" | tail -n 1; }
	download_answers=$(jq --compact-output --arg key "$key" --argjson code "$code" \
		--arg type "$(header Content-Type)" --arg disposition "$(header Content-Disposition)" \
		--arg ranges "$(header Accept-Ranges)" --arg range "$(header Content-Range)" \
		--argjson length "$(wc -c <"$headers.body")" --rawfile body "$headers.body" \
		'.[$key] = {status: $code, contentType: $type, contentDisposition: $disposition,
			acceptRanges: $ranges, contentRange: $range, length: $length}
			+ (if $code >= 400 then {body: ($body | fromjson? // $body)} else {} end)' <<<"$download_answers")
	rm -f "$headers" "$headers.body"
}
download_token=$(sed -n 's/.*Token="\([^"]*\)".*/\1/p' <<<"$signed")
download_answer 'Movie with the token in ApiKey' "/Items/$movie/Download?ApiKey=$download_token"
download_answer 'Movie with the token in the header' "/Items/$movie/Download" --header "Authorization: $signed"
download_answer 'Movie, a byte range' "/Items/$movie/Download?ApiKey=$download_token" --header 'Range: bytes=0-99'
download_answer 'Movie, HEAD' "/Items/$movie/Download?ApiKey=$download_token" --head
download_answer 'Episode' "/Items/$episode/Download?ApiKey=$download_token"
download_answer 'Remote movie (.strm)' "/Items/$strm/Download?ApiKey=$download_token"
download_answer 'Series' "/Items/$series/Download?ApiKey=$download_token"
download_answer 'Unknown item' "/Items/$missing/Download?ApiKey=$download_token"
download_answer 'Not an id' "/Items/nothing/Download?ApiKey=$download_token"
download_answer 'Anonymous' "/Items/$movie/Download"
download_answer 'Wrong token' "/Items/$movie/Download?ApiKey=$missing"
# The viewer, with content downloading turned off for a moment.
viewer_policy=$(get "/Users/$viewer" | jq --compact-output .Policy)
post "$base/Users/$viewer/Policy" --header "Authorization: $signed" \
	--data "$(jq --compact-output '.EnableContentDownloading = false' <<<"$viewer_policy")"
download_answer 'Viewer not allowed to download' "/Items/$movie/Download" --header "Authorization: $viewer_signed"
post "$base/Users/$viewer/Policy" --header "Authorization: $signed" --data "$viewer_policy"
download_answer 'Viewer allowed to download' "/Items/$movie/Download" --header "Authorization: $viewer_signed"
mkdir -p "$out/downloads"
jq --sort-keys . <<<"$download_answers" >"$out/downloads/answers.json"
can_download() { get "$1" | jq "$2"; }
jq --null-input --sort-keys \
	--argjson movie "$(can_download "/Users/$user/Items/$movie" .CanDownload)" \
	--argjson episode "$(can_download "/Users/$user/Items/$episode" .CanDownload)" \
	--argjson remote "$(can_download "/Users/$user/Items/$strm" .CanDownload)" \
	--argjson series "$(can_download "/Users/$user/Items/$series" .CanDownload)" \
	--argjson listed "$(can_download "/Shows/$series/Episodes?seasonId=$season&userId=$user&fields=CanDownload,Path" \
		'.Items[0] | {CanDownload, Path: (.Path | type)}')" \
	--argjson unasked "$(can_download "/Shows/$series/Episodes?seasonId=$season&userId=$user" '.Items[0] | has("CanDownload")')" \
	'{movieDetail: $movie, episodeDetail: $episode, remoteMovieDetail: $remote, seriesDetail: $series,
		episodeListedWithTheFields: $listed, listingWithoutTheFieldHasIt: $unasked}' >"$out/downloads/can-download.json"
