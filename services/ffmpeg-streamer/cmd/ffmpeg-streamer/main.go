package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/w0rxbend/instachron/services/ffmpeg-streamer/internal/app"
)

func main() {
	// Cancel the context when the process is asked to stop, so the streamer can
	// shut ffmpeg down instead of being killed mid-write. SIGTERM is what a
	// container runtime sends on "docker stop"; os.Interrupt is Ctrl-C.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
