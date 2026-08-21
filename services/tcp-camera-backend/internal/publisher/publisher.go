package publisher

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"github.com/w0rxbend/instachron/shared/streamproto"

	"github.com/w0rxbend/instachron/shared/frameipc"
)

const ipcChannelSize = 64

type consumerConn struct {
	conn net.Conn
	ch   chan frameipc.Frame
}

// Publisher listens on a Unix domain socket and fans out IPC messages
// to every connected consumer (camera-web-api, ffmpeg-streamer, etc.).
// Multiple camera goroutines call Publish concurrently; serialisation happens
// inside each per-consumer writer goroutine so the hot path never blocks.
type Publisher struct {
	socketPath string
	logger     *log.Logger

	mu        sync.Mutex
	consumers map[*consumerConn]struct{}
}

func New(socketPath string, logger *log.Logger) *Publisher {
	return &Publisher{
		socketPath: socketPath,
		logger:     logger,
		consumers:  make(map[*consumerConn]struct{}),
	}
}

func (p *Publisher) Listen(ctx context.Context) error {
	if err := os.Remove(p.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket %s: %w", p.socketPath, err)
	}

	ln, err := net.Listen("unix", p.socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", p.socketPath, err)
	}

	var wg sync.WaitGroup

	go func() {
		<-ctx.Done()
		_ = ln.Close()
		p.mu.Lock()
		for c := range p.consumers {
			_ = c.conn.Close()
		}
		p.mu.Unlock()
	}()

	defer func() {
		_ = ln.Close()
		_ = os.Remove(p.socketPath)
		wg.Wait()
	}()

	p.logger.Printf("IPC publisher listening on %s", p.socketPath)

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			p.logger.Printf("IPC accept error: %v", err)
			continue
		}

		c := &consumerConn{
			conn: conn,
			ch:   make(chan frameipc.Frame, ipcChannelSize),
		}

		p.mu.Lock()
		p.consumers[c] = struct{}{}
		p.mu.Unlock()

		wg.Add(1)
		go func() {
			defer wg.Done()
			p.serveConsumer(ctx, c)
		}()
	}
}

func (p *Publisher) serveConsumer(ctx context.Context, c *consumerConn) {
	defer func() {
		_ = c.conn.Close()
		p.mu.Lock()
		delete(p.consumers, c)
		p.mu.Unlock()
		p.logger.Printf("IPC consumer disconnected: %s", c.conn.RemoteAddr())
	}()

	p.logger.Printf("IPC consumer connected: %s", c.conn.RemoteAddr())

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-c.ch:
			if !ok {
				return
			}
			if err := frameipc.Write(c.conn, msg); err != nil {
				return
			}
		}
	}
}

func (p *Publisher) Publish(cameraID streamproto.CameraID, jpeg []byte) {
	p.fanOut(frameipc.Frame{Kind: frameipc.KindFrame, CameraID: cameraID, Payload: jpeg})
}

func (p *Publisher) PublishOffline(cameraID streamproto.CameraID) {
	p.fanOut(frameipc.Frame{Kind: frameipc.KindOffline, CameraID: cameraID})
}

func (p *Publisher) fanOut(msg frameipc.Frame) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for c := range p.consumers {
		select {
		case c.ch <- msg:
		default:
			// slow consumer — drop rather than block the camera ingestion path
		}
	}
}
