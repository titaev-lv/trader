FROM golang:1.25-alpine AS builder

ENV GOTOOLCHAIN=auto

RUN apk add --no-cache git ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN TRADER_VERSION="$(cat VERSION)" && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -X main.version=${TRADER_VERSION}" \
    -o trader \
    cmd/trader/main.go

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/trader ./trader

ENTRYPOINT ["./trader"]
CMD ["-c", "conf/config.yaml"]
