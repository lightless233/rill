// Package eventhub provides a small process-local notification fan-out for
// server-sent events. It deliberately has no persistence or replay; clients
// refetch authoritative state after connecting or receiving a signal.
package eventhub

import "sync"

// Hub broadcasts coalesced notifications to current subscribers. A subscriber
// channel has capacity one, so a slow consumer observes that a change happened
// without accumulating an unbounded queue.
type Hub struct {
	mu          sync.Mutex
	subscribers map[chan struct{}]struct{}
	closed      bool
}

func New() *Hub {
	return &Hub{subscribers: make(map[chan struct{}]struct{})}
}

// Subscribe returns a notification channel and an idempotent unsubscribe
// function. If the hub is already closed, the channel is closed immediately.
func (h *Hub) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)

	h.mu.Lock()
	if h.closed {
		close(ch)
		h.mu.Unlock()
		return ch, func() {}
	}
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			if _, ok := h.subscribers[ch]; ok {
				delete(h.subscribers, ch)
				close(ch)
			}
			h.mu.Unlock()
		})
	}

	return ch, unsubscribe
}

// Notify wakes every current subscriber. It never blocks: if a subscriber
// already has a pending notification, that notification is reused.
func (h *Hub) Notify() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return
	}
	for ch := range h.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Close closes every subscriber channel and rejects future subscriptions.
// It is safe to call more than once.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return
	}
	h.closed = true
	for ch := range h.subscribers {
		close(ch)
		delete(h.subscribers, ch)
	}
}
