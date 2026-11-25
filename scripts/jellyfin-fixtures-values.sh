# Item values, sourced by jellyfin-fixtures.sh: values_media makes the files
# before the server starts, values_fixtures records the answers once
# everything else is. Unlike the shape fixtures, values/items.json keeps
# real values, for the fields whose value depends on the item's type, the
# user's permissions or the request, and that apps branch on:
#
# - Type, IsFolder: the page and the player apps open an item with.
# - MediaType: whether the video or the audio player plays it.
# - CollectionType: the tabs apps give a library.
# - LocationType: Virtual marks what has no file, which apps grey out.
# - PlayAccess, IsPlaceHolder: whether jellyfin-web offers to play it.
# - CanDownload, CanDelete: whether apps offer to download or delete it.
# - EnableMediaSourceDisplay: whether jellyfin-web lists its versions.
# - VideoType: discs play otherwise than files.
# - ChannelType: TV or radio, for channels.
# - IsSeries, IsMovie, IsNews: how Live TV apps file programmes.
#
# and, as true or false, whether these come at all: MediaSources (apps
# play from it), Chapters (scene lists), RunTimeTicks (progress bars and
# resuming), UserData (played and favorite marks), ParentId, SeriesId,
# SeasonId and AlbumId (the links to the pages above), ChildCount and
# RecursiveItemCount (counts on folders) and MediaSourceCount (the
# versions badge); then the keys of UserData and of ImageTags. A field
# left out of "values" was not sent, or was null.
#
# Each kind is recorded as its details (/Users/{userId}/Items/{itemId}) and
# in a listing by its identifier asking for the fields apps ask for, by two
# users who are not administrators: one allowed to download, one not. A
# movie, an episode, its season and series, a BoxSet, a home video and its
# folder, a song, its album and artist, an audiobook, a playlist, a Live TV
# channel and a programme, a recording of each kind of programme (a movie,
# a series' episode, news, sports, a children's programme and one of no
# category, as "recording-" and the kind), and the libraries' views, under
# "view-" and their type, or their name for those without one (the
# Recordings library). Books and photos are left out: Polyfin serves
# neither.

# A home videos library: one video in a folder.
values_media() {
	local dir=$1
	mkdir -p "$dir/homevideos/Holiday"
	ffmpeg -nostdin -loglevel error -y -f lavfi -i 'testsrc=size=640x360:rate=24:duration=3' \
		-f lavfi -i 'sine=frequency=440:duration=3' -c:v libx264 -b:v 1M -pix_fmt yuv420p -c:a aac -shortest \
		"$dir/homevideos/Holiday/Beach.mkv"
}

