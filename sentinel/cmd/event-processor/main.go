package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

const serviceName = "event-processor"

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

	// TODO: Initialize config, logger, Kafka consumer, rule engine, PostgreSQL.
	// TODO: Start worker pool consuming from Kafka.

	<-ctx.Done()

	fmt.Printf("\n[%s] shutting down gracefully...\n", serviceName)
	fmt.Printf("[%s] stopped\n", serviceName)
	return nil
}
