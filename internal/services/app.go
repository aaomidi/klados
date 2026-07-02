package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Vilsol/slox"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/Vilsol/klados/internal/cluster"
	"github.com/Vilsol/klados/internal/config"
	"github.com/Vilsol/klados/internal/exec"
	"github.com/Vilsol/klados/internal/logs"
	"github.com/Vilsol/klados/internal/portforward"
	"github.com/Vilsol/klados/internal/session"
	"github.com/Vilsol/klados/internal/streaming"
	"github.com/Vilsol/klados/internal/volumebrowser"

	"github.com/google/uuid"
)

type AppService struct {
	clusterMgr         *cluster.Manager
	streamingSrv       *streaming.Server
	logStreamer         *logs.Streamer
	execManager        *exec.Manager
	portForwardManager   *portforward.Manager
	volumeBrowserManager *volumebrowser.Manager
	session              *session.Session
	config             *config.Config
	pluginSvc          *PluginService
	volumeBrowserSvc   *VolumeBrowserService
	ctx                context.Context
	app                *application.App
}

func NewAppService(cfg *config.Config, sess *session.Session, ctx context.Context) *AppService {
	return &AppService{
		config:  cfg,
		session: sess,
		ctx:     ctx,
	}
}

func (a *AppService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	a.app = application.Get()
	a.ctx = slox.Into(ctx, slog.Default())

	emitEvent := func(name string, data any) {
		if a.app != nil {
			a.app.Event.Emit(name, data)
		}
	}

	a.clusterMgr = cluster.NewManager(emitEvent, a.config, a.ctx)
	a.logStreamer = logs.NewStreamer(a.clusterMgr, a.ctx)
	a.execManager = exec.NewManager(a.clusterMgr, a.ctx)
	a.portForwardManager = portforward.NewManager(a.clusterMgr, a.config, emitEvent, a.ctx)
	a.volumeBrowserManager = volumebrowser.NewManager(a.ctx, a.clusterMgr, uuid.NewString())
	a.streamingSrv = streaming.NewServer(emitEvent, a.ctx)
	a.streamingSrv.RegisterHandlers(a.logStreamer, a.execManager)

	if err := a.streamingSrv.Start(a.ctx); err != nil {
		return err
	}

	if err := a.clusterMgr.LoadKubeconfigs(a.config.KubeconfigPaths); err != nil {
		slox.Warn(a.ctx, "failed to load kubeconfigs", "error", err)
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

func (a *AppService) ServiceShutdown() error {
	if a.session != nil {
		_ = a.session.Save()
	}

	if a.clusterMgr != nil {
		if err := a.clusterMgr.DisconnectAll(); err != nil {
			slox.Error(a.ctx, "error disconnecting clusters", "error", err)
		}
	}

	if a.streamingSrv != nil {
		if err := a.streamingSrv.Stop(); err != nil {
			slox.Error(a.ctx, "error stopping streaming server", "error", err)
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

func (a *AppService) BrowseKubeconfigFile() (string, error) {
	return a.app.Dialog.OpenFile().
		AddFilter("Kubeconfig files", "*.yaml").
		PromptForSingleSelection()
}

func (a *AppService) BrowsePluginFile() (string, error) {
	return a.app.Dialog.OpenFile().
		AddFilter("Klados plugin archives", "*.oci.tar.gz").
		PromptForSingleSelection()
}

func (a *AppService) BrowseManifestFile() (string, error) {
	path, err := a.app.Dialog.OpenFile().
		AddFilter("YAML files", "*.yaml;*.yml").
		PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a *AppService) GetStreamingConfig() streaming.StreamingConfig {
	return streaming.StreamingConfig{
		Port:  a.streamingSrv.Port(),
		Token: a.streamingSrv.Token(),
	}
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

func (a *AppService) RegisterPluginsDir(dir string) {
	if a.streamingSrv != nil {
		a.streamingSrv.SetPluginsDir(dir)
	}
}

func (a *AppService) Ctx() context.Context {
	return a.ctx
}

//wails:ignore
func (a *AppService) SetPluginService(svc *PluginService) {
	a.pluginSvc = svc
}

//wails:ignore
func (a *AppService) SetVolumeBrowserService(svc *VolumeBrowserService) {
	a.volumeBrowserSvc = svc
}

//wails:ignore
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
