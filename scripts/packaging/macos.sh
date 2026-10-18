#!/usr/bin/env bash
# Usage: scripts/packaging/macos.sh arm64|x64
#
# Builds dist/packages/Polyfin-Desktop-<version>-macos-<arch>.dmg on a Mac:
# a disk image holding Polyfin.app, and a link to Applications to drag it
# to. The app's executable is polyfin-desktop, built with cgo for the menu
# bar; its Contents/Resources is the install folder: polyfin, PostgreSQL
# 18's bin, lib and share from EnterpriseDB's build, jellyfin-web, the
# fonts and the licenses. Needs Go, Node.js (for the admin app), Python 3,
# and Docker unless POLYFIN_DESKTOP_FILES names the folder of files.sh.
# The app is signed ad hoc, as Apple silicon runs nothing unsigned: no
# certificate, so macOS asks before opening it the first time.
source "$(dirname "$0")/common.sh"

arch="${1:-}"
case "$arch" in
arm64) goarch=arm64 clang_arch=arm64 ;;
x64) goarch=amd64 clang_arch=x86_64 ;;
*) echo "usage: $0 arm64|x64" >&2; exit 2 ;;
esac
# EnterpriseDB's PostgreSQL 18 runs from macOS 13.
minimum=13.0

stage="$WORK/macos-$arch"
app="$stage/Polyfin.app"
resources="$app/Contents/Resources"
rm -rf "$stage"
mkdir -p "$app/Contents/MacOS" "$resources/postgresql" "$resources/licenses/postgresql"

build_web
build_server darwin "$goarch" "$resources/polyfin"
# The menu bar needs cgo; clang builds for either architecture.
CC="clang -arch $clang_arch" MACOSX_DEPLOYMENT_TARGET="$minimum" \
	build_desktop darwin "$goarch" "$app/Contents/MacOS/polyfin-desktop" 1
lipo "$app/Contents/MacOS/polyfin-desktop" -verify_arch "$clang_arch"

log "Taking PostgreSQL 18 from EnterpriseDB's build"
zip="$(fetch "$EDB_MACOS_URL" "$EDB_MACOS_SHA256" "$(basename "$EDB_MACOS_URL")")"
rm -rf "$stage/edb"
unzip -q "$zip" 'pgsql/bin/*' 'pgsql/lib/*' 'pgsql/share/*' 'pgsql/server_license.txt' \
	'pgsql/commandlinetools_3rd_party_licenses.txt' -d "$stage/edb"
mv "$stage/edb/pgsql/bin" "$stage/edb/pgsql/lib" "$stage/edb/pgsql/share" "$resources/postgresql/"
mv "$stage/edb/pgsql/server_license.txt" "$stage/edb/pgsql/commandlinetools_3rd_party_licenses.txt" "$resources/licenses/postgresql/"
rm -rf "$stage/edb"
# What only builds against PostgreSQL goes; so do the copies of a library
# under its other names, which the archive holds as files: each becomes a
# link to the one kept, as the system's installs have them.
(
	cd "$resources/postgresql/lib" || exit 1
	rm -rf pgxs pkgconfig ./*.a
	previous_sum='' previous_name=''
	shasum -a 256 ./*.dylib | sort | while read -r sum name; do
		name="${name#./}"
		if [ "$sum" = "$previous_sum" ]; then
			ln -sf "$previous_name" "$name"
		else
			previous_sum="$sum" previous_name="$name"
		fi
	done
)
rm -rf "$resources/postgresql/share/man" "$resources/postgresql/share/doc"

install_shared "$resources"
check_install "$resources" ""

log "Making the app bundle"
sed -e "s/@VERSION@/$VERSION/" -e "s/@NUMERIC_VERSION@/$(numeric_version)/" -e "s/@MINIMUM_MACOS@/$minimum/" \
	"$PACKAGING/Info.plist" > "$app/Contents/Info.plist"
plutil -lint "$app/Contents/Info.plist" > /dev/null
printf 'APPL????' > "$app/Contents/PkgInfo"
python3 "$PACKAGING/icons.py" iconset "$stage/polyfin.iconset"
iconutil -c icns "$stage/polyfin.iconset" -o "$resources/polyfin.icns"
# Ad hoc: the bundle's files are sealed, and EnterpriseDB's programs keep
# their own signatures.
codesign --force --sign - --timestamp=none "$app"
codesign --verify --deep --strict "$app"

log "Making the disk image"
mkdir "$stage/dmg"
mv "$app" "$stage/dmg/"
ln -s /Applications "$stage/dmg/Applications"
dmg="$OUT/Polyfin-Desktop-$VERSION-macos-$arch.dmg"
rm -f "$dmg"
hdiutil create -quiet -volname Polyfin -srcfolder "$stage/dmg" -fs HFS+ -format ULFO "$dmg"
echo "$dmg"
