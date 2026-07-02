package server

import (
	"context"

	"connectrpc.com/connect"

	kladosv1 "github.com/Vilsol/klados/gen/klados/v1"
	"github.com/Vilsol/klados/internal/services"
)

type HelmHandler struct {
	svc *services.HelmService
}

func NewHelmHandler(svc *services.HelmService) *HelmHandler {
	return &HelmHandler{svc: svc}
}

func (h *HelmHandler) Rollback(ctx context.Context, req *connect.Request[kladosv1.HelmRollbackRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.Rollback(ctx, m.GetContext(), m.GetNamespace(), m.GetReleaseName(), int(m.GetRevision()), services.HelmRollbackOpts{
		Wait:         m.GetWait(),
		Timeout:      int(m.GetTimeout()),
		DisableHooks: m.GetDisableHooks(),
	}))
}

func (h *HelmHandler) Uninstall(ctx context.Context, req *connect.Request[kladosv1.HelmUninstallRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.Uninstall(ctx, m.GetContext(), m.GetNamespace(), m.GetReleaseName(), services.HelmUninstallOpts{
		Wait:         m.GetWait(),
		Timeout:      int(m.GetTimeout()),
		DisableHooks: m.GetDisableHooks(),
		KeepHistory:  m.GetKeepHistory(),
	}))
}

func (h *HelmHandler) Test(ctx context.Context, req *connect.Request[kladosv1.HelmTestRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	result, err := h.svc.Test(ctx, m.GetContext(), m.GetNamespace(), m.GetReleaseName(), services.HelmTestOpts{
		Timeout: int(m.GetTimeout()),
		Filters: m.GetFilters(),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}

func (h *HelmHandler) ForceDeleteReleaseSecret(ctx context.Context, req *connect.Request[kladosv1.HelmRevisionRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.ForceDeleteReleaseSecret(ctx, m.GetContext(), m.GetNamespace(), m.GetReleaseName(), int(m.GetRevision())))
}

func (h *HelmHandler) GetValues(ctx context.Context, req *connect.Request[kladosv1.HelmGetValuesRequest]) (*connect.Response[kladosv1.HelmTextResponse], error) {
	m := req.Msg
	text, err := h.svc.GetValues(ctx, m.GetContext(), m.GetNamespace(), m.GetReleaseName(), m.GetComputed(), int(m.GetRevision()))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return connect.NewResponse(&kladosv1.HelmTextResponse{Text: text}), nil
}

func (h *HelmHandler) GetManifest(ctx context.Context, req *connect.Request[kladosv1.HelmRevisionRequest]) (*connect.Response[kladosv1.HelmTextResponse], error) {
	m := req.Msg
	text, err := h.svc.GetManifest(ctx, m.GetContext(), m.GetNamespace(), m.GetReleaseName(), int(m.GetRevision()))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return connect.NewResponse(&kladosv1.HelmTextResponse{Text: text}), nil
}

func (h *HelmHandler) GetHistory(ctx context.Context, req *connect.Request[kladosv1.HelmReleaseRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	result, err := h.svc.GetHistory(ctx, m.GetContext(), m.GetNamespace(), m.GetReleaseName())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}

func (h *HelmHandler) GetNotes(ctx context.Context, req *connect.Request[kladosv1.HelmRevisionRequest]) (*connect.Response[kladosv1.HelmTextResponse], error) {
	m := req.Msg
	text, err := h.svc.GetNotes(ctx, m.GetContext(), m.GetNamespace(), m.GetReleaseName(), int(m.GetRevision()))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return connect.NewResponse(&kladosv1.HelmTextResponse{Text: text}), nil
}

func (h *HelmHandler) GetHooks(ctx context.Context, req *connect.Request[kladosv1.HelmRevisionRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	result, err := h.svc.GetHooks(ctx, m.GetContext(), m.GetNamespace(), m.GetReleaseName(), int(m.GetRevision()))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}

func (h *HelmHandler) GetOwnedResources(ctx context.Context, req *connect.Request[kladosv1.HelmGetOwnedResourcesRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	result, err := h.svc.GetOwnedResources(ctx, m.GetContext(), m.GetNamespace(), m.GetReleaseName(), m.GetScanAll())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}

func (h *HelmHandler) DiffRevisions(ctx context.Context, req *connect.Request[kladosv1.HelmDiffRevisionsRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	result, err := h.svc.DiffRevisions(ctx, m.GetContext(), m.GetNamespace(), m.GetReleaseName(), int(m.GetFrom()), int(m.GetTo()))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}

func (h *HelmHandler) GetReleasePermissions(ctx context.Context, req *connect.Request[kladosv1.HelmReleaseRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	result, err := h.svc.GetReleasePermissions(ctx, m.GetContext(), m.GetNamespace(), m.GetReleaseName())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}

func (h *HelmHandler) CleanupTestPods(ctx context.Context, req *connect.Request[kladosv1.HelmReleaseRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.CleanupTestPods(ctx, m.GetContext(), m.GetNamespace(), m.GetReleaseName()))
}
