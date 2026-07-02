package server

import (
	"context"
	"encoding/json"
	"errors"

	"connectrpc.com/connect"

	kladosv1 "github.com/Vilsol/klados/gen/klados/v1"
)

var errOverflow = errors.New("event subscriber overflowed; resubscribe and re-list")

// EventHandler implements kladosv1connect.EventServiceHandler on top of Hub.
type EventHandler struct {
	hub *Hub
}

func NewEventHandler(hub *Hub) *EventHandler {
	return &EventHandler{hub: hub}
}

func (e *EventHandler) Publish(
	ctx context.Context,
	req *connect.Request[kladosv1.PublishRequest],
) (*connect.Response[kladosv1.PublishResponse], error) {
	ev := req.Msg.GetEvent()
	if ev == nil || ev.GetName() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("event name required"))
	}
	var payload any
	if len(ev.GetPayloadJson()) > 0 {
		if err := json.Unmarshal(ev.GetPayloadJson(), &payload); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}
	e.hub.Emit(ev.GetName(), payload)
	return connect.NewResponse(&kladosv1.PublishResponse{}), nil
}

func (e *EventHandler) Subscribe(
	ctx context.Context,
	req *connect.Request[kladosv1.SubscribeRequest],
	stream *connect.ServerStream[kladosv1.EventBatch],
) error {
	topics := req.Msg.GetTopics()
	if len(topics) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("at least one topic required"))
	}

	sub := e.hub.Subscribe(topics)

	// Flush an empty ack batch immediately: response headers are only written
	// on the first Send, and clients block on them to learn the subscription
	// is live.
	if err := stream.Send(&kladosv1.EventBatch{}); err != nil {
		e.hub.remove(sub)
		return err
	}

	err := sub.Run(ctx, func(batch []Event) error {
		out := &kladosv1.EventBatch{Events: make([]*kladosv1.Event, len(batch))}
		for i, ev := range batch {
			out.Events[i] = &kladosv1.Event{Name: ev.Name, PayloadJson: ev.PayloadJSON}
		}
		return stream.Send(out)
	})
	if errors.Is(err, errOverflow) {
		return connect.NewError(connect.CodeResourceExhausted, err)
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
