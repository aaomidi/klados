package server

import (
	"context"
	"encoding/json"

	"connectrpc.com/connect"

	kladosv1 "github.com/Vilsol/klados/gen/klados/v1"
	"github.com/Vilsol/klados/internal/services"
	"github.com/Vilsol/klados/internal/session"
)

// Capabilities describes what this deployment supports so the frontend can
// hide features that make no sense for the transport (port-forwarding to a
// remote server's loopback, native dialogs, OS windows).
type Capabilities struct {
	PortForwarding bool
	OSWindows      bool
	NativeDialogs  bool
	Mode           string // "desktop" | "server"
}

type AppHandler struct {
	svc     *services.AppService
	caps    Capabilities
	desktop Desktop
}

func NewAppHandler(svc *services.AppService, caps Capabilities, desktop Desktop) *AppHandler {
	return &AppHandler{svc: svc, caps: caps, desktop: desktop}
}

func (h *AppHandler) GetSession(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	return jsonResponse(h.svc.GetSession())
}

func (h *AppHandler) SaveUIState(ctx context.Context, req *connect.Request[kladosv1.SaveUIStateRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	var tabs []session.TabState
	if len(m.GetOpenTabsJson()) > 0 {
		if err := json.Unmarshal(m.GetOpenTabsJson(), &tabs); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}
	h.svc.SaveUIState(tabs, int(m.GetActiveTab()), m.GetSidebarCollapsed(), int(m.GetTerminalFontSize()), int(m.GetSidebarWidth()))
	return emptyResponse(nil)
}

func (h *AppHandler) LogFrontend(ctx context.Context, req *connect.Request[kladosv1.LogFrontendRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	m := req.Msg
	h.svc.LogFrontend(m.GetLevel(), m.GetMessage(), m.GetAttrsJson())
	return emptyResponse(nil)
}

func (h *AppHandler) SetReadOnly(ctx context.Context, req *connect.Request[kladosv1.SetReadOnlyRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	return emptyResponse(h.svc.SetReadOnly(ctx, req.Msg.GetEnabled()))
}

func (h *AppHandler) SetLastActiveContext(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	h.svc.SetLastActiveContext(req.Msg.GetContext())
	return emptyResponse(nil)
}

func (h *AppHandler) GetClusterHealth(ctx context.Context, req *connect.Request[kladosv1.ContextRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	health, err := h.svc.GetClusterHealth(ctx, req.Msg.GetContext())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(health)
}

func (h *AppHandler) GetCapabilities(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.GetCapabilitiesResponse], error) {
	return connect.NewResponse(&kladosv1.GetCapabilitiesResponse{
		PortForwarding: h.caps.PortForwarding,
		OsWindows:      h.caps.OSWindows,
		NativeDialogs:  h.caps.NativeDialogs,
		Mode:           h.caps.Mode,
	}), nil
}
