GO ?= go
NPM ?= npm
VERSION ?= dev

.PHONY: web build dev check compose-check image-check

web:
	$(NPM) --prefix web ci --no-audit --no-fund
	$(NPM) --prefix web run build

build: web
	CGO_ENABLED=0 $(GO) build -tags production -trimpath -ldflags "-X main.version=$(VERSION)" -o bin/polyfin ./cmd/polyfin

# Serves web/dist from disk; set POLYFIN_DATABASE_URL first.
dev: web
	$(GO) run ./cmd/polyfin serve

# Database tests run only when POLYFIN_TEST_DATABASE_URL is set.
check: web
	$(NPM) --prefix web run format:check
	@unformatted="$$(gofmt -l cmd internal web/*.go)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed:"; printf '%s\n' "$$unformatted"; exit 1; \
	fi
	$(GO) vet ./...
	$(GO) test -race ./...

# An empty env file keeps a developer's private .env out of the validation.
compose-check:
	POSTGRES_PASSWORD=config-only docker compose --env-file /dev/null -f compose.yaml config --quiet
	POSTGRES_PASSWORD=config-only docker compose --env-file /dev/null -f compose.yaml -f compose.build.yaml config --quiet

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
