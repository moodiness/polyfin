GO ?= go
NPM ?= npm
VERSION ?= dev
# The PostgreSQL image-health starts the image against, on the host.
IMAGE_DATABASE_URL ?= postgresql://polyfin:polyfin@host.docker.internal:5432/polyfin

.PHONY: web build dev check lint test test-race-1 test-race-2 test-jellyfin-1 test-jellyfin-2 bounds compose-check image-check image-health

web:
	$(NPM) --prefix web ci --no-audit --no-fund
	$(NPM) --prefix web run build

build: web
	CGO_ENABLED=0 $(GO) build -tags production -trimpath -ldflags "-X main.version=$(VERSION)" -o bin/polyfin ./cmd/polyfin

# Serves web/dist from disk; set POLYFIN_DATABASE_URL first.
dev: web
	$(GO) run ./cmd/polyfin serve

# Database tests run only when POLYFIN_TEST_DATABASE_URL is set.
check: web lint test bounds

# Formatting and vet; the admin app's formatting needs make web first.
lint:
	$(NPM) --prefix web run format:check
	@unformatted="$$(gofmt -l cmd internal web/*.go)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed:"; printf '%s\n' "$$unformatted"; exit 1; \
	fi
	$(GO) vet ./...

# Every package's tests under the race detector. CI runs them in four jobs
# at once: internal/jellyfin, by far the longest package, in two halves
# (test-jellyfin-1 and -2), and the other packages in two shares
# (test-race-1 and -2), balanced by their times on CI's runners.
POLYFIN := github.com/moodiness/polyfin/internal
JELLYFIN := $(POLYFIN)/jellyfin

# go test starts the packages' tests in the order given, 4 at once on CI:
# the slowest go first, so that none of them starts last and runs alone.
# test-race-2 takes these, then every package neither list names, so that
# a new package is tested without being listed.
RACE_1 := $(addprefix $(POLYFIN)/,playback backup admin library source thumbnails commercials \
	notifications audiosegments trackers iptv jellyfinimport recordings lyrics)
RACE_2 := $(addprefix $(POLYFIN)/,localfiles hls accounts database container torrent addons \
	torrent/utp appimport desktop/cluster)
RACE_REST = $$($(GO) list ./... | grep -vxF $(addprefix -e ,$(JELLYFIN) $(RACE_1) $(RACE_2)))

# internal/jellyfin's halves, by the first letter after Test of each
# test's name: tests are run by -run and left out by -skip with the same
# expression, so that each test runs in one half, a new one too.
JELLYFIN_1 := ^Test[A-K]

test:
	$(GO) test -race -timeout 18m $(JELLYFIN) $(RACE_1) $(RACE_2) $(RACE_REST)

test-race-1:
	$(GO) test -race $(RACE_1)

test-race-2:
	$(GO) test -race $(RACE_2) $(RACE_REST)

# Whole, under the race detector, this package took about 9 minutes on
# CI's runners, near go test's default of 10, so a slower runner timed out
# with no test failing: each half keeps the larger bound.
test-jellyfin-1:
	$(GO) test -race -timeout 18m -run '$(JELLYFIN_1)' $(JELLYFIN)

test-jellyfin-2:
	$(GO) test -race -timeout 18m -skip '$(JELLYFIN_1)' $(JELLYFIN)

# Time bounds checked without the race detector, which slows the work they
# time tenfold.
bounds:
	$(GO) test -run '^TestLargeLineupsStayFast$$' ./internal/iptv

# An empty env file keeps a developer's private .env out of the validation.
compose-check:
	POSTGRES_PASSWORD=config-only docker compose --env-file /dev/null -f compose.yaml config --quiet
	POSTGRES_PASSWORD=config-only docker compose --env-file /dev/null -f compose.yaml -f compose.build.yaml config --quiet
	POSTGRES_PASSWORD=config-only docker compose --env-file /dev/null -f compose.yaml -f compose.host.yaml config --quiet

# Local tag only: this target never pushes. It also checks that the
# jellyfin-web source releases attach still matches its checksum.
image-check:
	docker buildx build --load --build-arg VERSION=$(VERSION) --tag polyfin-local:check .
	@printed="$$(docker run --rm --network none polyfin-local:check version)"; \
	if [ "$$printed" != "$(VERSION)" ]; then \
		echo "image reports version '$$printed', expected '$(VERSION)'"; exit 1; \
	fi
	docker buildx build --platform linux/amd64,linux/arm64 --build-arg VERSION=$(VERSION) --output type=cacheonly .
	docker buildx build --target jellyfin-web-source --output type=cacheonly .

# The image image-check built becomes healthy within its start period,
# 2 minutes, against the PostgreSQL of IMAGE_DATABASE_URL.
image-health:
	docker run -d --name polyfin-health --add-host host.docker.internal:host-gateway \
		-e POLYFIN_DATABASE_URL='$(IMAGE_DATABASE_URL)' polyfin-local:check
	@status=starting; tries=0; \
	while [ "$$status" = starting ] && [ $$tries -lt 60 ]; do \
		sleep 2; tries=$$((tries + 1)); \
		status="$$(docker inspect --format '{{if .State.Running}}{{.State.Health.Status}}{{else}}exited{{end}}' polyfin-health)"; \
	done; \
	echo "image health: $$status after $$((tries * 2)) seconds"; \
	if [ "$$status" != healthy ]; then docker logs polyfin-health; fi; \
	docker rm -f polyfin-health >/dev/null; \
	[ "$$status" = healthy ]
