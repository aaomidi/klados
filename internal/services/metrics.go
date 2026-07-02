package services

import (
	"context"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsClientset "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/Vilsol/klados/internal/config"
	"github.com/Vilsol/klados/internal/metrics"
	"github.com/Vilsol/slox"
)

type MetricsService struct {
	appService *AppService
	pluginSvc  *PluginService
	ctx        context.Context
}

func NewMetricsService(appSvc *AppService) *MetricsService {
	return &MetricsService{appService: appSvc}
}

//wails:ignore
func (s *MetricsService) SetPluginService(ps *PluginService) {
	s.pluginSvc = ps
}

func (s *MetricsService) Startup(ctx context.Context) error {
	s.ctx = ctx
	return nil
}

func (s *MetricsService) Shutdown() error {
	return nil
}

func (s *MetricsService) GetCapabilities(clusterCtx string) metrics.MetricsCapability {
	return s.appService.ClusterManager().GetMetricsCapability(clusterCtx)
}

func (s *MetricsService) GetResourceMetrics(clusterCtx, gvr, namespace, name string, rangeMinutes int) (*metrics.MetricsResponse, error) {
	cap := s.GetCapabilities(clusterCtx)

	if rangeMinutes > 0 && cap.HasPrometheus {
		resp, err := s.queryPrometheusRange(clusterCtx, gvr, namespace, name, rangeMinutes)
		if err != nil {
			slox.Warn(s.ctx, "prometheus range query failed, falling back to metrics-server", "error", err)
		} else {
			return resp, nil
		}
	}

	if cap.HasMetricsServer {
		provider, err := s.getMetricsServerProvider(clusterCtx)
		if err != nil {
			return nil, err
		}
		resp, err := provider.QueryInstant(s.ctx, gvr, namespace, name)
		if err != nil {
			return nil, err
		}
		// Add pod spec threshold fallback for metrics-server-only clusters
		end := time.Now()
		start := end.Add(-time.Duration(rangeMinutes) * time.Minute)
		if rangeMinutes <= 0 {
			start = end.Add(-15 * time.Minute)
		}
		if thresholds, terr := s.collectThresholds(s.ctx, clusterCtx, cap, gvr, namespace, name, start, end); terr == nil {
			resp.Thresholds = thresholds
		}
		return resp, nil
	}

	if cap.HasPrometheus {
		provider, err := s.getPrometheusProvider(clusterCtx)
		if err != nil {
			return nil, err
		}
		return provider.QueryInstant(s.ctx, gvr, namespace, name)
	}

	return nil, fmt.Errorf("no metrics source available for %q", clusterCtx)
}

func (s *MetricsService) GetNamespaceMetrics(clusterCtx, namespace string, rangeMinutes int) (*metrics.MetricsResponse, error) {
	cap := s.GetCapabilities(clusterCtx)

	if rangeMinutes > 0 && cap.HasPrometheus {
		resp, err := s.queryPrometheusRange(clusterCtx, "namespace", namespace, "", rangeMinutes)
		if err != nil {
			slox.Warn(s.ctx, "prometheus namespace query failed, falling back", "error", err)
		} else {
			return resp, nil
		}
	}

	provider, err := s.getMetricsServerProvider(clusterCtx)
	if err != nil {
		return nil, err
	}
	return provider.QueryNamespaceMetrics(s.ctx, namespace)
}

func (s *MetricsService) GetListMetrics(clusterCtx, gvr, namespace string) (map[string][]metrics.MetricResult, error) {
	sparklineKey := "sparkline:" + gvr
	queries, ok := metrics.BuiltinQueries[sparklineKey]
	if !ok {
		return map[string][]metrics.MetricResult{}, nil
	}

	cap := s.GetCapabilities(clusterCtx)

	// Prometheus path: range query over last 5 minutes, fan out by resource label
	if cap.HasPrometheus {
		provider, err := s.getPrometheusProvider(clusterCtx)
		if err == nil {
			result := make(map[string][]metrics.MetricResult)

			// Determine the label key used to fan out series
			labelKey := "pod"
			if gvr == "core.v1.nodes" {
				labelKey = "node"
			}

			vars := map[string]string{"namespace": namespace}
			end := time.Now()
			start := end.Add(-5 * time.Minute)
			step := 15 * time.Second

			for _, mq := range queries {
				promQL := metrics.SubstituteVars(mq.Query, vars)
				series, qerr := provider.QueryRange(s.ctx, promQL, start, end, step)
				if qerr != nil {
					slox.Warn(s.ctx, "sparkline query failed", "query", mq.Name, "error", qerr)
					continue
				}

				for _, ts := range series {
					resName := ts.Labels[labelKey]
					if resName == "" {
						continue
					}
					result[resName] = append(result[resName], metrics.MetricResult{
						Name:   mq.Name,
						Unit:   mq.Unit,
						Series: []metrics.TimeSeries{ts},
					})
				}
			}

			// Check resource count cap after query
			if len(result) > 200 {
				return nil, metrics.ErrTooManyResources
			}

			return result, nil
		}
		slox.Warn(s.ctx, "prometheus sparkline query failed, falling back to metrics-server", "error", err)
	}

	// metrics-server fallback
	if cap.HasMetricsServer {
		provider, err := s.getMetricsServerProvider(clusterCtx)
		if err != nil {
			return nil, err
		}
		switch gvr {
		case "core.v1.pods":
			return provider.ListPodMetrics(s.ctx, namespace)
		case "core.v1.nodes":
			return provider.ListNodeMetrics(s.ctx)
		}
		return nil, fmt.Errorf("unsupported GVR for metrics-server sparklines: %s", gvr)
	}

	return nil, fmt.Errorf("no metrics source available for %q", clusterCtx)
}

