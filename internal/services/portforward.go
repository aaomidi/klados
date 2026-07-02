package services

import (
	"context"
	"fmt"

	"github.com/Vilsol/klados/internal/config"
	"github.com/Vilsol/klados/internal/portforward"
)

type PortForwardService struct {
	appService *AppService
	manager    *portforward.Manager
	ctx        context.Context
}

func NewPortForwardService(appSvc *AppService) *PortForwardService {
	return &PortForwardService{appService: appSvc}
}

func (s *PortForwardService) Startup(ctx context.Context) error {
	s.ctx = ctx
	s.manager = s.appService.PortForwardManager()
	return nil
}

func (s *PortForwardService) Shutdown() error {
	if s.manager != nil {
		s.manager.StopAll()
	}
	return nil
}

func (s *PortForwardService) StartForward(contextName, namespace string, targetKind portforward.TargetKind, targetName, targetGVR string, localPort, remotePort int) (portforward.ForwardSpec, error) {
	spec, err := s.manager.StartForward(portforward.ForwardSpec{
		ContextName: contextName,
		Namespace:   namespace,
		TargetKind:  targetKind,
		TargetName:  targetName,
		TargetGVR:   targetGVR,
		LocalPort:   localPort,
		RemotePort:  remotePort,
	})
	if err != nil {
		return portforward.ForwardSpec{}, err
	}
	_ = s.manager.SaveForward(contextName, config.SavedPortForward{
		ID:         spec.ID,
		Namespace:  namespace,
		Resource:   targetName,
		TargetKind: string(targetKind),
		TargetName: targetName,
		TargetGVR:  targetGVR,
		LocalPort:  localPort,
		RemotePort: remotePort,
		Enabled:    true,
	})
	return spec, nil
}

func (s *PortForwardService) StopForward(forwardID string) error {
	return s.manager.StopForward(forwardID)
}

// ConnectSavedForward starts a saved forward using its existing ID so it
// doesn't create a duplicate saved entry.
func (s *PortForwardService) ConnectSavedForward(ctxName, savedID string) (portforward.ForwardSpec, error) {
	var saved config.SavedPortForward
	var found bool
	s.manager.Cfg().Read(func(c *config.Config) {
		for _, f := range c.PortForwards[ctxName] {
			if f.ID == savedID {
				saved = f
				found = true
				return
			}
		}
	})
	if !found {
		return portforward.ForwardSpec{}, fmt.Errorf("saved forward %q not found", savedID)
	}
	return s.manager.StartForward(portforward.ForwardSpec{
		ID:          savedID,
		ContextName: ctxName,
		Namespace:   saved.Namespace,
		TargetKind:  portforward.TargetKind(saved.TargetKind),
		TargetName:  saved.TargetName,
		TargetGVR:   saved.TargetGVR,
		LocalPort:   saved.LocalPort,
		RemotePort:  saved.RemotePort,
	})
}

func (s *PortForwardService) ListForwards(contextName string) []portforward.ForwardSpec {
	return s.manager.ListForwards(contextName)
}

func (s *PortForwardService) SavePortForward(ctxName string, fwd config.SavedPortForward) error {
	return s.manager.SaveForward(ctxName, fwd)
}

func (s *PortForwardService) RemoveSavedPortForward(ctxName, id string) error {
	return s.manager.RemoveSavedForward(ctxName, id)
}

func (s *PortForwardService) SetPortForwardEnabled(ctxName, id string, enabled bool) error {
	return s.manager.SetForwardEnabled(ctxName, id, enabled)
}

func (s *PortForwardService) ListSavedPortForwards(ctxName string) []config.SavedPortForward {
	return s.manager.ListSavedForwards(ctxName)
}
