# Shared by the packaging scripts of Polyfin Desktop: sourced, not run.
#
# Variables a script reads:
#   VERSION                 the version stamped in the programs and the
#                           package names, "dev" by default; a release's,
#                           such as 0.42.0, in CI.
#   POLYFIN_PACKAGING_CACHE where downloads are kept, checked against their
#                           checksums, ~/.cache/polyfin-packaging by default.
#   POLYFIN_DESKTOP_FILES   a folder made by files.sh, with jellyfin-web, the
#                           fonts and their licenses: made with Docker when
#                           unset.
#   SKIP_WEB                1 to use the admin app already in web/dist
#                           rather than building it.
#   DOCKER_HUB_MIRROR       a mirror of Docker Hub to pull Debian's image
#                           from, such as mirror.gcr.io.
# Packages go to dist/packages, the work to dist/packaging.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PACKAGING="$ROOT/scripts/packaging"
VERSION="${VERSION:-dev}"
CACHE="${POLYFIN_PACKAGING_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/polyfin-packaging}"
OUT="$ROOT/dist/packages"
WORK="$ROOT/dist/packaging"

# Windows names Python python; python3 may be the Store's stub there. The
# folders CI names there become Git Bash paths, which tar takes: it reads
# D:\a\file as a file on a host named D.
PYTHON=python3
if [ "${OS:-}" = Windows_NT ]; then
	PYTHON=python
	CACHE="$(cygpath -u "$CACHE")"
	if [ -n "${POLYFIN_DESKTOP_FILES:-}" ]; then POLYFIN_DESKTOP_FILES="$(cygpath -u "$POLYFIN_DESKTOP_FILES")"; fi
fi
mkdir -p "$CACHE" "$OUT" "$WORK"

# Pinned inputs, each checked against its SHA-256 once downloaded.
#
# PostgreSQL 18's server programs for Windows x64 and macOS: EnterpriseDB's
# binaries, which postgresql.org links, as CI's desktop job tests them. The
# macOS build is universal (Apple silicon and Intel). Both carry ICU.
# Windows on Arm's is built from source by postgresql-windows.sh.
EDB_WINDOWS_URL=https://get.enterprisedb.com/postgresql/postgresql-18.6-2-windows-x64-binaries.zip
EDB_WINDOWS_SHA256=41bb1496f60666d3745ae8a855753b877d14fa9e34667f266867635b790fc323
EDB_MACOS_URL=https://get.enterprisedb.com/postgresql/postgresql-18.6-2-osx-binaries.zip
EDB_MACOS_SHA256=21c0432b3cb45ab763aab2b4efe9edfa91f658c3be09cddf272c7b52eb2aa3fc
# Microsoft's Visual C++ runtime, which EnterpriseDB's programs load and a
# fresh Windows lacks: its three DLLs are taken from the redistributable
# and put beside them, as Microsoft allows.
VC_REDIST_URL=https://download.visualstudio.microsoft.com/download/pr/990c720a-46ce-4dc7-87b9-c2eb615d34b9/CC0FF0EB1DC3F5188AE6300FAEF32BF5BEEBA4BDD6E8E445A9184072096B713B/VC_redist.x64.exe
VC_REDIST_SHA256=cc0ff0eb1dc3f5188ae6300faef32bf5beeba4bdd6e8e445a9184072096b713b
# The runtime an AppImage starts with: static, it needs no libfuse2.
APPIMAGE_RUNTIME_URL=https://github.com/AppImage/type2-runtime/releases/download/20251108
APPIMAGE_RUNTIME_X86_64_SHA256=2fca8b443c92510f1483a883f60061ad09b46b978b2631c807cd873a47ec260d
APPIMAGE_RUNTIME_AARCH64_SHA256=00cbdfcf917cc6c0ff6d3347d59e0ca1f7f45a6df1a428a0d6d8a78664d87444
# The Debian of the Dockerfile, whose packages make the .deb, the
# AppImage's file system and the Windows installer. DOCKER_HUB_MIRROR,
# such as mirror.gcr.io, pulls it from a mirror of Docker Hub, by the same
# digest.
DEBIAN_IMAGE="${DOCKER_HUB_MIRROR:-docker.io}/library/debian:trixie-slim@sha256:a99cfc517144bc59b1978475ec53b46ecabec7e43635402ee5b77cc54cd1b20a"

log() { printf '== %s\n' "$*" >&2; }

