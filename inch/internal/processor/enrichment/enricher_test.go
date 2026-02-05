package enrichment_test

import (
	"context"
	"errors"
	"testing"

	pb "github.com/AudreyRodrygo/Inch/gen/inch/v1"
	"github.com/AudreyRodrygo/Inch/inch/internal/processor/enrichment"
)

func TestGeoIP_PrivateIP(t *testing.T) {
	geo := enrichment.NewGeoIP()
	event := &pb.SecurityEvent{
		SourceIp: "192.168.1.100",
		Metadata: make(map[string]string),
	}

	err := geo.Enrich(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Metadata["geo_is_private"] != "true" {
		t.Errorf("geo_is_private = %q, want %q", event.Metadata["geo_is_private"], "true")
	}
	if event.Metadata["geo_country"] != "PRIVATE" {
		t.Errorf("geo_country = %q, want %q", event.Metadata["geo_country"], "PRIVATE")
	}
}

func TestGeoIP_PublicIP(t *testing.T) {
	geo := enrichment.NewGeoIP()
	event := &pb.SecurityEvent{
		SourceIp: "8.8.8.8",
		Metadata: make(map[string]string),
	}

	err := geo.Enrich(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Metadata["geo_is_private"] != "false" {
		t.Errorf("geo_is_private = %q, want %q", event.Metadata["geo_is_private"], "false")
	}
}

func TestGeoIP_EmptyIP(t *testing.T) {
	geo := enrichment.NewGeoIP()
	event := &pb.SecurityEvent{
		Metadata: make(map[string]string),
	}

	err := geo.Enrich(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := event.Metadata["geo_country"]; ok {
		t.Error("should not set geo_country for empty IP")
	}
}

func TestThreatIntel_KnownThreat(t *testing.T) {
	ti := enrichment.NewThreatIntel()
	event := &pb.SecurityEvent{
		SourceIp: "185.220.101.34", // Known Tor exit node.
		Metadata: make(map[string]string),
	}

	err := ti.Enrich(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Metadata["threat_known"] != "true" {
		t.Errorf("threat_known = %q, want %q", event.Metadata["threat_known"], "true")
	}
	if event.Metadata["threat_category"] != "tor_exit" {
		t.Errorf("threat_category = %q, want %q", event.Metadata["threat_category"], "tor_exit")
	}
}

func TestThreatIntel_UnknownIP(t *testing.T) {
	ti := enrichment.NewThreatIntel()
	event := &pb.SecurityEvent{
		SourceIp: "1.2.3.4",
		Metadata: make(map[string]string),
	}

	err := ti.Enrich(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Metadata["threat_known"] != "false" {
		t.Errorf("threat_known = %q, want %q", event.Metadata["threat_known"], "false")
	}
}

func TestPipeline_AppliesAllEnrichers(t *testing.T) {
	pipeline := enrichment.NewPipeline(
		enrichment.NewGeoIP(),
		enrichment.NewThreatIntel(),
	)

	event := &pb.SecurityEvent{
		SourceIp: "185.220.101.34", // Tor exit + public IP.
	}

	err := pipeline.Enrich(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// GeoIP should have run.
	if event.Metadata["geo_is_private"] != "false" {
		t.Errorf("GeoIP didn't run: geo_is_private = %q", event.Metadata["geo_is_private"])
	}

	// ThreatIntel should have run.
	if event.Metadata["threat_known"] != "true" {
		t.Errorf("ThreatIntel didn't run: threat_known = %q", event.Metadata["threat_known"])
	}
}

func TestPipeline_ContinuesOnError(t *testing.T) {
	// A failing enricher should not prevent others from running.
	failing := &failingEnricher{}
	ti := enrichment.NewThreatIntel()

	pipeline := enrichment.NewPipeline(failing, ti)

	event := &pb.SecurityEvent{
		SourceIp: "185.220.101.34",
	}

	err := pipeline.Enrich(context.Background(), event)
	// Should return the error from the failing enricher...
	if err == nil {
		t.Fatal("expected error from failing enricher")
	}

	// ...but ThreatIntel should still have run.
	if event.Metadata["threat_known"] != "true" {
		t.Error("ThreatIntel should have run despite earlier enricher failure")
	}
}

func TestPipeline_InitializesMetadata(t *testing.T) {
	pipeline := enrichment.NewPipeline(enrichment.NewGeoIP())

	// Event with nil metadata — pipeline should initialize it.
	event := &pb.SecurityEvent{SourceIp: "10.0.0.1"}

	err := pipeline.Enrich(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Metadata == nil {
		t.Fatal("metadata should be initialized by pipeline")
	}
}

// failingEnricher is a test helper that always returns an error.
type failingEnricher struct{}

func (f *failingEnricher) Name() string { return "failing" }

func (f *failingEnricher) Enrich(_ context.Context, _ *pb.SecurityEvent) error {
	return errors.New("enricher failed")
}
