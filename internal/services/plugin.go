package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Vilsol/slox"
	"github.com/adrg/xdg"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/Vilsol/klados/internal/cluster"
	"github.com/Vilsol/klados/internal/config"
	"github.com/Vilsol/klados/internal/plugin"
	"github.com/Vilsol/klados/internal/resource"
)

type PluginService struct {
	appService  *AppService
	resourceSvc *ResourceService
	loader      *plugin.Loader
	registry    *plugin.Registry
	runtimes    map[string]*plugin.WasmRuntime
	storages    map[string]*plugin.PluginStorage
	pluginPerms map[string]plugin.PermissionSet
	pluginDirs  map[string]string // pluginName → pluginDir
	watcher     *plugin.PluginWatcher
	pluginsDir  string
	ctx         context.Context
	pullFn      func(ref, destDir string, opts plugin.RemoteOpts) error
}

func NewPluginService(appSvc *AppService, resourceSvc *ResourceService) *PluginService {
	return &PluginService{appService: appSvc, resourceSvc: resourceSvc}
}

func (s *PluginService) Startup(ctx context.Context) error {
	s.ctx = ctx
	s.runtimes = make(map[string]*plugin.WasmRuntime)
	s.storages = make(map[string]*plugin.PluginStorage)
	s.pluginPerms = make(map[string]plugin.PermissionSet)
	s.pluginDirs = make(map[string]string)

	pluginsDir := filepath.Join(xdg.DataHome, "klados", "plugins")
	s.pluginsDir = pluginsDir

	loader, err := plugin.NewLoader(pluginsDir)
	if err != nil {
		slox.Warn(ctx, "plugin loader init failed", "error", err)
		s.loader = nil
		s.registry = plugin.NewRegistry(ctx)
		return nil
	}
	s.loader = loader

	loaded, errs := loader.Load()
	for _, e := range errs {
		slox.Warn(ctx, "plugin load error", "error", e)
	}

	reg := plugin.NewRegistry(ctx)
	descReg := s.resourceSvc.Registry()
	for _, p := range loaded {
		if err := reg.Register(p, descReg); err != nil {
			slox.Warn(ctx, "plugin register error", "plugin", p.Manifest.Name, "error", err)
		} else {
			s.pluginDirs[p.Manifest.Name] = p.Dir
		}
	}
	s.registry = reg

	enricherReg := s.resourceSvc.EnricherRegistry()
	for _, p := range loaded {
		s.initPluginRuntime(p, enricherReg)
	}

	watcher, err := plugin.NewPluginWatcher(ctx, func(name string) {
		if err := s.ReloadPlugin(name); err != nil {
			slox.Warn(ctx, "plugin auto-reload failed", "plugin", name, "error", err)
		}
	})
	if err != nil {
		slox.Warn(ctx, "plugin watcher init failed", "error", err)
	} else {
		s.watcher = watcher
		for name, dir := range s.pluginDirs {
			if err := watcher.Watch(name, dir); err != nil {
				slox.Warn(ctx, "plugin watcher add failed", "plugin", name, "error", err)
			}
		}
		watcher.Start()
	}

	// Restore persisted disabled state.
	for _, name := range s.appService.Config().DisabledPlugins {
		if err := s.unloadPlugin(name, true); err != nil {
			slox.Warn(ctx, "plugin disable restore failed", "plugin", name, "error", err)
		}
		s.registry.SetStatus(name, plugin.StatusDisabled, "")
	}

	s.appService.RegisterPluginsDir(pluginsDir)

	s.appService.Emit("plugins:loaded", len(reg.GetPlugins()))
	return nil
}

