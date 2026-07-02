package cluster

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MarvinJWendt/testza"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestProbeHealthz_TimesOutOnHangingServer(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // hang until test cleanup
	}))
	defer srv.Close()
	// Registered after srv.Close() so it runs first on unwind (LIFO), releasing
	// the blocked handler before Close() waits on the active connection.
	defer close(release)

	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	testza.AssertNoError(t, err)
	conn := &Connection{Clientset: cs}

	orig := healthProbeTimeout
	healthProbeTimeout = 100 * time.Millisecond
	defer func() { healthProbeTimeout = orig }()

	done := make(chan error, 1)
	go func() { _, err := probeHealthz(context.Background(), conn); done <- err }()

	select {
	case err := <-done:
		testza.AssertNotNil(t, err) // deadline exceeded, not a hang
	case <-time.After(2 * time.Second):
		t.Fatal("health probe hung on unresponsive server — no timeout applied")
	}
}
