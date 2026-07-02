package server

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	kladosv1 "github.com/Vilsol/klados/gen/klados/v1"
)

// Desktop is implemented by the desktop shell (Wails) to expose native
// integrations the browser can't provide. Nil in server mode; the frontend
// gates on GetCapabilities and falls back to web equivalents.
type Desktop interface {
	// BrowseKubeconfigFile / BrowsePluginFile return the selected file path
	// ("" on cancel); BrowseManifestFile returns the file's content.
	BrowseKubeconfigFile() (string, error)
	BrowseManifestFile() (string, error)
	BrowsePluginFile() (string, error)
	// OpenPanelWindow opens (or focuses) a real OS window for a bottom-panel
	// tab, loading the SPA with ?panel={id}.
	OpenPanelWindow(panelID, title string) error
}

var errDesktopOnly = errors.New("only available in the desktop app")

func (h *AppHandler) BrowseKubeconfigFile(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.AppBrowseFileResponse], error) {
	if h.desktop == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errDesktopOnly)
	}
	path, err := h.desktop.BrowseKubeconfigFile()
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return connect.NewResponse(&kladosv1.AppBrowseFileResponse{Value: path}), nil
}

func (h *AppHandler) BrowseManifestFile(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.AppBrowseFileResponse], error) {
	if h.desktop == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errDesktopOnly)
	}
	content, err := h.desktop.BrowseManifestFile()
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return connect.NewResponse(&kladosv1.AppBrowseFileResponse{Value: content}), nil
}

func (h *AppHandler) BrowsePluginFile(ctx context.Context, req *connect.Request[kladosv1.EmptyRequest]) (*connect.Response[kladosv1.AppBrowseFileResponse], error) {
	if h.desktop == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errDesktopOnly)
	}
	path, err := h.desktop.BrowsePluginFile()
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return connect.NewResponse(&kladosv1.AppBrowseFileResponse{Value: path}), nil
}

// WindowHandler implements kladosv1connect.WindowServiceHandler.
type WindowHandler struct {
	desktop Desktop
}

func NewWindowHandler(desktop Desktop) *WindowHandler {
	return &WindowHandler{desktop: desktop}
}

func (h *WindowHandler) OpenPanelWindow(ctx context.Context, req *connect.Request[kladosv1.WindowOpenPanelRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	if h.desktop == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errDesktopOnly)
	}
	if err := h.desktop.OpenPanelWindow(req.Msg.GetPanelId(), req.Msg.GetTitle()); err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return connect.NewResponse(&kladosv1.EmptyResponse{}), nil
}
