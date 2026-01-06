package processor_test

import (
	"testing"

	pb "github.com/AudreyRodrygo/Inch/gen/sentinel/v1"
	"github.com/AudreyRodrygo/Inch/inch/internal/processor"
)

func TestClassifySeverity(t *testing.T) {
	tests := []struct {
		name     string
		event    *pb.SecurityEvent
		expected pb.Severity
	}{
		{
			name: "privilege escalation from threat = CRITICAL",
			event: &pb.SecurityEvent{
				Type:     pb.EventType_EVENT_TYPE_PRIVILEGE_ESCALATION,
				Metadata: map[string]string{"threat_known": "true"},
			},
			expected: pb.Severity_SEVERITY_CRITICAL,
		},
		{
			name: "privilege escalation normal = HIGH",
			event: &pb.SecurityEvent{
				Type:     pb.EventType_EVENT_TYPE_PRIVILEGE_ESCALATION,
				Metadata: map[string]string{},
			},
			expected: pb.Severity_SEVERITY_HIGH,
		},
		{
			name: "auth failure from threat = HIGH",
			event: &pb.SecurityEvent{
				Type:     pb.EventType_EVENT_TYPE_AUTH_FAILURE,
				Metadata: map[string]string{"threat_known": "true"},
			},
			expected: pb.Severity_SEVERITY_HIGH,
		},
		{
			name: "auth failure normal = MEDIUM",
			event: &pb.SecurityEvent{
				Type:     pb.EventType_EVENT_TYPE_AUTH_FAILURE,
				Metadata: map[string]string{},
			},
			expected: pb.Severity_SEVERITY_MEDIUM,
		},
		{
			name: "file modify = MEDIUM",
			event: &pb.SecurityEvent{
				Type:     pb.EventType_EVENT_TYPE_FILE_MODIFY,
				Metadata: map[string]string{},
			},
			expected: pb.Severity_SEVERITY_MEDIUM,
		},
		{
			name: "auth success = LOW",
			event: &pb.SecurityEvent{
				Type:     pb.EventType_EVENT_TYPE_AUTH_SUCCESS,
				Metadata: map[string]string{},
			},
			expected: pb.Severity_SEVERITY_LOW,
		},
		{
			name: "nil metadata handled",
			event: &pb.SecurityEvent{
				Type: pb.EventType_EVENT_TYPE_PROCESS_START,
			},
			expected: pb.Severity_SEVERITY_LOW,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.event.Metadata == nil {
				tt.event.Metadata = make(map[string]string)
			}

			got := processor.ClassifySeverity(tt.event)
			if got != tt.expected {
				t.Errorf("ClassifySeverity() = %v, want %v", got, tt.expected)
			}
		})
	}
}
