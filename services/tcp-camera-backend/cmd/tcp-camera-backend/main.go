package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/w0rxbend/instachron/services/tcp-camera-backend/internal/app"
)

func main() {
	// The context is cancelled when the process is asked to stop (Ctrl-C sends
	// SIGINT, most supervisors send SIGTERM), which tells app.Run to shut down.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
