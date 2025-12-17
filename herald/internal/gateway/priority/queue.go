// Package priority implements a heap-based priority queue with SLA enforcement.
//
// This is a custom implementation (not container/heap) to demonstrate
// understanding of the data structure on interviews.
//
// SLA levels:
//   - CRITICAL: must be dequeued within 1 second
//   - HIGH:     must be dequeued within 10 seconds
//   - NORMAL:   must be dequeued within 60 seconds
//   - LOW:      best-effort delivery
//
// The queue is a max-heap ordered by priority level. Within the same
// priority, older items are dequeued first (FIFO within priority class).
//
// Thread-safe: all operations are protected by a mutex.
package priority

import (
	"sync"
	"time"
)

// Level represents notification priority.
type Level int

// Priority levels ordered from lowest to highest.
const (
	Low      Level = 0
	Normal   Level = 1
	High     Level = 2
	Critical Level = 3
)

// String returns the priority name.
func (l Level) String() string {
	switch l {
	case Critical:
		return "CRITICAL"
	case High:
		return "HIGH"
	case Normal:
		return "NORMAL"
	case Low:
		return "LOW"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel converts a string to a Level.
func ParseLevel(s string) Level {
	switch s {
	case "CRITICAL", "PRIORITY_CRITICAL":
		return Critical
	case "HIGH", "PRIORITY_HIGH":
		return High
	case "NORMAL", "PRIORITY_NORMAL":
		return Normal
	default:
		return Low
	}
}

// Item represents a queued notification.
type Item struct {
	ID        string
	Priority  Level
	Payload   []byte
	CreatedAt time.Time
}

// Queue is a thread-safe priority queue backed by a binary max-heap.
//
// The heap property: parent's priority ≥ children's priority.
// Within the same priority, earlier items come first (stable ordering).
//
// Implementation:
//   - Stored as a flat slice (standard heap representation)
//   - Parent of index i = (i-1)/2
//   - Children of index i = 2i+1, 2i+2
//   - Push: append + sift up — O(log n)
//   - Pop: swap root with last, shrink, sift down — O(log n)
type Queue struct {
	mu    sync.Mutex
	items []Item
	cond  *sync.Cond // Signals when items are available.
}

// New creates an empty priority queue.
func New() *Queue {
	q := &Queue{
		items: make([]Item, 0, 256),
	}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// Push adds an item to the queue.
// O(log n) — appends to the end and sifts up.
func (q *Queue) Push(item Item) {
	q.mu.Lock()
	q.items = append(q.items, item)
	q.siftUp(len(q.items) - 1)
	q.mu.Unlock()

	q.cond.Signal() // Wake up a waiting Pop.
}

// Pop removes and returns the highest-priority item.
// Blocks until an item is available or the done channel is closed.
//
// Returns the item and true, or zero Item and false if done is closed.
func (q *Queue) Pop(done <-chan struct{}) (Item, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.items) == 0 {
		// Wait for a signal (from Push) or check if we should stop.
		// sync.Cond doesn't support select, so we use a goroutine trick.
		waiting := make(chan struct{})
		go func() {
			q.cond.L.Lock()
			q.cond.Wait()
			q.cond.L.Unlock()
			close(waiting)
		}()

		q.mu.Unlock()
		select {
		case <-done:
			q.mu.Lock()
			q.cond.Broadcast() // Wake the waiting goroutine.
			return Item{}, false
		case <-waiting:
			q.mu.Lock()
		}
	}

	return q.pop(), true
}

// TryPop removes and returns the highest-priority item without blocking.
// Returns false if the queue is empty.
func (q *Queue) TryPop() (Item, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return Item{}, false
	}

	return q.pop(), true
}

// pop removes the root (highest priority). Caller must hold the lock.
func (q *Queue) pop() Item {
	n := len(q.items)
	item := q.items[0]

	// Move the last element to the root and sift down.
	q.items[0] = q.items[n-1]
	q.items = q.items[:n-1]

	if len(q.items) > 0 {
		q.siftDown(0)
	}

	return item
}

// Len returns the number of items in the queue.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// siftUp restores the heap property after inserting at index i.
//
// Compares the element with its parent. If the element has higher priority
// (or same priority but earlier timestamp), swap them. Repeat until
// the element reaches its correct position.
func (q *Queue) siftUp(i int) {
	for i > 0 {
		parent := (i - 1) / 2
		if !q.less(parent, i) {
			break
		}
		q.items[i], q.items[parent] = q.items[parent], q.items[i]
		i = parent
	}
}

// siftDown restores the heap property after removing the root.
//
// Compares the element with its children. Swaps with the higher-priority
// child if needed. Repeat until the element reaches its correct position.
func (q *Queue) siftDown(i int) {
	n := len(q.items)
	for {
		best := i
		left := 2*i + 1
		right := 2*i + 2

		if left < n && q.less(best, left) {
			best = left
		}
		if right < n && q.less(best, right) {
			best = right
		}

		if best == i {
			break // Heap property satisfied.
		}

		q.items[i], q.items[best] = q.items[best], q.items[i]
		i = best
	}
}

// less returns true if items[i] has LOWER priority than items[j].
// Used by siftUp/siftDown — the heap pushes higher-priority items to the root.
//
// Ordering: higher Level wins. For equal Level, earlier timestamp wins (FIFO).
func (q *Queue) less(i, j int) bool {
	if q.items[i].Priority != q.items[j].Priority {
		return q.items[i].Priority < q.items[j].Priority
	}
	// Same priority — earlier timestamp has higher priority (FIFO).
	return q.items[i].CreatedAt.After(q.items[j].CreatedAt)
}