func (s *PluginService) initPluginRuntime(p *plugin.LoadedPlugin, enricherReg *resource.EnricherRegistry) {
	name := p.Manifest.Name
	perms := plugin.NewPermissionSet(p.Manifest.Permissions)
	s.pluginPerms[name] = perms

	var stor *plugin.PluginStorage
	if perms.AllowsStorage() {
		st, err := plugin.NewPluginStorage(name)
		if err != nil {
			slox.Warn(s.ctx, "plugin storage init failed", "plugin", name, "error", err)
		} else {
			stor = st
			s.storages[name] = st
		}
	}

	hasEnricher := p.Manifest.Extensions != nil && p.Manifest.Extensions.Enrichers != nil
	hasWasmCommands := false
	if p.Manifest.Extensions != nil {
		for _, cmd := range p.Manifest.Extensions.Commands {
			if cmd.Component == nil {
				hasWasmCommands = true
				break
			}
		}
	}
	if !hasEnricher && !hasWasmCommands {
		return
	}

	// Wasm binary path comes from the enricher config. Command-only Wasm plugins
	// must still declare an enricher config to specify the binary path.
	if !hasEnricher {
		return
	}

	ec := p.Manifest.Extensions.Enrichers
	wasmPath := filepath.Join(p.Dir, ec.Wasm)
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		slox.Warn(s.ctx, "plugin wasm read failed", "plugin", name, "error", err)
		s.registry.SetStatus(name, plugin.StatusErrored, err.Error())
		return
	}

	deps := plugin.HostAPIDeps{
		ResourceEngine:   s.resourceSvc.Engine(),
		WatchManager:     s.resourceSvc.WatchMgr(),
		LogStreamer:      s.appService.LogStreamer(),
		ExecManager:      s.appService.ExecManager(),
		TemplateRegistry: s.resourceSvc.TemplateRegistry(),
		On:               s.appService.On,
		GetActiveContext: func() string {
			for _, c := range s.appService.ClusterManager().ListContexts() {
				if c.Status == cluster.StatusConnected {
					return c.Name
				}
			}
			return ""
		},
	}
	rt, err := plugin.NewWasmRuntime(s.ctx, wasmBytes, name, perms, stor, deps)
	if err != nil {
		slox.Warn(s.ctx, "plugin wasm init failed", "plugin", name, "error", err)
		s.registry.SetStatus(name, plugin.StatusErrored, err.Error())
		return
	}
	s.runtimes[name] = rt

	for _, gvr := range ec.Gvrs {
		existing := enricherReg.GetAll(gvr)
		if len(existing) > 0 {
			warn := fmt.Sprintf("enricher conflict on %s (multiple plugins enriching same GVR; last writer wins)", gvr)
			p.ConflictWarnings = append(p.ConflictWarnings, warn)
			slox.Warn(s.ctx, "plugin enricher GVR conflict", "plugin", name, "gvr", gvr)
		}
		enricherReg.Register(gvr, &plugin.PluginEnricher{
			Runtime:    rt,
			GVR:        gvr,
			PluginName: name,
			Ctx:        s.ctx,
			OnError: func(err error) {
				s.registry.SetStatus(name, plugin.StatusErrored, err.Error())
				s.appService.Emit("plugin:error", map[string]string{"name": name, "error": err.Error()})
			},
		})
	}
}

// InvokeCommand dispatches a plugin command asynchronously.
// Returns immediately; errors are emitted as plugin:error events.
func (s *PluginService) InvokeCommand(pluginName, commandID string) error {
	slox.Info(s.ctx, "[wasm-cmd] InvokeCommand called", "plugin", pluginName, "command", commandID)
	if s.appService != nil && s.appService.config != nil && s.appService.config.ReadOnly {
		return fmt.Errorf("app is in read-only mode")
	}
	rt, ok := s.runtimes[pluginName]
	if !ok {
		knownRuntimes := make([]string, 0, len(s.runtimes))
		for k := range s.runtimes {
			knownRuntimes = append(knownRuntimes, k)
		}
		slox.Warn(s.ctx, "[wasm-cmd] no runtime found", "plugin", pluginName, "known", knownRuntimes)
		return fmt.Errorf("no runtime for plugin %q", pluginName)
	}
	slox.Info(s.ctx, "[wasm-cmd] runtime found, dispatching goroutine", "plugin", pluginName, "command", commandID)
	go func() {
		if err := rt.CallCommand(commandID); err != nil {
			slox.Warn(s.ctx, "[wasm-cmd] CallCommand error", "plugin", pluginName, "command", commandID, "error", err)
			s.appService.Emit("plugin:error", map[string]string{"name": pluginName, "error": err.Error()})
		} else {
			slox.Info(s.ctx, "[wasm-cmd] CallCommand completed", "plugin", pluginName, "command", commandID)
		}
	}()
	return nil
}

