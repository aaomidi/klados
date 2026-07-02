package plugin

import (
	"context"
	"testing"

	"github.com/MarvinJWendt/testza"

	"github.com/Vilsol/klados/internal/plugin/types"
	"github.com/Vilsol/klados/internal/resource"
)

func newDescReg(t *testing.T) *resource.Registry {
	t.Helper()
	reg, err := resource.NewRegistry()
	testza.AssertNoError(t, err)
	return reg
}

func makePlugin(name string, descriptors []*resource.Descriptor, sidebar []types.SidebarEntry) *LoadedPlugin {
	desc := "desc"
	return &LoadedPlugin{
		Dir: "/tmp/" + name,
		Manifest: &types.ManifestV1Json{
			SchemaVersion:  1,
			Name:           name,
			Version:        "1.0.0",
			DisplayName:    name,
			Description:    &desc,
			MinHostVersion: "1.0.0",
			Extensions: &types.Extensions{
				Sidebar: sidebar,
			},
		},
		Descriptors: descriptors,
	}
}

func TestRegistryRegisterDescriptors(t *testing.T) {
	reg := NewRegistry(context.Background())
	descReg := newDescReg(t)

	p := makePlugin("myplugin", []*resource.Descriptor{
		{Group: "cert-manager.io", Version: "v1", Resource: "certificates", Columns: []resource.Column{
			{Name: "Name", Expr: "metadata.name", RenderType: resource.RenderText},
		}},
	}, nil)

	err := reg.Register(p, descReg)
	testza.AssertNoError(t, err)
	testza.AssertLen(t, reg.GetPlugins(), 1)
	testza.AssertLen(t, reg.GetDescriptors(), 1)

	// Plugin descriptors are stored in the plugin registry only — not merged into descReg.
	// The frontend fetches them via GetPluginDescriptors() and merges client-side.
	_, inDescReg := descReg.Get("cert-manager.io.v1.certificates")
	testza.AssertFalse(t, inDescReg)

	pluginDescs := reg.GetDescriptors()
	testza.AssertEqual(t, "certificates", pluginDescs[0].Resource)
}

func TestRegistryDoesNotMutateBuiltins(t *testing.T) {
	reg := NewRegistry(context.Background())
	descReg := newDescReg(t)

	// Register built-in first
	testza.AssertNoError(t, descReg.Register(&resource.Descriptor{
		Group: "apps", Version: "v1", Resource: "deployments",
		Columns: []resource.Column{{Name: "Name", Expr: "metadata.name", RenderType: resource.RenderText}},
	}))

	// Plugin adds a column for the same GVR — should NOT mutate the builtin descriptor
	p := makePlugin("myplugin", []*resource.Descriptor{
		{Group: "apps", Version: "v1", Resource: "deployments", Columns: []resource.Column{
			{Name: "Score", Expr: "status.score", RenderType: resource.RenderText},
		}},
	}, nil)

	err := reg.Register(p, descReg)
	testza.AssertNoError(t, err)

	// Builtin is unchanged — merging is the frontend's responsibility
	d, ok := descReg.Get("apps.v1.deployments")
	testza.AssertTrue(t, ok)
	testza.AssertLen(t, d.Columns, 1)
	testza.AssertEqual(t, "Name", d.Columns[0].Name)

	// Plugin descriptor is accessible via the plugin registry
	descs := reg.GetDescriptors()
	testza.AssertLen(t, descs, 1)
	testza.AssertEqual(t, "Score", descs[0].Columns[0].Name)
}

func TestRegistryDuplicatePluginRejected(t *testing.T) {
	reg := NewRegistry(context.Background())
	descReg := newDescReg(t)

	p := makePlugin("myplugin", nil, nil)
	testza.AssertNoError(t, reg.Register(p, descReg))

	p2 := makePlugin("myplugin", nil, nil)
	err := reg.Register(p2, descReg)
	testza.AssertNotNil(t, err)
	testza.AssertContains(t, err.Error(), "already registered")
}

