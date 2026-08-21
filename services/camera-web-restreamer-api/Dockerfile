FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY shared/envconf/      shared/envconf/
COPY shared/livefeed/     shared/livefeed/
COPY shared/mjpeg/        shared/mjpeg/
COPY shared/streamproto/  shared/streamproto/
COPY shared/webui/        shared/webui/
COPY services/camera-web-restreamer-api/   services/camera-web-restreamer-api/
WORKDIR /src/services/camera-web-restreamer-api
RUN GOWORK=off CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/camera-web-restreamer-api ./cmd/camera-web-restreamer-api

FROM alpine:3.21
COPY --from=builder /out/camera-web-restreamer-api /usr/local/bin/camera-web-restreamer-api
EXPOSE 8090
ENTRYPOINT ["camera-web-restreamer-api"]
