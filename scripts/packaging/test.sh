#!/usr/bin/env bash
# Usage: scripts/packaging/test.sh <package> [work folder]
#
# Installs or opens a package of Polyfin Desktop on the system it is for,
# checks the install layout, runs scripts/desktop-smoke.py on the installed
# app (headless, on a data folder of the work folder, behind a proxy that
# refuses every request for outside, leaving no process), then removes it:
#   .dmg        mounted read-only, the app run from the disk image;
#   -setup.exe  installed for the current user, silently, its programs and
#               libraries checked to be the package's architecture (x64 or
#               arm64) and to find every DLL they import, then uninstalled:
#               the data folder, %LOCALAPPDATA%\Polyfin, must stay;
#   .deb        installed with apt (sudo), then removed;
#   .AppImage   run as it is when FUSE is there, else from its files
#               extracted.
# CI runs it on each runner; it changes the system it runs on, so run it
# locally on a throwaway machine or container. VERSION, when set, must be
# the version the app reports.
source "$(dirname "$0")/common.sh"

package="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
work="${2:-$WORK/test}"
rm -rf "$work"
mkdir -p "$work"

smoke() {
	"$PYTHON" "$ROOT/scripts/desktop-smoke.py" --installed "$@"
}

check_version() {
	local reported
	# Through a pipe, which a Windows app without a console writes to too.
	reported="$("$@" -version | cat)"
	if [ "$VERSION" != dev ] && [ "$reported" != "$VERSION" ]; then
		echo "$* reports version $reported, not $VERSION" >&2
		exit 1
	fi
}

as_root() {
	if [ "$(id -u)" = 0 ]; then "$@"; else sudo "$@"; fi
}

