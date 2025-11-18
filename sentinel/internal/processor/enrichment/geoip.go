package enrichment

import (
	"context"
	"net"

	pb "github.com/AudreyRodrygo/Sentinel/gen/sentinel/v1"
)

// GeoIP enriches events with geographic information based on IP addresses.
//
// In production, this would use MaxMind GeoLite2 database (oschwald/geoip2-golang).
// For now, we use a simple lookup table to demonstrate the pattern without
// requiring a 60MB database file in the repo.
//
// The enricher adds these metadata keys:
//   - geo_country: two-letter country code (e.g., "US", "DE", "RU")
//   - geo_is_private: "true" if the IP is in a private range (RFC 1918)
//
// Integration with MaxMind (future):
//
//	db, _ := geoip2.Open("GeoLite2-City.mmdb")
//	record, _ := db.City(net.ParseIP(event.SourceIp))
//	event.Metadata["geo_country"] = record.Country.IsoCode
type GeoIP struct{}

// NewGeoIP creates a GeoIP enricher.
func NewGeoIP() *GeoIP {
	return &GeoIP{}
}

// Name returns the enricher identifier for logging and metrics.
func (g *GeoIP) Name() string { return "geoip" }

// Enrich adds geographic data to the event based on source IP.
func (g *GeoIP) Enrich(_ context.Context, event *pb.SecurityEvent) error {
	if event.SourceIp == "" {
		return nil
	}

	ip := net.ParseIP(event.SourceIp)
	if ip == nil {
		return nil // Invalid IP — skip silently.
	}

	// Check if IP is private (RFC 1918).
	// Private IPs are internal — no GeoIP data, but useful to tag.
	if ip.IsPrivate() || ip.IsLoopback() {
		event.Metadata["geo_is_private"] = "true"
		event.Metadata["geo_country"] = "PRIVATE"
		return nil
	}

	// Placeholder: in production, query MaxMind GeoLite2 database here.
	// For now, mark as "unknown" to show the enrichment was attempted.
	event.Metadata["geo_is_private"] = "false"
	event.Metadata["geo_country"] = "UNKNOWN"

	return nil
}
