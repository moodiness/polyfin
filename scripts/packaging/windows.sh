#!/usr/bin/env bash
# Usage: scripts/packaging/windows.sh [x64|arm64]
#
# Builds dist/packages/Polyfin-Desktop-<version>-windows-<arch>-setup.exe,
# for x64 (the default) or arm64, the installer of polyfin.nsi: the
# programs built with Go (polyfin-desktop.exe without a console),
# PostgreSQL 18's bin, lib and share, with the Visual C++ runtime they load
# beside them, jellyfin-web, the fonts and the licenses. Needs Go, Node.js
# (for the admin app) and Python 3, and:
#   x64    Linux or macOS, with Docker, whose Debian container unpacks
#          EnterpriseDB's build of PostgreSQL and the runtime's
#          redistributable, and runs NSIS;
#   arm64  Windows on Arm, in Git Bash, with NSIS, and what
#          postgresql-windows.sh needs to build PostgreSQL, for which
#          EnterpriseDB makes no build: its folder,
#          dist/packaging/postgresql-windows-arm64, is kept between runs
#          while the build's inputs stay the same. Docker is needed only
#          when POLYFIN_DESKTOP_FILES names no folder of files.sh.
source "$(dirname "$0")/common.sh"

arch="${1:-x64}"
case "$arch" in
x64) goarch=amd64 ;;
arm64)
	goarch=arm64
	if [ "${OS:-}" != Windows_NT ]; then echo "$0 arm64 runs on Windows on Arm, in Git Bash" >&2; exit 2; fi
	;;
*) echo "usage: $0 [x64|arm64]" >&2; exit 2 ;;
esac

stage="$WORK/windows-$arch"
install="$stage/install"
rm -rf "$stage"
mkdir -p "$install"

build_web
build_server windows "$goarch" "$install/polyfin.exe"
build_desktop windows "$goarch" "$install/polyfin-desktop.exe" 0 "-H windowsgui"
"$PYTHON" "$PACKAGING/icons.py" ico "$install/polyfin.ico"
install_shared "$install"
setup="Polyfin-Desktop-$VERSION-windows-$arch-setup.exe"

# PostgreSQL built from source, and NSIS, on Windows on Arm.
package_arm64() {
	local postgresql="$WORK/postgresql-windows-arm64" makensis
	"$PACKAGING/postgresql-windows.sh" "$postgresql"
	mkdir -p "$install/postgresql" "$install/licenses"
	cp -R "$postgresql/bin" "$postgresql/lib" "$postgresql/share" "$install/postgresql/"
	cp -R "$postgresql/licenses/." "$install/licenses/"
	check_install "$install" .exe
	makensis="$(command -v makensis || echo "/c/Program Files (x86)/NSIS/makensis.exe")"
	log "Making $setup"
	# Windows paths, which Git Bash leaves as they are with
	# MSYS_NO_PATHCONV.
	MSYS_NO_PATHCONV=1 "$makensis" -V2 -DVERSION="$VERSION" -DNUMERIC_VERSION="$(numeric_version)" \
		-DSOURCE="$(cygpath -w "$install")" -DICON="$(cygpath -w "$install/polyfin.ico")" \
		-DOUTFILE="$(cygpath -w "$OUT/$setup")" "$(cygpath -w "$PACKAGING/polyfin.nsi")"
}

# EnterpriseDB's PostgreSQL, and NSIS, in Debian's container.
package_x64() {
	local edb redist
	edb="$(fetch "$EDB_WINDOWS_URL" "$EDB_WINDOWS_SHA256" "$(basename "$EDB_WINDOWS_URL")")"
	redist="$(fetch "$VC_REDIST_URL" "$VC_REDIST_SHA256" vc_redist.x64.exe)"
	log "Making $setup"
	docker run --rm -v "$ROOT:/repo:ro" -v "$stage:/stage" -v "$OUT:/out" \
		-v "$edb:/downloads/postgresql.zip:ro" -v "$redist:/downloads/vc_redist.x64.exe:ro" \
		-e VERSION="$VERSION" -e SETUP="$setup" -e OWNER="$(id -u):$(id -g)" -e POLYFIN_PACKAGING_CACHE=/tmp/cache \
		"$DEBIAN_IMAGE" bash -euo pipefail -c '
apt-get update -qq
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends unzip cabextract nsis > /dev/null
install=/stage/install

unzip -q /downloads/postgresql.zip "pgsql/bin/*" "pgsql/lib/*" "pgsql/share/*" "pgsql/server_license.txt" \
	"pgsql/commandlinetools_3rd_party_licenses.txt" -d /tmp/edb
mkdir -p $install/postgresql $install/licenses/postgresql
mv /tmp/edb/pgsql/bin /tmp/edb/pgsql/lib /tmp/edb/pgsql/share $install/postgresql/
mv /tmp/edb/pgsql/server_license.txt /tmp/edb/pgsql/commandlinetools_3rd_party_licenses.txt $install/licenses/postgresql/
# What only builds against PostgreSQL goes.
rm -rf $install/postgresql/lib/pgxs $install/postgresql/lib/pkgconfig $install/postgresql/share/doc
rm -f $install/postgresql/lib/*.lib $install/postgresql/lib/*.a

# The redistributable holds cabinets, one of which has the DLLs, named
# after their architecture.
cabextract -q -d /tmp/redist /downloads/vc_redist.x64.exe
for cab in /tmp/redist/a*; do cabextract -q -d /tmp/crt -F "*140*.dll_amd64" "$cab" 2> /dev/null || true; done
for dll in msvcp140.dll vcruntime140.dll vcruntime140_1.dll; do
	cp /tmp/crt/${dll}_amd64 $install/postgresql/bin/$dll
done

source /repo/scripts/packaging/common.sh
check_install $install .exe
numeric="$(numeric_version)"
makensis -V2 -DVERSION="$VERSION" -DNUMERIC_VERSION="$numeric" -DSOURCE=$install -DICON=$install/polyfin.ico \
	-DOUTFILE=/out/$SETUP /repo/scripts/packaging/polyfin.nsi
chown -R "$OWNER" /out/$SETUP /stage
'
}

"package_$arch"
echo "$OUT/$setup"
