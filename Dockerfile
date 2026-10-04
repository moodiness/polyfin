# The web and Go stages run natively on the builder and cross-compile; only
# the final stage installs packages for the target platform, under emulation
# when it differs.
FROM --platform=$BUILDPLATFORM node:24.21.0-alpine@sha256:ebfe2f90462722a7a4de65e91990e97fe0d401c70e0e762c5b53302f905ec1c1 AS web
WORKDIR /build/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS build
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web/assets_embed.go ./web/assets_embed.go
COPY --from=web /build/web/dist/ ./web/dist/
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
# The final stage runs nothing, so as to need no emulation on arm64: the
# cache directory is copied.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -tags production -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /polyfin ./cmd/polyfin \
	&& mkdir /cache

# jellyfin-web 12.1, the web client of the Jellyfin version Polyfin speaks.
# Its two pins are here, to change together when the client is updated
# (and the version in third_party/jellyfin-web/NOTICE with them):
# - its built files, taken unmodified from the official Jellyfin image,
#   pinned by the digest of its multi-platform index: the very files
#   Jellyfin 12.1 serves, with no Node.js build to reproduce. They are the
#   same on every platform, so the builder's are taken;
# - its source code (GPL-2.0), which the release workflow attaches to every
#   Polyfin release; no image build reads it.
FROM --platform=$BUILDPLATFORM jellyfin/jellyfin:12.1@sha256:78d3ea1207d1322471fcac39a614f004f2ccf7e878f95ab2977d752f07e4dd7e AS jellyfin-web

FROM scratch AS jellyfin-web-source
ADD --checksum=sha256:359095593e55593b4460b125fa6df22a09993bd8e6455779e5c53fff5dd7b3c9 \
	https://github.com/jellyfin/jellyfin-web/archive/refs/tags/v12.1.tar.gz /jellyfin-web-12.1-source.tar.gz

# FFmpeg 9.0 built against glibc by BtbN, which can load GPU drivers at run
# time (NVIDIA through the NVIDIA container runtime, VAAPI through libva):
# the last build of a month, kept for two years. The shared build keeps
# ffmpeg and ffprobe from carrying a copy of every library each.
FROM --platform=$BUILDPLATFORM debian:trixie-slim@sha256:a99cfc517144bc59b1978475ec53b46ecabec7e43635402ee5b77cc54cd1b20a AS ffmpeg-amd64
ADD --checksum=sha256:01a9764d0b5364b66cfeb4617557c64321b0e232e172ec73f0a701c5f7694326 \
	https://github.com/BtbN/FFmpeg-Builds/releases/download/autobuild-2026-09-30-13-08/ffmpeg-n9.0.2-17-g2a571b6068-linux64-gpl-shared-9.0.tar.xz /ffmpeg.tar.xz

FROM --platform=$BUILDPLATFORM debian:trixie-slim@sha256:a99cfc517144bc59b1978475ec53b46ecabec7e43635402ee5b77cc54cd1b20a AS ffmpeg-arm64
ADD --checksum=sha256:706ccdbc8b0537646345532713a9b6750c0a6258cc001ec1c0379754f0ae730a \
	https://github.com/BtbN/FFmpeg-Builds/releases/download/autobuild-2026-09-30-13-08/ffmpeg-n9.0.2-17-g2a571b6068-linuxarm64-gpl-shared-9.0.tar.xz /ffmpeg.tar.xz

# A stage of this file, chosen by platform: no tag to give.
# hadolint ignore=DL3006
FROM ffmpeg-${TARGETARCH} AS ffmpeg-unpack
# hadolint ignore=DL3008
RUN apt-get update \
	&& apt-get install -y --no-install-recommends xz-utils \
	&& mkdir /ffmpeg \
	&& tar -xJf /ffmpeg.tar.xz -C /ffmpeg --strip-components=1

# ffmpeg and ffprobe find their libraries in ../lib. CI tests with this stage.
FROM scratch AS ffmpeg
COPY --from=ffmpeg-unpack /ffmpeg/bin/ffmpeg /ffmpeg/bin/ffprobe /bin/
COPY --from=ffmpeg-unpack /ffmpeg/lib/ /lib/

