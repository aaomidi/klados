package plugin

import (
	"context"
	"fmt"
	"github.com/sasha-s/go-deadlock"

	"github.com/Vilsol/slox"

	"github.com/Vilsol/klados/internal/plugin/types"
	"github.com/Vilsol/klados/internal/resource"
)

type PluginStatus string

const (
	StatusActive   PluginStatus = "active"
	StatusDisabled PluginStatus = "disabled"
	StatusErrored  PluginStatus = "errored"
)

type SidebarEntry struct {
	Category string `json:"category"`
	Label    string `json:"label"`
	GVR      string `json:"gvr"`
	Icon     string `json:"icon,omitempty"`
	Plugin   string `json:"plugin"`
}

type ResourcePerm struct {
	Group    string   `json:"group"`
	Version  string   `json:"version"`
	Resource string   `json:"resource"`
	Verbs    []string `json:"verbs"`
}

type DetailTabEntry struct {
	PluginName string       `json:"pluginName"`
	GVR        string       `json:"gvr"`
	ID         string       `json:"id"`
	Label      string       `json:"label"`
	Component  string       `json:"component"`
	Perms      PermsSummary `json:"perms"`
}

type CommandEntry struct {
	PluginName string       `json:"pluginName"`
	ID         string       `json:"id"`
	Label      string       `json:"label"`
	Icon       *string      `json:"icon,omitempty"`
	Component  *string      `json:"component,omitempty"`
	Perms      PermsSummary `json:"perms"`
}

type OverviewFieldEntry struct {
	PluginName string `json:"pluginName"`
	GVR        string `json:"gvr"`
	ID         string `json:"id"`
	Label      string `json:"label"`
	Component  string `json:"component"`
}

type ListColumnEntry struct {
	PluginName string `json:"pluginName"`
	GVR        string `json:"gvr"`
	ID         string `json:"id"`
	Label      string `json:"label"`
	Component  string `json:"component"`
}

type ContextMenuEntry struct {
	PluginName string `json:"pluginName"`
	GVR        string `json:"gvr"`
	ID         string `json:"id"`
	Label      string `json:"label"`
	Component  string `json:"component"`
}

type HeaderWidgetEntry struct {
	PluginName string `json:"pluginName"`
	ID         string `json:"id"`
	Component  string `json:"component"`
}

type StatusBarEntry struct {
	PluginName string `json:"pluginName"`
	ID         string `json:"id"`
	Component  string `json:"component"`
}

type MetricQueryEntry struct {
	PluginName string `json:"pluginName"`
	GVR        string `json:"gvr"`
	Name       string `json:"name"`
	Query      string `json:"query"`
	Unit       string `json:"unit"`
}

type PermsSummary struct {
	Resources []ResourcePerm `json:"resources,omitempty"`
	Logs      bool           `json:"logs,omitempty"`
	Exec      bool           `json:"exec,omitempty"`
	Storage   bool           `json:"storage,omitempty"`
	Events    bool           `json:"events,omitempty"`
	Wasi      []string       `json:"wasi,omitempty"`
}

type PluginInfo struct {
	Name             string        `json:"name"`
	Version          string        `json:"version"`
	DisplayName      string        `json:"displayName"`
	Description      string        `json:"description,omitempty"`
	Status           string        `json:"status"`
	Error            string        `json:"error,omitempty"`
	ConflictWarnings []string      `json:"conflictWarnings,omitempty"`
	Dir              string        `json:"dir"`
	Permissions      *PermsSummary `json:"permissions,omitempty"`
}

type Registry struct {
	mu               deadlock.RWMutex
	plugins          []*LoadedPlugin
	sidebarEntries   []SidebarEntry
	detailTabs       []DetailTabEntry
	commands         []CommandEntry
	overviewFields   []OverviewFieldEntry
	listColumns      []ListColumnEntry
	contextMenuItems []ContextMenuEntry
	headerWidgets    []HeaderWidgetEntry
	statusBarWidgets []StatusBarEntry
	metricQueries    []MetricQueryEntry
	pluginNames      map[string]struct{}
	ctx              context.Context
}

func NewRegistry(ctx context.Context) *Registry {
	return &Registry{pluginNames: make(map[string]struct{}), ctx: ctx}
}

