#!/usr/bin/env bash
# Usage: scripts/packaging/files.sh [folder]
#
# Makes the files every package of Polyfin Desktop takes from the Docker
# image, through the Dockerfile's desktop-files stage: jellyfin-web, from
# the image's pinned jellyfin/jellyfin, unmodified, the fallback fonts, and
# their licenses. Needs Docker with Buildx. The other scripts run it when
# POLYFIN_DESKTOP_FILES names no folder made by it, such as on a Mac
# without Docker, where CI gives them the folder its Linux job made.
# The folder is dist/packaging/files by default.
#
# POLYFIN_DESKTOP_FILES_CACHE, a buildx cache such as CI's
# type=gha,scope=desktop-files, gives the build its layers and keeps them,
# in full (mode=max); a failure to keep them fails nothing.
source "$(dirname "$0")/common.sh"

dir="${1:-$WORK/files}"
cache=()
if [ -n "${POLYFIN_DESKTOP_FILES_CACHE:-}" ]; then
	cache=(--cache-from "$POLYFIN_DESKTOP_FILES_CACHE" --cache-to "$POLYFIN_DESKTOP_FILES_CACHE,mode=max,ignore-error=true")
fi
log "Taking jellyfin-web and the fonts from the image's stages into $dir"
rm -rf "$dir"
docker buildx build ${cache[@]+"${cache[@]}"} --target desktop-files --output type=local,dest="$dir" "$ROOT"
test -f "$dir/jellyfin-web/index.html"
