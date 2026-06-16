FROM golang:1.23-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG TARGETVARIANT
RUN GOARM="${TARGETVARIANT#v}" && \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" GOARM="${GOARM}" go build \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
      -o /out/openpt ./cmd/openpt

FROM alpine:3.20

RUN apk add --no-cache ca-certificates su-exec tzdata && \
    addgroup -S openpt && \
    adduser -S -G openpt openpt && \
    mkdir -p /data/torrents/archived /data/clients && \
    chown -R openpt:openpt /data

WORKDIR /app
COPY --from=build /out/openpt /usr/local/bin/openpt
COPY examples /app/examples
COPY clients /app/clients
COPY docker/entrypoint.sh /usr/local/bin/openpt-entrypoint

RUN chmod +x /usr/local/bin/openpt-entrypoint

VOLUME ["/data"]
ENTRYPOINT ["openpt-entrypoint"]
CMD ["--config", "/data/config.json"]