// Register validates a plugin's descriptors and collects sidebar entries.
// Descriptors are stored in the plugin registry only — the frontend is responsible
// for merging them with builtins (via GetPluginDescriptors), which supports hot-reload.
// Returns an error if the plugin name is already registered.
func (r *Registry) Register(p *LoadedPlugin, descReg *resource.Registry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := p.Manifest.Name
	if _, exists := r.pluginNames[name]; exists {
		return fmt.Errorf("plugin %q already registered", name)
	}

	for _, d := range p.Descriptors {
		if err := descReg.Validate(d); err != nil {
			return fmt.Errorf("plugin %q descriptor %s: %w", name, d.GVR(), err)
		}
	}

	if p.Manifest.Extensions != nil {
		for _, se := range p.Manifest.Extensions.Sidebar {
			r.sidebarEntries = append(r.sidebarEntries, toSidebarEntry(se, name))
		}
		perms := derefPermsSummary(buildPermsSummary(p.Manifest.Permissions))
		for _, dt := range p.Manifest.Extensions.DetailTabs {
			r.detailTabs = append(r.detailTabs, DetailTabEntry{
				PluginName: name,
				GVR:        dt.Gvr,
				ID:         dt.Id,
				Label:      dt.Label,
				Component:  dt.Component,
				Perms:      perms,
			})
		}
		for _, cmd := range p.Manifest.Extensions.Commands {
			r.commands = append(r.commands, CommandEntry{
				PluginName: name,
				ID:         cmd.Id,
				Label:      cmd.Label,
				Icon:       cmd.Icon,
				Component:  cmd.Component,
				Perms:      perms,
			})
		}
		for _, of := range p.Manifest.Extensions.OverviewFields {
			gvr := ""
			if of.Gvr != nil {
				gvr = *of.Gvr
			}
			r.overviewFields = append(r.overviewFields, OverviewFieldEntry{
				PluginName: name, GVR: gvr, ID: of.Id, Label: of.Label, Component: of.Component,
			})
		}
		for _, lc := range p.Manifest.Extensions.ListColumns {
			gvr := ""
			if lc.Gvr != nil {
				gvr = *lc.Gvr
			}
			r.listColumns = append(r.listColumns, ListColumnEntry{
				PluginName: name, GVR: gvr, ID: lc.Id, Label: lc.Label, Component: lc.Component,
			})
		}
		for _, cm := range p.Manifest.Extensions.ContextMenu {
			gvr := ""
			if cm.Gvr != nil {
				gvr = *cm.Gvr
			}
			r.contextMenuItems = append(r.contextMenuItems, ContextMenuEntry{
				PluginName: name, GVR: gvr, ID: cm.Id, Label: cm.Label, Component: cm.Component,
			})
		}
		for _, hw := range p.Manifest.Extensions.HeaderWidgets {
			r.headerWidgets = append(r.headerWidgets, HeaderWidgetEntry{
				PluginName: name, ID: hw.Id, Component: hw.Component,
			})
		}
		for _, sb := range p.Manifest.Extensions.StatusBar {
			r.statusBarWidgets = append(r.statusBarWidgets, StatusBarEntry{
				PluginName: name, ID: sb.Id, Component: sb.Component,
			})
		}
		for _, mg := range p.Manifest.Extensions.Metrics {
			for _, q := range mg.Queries {
				r.metricQueries = append(r.metricQueries, MetricQueryEntry{
					PluginName: name, GVR: mg.Gvr, Name: q.Name, Query: q.Query, Unit: q.Unit,
				})
			}
		}
	}

	p.Status = StatusActive
	r.pluginNames[name] = struct{}{}
	r.plugins = append(r.plugins, p)
	slox.Info(r.ctx, "plugin registered", "plugin", name)
	return nil
}

// Deactivate removes a plugin's extension points (sidebar, tabs, commands, enrichers)
// but keeps its entry in the registry for management UI display.
func (r *Registry) Deactivate(name string, enricherReg *resource.EnricherRegistry) {
	slox.Info(r.ctx, "plugin deactivated", "plugin", name)
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sidebarEntries = filterSidebarEntries(r.sidebarEntries, name)
	r.detailTabs = filterDetailTabs(r.detailTabs, name)
	r.commands = filterCommands(r.commands, name)
	r.overviewFields = filterOverviewFields(r.overviewFields, name)
	r.listColumns = filterListColumns(r.listColumns, name)
	r.contextMenuItems = filterContextMenuItems(r.contextMenuItems, name)
	r.headerWidgets = filterHeaderWidgets(r.headerWidgets, name)
	r.statusBarWidgets = filterStatusBarWidgets(r.statusBarWidgets, name)
	r.metricQueries = filterMetricQueries(r.metricQueries, name)

	if enricherReg != nil {
		enricherReg.UnregisterPlugin(name)
	}
}

