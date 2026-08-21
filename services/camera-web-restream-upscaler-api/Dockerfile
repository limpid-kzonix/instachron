# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder
WORKDIR /src

COPY shared/dropchan/go.mod     shared/dropchan/
COPY shared/envconf/go.mod      shared/envconf/
COPY shared/imageutil/go.mod    shared/imageutil/
COPY shared/livefeed/go.mod     shared/livefeed/
COPY shared/mjpeg/go.mod        shared/mjpeg/
COPY shared/streamproto/go.mod  shared/streamproto/
COPY shared/webui/go.mod        shared/webui/
COPY services/camera-web-restream-upscaler-api/go.mod \
     services/camera-web-restream-upscaler-api/go.sum \
     services/camera-web-restream-upscaler-api/
WORKDIR /src/services/camera-web-restream-upscaler-api
RUN GOWORK=off go mod download

WORKDIR /src
COPY shared/dropchan/     shared/dropchan/
COPY shared/envconf/      shared/envconf/
COPY shared/imageutil/    shared/imageutil/
COPY shared/livefeed/     shared/livefeed/
COPY shared/mjpeg/        shared/mjpeg/
COPY shared/streamproto/  shared/streamproto/
COPY shared/webui/        shared/webui/
COPY services/camera-web-restream-upscaler-api/       services/camera-web-restream-upscaler-api/
WORKDIR /src/services/camera-web-restream-upscaler-api
RUN GOWORK=off CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/camera-web-restream-upscaler-api ./cmd/camera-web-restream-upscaler-api

# ── Runtime stage ─────────────────────────────────────────────────────────────
# Pure static binary — no OS needed.
FROM scratch
COPY --from=builder /out/camera-web-restream-upscaler-api /camera-web-restream-upscaler-api

EXPOSE 8092
ENTRYPOINT ["/camera-web-restream-upscaler-api"]
