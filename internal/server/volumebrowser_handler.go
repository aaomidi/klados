package server

import (
	"context"
	"encoding/json"
	"errors"

	"connectrpc.com/connect"

	kladosv1 "github.com/Vilsol/klados/gen/klados/v1"
	"github.com/Vilsol/klados/internal/services"
)

type VolumeBrowserHandler struct {
	svc *services.VolumeBrowserService
}

func NewVolumeBrowserHandler(svc *services.VolumeBrowserService) *VolumeBrowserHandler {
	return &VolumeBrowserHandler{svc: svc}
}

func decodeSpawnRequest(data []byte) (services.SpawnRequestDTO, error) {
	var req services.SpawnRequestDTO
	err := json.Unmarshal(data, &req)
	return req, err
}

// spawnError preserves the structured CollisionError fields the frontend
// inspects, riding them in the connect error details message.
func spawnError(err error) error {
	var collision *services.CollisionError
	if errors.As(err, &collision) {
		detail, _ := json.Marshal(collision)
		return connect.NewError(connect.CodeAlreadyExists, errors.New(string(detail)))
	}
	return connect.NewError(connect.CodeUnknown, err)
}

func (h *VolumeBrowserHandler) Spawn(ctx context.Context, req *connect.Request[kladosv1.VolumeBrowserSpawnRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	dto, err := decodeSpawnRequest(req.Msg.GetRequestJson())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	result, err := h.svc.Spawn(dto)
	if err != nil {
		return nil, spawnError(err)
	}
	return jsonResponse(result)
}

func (h *VolumeBrowserHandler) Stop(ctx context.Context, req *connect.Request[kladosv1.VolumeBrowserIDRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.Stop(req.Msg.GetId()))
}

func (h *VolumeBrowserHandler) Replace(ctx context.Context, req *connect.Request[kladosv1.VolumeBrowserReplaceRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	dto, err := decodeSpawnRequest(req.Msg.GetRequestJson())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	result, err := h.svc.Replace(req.Msg.GetId(), dto)
	if err != nil {
		return nil, spawnError(err)
	}
	return jsonResponse(result)
}

func (h *VolumeBrowserHandler) AttachTab(ctx context.Context, req *connect.Request[kladosv1.VolumeBrowserAttachTabRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.AttachTab(req.Msg.GetId(), req.Msg.GetTabId()))
}

func (h *VolumeBrowserHandler) ListManaged(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.ListManaged(req.Msg.GetContext()))
}

func (h *VolumeBrowserHandler) ScanOrphans(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	result, err := h.svc.ScanOrphans(req.Msg.GetContext())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}

func (h *VolumeBrowserHandler) CleanupOrphans(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.CleanupOrphans(req.Msg.GetContext()))
}

func (h *VolumeBrowserHandler) TriggerOrphanScan(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	result, err := h.svc.TriggerOrphanScan(req.Msg.GetContext())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}