func TestRegistryDeactivate(t *testing.T) {
	reg := NewRegistry(context.Background())
	descReg := newDescReg(t)
	enricherReg := resource.NewEnricherRegistry()

	p := makePlugin("myplugin", nil, []types.SidebarEntry{
		{Category: "Tools", Label: "Certs", Gvr: "certs.v1.certs"},
	})
	testza.AssertNoError(t, reg.Register(p, descReg))
	testza.AssertLen(t, reg.GetSidebarEntries(), 1)

	reg.Deactivate("myplugin", enricherReg)

	// Extensions removed, but plugin entry still present
	testza.AssertLen(t, reg.GetSidebarEntries(), 0)
	testza.AssertLen(t, reg.GetPlugins(), 1)
}

func TestRegistryRemove(t *testing.T) {
	reg := NewRegistry(context.Background())
	descReg := newDescReg(t)

	p := makePlugin("myplugin", nil, []types.SidebarEntry{
		{Category: "Tools", Label: "Certs", Gvr: "certs.v1.certs"},
	})
	testza.AssertNoError(t, reg.Register(p, descReg))

	reg.Remove("myplugin")

	testza.AssertLen(t, reg.GetPlugins(), 0)
	testza.AssertLen(t, reg.GetSidebarEntries(), 0)

	// Can re-register after remove
	p2 := makePlugin("myplugin", nil, nil)
	testza.AssertNoError(t, reg.Register(p2, descReg))
}

func TestRegistrySetStatus(t *testing.T) {
	reg := NewRegistry(context.Background())
	descReg := newDescReg(t)

	p := makePlugin("myplugin", nil, nil)
	testza.AssertNoError(t, reg.Register(p, descReg))

	plugins := reg.GetPlugins()
	testza.AssertEqual(t, "active", plugins[0].Status)

	reg.SetStatus("myplugin", StatusErrored, "wasm trap")

	plugins = reg.GetPlugins()
	testza.AssertEqual(t, "errored", plugins[0].Status)
	testza.AssertEqual(t, "wasm trap", plugins[0].Error)

	reg.SetStatus("myplugin", StatusDisabled, "")
	plugins = reg.GetPlugins()
	testza.AssertEqual(t, "disabled", plugins[0].Status)
}

func makePluginWithMetrics(name string, metricsGroups []types.MetricTemplateGroup) *LoadedPlugin {
	desc := "desc"
	return &LoadedPlugin{
		Dir: "/tmp/" + name,
		Manifest: &types.ManifestV1Json{
			SchemaVersion:  1,
			Name:           name,
			Version:        "1.0.0",
			DisplayName:    name,
			Description:    &desc,
			MinHostVersion: "1.0.0",
			Extensions: &types.Extensions{
				Metrics: metricsGroups,
			},
		},
	}
}

func TestRegistryMetricQueries(t *testing.T) {
	reg := NewRegistry(context.Background())
	descReg := newDescReg(t)

	p := makePluginWithMetrics("istio-metrics", []types.MetricTemplateGroup{
		{
			Gvr: "core.v1.pods",
			Queries: []types.MetricTemplateQuery{
				{Name: "HTTP Request Rate", Query: `sum(rate(http_requests_total{namespace="{{namespace}}", pod="{{name}}"}[5m])) by (code)`, Unit: "req/s"},
				{Name: "HTTP Error Rate", Query: `sum(rate(http_requests_total{namespace="{{namespace}}", pod="{{name}}", code=~"5.."}[5m]))`, Unit: "req/s"},
			},
		},
	})

	testza.AssertNoError(t, reg.Register(p, descReg))

	queries := reg.GetMetricQueries("core.v1.pods")
	testza.AssertLen(t, queries, 2)
	testza.AssertEqual(t, "istio-metrics", queries[0].PluginName)
	testza.AssertEqual(t, "core.v1.pods", queries[0].GVR)
	testza.AssertEqual(t, "HTTP Request Rate", queries[0].Name)
	testza.AssertEqual(t, "req/s", queries[0].Unit)

	// No queries for unrelated GVR
	testza.AssertLen(t, reg.GetMetricQueries("apps.v1.deployments"), 0)
}

