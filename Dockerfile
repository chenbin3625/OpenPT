# ---- 前端构建（Vite + React + antd，产物输出到 ../internal/web/dist）----
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ---- Go 构建 ----
FROM --platform=$BUILDPLATFORM golang:1.26.5-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /src/internal/web/dist ./internal/web/dist
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG TARGETVARIANT
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
RUN GOARM="${TARGETVARIANT#v}" && \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" GOARM="${GOARM}" go build \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
      -o /out/openpt ./cmd/openpt && \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" GOARM="${GOARM}" go build \
      -trimpath \
      -ldflags="-s -w" \
      -o /out/openpt-entrypoint ./cmd/openpt-entrypoint

FROM --platform=$BUILDPLATFORM alpine:3.22 AS runtime-files

RUN apk add --no-cache ca-certificates tzdata

FROM alpine:3.22

WORKDIR /app
COPY --from=runtime-files /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=runtime-files /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /out/openpt /usr/local/bin/openpt
COPY --from=build /out/openpt-entrypoint /usr/local/bin/openpt-entrypoint
COPY examples /app/examples
COPY clients /app/clients

VOLUME ["/data"]
EXPOSE 9090
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD wget -q -T 2 -O /dev/null http://127.0.0.1:9090/healthz || exit 1
ENTRYPOINT ["openpt-entrypoint"]
CMD ["--config", "/data/config.toml"]
