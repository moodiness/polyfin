#!/usr/bin/env bash
# Usage: scripts/packaging/windows.sh
#
# Builds dist/packages/Polyfin-Desktop-<version>-windows-x64-setup.exe, the
# installer of polyfin.nsi, on Linux or macOS: the programs cross-compiled
# with Go (polyfin-desktop.exe without a console), PostgreSQL 18's bin, lib
# and share from EnterpriseDB's build, with the Visual C++ runtime they
# load beside them, jellyfin-web, the fonts and the licenses. Needs Go,
# Node.js (for the admin app), Python 3, and Docker, whose Debian container
# unpacks the archives and runs NSIS. Windows on Arm has no package:
# EnterpriseDB builds PostgreSQL for x64 only.
source "$(dirname "$0")/common.sh"

stage="$WORK/windows-x64"
install="$stage/install"
rm -rf "$stage"
mkdir -p "$install"

build_web
build_server windows amd64 "$install/polyfin.exe"
build_desktop windows amd64 "$install/polyfin-desktop.exe" 0 "-H windowsgui"
python3 "$PACKAGING/icons.py" ico "$install/polyfin.ico"
install_shared "$install"
edb="$(fetch "$EDB_WINDOWS_URL" "$EDB_WINDOWS_SHA256" "$(basename "$EDB_WINDOWS_URL")")"
redist="$(fetch "$VC_REDIST_URL" "$VC_REDIST_SHA256" vc_redist.x64.exe)"

setup="Polyfin-Desktop-$VERSION-windows-x64-setup.exe"
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
echo "$OUT/$setup"
