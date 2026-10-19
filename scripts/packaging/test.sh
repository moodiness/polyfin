#!/usr/bin/env bash
# Usage: scripts/packaging/test.sh <package> [work folder]
#
# Installs or opens a package of Polyfin Desktop on the system it is for,
# checks the install layout, runs scripts/desktop-smoke.py on the installed
# app (headless, on a data folder of the work folder, behind a proxy that
# refuses every request for outside, leaving no process), then removes it:
#   .dmg        mounted read-only, the app run from the disk image;
#   -setup.exe  installed for the current user, silently, then uninstalled:
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

test_windows() {
	local local_data install programs marker run_key uninstall_key
	local_data="$(cygpath -u "$LOCALAPPDATA")"
	install="$local_data/Programs/Polyfin"
	programs="$(cygpath -u "$APPDATA")/Microsoft/Windows/Start Menu/Programs"
	run_key='HKCU\Software\Microsoft\Windows\CurrentVersion\Run'
	uninstall_key='HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\Polyfin'
	# Git Bash leaves the switches of Windows programs as they are with
	# MSYS_NO_PATHCONV, rather than taking them for paths.
	MSYS_NO_PATHCONV=1 "$package" /S
	check_install "$install" .exe
	test -f "$install/polyfin-desktop.exe"
	test -f "$install/postgresql/bin/vcruntime140.dll"
	test -f "$programs/Polyfin.lnk"
	MSYS_NO_PATHCONV=1 reg query "$uninstall_key" /v UninstallString
	check_version "$install/polyfin-desktop.exe"
	# The package's app has no console, which the smoke interrupts to quit
	# it: a console build of the same app runs from the install folder in
	# its place, and finds it the same way.
	(cd "$ROOT" && CGO_ENABLED=0 go build -trimpath -ldflags "-X main.version=$VERSION" -o "$install/polyfin-desktop-smoke.exe" ./cmd/polyfin-desktop)
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
