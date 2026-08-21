set shell := ["bash", "-eu", "-o", "pipefail", "-c"]
set dotenv-load := true

root := justfile_directory()
ipc := "/tmp/instachron/frames.sock"

# Show all recipes.
default:
  @just --list --unsorted

# Show the important local URLs and TCP ports.
urls:
  @printf '%s\n' \
    'Core:' \
    '  camera-web-api HTTP        http://localhost:8080' \
    '  camera-web-api TCP         localhost:9001' \
    '' \
    'Restream services:' \
    '  restreamer HTTP/TCP        http://localhost:8090  localhost:9002' \
    '  enhancer HTTP/TCP          http://localhost:8091  localhost:9003' \
    '  upscaler HTTP/TCP          http://localhost:8092  localhost:9004' \
    '  detector HTTP/TCP          http://localhost:8093  localhost:9005' \
    '' \
    'Recorder and dashboard:' \
    '  recorder API/metrics       http://localhost:8094  http://localhost:8094/metrics' \
    '  dashboard                  http://localhost:3001'

# Create a .env from the sample if one is not present.
env-init:
  @test -f .env || cp .env.example .env
  @printf 'env ready: %s/.env\n' '{{root}}'

# Print local tool availability.
doctor:
  @printf 'just: '; just --version
  @printf 'go: '; command -v go >/dev/null && go version || printf 'not found\n'
  @printf 'gofmt: '; command -v gofmt || printf 'not found\n'
  @printf 'ffmpeg: '; command -v ffmpeg || printf 'not found\n'
  @printf 'docker: '; command -v docker || printf 'not found\n'
  @printf 'tmux: '; command -v tmux || printf 'not found\n'
  @printf 'npm: '; command -v npm || printf 'not found\n'

# Format all Go code using local gofmt.
fmt:
  gofmt -w $(find services shared -name '*.go')

# Format all Go code through the Go Docker image.
fmt-docker:
  docker run --rm -v "{{root}}":/src -w /src golang:1.22-alpine sh -c 'gofmt -w $(find services shared -name "*.go")'

# Fail if any Go file is not gofmt-formatted (checks only, never rewrites).
fmt-check:
  @unformatted="$(gofmt -l $(find services shared -name '*.go'))"; \
  if [ -n "$unformatted" ]; then \
    printf 'these files are not gofmt-formatted (run: just fmt):\n%s\n' "$unformatted"; \
    exit 1; \
  fi

# This repository is a Go workspace made of several separate modules (every
# directory holding its own go.mod). From the repository root `go test ./...`
# matches no packages at all and exits without running a single test, so each
# module directory has to be entered and tested on its own.
# Run all Go tests using the local Go toolchain, one module at a time.
test:
  for m in $(find services shared -name go.mod | xargs -n1 dirname | sort); do \
    printf '\n=== %s ===\n' "$m"; \
    (cd "$m" && go test ./...) || exit 1; \
  done

# Run `go vet` over every module, same per-module walk as `test`.
vet:
  for m in $(find services shared -name go.mod | xargs -n1 dirname | sort); do \
    printf '\n=== %s ===\n' "$m"; \
    (cd "$m" && go vet ./...) || exit 1; \
  done

# Run all Go tests through the Go Docker image.
test-docker:
  docker run --rm -v "{{root}}":/src -w /src golang:1.22-alpine sh -c 'for m in $(find services shared -name go.mod | xargs -n1 dirname | sort); do printf "\n=== %s ===\n" "$m"; (cd "$m" && go test ./...) || exit 1; done'

# Run tests for one service/module through the Go Docker image.
test-docker-module module:
  docker run --rm -v "{{root}}":/src -w /src golang:1.22-alpine sh -c 'go test ./{{module}}/...'

# Build a service Docker image. Example: just docker-build camera-recorder
docker-build service:
  docker build -f services/{{service}}/Dockerfile -t instachron-{{service}}:local .

# Validate compose configuration for every profile.
compose-config:
  docker compose --profile upscaler --profile streaming --profile recording config

# Start the core Docker stack.
compose-up:
  docker compose up --build

# Start Docker stack with recording.
compose-up-recording:
  docker compose --profile recording up --build

# Start Docker stack with streaming.
compose-up-streaming:
  docker compose --profile streaming up --build

# Start Docker stack with upscaler.
compose-up-upscaler:
  docker compose --profile upscaler up --build

# Start Docker stack with every profile.
compose-up-all:
  docker compose --profile upscaler --profile streaming --profile recording up --build

