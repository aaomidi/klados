package server

import (
	"context"

	"connectrpc.com/connect"

	kladosv1 "github.com/Vilsol/klados/gen/klados/v1"
	"github.com/Vilsol/klados/internal/logs"
	"github.com/Vilsol/klados/internal/services"
)

// LogHandler and ExecHandler allocate stream/session ids over Connect; the
// byte planes themselves ride the WebSocket routes registered in server.go.
type LogHandler struct {
	svc *services.LogService
}

func NewLogHandler(svc *services.LogService) *LogHandler {
	return &LogHandler{svc: svc}
}

func (h *LogHandler) StartLogStream(ctx context.Context, req *connect.Request[kladosv1.StartLogStreamRequest]) (*connect.Response[kladosv1.StartLogStreamResponse], error) {
	m := req.Msg
	o := m.GetOptions()
	opts := logs.LogOptions{}
	if o != nil {
		opts.Follow = o.GetFollow()
		opts.Timestamps = o.GetTimestamps()
		opts.Previous = o.GetPrevious()
		opts.Container = o.GetContainer()
		if o.TailLines != nil {
			tail := o.GetTailLines()
			opts.TailLines = &tail
		}
	}
	id, err := h.svc.StartLogStream(m.GetContext(), m.GetNamespace(), m.GetPodName(), opts)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return connect.NewResponse(&kladosv1.StartLogStreamResponse{StreamId: id}), nil
}

func (h *LogHandler) StopLogStream(ctx context.Context, req *connect.Request[kladosv1.StopLogStreamRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	h.svc.StopLogStream(req.Msg.GetStreamId())
	return emptyResponse(nil)
}

type ExecHandler struct {
	svc *services.ExecService
}

func NewExecHandler(svc *services.ExecService) *ExecHandler {
	return &ExecHandler{svc: svc}
}

func (h *ExecHandler) OpenExecSession(ctx context.Context, req *connect.Request[kladosv1.OpenExecSessionRequest]) (*connect.Response[kladosv1.OpenExecSessionResponse], error) {
	m := req.Msg
	id, err := h.svc.OpenExecSession(m.GetContext(), m.GetNamespace(), m.GetPodName(), m.GetContainer(), m.GetShell())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return connect.NewResponse(&kladosv1.OpenExecSessionResponse{SessionId: id}), nil
}

func (h *ExecHandler) CloseExecSession(ctx context.Context, req *connect.Request[kladosv1.CloseExecSessionRequest]) (*connect.Response[kladosv1.EmptyResponse], error) {
	h.svc.CloseExecSession(req.Msg.GetSessionId())
	return emptyResponse(nil)
}