test_dmg() {
	local app
	mount="$work/mount"
	mkdir -p "$mount"
	hdiutil attach -nobrowse -readonly -noautoopen -mountpoint "$mount" "$package" > /dev/null
	trap 'hdiutil detach -force "$mount" > /dev/null || true' EXIT
	# One app, which the update replaces the installed one with.
	ls -d "$mount"/*.app > "$work/apps"
	test "$(cat "$work/apps")" = "$mount/Polyfin.app" || { echo "the disk image holds: $(cat "$work/apps")" >&2; exit 1; }
	test "$(readlink "$mount/Applications")" = /Applications
	app="$mount/Polyfin.app"
	test "$(plutil -extract LSUIElement raw "$app/Contents/Info.plist")" = true
	test "$(plutil -extract CFBundleExecutable raw "$app/Contents/Info.plist")" = polyfin-desktop
	codesign --verify --deep --strict "$app"
	case "$package" in
	*-arm64.dmg) lipo "$app/Contents/MacOS/polyfin-desktop" -verify_arch arm64 ;;
	*-x64.dmg) lipo "$app/Contents/MacOS/polyfin-desktop" -verify_arch x86_64 ;;
	esac
	check_install "$app/Contents/Resources" ""
	check_version "$app/Contents/MacOS/polyfin-desktop"
	smoke "$app/Contents/MacOS/polyfin-desktop" "$app/Contents/Resources" "$work/smoke"
	# The secret key the smoke's data folder made, in the login keychain.
	security delete-generic-password -s Polyfin -a "Secret key of $work/smoke/data" > /dev/null 2>&1 || true
}

# check_programs DIR MACHINE: every program and library of DIR, a Windows
# install folder, is built for MACHINE, the PE header's: 0xAA64 for ARM64,
# 0x8664 for x64; a package that took another architecture's programs
# would still run, under Windows' emulation. And each finds the DLLs it
# imports, delay-loaded ones too, where its process loads them from:
# postgresql/bin for PostgreSQL's (lib's modules are loaded by postgres),
# DIR for the others; else among API sets, or in System32, but for the
# Visual C++ runtime, which a fresh Windows lacks while runners have it
# (msvcrt.dll, the system's own C runtime, is Windows'). It prints the
# DLLs taken from System32. The uninstaller NSIS writes is
# its x86 stub on every system, and is left out.
check_programs() {
	"$PYTHON" -c 'import os, re, struct, sys
root, want = sys.argv[1], int(sys.argv[2], 16)
system = sys.argv[3] if len(sys.argv) > 3 else os.path.join(os.environ["SYSTEMROOT"], "System32")
runtime = re.compile(r"msvcp|vcruntime|concrt|vccorlib|msvcr\d|ucrtbased")
listings = {}
def listing(folder):
    if folder not in listings:
        listings[folder] = {name.lower() for name in os.listdir(folder)}
    return listings[folder]
def pe(data):
    """The machine and the imported DLLs, the delay-loaded ones too."""
    if data[:2] != b"MZ" or len(data) < 0x40:
        return None, []
    header, = struct.unpack_from("<I", data, 0x3C)
    if data[header:header + 4] != b"PE\0\0":
        return None, []
    machine, count = struct.unpack_from("<HH", data, header + 4)
    optional = header + 24
    plus = struct.unpack_from("<H", data, optional)[0] == 0x20B
    base, = struct.unpack_from("<Q" if plus else "<I", data, optional + (24 if plus else 28))
    directories, = struct.unpack_from("<I", data, optional + (108 if plus else 92))
    table = optional + struct.unpack_from("<H", data, header + 20)[0]
    sections = [struct.unpack_from("<IIII", data, table + 40 * i + 8) for i in range(count)]
    def offset(rva):
        for size, address, raw_size, raw in sections:
            if address <= rva < address + max(size, raw_size):
                return raw + rva - address
        raise ValueError("RVA %#x is in no section" % rva)
    found = []
    # The import table (1): 20-byte entries, the name at 12; the delay-load
    # one (13): 32-byte entries, the name at 4, a VA unless attribute 1.
    for index, size, at in ((1, 20, 12), (13, 32, 4)):
        rva = struct.unpack_from("<I", data, optional + (112 if plus else 96) + 8 * index)[0] if index < directories else 0
        entry = offset(rva) if rva else None
        while entry is not None:
            name, = struct.unpack_from("<I", data, entry + at)
            if not name:
                break
            if index == 13 and not data[entry] & 1:
                name -= base
            start = offset(name)
            found.append(data[start:data.index(b"\0", start)].decode("ascii"))
            entry += size
    return machine, found
problems, from_system = [], set()
for folder, _, files in os.walk(root):
    for name in files:
        path = os.path.join(folder, name)
        program = os.path.relpath(path, root)
        if not name.lower().endswith((".exe", ".dll")) or program == "Uninstall.exe":
            continue
        with open(path, "rb") as f:
            machine, imports = pe(f.read())
        if machine != want:
            problems.append("%s: built for %s, not %#x" % (program, "no machine" if machine is None else "%#x" % machine, want))
            continue
        beside = os.path.join(root, "postgresql", "bin") if program.split(os.sep)[0] == "postgresql" else root
        for dll in imports:
            lower = dll.lower()
            if lower in listing(beside) or lower.startswith(("api-ms-win-", "ext-ms-win-")):
                continue
            if not runtime.match(lower) and lower in listing(system):
                from_system.add(lower)
                continue
            problems.append("%s: imports %s, which is neither beside it nor in %s" % (program, dll, system))
if problems:
    sys.exit("\n".join(sorted(problems)))
print("DLLs from %s: %s" % (system, ", ".join(sorted(from_system))))' "$(cygpath -w "$1")" "$2"
}

test_windows() {
	local local_data install programs marker run_key uninstall_key goarch machine
	case "$package" in
	*-windows-arm64-setup.exe) goarch=arm64 machine=0xAA64 ;;
	*-windows-x64-setup.exe) goarch=amd64 machine=0x8664 ;;
	*) echo "not a Windows installer of Polyfin Desktop: $package" >&2; exit 2 ;;
	esac
	local_data="$(cygpath -u "$LOCALAPPDATA")"
	install="$local_data/Programs/Polyfin"
	programs="$(cygpath -u "$APPDATA")/Microsoft/Windows/Start Menu/Programs"
	run_key='HKCU\Software\Microsoft\Windows\CurrentVersion\Run'
	uninstall_key='HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\Polyfin'
	# Git Bash leaves the switches of Windows programs as they are with
	# MSYS_NO_PATHCONV, rather than taking them for paths.
	MSYS_NO_PATHCONV=1 "$package" /S
	check_install "$install" .exe
	check_programs "$install" "$machine"
	test -f "$install/polyfin-desktop.exe"
	test -f "$install/postgresql/bin/vcruntime140.dll"
	test -f "$programs/Polyfin.lnk"
	MSYS_NO_PATHCONV=1 reg query "$uninstall_key" /v UninstallString
	check_version "$install/polyfin-desktop.exe"
	# The package's app has no console, which the smoke interrupts to quit
	# it: a console build of the same app runs from the install folder in
	# its place, and finds it the same way.
	(cd "$ROOT" && CGO_ENABLED=0 GOARCH="$goarch" go build -trimpath -ldflags "-X main.version=$VERSION" \
		-o "$install/polyfin-desktop-smoke.exe" ./cmd/polyfin-desktop)
	smoke "$(cygpath -w "$install/polyfin-desktop-smoke.exe")" "$(cygpath -w "$install")" "$(cygpath -w "$work/smoke")"
	rm "$install/polyfin-desktop-smoke.exe"

	# Uninstalling leaves the data folder, and removes the login item.
	marker=
	if [ ! -e "$local_data/Polyfin" ]; then
		mkdir -p "$local_data/Polyfin"
		marker="$local_data/Polyfin/kept-by-the-uninstaller"
		touch "$marker"
	fi
	MSYS_NO_PATHCONV=1 reg add "$run_key" /v io.github.moodiness.polyfin-desktop /d "\"$(cygpath -w "$install/polyfin-desktop.exe")\" --background" /f > /dev/null
	MSYS_NO_PATHCONV=1 "$install/Uninstall.exe" /S
	# The uninstaller runs from a copy of itself, which returns at once.
	for _ in $(seq 60); do
		[ -e "$install" ] || break
		sleep 1
	done
	if [ -e "$install" ]; then
		echo "the uninstaller left $install:" >&2
		find "$install" >&2
		exit 1
	fi
	test ! -e "$programs/Polyfin.lnk"
	if reg query "$uninstall_key" > /dev/null 2>&1; then echo "the uninstaller left $uninstall_key" >&2; exit 1; fi
	if MSYS_NO_PATHCONV=1 reg query "$run_key" /v io.github.moodiness.polyfin-desktop > /dev/null 2>&1; then
		echo "the uninstaller left the login item" >&2
		exit 1
	fi
	test -d "$local_data/Polyfin"
	if [ -n "$marker" ]; then
		test -f "$marker"
		rm -rf "$local_data/Polyfin"
	fi
}

test_deb() {
	as_root apt-get install -y --no-install-recommends "$package"
	check_install /opt/polyfin ""
	test -x /opt/polyfin/polyfin-desktop
	test "$(readlink /usr/bin/polyfin-desktop)" = /opt/polyfin/polyfin-desktop
	test -f /usr/share/applications/polyfin-desktop.desktop
	test -f /usr/share/icons/hicolor/scalable/apps/polyfin-desktop.svg
	if command -v desktop-file-validate > /dev/null; then
		desktop-file-validate /usr/share/applications/polyfin-desktop.desktop
	fi
	check_version /usr/bin/polyfin-desktop
	smoke /usr/bin/polyfin-desktop /opt/polyfin "$work/smoke"
	as_root apt-get remove -y polyfin-desktop
	test ! -e /opt/polyfin
	test ! -e /usr/bin/polyfin-desktop
}

test_appimage() {
	chmod +x "$package"
	# Its files, extracted without FUSE, from the file system that follows
	# the runtime, whose ELF headers give its size: unsquashfs reads it
	# without running the runtime, which an emulator may refuse to run.
	if command -v unsquashfs > /dev/null; then
		local offset
		offset="$("$PYTHON" -c 'import struct, sys
head = open(sys.argv[1], "rb").read(64)
shoff, = struct.unpack_from("<Q", head, 0x28)
size, count = struct.unpack_from("<HH", head, 0x3A)
print(shoff + size * count)' "$package")"
		unsquashfs -q -o "$offset" -d "$work/squashfs-root" "$package" > /dev/null
	else
		(cd "$work" && "$package" --appimage-extract > /dev/null)
	fi
	check_install "$work/squashfs-root" ""
	test -x "$work/squashfs-root/AppRun"
	test -f "$work/squashfs-root/polyfin-desktop.desktop"
	if command -v desktop-file-validate > /dev/null; then
		desktop-file-validate "$work/squashfs-root/polyfin-desktop.desktop"
	fi
	if [ -c /dev/fuse ] && { command -v fusermount3 || command -v fusermount; } > /dev/null; then
		check_version "$package"
		# Mounted where the system chooses: any install folder will do.
		smoke "$package" auto "$work/smoke"
	else
		echo "no FUSE here: the smoke runs the AppImage's extracted files" >&2
		check_version "$work/squashfs-root/AppRun"
		smoke "$work/squashfs-root/AppRun" "$work/squashfs-root" "$work/smoke"
	fi
}

case "$package" in
*.dmg) test_dmg ;;
*-setup.exe) test_windows ;;
*.deb) test_deb ;;
*.AppImage) test_appimage ;;
*) echo "not a package of Polyfin Desktop: $package" >&2; exit 2 ;;
esac
echo "ok: $(basename "$package")"
