package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MarvinJWendt/testza"
	"github.com/adrg/xdg"

	"github.com/Vilsol/klados/internal/config"
	"github.com/Vilsol/klados/internal/plugin"
	"github.com/Vilsol/klados/internal/resource"
)

const minimalManifest = `{
  "schemaVersion": 1,
  "name": "test-plugin",
  "version": "0.1.0",
  "displayName": "Test Plugin",
  "minHostVersion": "1.0.0"
}`

func newTestPluginService(t *testing.T) (*PluginService, *config.Config) {
	t.Helper()

	pluginsDir := t.TempDir()

	loader, err := plugin.NewLoader(pluginsDir)
	testza.AssertNoError(t, err)

	resRegistry, err := resource.NewRegistry()
	testza.AssertNoError(t, err)

	resourceSvc := &ResourceService{
		registry:    resRegistry,
		enricherReg: resource.NewEnricherRegistry(),
	}

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	xdg.Reload()
	t.Cleanup(func() { xdg.Reload() })

	cfg, err := config.Load()
	testza.AssertNoError(t, err)

	appSvc := NewAppService(cfg, nil, context.Background(), nil, nil)

	svc := &PluginService{
		appService:  appSvc,
		resourceSvc: resourceSvc,
		loader:      loader,
		registry:    plugin.NewRegistry(context.Background()),
		pluginDirs:  make(map[string]string),
		pluginPerms: make(map[string]plugin.PermissionSet),
		storages:    make(map[string]*plugin.PluginStorage),
		pluginsDir:  pluginsDir,
		ctx:         context.Background(),
	}
	return svc, cfg
}

func TestInstallPlugin_OCI_AuthError(t *testing.T) {
	svc, _ := newTestPluginService(t)
	svc.pullFn = func(ref, destDir string, opts plugin.RemoteOpts) error {
		return plugin.ErrAuthRequired
	}

	err := svc.InstallPlugin("oci://ghcr.io/test/plugin:v1")
	testza.AssertTrue(t, errors.Is(err, plugin.ErrAuthRequired))
}

func TestInstallPlugin_OCI_Success(t *testing.T) {
	svc, _ := newTestPluginService(t)
	svc.pullFn = func(ref, destDir string, opts plugin.RemoteOpts) error {
		pluginDir := filepath.Join(destDir, "test-plugin")
		testza.AssertNoError(t, os.MkdirAll(pluginDir, 0o755))
		return os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte(minimalManifest), 0o644)
	}

	err := svc.InstallPlugin("oci://ghcr.io/test/plugin:v1")
	testza.AssertNoError(t, err)
	testza.AssertTrue(t, svc.pluginDirs["test-plugin"] != "")
}

func TestInstallPlugin_OCI_InsecureFlag(t *testing.T) {
	svc, cfg := newTestPluginService(t)
	testza.AssertNoError(t, cfg.Update(func(c *config.Config) {
		c.InsecureRegistries = []string{"localhost:5000"}
	}))

	var capturedOpts plugin.RemoteOpts
	svc.pullFn = func(ref, destDir string, opts plugin.RemoteOpts) error {
		capturedOpts = opts
		return plugin.ErrAuthRequired // stop after opts captured
	}

	_ = svc.InstallPlugin("oci://localhost:5000/foo/bar:v1")
	testza.AssertTrue(t, capturedOpts.Insecure)
}

func TestSaveRegistryCredentials(t *testing.T) {
	svc, _ := newTestPluginService(t)

	home := t.TempDir()
	t.Setenv("HOME", home)

	err := svc.SaveRegistryCredentials("ghcr.io", "user", "token123")
	testza.AssertNoError(t, err)

	data, err := os.ReadFile(filepath.Join(home, ".docker", "config.json"))
	testza.AssertNoError(t, err)
	testza.AssertTrue(t, len(data) > 0)
}

func TestAddInsecureRegistry(t *testing.T) {
	svc, cfg := newTestPluginService(t)

	testza.AssertNoError(t, svc.AddInsecureRegistry("localhost:5000"))
	testza.AssertEqual(t, []string{"localhost:5000"}, cfg.InsecureRegistries)

	// idempotent
	testza.AssertNoError(t, svc.AddInsecureRegistry("localhost:5000"))
	testza.AssertEqual(t, []string{"localhost:5000"}, cfg.InsecureRegistries)

	testza.AssertNoError(t, svc.AddInsecureRegistry("registry.internal"))
	testza.AssertEqual(t, []string{"localhost:5000", "registry.internal"}, cfg.InsecureRegistries)
}
