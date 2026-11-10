#!/usr/bin/env bash
# The FFmpeg archives the Dockerfile builds the image with, one for each
# platform: the stage ffmpeg-<platform>-archive downloads it, and the stage
# ffmpeg-<platform> checks it against its SHA-256. CI keeps them in its
# cache, under their URLs and SHA-256, and gives them to its builds in
# place of the download stages, so that an outage of the download fails no
# run (see .github/workflows/ci.yml):
#   docker buildx build --build-context ffmpeg-amd64-archive=DIR/amd64 ...
#
# Without arguments, prints each archive as "platform URL SHA-256", read
# from the Dockerfile. With fetch DIR, downloads into DIR/<platform>/
# ffmpeg.tar.xz each archive DIR lacks, or holds with another SHA-256, and
# fails on a download that differs from it.
#
# Requirements: awk, curl, sha256sum or shasum.
# Usage: scripts/ffmpeg-archives.sh [fetch DIR]
set -euo pipefail

dockerfile="$(cd "$(dirname "$0")/.." && pwd)/Dockerfile"
if command -v sha256sum >/dev/null; then
	sha256() { sha256sum "$1" | cut -d' ' -f1; }
else
	sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
fi

# archives prints the lines, from the URL of each download stage and the
# SHA-256 of the stage after it.
archives() {
	local platform url sum
	for platform in amd64 arm64; do
		url=$(awk -v stage="ffmpeg-$platform-archive" \
			'$1 == "FROM" { f = $NF == stage; next } f && $1 ~ /^https:/ { print $1 }' "$dockerfile")
		sum=$(awk -v stage="ffmpeg-$platform" \
			'$1 == "FROM" { f = $NF == stage; next } f && $1 == "RUN" && $2 == "echo" { print substr($3, 2) }' "$dockerfile")
		if ! [[ $url =~ ^https://[^[:space:]]+$ && $sum =~ ^[0-9a-f]{64}$ ]]; then
			echo "the Dockerfile's stages ffmpeg-$platform-archive and ffmpeg-$platform name no single URL and SHA-256" >&2
			exit 1
		fi
		echo "$platform $url $sum"
	done
}

case "${1:-}" in
'') archives ;;
fetch)
	[[ $# == 2 ]] || { echo "usage: $0 [fetch DIR]" >&2; exit 2; }
	list=$(archives)
	while read -r platform url sum; do
		file="$2/$platform/ffmpeg.tar.xz"
		if [[ -f $file && $(sha256 "$file") == "$sum" ]]; then continue; fi
		mkdir -p "$2/$platform"
		echo "Downloading $url" >&2
		curl -fsSL --retry 5 --retry-all-errors --retry-delay 10 -o "$file.partial" "$url"
		got=$(sha256 "$file.partial")
		if [[ $got != "$sum" ]]; then
			rm -f "$file.partial"
			echo "SHA-256 of $url: $got, expected $sum" >&2
			exit 1
		fi
		mv "$file.partial" "$file"
	done <<<"$list"
	;;
*)
	echo "usage: $0 [fetch DIR]" >&2
	exit 2
	;;
esac