# Stop Docker stack and remove containers.
compose-down:
  docker compose --profile upscaler --profile streaming --profile recording down

# Follow Docker logs. Example: just compose-logs camera-recorder
compose-logs service="":
  docker compose logs -f {{service}}

# Run ESP32 TCP ingest backend locally.
backend tcp_addr="0.0.0.0:5000" ipc_path=ipc max_frame_bytes="5242880" read_timeout="30s":
  mkdir -p "$(dirname '{{ipc_path}}')"
  TCP_ADDR='{{tcp_addr}}' IPC_SOCKET_PATH='{{ipc_path}}' MAX_FRAME_BYTES='{{max_frame_bytes}}' READ_TIMEOUT='{{read_timeout}}' go run ./services/tcp-camera-backend/cmd/tcp-camera-backend

# Run camera-web-api locally. Publishes HTTP on :8080 and streamproto TCP on :9001 by default.
web-api http_addr=":8080" ipc_path=ipc tcp_addr=":9001" camera_config="services/camera-web-api/cameras.json":
  HTTP_ADDR='{{http_addr}}' IPC_SOCKET_PATH='{{ipc_path}}' TCP_ADDR='{{tcp_addr}}' TCP_ENABLED='true' CAMERA_CONFIG='{{camera_config}}' go run ./services/camera-web-api/cmd/camera-web-api

# Run the no-op restream proxy locally.
restreamer upstream="localhost:9001" http_addr=":8090" tcp_addr=":9002":
  HTTP_ADDR='{{http_addr}}' UPSTREAM_TCP_ADDR='{{upstream}}' TCP_ADDR='{{tcp_addr}}' TCP_ENABLED='true' go run ./services/camera-web-restreamer-api/cmd/camera-web-restreamer-api

# Run the enhancement restream proxy locally.
enhancer upstream="localhost:9001" http_addr=":8091" tcp_addr=":9003" config_file="config.json":
  cd services/camera-web-restream-enhancer-api && HTTP_ADDR='{{http_addr}}' UPSTREAM_TCP_ADDR='{{upstream}}' TCP_ADDR='{{tcp_addr}}' TCP_ENABLED='true' CONFIG_FILE='{{config_file}}' go run ./cmd/camera-web-restream-enhancer-api

# Run the Lanczos upscaler restream proxy locally.
upscaler upstream="localhost:9001" http_addr=":8092" tcp_addr=":9004" scale="2" workers="" max_width="960" max_height="540":
  HTTP_ADDR='{{http_addr}}' UPSTREAM_TCP_ADDR='{{upstream}}' TCP_ADDR='{{tcp_addr}}' TCP_ENABLED='true' UPSCALE_FACTOR='{{scale}}' NUM_WORKERS='{{workers}}' MAX_INPUT_WIDTH='{{max_width}}' MAX_INPUT_HEIGHT='{{max_height}}' go run ./services/camera-web-restream-upscaler-api/cmd/camera-web-restream-upscaler-api

# Run the YOLO detector restream proxy locally. Falls back to passthrough if ORT/model are unavailable.
detector upstream="localhost:9001" http_addr=":8093" tcp_addr=":9005" config_file="config.json" ort_lib_path="":
  cd services/camera-web-restream-detector-api && HTTP_ADDR='{{http_addr}}' UPSTREAM_TCP_ADDR='{{upstream}}' TCP_ADDR='{{tcp_addr}}' TCP_ENABLED='true' CONFIG_FILE='{{config_file}}' ORT_LIB_PATH='{{ort_lib_path}}' go run ./cmd/camera-web-restream-detector-api

# Run single-image detector debugging. Example: just detect-image input.jpg output.jpg
detect-image input output="detected.jpg" config_file="config.json" ort_lib_path="":
  cd services/camera-web-restream-detector-api && ORT_LIB_PATH='{{ort_lib_path}}' go run ./cmd/detect-image '{{config_file}}' '{{root}}/{{input}}' '{{root}}/{{output}}'

# Run the H.264 timelapse recorder locally.
recorder upstream="localhost:9001" http_addr=":8094" storage_root="./recordings" output_fps="10" timelapse_factor="10" segment_raw_duration="10m" keep="144":
  CONFIG_FILE='services/camera-recorder/config.json' HTTP_ADDR='{{http_addr}}' UPSTREAM_TCP_ADDR='{{upstream}}' STORAGE_ROOT_DIR='{{storage_root}}' OUTPUT_FPS='{{output_fps}}' TIMELAPSE_FACTOR='{{timelapse_factor}}' SEGMENT_RAW_DURATION='{{segment_raw_duration}}' KEEP_FILES_PER_CAMERA='{{keep}}' go run ./services/camera-recorder/cmd/camera-recorder

