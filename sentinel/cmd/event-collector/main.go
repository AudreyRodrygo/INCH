package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

const serviceName = "event-collector"

func main() {
	if err := run(); err != nil {
		log.Fatalf("%s: %v", serviceName, err)
	}
}

// run is the real entrypoint. Returning an error instead of calling log.Fatal
// directly allows deferred cleanup functions to execute properly.
func run() error {
	// Create a context that is cancelled on SIGINT or SIGTERM.
	// This is the standard Go pattern for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("[%s] starting...\n", serviceName)
	fmt.Printf("[%s] press Ctrl+C to stop\n", serviceName)

	// TODO: Initialize dependencies (config, logger, DB, Kafka, gRPC server).
	// TODO: Start gRPC server in a goroutine.

	// Block until shutdown signal.
	<-ctx.Done()

	fmt.Printf("\n[%s] shutting down gracefully...\n", serviceName)

	// TODO: Drain connections, flush buffers, close resources.

	fmt.Printf("[%s] stopped\n", serviceName)
	return nil
}
