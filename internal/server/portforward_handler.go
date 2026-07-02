package server

import (
	"context"
	"encoding/json"

	"connectrpc.com/connect"

	kladosv1 "github.com/Vilsol/klados/gen/klados/v1"
	"github.com/Vilsol/klados/internal/config"
	"github.com/Vilsol/klados/internal/portforward"
	"github.com/Vilsol/klados/internal/services"
)

type PortForwardHandler struct {
	svc *services.PortForwardService
}

func NewPortForwardHandler(svc *services.PortForwardService) *PortForwardHandler {
	return &PortForwardHandler{svc: svc}
}

func (h *PortForwardHandler) StartForward(ctx context.Context, req *connect.Request[kladosv1.PortForwardStartRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	spec, err := h.svc.StartForward(
		m.GetContext(), m.GetNamespace(),
		portforward.TargetKind(m.GetTargetKind()),
		m.GetTargetName(), m.GetTargetGvr(),
		int(m.GetLocalPort()), int(m.GetRemotePort()),
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(spec)
}

func (h *PortForwardHandler) StopForward(ctx context.Context, req *connect.Request[kladosv1.PortForwardIDRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.StopForward(req.Msg.GetForwardId()))
}

func (h *PortForwardHandler) ConnectSavedForward(ctx context.Context, req *connect.Request[kladosv1.PortForwardSavedRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	spec, err := h.svc.ConnectSavedForward(req.Msg.GetContext(), req.Msg.GetSavedId())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(spec)
}

func (h *PortForwardHandler) ListForwards(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.ListForwards(req.Msg.GetContext()))
}

func (h *PortForwardHandler) SavePortForward(ctx context.Context, req *connect.Request[kladosv1.PortForwardSaveRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	var fwd config.SavedPortForward
	if err := json.Unmarshal(req.Msg.GetForwardJson(), &fwd); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return emptyResponse(h.svc.SavePortForward(req.Msg.GetContext(), fwd))
}

func (h *PortForwardHandler) RemoveSavedPortForward(ctx context.Context, req *connect.Request[kladosv1.PortForwardSavedRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.RemoveSavedPortForward(req.Msg.GetContext(), req.Msg.GetSavedId()))
}

func (h *PortForwardHandler) SetPortForwardEnabled(ctx context.Context, req *connect.Request[kladosv1.PortForwardSetEnabledRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.SetPortForwardEnabled(m.GetContext(), m.GetId(), m.GetEnabled()))
}

func (h *PortForwardHandler) ListSavedPortForwards(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.ListSavedPortForwards(req.Msg.GetContext()))
}
