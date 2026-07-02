package cluster

import (
	"context"
	"time"

	"github.com/Vilsol/slox"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
)

// crdRediscoverDebounce coalesces bursts of CRD changes (e.g. installing a
// helm chart with 20 CRDs) into one discovery pass. Var for tests.
var crdRediscoverDebounce = 2 * time.Second

// watchCRDs keeps resource discovery current while a connection stays healthy:
// without it, CRDs installed at runtime only appear after a reconnect.
func (m *Manager) watchCRDs(ctx context.Context, contextName string, conn *Connection) {
	crdGVR := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}
	var debounce *time.Timer
	defer func() {
		if debounce != nil {
			debounce.Stop()
		}
	}()

	for {
		list, err := conn.Dynamic.Resource(crdGVR).List(ctx, metav1.ListOptions{})
		if err != nil {
			if k8serrors.IsForbidden(err) {
				slox.Info(m.ctx, "no permission to watch CRDs, runtime CRD changes won't auto-refresh", "context", contextName)
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Minute):
			}
			continue
		}

		wi, err := conn.Dynamic.Resource(crdGVR).Watch(ctx, metav1.ListOptions{
			ResourceVersion:     list.GetResourceVersion(),
			AllowWatchBookmarks: true,
		})
		if err != nil {
			if k8serrors.IsForbidden(err) {
				slox.Info(m.ctx, "no permission to watch CRDs, runtime CRD changes won't auto-refresh", "context", contextName)
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		for event := range wi.ResultChan() {
			if event.Type == watch.Bookmark || event.Type == watch.Error {
				continue
			}
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(crdRediscoverDebounce, func() {
				m.emitDiscovery(ctx, contextName)
			})
		}
		wi.Stop()

		select {
		case <-ctx.Done():
			return
		default: // channel closed by server timeout — re-list and re-watch
		}
	}
}
