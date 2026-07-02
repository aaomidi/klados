package server

import (
	"context"

	"connectrpc.com/connect"

	kladosv1 "github.com/Vilsol/klados/gen/klados/v1"
	"github.com/Vilsol/klados/internal/services"
)

type ClusterHandler struct {
	svc *services.ClusterService
}

func NewClusterHandler(svc *services.ClusterService) *ClusterHandler {
	return &ClusterHandler{svc: svc}
}

func (h *ClusterHandler) ListContexts(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.ListContexts())
}

func (h *ClusterHandler) Connect(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.Connect(req.Msg.GetContext()))
}

func (h *ClusterHandler) Disconnect(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.Disconnect(req.Msg.GetContext()))
}

func (h *ClusterHandler) Activate(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.Activate(req.Msg.GetContext()))
}

func (h *ClusterHandler) Deactivate(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.Deactivate(req.Msg.GetContext()))
}

func (h *ClusterHandler) ListNamespaces(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.StringListResponse], error) {
	namespaces, err := h.svc.ListNamespaces(req.Msg.GetContext())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return connect.NewResponse(&kladosv1.StringListResponse{Values: namespaces}), nil
}

func (h *ClusterHandler) SwitchNamespace(ctx context.Context, req *connect.Request[kladosv1.SwitchNamespaceRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.SwitchNamespace(req.Msg.GetContext(), req.Msg.GetNamespace()))
}

func (h *ClusterHandler) GetActiveNamespace(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.GetActiveNamespaceResponse], error) {
	return connect.NewResponse(&kladosv1.GetActiveNamespaceResponse{
		Namespace: h.svc.GetActiveNamespace(req.Msg.GetContext()),
	}), nil
}

func (h *ClusterHandler) GetStatus(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.GetStatusResponse], error) {
	return connect.NewResponse(&kladosv1.GetStatusResponse{
		Status: int32(h.svc.GetStatus(req.Msg.GetContext())),
	}), nil
}

func (h *ClusterHandler) CreateNamespace(ctx context.Context, req *connect.Request[kladosv1.NamespaceNameRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.CreateNamespace(req.Msg.GetContext(), req.Msg.GetName()))
}

func (h *ClusterHandler) DeleteNamespace(ctx context.Context, req *connect.Request[kladosv1.NamespaceNameRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.DeleteNamespace(req.Msg.GetContext(), req.Msg.GetName()))
}

func (h *ClusterHandler) AddKubeconfigPath(ctx context.Context, req *connect.Request[kladosv1.KubeconfigPathRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	contexts, err := h.svc.AddKubeconfigPath(req.Msg.GetPath())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(contexts)
}

func (h *ClusterHandler) ImportKubeconfigContent(ctx context.Context, req *connect.Request[kladosv1.ImportKubeconfigContentRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	contexts, err := h.svc.ImportKubeconfigContent(req.Msg.GetContent())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(contexts)
}

func (h *ClusterHandler) RemoveKubeconfigPath(ctx context.Context, req *connect.Request[kladosv1.KubeconfigPathRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	contexts, err := h.svc.RemoveKubeconfigPath(req.Msg.GetPath())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(contexts)
}

func (h *ClusterHandler) GetClusterInfo(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	info, err := h.svc.GetClusterInfo(req.Msg.GetContext())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(info)
}
