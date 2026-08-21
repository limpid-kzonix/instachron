module github.com/w0rxbend/instachron/shared/livefeed

go 1.22.5

require (
	github.com/w0rxbend/instachron/shared/dropchan v0.0.0
	github.com/w0rxbend/instachron/shared/mjpeg v0.0.0
	github.com/w0rxbend/instachron/shared/streamproto v0.0.0
	github.com/w0rxbend/instachron/shared/webui v0.0.0
)

replace (
	github.com/w0rxbend/instachron/shared/mjpeg => ../mjpeg
	github.com/w0rxbend/instachron/shared/streamproto => ../streamproto
	github.com/w0rxbend/instachron/shared/webui => ../webui
)

replace github.com/w0rxbend/instachron/shared/dropchan => ../dropchan
