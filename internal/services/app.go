package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/Vilsol/slox"
	"github.com/adrg/xdg"

	"github.com/Vilsol/klados/internal/cluster"
	"github.com/Vilsol/klados/internal/config"
	"github.com/Vilsol/klados/internal/exec"
	"github.com/Vilsol/klados/internal/logs"
	"github.com/Vilsol/klados/internal/portforward"
	"github.com/Vilsol/klados/internal/session"
	"github.com/Vilsol/klados/internal/volumebrowser"

	"github.com/google/uuid"
)

type AppService struct {
	clusterMgr           *cluster.Manager
	logStreamer          *logs.Streamer
	execManager          *exec.Manager
	portForwardManager   *portforward.Manager
	volumeBrowserManager *volumebrowser.Manager
	session              *session.Session
	config               *config.Config
	pluginSvc            *PluginService
	volumeBrowserSvc     *VolumeBrowserService
	pluginsDir           string
	ctx                  context.Context
	emit                 func(string, any)
	on                   func(string, func([]byte)) func()
}

func NewAppService(cfg *config.Config, sess *session.Session, ctx context.Context, emit func(string, any), on func(string, func([]byte)) func()) *AppService {
	return &AppService{
		config:  cfg,
		session: sess,
		ctx:     ctx,
		emit:    emit,
		on:      on,
	}
}

// Emit publishes an event on the hub this service graph is wired to.
func (a *AppService) Emit(name string, data any) {
	if a.emit != nil {
		a.emit(name, data)
	}
}

// On registers an in-process event handler; the callback receives the
// JSON-encoded payload. Returns an unsubscribe func.
func (a *AppService) On(name string, cb func(payloadJSON []byte)) func() {
	if a.on == nil {
		return func() {}
	}
	return a.on(name, cb)
}

func (a *AppService) Startup(ctx context.Context) error {
	a.ctx = slox.Into(ctx, slog.Default())

	a.clusterMgr = cluster.NewManager(a.Emit, a.config, a.ctx)
	a.logStreamer = logs.NewStreamer(a.clusterMgr, a.ctx)
	a.execManager = exec.NewManager(a.clusterMgr, a.ctx)
	a.portForwardManager = portforward.NewManager(a.clusterMgr, a.config, a.Emit, a.ctx)
	a.volumeBrowserManager = volumebrowser.NewManager(a.ctx, a.clusterMgr, uuid.NewString())

	if err := a.clusterMgr.LoadKubeconfigs(a.config.KubeconfigPaths); err != nil {
		slox.Warn(a.ctx, "failed to load kubeconfigs", "error", err)
	}

	importsDir := filepath.Join(xdg.ConfigHome, "klados", "kubeconfigs")
	extraPaths := func() []string {
		var p []string
		a.config.Read(func(c *config.Config) { p = append(p, c.KubeconfigPaths...) })
		return p
	}
	if err := a.clusterMgr.WatchKubeconfigs(a.ctx, importsDir, extraPaths, a.reconnectChangedContexts); err != nil {
		slox.Warn(a.ctx, "kubeconfig file watching unavailable", "error", err)
	}

	if last := a.session.LastActiveContext; last != "" {
		known := false
		for _, name := range a.session.ConnectedClusters {
			if name == last {
				known = true
				break
			}
		}
		if known {
			go func(name string) {
				backoff := 2 * time.Second
				for attempt := 1; ; attempt++ {
					err := a.clusterMgr.Connect(a.ctx, name)
					if err == nil {
						break
					}
					slox.Warn(a.ctx, "startup reconnect failed", "context", name, "attempt", attempt, "error", err)
					if attempt >= 5 {
						return
					}
					select {
					case <-a.ctx.Done():
						return
					case <-time.After(backoff):
					}
					backoff *= 2
				}
				if err := a.clusterMgr.Activate(a.ctx, name); err != nil {
					slox.Warn(a.ctx, "failed to activate cluster on startup", "context", name, "error", err)
				}
				a.portForwardManager.ReconnectSaved(name)
				if a.volumeBrowserSvc != nil {
					a.volumeBrowserSvc.OnClusterConnected(name)
				}
			}(last)
		}
	}

	return nil
}

func (a *AppService) Shutdown() error {
	if a.session != nil {
		_ = a.session.Save()
	}

	if a.clusterMgr != nil {
		if err := a.clusterMgr.DisconnectAll(); err != nil {
			slox.Error(a.ctx, "error disconnecting clusters", "error", err)
		}
	}

	return nil
}

