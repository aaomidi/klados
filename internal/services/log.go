package services

import (
	"context"

	"github.com/Vilsol/klados/internal/logs"
)

type LogService struct {
	appService *AppService
	streamer   *logs.Streamer
	ctx        context.Context
}

func NewLogService(appSvc *AppService) *LogService {
	return &LogService{appService: appSvc}
}

func (s *LogService) Startup(ctx context.Context) error {
	s.ctx = ctx
	s.streamer = s.appService.LogStreamer()
	return nil
}

func (s *LogService) Shutdown() error {
	if s.streamer != nil {
		s.streamer.StopAll()
	}
	return nil
}

func (s *LogService) StartLogStream(ctxName, ns, podName string, opts logs.LogOptions) (string, error) {
	return s.streamer.StartStream(ctxName, ns, podName, opts)
}

func (s *LogService) StopLogStream(streamID string) {
	s.streamer.StopStream(streamID)
}
