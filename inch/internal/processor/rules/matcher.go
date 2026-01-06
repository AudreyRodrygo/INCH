package rules

import (
	"fmt"
	"regexp"
	"strings"

	pb "github.com/AudreyRodrygo/Inch/gen/sentinel/v1"
)

// MatchesConditions checks if an event satisfies ALL conditions of a rule.
//
// All conditions must match (AND logic). For OR logic, create separate rules.
// This is intentional — OR rules are harder to reason about in security contexts
// and can lead to alert fatigue.
func MatchesConditions(event *pb.SecurityEvent, conditions []Condition) bool {
	for _, cond := range conditions {
		if !matchCondition(event, cond) {
			return false // Short-circuit on first failure.
		}
	}
	return len(conditions) > 0
}

// matchCondition evaluates a single condition against an event.
func matchCondition(event *pb.SecurityEvent, cond Condition) bool {
	fieldValue := getFieldValue(event, cond.Field)

	switch cond.Op {
	case "eq":
		return fieldValue == cond.Value
	case "neq":
		return fieldValue != cond.Value
	case "contains":
		return strings.Contains(fieldValue, cond.Value)
	case "regex":
		matched, err := regexp.MatchString(cond.Value, fieldValue)
		return err == nil && matched
	case "exists":
		return fieldValue != ""
	default:
		return false // Unknown operator — don't match.
	}
}

// getFieldValue extracts a field value from a SecurityEvent by name.
//
// Supports both direct proto fields and metadata keys:
//   - "type" → event.Type.String()
//   - "source_ip" → event.SourceIp
//   - "severity" → event.Severity.String()
//   - "meta.threat_known" → event.Metadata["threat_known"]
//   - "threat_known" → event.Metadata["threat_known"] (shorthand)
//
// This mapping allows YAML rules to reference any event field
// without knowing Protobuf internals.
func getFieldValue(event *pb.SecurityEvent, field string) string { //nolint:cyclop // flat switch, not complex
	// Direct proto fields.
	switch field {
	case "type", "event_type":
		return event.Type.String()
	case "hostname":
		return event.Hostname
	case "source_ip":
		return event.SourceIp
	case "destination_ip":
		return event.DestinationIp
	case "source_port":
		if event.SourcePort > 0 {
			return fmt.Sprintf("%d", event.SourcePort)
		}
		return ""
	case "user":
		return event.User
	case "process":
		return event.Process
	case "service":
		return event.Service
	case "severity":
		return event.Severity.String()
	case "agent_id":
		return event.AgentId
	case "raw_message":
		return event.RawMessage
	}

	// Metadata fields: "meta.key" or just "key".
	if event.Metadata != nil {
		metaKey := field
		if strings.HasPrefix(field, "meta.") {
			metaKey = strings.TrimPrefix(field, "meta.")
		}
		if val, ok := event.Metadata[metaKey]; ok {
			return val
		}
	}

	return ""
}

// GetGroupKey builds a composite key from the event's group_by fields.
//
// For group_by: [source_ip], an event with source_ip=1.2.3.4 produces "1.2.3.4".
// For group_by: [source_ip, service], it produces "1.2.3.4|sshd".
//
// This key is used to track per-group state in threshold and sequence rules.
// Events with the same group key are correlated together.
func GetGroupKey(event *pb.SecurityEvent, groupBy []string) string {
	if len(groupBy) == 0 {
		return ""
	}

	parts := make([]string, len(groupBy))
	for i, field := range groupBy {
		parts[i] = getFieldValue(event, field)
	}
	return strings.Join(parts, "|")
}