// Remove completely removes a plugin from the registry (for uninstall).
func (r *Registry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var plugins []*LoadedPlugin
	for _, p := range r.plugins {
		if p.Manifest.Name != name {
			plugins = append(plugins, p)
		}
	}
	r.plugins = plugins
	delete(r.pluginNames, name)

	r.sidebarEntries = filterSidebarEntries(r.sidebarEntries, name)
	r.detailTabs = filterDetailTabs(r.detailTabs, name)
	r.commands = filterCommands(r.commands, name)
	r.overviewFields = filterOverviewFields(r.overviewFields, name)
	r.listColumns = filterListColumns(r.listColumns, name)
	r.contextMenuItems = filterContextMenuItems(r.contextMenuItems, name)
	r.headerWidgets = filterHeaderWidgets(r.headerWidgets, name)
	r.statusBarWidgets = filterStatusBarWidgets(r.statusBarWidgets, name)
	r.metricQueries = filterMetricQueries(r.metricQueries, name)
}

// SetStatus updates the status of a registered plugin.
func (r *Registry) SetStatus(name string, status PluginStatus, errMsg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.plugins {
		if p.Manifest.Name == name {
			p.Status = status
			p.ErrorMessage = errMsg
			return
		}
	}
}

func (r *Registry) GetPlugins() []PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]PluginInfo, len(r.plugins))
	for i, p := range r.plugins {
		out[i] = toPluginInfo(p)
	}
	return out
}

func (r *Registry) GetDescriptors() []*resource.Descriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []*resource.Descriptor
	for _, p := range r.plugins {
		if p.Status == StatusActive {
			out = append(out, p.Descriptors...)
		}
	}
	return out
}

func (r *Registry) GetSidebarEntries() []SidebarEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sidebarEntries
}

func (r *Registry) GetDetailTabs() []DetailTabEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.detailTabs
}

func (r *Registry) GetCommands() []CommandEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.commands
}

func toPluginInfo(p *LoadedPlugin) PluginInfo {
	desc := ""
	if p.Manifest.Description != nil {
		desc = *p.Manifest.Description
	}
	status := string(p.Status)
	if status == "" {
		status = string(StatusActive)
	}
	return PluginInfo{
		Name:             p.Manifest.Name,
		Version:          p.Manifest.Version,
		DisplayName:      p.Manifest.DisplayName,
		Description:      desc,
		Status:           status,
		Error:            p.ErrorMessage,
		ConflictWarnings: p.ConflictWarnings,
		Dir:              p.Dir,
		Permissions:      buildPermsSummary(p.Manifest.Permissions),
	}
}

func derefPermsSummary(p *PermsSummary) PermsSummary {
	if p == nil {
		return PermsSummary{}
	}
	return *p
}

func buildPermsSummary(perms *types.Permissions) *PermsSummary {
	if perms == nil {
		return nil
	}
	s := &PermsSummary{
		Resources: toResourcePerms(perms),
	}
	if perms.Logs != nil {
		s.Logs = *perms.Logs
	}
	if perms.Exec != nil {
		s.Exec = *perms.Exec
	}
	if perms.Storage != nil {
		s.Storage = *perms.Storage
	}
	if perms.Events != nil {
		s.Events = *perms.Events
	}
	if perms.Wasi != nil {
		w := perms.Wasi
		for _, cap := range []struct {
			flag *bool
			name string
		}{
			{w.Clock, "clock"}, {w.Env, "env"}, {w.Filesystem, "filesystem"}, {w.Network, "network"},
		} {
			if cap.flag != nil && *cap.flag {
				s.Wasi = append(s.Wasi, cap.name)
			}
		}
	}
	return s
}

func toResourcePerms(perms *types.Permissions) []ResourcePerm {
	if perms == nil {
		return nil
	}
	out := make([]ResourcePerm, len(perms.Resources))
	for i, rp := range perms.Resources {
		verbs := make([]string, len(rp.Verbs))
		for j, v := range rp.Verbs {
			verbs[j] = string(v)
		}
		out[i] = ResourcePerm{
			Group:    rp.Group,
			Version:  rp.Version,
			Resource: rp.Resource,
			Verbs:    verbs,
		}
	}
	return out
}