func (a *AppService) ClusterManager() *cluster.Manager {
	return a.clusterMgr
}

func (a *AppService) Config() *config.Config {
	return a.config
}

func (a *AppService) LogStreamer() *logs.Streamer {
	return a.logStreamer
}

func (a *AppService) ExecManager() *exec.Manager {
	return a.execManager
}

func (a *AppService) PortForwardManager() *portforward.Manager {
	return a.portForwardManager
}

func (a *AppService) VolumeBrowserManager() *volumebrowser.Manager {
	return a.volumeBrowserManager
}

// RegisterPluginsDir records where plugin JS bundles live; the HTTP server
// serves them from /plugins/.
func (a *AppService) RegisterPluginsDir(dir string) {
	a.pluginsDir = dir
}

func (a *AppService) PluginsDir() string {
	return a.pluginsDir
}

func (a *AppService) Ctx() context.Context {
	return a.ctx
}

func (a *AppService) SetPluginService(svc *PluginService) {
	a.pluginSvc = svc
}

func (a *AppService) SetVolumeBrowserService(svc *VolumeBrowserService) {
	a.volumeBrowserSvc = svc
}

func (a *AppService) PluginService() *PluginService {
	return a.pluginSvc
}

func (a *AppService) GetSession() *session.Session {
	return a.session
}

func (a *AppService) SaveUIState(openTabs []session.TabState, activeTab int, sidebarCollapsed bool, terminalFontSize int, sidebarWidth int) {
	a.session.OpenTabs = openTabs
	a.session.ActiveTab = activeTab
	a.session.SidebarCollapsed = sidebarCollapsed
	a.session.TerminalFontSize = terminalFontSize
	a.session.SidebarWidth = sidebarWidth
	a.session.SaveDebounced()
}

func (a *AppService) LogFrontend(level, message, attrsJSON string) {
	args := []any{"source", "frontend"}
	if attrsJSON != "" {
		var attrs map[string]any
		if err := json.Unmarshal([]byte(attrsJSON), &attrs); err == nil {
			for k, v := range attrs {
				args = append(args, k, v)
			}
		}
	}
	switch level {
	case "debug":
		slox.Debug(a.ctx, message, args...)
	case "warn":
		slox.Warn(a.ctx, message, args...)
	case "error":
		slox.Error(a.ctx, message, args...)
	default:
		slox.Info(a.ctx, message, args...)
	}
}

func (a *AppService) SetReadOnly(ctx context.Context, enabled bool) error {
	return a.config.Update(func(c *config.Config) {
		c.ReadOnly = enabled
	})
}

func (a *AppService) SetLastActiveContext(name string) {
	if a.session == nil {
		return
	}
	a.session.LastActiveContext = name
	a.session.SaveDebounced()
}

func (a *AppService) GetClusterHealth(ctx context.Context, connCtx string) (cluster.ClusterHealth, error) {
	conn, err := a.clusterMgr.GetConnection(connCtx)
	if err != nil {
		return cluster.ClusterHealth{}, fmt.Errorf("not connected to %q", connCtx)
	}
	return cluster.CheckHealth(ctx, conn), nil
}

// reconnectChangedContexts refreshes live connections whose kubeconfig entry
// changed on disk (cert rotation, server change). Disconnect fires the watch
// teardown hooks; the frontend re-lists on the status events.
func (a *AppService) reconnectChangedContexts(names []string) {
	for _, name := range names {
		if _, err := a.clusterMgr.GetConnection(name); err != nil {
			continue // not connected — nothing to refresh
		}
		activated := a.clusterMgr.IsActivated(name)
		slox.Info(a.ctx, "kubeconfig changed on disk, reconnecting", "context", name)
		if err := a.clusterMgr.Disconnect(name); err != nil {
			slox.Warn(a.ctx, "reconnect: disconnect failed", "context", name, "error", err)
			continue
		}
		if err := a.clusterMgr.Connect(a.ctx, name); err != nil {
			slox.Warn(a.ctx, "reconnect: connect failed", "context", name, "error", err)
			continue
		}
		if activated {
			if err := a.clusterMgr.Activate(a.ctx, name); err != nil {
				slox.Warn(a.ctx, "reconnect: activate failed", "context", name, "error", err)
			}
		}
	}
}
