package services

import (
	"context"

	"github.com/Vilsol/klados/internal/exec"
)

type ExecService struct {
	appService *AppService
	manager    *exec.Manager
	ctx        context.Context
}

func NewExecService(appSvc *AppService) *ExecService {
	return &ExecService{appService: appSvc}
}

func (s *ExecService) Startup(ctx context.Context) error {
	s.ctx = ctx
	s.manager = s.appService.ExecManager()
	return nil
}

func (s *ExecService) Shutdown() error {
	if s.manager != nil {
		s.manager.StopAll()
	}
	return nil
}

func (s *ExecService) OpenExecSession(ctxName, ns, podName, container, shell string) (string, error) {
	return s.manager.OpenSession(ctxName, ns, podName, container, shell)
}

func (s *ExecService) CloseExecSession(sessionID string) {
	s.manager.CloseSession(sessionID)
}