func (s *MetricsService) GetPluginMetrics(clusterCtx, gvr, namespace, name string, rangeMinutes int) (map[string][]metrics.MetricResult, error) {
	if s.pluginSvc == nil {
		return map[string][]metrics.MetricResult{}, nil
	}

	queries := s.pluginSvc.GetPluginMetricQueries(gvr)
	if len(queries) == 0 {
		return map[string][]metrics.MetricResult{}, nil
	}

	cap := s.GetCapabilities(clusterCtx)
	if !cap.HasPrometheus {
		return map[string][]metrics.MetricResult{}, nil
	}

	provider, err := s.getPrometheusProvider(clusterCtx)
	if err != nil {
		return map[string][]metrics.MetricResult{}, nil
	}

	vars := map[string]string{
		"namespace": namespace,
		"name":      name,
	}

	end := time.Now()
	start := end.Add(-time.Duration(rangeMinutes) * time.Minute)
	if rangeMinutes <= 0 {
		start = end.Add(-15 * time.Minute)
	}
	step := metrics.StepForRange(rangeMinutes)

	results := make(map[string][]metrics.MetricResult)
	for _, q := range queries {
		promQL := metrics.SubstituteVars(q.Query, vars)
		series, qerr := provider.QueryRange(s.ctx, promQL, start, end, step)
		if qerr != nil {
			slox.Warn(s.ctx, "plugin metric query failed", "plugin", q.PluginName, "query", q.Name, "error", qerr)
			continue
		}
		results[q.PluginName] = append(results[q.PluginName], metrics.MetricResult{
			Name:   q.Name,
			Unit:   q.Unit,
			Series: series,
		})
	}

	return results, nil
}

func (s *MetricsService) SetPrometheusEndpoint(clusterCtx, url string) error {
	cfg := s.appService.Config()
	if err := cfg.Update(func(c *config.Config) {
		if c.Metrics == nil {
			c.Metrics = make(map[string]*config.MetricsConfig)
		}
		if c.Metrics[clusterCtx] == nil {
			c.Metrics[clusterCtx] = &config.MetricsConfig{}
		}
		c.Metrics[clusterCtx].PrometheusURL = url
	}); err != nil {
		return fmt.Errorf("saving prometheus endpoint: %w", err)
	}

	_, err := s.RedetectSources(clusterCtx)
	return err
}

func (s *MetricsService) RedetectSources(clusterCtx string) (*metrics.MetricsCapability, error) {
	conn, err := s.appService.ClusterManager().GetConnection(clusterCtx)
	if err != nil {
		return nil, fmt.Errorf("getting connection: %w", err)
	}

	mc, err := metricsClientset.NewForConfig(conn.Config)
	if err != nil {
		return nil, fmt.Errorf("creating metrics client: %w", err)
	}
	msProvider := metrics.NewMetricsServerProvider(mc.MetricsV1beta1(), conn.Discovery)

	cap := metrics.MetricsCapability{
		HasMetricsServer: msProvider.Available(),
	}

	var manualURL string
	cfg := s.appService.Config()
	if mc, ok := cfg.Metrics[clusterCtx]; ok && mc != nil {
		manualURL = mc.PrometheusURL
	}

	if promURL, found := metrics.DetectPrometheus(s.ctx, conn.Clientset, conn.Discovery, conn.Dynamic, conn.Config, manualURL); found {
		cap.HasPrometheus = true
		cap.PrometheusURL = promURL

		// Probe for kube-state-metrics by querying a KSM-only metric
		promProv, perr := metrics.NewPrometheusProvider(promURL, conn.Config)
		if perr == nil {
			probeCtx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
			series, perr := promProv.RawQueryInstant(probeCtx, "kube_pod_container_resource_requests")
			cancel()
			cap.HasKSM = perr == nil && len(series) > 0
		}
	}

	s.appService.ClusterManager().SetMetricsCapability(clusterCtx, cap)

	s.appService.Emit(fmt.Sprintf("metrics:%s:capabilities", clusterCtx), cap)

	return &cap, nil
}