func (s *PluginService) Shutdown() error {
	if s.watcher != nil {
		s.watcher.Stop()
	}
	for name, st := range s.storages {
		if err := st.Flush(); err != nil {
			slox.Warn(s.ctx, "plugin storage flush failed", "plugin", name, "error", err)
		}
	}
	for name, rt := range s.runtimes {
		if err := rt.Close(); err != nil {
			slox.Warn(s.ctx, "plugin wasm close failed", "plugin", name, "error", err)
		}
	}
	return nil
}

// ReloadPlugin performs a full unload-then-reload cycle for the named plugin.
// Called by the PluginWatcher on file changes.
func (s *PluginService) ReloadPlugin(name string) error {
	s.appService.Emit("plugin:reloading", map[string]string{"name": name})

	dir, ok := s.pluginDirs[name]
	if !ok {
		return fmt.Errorf("plugin dir not found for %q", name)
	}

	if err := s.unloadPlugin(name, false); err != nil {
		slox.Warn(s.ctx, "plugin unload failed during reload", "plugin", name, "error", err)
	}

	p, err := s.loader.LoadPlugin(dir)
	if err != nil {
		slox.Warn(s.ctx, "plugin reload failed", "plugin", name, "error", err)
		s.appService.Emit("plugin:error", map[string]string{"name": name, "error": err.Error()})
		return err
	}

	if err := s.registry.Register(p, s.resourceSvc.Registry()); err != nil {
		slox.Warn(s.ctx, "plugin re-register failed", "plugin", name, "error", err)
		s.appService.Emit("plugin:error", map[string]string{"name": name, "error": err.Error()})
		return err
	}

	s.pluginDirs[name] = dir
	s.initPluginRuntime(p, s.resourceSvc.EnricherRegistry())

	if s.watcher != nil {
		_ = s.watcher.Watch(name, dir)
	}

	s.appService.Emit("plugin:loaded", map[string]string{"name": name})
	s.appService.Emit("plugins:loaded", len(s.registry.GetPlugins()))
	return nil
}

// unloadPlugin removes a plugin's runtime and extension points.
// If keepEntry is false (for reload), it removes the registry entry too.
func (s *PluginService) unloadPlugin(name string, keepEntry bool) error {
	if rt, ok := s.runtimes[name]; ok {
		if err := rt.Close(); err != nil {
			slox.Warn(s.ctx, "plugin wasm close failed", "plugin", name, "error", err)
		}
		delete(s.runtimes, name)
	}

	if s.watcher != nil {
		if dir, ok := s.pluginDirs[name]; ok {
			s.watcher.Unwatch(dir)
		}
	}

	if tr := s.resourceSvc.TemplateRegistry(); tr != nil {
		tr.UnregisterPlugin(name)
	}

	if keepEntry {
		s.registry.Deactivate(name, s.resourceSvc.EnricherRegistry())
	} else {
		s.registry.Deactivate(name, s.resourceSvc.EnricherRegistry())
		s.registry.Remove(name)
		delete(s.pluginDirs, name)
	}

	return nil
}

// DisablePlugin deactivates a plugin without removing its registry entry.
func (s *PluginService) DisablePlugin(name string) error {
	if s.registry == nil {
		return fmt.Errorf("plugin registry not initialized")
	}
	if err := s.unloadPlugin(name, true); err != nil {
		return err
	}
	s.registry.SetStatus(name, plugin.StatusDisabled, "")

	_ = s.appService.Config().Update(func(c *config.Config) {
		for _, n := range c.DisabledPlugins {
			if n == name {
				return
			}
		}
		c.DisabledPlugins = append(c.DisabledPlugins, name)
	})

	s.appService.Emit("plugins:loaded", len(s.registry.GetPlugins()))
	return nil
}

