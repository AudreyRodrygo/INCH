package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

const serviceName = "gateway-api"

func main() {
	if err := run(); err != nil {
		log.Fatalf("%s: %v", serviceName, err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("[%s] starting...\n", serviceName)
	fmt.Printf("[%s] press Ctrl+C to stop\n", serviceName)

	// TODO: Initialize config, logger, REST server, gRPC server, priority queue, NATS publisher.
	// TODO: Start HTTP and gRPC servers.

	<-ctx.Done()

	fmt.Printf("\n[%s] shutting down gracefully...\n", serviceName)
	fmt.Printf("[%s] stopped\n", serviceName)
	return nil
}