func (s *MetricsService) queryPrometheusRange(clusterCtx, gvr, namespace, name string, rangeMinutes int) (*metrics.MetricsResponse, error) {
	queries, ok := metrics.BuiltinQueries[gvr]
	if !ok {
		return nil, fmt.Errorf("no built-in queries for %q", gvr)
	}

	provider, err := s.getPrometheusProvider(clusterCtx)
	if err != nil {
		return nil, err
	}

	vars := map[string]string{
		"namespace": namespace,
		"name":      name,
	}

	end := time.Now()
	start := end.Add(-time.Duration(rangeMinutes) * time.Minute)
	step := metrics.StepForRange(rangeMinutes)

	var results []metrics.MetricResult
	for _, mq := range queries {
		promQL := metrics.SubstituteVars(mq.Query, vars)
		series, err := provider.QueryRange(s.ctx, promQL, start, end, step)
		if err != nil {
			return nil, fmt.Errorf("range query %q: %w", mq.Name, err)
		}
		results = append(results, metrics.MetricResult{
			Name:   mq.Name,
			Unit:   mq.Unit,
			Series: series,
		})
	}

	cap := s.GetCapabilities(clusterCtx)
	thresholds, terr := s.collectThresholds(s.ctx, clusterCtx, cap, gvr, namespace, name, start, end)
	if terr != nil {
		slox.Warn(s.ctx, "collectThresholds failed", "error", terr)
	}
	annotations, aerr := s.collectAnnotations(s.ctx, clusterCtx, cap, namespace, name, start, end)
	if aerr != nil {
		slox.Warn(s.ctx, "collectAnnotations failed", "error", aerr)
	}
	return &metrics.MetricsResponse{Metrics: results, Thresholds: thresholds, Annotations: annotations}, nil
}

// collectThresholds returns request/limit overlay lines for the given resource.
// Uses Prometheus/KSM for time-varying data when available; falls back to pod spec constants.
func (s *MetricsService) collectThresholds(ctx context.Context, clusterCtx string, cap metrics.MetricsCapability, gvr, namespace, name string, start, end time.Time) ([]metrics.ThresholdLine, error) {
	thresholdKey := gvr + ":thresholds"
	queries, ok := metrics.BuiltinQueries[thresholdKey]
	if !ok {
		return nil, nil
	}

	if cap.HasKSM {
		provider, err := s.getPrometheusProvider(clusterCtx)
		if err != nil {
			return nil, err
		}

		vars := map[string]string{"namespace": namespace, "name": name}
		step := metrics.StepForRange(int(end.Sub(start).Minutes()))
		var lines []metrics.ThresholdLine

		for _, mq := range queries {
			promQL := metrics.SubstituteVars(mq.Query, vars)
			series, err := provider.QueryRange(ctx, promQL, start, end, step)
			if err != nil {
				slox.Warn(ctx, "threshold query failed", "query", mq.Name, "error", err)
				continue
			}
			for _, ts := range series {
				container := ts.Labels["container"]
				label := mq.Name
				if container != "" {
					label = mq.Name + ":" + container
				}
				lines = append(lines, metrics.ThresholdLine{Label: label, Series: ts.Points})
			}
		}
		return lines, nil
	}

	// Pod spec fallback — constant lines
	if gvr != "core.v1.pods" {
		return nil, nil
	}

	conn, err := s.appService.ClusterManager().GetConnection(clusterCtx)
	if err != nil {
		return nil, fmt.Errorf("getting connection: %w", err)
	}

	pod, err := conn.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting pod for threshold fallback: %w", err)
	}

	startTs := start.Unix()
	endTs := end.Unix()

	var lines []metrics.ThresholdLine
	for _, c := range pod.Spec.Containers {
		type resourceSpec struct {
			queryName string
			qty       *corev1.ResourceList
		}
		for _, resource := range []struct {
			queryName string
			resName   corev1.ResourceName
			list      corev1.ResourceList
		}{
			{"CPU Request", corev1.ResourceCPU, c.Resources.Requests},
			{"CPU Limit", corev1.ResourceCPU, c.Resources.Limits},
			{"Memory Request", corev1.ResourceMemory, c.Resources.Requests},
			{"Memory Limit", corev1.ResourceMemory, c.Resources.Limits},
		} {
			qty, ok := resource.list[resource.resName]
			if !ok {
				continue
			}
			val := qty.AsApproximateFloat64()
			lines = append(lines, metrics.ThresholdLine{
				Label: resource.queryName + ":" + c.Name,
				Series: []metrics.TimeSeriesPoint{
					{Timestamp: startTs, Value: val},
					{Timestamp: endTs, Value: val},
				},
			})
		}
	}
	return lines, nil
}

