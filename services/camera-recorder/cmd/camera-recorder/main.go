package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/w0rxbend/instachron/services/camera-recorder/internal/app"
)

func main() {
	// The context is cancelled on Ctrl-C or on the SIGTERM a container runtime
	// sends when stopping the service, which tells app.Run to shut down.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
