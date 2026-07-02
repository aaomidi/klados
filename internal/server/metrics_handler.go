package server

import (
	"context"

	"connectrpc.com/connect"

	kladosv1 "github.com/Vilsol/klados/gen/klados/v1"
	"github.com/Vilsol/klados/internal/services"
)

type MetricsHandler struct {
	svc *services.MetricsService
}

func NewMetricsHandler(svc *services.MetricsService) *MetricsHandler {
	return &MetricsHandler{svc: svc}
}

func (h *MetricsHandler) GetCapabilities(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetCapabilities(req.Msg.GetContext()))
}

func (h *MetricsHandler) GetResourceMetrics(ctx context.Context, req *connect.Request[kladosv1.MetricsResourceRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	result, err := h.svc.GetResourceMetrics(m.GetContext(), m.GetGvr(), m.GetNamespace(), m.GetName(), int(m.GetRangeMinutes()))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}

func (h *MetricsHandler) GetNamespaceMetrics(ctx context.Context, req *connect.Request[kladosv1.MetricsNamespaceRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	result, err := h.svc.GetNamespaceMetrics(m.GetContext(), m.GetNamespace(), int(m.GetRangeMinutes()))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}

func (h *MetricsHandler) GetListMetrics(ctx context.Context, req *connect.Request[kladosv1.MetricsListRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	result, err := h.svc.GetListMetrics(m.GetContext(), m.GetGvr(), m.GetNamespace())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}

func (h *MetricsHandler) GetPluginMetrics(ctx context.Context, req *connect.Request[kladosv1.MetricsResourceRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	result, err := h.svc.GetPluginMetrics(m.GetContext(), m.GetGvr(), m.GetNamespace(), m.GetName(), int(m.GetRangeMinutes()))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}

func (h *MetricsHandler) SetPrometheusEndpoint(ctx context.Context, req *connect.Request[kladosv1.MetricsSetPrometheusEndpointRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.SetPrometheusEndpoint(req.Msg.GetContext(), req.Msg.GetUrl()))
}

func (h *MetricsHandler) RedetectSources(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	result, err := h.svc.RedetectSources(req.Msg.GetContext())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}