// collectAnnotations gathers OOMKill, CPU throttle, and warning event markers.
func (s *MetricsService) collectAnnotations(ctx context.Context, clusterCtx string, cap metrics.MetricsCapability, namespace, name string, start, end time.Time) ([]metrics.Annotation, error) {
	conn, err := s.appService.ClusterManager().GetConnection(clusterCtx)
	if err != nil {
		return nil, fmt.Errorf("getting connection: %w", err)
	}

	var annotations []metrics.Annotation

	// OOMKill — from pod status
	pod, err := conn.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		for _, cs := range pod.Status.ContainerStatuses {
			term := cs.LastTerminationState.Terminated
			if term == nil || term.Reason != "OOMKilled" {
				continue
			}
			ts := term.FinishedAt.Time
			if !ts.IsZero() && ts.After(start) && ts.Before(end) {
				annotations = append(annotations, metrics.Annotation{
					Timestamp: ts.Unix(),
					Label:     "OOMKilled",
					Severity:  "error",
				})
			}
		}
	}

	// CPU throttling — Prometheus only, silently skip if unavailable
	if cap.HasPrometheus {
		provider, perr := s.getPrometheusProvider(clusterCtx)
		if perr == nil {
			throttleQuery := fmt.Sprintf(
				`rate(container_cpu_cfs_throttled_periods_total{namespace=%q, pod=%q}[1m]) / rate(container_cpu_cfs_periods_total{namespace=%q, pod=%q}[1m]) > 0.5`,
				namespace, name, namespace, name,
			)
			step := metrics.StepForRange(int(end.Sub(start).Minutes()))
			series, qerr := provider.QueryRange(ctx, throttleQuery, start, end, step)
			if qerr == nil {
				for _, ts := range series {
					for _, pt := range ts.Points {
						annotations = append(annotations, metrics.Annotation{
							Timestamp: pt.Timestamp,
							Label:     "CPU Throttled",
							Severity:  "warning",
						})
					}
				}
			}
		}
	}

	// All events (Warning → warning severity, Normal → info severity)
	fieldSelector := fmt.Sprintf("involvedObject.name=%s", name)
	events, eerr := conn.Clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if eerr == nil {
		for _, ev := range events.Items {
			var ts time.Time
			if !ev.EventTime.IsZero() {
				ts = ev.EventTime.Time
			} else if !ev.LastTimestamp.IsZero() {
				ts = ev.LastTimestamp.Time
			} else {
				ts = ev.FirstTimestamp.Time
			}
			if ts.IsZero() || !ts.After(start) || !ts.Before(end) {
				continue
			}
			severity := "info"
			if ev.Type == "Warning" {
				severity = "warning"
			}
			annotations = append(annotations, metrics.Annotation{
				Timestamp: ts.Unix(),
				Label:     ev.Reason,
				Severity:  severity,
			})
		}
	}

	sort.Slice(annotations, func(i, j int) bool {
		return annotations[i].Timestamp < annotations[j].Timestamp
	})

	return annotations, nil
}

func (s *MetricsService) getPrometheusProvider(clusterCtx string) (*metrics.PrometheusProvider, error) {
	cap := s.GetCapabilities(clusterCtx)
	if !cap.HasPrometheus || cap.PrometheusURL == "" {
		return nil, fmt.Errorf("prometheus not available on %q", clusterCtx)
	}

	conn, err := s.appService.ClusterManager().GetConnection(clusterCtx)
	if err != nil {
		return nil, fmt.Errorf("getting connection: %w", err)
	}

	return metrics.NewPrometheusProvider(cap.PrometheusURL, conn.Config)
}

func (s *MetricsService) getMetricsServerProvider(clusterCtx string) (*metrics.MetricsServerProvider, error) {
	conn, err := s.appService.ClusterManager().GetConnection(clusterCtx)
	if err != nil {
		return nil, fmt.Errorf("getting connection: %w", err)
	}
	if !conn.MetricsCapability.HasMetricsServer {
		return nil, fmt.Errorf("metrics-server not available on %q", clusterCtx)
	}
	mc, err := metricsClientset.NewForConfig(conn.Config)
	if err != nil {
		return nil, fmt.Errorf("creating metrics client: %w", err)
	}
	return metrics.NewMetricsServerProvider(mc.MetricsV1beta1(), conn.Discovery), nil
}
