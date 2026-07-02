package resource

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MarvinJWendt/testza"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/Vilsol/klados/internal/cluster"
)

type hangingProvider struct {
	conn *cluster.Connection
}

func (p *hangingProvider) GetConnection(_ string) (*cluster.Connection, error) {
	return p.conn, nil
}

func TestResourceEngine_List_TimesOutOnHangingServer(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // hang until test cleanup
	}))
	defer srv.Close()
	// Registered after srv.Close() so it runs first on unwind (LIFO), releasing
	// the blocked handler before Close() waits on the active connection.
	defer close(release)

	dyn, err := dynamic.NewForConfig(&rest.Config{Host: srv.URL})
	testza.AssertNoError(t, err)
	conn := cluster.NewTestConnection(dyn, nil)

	engine := NewResourceEngine(&hangingProvider{conn: conn}, NewEnricherRegistry())

	orig := requestTimeout
	requestTimeout = 100 * time.Millisecond
	defer func() { requestTimeout = orig }()

	done := make(chan error, 1)
	go func() {
		_, _, err := engine.List(context.Background(), "test", "core.v1.pods", "")
		done <- err
	}()

	select {
	case err := <-done:
		testza.AssertNotNil(t, err) // deadline exceeded, not a hang
	case <-time.After(2 * time.Second):
		t.Fatal("List hung on unresponsive server — no timeout applied")
	}
}
