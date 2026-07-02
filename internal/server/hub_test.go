package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/MarvinJWendt/testza"

	kladosv1 "github.com/Vilsol/klados/gen/klados/v1"
	"github.com/Vilsol/klados/gen/klados/v1/kladosv1connect"
)

func TestHubCoalescesBurstIntoOneBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(ctx)
	sub := hub.Subscribe([]string{"watch:test"})

	var mu sync.Mutex
	var batches [][]Event
	runCtx, stopRun := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = sub.Run(runCtx, func(batch []Event) error {
			mu.Lock()
			batches = append(batches, batch)
			mu.Unlock()
			return nil
		})
	}()

	for i := 0; i < 50; i++ {
		hub.Emit("watch:test", map[string]int{"i": i})
	}

	testza.AssertNoError(t, waitFor(2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		total := 0
		for _, b := range batches {
			total += len(b)
		}
		return total == 50
	}))

	mu.Lock()
	// The whole burst should land in far fewer frames than events — the
	// coalescing window batches them (typically 1 frame).
	testza.AssertTrue(t, len(batches) <= 3, "expected coalesced batches, got %d frames", len(batches))
	mu.Unlock()

	stopRun()
	<-done
}

func TestHubLocalOnSubscription(t *testing.T) {
	ctx := context.Background()
	hub := NewHub(ctx)

	got := make(chan []byte, 1)
	unsub := hub.On("plugin:event", func(payload []byte) {
		got <- payload
	})

	hub.Emit("plugin:event", map[string]string{"a": "b"})
	select {
	case payload := <-got:
		var decoded map[string]string
		testza.AssertNoError(t, json.Unmarshal(payload, &decoded))
		testza.AssertEqual(t, "b", decoded["a"])
	case <-time.After(time.Second):
		t.Fatal("local handler not invoked")
	}

	unsub()
	hub.Emit("plugin:event", map[string]string{"a": "c"})
	select {
	case <-got:
		t.Fatal("handler invoked after unsubscribe")
	case <-time.After(150 * time.Millisecond):
	}
}

func TestEventServiceSubscribeOverConnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(ctx)
	mux := http.NewServeMux()
	mux.Handle(kladosv1connect.NewEventServiceHandler(NewEventHandler(hub)))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := kladosv1connect.NewEventServiceClient(http.DefaultClient, srv.URL)

	streamCtx, stopStream := context.WithCancel(ctx)
	defer stopStream()
	stream, err := client.Subscribe(streamCtx, connect.NewRequest(&kladosv1.SubscribeRequest{
		Topics: []string{"status:ctx:connection"},
	}))
	testza.AssertNoError(t, err)

	// Give the server a moment to register the subscription before emitting.
	testza.AssertNoError(t, waitFor(2*time.Second, func() bool {
		hub.mu.RLock()
		defer hub.mu.RUnlock()
		return len(hub.subs["status:ctx:connection"]) == 1
	}))

	hub.Emit("status:ctx:connection", "connected")

	// Skip the initial empty ack batch, then expect the emitted event.
	batch := receiveNonEmpty(t, stream)
	testza.AssertEqual(t, 1, len(batch.Events))
	testza.AssertEqual(t, "status:ctx:connection", batch.Events[0].Name)
	testza.AssertEqual(t, `"connected"`, string(batch.Events[0].PayloadJson))

	// Publish injects into the hub (used for cross-window panel events).
	_, err = client.Publish(ctx, connect.NewRequest(&kladosv1.PublishRequest{
		Event: &kladosv1.Event{Name: "status:ctx:connection", PayloadJson: []byte(`"disconnected"`)},
	}))
	testza.AssertNoError(t, err)
	published := receiveNonEmpty(t, stream)
	testza.AssertEqual(t, `"disconnected"`, string(published.Events[0].PayloadJson))
}

func receiveNonEmpty(t *testing.T, stream *connect.ServerStreamForClient[kladosv1.EventBatch]) *kladosv1.EventBatch {
	t.Helper()
	for stream.Receive() {
		if len(stream.Msg().Events) > 0 {
			return stream.Msg()
		}
	}
	t.Fatalf("stream ended before a non-empty batch: %v", stream.Err())
	return nil
}

func waitFor(timeout time.Duration, cond func() bool) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return context.DeadlineExceeded
}