# Run the RTMP streamer for a single camera. Example: just stream-camera 2 rtmp://localhost/live/test
stream-camera camera_id="0" stream_url="rtmp://localhost/live/instachron" ipc_path=ipc fps="10":
  STREAM_URL='{{stream_url}}' IPC_SOCKET_PATH='{{ipc_path}}' CAMERA_ID='{{camera_id}}' STREAM_FRAME_RATE='{{fps}}' go run ./services/ffmpeg-streamer/cmd/ffmpeg-streamer --camera-id '{{camera_id}}'

# Run the RTMP streamer in merged-grid mode.
stream-merge stream_url="rtmp://localhost/live/instachron" ipc_path=ipc fps="10" cell_width="320" cell_height="240":
  STREAM_URL='{{stream_url}}' IPC_SOCKET_PATH='{{ipc_path}}' MERGE_ALL='true' STREAM_FRAME_RATE='{{fps}}' CELL_WIDTH='{{cell_width}}' CELL_HEIGHT='{{cell_height}}' go run ./services/ffmpeg-streamer/cmd/ffmpeg-streamer --merge --cell-width '{{cell_width}}' --cell-height '{{cell_height}}'

# Run the Nuxt dashboard against camera-web-api or any compatible restream HTTP API.
dashboard api_base="":
  cd dashboard && NUXT_PUBLIC_CAMERA_API_BASE='{{api_base}}' npm run dev

# Run dashboard pointed at restreamer.
dashboard-restreamer:
  just dashboard 'http://localhost:8090'

# Run dashboard pointed at enhancer.
dashboard-enhancer:
  just dashboard 'http://localhost:8091'

# Run dashboard pointed at upscaler.
dashboard-upscaler:
  just dashboard 'http://localhost:8092'

# Run dashboard pointed at detector.
dashboard-detector:
  just dashboard 'http://localhost:8093'

# Start backend + camera-web-api in tmux.
chain-core session="instachron-core":
  @tmux has-session -t '{{session}}' 2>/dev/null && { echo "tmux session '{{session}}' already exists"; exit 1; } || true
  tmux new-session -d -s '{{session}}' -n backend "cd '{{root}}' && just backend"
  tmux new-window -t '{{session}}' -n web-api "cd '{{root}}' && just web-api"
  @echo "Started {{session}}. Attach with: just attach {{session}}"

# Start core + no-op restreamer in tmux.
chain-restreamer session="instachron-restreamer":
  @tmux has-session -t '{{session}}' 2>/dev/null && { echo "tmux session '{{session}}' already exists"; exit 1; } || true
  tmux new-session -d -s '{{session}}' -n backend "cd '{{root}}' && just backend"
  tmux new-window -t '{{session}}' -n web-api "cd '{{root}}' && just web-api"
  tmux new-window -t '{{session}}' -n restreamer "cd '{{root}}' && sleep 1 && just restreamer localhost:9001 :8090 :9002"
  @echo "Started {{session}}. Attach with: just attach {{session}}"

# Start core + enhancer in tmux.
chain-enhancer session="instachron-enhancer":
  @tmux has-session -t '{{session}}' 2>/dev/null && { echo "tmux session '{{session}}' already exists"; exit 1; } || true
  tmux new-session -d -s '{{session}}' -n backend "cd '{{root}}' && just backend"
  tmux new-window -t '{{session}}' -n web-api "cd '{{root}}' && just web-api"
  tmux new-window -t '{{session}}' -n enhancer "cd '{{root}}' && sleep 1 && just enhancer localhost:9001 :8091 :9003"
  @echo "Started {{session}}. Attach with: just attach {{session}}"

# Start core + upscaler in tmux.
chain-upscaler session="instachron-upscaler":
  @tmux has-session -t '{{session}}' 2>/dev/null && { echo "tmux session '{{session}}' already exists"; exit 1; } || true
  tmux new-session -d -s '{{session}}' -n backend "cd '{{root}}' && just backend"
  tmux new-window -t '{{session}}' -n web-api "cd '{{root}}' && just web-api"
  tmux new-window -t '{{session}}' -n upscaler "cd '{{root}}' && sleep 1 && just upscaler localhost:9001 :8092 :9004"
  @echo "Started {{session}}. Attach with: just attach {{session}}"

