#!/usr/bin/env bash
# Usage: scripts/packaging/linux.sh amd64|arm64
#
# Builds Polyfin Desktop's Linux packages into dist/packages:
#   polyfin-desktop_<version>_<arch>.deb, which installs the app in
#     /opt/polyfin, with /usr/bin/polyfin-desktop, a menu entry and its icon;
#   Polyfin-Desktop-<version>-linux-<x86_64|aarch64>.AppImage, the same
#     files in one executable that needs nothing installed.
# Both hold the install folder: polyfin-desktop, polyfin, PostgreSQL 18's
# bin, lib and share (built from source by postgresql-linux.Dockerfile, with
# ICU), jellyfin-web, the fonts and the licenses. Needs Go, Node.js (for
# the admin app), Python 3, and Docker with Buildx, which builds PostgreSQL
# for the architecture (under emulation when it is not the machine's) and
# runs Debian's packaging tools.
source "$(dirname "$0")/common.sh"

arch="${1:-}"
case "$arch" in
amd64) appimage_arch=x86_64 runtime_sha256="$APPIMAGE_RUNTIME_X86_64_SHA256" ;;
arm64) appimage_arch=aarch64 runtime_sha256="$APPIMAGE_RUNTIME_AARCH64_SHA256" ;;
*) echo "usage: $0 amd64|arm64" >&2; exit 2 ;;
esac

stage="$WORK/linux-$arch"
install="$stage/install"
rm -rf "$stage"
mkdir -p "$install/postgresql"

build_web
build_server linux "$arch" "$install/polyfin"
# The tray speaks D-Bus itself: no cgo.
build_desktop linux "$arch" "$install/polyfin-desktop" 0

log "Building PostgreSQL 18 for linux/$arch"
docker buildx build --platform "linux/$arch" -f "$PACKAGING/postgresql-linux.Dockerfile" \
	--output type=local,dest="$stage/postgresql" "$PACKAGING"
mv "$stage/postgresql/bin" "$stage/postgresql/lib" "$stage/postgresql/share" "$install/postgresql/"
mkdir -p "$install/licenses"
cp -R "$stage/postgresql/licenses/." "$install/licenses/"

install_shared "$install"
check_install "$install" ""

cp "$PACKAGING/polyfin-desktop.desktop" "$stage/"
cp "$ROOT/assets/polyfin.svg" "$stage/polyfin-desktop.svg"
python3 "$PACKAGING/icons.py" png 256 "$stage/polyfin-desktop.png"
cp "$(fetch "$APPIMAGE_RUNTIME_URL/runtime-$appimage_arch" "$runtime_sha256" "appimage-runtime-$appimage_arch")" "$stage/runtime"
cat > "$stage/copyright" << 'EOF'
Polyfin Desktop: Polyfin, a Jellyfin-compatible media server, without Docker.
Source: https://github.com/moodiness/polyfin

Polyfin is under the MIT License, in /opt/polyfin/licenses/polyfin/LICENSE.
The other programs it carries are under their own licenses, in
/opt/polyfin/licenses: PostgreSQL and ICU; jellyfin-web, unmodified, under
the GPL-2.0, whose source is attached to every release of Polyfin; the
DejaVu fonts; hls.js; and the LumaaGlaass theme.
EOF

deb="polyfin-desktop_${VERSION}_${arch}.deb"
appimage="Polyfin-Desktop-$VERSION-linux-$appimage_arch.AppImage"
log "Making $deb and $appimage"
# Debian's tools, in its container, on copies whose owner and modes the
# packages fix: root, and no file writable but by it.
docker run --rm -v "$stage:/stage:ro" -v "$OUT:/out" \
	-e VERSION="$VERSION" -e DEBIAN_VERSION="$(debian_version)" -e ARCH="$arch" -e DEB="$deb" -e APPIMAGE="$appimage" \
	-e OWNER="$(id -u):$(id -g)" "$DEBIAN_IMAGE" bash -euo pipefail -c '
apt-get update -qq
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends squashfs-tools desktop-file-utils > /dev/null
desktop-file-validate /stage/polyfin-desktop.desktop

root=/tmp/deb
mkdir -p $root/opt $root/usr/bin $root/usr/share/applications $root/usr/share/icons/hicolor/scalable/apps \
	$root/usr/share/icons/hicolor/256x256/apps $root/usr/share/doc/polyfin-desktop $root/DEBIAN
cp -R /stage/install $root/opt/polyfin
ln -s /opt/polyfin/polyfin-desktop $root/usr/bin/polyfin-desktop
cp /stage/polyfin-desktop.desktop $root/usr/share/applications/
cp /stage/polyfin-desktop.svg $root/usr/share/icons/hicolor/scalable/apps/
cp /stage/polyfin-desktop.png $root/usr/share/icons/hicolor/256x256/apps/
cp /stage/copyright $root/usr/share/doc/polyfin-desktop/
cat > $root/DEBIAN/control << EOF
Package: polyfin-desktop
Version: $DEBIAN_VERSION
Architecture: $ARCH
Maintainer: Polyfin <moodiness@users.noreply.github.com>
Installed-Size: $(du -sk --exclude=DEBIAN $root | cut -f1)
Depends: libc6 (>= 2.35), libstdc++6, zlib1g
Recommends: xdg-utils
Section: video
Priority: optional
Homepage: https://github.com/moodiness/polyfin
Description: Polyfin, a Jellyfin-compatible media server, without Docker
 Polyfin Desktop runs Polyfin on this computer from an icon in the tray,
 with a PostgreSQL 18 database of its own, and FFmpeg downloaded at its
 first start. It opens the admin app in the browser, at
 http://localhost:8096/admin/.
EOF
chmod -R u+rwX,go+rX,go-w $root
dpkg-deb --root-owner-group -Zxz --build $root /out/$DEB > /dev/null

appdir=/tmp/AppDir
cp -R /stage/install $appdir
ln -s polyfin-desktop $appdir/AppRun
sed "\$aX-AppImage-Version=$VERSION" /stage/polyfin-desktop.desktop > $appdir/polyfin-desktop.desktop
desktop-file-validate $appdir/polyfin-desktop.desktop
cp /stage/polyfin-desktop.svg $appdir/
cp /stage/polyfin-desktop.png $appdir/.DirIcon
chmod -R u+rwX,go+rX,go-w $appdir
mksquashfs $appdir /tmp/AppDir.squashfs -root-owned -noappend -comp zstd -quiet > /dev/null
cat /stage/runtime /tmp/AppDir.squashfs > /out/$APPIMAGE
chmod 755 /out/$APPIMAGE
chown "$OWNER" /out/$DEB /out/$APPIMAGE
'
echo "$OUT/$deb"
echo "$OUT/$appimage"
