FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY shared/envconf/      shared/envconf/
COPY shared/frameipc/     shared/frameipc/
COPY shared/streamproto/  shared/streamproto/
COPY services/ffmpeg-streamer/ services/ffmpeg-streamer/
WORKDIR /src/services/ffmpeg-streamer
RUN GOWORK=off CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/ffmpeg-streamer ./cmd/ffmpeg-streamer

FROM alpine:3.21
RUN apk add --no-cache ffmpeg
COPY --from=builder /out/ffmpeg-streamer /usr/local/bin/ffmpeg-streamer
ENTRYPOINT ["ffmpeg-streamer"]
