#!/usr/bin/env bash
# Usage: scripts/packaging/postgresql-windows.sh [folder]
#
# Builds PostgreSQL 18 for Windows on Arm (arm64) into folder,
# dist/packaging/postgresql-windows-arm64 by default: bin, lib, share and
# licenses, laid out as EnterpriseDB's Windows build has them, which
# windows.sh arm64 puts in the installer. A folder that already holds a
# build of this script's inputs is left as it is.
#
# Why a build of its own: EnterpriseDB, whose build the x64 package
# carries, makes none for Windows on Arm. This one starts from the official
# source postgresql-linux.Dockerfile builds, with upstream's commit
# a516b3f00d74 "MSVC: Support building for AArch64" applied
# (postgresql-msvc-arm64.patch, unchanged), which PostgreSQL 18 lacks: an
# ISB to spin with, and a spinlock release that is a barrier on Arm too.
# Built with Meson and Visual Studio's ARM64 compiler, with ICU (the app's
# cluster sorts with ICU's root collation), from ICU's own ARM64 build, and
# zlib (pg_dump's custom format is compressed with it, as the Docker
# image's backups are), linked in. The Visual C++ runtime that PostgreSQL
# and ICU load goes beside them, from the Visual Studio that built them.
# The build then makes a cluster with ICU, and backs a database up with
# pg_dump and restores it with pg_restore.
#
# Runs on Windows on Arm, in Git Bash, with Visual Studio 2022's ARM64
# build tools, Python 3, a native Perl (such as Strawberry Perl) and Ninja,
# as GitHub's windows-11-arm runners have them; Visual Studio is found with
# vswhere. Every download is checked against its checksum.
source "$(dirname "$0")/common.sh"

# Pinned inputs. PostgreSQL's source is postgresql-linux.Dockerfile's.
POSTGRESQL_URL=https://ftp.postgresql.org/pub/source/v18.6/postgresql-18.6.tar.bz2
POSTGRESQL_SHA256=555610c24d53e4316da5b7d3fc25c279d96856d5e0e23ee308c328c5fa881d9f
# ICU's own build for Windows on Arm, made with Visual Studio 2022.
ICU_URL=https://github.com/unicode-org/icu/releases/download/release-78.3/icu4c-78.3-WinARM64-MSVC2022.zip
ICU_SHA256=34fedefb5aa7e3a31e77a8c4befd33b0020c3cc8056ec7d73550f2eeb40b3d85
ZLIB_URL=https://github.com/madler/zlib/releases/download/v1.3.2/zlib-1.3.2.tar.gz
ZLIB_SHA256=bb329a0a2cd0274d05519d61c667c062e06990d72e125ee2dfa8de64f0119d16
# Bison and flex for Windows, which PostgreSQL's grammar needs.
WINFLEXBISON_URL=https://github.com/lexxmark/winflexbison/releases/download/v2.5.25/win_flex_bison-2.5.25.zip
WINFLEXBISON_SHA256=8d324b62be33604b2c45ad1dd34ab93d722534448f55a16ca7292de32b6ac135
MESON_URL=https://files.pythonhosted.org/packages/57/99/36cd25f9598eef70db97577440a70a360af505f6f01c2338a18d1556079c/meson-1.12.1-py3-none-any.whl
MESON_SHA256=930bc7542cbd9f57009e182fd014eba48cf1a0180a7b9006d2d32d8f168d8b02

out="${1:-$WORK/postgresql-windows-arm64}"
patch="$PACKAGING/postgresql-msvc-arm64.patch"
grep -q "$POSTGRESQL_SHA256" "$PACKAGING/postgresql-linux.Dockerfile" \
	|| { echo "postgresql-linux.Dockerfile builds another source than $POSTGRESQL_URL" >&2; exit 1; }
inputs="$(sha256 "$PACKAGING/postgresql-windows.sh") $(sha256 "$patch")"
if [ -f "$out/build-inputs" ] && [ "$(cat "$out/build-inputs")" = "$inputs" ]; then
	log "PostgreSQL for Windows arm64 is already built in $out"
	exit 0
fi
if [ "${OS:-}" != Windows_NT ]; then
	echo "$0 runs on Windows on Arm, in Git Bash" >&2
	exit 2
fi

work="$WORK/postgresql-windows-arm64-build"
rm -rf "$work"
mkdir -p "$work"

