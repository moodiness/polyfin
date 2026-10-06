# Music and audiobook fixtures, sourced by jellyfin-fixtures.sh: music_media
# makes the files before the server starts, music_fixtures records the
# answers once everything else is, on libraries of their own so that no
# fixture above sees them. Shapes go to music/ like the others; music/raw/
# keeps unscrubbed answers (tokens redacted) where values matter: playback
# decisions, playlists, statuses and error bodies.

# Ten-second tones tagged as two albums by two artists, one track shared
# with a guest artist, and a cover image per album; then a short audiobook
# with three chapters.
music_media() {
	local dir=$1 tone=0
	mkdir -p "$dir/music/Tone Quartet/Sine Studies" "$dir/music/Square Circle/Waveforms" "$dir/books/Narrated Tale"
	track() {
		local file=$1 artist=$2 album_artist=$3 album=$4 title=$5 number=$6 year=$7 codec=$8
		tone=$((tone + 110))
		ffmpeg -nostdin -loglevel error -y -f lavfi -i "sine=frequency=$((220 + tone)):duration=10" \
			-ac 2 -c:a "$codec" -metadata artist="$artist" -metadata album_artist="$album_artist" -metadata album="$album" \
			-metadata title="$title" -metadata track="$number" -metadata date="$year" -metadata genre=Electronic "$file"
	}
	cover() { ffmpeg -nostdin -loglevel error -y -f lavfi -i "testsrc=size=300x300:duration=1" -frames:v 1 "$1/cover.jpg"; }
	track "$dir/music/Tone Quartet/Sine Studies/01 First Light.flac" 'Tone Quartet' 'Tone Quartet' 'Sine Studies' 'First Light' 1 2021 flac
	track "$dir/music/Tone Quartet/Sine Studies/02 Second Wind.flac" 'Tone Quartet' 'Tone Quartet' 'Sine Studies' 'Second Wind' 2 2021 flac
	track "$dir/music/Tone Quartet/Sine Studies/03 Third Way.mp3" 'Tone Quartet' 'Tone Quartet' 'Sine Studies' 'Third Way' 3 2021 libmp3lame
	track "$dir/music/Square Circle/Waveforms/01 Saw Tooth.flac" 'Square Circle' 'Square Circle' 'Waveforms' 'Saw Tooth' 1 2019 flac
	track "$dir/music/Square Circle/Waveforms/02 Duet.flac" 'Square Circle; Tone Quartet' 'Square Circle' 'Waveforms' 'Duet' 2 2019 flac
	cover "$dir/music/Tone Quartet/Sine Studies"
	cover "$dir/music/Square Circle/Waveforms"
	printf ';FFMETADATA1\ntitle=Narrated Tale\nartist=Story Teller\nalbum=Narrated Tale\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=10000\ntitle=Opening\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=10000\nEND=20000\ntitle=Middle\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=20000\nEND=30000\ntitle=Ending\n' >"$dir/chapters.txt"
	ffmpeg -nostdin -loglevel error -y -f lavfi -i 'sine=frequency=330:duration=30' -i "$dir/chapters.txt" -map_metadata 1 -map_chapters 1 \
		-c:a aac -b:a 64k "$dir/books/Narrated Tale/Narrated Tale.m4b"
}