sha256() {
	if command -v sha256sum > /dev/null; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

# fetch URL SHA256 NAME: the path of the download in the cache, fetched
# once, refused when its checksum differs.
fetch() {
	local url="$1" want="$2" file="$CACHE/$3"
	if [ ! -f "$file" ] || [ "$(sha256 "$file")" != "$want" ]; then
		log "Downloading $url"
		curl -fsSL --retry 3 -o "$file.partial" "$url"
		local got
		got="$(sha256 "$file.partial")"
		if [ "$got" != "$want" ]; then
			rm -f "$file.partial"
			echo "checksum of $url: $got, expected $want" >&2
			exit 1
		fi
		mv "$file.partial" "$file"
	fi
	printf '%s\n' "$file"
}

# The admin app, which the server embeds.
build_web() {
	if [ "${SKIP_WEB:-}" = 1 ]; then
		test -f "$ROOT/web/dist/index.html" || { echo "SKIP_WEB=1 but web/dist has no build" >&2; exit 1; }
		return
	fi
	log "Building the admin app"
	npm --prefix "$ROOT/web" ci --no-audit --no-fund --silent
	npm --prefix "$ROOT/web" run build --silent
}

# build_server GOOS GOARCH OUTPUT: polyfin, as the Dockerfile builds it.
build_server() {
	log "Building polyfin for $1/$2"
	(cd "$ROOT" && CGO_ENABLED=0 GOOS="$1" GOARCH="$2" go build -tags production -trimpath \
		-ldflags "-s -w -X main.version=$VERSION" -o "$3" ./cmd/polyfin)
}

# build_desktop GOOS GOARCH OUTPUT [CGO_ENABLED] [extra -ldflags]: the app.
build_desktop() {
	log "Building polyfin-desktop for $1/$2"
	(cd "$ROOT" && CGO_ENABLED="${4:-0}" GOOS="$1" GOARCH="$2" go build -trimpath \
		-ldflags "-s -w -X main.version=$VERSION ${5:-}" -o "$3" ./cmd/polyfin-desktop)
}

# shared_files: the folder of jellyfin-web, the fonts and their licenses,
# made by files.sh unless POLYFIN_DESKTOP_FILES names one.
shared_files() {
	local dir="${POLYFIN_DESKTOP_FILES:-$WORK/files}"
	if [ -z "${POLYFIN_DESKTOP_FILES:-}" ]; then
		"$PACKAGING/files.sh" "$dir" >&2
	fi
	for want in jellyfin-web/index.html fonts licenses/jellyfin-web/LICENSE; do
		test -e "$dir/$want" || { echo "$dir lacks $want: run scripts/packaging/files.sh" >&2; exit 1; }
	done
	printf '%s\n' "$dir"
}

# install_shared DEST: jellyfin-web, the fonts and the licenses, into the
# install folder DEST, with Polyfin's own license and hls.js's, which the
# server embeds.
install_shared() {
	local files
	files="$(shared_files)"
	mkdir -p "$1/licenses/polyfin" "$1/licenses/hls.js"
	cp -R "$files/jellyfin-web" "$files/fonts" "$1/"
	cp -R "$files/licenses/." "$1/licenses/"
	cp "$ROOT/LICENSE" "$1/licenses/polyfin/"
	cp "$ROOT/third_party/hls.js/LICENSE" "$ROOT/third_party/hls.js/NOTICE" "$1/licenses/hls.js/"
}

# check_install DIR EXE: DIR holds the install layout of docs/desktop.md,
# EXE being the executable suffix.
check_install() {
	local missing=0
	for want in "polyfin$2" "postgresql/bin/postgres$2" "postgresql/bin/initdb$2" "postgresql/bin/pg_ctl$2" \
		"postgresql/bin/pg_dump$2" "postgresql/bin/pg_restore$2" \
		jellyfin-web/index.html fonts/truetype/dejavu/DejaVuSans.ttf licenses/jellyfin-web/LICENSE \
		licenses/jellyfin-web/NOTICE licenses/postgresql licenses/polyfin/LICENSE; do
		if [ ! -e "$1/$want" ]; then
			echo "the install folder $1 lacks $want" >&2
			missing=1
		fi
	done
	# share is flat, or in share/postgresql, as PostgreSQL's prefix had it.
	if [ ! -e "$1/postgresql/share/postgres.bki" ] && [ ! -e "$1/postgresql/share/postgresql/postgres.bki" ]; then
		echo "the install folder $1 lacks PostgreSQL's share" >&2
		missing=1
	fi
	return "$missing"
}

# debian_version: VERSION as a Debian version: a pre-release sorts before
# its release, and a development build before any release.
debian_version() {
	case "$VERSION" in
	[0-9]*) printf '%s\n' "${VERSION/-/\~}" ;;
	*) printf '0~%s\n' "$VERSION" ;;
	esac
}

# numeric_version: the numbers of VERSION, for macOS's and Windows' fields
# that take only them; 0.0.0 for a development build.
numeric_version() {
	case "$VERSION" in
	[0-9]*) printf '%s\n' "${VERSION%%-*}" ;;
	*) printf '0.0.0\n' ;;
	esac
}
