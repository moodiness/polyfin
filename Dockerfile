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
# data directory is copied.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -tags production -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /polyfin ./cmd/polyfin \
	&& mkdir /data

# jellyfin-web 12.2, the web client of the Jellyfin version Polyfin speaks.
# Its two pins are here, to change together when the client is updated
# (and the version in third_party/jellyfin-web/NOTICE with them):
# - its built files, taken unmodified from the official Jellyfin image,
#   pinned by the digest of its multi-platform index: the very files
#   Jellyfin 12.2 serves, with no Node.js build to reproduce. They are the
#   same on every platform, so the builder's are taken;
# - its source code (GPL-2.0), which the release workflow attaches to every
#   Polyfin release; no image build reads it.
FROM --platform=$BUILDPLATFORM jellyfin/jellyfin:12.2@sha256:357724bf0ae27a672c7cbaa899db2d9abeb13dbd8657ccce750258a4c059d037 AS jellyfin-web

FROM scratch AS jellyfin-web-source
ADD --checksum=sha256:a89a68a12f9c3976d50ed33438a7e58c6a73f608d5d8e86cd98a360fd084bd95 \
	https://github.com/jellyfin/jellyfin-web/archive/refs/tags/v12.2.tar.gz /jellyfin-web-12.2-source.tar.gz

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

# pg_dump of PostgreSQL 18, which backs the database up (POLYFIN_BACKUP_DIR):
# Debian's is 17's, which refuses to dump an 18 server. It is taken, with
# its libpq and the libraries libpq loads that the final stage lacks, from
# the official image compose.yaml runs, pinned by the digest of its
# multi-platform index (the two digests change together): the target
# platform's files, copied, nothing run. Both images are the same Debian.
FROM postgres:18.6@sha256:5a5a84b19854a9ffaa54082c166ff4ec27473a361e496e5ea167f298f2da9722 AS postgres

# The libraries go in the folder of the target platform, which a native
# stage names: copies follow links, so each is copied by the name it is
# loaded by. Their copyright notices go with them.
FROM --platform=$BUILDPLATFORM debian:trixie-slim@sha256:a99cfc517144bc59b1978475ec53b46ecabec7e43635402ee5b77cc54cd1b20a AS pg-dump
ARG TARGETARCH
COPY --from=postgres /usr/lib/postgresql/18/bin/pg_dump /pg-dump/usr/local/bin/pg_dump
COPY --from=postgres /usr/lib/*-linux-gnu/libpq.so.5 /usr/lib/*-linux-gnu/libldap.so.2 /usr/lib/*-linux-gnu/liblber.so.2 \
	/usr/lib/*-linux-gnu/libsasl2.so.2 /usr/lib/*-linux-gnu/libgssapi_krb5.so.2 /usr/lib/*-linux-gnu/libkrb5.so.3 \
	/usr/lib/*-linux-gnu/libk5crypto.so.3 /usr/lib/*-linux-gnu/libkrb5support.so.0 /usr/lib/*-linux-gnu/libkeyutils.so.1 \
	/usr/lib/*-linux-gnu/libcom_err.so.2 /libraries/
COPY --from=postgres /usr/share/doc/ /doc/
RUN case "$TARGETARCH" in amd64) triplet=x86_64-linux-gnu ;; arm64) triplet=aarch64-linux-gnu ;; *) exit 1 ;; esac \
	&& mkdir -p "/pg-dump/usr/lib/$triplet" /pg-dump/usr/share/doc \
	&& mv /libraries/* "/pg-dump/usr/lib/$triplet/" \
	&& for package in postgresql-client-18 libpq5 libldap2 libsasl2-2 libgssapi-krb5-2 libkrb5-3 libk5crypto3 libkrb5support0 libkeyutils1 libcom-err2; do \
		mkdir "/pg-dump/usr/share/doc/$package" && cp "/doc/$package/copyright" "/pg-dump/usr/share/doc/$package/"; \
	done

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
COPY --from=pg-dump /pg-dump/ /
# jellyfin-web, a separate GPL-2.0 program, unmodified, in the folder
# POLYFIN_WEB_DIR names by default; its license, and a notice saying where
# its source is, in the usual documentation folder.
COPY --from=jellyfin-web /jellyfin/jellyfin-web/ /usr/share/polyfin/jellyfin-web/
COPY third_party/jellyfin-web/LICENSE third_party/jellyfin-web/NOTICE /usr/share/doc/jellyfin-web/
COPY --from=build /polyfin /polyfin
# Polyfin's files: the parts of the files being read, which it empties
# when it starts, and by default the Live TV recordings and the database
# backups. A volume, so that it stays writable in a read-only container,
# owned by the user Polyfin runs as, which a new named volume copies.
COPY --from=build --chown=65532:65532 /data /data
VOLUME /data
# NVIDIA's runtime exposes the GPUs and the video and Vulkan libraries to
# containers that ask for it; other runtimes ignore these. Mesa would keep
# a shader cache in a home directory the read-only image does not have.
ENV POLYFIN_LISTEN=:8096 POLYFIN_DATA_DIR=/data NVIDIA_VISIBLE_DEVICES=all NVIDIA_DRIVER_CAPABILITIES=compute,video,utility,graphics \
	MESA_SHADER_CACHE_DISABLE=true
EXPOSE 8096
USER 65532:65532
ENTRYPOINT ["/polyfin"]
CMD ["serve"]
