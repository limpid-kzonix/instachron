ARG ONNXRUNTIME_VERSION=1.19.2

# ── ORT shared library download ───────────────────────────────────────────────
FROM debian:bookworm-slim AS ort
ARG ONNXRUNTIME_VERSION
RUN apt-get update && apt-get install -y --no-install-recommends wget ca-certificates && \
    wget -q "https://github.com/microsoft/onnxruntime/releases/download/v${ONNXRUNTIME_VERSION}/onnxruntime-linux-x64-${ONNXRUNTIME_VERSION}.tgz" && \
    tar -xzf "onnxruntime-linux-x64-${ONNXRUNTIME_VERSION}.tgz" && \
    mv "onnxruntime-linux-x64-${ONNXRUNTIME_VERSION}" /ort

# ── Go build (CGO required for onnxruntime_go) ────────────────────────────────
FROM golang:1.22 AS builder
WORKDIR /src

COPY shared/dropchan/go.mod     shared/dropchan/
COPY shared/envconf/go.mod      shared/envconf/
COPY shared/imageutil/go.mod    shared/imageutil/
COPY shared/livefeed/go.mod     shared/livefeed/
COPY shared/mjpeg/go.mod        shared/mjpeg/
COPY shared/streamproto/go.mod  shared/streamproto/
COPY shared/webui/go.mod        shared/webui/
COPY services/camera-web-restream-detector-api/go.mod \
     services/camera-web-restream-detector-api/go.sum \
     services/camera-web-restream-detector-api/
WORKDIR /src/services/camera-web-restream-detector-api
RUN GOWORK=off go mod download

WORKDIR /src
COPY shared/dropchan/     shared/dropchan/
COPY shared/envconf/      shared/envconf/
COPY shared/imageutil/    shared/imageutil/
COPY shared/livefeed/     shared/livefeed/
COPY shared/mjpeg/        shared/mjpeg/
COPY shared/streamproto/  shared/streamproto/
COPY shared/webui/        shared/webui/
COPY services/camera-web-restream-detector-api/ services/camera-web-restream-detector-api/

WORKDIR /src/services/camera-web-restream-detector-api
RUN GOWORK=off CGO_ENABLED=1 \
    go build -ldflags="-s -w" \
    -o /out/camera-web-restream-detector-api \
    ./cmd/camera-web-restream-detector-api

# ── Runtime image ─────────────────────────────────────────────────────────────
FROM debian:bookworm-slim
COPY --from=ort /ort/lib/libonnxruntime.so* /usr/local/lib/
RUN ldconfig

COPY --from=builder /out/camera-web-restream-detector-api /usr/local/bin/camera-web-restream-detector-api

ENV ORT_LIB_PATH=libonnxruntime.so
EXPOSE 8093
ENTRYPOINT ["camera-web-restream-detector-api"]