// EnablePlugin reloads a disabled plugin's runtime and re-registers its extension points.
func (s *PluginService) EnablePlugin(name string) error {
	if s.registry == nil {
		return fmt.Errorf("plugin registry not initialized")
	}
	dir, ok := s.pluginDirs[name]
	if !ok {
		return fmt.Errorf("plugin dir not found for %q", name)
	}

	_ = s.appService.Config().Update(func(c *config.Config) {
		filtered := c.DisabledPlugins[:0]
		for _, n := range c.DisabledPlugins {
			if n != name {
				filtered = append(filtered, n)
			}
		}
		c.DisabledPlugins = filtered
	})

	// Remove the disabled entry so Register won't see a duplicate.
	s.registry.Remove(name)

	p, err := s.loader.LoadPlugin(dir)
	if err != nil {
		return fmt.Errorf("loading plugin %q: %w", name, err)
	}

	if err := s.registry.Register(p, s.resourceSvc.Registry()); err != nil {
		return fmt.Errorf("re-registering plugin %q: %w", name, err)
	}

	s.initPluginRuntime(p, s.resourceSvc.EnricherRegistry())

	if s.watcher != nil {
		_ = s.watcher.Watch(name, dir)
	}

	s.appService.Emit("plugins:loaded", len(s.registry.GetPlugins()))
	return nil
}

// ReloadPluginManual is the RPC-exposed alias of ReloadPlugin.
func (s *PluginService) ReloadPluginManual(name string) error {
	return s.ReloadPlugin(name)
}

// UninstallPlugin removes the plugin directory and all associated data.
func (s *PluginService) UninstallPlugin(name string) error {
	if s.registry == nil {
		return fmt.Errorf("plugin registry not initialized")
	}
	dir, ok := s.pluginDirs[name]
	if !ok {
		return fmt.Errorf("plugin dir not found for %q", name)
	}

	if err := s.unloadPlugin(name, false); err != nil {
		slox.Warn(s.ctx, "plugin unload failed during uninstall", "plugin", name, "error", err)
	}

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("removing plugin dir: %w", err)
	}

	s.appService.Emit("plugins:loaded", len(s.registry.GetPlugins()))
	return nil
}

// EmitClusterEvent notifies all active plugins with events permission about a cluster event.
// Also emits a Wails "plugin:event" event so frontend plugin components can subscribe.
func (s *PluginService) EmitClusterEvent(eventName string, payload []byte) {
	if s.registry == nil {
		return
	}

	// Notify frontend plugin components.
	var decoded interface{}
	_ = json.Unmarshal(payload, &decoded)
	s.appService.Emit("plugin:event", map[string]interface{}{
		"eventName": eventName,
		"payload":   decoded,
	})

	// Notify Wasm runtimes for plugins with events permission.
	for _, info := range s.registry.GetPlugins() {
		if info.Status != string(plugin.StatusActive) {
			continue
		}
		rt, ok := s.runtimes[info.Name]
		if !ok {
			continue
		}
		perms, ok := s.pluginPerms[info.Name]
		if !ok || !perms.AllowsEvents() {
			continue
		}
		if err := rt.CallOnEvent(eventName, payload); err != nil {
			slox.Warn(s.ctx, "plugin event call failed", "plugin", info.Name, "event", eventName, "error", err)
		}
	}
}

// GetPluginStorageKey returns a storage value for the named plugin.
func (s *PluginService) GetPluginStorageKey(pluginName, key string) (string, error) {
	st, ok := s.storages[pluginName]
	if !ok {
		return "", fmt.Errorf("no storage for plugin %q", pluginName)
	}
	val, found := st.Get(key)
	if !found {
		return "", nil
	}
	return val, nil
}

// SetPluginStorageKey sets a storage value for the named plugin.
func (s *PluginService) SetPluginStorageKey(pluginName, key, value string) error {
	st, ok := s.storages[pluginName]
	if !ok {
		return fmt.Errorf("no storage for plugin %q", pluginName)
	}
	st.Set(key, value)
	return nil
}

// DeletePluginStorageKey deletes a storage key for the named plugin.
func (s *PluginService) DeletePluginStorageKey(pluginName, key string) error {
	st, ok := s.storages[pluginName]
	if !ok {
		return fmt.Errorf("no storage for plugin %q", pluginName)
	}
	st.Delete(key)
	return nil
}

