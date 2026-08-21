FROM golang:1.22-alpine AS builder
WORKDIR /src

# go.mod + go.sum first for layer-cached dependency download
COPY shared/envconf/go.mod      shared/envconf/
COPY shared/imageutil/go.mod    shared/imageutil/
COPY shared/livefeed/go.mod     shared/livefeed/
COPY shared/mjpeg/go.mod        shared/mjpeg/
COPY shared/streamproto/go.mod  shared/streamproto/
COPY shared/webui/go.mod        shared/webui/
COPY services/camera-web-restream-enhancer-api/go.mod \
     services/camera-web-restream-enhancer-api/go.sum \
     services/camera-web-restream-enhancer-api/
WORKDIR /src/services/camera-web-restream-enhancer-api
RUN GOWORK=off go mod download

WORKDIR /src
COPY shared/envconf/      shared/envconf/
COPY shared/imageutil/    shared/imageutil/
COPY shared/livefeed/     shared/livefeed/
COPY shared/mjpeg/        shared/mjpeg/
COPY shared/streamproto/  shared/streamproto/
COPY shared/webui/        shared/webui/
COPY services/camera-web-restream-enhancer-api/     services/camera-web-restream-enhancer-api/
WORKDIR /src/services/camera-web-restream-enhancer-api
RUN GOWORK=off CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/camera-web-restream-enhancer-api ./cmd/camera-web-restream-enhancer-api

FROM alpine:3.21
COPY --from=builder /out/camera-web-restream-enhancer-api /usr/local/bin/camera-web-restream-enhancer-api
EXPOSE 8091
ENTRYPOINT ["camera-web-restream-enhancer-api"]
