package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"

	kladosv1 "github.com/Vilsol/klados/gen/klados/v1"
	"github.com/Vilsol/klados/internal/services"
)

type PluginHandler struct {
	svc *services.PluginService
}

func NewPluginHandler(svc *services.PluginService) *PluginHandler {
	return &PluginHandler{svc: svc}
}

func (h *PluginHandler) InvokeCommand(ctx context.Context, req *connect.Request[kladosv1.PluginInvokeCommandRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.InvokeCommand(req.Msg.GetPluginName(), req.Msg.GetCommandId()))
}

func (h *PluginHandler) ReloadPluginManual(ctx context.Context, req *connect.Request[kladosv1.PluginNameRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.ReloadPluginManual(req.Msg.GetName()))
}

func (h *PluginHandler) EnablePlugin(ctx context.Context, req *connect.Request[kladosv1.PluginNameRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.EnablePlugin(req.Msg.GetName()))
}

func (h *PluginHandler) DisablePlugin(ctx context.Context, req *connect.Request[kladosv1.PluginNameRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.DisablePlugin(req.Msg.GetName()))
}

func (h *PluginHandler) UninstallPlugin(ctx context.Context, req *connect.Request[kladosv1.PluginNameRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.UninstallPlugin(req.Msg.GetName()))
}

func (h *PluginHandler) InstallPlugin(ctx context.Context, req *connect.Request[kladosv1.PluginInstallRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	if len(m.GetArchiveData()) > 0 {
		name := filepath.Base(m.GetArchiveName())
		if name == "" || name == "." || !strings.HasSuffix(name, ".oci.tar.gz") && !strings.HasSuffix(name, ".oci.tar") {
			name = "upload.oci.tar.gz"
		}
		dir, err := os.MkdirTemp("", "klados-plugin-upload-*")
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		defer os.RemoveAll(dir)
		tmp := filepath.Join(dir, name)
		if err := os.WriteFile(tmp, m.GetArchiveData(), 0o600); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		return emptyResponse(h.svc.InstallPlugin(tmp))
	}
	return emptyResponse(h.svc.InstallPlugin(m.GetPath()))
}

func (h *PluginHandler) PackPlugin(ctx context.Context, req *connect.Request[kladosv1.PluginPackRequest]) (*connect.Response[kladosv1.PluginPackResponse], error) {
	path, err := h.svc.PackPlugin(req.Msg.GetPluginDir())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return connect.NewResponse(&kladosv1.PluginPackResponse{ArchivePath: path}), nil
}

func (h *PluginHandler) SaveRegistryCredentials(ctx context.Context, req *connect.Request[kladosv1.PluginSaveRegistryCredentialsRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.SaveRegistryCredentials(m.GetHost(), m.GetUsername(), m.GetPassword()))
}

func (h *PluginHandler) AddInsecureRegistry(ctx context.Context, req *connect.Request[kladosv1.PluginHostRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.AddInsecureRegistry(req.Msg.GetHost()))
}

func (h *PluginHandler) EmitClusterEvent(ctx context.Context, req *connect.Request[kladosv1.PluginEmitClusterEventRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	h.svc.EmitClusterEvent(req.Msg.GetEventName(), req.Msg.GetPayloadJson())
	return emptyResponse(nil)
}

func (h *PluginHandler) GetPluginStorageKey(ctx context.Context, req *connect.Request[kladosv1.PluginStorageKeyRequest]) (*connect.Response[kladosv1.PluginStorageValueResponse], error) {
	val, err := h.svc.GetPluginStorageKey(req.Msg.GetPluginName(), req.Msg.GetKey())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return connect.NewResponse(&kladosv1.PluginStorageValueResponse{Value: val}), nil
}

func (h *PluginHandler) SetPluginStorageKey(ctx context.Context, req *connect.Request[kladosv1.PluginSetStorageKeyRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	return emptyResponse(h.svc.SetPluginStorageKey(m.GetPluginName(), m.GetKey(), m.GetValue()))
}

func (h *PluginHandler) DeletePluginStorageKey(ctx context.Context, req *connect.Request[kladosv1.PluginStorageKeyRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.DeletePluginStorageKey(req.Msg.GetPluginName(), req.Msg.GetKey()))
}

func (h *PluginHandler) ListPluginStorageKeys(ctx context.Context, req *connect.Request[kladosv1.PluginNameRequest]) (*connect.Response[kladosv1.StringListResponse], error) {
	keys, err := h.svc.ListPluginStorageKeys(req.Msg.GetName())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return connect.NewResponse(&kladosv1.StringListResponse{Values: keys}), nil
}

func (h *PluginHandler) ListPlugins(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.ListPlugins())
}

func (h *PluginHandler) GetPluginDescriptors(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetPluginDescriptors())
}

func (h *PluginHandler) GetPluginSidebarEntries(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetPluginSidebarEntries())
}

func (h *PluginHandler) GetPluginDetailTabs(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetPluginDetailTabs())
}

func (h *PluginHandler) GetPluginCommands(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetPluginCommands())
}

func (h *PluginHandler) GetPluginOverviewFields(ctx context.Context, req *connect.Request[kladosv1.PluginGVRRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetPluginOverviewFields(req.Msg.GetGvr()))
}

func (h *PluginHandler) GetPluginListColumns(ctx context.Context, req *connect.Request[kladosv1.PluginGVRRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetPluginListColumns(req.Msg.GetGvr()))
}

func (h *PluginHandler) GetPluginContextMenuItems(ctx context.Context, req *connect.Request[kladosv1.PluginGVRRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetPluginContextMenuItems(req.Msg.GetGvr()))
}

func (h *PluginHandler) GetPluginHeaderWidgets(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetPluginHeaderWidgets())
}

func (h *PluginHandler) GetPluginStatusBarWidgets(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetPluginStatusBarWidgets())
}

func (h *PluginHandler) GetPluginMetricQueries(ctx context.Context, req *connect.Request[kladosv1.PluginGVRRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetPluginMetricQueries(req.Msg.GetGvr()))
}

func (h *PluginHandler) GetPluginSettings(ctx context.Context, req *connect.Request[kladosv1.PluginNameRequest]) (*connect.Response[kladosv1.PluginSettingsResponse], error) {
	val, err := h.svc.GetPluginSettings(req.Msg.GetName())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return connect.NewResponse(&kladosv1.PluginSettingsResponse{Value: val}), nil
}

func (h *PluginHandler) SetPluginSettings(ctx context.Context, req *connect.Request[kladosv1.PluginSetSettingsRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.SetPluginSettings(req.Msg.GetName(), req.Msg.GetSettingsJson()))
}

func (h *PluginHandler) GetPluginSettingsSchema(ctx context.Context, req *connect.Request[kladosv1.PluginNameRequest]) (*connect.Response[kladosv1.PluginSettingsResponse], error) {
	val, err := h.svc.GetPluginSettingsSchema(req.Msg.GetName())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return connect.NewResponse(&kladosv1.PluginSettingsResponse{Value: val}), nil
}
