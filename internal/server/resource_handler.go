package server

import (
	"context"
	"encoding/json"

	"connectrpc.com/connect"

	kladosv1 "github.com/Vilsol/klados/gen/klados/v1"
	"github.com/Vilsol/klados/internal/services"
)

// ResourceHandler adapts services.ResourceService to the Connect interface.
// Pattern for all klados.v1 handlers: unwrap typed request fields, call the
// existing service method, JSON-marshal structured results into the
// response's *_json field. Errors map to connect.CodeUnknown unless a more
// specific code applies.
type ResourceHandler struct {
	svc *services.ResourceService
}

func NewResourceHandler(svc *services.ResourceService) *ResourceHandler {
	return &ResourceHandler{svc: svc}
}

func jsonResponse(v any) (*connect.Response[kladosv1.JsonResponse], error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&kladosv1.JsonResponse{ResultJson: data}), nil
}

func emptyResponse(err error) (*connect.Response[kladosv1.EmptyResponse], error) {
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return connect.NewResponse(&kladosv1.EmptyResponse{}), nil
}

func (h *ResourceHandler) ListResourcesWithVersion(ctx context.Context, req *connect.Request[kladosv1.ListResourcesWithVersionRequest]) (*connect.Response[kladosv1.ListResourcesWithVersionResponse], error) {
	m := req.Msg
	result, err := h.svc.ListResourcesWithVersion(m.GetContext(), m.GetGvr(), m.GetNamespace())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	items, err := json.Marshal(result.Items)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&kladosv1.ListResourcesWithVersionResponse{
		ItemsJson:       items,
		ResourceVersion: result.ResourceVersion,
	}), nil
}

func (h *ResourceHandler) GetResource(ctx context.Context, req *connect.Request[kladosv1.GetResourceRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	obj, err := h.svc.GetResource(m.GetContext(), m.GetGvr(), m.GetNamespace(), m.GetName())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(obj)
}

func (h *ResourceHandler) DeleteResource(ctx context.Context, req *connect.Request[kladosv1.ResourceRefRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.DeleteResource(m.GetContext(), m.GetGvr(), m.GetNamespace(), m.GetName()))
}

func (h *ResourceHandler) ForceDeleteResource(ctx context.Context, req *connect.Request[kladosv1.ResourceRefRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.ForceDeleteResource(m.GetContext(), m.GetGvr(), m.GetNamespace(), m.GetName()))
}

func (h *ResourceHandler) CreateResource(ctx context.Context, req *connect.Request[kladosv1.MutateResourceRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	var obj map[string]any
	if err := json.Unmarshal(m.GetObjectJson(), &obj); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	result, err := h.svc.CreateResource(m.GetContext(), m.GetGvr(), m.GetNamespace(), obj)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}

func (h *ResourceHandler) UpdateResource(ctx context.Context, req *connect.Request[kladosv1.MutateResourceRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	var obj map[string]any
	if err := json.Unmarshal(m.GetObjectJson(), &obj); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	result, err := h.svc.UpdateResource(m.GetContext(), m.GetGvr(), m.GetNamespace(), obj)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}

func (h *ResourceHandler) StartWatch(ctx context.Context, req *connect.Request[kladosv1.StartWatchRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.StartWatch(m.GetContext(), m.GetGvr(), m.GetNamespace(), m.GetResourceVersion()))
}

func (h *ResourceHandler) StopWatch(ctx context.Context, req *connect.Request[kladosv1.StopWatchRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	h.svc.StopWatch(m.GetContext(), m.GetGvr(), m.GetNamespace())
	return emptyResponse(nil)
}

func (h *ResourceHandler) ListAPIResources(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	result, err := h.svc.ListAPIResources(req.Msg.GetContext())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}

func (h *ResourceHandler) GetDescriptors(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetDescriptors())
}

func (h *ResourceHandler) GetEvents(ctx context.Context, req *connect.Request[kladosv1.GetEventsRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	result, err := h.svc.GetEvents(m.GetContext(), m.GetNamespace(), m.GetUid())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}

func (h *ResourceHandler) ScaleResource(ctx context.Context, req *connect.Request[kladosv1.ScaleResourceRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.ScaleResource(m.GetContext(), m.GetGvr(), m.GetNamespace(), m.GetName(), m.GetReplicas()))
}

func (h *ResourceHandler) ExpandPVC(ctx context.Context, req *connect.Request[kladosv1.ExpandPVCRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.ExpandPVC(m.GetContext(), m.GetNamespace(), m.GetName(), m.GetNewSize()))
}

func (h *ResourceHandler) RestartResource(ctx context.Context, req *connect.Request[kladosv1.ResourceRefRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.RestartResource(m.GetContext(), m.GetGvr(), m.GetNamespace(), m.GetName()))
}

func (h *ResourceHandler) PauseRollout(ctx context.Context, req *connect.Request[kladosv1.NamespacedNameRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.PauseRollout(m.GetContext(), m.GetNamespace(), m.GetName()))
}

func (h *ResourceHandler) ResumeRollout(ctx context.Context, req *connect.Request[kladosv1.NamespacedNameRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.ResumeRollout(m.GetContext(), m.GetNamespace(), m.GetName()))
}

func (h *ResourceHandler) GetRolloutHistory(ctx context.Context, req *connect.Request[kladosv1.ResourceRefRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	result, err := h.svc.GetRolloutHistory(m.GetContext(), m.GetGvr(), m.GetNamespace(), m.GetName())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}

func (h *ResourceHandler) RollbackToRevision(ctx context.Context, req *connect.Request[kladosv1.RollbackToRevisionRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.RollbackToRevision(m.GetContext(), m.GetGvr(), m.GetNamespace(), m.GetName(), m.GetRevision()))
}

func (h *ResourceHandler) DeleteJobCascade(ctx context.Context, req *connect.Request[kladosv1.NamespacedNameRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.DeleteJobCascade(m.GetContext(), m.GetNamespace(), m.GetName()))
}

func (h *ResourceHandler) DeleteJobOrphan(ctx context.Context, req *connect.Request[kladosv1.NamespacedNameRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.DeleteJobOrphan(m.GetContext(), m.GetNamespace(), m.GetName()))
}

func (h *ResourceHandler) TriggerCronJob(ctx context.Context, req *connect.Request[kladosv1.NamespacedNameRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.TriggerCronJob(m.GetContext(), m.GetNamespace(), m.GetName()))
}

func (h *ResourceHandler) SuspendCronJob(ctx context.Context, req *connect.Request[kladosv1.NamespacedNameRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.SuspendCronJob(m.GetContext(), m.GetNamespace(), m.GetName()))
}

func (h *ResourceHandler) ResumeCronJob(ctx context.Context, req *connect.Request[kladosv1.NamespacedNameRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.ResumeCronJob(m.GetContext(), m.GetNamespace(), m.GetName()))
}

func (h *ResourceHandler) GetTemplates(ctx context.Context, req *connect.Request[kladosv1.GetTemplatesRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	result, err := h.svc.GetTemplates(m.GetContext(), m.GetGvr())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}

func (h *ResourceHandler) GetAllTemplateGVRs(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.StringListResponse], error) {
	result, err := h.svc.GetAllTemplateGVRs(req.Msg.GetContext())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return connect.NewResponse(&kladosv1.StringListResponse{Values: result}), nil
}

func (h *ResourceHandler) ApplyManifest(ctx context.Context, req *connect.Request[kladosv1.ApplyManifestRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	result, err := h.svc.ApplyManifest(m.GetContext(), m.GetYamlContent())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(result)
}