// ListPluginStorageKeys returns all storage keys for the named plugin.
func (s *PluginService) ListPluginStorageKeys(pluginName string) ([]string, error) {
	st, ok := s.storages[pluginName]
	if !ok {
		return nil, fmt.Errorf("no storage for plugin %q", pluginName)
	}
	return st.List(), nil
}

func (s *PluginService) ListPlugins() []plugin.PluginInfo {
	if s.registry == nil {
		return nil
	}
	return s.registry.GetPlugins()
}

func (s *PluginService) GetPluginDescriptors() []*resource.Descriptor {
	if s.registry == nil {
		return nil
	}
	return s.registry.GetDescriptors()
}

func (s *PluginService) GetPluginSidebarEntries() []plugin.SidebarEntry {
	if s.registry == nil {
		return nil
	}
	return s.registry.GetSidebarEntries()
}

func (s *PluginService) GetPluginDetailTabs() []plugin.DetailTabEntry {
	if s.registry == nil {
		return nil
	}
	return s.registry.GetDetailTabs()
}

func (s *PluginService) GetPluginCommands() []plugin.CommandEntry {
	if s.registry == nil {
		return nil
	}
	return s.registry.GetCommands()
}

func (s *PluginService) GetPluginOverviewFields(gvr string) []plugin.OverviewFieldEntry {
	if s.registry == nil {
		return nil
	}
	return s.registry.GetOverviewFields(gvr)
}

func (s *PluginService) GetPluginListColumns(gvr string) []plugin.ListColumnEntry {
	if s.registry == nil {
		return nil
	}
	return s.registry.GetListColumns(gvr)
}

func (s *PluginService) GetPluginContextMenuItems(gvr string) []plugin.ContextMenuEntry {
	if s.registry == nil {
		return nil
	}
	return s.registry.GetContextMenuItems(gvr)
}

func (s *PluginService) GetPluginHeaderWidgets() []plugin.HeaderWidgetEntry {
	if s.registry == nil {
		return nil
	}
	return s.registry.GetHeaderWidgets()
}

func (s *PluginService) GetPluginStatusBarWidgets() []plugin.StatusBarEntry {
	if s.registry == nil {
		return nil
	}
	return s.registry.GetStatusBarWidgets()
}

func (s *PluginService) GetPluginMetricQueries(gvr string) []plugin.MetricQueryEntry {
	if s.registry == nil {
		return nil
	}
	return s.registry.GetMetricQueries(gvr)
}

func (s *PluginService) GetPluginSettings(name string) (string, error) {
	st, ok := s.storages[name]
	if !ok {
		return "{}", nil
	}
	val, found := st.Get("settings")
	if !found {
		return "{}", nil
	}
	return val, nil
}

func (s *PluginService) SetPluginSettings(name string, settingsJSON string) error {
	st, ok := s.storages[name]
	if !ok {
		return fmt.Errorf("plugin %q not found", name)
	}

	// Validate against the plugin's settings schema if present
	if s.registry != nil {
		lp := s.registry.GetLoadedPlugin(name)
		if lp != nil && lp.Manifest != nil && lp.Manifest.Extensions != nil && lp.Manifest.Extensions.Settings != nil {
			var schemaDoc any
			schemaBytes, err := json.Marshal(lp.Manifest.Extensions.Settings.Schema)
			if err == nil {
				if err := json.Unmarshal(schemaBytes, &schemaDoc); err == nil {
					compiler := jsonschema.NewCompiler()
					if err := compiler.AddResource("settings.json", schemaDoc); err == nil {
						schema, err := compiler.Compile("settings.json")
						if err == nil {
							var value any
							if err := json.Unmarshal([]byte(settingsJSON), &value); err != nil {
								return fmt.Errorf("invalid JSON: %w", err)
							}
							if err := schema.Validate(value); err != nil {
								return fmt.Errorf("settings validation failed: %w", err)
							}
						}
					}
				}
			}
		}
	}

	st.Set("settings", settingsJSON)
	return nil
}

