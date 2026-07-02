package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Vilsol/slox"
	"github.com/fsnotify/fsnotify"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// kubeconfigDebounce coalesces the write bursts most tools produce when
// rewriting kubeconfigs (truncate+write, atomic rename). Var for tests.
var kubeconfigDebounce = 500 * time.Millisecond

// contextCredHash hashes everything that affects how we connect to the named
// context: the context entry, its cluster (server, CA), and its user (certs,
// tokens, exec config). Changes here mean live connections hold stale creds.
func contextCredHash(cfg *clientcmdapi.Config, name string) string {
	kctx, ok := cfg.Contexts[name]
	if !ok {
		return ""
	}
	h := sha256.New()
	enc := json.NewEncoder(h)
	_ = enc.Encode(kctx)
	if c, ok := cfg.Clusters[kctx.Cluster]; ok {
		_ = enc.Encode(c)
	}
	if u, ok := cfg.AuthInfos[kctx.AuthInfo]; ok {
		_ = enc.Encode(u)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// WatchKubeconfigs watches every kubeconfig source (default precedence paths,
// current extra paths, and importsDir for klados-managed imports) and reloads
// contexts when any of them change on disk — e.g. a cloud CLI rotating certs.
// onChanged receives the names of contexts whose connection-relevant config
// changed, so the caller can reconnect them.
func (m *Manager) WatchKubeconfigs(ctx context.Context, importsDir string, extraPaths func() []string, onChanged func(changed []string)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	dirs := map[string]struct{}{importsDir: {}}
	for _, p := range append(clientcmd.NewDefaultClientConfigLoadingRules().Precedence, extraPaths()...) {
		dirs[filepath.Dir(p)] = struct{}{}
	}
	for d := range dirs {
		if err := w.Add(d); err != nil {
			slox.Debug(m.ctx, "kubeconfig watch: cannot watch dir", "dir", d, "error", err)
		}
	}

	go func() {
		defer w.Close()
		var debounce *time.Timer
		defer func() {
			if debounce != nil {
				debounce.Stop()
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if !isKubeconfigPath(ev.Name, importsDir, extraPaths()) {
					continue
				}
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(kubeconfigDebounce, func() {
					m.reloadAfterFileChange(extraPaths(), onChanged)
				})
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	return nil
}

func isKubeconfigPath(path, importsDir string, extras []string) bool {
	if filepath.Dir(path) == importsDir {
		return true
	}
	for _, p := range append(clientcmd.NewDefaultClientConfigLoadingRules().Precedence, extras...) {
		if p == path {
			return true
		}
	}
	return false
}

func (m *Manager) reloadAfterFileChange(extras []string, onChanged func([]string)) {
	// Serialize reloads: Stop() on an already-fired debounce timer is a no-op,
	// so a write burst can otherwise start a second reload while the first is
	// still inside a slow reconnect callback, racing Connect's check-then-write.
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()

	m.mu.RLock()
	old := m.rawConfig
	m.mu.RUnlock()
	oldHashes := map[string]string{}
	if old != nil {
		for name := range old.Contexts {
			oldHashes[name] = contextCredHash(old, name)
		}
	}

	if err := m.LoadKubeconfigs(extras); err != nil {
		slox.Warn(m.ctx, "kubeconfig reload after file change failed", "error", err)
		return
	}

	m.mu.RLock()
	cur := m.rawConfig
	m.mu.RUnlock()
	var changed []string
	for name := range cur.Contexts {
		if contextCredHash(cur, name) != oldHashes[name] {
			changed = append(changed, name)
		}
	}
	for name := range oldHashes {
		if _, ok := cur.Contexts[name]; !ok {
			changed = append(changed, name)
		}
	}

	m.emitEvent("kubeconfigs:updated", m.ListContexts())
	if len(changed) > 0 {
		slox.Info(m.ctx, "kubeconfig files changed on disk", "changedContexts", changed)
		if onChanged != nil {
			onChanged(changed)
		}
	}
}
