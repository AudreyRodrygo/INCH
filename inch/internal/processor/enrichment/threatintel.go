package enrichment

import (
	"context"

	pb "github.com/AudreyRodrygo/Inch/gen/sentinel/v1"
)

// ThreatIntel enriches events with threat intelligence data.
//
// Checks the source IP against known threat indicators:
//   - Known Tor exit nodes
//   - Known scanners (Shodan, Censys, etc.)
//   - Botnet C2 servers
//   - Previously flagged IPs
//
// In production, this would query:
//   - A local Redis set of known bad IPs (updated hourly)
//   - External APIs (AbuseIPDB, VirusTotal, AlienVault OTX)
//
// For now, we use a hardcoded set to demonstrate the pattern.
//
// Metadata keys added:
//   - threat_known: "true" if the IP is in the threat database
//   - threat_category: type of threat (tor, scanner, botnet, etc.)
type ThreatIntel struct {
	// knownThreats is a lookup table of known malicious IPs.
	// In production: Redis SET with periodic refresh from threat feeds.
	knownThreats map[string]string
}

// NewThreatIntel creates a threat intelligence enricher.
//
// In production, pass a Redis client for real-time threat lookups.
// Here we seed with some well-known Tor exit nodes for demo purposes.
func NewThreatIntel() *ThreatIntel {
	return &ThreatIntel{
		knownThreats: map[string]string{
			// Sample Tor exit nodes (public, well-known).
			"185.220.101.34": "tor_exit",
			"185.220.101.35": "tor_exit",
			"104.244.76.13":  "tor_exit",
			// Sample scanners.
			"71.6.135.131":   "scanner",    // Censys
			"162.142.125.10": "scanner",    // Censys
			"198.235.24.0":   "researcher", // Shadowserver
		},
	}
}

// Name returns the enricher identifier for logging and metrics.
func (t *ThreatIntel) Name() string { return "threatintel" }

// Enrich checks the source IP against the threat intelligence database.
func (t *ThreatIntel) Enrich(_ context.Context, event *pb.SecurityEvent) error {
	if event.SourceIp == "" {
		return nil
	}

	category, found := t.knownThreats[event.SourceIp]
	if found {
		event.Metadata["threat_known"] = "true"
		event.Metadata["threat_category"] = category
	} else {
		event.Metadata["threat_known"] = "false"
	}

	return nil
}
