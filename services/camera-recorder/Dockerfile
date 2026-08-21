FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY shared/dropchan/go.mod     shared/dropchan/
COPY shared/envconf/go.mod      shared/envconf/
COPY shared/imageutil/go.mod    shared/imageutil/
COPY shared/livefeed/go.mod     shared/livefeed/
COPY shared/mjpeg/go.mod        shared/mjpeg/
COPY shared/streamproto/go.mod  shared/streamproto/
COPY shared/webui/go.mod        shared/webui/
COPY services/camera-recorder/go.mod                services/camera-recorder/
WORKDIR /src/services/camera-recorder
RUN GOWORK=off go mod download

WORKDIR /src
COPY shared/dropchan/     shared/dropchan/
COPY shared/envconf/      shared/envconf/
COPY shared/imageutil/    shared/imageutil/
COPY shared/livefeed/     shared/livefeed/
COPY shared/mjpeg/        shared/mjpeg/
COPY shared/streamproto/  shared/streamproto/
COPY shared/webui/        shared/webui/
COPY services/camera-recorder/                      services/camera-recorder/
WORKDIR /src/services/camera-recorder
RUN GOWORK=off CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/camera-recorder ./cmd/camera-recorder

FROM alpine:3.21
RUN apk add --no-cache ffmpeg
WORKDIR /app
COPY --from=builder /out/camera-recorder /usr/local/bin/camera-recorder
COPY --from=builder /src/services/camera-recorder/config.json /app/config.json
ENV CONFIG_FILE=/app/config.json
EXPOSE 8094
VOLUME ["/recordings"]
ENTRYPOINT ["camera-recorder"]
