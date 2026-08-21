FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY shared/envconf/      shared/envconf/
COPY shared/frameipc/     shared/frameipc/
COPY shared/imageutil/    shared/imageutil/
COPY shared/streamproto/  shared/streamproto/
COPY services/tcp-camera-backend/  services/tcp-camera-backend/
WORKDIR /src/services/tcp-camera-backend
RUN GOWORK=off CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/tcp-camera-backend ./cmd/tcp-camera-backend

FROM alpine:3.21
COPY --from=builder /out/tcp-camera-backend /usr/local/bin/tcp-camera-backend
EXPOSE 5000
ENTRYPOINT ["tcp-camera-backend"]