func toSidebarEntry(se types.SidebarEntry, pluginName string) SidebarEntry {
	icon := ""
	if se.Icon != nil {
		icon = *se.Icon
	}
	return SidebarEntry{
		Category: se.Category,
		Label:    se.Label,
		GVR:      se.Gvr,
		Icon:     icon,
		Plugin:   pluginName,
	}
}

func filterSidebarEntries(entries []SidebarEntry, name string) []SidebarEntry {
	var out []SidebarEntry
	for _, e := range entries {
		if e.Plugin != name {
			out = append(out, e)
		}
	}
	return out
}

func filterDetailTabs(tabs []DetailTabEntry, name string) []DetailTabEntry {
	var out []DetailTabEntry
	for _, t := range tabs {
		if t.PluginName != name {
			out = append(out, t)
		}
	}
	return out
}

func filterCommands(cmds []CommandEntry, name string) []CommandEntry {
	var out []CommandEntry
	for _, c := range cmds {
		if c.PluginName != name {
			out = append(out, c)
		}
	}
	return out
}

func filterOverviewFields(entries []OverviewFieldEntry, name string) []OverviewFieldEntry {
	var out []OverviewFieldEntry
	for _, e := range entries {
		if e.PluginName != name {
			out = append(out, e)
		}
	}
	return out
}

func filterListColumns(entries []ListColumnEntry, name string) []ListColumnEntry {
	var out []ListColumnEntry
	for _, e := range entries {
		if e.PluginName != name {
			out = append(out, e)
		}
	}
	return out
}

func filterContextMenuItems(entries []ContextMenuEntry, name string) []ContextMenuEntry {
	var out []ContextMenuEntry
	for _, e := range entries {
		if e.PluginName != name {
			out = append(out, e)
		}
	}
	return out
}

func filterHeaderWidgets(entries []HeaderWidgetEntry, name string) []HeaderWidgetEntry {
	var out []HeaderWidgetEntry
	for _, e := range entries {
		if e.PluginName != name {
			out = append(out, e)
		}
	}
	return out
}

func filterStatusBarWidgets(entries []StatusBarEntry, name string) []StatusBarEntry {
	var out []StatusBarEntry
	for _, e := range entries {
		if e.PluginName != name {
			out = append(out, e)
		}
	}
	return out
}

func (r *Registry) GetOverviewFields(gvr string) []OverviewFieldEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if gvr == "" {
		return r.overviewFields
	}
	var out []OverviewFieldEntry
	for _, e := range r.overviewFields {
		if e.GVR == gvr {
			out = append(out, e)
		}
	}
	return out
}

func (r *Registry) GetListColumns(gvr string) []ListColumnEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if gvr == "" {
		return r.listColumns
	}
	var out []ListColumnEntry
	for _, e := range r.listColumns {
		if e.GVR == gvr {
			out = append(out, e)
		}
	}
	return out
}

func (r *Registry) GetContextMenuItems(gvr string) []ContextMenuEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if gvr == "" {
		return r.contextMenuItems
	}
	var out []ContextMenuEntry
	for _, e := range r.contextMenuItems {
		if e.GVR == gvr {
			out = append(out, e)
		}
	}
	return out
}

func (r *Registry) GetHeaderWidgets() []HeaderWidgetEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.headerWidgets
}

func (r *Registry) GetStatusBarWidgets() []StatusBarEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.statusBarWidgets
}

func (r *Registry) GetLoadedPlugin(name string) *LoadedPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		if p.Manifest != nil && p.Manifest.Name == name {
			return p
		}
	}
	return nil
}

func (r *Registry) GetMetricQueries(gvr string) []MetricQueryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if gvr == "" {
		return r.metricQueries
	}
	var out []MetricQueryEntry
	for _, e := range r.metricQueries {
		if e.GVR == gvr {
			out = append(out, e)
		}
	}
	return out
}

func filterMetricQueries(entries []MetricQueryEntry, name string) []MetricQueryEntry {
	var out []MetricQueryEntry
	for _, e := range entries {
		if e.PluginName != name {
			out = append(out, e)
		}
	}
	return out
}
