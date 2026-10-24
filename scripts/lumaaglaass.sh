#!/usr/bin/env bash
# Refreshes the copy of the LumaaGlaass theme that Polyfin bundles as the
# web player's default theme (see docs/web-client.md#default-theme), from a
# commit of the theme's repository: its stylesheet, its script and their
# extensions (assets/ in the repository) go to internal/webtheme/lumaaglaass,
# its license to third_party/lumaaglaass, with a notice naming the commit.
# Polyfin's patches, third_party/lumaaglaass/patches/*.patch, are then
# applied in order: a patch that no longer applies stops the script, to be
# made again on the new files. The result is a diff to review.
#
# The bundled files are served under a version made from their content, which
# the custom CSS and script Polyfin serves for the theme name
# (accounts.LumaaGlaassCss and LumaaGlaassJs). They are made at start, not
# stored, so a new copy takes no migration.
#
# Requirements: curl, tar, git. Usage: scripts/lumaaglaass.sh COMMIT (the
# full hash of a commit of the theme's repository).
set -euo pipefail

repository='https://github.com/Dyhlio/LumaaGlaass'
commit="${1:-}"
if ! [[ "$commit" =~ ^[0-9a-f]{40}$ ]]; then
	echo "usage: $0 COMMIT (a full commit hash of $repository)" >&2
	exit 2
fi
for tool in curl tar git; do
	command -v "$tool" >/dev/null || { echo "$tool is required" >&2; exit 1; }
done

root="$(cd "$(dirname "$0")/.." && pwd)"
bundle="$root/internal/webtheme/lumaaglaass"
third="$root/third_party/lumaaglaass"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

curl --silent --show-error --fail --location "$repository/archive/$commit.tar.gz" | tar -xz -C "$work"
src="$work/LumaaGlaass-$commit"
[[ -f "$src/assets/lumaaglaass.js" && -f "$src/assets/lumaaglaass.css" && -f "$src/LICENSE" ]] ||
	{ echo "$commit has no assets/lumaaglaass.js, assets/lumaaglaass.css or LICENSE" >&2; exit 1; }

for patch in "$third"/patches/*.patch; do
	git -C "$src" apply --verbose "$patch"
done

# Nothing the browser loads may come from elsewhere. The SVG namespace of the
# stylesheet's inline images and the placeholders of the settings page's
# profile address field name no file to load.
outside="$(grep -rnoE "([a-z][a-z0-9+.-]*:)?//[a-z0-9.-]+\.[a-z]{2,}[^'\"\` )]*" "$src/assets" |
	grep -vE ":(http://www\.w3\.org/2000/svg|https://example\.com/profile\.json|https://exemple\.com/profil\.json)$" || true)"
if [[ -n "$outside" ]]; then
	printf 'The theme names addresses outside Polyfin; patch them to the bundled files:\n%s\n' "$outside" >&2
	exit 1
fi

rm -rf "$bundle"
mkdir -p "$bundle" "$third"
cp -R "$src/assets" "$bundle/"
cp "$src/LICENSE" "$third/LICENSE"
cat >"$third/NOTICE" <<EOF
LumaaGlaass, a theme for jellyfin-web, by Om Gupta and Dyhlio.
Licensed under the MIT License: see LICENSE.

Polyfin bundles the theme's assets/ folder (its stylesheet, its script and
their extensions) at commit $commit of
its repository, at internal/webtheme/lumaaglaass, with Polyfin's patches
(patches/), applied by scripts/lumaaglaass.sh:
EOF
for patch in "$third"/patches/*.patch; do
	echo "- $(basename "$patch")" >>"$third/NOTICE"
done
cat >>"$third/NOTICE" <<EOF

The web client serves them at /web/polyfin/themes/lumaaglaass/, where the
custom CSS and script Polyfin serves for the LumaaGlaass theme load them:
nothing is loaded from elsewhere.

Source: $repository/tree/$commit
EOF
echo "LumaaGlaass $commit bundled."
