package rules_test

import (
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	pb "github.com/AudreyRodrygo/Inch/gen/inch/v1"
	"github.com/AudreyRodrygo/Inch/inch/internal/processor/rules"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func testLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

// --- Parser Tests ---

func TestParse_SingleRule(t *testing.T) {
	yaml := `
id: test-single
name: Test Single Rule
type: single
conditions:
  - field: type
    op: eq
    value: EVENT_TYPE_AUTH_FAILURE
severity: HIGH
`
	rule, err := rules.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if rule.ID != "test-single" {
		t.Errorf("ID = %q, want %q", rule.ID, "test-single")
	}
	if rule.Type != rules.RuleTypeSingle {
		t.Errorf("Type = %q, want %q", rule.Type, rules.RuleTypeSingle)
	}
}

func TestParse_ThresholdRule(t *testing.T) {
	yaml := `
id: test-threshold
name: Test Threshold
type: threshold
conditions:
  - field: type
    op: eq
    value: EVENT_TYPE_AUTH_FAILURE
group_by: [source_ip]
threshold: 5
window: 60s
severity: HIGH
`
	rule, err := rules.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if rule.Threshold != 5 {
		t.Errorf("Threshold = %d, want 5", rule.Threshold)
	}
	if rule.Window != 60*time.Second {
		t.Errorf("Window = %v, want 60s", rule.Window)
	}
}

func TestParse_ValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{"missing id", `name: test
type: single
conditions:
  - field: type
    op: eq
    value: x`},
		{"threshold without window", `id: t
name: test
type: threshold
conditions:
  - field: type
    op: eq
    value: x
group_by: [source_ip]
threshold: 5`},
		{"sequence with one step", `id: t
name: test
type: sequence
group_by: [source_ip]
window: 60s
steps:
  - conditions:
      - field: type
        op: eq
        value: x`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := rules.Parse([]byte(tt.yaml))
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

// --- Matcher Tests ---

func TestMatchesConditions_Eq(t *testing.T) {
	event := &pb.SecurityEvent{
		Type:    pb.EventType_EVENT_TYPE_AUTH_FAILURE,
		Service: "sshd",
	}

	conditions := []rules.Condition{
		{Field: "type", Op: "eq", Value: "EVENT_TYPE_AUTH_FAILURE"},
		{Field: "service", Op: "eq", Value: "sshd"},
	}

	if !rules.MatchesConditions(event, conditions) {
		t.Error("expected match")
	}
}

func TestMatchesConditions_Contains(t *testing.T) {
	event := &pb.SecurityEvent{
		RawMessage: "Failed password for root from 1.2.3.4",
	}

	conditions := []rules.Condition{
		{Field: "raw_message", Op: "contains", Value: "Failed password"},
	}

	if !rules.MatchesConditions(event, conditions) {
		t.Error("expected contains match")
	}
}

func TestMatchesConditions_Metadata(t *testing.T) {
	event := &pb.SecurityEvent{
		Metadata: map[string]string{
			"threat_category": "tor_exit",
		},
	}

	conditions := []rules.Condition{
		{Field: "threat_category", Op: "eq", Value: "tor_exit"},
	}

	if !rules.MatchesConditions(event, conditions) {
		t.Error("expected metadata match")
	}
}

func TestMatchesConditions_NoMatch(t *testing.T) {
	event := &pb.SecurityEvent{
		Type: pb.EventType_EVENT_TYPE_AUTH_SUCCESS,
	}

	conditions := []rules.Condition{
		{Field: "type", Op: "eq", Value: "EVENT_TYPE_AUTH_FAILURE"},
	}

	if rules.MatchesConditions(event, conditions) {
		t.Error("should not match")
	}
}

func TestGetGroupKey(t *testing.T) {
	event := &pb.SecurityEvent{
		SourceIp: "1.2.3.4",
		Service:  "sshd",
	}

	key := rules.GetGroupKey(event, []string{"source_ip", "service"})
	if key != "1.2.3.4|sshd" {
		t.Errorf("group key = %q, want %q", key, "1.2.3.4|sshd")
	}
}

// --- Engine Tests ---

func TestEngine_SingleRule(t *testing.T) {
	rule := &rules.Rule{
		ID:   "test",
		Name: "test",
		Type: rules.RuleTypeSingle,
		Conditions: []rules.Condition{
			{Field: "type", Op: "eq", Value: "EVENT_TYPE_AUTH_FAILURE"},
		},
		Severity: "HIGH",
	}

	engine := rules.New([]*rules.Rule{rule}, testLogger())

	event := &pb.SecurityEvent{
		EventId:   "e1",
		Type:      pb.EventType_EVENT_TYPE_AUTH_FAILURE,
		Timestamp: timestamppb.Now(),
	}

	results := engine.Evaluate(event)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Rule.ID != "test" {
		t.Errorf("result rule ID = %q, want %q", results[0].Rule.ID, "test")
	}
}

func TestEngine_ThresholdRule(t *testing.T) {
	rule := &rules.Rule{
		ID:   "brute",
		Name: "brute force",
		Type: rules.RuleTypeThreshold,
		Conditions: []rules.Condition{
			{Field: "type", Op: "eq", Value: "EVENT_TYPE_AUTH_FAILURE"},
		},
		GroupBy:   []string{"source_ip"},
		Threshold: 3,
		Window:    60 * time.Second,
		Severity:  "HIGH",
	}

	engine := rules.New([]*rules.Rule{rule}, testLogger())
	now := time.Now()

	// Send 2 events — should not fire.
	for i := range 2 {
		event := &pb.SecurityEvent{
			EventId:   fmt.Sprintf("e%d", i),
			Type:      pb.EventType_EVENT_TYPE_AUTH_FAILURE,
			SourceIp:  "1.2.3.4",
			Timestamp: timestamppb.New(now.Add(time.Duration(i) * time.Second)),
		}
		results := engine.Evaluate(event)
		if len(results) != 0 {
			t.Fatalf("event %d: expected 0 results, got %d", i, len(results))
		}
	}

	// 3rd event — should fire!
	event3 := &pb.SecurityEvent{
		EventId:   "e3",
		Type:      pb.EventType_EVENT_TYPE_AUTH_FAILURE,
		SourceIp:  "1.2.3.4",
		Timestamp: timestamppb.New(now.Add(2 * time.Second)),
	}

	results := engine.Evaluate(event3)
	if len(results) != 1 {
		t.Fatalf("expected threshold to fire, got %d results", len(results))
	}
}

func TestEngine_ThresholdRule_DifferentGroups(t *testing.T) {
	rule := &rules.Rule{
		ID:   "brute",
		Name: "brute force",
		Type: rules.RuleTypeThreshold,
		Conditions: []rules.Condition{
			{Field: "type", Op: "eq", Value: "EVENT_TYPE_AUTH_FAILURE"},
		},
		GroupBy:   []string{"source_ip"},
		Threshold: 3,
		Window:    60 * time.Second,
	}

	engine := rules.New([]*rules.Rule{rule}, testLogger())
	now := time.Now()

	// 2 events from IP A, 2 from IP B — neither should fire.
	ips := []string{"1.1.1.1", "1.1.1.1", "2.2.2.2", "2.2.2.2"}
	for i, ip := range ips {
		event := &pb.SecurityEvent{
			EventId:   fmt.Sprintf("e%d", i),
			Type:      pb.EventType_EVENT_TYPE_AUTH_FAILURE,
			SourceIp:  ip,
			Timestamp: timestamppb.New(now.Add(time.Duration(i) * time.Second)),
		}
		results := engine.Evaluate(event)
		if len(results) != 0 {
			t.Fatalf("event %d: unexpected fire for IP %s", i, ip)
		}
	}
}

func TestEngine_SequenceRule(t *testing.T) {
	rule := &rules.Rule{
		ID:      "kill-chain",
		Name:    "kill chain",
		Type:    rules.RuleTypeSequence,
		GroupBy: []string{"source_ip"},
		Window:  10 * time.Minute,
		Steps: []rules.Step{
			{
				Conditions: []rules.Condition{
					{Field: "type", Op: "eq", Value: "EVENT_TYPE_AUTH_FAILURE"},
				},
				Count: 2,
			},
			{
				Conditions: []rules.Condition{
					{Field: "type", Op: "eq", Value: "EVENT_TYPE_AUTH_SUCCESS"},
				},
				Count: 1,
			},
		},
		Severity: "CRITICAL",
	}

	engine := rules.New([]*rules.Rule{rule}, testLogger())

	// Step 0: 2 failures needed.
	for i := range 2 {
		results := engine.Evaluate(&pb.SecurityEvent{
			EventId:   fmt.Sprintf("f%d", i),
			Type:      pb.EventType_EVENT_TYPE_AUTH_FAILURE,
			SourceIp:  "attacker",
			Timestamp: timestamppb.Now(),
		})
		if len(results) != 0 {
			t.Fatalf("step 0, event %d: should not fire yet", i)
		}
	}

	// Step 1: 1 success — should complete the sequence.
	results := engine.Evaluate(&pb.SecurityEvent{
		EventId:   "s1",
		Type:      pb.EventType_EVENT_TYPE_AUTH_SUCCESS,
		SourceIp:  "attacker",
		Timestamp: timestamppb.Now(),
	})

	if len(results) != 1 {
		t.Fatalf("expected sequence to fire, got %d results", len(results))
	}
	if results[0].Rule.ID != "kill-chain" {
		t.Errorf("wrong rule fired: %s", results[0].Rule.ID)
	}
}

func TestEngine_DisabledRule(t *testing.T) {
	disabled := false
	rule := &rules.Rule{
		ID:      "disabled",
		Name:    "disabled rule",
		Type:    rules.RuleTypeSingle,
		Enabled: &disabled,
		Conditions: []rules.Condition{
			{Field: "type", Op: "eq", Value: "EVENT_TYPE_AUTH_FAILURE"},
		},
	}

	engine := rules.New([]*rules.Rule{rule}, testLogger())
	results := engine.Evaluate(&pb.SecurityEvent{
		Type:      pb.EventType_EVENT_TYPE_AUTH_FAILURE,
		Timestamp: timestamppb.Now(),
	})

	if len(results) != 0 {
		t.Error("disabled rule should not fire")
	}
}

func TestEngine_Reload(t *testing.T) {
	engine := rules.New(nil, testLogger())

	if len(engine.Rules()) != 0 {
		t.Fatal("expected 0 rules initially")
	}

	newRules := []*rules.Rule{
		{
			ID: "r1", Name: "r1", Type: rules.RuleTypeSingle,
			Conditions: []rules.Condition{{Field: "type", Op: "eq", Value: "x"}},
		},
	}
	engine.Reload(newRules)

	if len(engine.Rules()) != 1 {
		t.Errorf("expected 1 rule after reload, got %d", len(engine.Rules()))
	}
}

func TestLoadFromDir(t *testing.T) {
	loaded, err := rules.LoadFromDir("../../../rules")
	if err != nil {
		t.Fatalf("LoadFromDir error: %v", err)
	}

	if len(loaded) < 3 {
		t.Errorf("expected at least 3 rules, got %d", len(loaded))
	}

	// Verify the brute force rule loaded correctly.
	var found bool
	for _, r := range loaded {
		if r.ID == "brute-force-ssh" {
			found = true
			if r.Threshold != 5 {
				t.Errorf("brute force threshold = %d, want 5", r.Threshold)
			}
		}
	}
	if !found {
		t.Error("brute-force-ssh rule not found")
	}
}

// --- Benchmark ---

func BenchmarkEngine_Evaluate_SingleRule(b *testing.B) {
	rule := &rules.Rule{
		ID:   "bench",
		Name: "bench",
		Type: rules.RuleTypeSingle,
		Conditions: []rules.Condition{
			{Field: "type", Op: "eq", Value: "EVENT_TYPE_AUTH_FAILURE"},
			{Field: "service", Op: "eq", Value: "sshd"},
		},
	}

	engine := rules.New([]*rules.Rule{rule}, zap.NewNop())
	event := &pb.SecurityEvent{
		EventId:   "bench-event",
		Type:      pb.EventType_EVENT_TYPE_AUTH_FAILURE,
		Service:   "sshd",
		Timestamp: timestamppb.Now(),
	}

	b.ResetTimer()
	for range b.N {
		engine.Evaluate(event)
	}
}

func BenchmarkEngine_Evaluate_ThresholdRule(b *testing.B) {
	rule := &rules.Rule{
		ID:   "bench-thresh",
		Name: "bench",
		Type: rules.RuleTypeThreshold,
		Conditions: []rules.Condition{
			{Field: "type", Op: "eq", Value: "EVENT_TYPE_AUTH_FAILURE"},
		},
		GroupBy:   []string{"source_ip"},
		Threshold: 1000000, // High threshold so it never fires during bench.
		Window:    time.Hour,
	}

	engine := rules.New([]*rules.Rule{rule}, zap.NewNop())

	b.ResetTimer()
	for i := range b.N {
		event := &pb.SecurityEvent{
			EventId:   "e",
			Type:      pb.EventType_EVENT_TYPE_AUTH_FAILURE,
			SourceIp:  fmt.Sprintf("10.0.0.%d", i%256),
			Timestamp: timestamppb.Now(),
		}
		engine.Evaluate(event)
	}
}
