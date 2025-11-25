// Package rules implements the Sentinel Correlation Rule Engine.
//
// The rule engine is the intellectual core of the platform. It evaluates
// each incoming security event against a set of configurable rules
// and generates alerts when conditions are met.
//
// Rule types:
//   - Single: matches individual events (field conditions)
//   - Threshold: counts events in a time window (e.g., "5 failures in 60s")
//   - Sequence: detects ordered event chains (kill chain detection)
//
// Rules are defined in YAML and loaded at startup. Hot reload allows
// adding/modifying rules without restarting the service.
//
// Architecture:
//
//	Event ──▶ [Rule Engine] ──▶ Match? ──▶ Alert
//	              │
//	              ├── Single rules (O(N) per event, N = rules)
//	              ├── Threshold rules (sliding window per group_by)
//	              └── Sequence rules (state machine per group_by)
package rules

import (
	"time"

	pb "github.com/AudreyRodrygo/Sentinel/gen/sentinel/v1"
)

// RuleType identifies the kind of correlation a rule performs.
type RuleType string

const (
	// RuleTypeSingle matches individual events based on field conditions.
	// Example: "alert on any privilege escalation from a Tor exit node"
	RuleTypeSingle RuleType = "single"

	// RuleTypeThreshold counts events in a sliding time window.
	// Example: "alert when 5+ failed logins from same IP in 60 seconds"
	RuleTypeThreshold RuleType = "threshold"

	// RuleTypeSequence detects ordered event chains (kill chain).
	// Example: "5 failed logins → success → privilege escalation in 10 minutes"
	RuleTypeSequence RuleType = "sequence"
)

// Rule is a correlation rule loaded from YAML.
//
// Example YAML:
//
//	id: brute-force-ssh
//	name: SSH Brute Force Detection
//	type: threshold
//	conditions:
//	  - field: type
//	    op: eq
//	    value: EVENT_TYPE_AUTH_FAILURE
//	  - field: service
//	    op: eq
//	    value: sshd
//	group_by: [source_ip]
//	threshold: 5
//	window: 60s
//	severity: HIGH
//	actions: [alert]
type Rule struct {
	ID          string        `yaml:"id"`
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Type        RuleType      `yaml:"type"`
	Enabled     *bool         `yaml:"enabled"` // Pointer to distinguish unset from false.
	Conditions  []Condition   `yaml:"conditions"`
	GroupBy     []string      `yaml:"group_by"`
	Threshold   int           `yaml:"threshold"`
	Window      time.Duration `yaml:"window"`
	Severity    string        `yaml:"severity"`
	Actions     []string      `yaml:"actions"`
	Tags        []string      `yaml:"tags"`

	// Sequence-specific fields.
	Steps []Step `yaml:"steps"`
}

// IsEnabled returns whether the rule is active.
// Rules are enabled by default if the field is not set.
func (r *Rule) IsEnabled() bool {
	if r.Enabled == nil {
		return true
	}
	return *r.Enabled
}

// SeverityProto converts the string severity to a Protobuf enum.
func (r *Rule) SeverityProto() pb.Severity {
	switch r.Severity {
	case "CRITICAL":
		return pb.Severity_SEVERITY_CRITICAL
	case "HIGH":
		return pb.Severity_SEVERITY_HIGH
	case "MEDIUM":
		return pb.Severity_SEVERITY_MEDIUM
	case "LOW":
		return pb.Severity_SEVERITY_LOW
	default:
		return pb.Severity_SEVERITY_MEDIUM
	}
}

// Condition defines a field-level check on an event.
//
// Supported operators:
//   - eq: exact string match
//   - neq: not equal
//   - contains: substring match
//   - regex: regular expression match
//   - exists: field is non-empty
type Condition struct {
	Field string `yaml:"field"`
	Op    string `yaml:"op"`
	Value string `yaml:"value"`
}

// Step defines one stage in a sequence rule.
// Each step has its own conditions and a minimum count.
type Step struct {
	Conditions []Condition `yaml:"conditions"`
	Count      int         `yaml:"count"` // Minimum events matching this step (default: 1).
}