func TestRegistryMetricQueriesNoField(t *testing.T) {
	reg := NewRegistry(context.Background())
	descReg := newDescReg(t)

	p := makePlugin("plain-plugin", nil, nil)
	testza.AssertNoError(t, reg.Register(p, descReg))
	testza.AssertLen(t, reg.GetMetricQueries(""), 0)
}

func TestRegistryMetricQueriesDeactivate(t *testing.T) {
	reg := NewRegistry(context.Background())
	descReg := newDescReg(t)
	enricherReg := resource.NewEnricherRegistry()

	p := makePluginWithMetrics("istio-metrics", []types.MetricTemplateGroup{
		{Gvr: "core.v1.pods", Queries: []types.MetricTemplateQuery{
			{Name: "RPS", Query: "rate(requests[5m])", Unit: "req/s"},
		}},
	})
	testza.AssertNoError(t, reg.Register(p, descReg))
	testza.AssertLen(t, reg.GetMetricQueries("core.v1.pods"), 1)

	reg.Deactivate("istio-metrics", enricherReg)
	testza.AssertLen(t, reg.GetMetricQueries("core.v1.pods"), 0)
	// Plugin entry still present
	testza.AssertLen(t, reg.GetPlugins(), 1)
}

func TestRegistryMetricQueriesMultiplePlugins(t *testing.T) {
	reg := NewRegistry(context.Background())
	descReg := newDescReg(t)

	p1 := makePluginWithMetrics("istio-metrics", []types.MetricTemplateGroup{
		{Gvr: "core.v1.pods", Queries: []types.MetricTemplateQuery{
			{Name: "RPS", Query: "rate(requests[5m])", Unit: "req/s"},
		}},
	})
	p2 := makePluginWithMetrics("linkerd-metrics", []types.MetricTemplateGroup{
		{Gvr: "core.v1.pods", Queries: []types.MetricTemplateQuery{
			{Name: "Latency", Query: "histogram_quantile(0.99, rate(latency_bucket[5m]))", Unit: "ms"},
		}},
	})

	testza.AssertNoError(t, reg.Register(p1, descReg))
	testza.AssertNoError(t, reg.Register(p2, descReg))

	queries := reg.GetMetricQueries("core.v1.pods")
	testza.AssertLen(t, queries, 2)

	pluginNames := map[string]bool{}
	for _, q := range queries {
		pluginNames[q.PluginName] = true
	}
	testza.AssertTrue(t, pluginNames["istio-metrics"])
	testza.AssertTrue(t, pluginNames["linkerd-metrics"])
}

func TestRegistrySidebarEntries(t *testing.T) {
	reg := NewRegistry(context.Background())
	descReg := newDescReg(t)

	icon := "shield"
	p := makePlugin("myplugin", nil, []types.SidebarEntry{
		{Category: "Security", Label: "Certificates", Gvr: "cert-manager.io.v1.certificates", Icon: &icon},
		{Category: "Security", Label: "Issuers", Gvr: "cert-manager.io.v1.issuers"},
	})

	testza.AssertNoError(t, reg.Register(p, descReg))

	entries := reg.GetSidebarEntries()
	testza.AssertLen(t, entries, 2)
	testza.AssertEqual(t, "Security", entries[0].Category)
	testza.AssertEqual(t, "Certificates", entries[0].Label)
	testza.AssertEqual(t, "cert-manager.io.v1.certificates", entries[0].GVR)
	testza.AssertEqual(t, "shield", entries[0].Icon)
	testza.AssertEqual(t, "myplugin", entries[0].Plugin)
}
