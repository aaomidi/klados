package server

import (
	"context"

	"connectrpc.com/connect"

	kladosv1 "github.com/Vilsol/klados/gen/klados/v1"
	"github.com/Vilsol/klados/internal/services"
)

type DrainHandler struct {
	svc *services.DrainService
}

func NewDrainHandler(svc *services.DrainService) *DrainHandler {
	return &DrainHandler{svc: svc}
}

func (h *DrainHandler) StartDrain(ctx context.Context, req *connect.Request[kladosv1.DrainNodeRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.StartDrain(req.Msg.GetContext(), req.Msg.GetNodeName()))
}

func (h *DrainHandler) CancelDrain(ctx context.Context, req *connect.Request[kladosv1.DrainNodeRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.CancelDrain(req.Msg.GetContext(), req.Msg.GetNodeName()))
}

func (h *DrainHandler) IsActive(ctx context.Context, req *connect.Request[kladosv1.DrainNodeRequest]) (*connect.Response[kladosv1.DrainIsActiveResponse], error) {
	return connect.NewResponse(&kladosv1.DrainIsActiveResponse{
		Active: h.svc.IsActive(req.Msg.GetContext(), req.Msg.GetNodeName()),
	}), nil
}

func (h *DrainHandler) ListActive(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.StringListResponse], error) {
	return connect.NewResponse(&kladosv1.StringListResponse{
		Values: h.svc.ListActive(req.Msg.GetContext()),
	}), nil
}

func (h *DrainHandler) CordonNode(ctx context.Context, req *connect.Request[kladosv1.DrainNodeRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.CordonNode(req.Msg.GetContext(), req.Msg.GetNodeName()))
}

func (h *DrainHandler) UncordonNode(ctx context.Context, req *connect.Request[kladosv1.DrainNodeRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.UncordonNode(req.Msg.GetContext(), req.Msg.GetNodeName()))
}
