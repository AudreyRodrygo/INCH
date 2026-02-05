// Package enrichment provides event enrichment for the processor pipeline.
//
// Enrichment adds context to raw security events before rule evaluation:
//   - GeoIP: country, city, ASN from IP address (offline MaxMind database)
//   - Threat Intel: known malicious IPs, Tor exit nodes, scanners
//
// The pipeline applies all enrichers to each event sequentially.
// Each enricher writes results into the event's metadata map.
//
// Architecture:
//
//	Raw Event → [GeoIP] → [ThreatIntel] → [Future enrichers...] → Enriched Event
//
// Adding a new enricher:
//  1. Implement the Enricher interface
//  2. Add it to the pipeline in processor config
//
// This follows the Open/Closed Principle: the pipeline is open for extension
// (new enrichers) but closed for modification (existing code doesn't change).
package enrichment

import (
	"context"

	pb "github.com/AudreyRodrygo/Inch/gen/inch/v1"
)

// Enricher adds contextual information to a security event.
//
// Implementations must be safe for concurrent use — multiple worker
// goroutines call Enrich simultaneously on different events.
//
// Convention: enrichers write results into event.Metadata with a
// prefixed key to avoid collisions:
//   - GeoIP: "geo_country", "geo_city", "geo_asn"
//   - ThreatIntel: "threat_score", "threat_tags"
type Enricher interface {
	// Enrich adds contextual data to the event's metadata.
	// Returns an error only for infrastructure failures (DB down, etc.),
	// not for "no data found" (that's normal for many IPs).
	Enrich(ctx context.Context, event *pb.SecurityEvent) error

	// Name returns a human-readable name for logging and metrics.
	Name() string
}

// Pipeline applies a chain of enrichers to each event.
//
// If one enricher fails, the pipeline logs the error and continues
// with the next one — a GeoIP failure shouldn't prevent threat intel lookup.
type Pipeline struct {
	enrichers []Enricher
}

// NewPipeline creates an enrichment pipeline with the given enrichers.
// Enrichers are applied in the order provided.
func NewPipeline(enrichers ...Enricher) *Pipeline {
	return &Pipeline{enrichers: enrichers}
}

// Enrich runs all enrichers on the event.
// Errors from individual enrichers are collected but don't stop the pipeline.
// Returns the first error encountered (for logging), or nil if all succeeded.
func (p *Pipeline) Enrich(ctx context.Context, event *pb.SecurityEvent) error {
	if event.Metadata == nil {
		event.Metadata = make(map[string]string)
	}

	var firstErr error
	for _, e := range p.enrichers {
		if err := e.Enrich(ctx, event); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			// Continue with other enrichers — don't let one failure
			// block the entire pipeline.
		}
	}

	return firstErr
}
