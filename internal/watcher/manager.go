package watcher

import (
	"context"
	"fmt"
	"github.com/sasha-s/go-deadlock"
	"time"

	"github.com/Vilsol/slox"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"

	"github.com/Vilsol/klados/internal/cluster"
	"github.com/Vilsol/klados/internal/resource"
)

// gracePeriod is the delay between a StopWatch call and the actual teardown.
// It's a var (not a const) so tests can shorten it via GracePeriodForTest /
// SetGracePeriodForTest.
var gracePeriod = 30 * time.Second

// GracePeriodForTest returns the current grace period. Test-only helper.
func GracePeriodForTest() time.Duration { return gracePeriod }

// SetGracePeriodForTest overrides the grace period. Test-only helper.
func SetGracePeriodForTest(d time.Duration) { gracePeriod = d }

// WatchCountForTest returns the number of tracked watches. Test-only helper.
func (m *WatchManager) WatchCountForTest() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.watches)
}

// Synthetic event types used by virtual watch sources to bracket a full
// snapshot replacement (e.g. after a transport reconnect). The frontend
// ResourceStore consumes these to replace items[] atomically.
const (
	EventSyncStart = "SYNC_START"
	EventSyncEnd   = "SYNC_END"
)

// WatchOptions are optional knobs accepted by StartWatch. Zero-value means
// "behave exactly as before".
type WatchOptions struct {
	// FieldSelector is forwarded into metav1.ListOptions.FieldSelector when
	// constructing the dynamic watch. Ignored for virtual sources.
	FieldSelector string
}

// VirtualWatchSource produces watch events for a synthetic GVR. The source
// owns its own goroutines and emits via the WatchManager's emitter; it
// returns a stop function the manager will invoke during teardown.
type VirtualWatchSource interface {
	Watch(ctx context.Context, contextName, namespace, resourceVersion string, emit func(string, any)) (stop func(), err error)
}

type watchKey struct {
	contextName string
	gvr         string
	namespace   string
}

type watchState struct {
	cancel     context.CancelFunc
	graceTimer *time.Timer
	// stopVirtual is set when a virtual source is providing this watch. The
	// manager invokes it on teardown to release the source's resources.
	stopVirtual func()
}

type WatchEvent struct {
	Type   string         `json:"type"`
	Object map[string]any `json:"object"`
}

type ConnectionProvider interface {
	GetConnection(contextName string) (*cluster.Connection, error)
}

type WatchManager struct {
	mu          deadlock.Mutex
	clusterMgr  ConnectionProvider
	enricherReg *resource.EnricherRegistry
	emitEvent   func(string, any)
	ctx         context.Context
	watches     map[watchKey]*watchState
	virtuals    map[string]VirtualWatchSource
}

func NewWatchManager(mgr ConnectionProvider, enricherReg *resource.EnricherRegistry, emit func(string, any), ctx context.Context) *WatchManager {
	return &WatchManager{
		clusterMgr:  mgr,
		enricherReg: enricherReg,
		emitEvent:   emit,
		ctx:         ctx,
		watches:     make(map[watchKey]*watchState),
		virtuals:    make(map[string]VirtualWatchSource),
	}
}

// RegisterVirtual associates a VirtualWatchSource with a GVR. StartWatch will
// delegate to the source instead of the dynamic Kubernetes client when the
// GVR matches. Must be called before StartWatch.
func (m *WatchManager) RegisterVirtual(gvr string, src VirtualWatchSource) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.virtuals[gvr] = src
}

func (m *WatchManager) StartWatch(contextName, gvr, namespace, resourceVersion string, opts ...WatchOptions) error {
	var opt WatchOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	key := watchKey{contextName, gvr, namespace}

	m.mu.Lock()
	defer m.mu.Unlock()

	if state, ok := m.watches[key]; ok {
		if state.graceTimer != nil {
			state.graceTimer.Stop()
			state.graceTimer = nil
		}
		return nil
	}

	slox.Debug(m.ctx, "watch started", "context", contextName, "gvr", gvr, "namespace", namespace, "rv", resourceVersion, "fieldSelector", opt.FieldSelector)

	// Virtual dispatch — bypasses the dynamic client entirely.
	if src, ok := m.virtuals[gvr]; ok {
		ctx, cancel := context.WithCancel(context.Background())
		eventName := fmt.Sprintf("watch:%s:%s:%s", contextName, gvr, namespace)
		stop, err := src.Watch(ctx, contextName, namespace, resourceVersion, func(name string, payload any) {
			if name == "" {
				name = eventName
			}
			m.emitEvent(name, payload)
		})
		if err != nil {
			cancel()
			return err
		}
		m.watches[key] = &watchState{cancel: cancel, stopVirtual: stop}
		return nil
	}

	conn, err := m.clusterMgr.GetConnection(contextName)
	if err != nil {
		return err
	}

	gvrParsed, err := resource.ParseGVR(gvr)
	if err != nil {
		return err
	}

	var ri dynamic.ResourceInterface
	dr := conn.Dynamic.Resource(gvrParsed)
	if namespace != "" {
		ri = dr.Namespace(namespace)
	} else {
		ri = dr
	}

	enrichers := m.enricherReg.GetAll(gvr)
	eventName := fmt.Sprintf("watch:%s:%s:%s", contextName, gvr, namespace)
	resyncName := eventName + ":resync"

	ctx, cancel := context.WithCancel(conn.Context())
	state := &watchState{cancel: cancel}
	m.watches[key] = state

	go m.runWatch(ctx, state, ri, enrichers, eventName, resyncName, key, contextName, resourceVersion, opt.FieldSelector)
	return nil
}

