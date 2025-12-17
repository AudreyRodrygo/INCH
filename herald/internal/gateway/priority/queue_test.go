package priority_test

import (
	"testing"
	"time"

	"github.com/AudreyRodrygo/Sentinel/herald/internal/gateway/priority"
)

func TestPush_Pop_PriorityOrder(t *testing.T) {
	q := priority.New()

	now := time.Now()

	// Push in wrong order — queue should return by priority.
	q.Push(priority.Item{ID: "low", Priority: priority.Low, CreatedAt: now})
	q.Push(priority.Item{ID: "critical", Priority: priority.Critical, CreatedAt: now})
	q.Push(priority.Item{ID: "normal", Priority: priority.Normal, CreatedAt: now})
	q.Push(priority.Item{ID: "high", Priority: priority.High, CreatedAt: now})

	expected := []string{"critical", "high", "normal", "low"}
	for _, wantID := range expected {
		item, ok := q.TryPop()
		if !ok {
			t.Fatalf("expected item %q, queue is empty", wantID)
		}
		if item.ID != wantID {
			t.Errorf("got %q, want %q", item.ID, wantID)
		}
	}
}

func TestPush_Pop_FIFOWithinPriority(t *testing.T) {
	q := priority.New()

	// Three HIGH items — should come out in insertion order (FIFO).
	q.Push(priority.Item{ID: "h1", Priority: priority.High, CreatedAt: time.Now()})
	q.Push(priority.Item{ID: "h2", Priority: priority.High, CreatedAt: time.Now().Add(time.Millisecond)})
	q.Push(priority.Item{ID: "h3", Priority: priority.High, CreatedAt: time.Now().Add(2 * time.Millisecond)})

	item1, _ := q.TryPop()
	item2, _ := q.TryPop()
	item3, _ := q.TryPop()

	if item1.ID != "h1" || item2.ID != "h2" || item3.ID != "h3" {
		t.Errorf("FIFO order broken: got %s, %s, %s", item1.ID, item2.ID, item3.ID)
	}
}

func TestTryPop_EmptyQueue(t *testing.T) {
	q := priority.New()

	_, ok := q.TryPop()
	if ok {
		t.Error("TryPop on empty queue should return false")
	}
}

func TestLen(t *testing.T) {
	q := priority.New()

	if q.Len() != 0 {
		t.Errorf("empty queue Len() = %d, want 0", q.Len())
	}

	q.Push(priority.Item{ID: "1", Priority: priority.Normal, CreatedAt: time.Now()})
	q.Push(priority.Item{ID: "2", Priority: priority.High, CreatedAt: time.Now()})

	if q.Len() != 2 {
		t.Errorf("Len() = %d, want 2", q.Len())
	}

	q.TryPop()

	if q.Len() != 1 {
		t.Errorf("after pop, Len() = %d, want 1", q.Len())
	}
}

func TestLevel_String(t *testing.T) {
	tests := []struct {
		level priority.Level
		want  string
	}{
		{priority.Critical, "CRITICAL"},
		{priority.High, "HIGH"},
		{priority.Normal, "NORMAL"},
		{priority.Low, "LOW"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("Level(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  priority.Level
	}{
		{"CRITICAL", priority.Critical},
		{"HIGH", priority.High},
		{"NORMAL", priority.Normal},
		{"LOW", priority.Low},
		{"unknown", priority.Low},
		{"PRIORITY_CRITICAL", priority.Critical},
	}

	for _, tt := range tests {
		if got := priority.ParseLevel(tt.input); got != tt.want {
			t.Errorf("ParseLevel(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func BenchmarkPush(b *testing.B) {
	q := priority.New()
	now := time.Now()

	b.ResetTimer()
	for i := range b.N {
		q.Push(priority.Item{
			ID:        "bench",
			Priority:  priority.Level(i % 4),
			CreatedAt: now,
		})
	}
}

func BenchmarkPushPop(b *testing.B) {
	q := priority.New()
	now := time.Now()

	b.ResetTimer()
	for i := range b.N {
		q.Push(priority.Item{
			ID:        "bench",
			Priority:  priority.Level(i % 4),
			CreatedAt: now,
		})
		q.TryPop()
	}
}