# Debian's slim image has no certificate authorities, which Polyfin needs
# to reach addons and their streams over HTTPS. The bundle is the same on
# every platform, so it is made natively.
FROM --platform=$BUILDPLATFORM debian:trixie-slim@sha256:a99cfc517144bc59b1978475ec53b46ecabec7e43635402ee5b77cc54cd1b20a AS certificates
# hadolint ignore=DL3008
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates

# DejaVu, the fallback fonts apps load to render subtitles whose own fonts
# are missing, for Latin, Greek and Cyrillic. Fonts are the same on every
# platform, so they are installed natively.
FROM --platform=$BUILDPLATFORM debian:trixie-slim@sha256:a99cfc517144bc59b1978475ec53b46ecabec7e43635402ee5b77cc54cd1b20a AS fonts
# hadolint ignore=DL3008
RUN apt-get update \
	&& apt-get install -y --no-install-recommends fonts-dejavu-core

# The same Debian on both platforms. amd64 adds libva and the VA drivers of
# AMD and Intel GPUs, Intel's from non-free, and what NVIDIA's Vulkan driver
# loads besides itself (the Vulkan loader, EGL, X11's extension library),
# for tone mapping HDR on NVIDIA GPUs through libplacebo; arm64 boards have
# none of these GPUs, and so their image runs nothing under emulation.
# NVIDIA's own libraries come from the NVIDIA container runtime.
FROM debian:trixie-slim@sha256:a99cfc517144bc59b1978475ec53b46ecabec7e43635402ee5b77cc54cd1b20a AS runtime-arm64

FROM debian:trixie-slim@sha256:a99cfc517144bc59b1978475ec53b46ecabec7e43635402ee5b77cc54cd1b20a AS runtime-amd64
# Packages are left unpinned so that each build gets Debian's security fixes.
# hadolint ignore=DL3008
RUN sed -i 's/^Components: main$/Components: main non-free/' /etc/apt/sources.list.d/debian.sources \
	&& apt-get update \
	&& apt-get install -y --no-install-recommends libva2 libva-drm2 mesa-va-drivers intel-media-va-driver-non-free \
		libvulkan1 libegl1 libxext6 \
	&& rm -rf /var/lib/apt/lists/*

# A stage of this file, chosen by platform: no tag to give.
# hadolint ignore=DL3006
FROM runtime-${TARGETARCH}
COPY --from=ffmpeg /bin/ffmpeg /bin/ffprobe /usr/local/bin/
COPY --from=ffmpeg /lib/ /usr/local/lib/
COPY --from=certificates /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=fonts /usr/share/fonts/truetype/dejavu/ /usr/share/fonts/truetype/dejavu/
# jellyfin-web, a separate GPL-2.0 program, unmodified, in the folder
# POLYFIN_WEB_DIR names by default; its license, and a notice saying where
# its source is, in the usual documentation folder.
COPY --from=jellyfin-web /jellyfin/jellyfin-web/ /usr/share/polyfin/jellyfin-web/
COPY third_party/jellyfin-web/LICENSE third_party/jellyfin-web/NOTICE /usr/share/doc/jellyfin-web/
COPY --from=build /polyfin /polyfin
# Parts of the files being read. A volume, so that it stays writable in a
# read-only container; Polyfin empties it when it starts.
COPY --from=build --chown=65532:65532 /cache /cache
VOLUME /cache
# NVIDIA's runtime exposes the GPUs and the video and Vulkan libraries to
# containers that ask for it; other runtimes ignore these. Mesa would keep
# a shader cache in a home directory the read-only image does not have.
ENV POLYFIN_LISTEN=:8096 POLYFIN_CACHE_DIR=/cache NVIDIA_VISIBLE_DEVICES=all NVIDIA_DRIVER_CAPABILITIES=compute,video,utility,graphics \
	MESA_SHADER_CACHE_DISABLE=true
EXPOSE 8096
USER 65532:65532
ENTRYPOINT ["/polyfin"]
CMD ["serve"]
