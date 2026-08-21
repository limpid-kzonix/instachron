module github.com/w0rxbend/instachron/services/ffmpeg-streamer

go 1.22.5

require (
	github.com/w0rxbend/instachron/shared/envconf v0.0.0
	github.com/w0rxbend/instachron/shared/frameipc v0.0.0
	github.com/w0rxbend/instachron/shared/streamproto v0.0.0
)

replace github.com/w0rxbend/instachron/shared/frameipc => ../../shared/frameipc

replace github.com/w0rxbend/instachron/shared/envconf => ../../shared/envconf

replace github.com/w0rxbend/instachron/shared/streamproto => ../../shared/streamproto
