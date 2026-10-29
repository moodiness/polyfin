#!/usr/bin/env bash
# Builds the FFmpeg Polyfin runs NVENC with when the NVIDIA driver is older
# than 610: the same FFmpeg as the Dockerfile's, from the same build recipe
# at the same commit, but built against NVENC's API 13.0 (nv-codec-headers
# sdk/13.0), which drivers from 570 take, where the Dockerfile's build asks
# for API 13.1 and driver 610. Nothing else differs: the dependencies, the
# configuration and the FFmpeg commit are the recipe's and the Dockerfile's.
#
# Everything the build takes is pinned: the recipe by the commit of the
# Dockerfile's release tag, FFmpeg by the commit its build names, the
# headers and every dependency by the commit the recipe pins them at, the
# cross toolchains by the last crosstool-ng commit before the recipe's,
# and the Ubuntu packages of the build images by Ubuntu's snapshot of the
# recipe's date. The Ubuntu base image and the build tools are recorded in
# BUILDINFO, with the digests of what was built.
#
# Usage:
#   scripts/ffmpeg-nvenc13.sh pins                 print the pins, as KEY=value lines
#   scripts/ffmpeg-nvenc13.sh recipe DIR           clone and adjust the recipe into DIR
#   scripts/ffmpeg-nvenc13.sh base DIR             build the base image (PUSH=1: push it)
#   scripts/ffmpeg-nvenc13.sh toolchain DIR TARGET build TARGET's toolchain image on the base
#                                                  image (PUSH=1: push it)
#   scripts/ffmpeg-nvenc13.sh image DIR TARGET     build TARGET's dependency image, with the base
#                                                  and toolchain images (QUICKBUILD=1: those of
#                                                  the registry)
#   scripts/ffmpeg-nvenc13.sh quick-image DIR TARGET
#                                                  to check the recipe locally in less than an
#                                                  hour: the recipe's published dependency image
#                                                  of TARGET, with the 13.0 headers installed as
#                                                  the adjusted recipe installs them; not for
#                                                  release
#   scripts/ffmpeg-nvenc13.sh ffmpeg DIR TARGET    build FFmpeg for TARGET into DIR/out
#   scripts/ffmpeg-nvenc13.sh sources DIR          gather the corresponding sources into DIR/out
#
# TARGET is linux64, linuxarm64 or win64: NVIDIA's drivers exist for these
# only. The images are named after NVENC13_IMAGES, owner/name, in GHCR,
# local/polyfin-ffmpeg-nvenc13 by default. Requirements: bash 4, git,
# python3, docker with buildx, GNU tar, xz, zip and coreutils.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
recipe_repo=https://github.com/BtbN/FFmpeg-Builds.git
ctng_repo=https://github.com/crosstool-ng/crosstool-ng.git
variant=gpl-shared
addin=9.0
# The suffix telling this build apart, in its file names and in
# `ffmpeg -version`.
suffix=nvenc13.0

die() { echo "$*" >&2; exit 1; }

# The recipe's scripts name their images after GITHUB_REPOSITORY.
export GITHUB_REPOSITORY="${NVENC13_IMAGES:-local/polyfin-ffmpeg-nvenc13}"