func (s *PluginService) GetPluginSettingsSchema(name string) (string, error) {
	if s.registry == nil {
		return "", nil
	}
	lp := s.registry.GetLoadedPlugin(name)
	if lp == nil || lp.Manifest == nil || lp.Manifest.Extensions == nil || lp.Manifest.Extensions.Settings == nil {
		return "", nil
	}
	data, err := json.Marshal(lp.Manifest.Extensions.Settings.Schema)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// InstallPlugin installs a plugin from an OCI reference (oci://...), a directory path,
// or an OCI archive (.oci.tar / .oci.tar.gz). After installation, loads the plugin and
// emits plugins:loaded.
func (s *PluginService) InstallPlugin(path string) error {
	if s.loader == nil {
		return fmt.Errorf("plugin loader not initialized")
	}

	var destDir string

	if strings.HasPrefix(path, "oci://") {
		host := strings.SplitN(strings.TrimPrefix(path, "oci://"), "/", 2)[0]
		insecure := slices.Contains(s.appService.Config().InsecureRegistries, host)
		pullFn := s.pullFn
		if pullFn == nil {
			pullFn = plugin.PullFromRegistry
		}
		if err := pullFn(path, s.pluginsDir, plugin.RemoteOpts{Insecure: insecure}); err != nil {
			return err
		}
		var err error
		destDir, err = s.findNewPluginDir()
		if err != nil {
			destDir = ""
		}
	} else {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("accessing path: %w", err)
		}
		if info.IsDir() {
			destDir, err = plugin.CopyPluginDir(path, s.pluginsDir)
			if err != nil {
				return fmt.Errorf("copying plugin dir: %w", err)
			}
		} else if strings.HasSuffix(path, ".oci.tar.gz") || strings.HasSuffix(path, ".oci.tar") {
			if err := plugin.Unpack(path, s.pluginsDir); err != nil {
				return fmt.Errorf("unpacking plugin: %w", err)
			}
			destDir, err = s.findNewPluginDir()
			if err != nil {
				destDir = ""
			}
		} else {
			return fmt.Errorf("unsupported path: must be a directory or .oci.tar/.oci.tar.gz file")
		}
	}

	if destDir != "" {
		p, err := s.loader.LoadPlugin(destDir)
		if err != nil {
			return fmt.Errorf("loading plugin: %w", err)
		}
		if err := s.registry.Register(p, s.resourceSvc.Registry()); err != nil {
			return fmt.Errorf("registering plugin: %w", err)
		}
		s.pluginDirs[p.Manifest.Name] = destDir
		s.initPluginRuntime(p, s.resourceSvc.EnricherRegistry())
		if s.watcher != nil {
			_ = s.watcher.Watch(p.Manifest.Name, destDir)
		}
	}

	s.appService.Emit("plugins:loaded", len(s.registry.GetPlugins()))
	return nil
}

// SaveRegistryCredentials persists credentials for an OCI registry host to ~/.docker/config.json.
func (s *PluginService) SaveRegistryCredentials(host, username, password string) error {
	return plugin.SaveDockerCredentials(host, username, password)
}

// AddInsecureRegistry appends host to the InsecureRegistries config list (no-op if already present).
func (s *PluginService) AddInsecureRegistry(host string) error {
	return s.appService.Config().Update(func(c *config.Config) {
		for _, h := range c.InsecureRegistries {
			if h == host {
				return
			}
		}
		c.InsecureRegistries = append(c.InsecureRegistries, host)
	})
}

// PackPlugin packs a plugin directory into an OCI tar.gz archive.
// Returns the path to the generated archive.
func (s *PluginService) PackPlugin(pluginDir string) (string, error) {
	return plugin.Pack(pluginDir, true)
}

// findNewPluginDir scans pluginsDir for a directory that has a manifest.json but is not
// yet registered in pluginDirs — i.e. a freshly installed plugin.
func (s *PluginService) findNewPluginDir() (string, error) {
	entries, err := os.ReadDir(s.pluginsDir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(s.pluginsDir, e.Name())
		if _, ok := s.pluginDirs[e.Name()]; !ok {
			if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err == nil {
				return dir, nil
			}
		}
	}
	return "", fmt.Errorf("could not find newly installed plugin dir")
}

