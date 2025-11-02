package kafkautil_test

import (
	"testing"

	"github.com/AudreyRodrygo/Sentinel/pkg/kafkautil"
)

func TestNewProducer_RequiresBrokers(t *testing.T) {
	_, err := kafkautil.NewProducer(kafkautil.ProducerConfig{
		Brokers: nil, // No brokers.
		Topic:   "test",
	})

	if err == nil {
		t.Fatal("expected error when no brokers provided")
	}
}

func TestNewConsumer_RequiresBrokers(t *testing.T) {
	_, err := kafkautil.NewConsumer(kafkautil.ConsumerConfig{
		Brokers: nil,
		GroupID: "test",
	})

	if err == nil {
		t.Fatal("expected error when no brokers provided")
	}
}

func TestNewConsumer_RequiresGroupID(t *testing.T) {
	_, err := kafkautil.NewConsumer(kafkautil.ConsumerConfig{
		Brokers: []string{"localhost:9094"},
		GroupID: "", // No group ID.
	})

	if err == nil {
		t.Fatal("expected error when no group ID provided")
	}
}

// Integration tests for actual Kafka produce/consume cycles
// are gated behind the "integration" build tag and require
// a running Kafka instance (via docker-compose).
