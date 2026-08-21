module github.com/w0rxbend/instachron/services/camera-web-restreamer-api

go 1.22.5

require (
	github.com/w0rxbend/instachron/shared/livefeed v0.0.0
	github.com/w0rxbend/instachron/shared/streamproto v0.0.0
)

require (
	github.com/w0rxbend/instachron/shared/dropchan v0.0.0 // indirect
	github.com/w0rxbend/instachron/shared/envconf v0.0.0
	github.com/w0rxbend/instachron/shared/mjpeg v0.0.0 // indirect
	github.com/w0rxbend/instachron/shared/webui v0.0.0 // indirect
)

replace (
	github.com/w0rxbend/instachron/shared/livefeed => ../../shared/livefeed
	github.com/w0rxbend/instachron/shared/mjpeg => ../../shared/mjpeg
	github.com/w0rxbend/instachron/shared/streamproto => ../../shared/streamproto
	github.com/w0rxbend/instachron/shared/webui => ../../shared/webui
)

replace github.com/w0rxbend/instachron/shared/envconf => ../../shared/envconf

replace github.com/w0rxbend/instachron/shared/dropchan => ../../shared/dropchan