# pins reads what the Dockerfile ships, and what the build takes with it.
pins() {
	local tag describe
	tag=$(grep -o 'releases/download/autobuild-[0-9-]*/ffmpeg-[^ ]*-linux64-gpl-shared-9.0.tar.xz' "$root/Dockerfile" | head -n 1)
	[[ -n $tag ]] || die "the Dockerfile names no linux64 gpl-shared 9.0 build"
	describe=${tag#*/ffmpeg-}
	describe=${describe%-linux64-gpl-shared-9.0.tar.xz}
	tag=${tag#releases/download/}
	tag=${tag%%/*}
	local commit
	commit=$(git ls-remote "$recipe_repo" "refs/tags/$tag" | cut -f1)
	[[ -n $commit ]] || die "the recipe has no tag $tag"
	echo "RECIPE_TAG=$tag"
	echo "RECIPE_COMMIT=$commit"
	echo "FFMPEG_DESCRIBE=$describe"
	echo "FFMPEG_COMMIT=${describe##*-g}"
	echo "RELEASE_TAG=ffmpeg-$suffix-$tag"
}

# recipe clones the recipe at its pinned commit into dir/recipe, and
# adjusts it, each change checked to apply:
# - FFmpeg 9.0 is built against the headers the recipe uses for 8.1, its
#   pinned sdk/13.0 commit;
# - FFmpeg is checked out at the commit the Dockerfile's build names, and
#   says the suffix in its version, where the recipe takes the release
#   branch's head and the day's date;
# - the build images start from Ubuntu's last dated image of the recipe's
#   release before the recipe's commit, and install Ubuntu's packages as
#   Ubuntu's snapshot of that commit has them, and crosstool-ng at its
#   last commit before the recipe's, where the recipe takes the latest of
#   all three.
recipe() {
	local dir=$1
	mkdir -p "$dir"
	pins >"$dir/pins.env"
	# shellcheck disable=SC1091
	source "$dir/pins.env"
	rm -rf "$dir/recipe"
	git clone --quiet --filter=blob:none "$recipe_repo" "$dir/recipe"
	git -C "$dir/recipe" -c advice.detachedHead=false checkout --quiet "$RECIPE_COMMIT"
	local date snapshot ctng release ubuntu
	date=$(git -C "$dir/recipe" show -s --format=%cI HEAD)
	snapshot=$(TZ=UTC git -C "$dir/recipe" show -s --format=%cd --date=format-local:%Y%m%dT%H%M%SZ HEAD)
	rm -rf "$dir/crosstool-ng"
	git clone --quiet --filter=blob:none --no-checkout "$ctng_repo" "$dir/crosstool-ng"
	ctng=$(git -C "$dir/crosstool-ng" rev-list -1 --before="$date" HEAD)
	rm -rf "$dir/crosstool-ng"
	release=$(sed -n 's/^FROM ubuntu:\([0-9.]*\)$/\1/p' "$dir/recipe/images/base/Dockerfile")
	[[ -n $release ]] || die "the recipe's base image is not ubuntu:<release>"
	ubuntu=$(dated_ubuntu "$release" "${snapshot%%T*}")
	{
		echo "RECIPE_DATE=$date"
		echo "UBUNTU_IMAGE=$ubuntu"
		echo "UBUNTU_SNAPSHOT=$snapshot"
		echo "CROSSTOOL_NG_COMMIT=$ctng"
	} >>"$dir/pins.env"

	local r=$dir/recipe
	edit "$r/scripts.d/50-ffnvcodec.sh" 'elif (( $FFVER <= 801 )); then' 'elif (( $FFVER <= 900 )); then'
	edit "$r/build.sh" '    cd ffmpeg' "    cd ffmpeg
    git -c advice.detachedHead=false checkout --detach '\${FFMPEG_COMMIT}'"
	edit "$r/build.sh" '--extra-version="\$(date +%Y%m%d)"' "--extra-version='\${FFMPEG_EXTRA_VERSION}'"
	edit "$r/images/base/Dockerfile" "FROM ubuntu:$release" "FROM $ubuntu"
	# The image has no certificates yet: apt fetches the snapshot without
	# checking its host's, as it checks the archive's signatures anyway.
	edit "$r/images/base/Dockerfile" 'ENV DEBIAN_FRONTEND=noninteractive' "ENV DEBIAN_FRONTEND=noninteractive
RUN echo 'APT::Snapshot \"$snapshot\";' > /etc/apt/apt.conf.d/50snapshot && \\
    echo 'Acquire::https::snapshot.ubuntu.com::Verify-Peer \"false\";' >> /etc/apt/apt.conf.d/50snapshot"
	local target
	for target in linux64 linuxarm64 win64; do
		edit "$r/images/base-$target/Dockerfile" 'git clone --filter=blob:none https://github.com/crosstool-ng/crosstool-ng.git /ct-ng && cd /ct-ng && \' \
			"git clone --filter=blob:none https://github.com/crosstool-ng/crosstool-ng.git /ct-ng && cd /ct-ng && git checkout $ctng && \\"
	done
	git -C "$r" diff >"$dir/recipe.diff"
	echo "The recipe at $RECIPE_COMMIT, adjusted as $dir/recipe.diff says." >&2
}

# dated_ubuntu is Ubuntu's last image of release dated on or before day,
# YYYYMMDD, as ubuntu:<codename>-<date>@<digest>: Docker Hub tags each
# image of a release with its codename and the day it was built.
dated_ubuntu() {
	local release=$1 day=$2 codename
	codename=$(docker run --rm "ubuntu:$release" sh -c '. /etc/os-release && echo "$VERSION_CODENAME"')
	curl -fsS "https://hub.docker.com/v2/repositories/library/ubuntu/tags?page_size=100&name=$codename-" |
		python3 -c '
import json, re, sys
codename, day = sys.argv[1], sys.argv[2]
tags = [t for t in json.load(sys.stdin)["results"] if re.fullmatch(codename + r"-(\d{8})(\.\d+)?", t["name"])]
tags = [t for t in tags if t["name"][len(codename) + 1:][:8] <= day]
if not tags:
    sys.exit(f"no image of {codename} is dated on or before {day}")
last = max(tags, key=lambda t: t["name"])
print("ubuntu:" + last["name"] + "@" + last["digest"])
' "$codename" "$day"
}

# edit replaces the one line holding old in file with new, and fails when
# old is not there once: the recipe changed, and this script must follow.
edit() {
	local file=$1 old=$2 new=$3
	python3 - "$file" "$old" "$new" <<-'EOF'
		import sys
		path, old, new = sys.argv[1:]
		text = open(path).read()
		if text.count(old) != 1:
		    sys.exit(f"{path}: {old!r} is there {text.count(old)} times, not once")
		open(path, "w").write(text.replace(old, new))
	EOF
}

# checked fails unless target is one this build is made for.
checked() {
	case $1 in
	linux64 | linuxarm64 | win64) ;;
	*) die "target $1: linux64, linuxarm64 or win64" ;;
	esac
}

# built tags an image built from context, and pushes it with PUSH=1.
built() {
	local tag=$1
	shift
	local output=--load
	[[ ${PUSH:-} == 1 ]] && output=--push
	docker buildx build "$output" --provenance=false --tag "$tag" "$@"
}

# base builds the base image of the adjusted recipe.
base() {
	local dir=$1
	built "$(image_repo)/base:latest" "$dir/recipe/images/base"
}

# toolchain builds target's cross toolchain image on the base image.
toolchain() {
	local dir=$1 target=$2
	checked "$target"
	built "$(image_repo)/base-$target:latest" --build-arg GH_REPO="$(image_repo)" "$dir/recipe/images/base-$target"
}

# image builds target's dependency image with the adjusted recipe: on the
# base and toolchain images it builds first, or, with QUICKBUILD=1, on
# those of the registry.
image() {
	local dir=$1 target=$2
	checked "$target"
	(cd "$dir/recipe" && ./makeimage.sh "$target" "$variant" "$addin")
}

# quick_image tags, as target's dependency image, the recipe's published
# one with the 13.0 headers installed, as the adjusted recipe's
# ffnvcodec stage installs them, at the commit it pins. Its dependencies
# are those of the recipe's latest build, not of its pinned commit: it
# checks that FFmpeg builds against the 13.0 headers, not what a release
# ships.
quick_image() {
	local dir=$1 target=$2
	checked "$target"
	# shellcheck disable=SC1091
	source "$dir/pins.env"
	local headers published local_image
	headers=$(sed -n 's/^SCRIPT_COMMIT2="\(.*\)"$/\1/p' "$dir/recipe/scripts.d/50-ffnvcodec.sh")
	published="ghcr.io/btbn/ffmpeg-builds/$target-$variant-$addin:latest"
	local_image="$(image_repo)/$target-$variant-$addin:latest"
	docker pull --quiet "$published" >/dev/null
	docker buildx build --load --tag "$local_image" - <<-EOF
		FROM $published
		RUN git clone --filter=blob:none https://github.com/FFmpeg/nv-codec-headers.git /nvh && \\
		    git -C /nvh -c advice.detachedHead=false checkout $headers && \\
		    rm -rf "\$FFBUILD_PREFIX/include/ffnvcodec" && \\
		    make -C /nvh PREFIX="\$FFBUILD_PREFIX" install && rm -rf /nvh
	EOF
	echo "PUBLISHED_IMAGE=$(docker inspect --format '{{index .RepoDigests 0}}' "$published")" >>"$dir/pins.env"
}

# image_repo is where the recipe's scripts name the images.
image_repo() {
	echo "${REGISTRY_OVERRIDE:-ghcr.io}/${GITHUB_REPOSITORY,,}"
}

# ffmpeg builds FFmpeg for target with its dependency image, into
# dir/out, as ffmpeg-<describe>-<target>-gpl-shared-9.0-nvenc13.0 with its
# SHA-256 file, and checks that it asks NVENC for driver 570.0.
ffmpeg() {
	local dir=$1 target=$2
	checked "$target"
	# shellcheck disable=SC1091
	source "$dir/pins.env"
	mkdir -p "$dir/out"
	local built
	built=$(mktemp -d)
	# shellcheck disable=SC2064
	trap "rm -rf '$built'" EXIT
	(cd "$dir/recipe" && FFMPEG_COMMIT="$FFMPEG_COMMIT" FFMPEG_EXTRA_VERSION="$suffix" FFBUILD_OUTPUT_DIR="$built/ffmpeg" \
		./build.sh "$target" "$variant" "$addin")
	local name="ffmpeg-$FFMPEG_DESCRIBE-$target-$variant-$addin-$suffix"
	local codec
	codec=$(find "$built/ffmpeg" \( -name 'libavcodec*.so.*' -o -name 'avcodec-*.dll' \) -print -quit)
	[[ -n $codec ]] || die "no libavcodec in the build"
	grep -aq 'nvenc is %s or newer' "$codec" || die "the build has no NVENC"
	if ! grep -aq '570\.0' "$codec" || grep -aq '610\.00' "$codec"; then
		die "the build does not ask NVENC for driver 570.0"
	fi
	grep -raq -- "$FFMPEG_DESCRIBE-$suffix" "$built/ffmpeg" || die "the build does not say $FFMPEG_DESCRIBE-$suffix"
	mv "$built/ffmpeg" "$built/$name"
	if [[ $target == win* ]]; then
		(cd "$built" && zip -q -9 -r -X "$dir/out/$name.zip" "$name")
	else
		tar -C "$built" --sort=name --owner=0 --group=0 --numeric-owner --mtime="@0" -cf - "$name" | xz -T0 -9 >"$dir/out/$name.tar.xz"
	fi
	rm -rf "$built"
	(cd "$dir/out" && for f in "$name".*; do [[ $f == *.sha256 ]] || sha256sum "$f" >"$f.sha256"; done)
	echo "Built $dir/out/$name" >&2
}

# sources gathers what the builds are made from, which the GPL asks to be
# offered with them: the sources of every dependency as the recipe fetches
# them, FFmpeg's at its commit, and the adjusted recipe, in one tar file
# cut into parts a release takes, each with its SHA-256, and the whole's.
sources() {
	local dir=$1
	# shellcheck disable=SC1091
	source "$dir/pins.env"
	local name="ffmpeg-$FFMPEG_DESCRIBE-$suffix-sources"
	local work="$dir/sources/$name"
	rm -rf "$dir/sources"
	mkdir -p "$work" "$dir/out"
	(cd "$dir/recipe" && ./download.sh)
	cp -L "$dir/recipe/.cache/downloads/"*_*.tar.xz "$work/"
	git clone --quiet --filter=blob:none https://github.com/FFmpeg/FFmpeg.git "$dir/sources/ffmpeg"
	git -C "$dir/sources/ffmpeg" archive --format=tar --prefix="ffmpeg-$FFMPEG_DESCRIBE/" "$FFMPEG_COMMIT" | xz -T0 >"$work/ffmpeg-$FFMPEG_DESCRIBE.tar.xz"
	git -C "$dir/recipe" archive --format=tar --prefix=recipe/ HEAD | xz -T0 >"$work/recipe-$RECIPE_COMMIT.tar.xz"
	cp "$dir/recipe.diff" "$dir/pins.env" "$work/"
	tar -C "$dir/sources" --sort=name --owner=0 --group=0 --numeric-owner --mtime="@0" -cf "$dir/sources/$name.tar" "$name"
	(cd "$dir/sources" && sha256sum "$name.tar" >"$dir/out/$name.tar.sha256" &&
		split --bytes=1900M --numeric-suffixes=1 --suffix-length=2 "$name.tar" "$dir/out/$name.tar.part")
	(cd "$dir/out" && for f in "$name".tar.part*; do [[ $f == *.sha256 ]] || sha256sum "$f" >"$f.sha256"; done)
	rm -rf "$dir/sources"
}

[[ $# -ge 1 ]] || die "usage: $0 pins | recipe DIR | base DIR | toolchain DIR TARGET | image DIR TARGET | quick-image DIR TARGET | ffmpeg DIR TARGET | sources DIR"
command=$1
shift
# The recipe's scripts give Docker files they make with mktemp to mount:
# they go in DIR, which Docker's host sees where this runs in a container
# sharing it.
[[ $command == pins ]] || {
	[[ $# -ge 1 ]] || die "$command needs DIR"
	mkdir -p "$1/tmp"
	dir=$(cd "$1" && pwd)
	export TMPDIR="$dir/tmp"
	shift
}
case $command in
pins) pins ;;
recipe) recipe "$dir" ;;
base) base "$dir" ;;
toolchain) toolchain "$dir" "$1" ;;
image) image "$dir" "$1" ;;
quick-image) quick_image "$dir" "$1" ;;
ffmpeg) ffmpeg "$dir" "$1" ;;
sources) sources "$dir" ;;
*) die "unknown command $command" ;;
esac