values_fixtures() {
	local values_out="$out/values"
	mkdir -p "$values_out"
	post "$base/Library/VirtualFolders?name=Home%20videos&collectionType=homevideos&paths=%2Fmedia%2Fhomevideos&refreshLibrary=true" \
		--header "Authorization: $signed" --data '{"LibraryOptions":{}}'
	await 'the home video' values_indexed
	await 'the library scan' idle

	# The two users, and their tokens.
	local allowed denied allowed_signed denied_signed
	allowed=$(values_user downloader true)
	denied=$(values_user keeper false)
	allowed_signed=$(values_signed downloader)
	denied_signed=$(values_signed keeper)

	# The programme airing now on the guide's channel.
	local channel program
	channel=$(get "/LiveTv/Channels?userId=$user" | jq --exit-status --raw-output '.Items[0].Id')
	program=$(get "/LiveTv/Programs?userId=$user&channelIds=$channel&IsAiring=true" | jq --exit-status --raw-output '.Items[0].Id')

	# A recording of each kind of programme, a movie, a series' episode,
	# news, sports, a children's one and one of no category, each on a
	# channel of its own, as Jellyfin files recordings by their programme.
	# The channels and their guide are new addresses, as Jellyfin keeps
	# what it read of the others.
	{
		printf '#EXTM3U\n'
		local kind number=100
		for kind in $values_kinds; do
			number=$((number + 1))
			printf '#EXTINF:-1 tvg-id="kind.%s" tvg-chno="%s",Kind %s\nhttp://%s:8000/live/channel.m3u8?kind=%s\n' \
				"$kind" "$number" "$kind" "$files" "$kind"
		done
	} >"$live_files/kinds.m3u"
	{
		printf '<?xml version="1.0" encoding="UTF-8"?>\n<tv>\n'
		for kind in $values_kinds; do
			printf '<channel id="kind.%s"><display-name>Kind %s</display-name></channel>\n' "$kind" "$kind"
		done
		values_programme movie 'Kind Movie' '<desc>A movie.</desc><category>Movie</category><date>2001</date>'
		values_programme episode 'Kind Series' '<sub-title>The Second One</sub-title><desc>An episode.</desc><category>Series</category><episode-num system="xmltv_ns">1.4.</episode-num>'
		values_programme news 'Kind News' '<desc>The news.</desc><category>News</category>'
		values_programme sports 'Kind Sports' '<desc>A match.</desc><category>Sports</category>'
		values_programme kids 'Kind Kids' '<desc>Cartoons.</desc><category>Kids</category>'
		values_programme other 'Kind Other' '<desc>Something else.</desc>'
		printf '</tv>\n'
	} >"$live_files/kinds.xml"
	post "$base/LiveTv/TunerHosts" --header "Authorization: $signed" \
		--data "{\"Type\":\"m3u\",\"Url\":\"http://$files:8000/live/kinds.m3u\",\"FriendlyName\":\"Kinds\"}" >/dev/null
	post "$base/LiveTv/ListingProviders?validateListings=false&validateLogin=false" --header "Authorization: $signed" \
		--data "{\"Type\":\"xmltv\",\"Path\":\"http://$files:8000/live/kinds.xml\",\"EnableAllTuners\":true}" >/dev/null
	await 'the programmes of each kind' values_kinds_airing
	local timer program_id
	for program_id in $(get "/LiveTv/Programs?userId=$user&IsAiring=true" |
		jq --raw-output '.Items[] | select(.Name | startswith("Kind ")) | .Id'); do
		timer=$(get "/LiveTv/Timers/Defaults?programId=$program_id" |
			jq --compact-output '.EndDate = (now + 20 | todate) | .PrePaddingSeconds = 0 | .PostPaddingSeconds = 0 | .IsPrePaddingRequired = false | .IsPostPaddingRequired = false')
		post "$base/LiveTv/Timers" --header "Authorization: $signed" --data "$timer"
	done
	await 'the recordings' values_recorded
	await 'the library scan' idle
	local recordings
	recordings=$(get "/LiveTv/Recordings?userId=$user")
	recording() { jq --exit-status --raw-output --arg name "$1" 'first(.Items[] | select(.Name == $name)) | .Id' <<<"$recordings"; }

	# The items, by kind; the series, its season and its episode those of
	# the Shows library, not the recordings'.
	local items series
	series=$(values_first "includeItemTypes=Series&searchTerm=Pioneer")
	items=$(jq --null-input --compact-output \
		--arg movie "$(values_first "includeItemTypes=Movie&searchTerm=Buck")" \
		--arg series "$series" \
		--arg season "$(values_first "parentId=$series&includeItemTypes=Season&sortBy=IndexNumber")" \
		--arg episode "$(values_first "parentId=$series&includeItemTypes=Episode&sortBy=ParentIndexNumber,IndexNumber")" \
		--arg boxset "$(values_first "includeItemTypes=BoxSet")" \
		--arg video "$(values_first "includeItemTypes=Video&searchTerm=Beach")" \
		--arg folder "$(values_first "includeItemTypes=Folder&searchTerm=Holiday")" \
		--arg track "$(values_first "includeItemTypes=Audio&searchTerm=First")" \
		--arg album "$(values_first "includeItemTypes=MusicAlbum&searchTerm=Sine")" \
		--arg artist "$(get "/Artists?userId=$user&searchTerm=Tone" | jq --exit-status --raw-output '.Items[0].Id')" \
		--arg audiobook "$(values_first "includeItemTypes=AudioBook")" \
		--arg playlist "$(values_first "includeItemTypes=Playlist")" \
		--arg channel "$channel" \
		--arg program "$program" \
		--arg recording-movie "$(recording 'Kind Movie')" \
		--arg recording-episode "$(recording 'The Second One')" \
		--arg recording-news "$(recording 'Kind News')" \
		--arg recording-sports "$(recording 'Kind Sports')" \
		--arg recording-kids "$(recording 'Kind Kids')" \
		--arg recording-other "$(recording 'Kind Other')" \
		'$ARGS.named')
	# Views by their collection type, those without one by their name: the
	# Recordings library, which Jellyfin 12.4 adds twice, as Recordings and
	# Recordings2.
	while IFS=$'\t' read -r kind id; do
		items=$(jq --compact-output --arg kind "$kind" --arg id "$id" '.[$kind] = $id' <<<"$items")
	done < <(get "/UserViews?userId=$user" |
		jq --exit-status --raw-output '.Items | map(. + {key: ("view-" + (.CollectionType // (.Name | ascii_downcase | sub("[0-9]+$"; ""))))})
			| unique_by(.key)[] | [.key, .Id] | @tsv')

	# The playlist is shared with both users, as playlists are their owner's.
	local playlist
	playlist=$(jq --exit-status --raw-output .playlist <<<"$items")
	post "$base/Playlists/$playlist/Users/$allowed" --header "Authorization: $signed" --data '{"CanEdit":false}'
	post "$base/Playlists/$playlist/Users/$denied" --header "Authorization: $signed" --data '{"CanEdit":false}'

	local recorded_values='{}' kind id who token detail listing
	while IFS=$'\t' read -r kind id; do
		for who in allowed denied; do
			if [[ $who == allowed ]]; then token=$allowed_signed; else token=$denied_signed; fi
			local uid=${!who}
			detail=$(values_get "$token" "/Users/$uid/Items/$id" | jq --compact-output "$values_project")
			listing=$(values_get "$token" "/Items?userId=$uid&ids=$id&fields=$values_fields" |
				jq --exit-status --compact-output ".Items[0] | $values_project")
			recorded_values=$(jq --compact-output --arg kind "$kind" --arg who "$who" \
				--argjson detail "$detail" --argjson listing "$listing" \
				'.[$kind][$who] = {detail: $detail, listing: $listing}' <<<"$recorded_values")
		done
	done < <(jq --raw-output 'to_entries[] | [.key, .value] | @tsv' <<<"$items")
	jq --null-input --sort-keys --arg fields "$values_fields" --argjson items "$recorded_values" \
		'{listingFields: ($fields | split(",")), items: $items}' >"$values_out/items.json"
	echo "Item values written to $values_out"
}

# The fields a listing asks for: those of the values recorded, as apps ask
# for them, jellyfin-web's and Findroid's among them.
values_fields='CanDelete,CanDownload,Chapters,ChildCount,MediaSources,MediaSourceCount,ParentId,PrimaryImageAspectRatio,RecursiveItemCount'
values_project='{
	values: with_entries(select(.value != null and (.key | IN("Type", "MediaType", "IsFolder", "CollectionType", "LocationType", "PlayAccess",
		"IsPlaceHolder", "CanDownload", "CanDelete", "EnableMediaSourceDisplay", "VideoType", "ChannelType", "IsSeries",
		"IsMovie", "IsNews")))),
	present: (with_entries(select(.value != null) | .value = true) as $sent
		| ["MediaSources", "Chapters", "RunTimeTicks", "UserData", "ParentId", "SeriesId", "SeasonId", "AlbumId",
			"ChildCount", "RecursiveItemCount", "MediaSourceCount"]
		| map({key: ., value: ($sent[.] // false)}) | from_entries),
	userDataKeys: ((.UserData // {}) | keys),
	imageTags: ((.ImageTags // {}) | keys)
}'

# values_indexed succeeds once the home video is indexed.
values_indexed() {
	indexed Video 1 && return 0
	idle && post "$base/Library/Refresh" --header "Authorization: $signed" && sleep 10
	return 1
}

# The kinds of programme recorded, each on its own channel.
values_kinds='movie episode news sports kids other'

# values_programme prints a guide's programme on kind's channel, airing
# from half an hour ago to half an hour from now, of the title given and
# the elements given.
values_programme() {
	printf '<programme start="%s +0000" stop="%s +0000" channel="kind.%s"><title>%s</title>%s</programme>\n' \
		"$(jq --null-input --raw-output 'now - 1800 | strftime("%Y%m%d%H%M%S")')" \
		"$(jq --null-input --raw-output 'now + 1800 | strftime("%Y%m%d%H%M%S")')" "$1" "$2" "$3"
}

# values_kinds_airing succeeds once the programme of each kind airs,
# refreshing the guide while it is idle.
values_kinds_airing() {
	get "/LiveTv/Programs?userId=$user&IsAiring=true" |
		jq --exit-status '[.Items[] | select(.Name | startswith("Kind "))] | length == 6' && return 0
	get /ScheduledTasks | jq --exit-status '.[] | select(.Key == "RefreshGuide") | .State == "Idle"' >/dev/null &&
		post "$base/ScheduledTasks/Running/$guide_task" --header "Authorization: $signed" && sleep 10
	return 1
}

# values_recorded succeeds once the recording of each kind is over and
# listed.
values_recorded() {
	get "/LiveTv/Recordings?userId=$user&IsInProgress=false" | jq --exit-status '.TotalRecordCount >= 6'
}

# values_first is the identifier of the first item of the fixtures user's
# items a query matches, recursively.
values_first() {
	get "/Items?userId=$user&recursive=true&$1" | jq --exit-status --raw-output '.Items[0].Id'
}

# values_user makes a user, allowed to download or not, and gives its
# identifier.
values_user() {
	local id
	id=$(post "$base/Users/New" --header "Authorization: $signed" --data "{\"Name\":\"$1\",\"Password\":\"$1-password\"}" |
		jq --exit-status --raw-output .Id)
	post "$base/Users/$id/Policy" --header "Authorization: $signed" \
		--data "$(get "/Users/$id" | jq --compact-output --argjson allowed "$2" '.Policy | .EnableContentDownloading = $allowed')"
	printf '%s' "$id"
}

# values_signed signs a user in and gives its Authorization header.
values_signed() {
	local client="MediaBrowser Client=\"Polyfin fixtures\", Device=\"Fixtures\", DeviceId=\"polyfin-fixtures-$1\", Version=\"1.0.0\""
	printf '%s, Token="%s"' "$client" "$(post "$base/Users/AuthenticateByName" --header "Authorization: $client" \
		--data "{\"Username\":\"$1\",\"Pw\":\"$1-password\"}" | jq --exit-status --raw-output .AccessToken)"
}

values_get() { curl --silent --show-error --fail --header "Authorization: $1" "$base$2"; }
