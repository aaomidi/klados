// Package server hosts the split-server transport: a ConnectRPC layer for
// request/response, an event hub streamed over Connect server-streams, and
// WebSocket endpoints for the log/exec byte planes.
package server

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/Vilsol/slox"
)

// Event is one published event, payload pre-marshalled to JSON exactly once
// regardless of subscriber count.
type Event struct {
	Name        string
	PayloadJSON []byte
}

const (
	// flushWindow coalesces bursts (Kubernetes watch storms) into single
	// frames. Small enough to be imperceptible for one-shot events.
	flushWindow = 75 * time.Millisecond
	// maxPending is the per-subscriber backlog cap. A consumer that falls
	// this far behind is cut off and must resubscribe (and re-list), the
	// same recovery contract Kubernetes watches already impose via 410 Gone.
	maxPending = 16384
	// maxBatch bounds a single flush so one frame can't grow unboundedly.
	maxBatch = 2048
)

// subscriber owns a mutex-guarded pending queue drained by Run. Emit never
// blocks on a slow consumer; overflow evicts the subscriber instead.
type subscriber struct {
	hub    *Hub
	topics []string

	mu       sync.Mutex
	pending  []Event
	wake     chan struct{}
	overflow bool
	closed   bool
}

// Hub is the transport-agnostic replacement for the Wails event bus. Domain
// packages keep their injected emitEvent closures; those closures now point
// at (*Hub).Emit.
type Hub struct {
	ctx context.Context

	mu     sync.RWMutex
	subs   map[string]map[*subscriber]struct{} // topic -> subscribers
	locals map[string]map[uint64]func([]byte)  // topic -> in-process handlers (plugin host)
	nextID uint64
}

func NewHub(ctx context.Context) *Hub {
	return &Hub{
		ctx:    ctx,
		subs:   make(map[string]map[*subscriber]struct{}),
		locals: make(map[string]map[uint64]func([]byte)),
	}
}

// Emit publishes an event to every subscriber of the exact topic name.
// Signature matches the emitEvent closures threaded through the codebase.
func (h *Hub) Emit(name string, data any) {
	var payload []byte
	if data != nil {
		var err error
		payload, err = json.Marshal(data)
		if err != nil {
			slox.Warn(h.ctx, "hub: marshal event payload failed", "event", name, "error", err)
			return
		}
	}
	ev := Event{Name: name, PayloadJSON: payload}

	h.mu.RLock()
	subs := h.subs[name]
	for s := range subs {
		s.enqueue(ev)
	}
	locals := h.locals[name]
	handlers := make([]func([]byte), 0, len(locals))
	for _, fn := range locals {
		handlers = append(handlers, fn)
	}
	h.mu.RUnlock()

	for _, fn := range handlers {
		fn(payload)
	}
}

// On registers an in-process handler (used by the plugin host in place of
// application.Get().Event.On). Returns an unsubscribe func.
func (h *Hub) On(name string, fn func(payloadJSON []byte)) func() {
	h.mu.Lock()
	id := h.nextID
	h.nextID++
	if h.locals[name] == nil {
		h.locals[name] = make(map[uint64]func([]byte))
	}
	h.locals[name][id] = fn
	h.mu.Unlock()

	return func() {
		h.mu.Lock()
		if m := h.locals[name]; m != nil {
			delete(m, id)
			if len(m) == 0 {
				delete(h.locals, name)
			}
		}
		h.mu.Unlock()
	}
}

// Subscribe registers a streaming subscriber for the given topics. The
// returned subscriber must have Run called on it; Run returns when ctx ends
// or the subscriber overflows.
func (h *Hub) Subscribe(topics []string) *subscriber {
	s := &subscriber{
		hub:    h,
		topics: topics,
		wake:   make(chan struct{}, 1),
	}
	h.mu.Lock()
	for _, t := range topics {
		if h.subs[t] == nil {
			h.subs[t] = make(map[*subscriber]struct{})
		}
		h.subs[t][s] = struct{}{}
	}
	h.mu.Unlock()
	return s
}

func (h *Hub) remove(s *subscriber) {
	h.mu.Lock()
	for _, t := range s.topics {
		if m := h.subs[t]; m != nil {
			delete(m, s)
			if len(m) == 0 {
				delete(h.subs, t)
			}
		}
	}
	h.mu.Unlock()
}

func (s *subscriber) enqueue(ev Event) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if len(s.pending) >= maxPending {
		s.overflow = true
		s.mu.Unlock()
		select {
		case s.wake <- struct{}{}:
		default:
		}
		return
	}
	s.pending = append(s.pending, ev)
	s.mu.Unlock()

	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Run drains the queue, invoking send with coalesced batches, until ctx is
// done, send fails, or the consumer overflows. It always cleans up the
// subscription.
func (s *subscriber) Run(ctx context.Context, send func(batch []Event) error) error {
	defer func() {
		s.hub.remove(s)
		s.mu.Lock()
		s.closed = true
		s.pending = nil
		s.mu.Unlock()
	}()

	timer := time.NewTimer(flushWindow)
	defer timer.Stop()
	timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.wake:
			// First event of a window: wait out the coalescing window so a
			// burst lands in one frame.
			timer.Reset(flushWindow)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
			}
		}

		for {
			s.mu.Lock()
			overflow := s.overflow
			n := len(s.pending)
			if n > maxBatch {
				n = maxBatch
			}
			batch := make([]Event, n)
			copy(batch, s.pending[:n])
			s.pending = s.pending[n:]
			remaining := len(s.pending)
			s.mu.Unlock()

			if len(batch) > 0 {
				if err := send(batch); err != nil {
					return err
				}
			}
			if overflow {
				return errOverflow
			}
			if remaining == 0 {
				break
			}
		}
	}
}