# Start core + detector in tmux.
chain-detector session="instachron-detector":
  @tmux has-session -t '{{session}}' 2>/dev/null && { echo "tmux session '{{session}}' already exists"; exit 1; } || true
  tmux new-session -d -s '{{session}}' -n backend "cd '{{root}}' && just backend"
  tmux new-window -t '{{session}}' -n web-api "cd '{{root}}' && just web-api"
  tmux new-window -t '{{session}}' -n detector "cd '{{root}}' && sleep 1 && just detector localhost:9001 :8093 :9005"
  @echo "Started {{session}}. Attach with: just attach {{session}}"

# Start a stacked restream chain: web-api:9001 -> restreamer:9002 -> enhancer:9003 -> upscaler:9004 -> detector:9005.
chain-restream-stack session="instachron-stack":
  @tmux has-session -t '{{session}}' 2>/dev/null && { echo "tmux session '{{session}}' already exists"; exit 1; } || true
  tmux new-session -d -s '{{session}}' -n backend "cd '{{root}}' && just backend"
  tmux new-window -t '{{session}}' -n web-api "cd '{{root}}' && just web-api"
  tmux new-window -t '{{session}}' -n restreamer "cd '{{root}}' && sleep 1 && just restreamer localhost:9001 :8090 :9002"
  tmux new-window -t '{{session}}' -n enhancer "cd '{{root}}' && sleep 2 && just enhancer localhost:9002 :8091 :9003"
  tmux new-window -t '{{session}}' -n upscaler "cd '{{root}}' && sleep 3 && just upscaler localhost:9003 :8092 :9004"
  tmux new-window -t '{{session}}' -n detector "cd '{{root}}' && sleep 4 && just detector localhost:9004 :8093 :9005"
  @echo "Started {{session}}. Attach with: just attach {{session}}"

# Start stacked restream chain plus recorder recording from the final detector TCP output.
chain-record-stack session="instachron-record-stack":
  @tmux has-session -t '{{session}}' 2>/dev/null && { echo "tmux session '{{session}}' already exists"; exit 1; } || true
  tmux new-session -d -s '{{session}}' -n backend "cd '{{root}}' && just backend"
  tmux new-window -t '{{session}}' -n web-api "cd '{{root}}' && just web-api"
  tmux new-window -t '{{session}}' -n restreamer "cd '{{root}}' && sleep 1 && just restreamer localhost:9001 :8090 :9002"
  tmux new-window -t '{{session}}' -n enhancer "cd '{{root}}' && sleep 2 && just enhancer localhost:9002 :8091 :9003"
  tmux new-window -t '{{session}}' -n upscaler "cd '{{root}}' && sleep 3 && just upscaler localhost:9003 :8092 :9004"
  tmux new-window -t '{{session}}' -n detector "cd '{{root}}' && sleep 4 && just detector localhost:9004 :8093 :9005"
  tmux new-window -t '{{session}}' -n recorder "cd '{{root}}' && sleep 5 && just recorder localhost:9005 :8094 ./recordings"
  @echo "Started {{session}}. Attach with: just attach {{session}}"

# Start recorder directly from camera-web-api TCP output.
chain-record-core session="instachron-record-core":
  @tmux has-session -t '{{session}}' 2>/dev/null && { echo "tmux session '{{session}}' already exists"; exit 1; } || true
  tmux new-session -d -s '{{session}}' -n backend "cd '{{root}}' && just backend"
  tmux new-window -t '{{session}}' -n web-api "cd '{{root}}' && just web-api"
  tmux new-window -t '{{session}}' -n recorder "cd '{{root}}' && sleep 1 && just recorder localhost:9001 :8094 ./recordings"
  @echo "Started {{session}}. Attach with: just attach {{session}}"

# Attach to a tmux chain session.
attach session="instachron-stack":
  tmux attach -t '{{session}}'

# Stop a tmux chain session.
stop session="instachron-stack":
  tmux kill-session -t '{{session}}'

# List tmux sessions.
sessions:
  tmux list-sessions

# Quick camera API probe. Example: just cameras http://localhost:8091
cameras api="http://localhost:8080":
  curl -fsS '{{api}}/cameras' | python3 -m json.tool

# Quick recorder video index probe.
videos api="http://localhost:8094" camera_id="":
  curl -fsS '{{api}}/videos?camera_id={{camera_id}}' | python3 -m json.tool
