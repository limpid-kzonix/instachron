package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/w0rxbend/instachron/services/camera-web-api/internal/app"
)

func main() {
	// The signal context lives here so that every deferred cleanup inside Run
	// gets to execute: log.Fatal below is reached only after Run has returned.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
