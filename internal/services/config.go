package services

import (
	"context"
	"fmt"
	"regexp"

	"github.com/Vilsol/klados/internal/config"
)

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type ConfigService struct {
	ctx    context.Context
	config *config.Config
	appSvc *AppService
}

func NewConfigService(ctx context.Context, cfg *config.Config) *ConfigService {
	return &ConfigService{ctx: ctx, config: cfg}
}

// SetAppService wires the event emitter used for config:updated broadcasts.
func (c *ConfigService) SetAppService(appSvc *AppService) {
	c.appSvc = appSvc
}

func (c *ConfigService) GetTheme() string {
	return c.config.Theme
}

func (c *ConfigService) SetTheme(theme string) error {
	switch theme {
	case "dark", "light", "system":
	default:
		return fmt.Errorf("invalid theme: %q", theme)
	}

	return c.config.Update(func(cfg *config.Config) {
		cfg.Theme = theme
	})
}

func (c *ConfigService) GetTerminalWebGL() bool {
	return c.config.TerminalWebGL
}

func (c *ConfigService) SetTerminalWebGL(enabled bool) error {
	return c.config.Update(func(cfg *config.Config) {
		cfg.TerminalWebGL = enabled
	})
}

func (c *ConfigService) GetInsecureSkipTLSVerify() bool {
	return c.config.InsecureSkipTLSVerify
}

func (c *ConfigService) SetInsecureSkipTLSVerify(skip bool) error {
	return c.config.Update(func(cfg *config.Config) {
		cfg.InsecureSkipTLSVerify = skip
	})
}

func (c *ConfigService) GetColumnPrefs(gvr string) *config.GVRColumnPrefs {
	if c.config.ColumnPrefs == nil {
		return nil
	}
	return c.config.ColumnPrefs[gvr]
}

func (c *ConfigService) SetColumnPrefs(gvr string, prefs *config.GVRColumnPrefs) error {
	return c.config.Update(func(cfg *config.Config) {
		if cfg.ColumnPrefs == nil {
			cfg.ColumnPrefs = make(map[string]*config.GVRColumnPrefs)
		}
		cfg.ColumnPrefs[gvr] = prefs
	})
}

func (c *ConfigService) DeleteColumnPrefs(gvr string) error {
	return c.config.Update(func(cfg *config.Config) {
		delete(cfg.ColumnPrefs, gvr)
	})
}

func (c *ConfigService) GetCompactRows() bool {
	return c.config.CompactRows
}

func (c *ConfigService) SetCompactRows(compact bool) error {
	return c.config.Update(func(cfg *config.Config) {
		cfg.CompactRows = compact
	})
}

func (c *ConfigService) SetContextualAutocomplete(enabled bool) error {
	return c.config.Update(func(cfg *config.Config) {
		cfg.ContextualAutocomplete = &enabled
	})
}

func (c *ConfigService) GetConfig() *config.Config {
	return c.config
}

func (c *ConfigService) Startup(_ context.Context) error {
	if c.appSvc != nil {
		c.config.SetEmit(c.appSvc.Emit)
	}
	return nil
}

func (c *ConfigService) Shutdown() error {
	return nil
}

func (c *ConfigService) GetResolvedPrefs(ctxName string) config.ResolvedPrefs {
	return c.config.ResolveForCluster(ctxName)
}

func (c *ConfigService) SetAccentColor(color string) error {
	if color != "" && !hexColorRe.MatchString(color) {
		return fmt.Errorf("invalid hex color: %q", color)
	}
	return c.config.Update(func(cfg *config.Config) {
		cfg.AccentColor = color
	})
}

func (c *ConfigService) SetFontSize(size int) error {
	return c.config.Update(func(cfg *config.Config) {
		cfg.FontSize = size
	})
}

func (c *ConfigService) SetStartupBehavior(behavior string, cluster string) error {
	switch behavior {
	case "last", "chooser", "specific":
	default:
		return fmt.Errorf("invalid startup behavior: %q", behavior)
	}
	return c.config.Update(func(cfg *config.Config) {
		cfg.StartupBehavior = behavior
		cfg.StartupCluster = cluster
	})
}

func (c *ConfigService) SetKeybinding(actionID string, keys string) error {
	return c.config.Update(func(cfg *config.Config) {
		if cfg.Keybindings == nil {
			cfg.Keybindings = make(map[string]string)
		}
		if keys == "" {
			delete(cfg.Keybindings, actionID)
		} else {
			cfg.Keybindings[actionID] = keys
		}
	})
}

func (c *ConfigService) ResetKeybindings() error {
	return c.config.Update(func(cfg *config.Config) {
		cfg.Keybindings = nil
	})
}

func (c *ConfigService) GetClusterPrefs(ctxName string) *config.ClusterPrefs {
	var result *config.ClusterPrefs
	c.config.Read(func(cfg *config.Config) {
		if cfg.Clusters != nil {
			result = cfg.Clusters[ctxName]
		}
	})
	return result
}

func (c *ConfigService) SetClusterPrefs(ctxName string, prefs *config.ClusterPrefs) error {
	return c.config.Update(func(cfg *config.Config) {
		if cfg.Clusters == nil {
			cfg.Clusters = make(map[string]*config.ClusterPrefs)
		}
		cfg.Clusters[ctxName] = prefs
	})
}

func (c *ConfigService) DeleteClusterPrefs(ctxName string) error {
	return c.config.Update(func(cfg *config.Config) {
		delete(cfg.Clusters, ctxName)
	})
}

func (c *ConfigService) GetSavedFilters(gvr string) []config.SavedFilter {
	var result []config.SavedFilter
	c.config.Read(func(cfg *config.Config) {
		if cfg.SavedFilters != nil {
			result = cfg.SavedFilters[gvr]
		}
	})
	return result
}

func (c *ConfigService) SetSavedFilters(gvr string, filters []config.SavedFilter) error {
	return c.config.Update(func(cfg *config.Config) {
		if cfg.SavedFilters == nil {
			cfg.SavedFilters = make(map[string][]config.SavedFilter)
		}
		if len(filters) == 0 {
			delete(cfg.SavedFilters, gvr)
		} else {
			cfg.SavedFilters[gvr] = filters
		}
	})
}

func (c *ConfigService) SetVolumeBrowser(vb *config.VolumeBrowserConfig) error {
	return c.config.Update(func(cfg *config.Config) {
		if vb == nil {
			cfg.VolumeBrowser = config.VolumeBrowserConfig{}
			return
		}
		cfg.VolumeBrowser = *vb
	})
}

func (c *ConfigService) SetClusterSavedFilters(ctxName string, gvr string, filters []config.SavedFilter) error {
	return c.config.Update(func(cfg *config.Config) {
		if cfg.Clusters == nil {
			cfg.Clusters = make(map[string]*config.ClusterPrefs)
		}
		cp := cfg.Clusters[ctxName]
		if cp == nil {
			cp = &config.ClusterPrefs{}
			cfg.Clusters[ctxName] = cp
		}
		if cp.SavedFilters == nil {
			cp.SavedFilters = make(map[string][]config.SavedFilter)
		}
		if len(filters) == 0 {
			delete(cp.SavedFilters, gvr)
		} else {
			cp.SavedFilters[gvr] = filters
		}
	})
}
