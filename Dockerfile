FROM golang:1.25-alpine AS builder

ENV GOTOOLCHAIN=auto

RUN apk add --no-cache git ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN set -eu; \
        TRADER_COMMIT="$(git rev-parse --short HEAD)"; \
        TRADER_BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"; \
        if TRADER_RELEASE="$(git describe --tags --exact-match 2>/dev/null)"; then \
            TRADER_RELEASE="$(echo "${TRADER_RELEASE}" | tr -d '[:space:]')"; \
        else \
            BASE_TAG="$(git describe --tags --abbrev=0 2>/dev/null || true)"; \
            if [ -z "${BASE_TAG}" ]; then \
                echo "ERROR: no git tags found. Create the first release tag (e.g. v0.0.1)."; \
                exit 1; \
            fi; \
            COMMITS_SINCE_TAG="$(git rev-list --count "${BASE_TAG}"..HEAD)"; \
            BUILD_STAMP="$(date -u +%Y%m%d%H%M%S)"; \
            TRADER_RELEASE="${BASE_TAG}-dev.${COMMITS_SINCE_TAG}+${BUILD_STAMP}.${TRADER_COMMIT}"; \
        fi; \
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
        go build -ldflags="-s -w -X main.release=${TRADER_RELEASE} -X main.commit=${TRADER_COMMIT} -X main.buildTime=${TRADER_BUILD_TIME}" \
        -o trader \
        cmd/trader/main.go

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/trader ./trader

ENTRYPOINT ["./trader"]
CMD ["-c", "conf/config.yaml"]
