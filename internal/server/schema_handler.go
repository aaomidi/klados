package server

import (
	"context"

	"connectrpc.com/connect"

	kladosv1 "github.com/Vilsol/klados/gen/klados/v1"
	"github.com/Vilsol/klados/internal/services"
)

type SchemaHandler struct {
	svc *services.SchemaService
}

func NewSchemaHandler(svc *services.SchemaService) *SchemaHandler {
	return &SchemaHandler{svc: svc}
}

func (h *SchemaHandler) GetSchema(ctx context.Context, req *connect.Request[kladosv1.SchemaGetSchemaRequest]) (*connect.Response[kladosv1.JsonResponse], error) {
	m := req.Msg
	schema, err := h.svc.GetSchema(m.GetContext(), m.GetGvr(), m.GetKind())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnknown, err)
	}
	return jsonResponse(schema)
}