// removeSelf deletes the map entry for key iff it still belongs to state.
// Idempotent; safe to call from both the Gone path and the deferred exit path.
func (m *WatchManager) removeSelf(key watchKey, state *watchState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.watches[key]; ok && s == state {
		if s.graceTimer != nil {
			s.graceTimer.Stop()
		}
		s.cancel()
		delete(m.watches, key)
	}
}

func (m *WatchManager) runWatch(
	ctx context.Context,
	state *watchState,
	ri dynamic.ResourceInterface,
	enrichers []resource.Enricher,
	eventName string,
	resyncName string,
	key watchKey,
	contextName string,
	initialRV string,
	fieldSelector string,
) {
	defer m.removeSelf(key, state)

	currentRV := initialRV
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		wi, err := ri.Watch(ctx, metav1.ListOptions{
			ResourceVersion:     currentRV,
			AllowWatchBookmarks: true,
			FieldSelector:       fieldSelector,
		})
		if err != nil {
			if k8serrors.IsGone(err) || k8serrors.IsResourceExpired(err) {
				slox.Warn(m.ctx, "watch RV too old, requesting resync", "event", eventName, "rv", currentRV)
				m.removeSelf(key, state)
				m.emitEvent(resyncName, nil)
				return
			}
			slox.Warn(m.ctx, "watch failed, retrying", "event", eventName, "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		nextRV, gone := m.processEvents(ctx, wi, enrichers, eventName, contextName, currentRV)
		if gone {
			slox.Warn(m.ctx, "watch stream returned Gone, requesting resync", "event", eventName, "rv", currentRV)
			m.removeSelf(key, state)
			m.emitEvent(resyncName, nil)
			return
		}
		currentRV = nextRV
	}
}

func (m *WatchManager) processEvents(
	ctx context.Context,
	wi watch.Interface,
	enrichers []resource.Enricher,
	eventName string,
	contextName string,
	currentRV string,
) (string, bool) {
	defer wi.Stop()

	for {
		select {
		case <-ctx.Done():
			return currentRV, false
		case event, ok := <-wi.ResultChan():
			if !ok {
				return currentRV, false
			}

			if event.Type == watch.Error {
				if status, ok := event.Object.(*metav1.Status); ok {
					if status.Reason == metav1.StatusReasonGone || status.Code == 410 {
						return currentRV, true
					}
					slox.Warn(m.ctx, "watch error event", "event", eventName, "reason", status.Reason, "message", status.Message)
				}
				return currentRV, false
			}

			obj, ok := event.Object.(*unstructured.Unstructured)
			if !ok {
				continue
			}

			if rv := obj.GetResourceVersion(); rv != "" {
				currentRV = rv
			}

			if event.Type == watch.Bookmark {
				continue
			}

			for _, enricher := range enrichers {
				if err := enricher.Enrich(contextName, obj); err != nil {
					slox.Debug(m.ctx, "enricher error on watch event", "error", err)
				}
			}

			m.emitEvent(eventName, WatchEvent{
				Type:   string(event.Type),
				Object: obj.Object,
			})
		}
	}
}

func (m *WatchManager) StopWatch(contextName, gvr, namespace string) {
	key := watchKey{contextName, gvr, namespace}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.watches[key]
	if !ok {
		return
	}

	if state.graceTimer != nil {
		return
	}

	state.graceTimer = time.AfterFunc(gracePeriod, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if s, ok := m.watches[key]; ok && s.graceTimer != nil {
			slox.Debug(m.ctx, "watch stopped", "context", contextName, "gvr", gvr)
			if s.stopVirtual != nil {
				s.stopVirtual()
			}
			s.cancel()
			delete(m.watches, key)
		}
	})
}

// StopContext synchronously tears down every watch for the given context.
// Returns the keys it stopped so callers can emit follow-up events.
func (m *WatchManager) StopContext(contextName string) []watchKey {
	m.mu.Lock()
	defer m.mu.Unlock()
	var stopped []watchKey
	for key, state := range m.watches {
		if key.contextName != contextName {
			continue
		}
		if state.graceTimer != nil {
			state.graceTimer.Stop()
		}
		if state.stopVirtual != nil {
			state.stopVirtual()
		}
		state.cancel()
		delete(m.watches, key)
		stopped = append(stopped, key)
	}
	return stopped
}

// ResyncContext tears down every watch for the context and asks the frontend
// to re-list + re-watch via the :resync protocol. Used after a connection
// recovers: any watch may be wedged on a dead socket or hold a stale client.
func (m *WatchManager) ResyncContext(contextName string) {
	for _, key := range m.StopContext(contextName) {
		m.emitEvent(fmt.Sprintf("watch:%s:%s:%s:resync", key.contextName, key.gvr, key.namespace), nil)
	}
}

func (m *WatchManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, state := range m.watches {
		if state.graceTimer != nil {
			state.graceTimer.Stop()
		}
		if state.stopVirtual != nil {
			state.stopVirtual()
		}
		state.cancel()
		delete(m.watches, key)
	}
}
