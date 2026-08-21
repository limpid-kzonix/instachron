module github.com/w0rxbend/instachron/services/camera-web-restream-upscaler-api

go 1.22.5

require (
	github.com/disintegration/imaging v1.6.2
	github.com/w0rxbend/instachron/shared/livefeed v0.0.0
	github.com/w0rxbend/instachron/shared/streamproto v0.0.0
)

require (
	github.com/w0rxbend/instachron/shared/dropchan v0.0.0
	github.com/w0rxbend/instachron/shared/envconf v0.0.0
	github.com/w0rxbend/instachron/shared/imageutil v0.0.0
	github.com/w0rxbend/instachron/shared/mjpeg v0.0.0 // indirect
	github.com/w0rxbend/instachron/shared/webui v0.0.0 // indirect
	golang.org/x/image v0.0.0-20191009234506-e7c1f5e7dbb8 // indirect
)

replace (
	github.com/w0rxbend/instachron/shared/livefeed => ../../shared/livefeed
	github.com/w0rxbend/instachron/shared/mjpeg => ../../shared/mjpeg
	github.com/w0rxbend/instachron/shared/streamproto => ../../shared/streamproto
	github.com/w0rxbend/instachron/shared/webui => ../../shared/webui
)

replace github.com/w0rxbend/instachron/shared/envconf => ../../shared/envconf

replace github.com/w0rxbend/instachron/shared/imageutil => ../../shared/imageutil

replace github.com/w0rxbend/instachron/shared/dropchan => ../../shared/dropchan
