# PostgreSQL 18's programs for Polyfin Desktop on Linux, built from the
# official source, for the platform the build runs for (linux/amd64 or
# linux/arm64): scripts/packaging/linux.sh builds it with
#   docker buildx build --platform linux/<arch> --output type=local,dest=<folder> -f <this file> .
# and gets bin, lib, share and licenses, laid out as the packages want them.
#
# Why a build of its own: the packages need a PostgreSQL that runs from any
# folder, with ICU (the app's cluster sorts with ICU's root collation), and
# with pg_dump and pg_restore for backups. PostgreSQL's own Debian and Ubuntu
# packages look for their files at fixed paths and load the system's
# libraries of one release; the widely used relocatable builds either leave
# out pg_dump and pg_restore or ICU. Built here on Ubuntu 22.04, the oldest
# release still supported, with glibc 2.35, its programs run on it and on
# every later Debian, Ubuntu, Fedora or Arch. ICU 70 is carried in lib, which
# the programs find beside them; libstdc++ and zlib are the system's.

FROM ubuntu:22.04@sha256:0a9887931b0e38af2e3487ed39e29436b0535c4f3f52e57745baaf79d1898956 AS build
ADD --checksum=sha256:555610c24d53e4316da5b7d3fc25c279d96856d5e0e23ee308c328c5fa881d9f \
	https://ftp.postgresql.org/pub/source/v18.6/postgresql-18.6.tar.bz2 /src/postgresql.tar.bz2
# The build's own tools, from Ubuntu 22.04's archive.
# hadolint ignore=DL3008
RUN apt-get update \
	&& DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
		bzip2 gcc make libc6-dev pkg-config bison flex perl libicu-dev zlib1g-dev patchelf
WORKDIR /src
# A prefix with "pgsql" in it keeps bin, lib and share flat, as
# EnterpriseDB's builds for Windows and macOS have them. PostgreSQL finds
# share and lib relative to its programs, wherever the folder goes. No
# readline (no one types into psql here) and no OpenSSL (the cluster
# listens on the loopback only).
RUN tar -xjf postgresql.tar.bz2 --strip-components=1 \
	&& ./configure --prefix=/usr/local/pgsql --with-icu --without-readline --disable-rpath \
	&& make -j"$(nproc)" -s \
	&& make -s install \
	&& rm -rf /usr/local/pgsql/include /usr/local/pgsql/lib/pgxs /usr/local/pgsql/lib/pkgconfig /usr/local/pgsql/lib/*.a
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
# ICU's libraries go in lib, by the names the programs load them by; every
# program and library looks in lib first: $ORIGIN, the folder of each, is
# for the loader to expand, not the shell.
WORKDIR /usr/local/pgsql
# hadolint ignore=SC2016
RUN for name in libicuuc.so.70 libicui18n.so.70 libicudata.so.70; do cp -L "/usr/lib/$(gcc -print-multiarch)/$name" lib/; done \
	&& for f in bin/*; do patchelf --set-rpath '$ORIGIN/../lib' "$f"; done \
	&& for f in lib/*.so*; do if [ ! -L "$f" ]; then patchelf --set-rpath '$ORIGIN' "$f"; fi; done \
	&& mkdir -p licenses/postgresql licenses/icu \
	&& cp /src/COPYRIGHT licenses/postgresql/ \
	&& cp /usr/share/doc/libicu70/copyright licenses/icu/ \
	&& ./bin/postgres --version && ./bin/pg_dump --version \
	&& ldd bin/postgres | grep -q '/usr/local/pgsql/bin/../lib/libicuuc.so.70'

FROM scratch
COPY --from=build /usr/local/pgsql/ /
