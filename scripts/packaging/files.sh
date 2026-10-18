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
source "$(dirname "$0")/common.sh"

dir="${1:-$WORK/files}"
log "Taking jellyfin-web and the fonts from the image's stages into $dir"
rm -rf "$dir"
docker buildx build --target desktop-files --output type=local,dest="$dir" "$ROOT"
test -f "$dir/jellyfin-web/index.html"
