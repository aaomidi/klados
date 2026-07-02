package server

import (
	"context"
	"encoding/json"

	"connectrpc.com/connect"

	kladosv1 "github.com/Vilsol/klados/gen/klados/v1"
	"github.com/Vilsol/klados/internal/config"
	"github.com/Vilsol/klados/internal/services"
)

type ConfigHandler struct {
	svc *services.ConfigService
}

func NewConfigHandler(svc *services.ConfigService) *ConfigHandler {
	return &ConfigHandler{svc: svc}
}

func (h *ConfigHandler) GetTheme(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.ConfigStringResponse], error) {
	return connect.NewResponse(&kladosv1.ConfigStringResponse{Value: h.svc.GetTheme()}), nil
}

func (h *ConfigHandler) SetTheme(ctx context.Context, req *connect.Request[kladosv1.ConfigSetThemeRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.SetTheme(req.Msg.GetTheme()))
}

func (h *ConfigHandler) GetTerminalWebGL(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.ConfigBoolResponse], error) {
	return connect.NewResponse(&kladosv1.ConfigBoolResponse{Value: h.svc.GetTerminalWebGL()}), nil
}

func (h *ConfigHandler) SetTerminalWebGL(ctx context.Context, req *connect.Request[kladosv1.ConfigSetBoolRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.SetTerminalWebGL(req.Msg.GetEnabled()))
}

func (h *ConfigHandler) GetInsecureSkipTLSVerify(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.ConfigBoolResponse], error) {
	return connect.NewResponse(&kladosv1.ConfigBoolResponse{Value: h.svc.GetInsecureSkipTLSVerify()}), nil
}

func (h *ConfigHandler) SetInsecureSkipTLSVerify(ctx context.Context, req *connect.Request[kladosv1.ConfigSetBoolRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.SetInsecureSkipTLSVerify(req.Msg.GetEnabled()))
}

func (h *ConfigHandler) GetCompactRows(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.ConfigBoolResponse], error) {
	return connect.NewResponse(&kladosv1.ConfigBoolResponse{Value: h.svc.GetCompactRows()}), nil
}

func (h *ConfigHandler) SetCompactRows(ctx context.Context, req *connect.Request[kladosv1.ConfigSetBoolRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.SetCompactRows(req.Msg.GetEnabled()))
}

func (h *ConfigHandler) SetContextualAutocomplete(ctx context.Context, req *connect.Request[kladosv1.ConfigSetBoolRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.SetContextualAutocomplete(req.Msg.GetEnabled()))
}

func (h *ConfigHandler) GetConfig(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetConfig())
}

func (h *ConfigHandler) GetResolvedPrefs(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetResolvedPrefs(req.Msg.GetContext()))
}

func (h *ConfigHandler) GetColumnPrefs(ctx context.Context, req *connect.Request[kladosv1.ConfigGVRRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetColumnPrefs(req.Msg.GetGvr()))
}

func (h *ConfigHandler) SetColumnPrefs(ctx context.Context, req *connect.Request[kladosv1.ConfigSetColumnPrefsRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	var prefs *config.GVRColumnPrefs
	if len(req.Msg.GetPrefsJson()) > 0 {
		if err := json.Unmarshal(req.Msg.GetPrefsJson(), &prefs); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}
	return emptyResponse(h.svc.SetColumnPrefs(req.Msg.GetGvr(), prefs))
}

func (h *ConfigHandler) DeleteColumnPrefs(ctx context.Context, req *connect.Request[kladosv1.ConfigGVRRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.DeleteColumnPrefs(req.Msg.GetGvr()))
}

func (h *ConfigHandler) SetAccentColor(ctx context.Context, req *connect.Request[kladosv1.ConfigSetStringRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.SetAccentColor(req.Msg.GetValue()))
}

func (h *ConfigHandler) SetFontSize(ctx context.Context, req *connect.Request[kladosv1.ConfigSetFontSizeRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.SetFontSize(int(req.Msg.GetSize())))
}

func (h *ConfigHandler) SetStartupBehavior(ctx context.Context, req *connect.Request[kladosv1.ConfigSetStartupBehaviorRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.SetStartupBehavior(req.Msg.GetBehavior(), req.Msg.GetCluster()))
}

func (h *ConfigHandler) SetKeybinding(ctx context.Context, req *connect.Request[kladosv1.ConfigSetKeybindingRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.SetKeybinding(req.Msg.GetActionId(), req.Msg.GetKeys()))
}

func (h *ConfigHandler) ResetKeybindings(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.ResetKeybindings())
}

func (h *ConfigHandler) GetClusterPrefs(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetClusterPrefs(req.Msg.GetContext()))
}

func (h *ConfigHandler) SetClusterPrefs(ctx context.Context, req *connect.Request[kladosv1.ConfigSetClusterPrefsRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	var prefs *config.ClusterPrefs
	if len(req.Msg.GetPrefsJson()) > 0 {
		if err := json.Unmarshal(req.Msg.GetPrefsJson(), &prefs); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}
	return emptyResponse(h.svc.SetClusterPrefs(req.Msg.GetContext(), prefs))
}

func (h *ConfigHandler) DeleteClusterPrefs(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.DeleteClusterPrefs(req.Msg.GetContext()))
}

func (h *ConfigHandler) GetSavedFilters(ctx context.Context, req *connect.Request[kladosv1.ConfigGVRRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetSavedFilters(req.Msg.GetGvr()))
}

func (h *ConfigHandler) SetSavedFilters(ctx context.Context, req *connect.Request[kladosv1.ConfigSetSavedFiltersRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	var filters []config.SavedFilter
	if len(req.Msg.GetFiltersJson()) > 0 {
		if err := json.Unmarshal(req.Msg.GetFiltersJson(), &filters); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}
	return emptyResponse(h.svc.SetSavedFilters(req.Msg.GetGvr(), filters))
}

func (h *ConfigHandler) SetClusterSavedFilters(ctx context.Context, req *connect.Request[kladosv1.ConfigSetClusterSavedFiltersRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	var filters []config.SavedFilter
	if len(req.Msg.GetFiltersJson()) > 0 {
		if err := json.Unmarshal(req.Msg.GetFiltersJson(), &filters); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}
	return emptyResponse(h.svc.SetClusterSavedFilters(req.Msg.GetContext(), req.Msg.GetGvr(), filters))
}

func (h *ConfigHandler) SetVolumeBrowser(ctx context.Context, req *connect.Request[kladosv1.ConfigSetVolumeBrowserRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	var vb *config.VolumeBrowserConfig
	if len(req.Msg.GetConfigJson()) > 0 {
		if err := json.Unmarshal(req.Msg.GetConfigJson(), &vb); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}
	return emptyResponse(h.svc.SetVolumeBrowser(vb))
}