music_fixtures() {
	local music_out="$out/music" raw="$out/music/raw"
	mkdir -p "$raw"
	msave() { jq --sort-keys "$shape" >"$music_out/$1.json"; }
	redact() { jq --sort-keys 'walk(if type == "string" then sub("(?<k>[Aa]pi_?[Kk]ey)=[^&]*"; "\(.k)=<redacted>") else . end)'; }
	post "$base/Library/VirtualFolders?name=Music&collectionType=music&paths=%2Fmedia%2Fmusic&refreshLibrary=true" \
		--header "Authorization: $signed" --data '{"LibraryOptions":{}}'
	post "$base/Library/VirtualFolders?name=Books&collectionType=books&paths=%2Fmedia%2Fbooks&refreshLibrary=true" \
		--header "Authorization: $signed" --data '{"LibraryOptions":{}}'
	music_scanned() {
		indexed Audio 5 && indexed AudioBook 1 && return 0
		idle && post "$base/Library/Refresh" --header "Authorization: $signed" && sleep 10
		return 1
	}
	await 'the five songs and the audiobook' music_scanned
	await 'the library scan' idle
	local music books album artist song mp3 duet book
	music=$(view music)
	books=$(view books)
	album=$(get "/Items?userId=$user&recursive=true&includeItemTypes=MusicAlbum&searchTerm=Sine" | jq --exit-status --raw-output '.Items[0].Id')
	artist=$(get "/Artists?userId=$user&searchTerm=Tone" | jq --exit-status --raw-output '.Items[0].Id')
	song=$(get "/Items?userId=$user&recursive=true&includeItemTypes=Audio&searchTerm=First" | jq --exit-status --raw-output '.Items[0].Id')
	mp3=$(get "/Items?userId=$user&recursive=true&includeItemTypes=Audio&searchTerm=Third" | jq --exit-status --raw-output '.Items[0].Id')
	duet=$(get "/Items?userId=$user&recursive=true&includeItemTypes=Audio&searchTerm=Duet" | jq --exit-status --raw-output '.Items[0].Id')
	book=$(get "/Items?userId=$user&recursive=true&includeItemTypes=AudioBook" | jq --exit-status --raw-output '.Items[0].Id')

	# The music library as jellyfin-web 12.2 lists it: its tabs, an album,
	# an artist and a song, with the web client's parameters.
	local list="/Users/$user/Items?SortBy=SortName&SortOrder=Ascending&Recursive=true&Fields=PrimaryImageAspectRatio,SortName&ImageTypeLimit=1&EnableImageTypes=Primary,Backdrop,Banner,Thumb&StartIndex=0&Limit=100&ParentId=$music"
	get "/UserViews?userId=$user" | jq '{Items: [.Items[] | select(.CollectionType == "music")], TotalRecordCount, StartIndex}' | msave views
	get "$list&IncludeItemTypes=MusicAlbum" | msave albums
	get "/Users/$user/Items?SortBy=Album,SortName&SortOrder=Ascending&IncludeItemTypes=Audio&Recursive=true&Fields=AudioInfo,ParentId&StartIndex=0&ImageTypeLimit=1&EnableImageTypes=Primary&Limit=100&ParentId=$music" | msave songs
	get "/Artists/AlbumArtists?SortBy=SortName&SortOrder=Ascending&Recursive=true&Fields=PrimaryImageAspectRatio,SortName&ImageTypeLimit=1&EnableImageTypes=Primary,Backdrop,Banner,Thumb&StartIndex=0&Limit=100&ParentId=$music&userId=$user" | msave album-artists
	get "/Artists?SortBy=SortName&SortOrder=Ascending&Recursive=true&Fields=PrimaryImageAspectRatio,SortName&ImageTypeLimit=1&EnableImageTypes=Primary,Backdrop,Banner,Thumb&StartIndex=0&Limit=100&ParentId=$music&userId=$user" | msave artists
	get "/Artists/Tone%20Quartet?userId=$user" | msave artist-by-name
	get "/MusicGenres?SortBy=SortName&SortOrder=Ascending&Recursive=true&Fields=PrimaryImageAspectRatio,ItemCounts&StartIndex=0&ParentId=$music&userId=$user" | msave music-genres
	get "/Users/$user/Items/Latest?IncludeItemTypes=Audio&Limit=16&Fields=PrimaryImageAspectRatio&ParentId=$music&ImageTypeLimit=1&EnableImageTypes=Primary,Backdrop,Thumb" | msave latest
	get "/Users/$user/Items/$album" | msave album
	get "/Users/$user/Items?ParentId=$album&Fields=ItemCounts,PrimaryImageAspectRatio,CanDelete,MediaSourceCount&SortBy=ParentIndexNumber,IndexNumber,SortName" | msave album-songs
	get "/Users/$user/Items/$artist" | msave artist
	get "/Users/$user/Items?SortOrder=Descending,Descending,Ascending&IncludeItemTypes=MusicAlbum&Recursive=true&Fields=ParentId,PrimaryImageAspectRatio,ParentId&ImageTypeLimit=1&EnableImageTypes=Primary,Backdrop,Thumb&SortBy=PremiereDate,ProductionYear,SortName&ArtistIds=$artist" | msave artist-albums
	get "/Users/$user/Items?SortBy=SortName&SortOrder=Ascending&IncludeItemTypes=Audio&Recursive=true&Fields=AudioInfo,ParentId&Limit=100&StartIndex=0&ImageTypeLimit=1&EnableImageTypes=Primary&ArtistIds=$artist" | msave artist-songs
	get "/Users/$user/Items/$song" | msave song
	get "/Users/$user/Items/$duet" | jq '{ArtistItems, Artists, AlbumArtist, AlbumArtists}' >"$raw/duet-artists.json"
	get "/Users/$user/Items/$book" | msave audiobook
	get "/Users/$user/Items?ParentId=$books&Fields=PrimaryImageAspectRatio&Recursive=true&IncludeItemTypes=AudioBook" | msave audiobooks
	get "/Items/$song/Ancestors?userId=$user" | msave song-ancestors
	# The values that tell the kinds apart, unscrubbed.
	local kinds='{Type, MediaType, IsFolder, LocationType, CollectionType, PrimaryImageAspectRatio, ChildCount, SongCount, AlbumCount, RecursiveItemCount, IndexNumber, HasLyrics, Container, DisplayOrder}'
	jq --null-input --sort-keys \
		--argjson view "$(get "/UserViews?userId=$user" | jq '.Items[] | select(.CollectionType == "music")')" \
		--argjson books "$(get "/UserViews?userId=$user" | jq '.Items[] | select(.CollectionType == "books")')" \
		--argjson album "$(get "/Users/$user/Items/$album")" --argjson artist "$(get "/Users/$user/Items/$artist")" \
		--argjson song "$(get "/Users/$user/Items/$song")" --argjson book "$(get "/Users/$user/Items/$book")" \
		--argjson listed "$(get "$list&IncludeItemTypes=MusicAlbum" | jq '.Items[0]')" \
		"{view: (\$view | $kinds), books: (\$books | $kinds), album: (\$album | $kinds), artist: (\$artist | $kinds),
			song: (\$song | $kinds + {SongStream: (.MediaSources[0].MediaStreams[0] | {Type, Codec, Channels, SampleRate, BitDepth, BitRate, DisplayTitle})}),
			audiobook: (\$book | $kinds), listedAlbum: (\$listed | $kinds)}" >"$raw/values.json"

	# Search, the way jellyfin-web 12.2 asks for each kind, and the hints.
	get "/Users/$user/Items?searchTerm=wind&IncludeItemTypes=Audio&Recursive=true&Limit=24&Fields=PrimaryImageAspectRatio,CanDelete,MediaSourceCount&ImageTypeLimit=1&EnableTotalRecordCount=false" | msave search-songs
	get "/Users/$user/Items?searchTerm=sine&IncludeItemTypes=MusicAlbum&Recursive=true&Limit=24&Fields=PrimaryImageAspectRatio,CanDelete,MediaSourceCount&ImageTypeLimit=1&EnableTotalRecordCount=false" | msave search-albums
	get "/Artists?userId=$user&searchTerm=tone&Limit=24&Fields=PrimaryImageAspectRatio,CanDelete,MediaSourceCount&ImageTypeLimit=1&EnableTotalRecordCount=false" | msave search-artists
	get "/Search/Hints?userId=$user&searchTerm=wind&includeItemTypes=Audio,MusicAlbum,MusicArtist&limit=10" | msave search-hints

	# Instant mixes from a song, an album and an artist, then lyrics, of
	# which the songs have none: statuses, with error bodies, the names a
	# list holds, or the body.
	get "/Items/$song/InstantMix?userId=$user&limit=20&Fields=PrimaryImageAspectRatio" | msave instant-mix
	local statuses='{}' bodies='{}'
	answer() {
		local key=$1 path=$2 response code body
		response=$(curl --silent --show-error --header "Authorization: $signed" --write-out '\n%{http_code}' "$base$path")
		code=${response##*$'\n'}
		body=${response%$'\n'*}
		statuses=$(jq --compact-output --arg key "$key" --argjson code "$code" '.[$key] = $code' <<<"$statuses")
		bodies=$(jq --compact-output --arg key "$key" --arg body "$body" '
			($body | fromjson? // $body) as $json
			| .[$key] = (if ($json | type) == "object" and ($json | has("Items")) then {names: ([$json.Items[].Name] | sort), TotalRecordCount: $json.TotalRecordCount}
				else $json end)' <<<"$bodies")
	}
	answer InstantMixSong "/Items/$song/InstantMix?userId=$user&limit=20"
	answer InstantMixAlbum "/Albums/$album/InstantMix?userId=$user&limit=20"
	answer InstantMixArtist "/Artists/$artist/InstantMix?userId=$user&limit=20"
	answer InstantMixArtistQuery "/Artists/InstantMix?id=$artist&userId=$user&limit=20"
	answer InstantMixSongs "/Songs/$song/InstantMix?userId=$user&limit=20"
	answer InstantMixNone "/Albums/$album/InstantMix?userId=$user&limit=0"
	answer InstantMixUnknown "/Items/0123456789abcdef0123456789abcdef/InstantMix?userId=$user"
	answer Lyrics "/Audio/$song/Lyrics"
	answer RemoteLyrics "/Audio/$song/RemoteSearch/Lyrics"
	answer LyricProviders "/Lyrics/Providers"
	answer LyricsUnknown "/Audio/0123456789abcdef0123456789abcdef/Lyrics"

	# User data on a song, an album and an artist.
	post "$base/UserFavoriteItems/$song?userId=$user" --header "Authorization: $signed" | msave favorite-song
	post "$base/UserFavoriteItems/$album?userId=$user" --header "Authorization: $signed" >/dev/null
	post "$base/UserFavoriteItems/$artist?userId=$user" --header "Authorization: $signed" >/dev/null
	post "$base/UserPlayedItems/$song?userId=$user" --header "Authorization: $signed" | jq --sort-keys . >"$raw/played-song.json"

	# Playback of a FLAC song as jellyfin-web 12.2 on Chrome asks it: as it
	# is, then held below the bitrate of FLAC, which converts it. The
	# answers are kept unscrubbed, tokens redacted, with the HLS playlists
	# the conversion offers and the headers of the static stream.
	local profile
	profile=$(jq --compact-output . "$out/playback/profiles/jellyfin-web-chrome.json")
	audio_info() {
		post "$base/Items/$1/PlaybackInfo?userId=$user" --header "Authorization: $signed" \
			--data "$(jq --null-input --compact-output --arg user "$user" --arg id "$1" --argjson profile "$profile" --argjson options "$2" \
				'$options + {UserId: $user, MediaSourceId: $id, DeviceProfile: $profile, AutoOpenLiveStream: true, IsPlayback: true}')"
	}
	audio_info "$song" '{}' | redact >"$raw/playback-info-flac.json"
	audio_info "$song" '{}' | msave playback-info
	audio_info "$mp3" '{}' | redact >"$raw/playback-info-mp3.json"
	audio_info "$song" '{"MaxStreamingBitrate":128000}' | redact >"$raw/playback-info-converted.json"
	local transcoding master
	transcoding=$(audio_info "$song" '{"MaxStreamingBitrate":128000}' | jq --exit-status --raw-output '.MediaSources[0].TranscodingUrl')
	master=$(get "$transcoding")
	local media_playlist
	media_playlist=$(grep -v '^#' <<<"$master" | head -n 1)
	get "/Audio/$song/$media_playlist" | sed -E 's/([Aa]pi_?[Kk]ey)=[^&]*/\1=<redacted>/g' >"$raw/main.m3u8"
	sed -E 's/([Aa]pi_?[Kk]ey)=[^&]*/\1=<redacted>/g' <<<"$master" >"$raw/master.m3u8"
	local token
	token=$(jq --raw-output '.AccessToken' <<<"$authentication")
	curl --silent --show-error --dump-header - --output /dev/null "$base/Audio/$song/stream.flac?static=true&ApiKey=$token" |
		tr -d '\r' >"$raw/stream-headers.txt"
	curl --silent --show-error --dump-header - --output /dev/null \
		"$base/Audio/$song/universal?UserId=$user&DeviceId=polyfin-fixtures&MaxStreamingBitrate=140000000&Container=opus,webm|opus,mp3,aac,m4a|aac,m4b|aac,flac,webma,webm|webma,wav,ogg&TranscodingContainer=mp4&TranscodingProtocol=hls&AudioCodec=aac&ApiKey=$token&PlaySessionId=1&StartTimeTicks=0&EnableRedirection=true&EnableRemoteMedia=false" |
		tr -d '\r' | sed -E 's/([Aa]pi_?[Kk]ey)=[^&]*/\1=<redacted>/g' >"$raw/universal-headers.txt"

	# A song's playback, reported as jellyfin-web reports it, then the user
	# data and the activity log entry it leaves.
	report() { post "$base/Sessions/Playing$1" --header "Authorization: $signed" --data "$2"; }
	report '' "{\"ItemId\":\"$duet\",\"MediaSourceId\":\"$duet\",\"PlayMethod\":\"DirectPlay\",\"PositionTicks\":0}"
	report /Progress "{\"ItemId\":\"$duet\",\"MediaSourceId\":\"$duet\",\"PositionTicks\":50000000}"
	report /Stopped "{\"ItemId\":\"$duet\",\"MediaSourceId\":\"$duet\",\"PositionTicks\":50000000}"
	get "/Users/$user/Items/$duet" | jq --sort-keys '.UserData | del(.ItemId, .Key, .LastPlayedDate)' >"$raw/reported-user-data.json"
	get "/System/ActivityLog/Entries?limit=10" | jq --sort-keys '[.Items[] | select(.Type | startswith("VideoPlayback") or startswith("AudioPlayback")) | {Name, Type}]' \
		>"$raw/activity.json"

	# An audiobook's position is kept, unlike a song's.
	report '' "{\"ItemId\":\"$book\",\"MediaSourceId\":\"$book\",\"PlayMethod\":\"DirectPlay\",\"PositionTicks\":0}"
	report /Stopped "{\"ItemId\":\"$book\",\"MediaSourceId\":\"$book\",\"PositionTicks\":150000000}"
	get "/Users/$user/Items/$book" | jq --sort-keys '{UserData: (.UserData | del(.ItemId, .Key, .LastPlayedDate)), Chapters: [.Chapters[] | {Name, StartPositionTicks}]}' \
		>"$raw/audiobook-state.json"

	jq --sort-keys . <<<"$statuses" >"$raw/statuses.json"
	jq --sort-keys . <<<"$bodies" >"$raw/bodies.json"
	echo "Music fixtures written to $music_out"
}