# Visual Studio's environment for ARM64, unless it is set already: the
# variables vcvarsall.bat sets, which cmd prints after it.
msvc_environment() {
	local vswhere="/c/Program Files (x86)/Microsoft Visual Studio/Installer/vswhere.exe" vs bat line name value
	vs="$("$vswhere" -latest -products '*' -property installationPath | tr -d '\r')"
	test -n "$vs" || { echo "no Visual Studio found" >&2; exit 1; }
	bat="$work/msvc.bat"
	printf '@set > "%s"\r\n@call "%s\\VC\\Auxiliary\\Build\\vcvarsall.bat" arm64 > nul || exit /b 1\r\n@set\r\n' \
		"$(cygpath -w "$work/before.txt")" "$vs" > "$bat"
	MSYS_NO_PATHCONV=1 cmd /d /c "$(cygpath -w "$bat")" | tr -d '\r' > "$work/after.txt"
	tr -d '\r' < "$work/before.txt" | grep -v '^$' > "$work/before-lf.txt"
	while IFS= read -r line; do
		name="${line%%=*}" value="${line#*=}"
		[[ "$name" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue
		if [ "$name" = Path ] || [ "$name" = PATH ]; then
			PATH="$(cygpath -u -p "$value")"
			export PATH
		else
			export "$name=$value"
		fi
	done < <(grep -vxF -f "$work/before-lf.txt" "$work/after.txt")
}
if ! command -v cl > /dev/null || [ "${VSCMD_ARG_TGT_ARCH:-}" != arm64 ]; then
	msvc_environment
fi
if ! command -v cl > /dev/null || [ "${VSCMD_ARG_TGT_ARCH:-}" != arm64 ]; then
	echo "Visual Studio has no ARM64 build tools for this computer" >&2
	exit 1
fi
log "Building PostgreSQL 18 for Windows arm64 with $(cl 2>&1 | head -n1 | tr -d '\r')"

postgresql="$(fetch "$POSTGRESQL_URL" "$POSTGRESQL_SHA256" "$(basename "$POSTGRESQL_URL")")"
icu="$(fetch "$ICU_URL" "$ICU_SHA256" "$(basename "$ICU_URL")")"
zlib="$(fetch "$ZLIB_URL" "$ZLIB_SHA256" "$(basename "$ZLIB_URL")")"
winflexbison="$(fetch "$WINFLEXBISON_URL" "$WINFLEXBISON_SHA256" "$(basename "$WINFLEXBISON_URL")")"
meson_wheel="$(fetch "$MESON_URL" "$MESON_SHA256" "$(basename "$MESON_URL")")"

# ICU and zlib, laid out as a prefix that CMake's FindICU and FindZLIB,
# through Meson, find: include and lib.
deps="$work/deps"
mkdir -p "$deps/include" "$deps/lib" "$work/icu"
unzip -q "$icu" -d "$work/icu"
cp -R "$work/icu/include/." "$deps/include/"
cp "$work/icu/libARM64/"*.lib "$deps/lib/"

log "Building zlib"
mkdir -p "$work/zlib"
tar -xzf "$zlib" -C "$work/zlib" --strip-components=1
# The static library, linked into the programs, with the DLL runtime.
(cd "$work/zlib" && MSYS_NO_PATHCONV=1 nmake -nologo -f win32/Makefile.msc zlib.lib > /dev/null)
cp "$work/zlib/zlib.h" "$work/zlib/zconf.h" "$deps/include/"
cp "$work/zlib/zlib.lib" "$deps/lib/"

unzip -q "$winflexbison" -d "$work/winflexbison"
# Meson in a virtual environment of its own, from the wheel.
"$PYTHON" -m venv "$work/venv"
"$work/venv/Scripts/python.exe" -m pip install -q --no-index --no-deps "$(cygpath -w "$meson_wheel")"
meson="$work/venv/Scripts/meson.exe"
# A native Perl: Git's, built for MSYS, does not suit PostgreSQL's build.
perl=
while IFS= read -r candidate; do
	case "$candidate" in /usr/*) ;; *) perl="$candidate" && break ;; esac
done < <(type -ap perl)
test -n "$perl" || { echo "no native Perl, such as Strawberry Perl, in PATH" >&2; exit 1; }

src="$work/postgresql"
mkdir -p "$src"
tar -xjf "$postgresql" -C "$src" --strip-components=1
# git apply as patch: no repository above the source, which the work
# folder's would be.
(cd "$src" && GIT_CEILING_DIRECTORIES="$(cygpath -m "$work")" git apply "$(cygpath -m "$patch")")

# A prefix with "pgsql" in it keeps share flat. Only ICU and zlib: no
# OpenSSL (the cluster listens on the loopback only), no readline (no one
# types into psql here). Meson finds both through CMake, never a
# pkg-config of another toolchain's libraries.
prefix="$work/pgsql"
log "Configuring PostgreSQL"
CC=cl PKG_CONFIG=pkg-config-not-used ICU_ROOT="$(cygpath -w "$deps")" "$meson" setup "$(cygpath -w "$work/build")" "$(cygpath -w "$src")" \
	--prefix="$(cygpath -m "$prefix")" --buildtype=release --auto-features=disabled \
	-Dicu=enabled -Dzlib=enabled -Dssl=none -Dcmake_prefix_path="$(cygpath -m "$deps")" \
	-DPERL="$(cygpath -m "$perl")" -DBISON="$(cygpath -m "$work/winflexbison/win_bison.exe")" \
	-DFLEX="$(cygpath -m "$work/winflexbison/win_flex.exe")"
log "Compiling PostgreSQL"
"$meson" compile -C "$(cygpath -w "$work/build")" > "$work/compile.log" || { tail -n 100 "$work/compile.log" >&2; exit 1; }
"$meson" install -C "$(cygpath -w "$work/build")" --quiet

# What only builds against PostgreSQL goes. ICU's libraries PostgreSQL
# loads, common, i18n and data, and the runtime go beside the programs,
# where Windows looks first: the two DLLs ARM64 programs import.
# vcruntime140_1.dll, the x64 ABI's exception handling, is for x64 and
# ARM64EC code only, which a native ARM64 process does not load.
rm -rf "$prefix/include" "$prefix/lib/pgxs" "$prefix/lib/pkgconfig"
rm -f "$prefix/lib/"*.lib "$prefix/lib/"*.a "$prefix/lib/"*.pdb "$prefix/bin/"*.pdb
cp "$work/icu/binARM64/"icu{uc,in,dt}[0-9]*.dll "$prefix/bin/"
crt="$(cygpath -u "${VCToolsRedistDir:?is not set by Visual Studio}")/arm64"
for dll in msvcp140.dll vcruntime140.dll; do
	found=("$crt"/Microsoft.VC*.CRT/"$dll")
	test -f "${found[0]}" || { echo "Visual Studio's ARM64 runtime lacks $dll in $crt" >&2; exit 1; }
	cp "${found[0]}" "$prefix/bin/"
done
mkdir -p "$prefix/licenses/postgresql" "$prefix/licenses/icu" "$prefix/licenses/zlib"
cp "$src/COPYRIGHT" "$prefix/licenses/postgresql/"
cp "$work/icu/LICENSE" "$prefix/licenses/icu/"
cp "$work/zlib/LICENSE" "$prefix/licenses/zlib/"

# The cluster the app makes, with ICU's root collation, and a backup made
# with pg_dump, compressed with zlib, restored with pg_restore.
log "Checking the build: a cluster with ICU, and a backup restored"
bin="$prefix/bin"
check="$work/check"
port=55439
"$bin/postgres.exe" --version
"$bin/initdb.exe" -D "$(cygpath -w "$check/data")" -U polyfin -A trust -E UTF8 --locale=C \
	--locale-provider=icu --icu-locale=und --no-instructions > "$work/initdb.log" || { cat "$work/initdb.log" >&2; exit 1; }
"$bin/pg_ctl.exe" -D "$(cygpath -w "$check/data")" -l "$(cygpath -w "$check/postgresql.log")" \
	-o "-p $port -c listen_addresses=127.0.0.1" -w start > /dev/null
stop_check() { "$bin/pg_ctl.exe" -D "$(cygpath -w "$check/data")" -m fast -w stop > /dev/null || true; }
trap 'stop_check' EXIT
sql() { "$bin/psql.exe" -X -q -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -p "$port" -U polyfin -d "$@" | tr -d '\r'; }
order="$(sql postgres -c "SELECT string_agg(x, ',' ORDER BY x) FROM unnest(ARRAY['b', 'B', 'a', 'Z']) x")"
test "$order" = "a,b,B,Z" || { echo "ICU's root collation sorts b, B, a, Z as $order" >&2; exit 1; }
sql postgres -c "CREATE DATABASE original" -c "CREATE DATABASE restored"
sql original -c "CREATE TABLE movies (title text)" \
	-c "INSERT INTO movies SELECT 'Movie ' || n FROM generate_series(1, 1000) n"
"$bin/pg_dump.exe" --format=custom --compress=gzip -h 127.0.0.1 -p "$port" -U polyfin \
	--file="$(cygpath -w "$check/original.dump")" original
"$bin/pg_restore.exe" -h 127.0.0.1 -p "$port" -U polyfin -d restored "$(cygpath -w "$check/original.dump")"
restored="$(sql restored -c "SELECT count(*) FROM movies WHERE title LIKE 'Movie %'")"
test "$restored" = 1000 || { echo "the restored database has $restored movies, not 1000" >&2; exit 1; }
stop_check
trap - EXIT

rm -rf "$out"
mkdir -p "$out"
cp -R "$prefix/bin" "$prefix/lib" "$prefix/share" "$prefix/licenses" "$out/"
printf '%s\n' "$inputs" > "$out/build-inputs"
rm -rf "$work"
log "PostgreSQL for Windows arm64 is in $out"
