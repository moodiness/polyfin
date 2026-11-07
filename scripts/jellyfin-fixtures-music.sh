# Music and audiobook fixtures, sourced by jellyfin-fixtures.sh: music_media
# makes the files before the server starts, music_fixtures records the
# answers once everything else is, on libraries of their own so that no
# fixture above sees them. Shapes go to music/ like the others.

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
	local music_out="$out/music"
	mkdir -p "$music_out"
	msave() { jq --sort-keys "$shape" >"$music_out/$1.json"; }
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
	local music album artist song book
	music=$(view music)
	album=$(get "/Items?userId=$user&recursive=true&includeItemTypes=MusicAlbum&searchTerm=Sine" | jq --exit-status --raw-output '.Items[0].Id')
	artist=$(get "/Artists?userId=$user&searchTerm=Tone" | jq --exit-status --raw-output '.Items[0].Id')
	song=$(get "/Items?userId=$user&recursive=true&includeItemTypes=Audio&searchTerm=First" | jq --exit-status --raw-output '.Items[0].Id')
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
	get "/Users/$user/Items/$book" | msave audiobook

	# Search, the way jellyfin-web 12.2 asks for each kind, and the hints.
	get "/Users/$user/Items?searchTerm=wind&IncludeItemTypes=Audio&Recursive=true&Limit=24&Fields=PrimaryImageAspectRatio,CanDelete,MediaSourceCount&ImageTypeLimit=1&EnableTotalRecordCount=false" | msave search-songs
	get "/Users/$user/Items?searchTerm=sine&IncludeItemTypes=MusicAlbum&Recursive=true&Limit=24&Fields=PrimaryImageAspectRatio,CanDelete,MediaSourceCount&ImageTypeLimit=1&EnableTotalRecordCount=false" | msave search-albums
	get "/Artists?userId=$user&searchTerm=tone&Limit=24&Fields=PrimaryImageAspectRatio,CanDelete,MediaSourceCount&ImageTypeLimit=1&EnableTotalRecordCount=false" | msave search-artists
	get "/Search/Hints?userId=$user&searchTerm=wind&includeItemTypes=Audio,MusicAlbum,MusicArtist&limit=10" | msave search-hints

	# An instant mix from a song.
	get "/Items/$song/InstantMix?userId=$user&limit=20&Fields=PrimaryImageAspectRatio" | msave instant-mix

	# Playback of a FLAC song as jellyfin-web 12.2 on Chrome asks it.
	local profile
	profile=$(jq --compact-output . "$out/playback/profiles/jellyfin-web-chrome.json")
	audio_info() {
		post "$base/Items/$1/PlaybackInfo?userId=$user" --header "Authorization: $signed" \
			--data "$(jq --null-input --compact-output --arg user "$user" --arg id "$1" --argjson profile "$profile" --argjson options "$2" \
				'$options + {UserId: $user, MediaSourceId: $id, DeviceProfile: $profile, AutoOpenLiveStream: true, IsPlayback: true}')"
	}
	audio_info "$song" '{}' | msave playback-info
	echo "Music fixtures written to $music_out"
}
