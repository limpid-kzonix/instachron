module github.com/w0rxbend/instachron/services/camera-recorder

go 1.22.5

require github.com/w0rxbend/instachron/shared/streamproto v0.0.0

require (
	github.com/disintegration/imaging v1.6.2 // indirect
	golang.org/x/image v0.0.0-20191009234506-e7c1f5e7dbb8 // indirect
)

require (
	github.com/w0rxbend/instachron/shared/dropchan v0.0.0
	github.com/w0rxbend/instachron/shared/envconf v0.0.0
	github.com/w0rxbend/instachron/shared/imageutil v0.0.0
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
