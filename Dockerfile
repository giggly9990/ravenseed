# Base images are pinned by digest; Dependabot (docker ecosystem) keeps the
# tag and digest current. Build stages run on the build machine's platform
# and cross-compile, so multi-arch builds don't emulate Node or Go.
FROM --platform=$BUILDPLATFORM node:26-alpine@sha256:40cd011b55415bc894e45b02fdd5a5dfde056921b7d23a64c74f80497af48797 AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine@sha256:27f2d3b83cfc5803c8078f8e730af0f7c431bf067afaec8f60a36fb763187cba AS go-base
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/

FROM go-base AS test
RUN apk add --no-cache build-base
RUN go vet ./... && go test -race ./...

FROM go-base AS build
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/ravenseed ./cmd/server

# Built only for the mock target, never for runtime.
FROM go-base AS build-mock
ARG TARGETOS=linux
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/mockqui ./cmd/mockqui

FROM alpine:3.23@sha256:ff7f0c727dd796aa36787bfcd9a64bf35a6ce72a73451b3359fde849b6703d63 AS mock
COPY --from=build-mock /out/mockqui /mockqui
USER 10001:10001
EXPOSE 7476
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD ["wget", "-q", "-O", "/dev/null", "--header", "X-API-Key: demo-key", "http://127.0.0.1:7476/api/instances"]
ENTRYPOINT ["/mockqui"]

FROM alpine:3.23@sha256:ff7f0c727dd796aa36787bfcd9a64bf35a6ce72a73451b3359fde849b6703d63 AS runtime
RUN apk add --no-cache ca-certificates tzdata && addgroup -g 10001 ravenseed && adduser -D -u 10001 -G ravenseed ravenseed && mkdir /data && chown ravenseed:ravenseed /data
WORKDIR /app
COPY --from=build /out/ravenseed /app/ravenseed
COPY --from=web /src/web/dist/ /app/web/
ENV WEB_DIR=/app/web DATABASE_PATH=/data/discovery.db LISTEN_ADDR=0.0.0.0 LISTEN_PORT=8080
USER 10001:10001
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/app/ravenseed", "healthcheck"]
ENTRYPOINT ["/app/ravenseed"]
