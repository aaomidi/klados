package server

import (
	"context"
	"fmt"

	"github.com/Vilsol/klados/internal/config"
	"github.com/Vilsol/klados/internal/services"
	"github.com/Vilsol/klados/internal/session"
)

// Bootstrap wires the full service graph against a Hub, mirroring the
// construction order the Wails registration previously guaranteed, and runs
// every Startup hook. It is shared by `klados serve` and the desktop shell.
func Bootstrap(ctx context.Context, hub *Hub, cfg *config.Config, sess *session.Session) (Deps, error) {
	appSvc := services.NewAppService(cfg, sess, ctx, hub.Emit, hub.On)
	clusterSvc := services.NewClusterService(appSvc, sess)
	configSvc := services.NewConfigService(ctx, cfg)
	configSvc.SetAppService(appSvc)
	drainSvc := services.NewDrainService(appSvc)
	resourceSvc := services.NewResourceService(appSvc, drainSvc)
	schemaSvc := services.NewSchemaService(appSvc)
	logSvc := services.NewLogService(appSvc)
	execSvc := services.NewExecService(appSvc)
	portForwardSvc := services.NewPortForwardService(appSvc)
	volumeBrowserSvc := services.NewVolumeBrowserService(appSvc)
	clusterSvc.SetVolumeBrowserService(volumeBrowserSvc)
	appSvc.SetVolumeBrowserService(volumeBrowserSvc)
	metricsSvc := services.NewMetricsService(appSvc)
	pluginSvc := services.NewPluginService(appSvc, resourceSvc)
	helmSvc := services.NewHelmService(appSvc, resourceSvc)
	clusterSvc.SetHelmService(helmSvc)
	appSvc.SetPluginService(pluginSvc)
	metricsSvc.SetPluginService(pluginSvc)

	deps := Deps{
		App:           appSvc,
		Cluster:       clusterSvc,
		Config:        configSvc,
		Resource:      resourceSvc,
		Schema:        schemaSvc,
		Log:           logSvc,
		Exec:          execSvc,
		PortForward:   portForwardSvc,
		VolumeBrowser: volumeBrowserSvc,
		Drain:         drainSvc,
		Metrics:       metricsSvc,
		Plugin:        pluginSvc,
		Helm:          helmSvc,
	}

	// Startup order matches the old Wails service registration order.
	type startable struct {
		name string
		fn   func(context.Context) error
	}
	for _, s := range []startable{
		{"app", appSvc.Startup},
		{"cluster", clusterSvc.Startup},
		{"config", configSvc.Startup},
		{"resource", resourceSvc.Startup},
		{"schema", schemaSvc.Startup},
		{"log", logSvc.Startup},
		{"exec", execSvc.Startup},
		{"portforward", portForwardSvc.Startup},
		{"volumebrowser", volumeBrowserSvc.Startup},
		{"drain", drainSvc.Startup},
		{"metrics", metricsSvc.Startup},
		{"plugin", pluginSvc.Startup},
		{"helm", helmSvc.Startup},
	} {
		if err := s.fn(ctx); err != nil {
			return Deps{}, fmt.Errorf("starting %s service: %w", s.name, err)
		}
	}

	return deps, nil
}

// Shutdown tears services down in reverse startup order.
func (d Deps) Shutdown() {
	for _, fn := range []func() error{
		d.Helm.Shutdown,
		d.Plugin.Shutdown,
		d.Metrics.Shutdown,
		d.Drain.Shutdown,
		d.VolumeBrowser.Shutdown,
		d.PortForward.Shutdown,
		d.Exec.Shutdown,
		d.Log.Shutdown,
		d.Schema.Shutdown,
		d.Resource.Shutdown,
		d.Config.Shutdown,
		d.Cluster.Shutdown,
		d.App.Shutdown,
	} {
		_ = fn()
	}
}
