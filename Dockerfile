# Both build stages run natively on the builder and cross-compile, so the final
# image needs no emulation for linux/amd64 or linux/arm64.
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
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -tags production -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /polyfin ./cmd/polyfin

# Static binary only: no shell, no package manager, no RUN in this stage.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
COPY --from=build /polyfin /polyfin
ENV POLYFIN_LISTEN=:8096
EXPOSE 8096
USER 65532:65532
ENTRYPOINT ["/polyfin"]
CMD ["serve"]
