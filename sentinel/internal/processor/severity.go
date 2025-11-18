package processor

import (
	pb "github.com/AudreyRodrygo/Sentinel/gen/sentinel/v1"
)

// ClassifySeverity assigns a severity level to an event based on its type
// and enrichment data.
//
// This is a baseline classifier — the Rule Engine (Phase 3) provides
// more sophisticated severity assignment based on temporal correlation
// and behavioral analysis.
//
// Classification logic:
//   - CRITICAL: privilege escalation from threat-flagged IP
//   - HIGH: privilege escalation, or auth failure from known threat
//   - MEDIUM: auth failures, file modifications
//   - LOW: auth success, process start/stop, network connections
func ClassifySeverity(event *pb.SecurityEvent) pb.Severity {
	isThreat := event.Metadata["threat_known"] == "true"

	switch event.Type {
	case pb.EventType_EVENT_TYPE_PRIVILEGE_ESCALATION:
		if isThreat {
			return pb.Severity_SEVERITY_CRITICAL
		}
		return pb.Severity_SEVERITY_HIGH

	case pb.EventType_EVENT_TYPE_AUTH_FAILURE:
		if isThreat {
			return pb.Severity_SEVERITY_HIGH
		}
		return pb.Severity_SEVERITY_MEDIUM

	case pb.EventType_EVENT_TYPE_FILE_MODIFY:
		return pb.Severity_SEVERITY_MEDIUM

	case pb.EventType_EVENT_TYPE_AUTH_SUCCESS,
		pb.EventType_EVENT_TYPE_PROCESS_START,
		pb.EventType_EVENT_TYPE_PROCESS_STOP,
		pb.EventType_EVENT_TYPE_NETWORK_CONNECT,
		pb.EventType_EVENT_TYPE_NETWORK_LISTEN,
		pb.EventType_EVENT_TYPE_FILE_ACCESS,
		pb.EventType_EVENT_TYPE_SYSTEM_CALL:
		return pb.Severity_SEVERITY_LOW

	default:
		return pb.Severity_SEVERITY_LOW
	}
}
