package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Vilsol/slox"

	"github.com/Vilsol/klados/internal/config"
	"github.com/Vilsol/klados/internal/volumebrowser"
)

const volumeBrowserShutdownTimeout = 5 * time.Second

// volumeBrowserManager is the subset of *volumebrowser.Manager the service needs.
// Extracted for testability.
type volumeBrowserManager interface {
	Spawn(ctx context.Context, req volumebrowser.SpawnRequest, resolved config.VolumeBrowserConfig) (*volumebrowser.ManagedPod, error)
	Stop(ctx context.Context, id string) error
	StopAll(ctx context.Context) error
	ListManaged(contextName string) []*volumebrowser.ManagedPod
	AttachTab(id, tabID string) error
	ScanOrphans(ctx context.Context, contextName string) ([]volumebrowser.OrphanPod, error)
	CleanupOrphans(ctx context.Context, contextName string) error
	FindByPVC(ctxName, namespace, pvc string) (*volumebrowser.ManagedPod, bool)
}

// configResolver is satisfied by *config.Config; abstracted for tests.
type configResolver interface {
	ResolveForCluster(ctxName string) config.ResolvedPrefs
}

type VolumeBrowserService struct {
	appService *AppService
	manager    volumeBrowserManager
	cfg        configResolver
	emitEvent  func(name string, data any)
	ctx        context.Context
}

func NewVolumeBrowserService(appSvc *AppService) *VolumeBrowserService {
	return &VolumeBrowserService{appService: appSvc}
}

// newVolumeBrowserServiceForTest wires an explicit manager + cfg for tests.
func newVolumeBrowserServiceForTest(mgr volumeBrowserManager, cfg configResolver) *VolumeBrowserService {
	return &VolumeBrowserService{manager: mgr, cfg: cfg, ctx: context.Background()}
}

func (s *VolumeBrowserService) Startup(ctx context.Context) error {
	s.ctx = ctx
	s.manager = s.appService.VolumeBrowserManager()
	s.cfg = s.appService.Config()
	s.emitEvent = s.appService.Emit
	return nil
}

func (s *VolumeBrowserService) Shutdown() error {
	if s.manager == nil {
		return nil
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), volumeBrowserShutdownTimeout)
	defer cancel()
	_ = s.manager.StopAll(stopCtx)
	return nil
}

// ---------------------------------------------------------------------------
// DTOs — pure Go shapes (no corev1) so Wails binding gen works cleanly.
// ---------------------------------------------------------------------------

type SpawnRequestDTO struct {
	ContextName string             `json:"contextName"`
	Namespace   string             `json:"namespace"`
	PVCName     string             `json:"pvcName"`
	Overrides   *SpawnOverridesDTO `json:"overrides,omitempty"`
}

type SpawnOverridesDTO struct {
	Image                 *string              `json:"image,omitempty"`
	MountPath             *string              `json:"mountPath,omitempty"`
	ReadOnly              *bool                `json:"readOnly,omitempty"`
	ActiveDeadlineSeconds *int64               `json:"activeDeadlineSeconds,omitempty"`
	Resources             *config.ResourceReqs `json:"resources,omitempty"`
	NodeSelector          map[string]string    `json:"nodeSelector,omitempty"`
	Tolerations           []map[string]any     `json:"tolerations,omitempty"`
}

type SpawnResult struct {
	ID        string `json:"id"`
	Namespace string `json:"namespace"`
	PodName   string `json:"podName"`
}

// CollisionError is returned when a managed pod for the same (ctx, ns, pvc)
// tuple already exists. It is an error so the Wails layer can surface it as
// an error to the frontend while still exposing the structured fields.
type CollisionError struct {
	ExistingPodName string `json:"existingPodName"`
	ExistingID      string `json:"existingId"`
}

func (e *CollisionError) Error() string {
	return fmt.Sprintf("volume browser already attached: pod %q (id %s)", e.ExistingPodName, e.ExistingID)
}

type ManagedPodDTO struct {
	ID            string    `json:"id"`
	ContextName   string    `json:"contextName"`
	Namespace     string    `json:"namespace"`
	PodName       string    `json:"podName"`
	PVCName       string    `json:"pvcName"`
	CreatedAt     time.Time `json:"createdAt"`
	SessionUUID   string    `json:"sessionUuid"`
	TerminalTabID string    `json:"terminalTabId"`
}

type OrphanPodDTO struct {
	ContextName string    `json:"contextName"`
	Namespace   string    `json:"namespace"`
	PodName     string    `json:"podName"`
	PVCName     string    `json:"pvcName"`
	CreatedAt   time.Time `json:"createdAt"`
	SessionUUID string    `json:"sessionUuid"`
}

func managedToDTO(p *volumebrowser.ManagedPod) ManagedPodDTO {
	return ManagedPodDTO{
		ID:            p.ID,
		ContextName:   p.ContextName,
		Namespace:     p.Namespace,
		PodName:       p.PodName,
		PVCName:       p.PVCName,
		CreatedAt:     p.CreatedAt,
		SessionUUID:   p.SessionUUID,
		TerminalTabID: p.TerminalTabID,
	}
}

func orphanToDTO(p volumebrowser.OrphanPod) OrphanPodDTO {
	return OrphanPodDTO{
		ContextName: p.ContextName,
		Namespace:   p.Namespace,
		PodName:     p.PodName,
		PVCName:     p.PVCName,
		CreatedAt:   p.CreatedAt,
		SessionUUID: p.SessionUUID,
	}
}

func dtoToOverrides(dto *SpawnOverridesDTO) *volumebrowser.SpawnOverrides {
	if dto == nil {
		return nil
	}
	return &volumebrowser.SpawnOverrides{
		Image:                 dto.Image,
		MountPath:             dto.MountPath,
		ReadOnly:              dto.ReadOnly,
		ActiveDeadlineSeconds: dto.ActiveDeadlineSeconds,
		Resources:             dto.Resources,
		NodeSelector:          dto.NodeSelector,
		Tolerations:           dto.Tolerations,
	}
}

// ---------------------------------------------------------------------------
// RPC surface
// ---------------------------------------------------------------------------

func (s *VolumeBrowserService) Spawn(req SpawnRequestDTO) (SpawnResult, error) {
	resolved := s.cfg.ResolveForCluster(req.ContextName)

	innerReq := volumebrowser.SpawnRequest{
		ContextName: req.ContextName,
		Namespace:   req.Namespace,
		PVCName:     req.PVCName,
		Overrides:   dtoToOverrides(req.Overrides),
	}

	pod, err := s.manager.Spawn(s.ctx, innerReq, resolved.VolumeBrowser)
	if err != nil {
		if errors.Is(err, volumebrowser.ErrCollision) {
			if existing, ok := s.manager.FindByPVC(req.ContextName, req.Namespace, req.PVCName); ok {
				return SpawnResult{}, &CollisionError{
					ExistingPodName: existing.PodName,
					ExistingID:      existing.ID,
				}
			}
			return SpawnResult{}, err // shouldn't happen: collision reported but FindByPVC missed it
		}
		if errors.Is(err, volumebrowser.ErrPVCNotBound) {
			return SpawnResult{}, err
		}
		return SpawnResult{}, err
	}

	return SpawnResult{
		ID:        pod.ID,
		Namespace: pod.Namespace,
		PodName:   pod.PodName,
	}, nil
}

func (s *VolumeBrowserService) Stop(id string) error {
	return s.manager.Stop(s.ctx, id)
}

// Replace stops the pod referenced by id (if any) and spawns a new one.
// Used when the user changes overrides for an already-attached browser pod.
func (s *VolumeBrowserService) Replace(id string, req SpawnRequestDTO) (SpawnResult, error) {
	if err := s.manager.Stop(s.ctx, id); err != nil {
		return SpawnResult{}, fmt.Errorf("stopping existing pod: %w", err)
	}
	return s.Spawn(req)
}

func (s *VolumeBrowserService) AttachTab(id, tabID string) error {
	return s.manager.AttachTab(id, tabID)
}

func (s *VolumeBrowserService) ListManaged(contextName string) []ManagedPodDTO {
	pods := s.manager.ListManaged(contextName)
	out := make([]ManagedPodDTO, 0, len(pods))
	for _, p := range pods {
		out = append(out, managedToDTO(p))
	}
	return out
}

func (s *VolumeBrowserService) ScanOrphans(contextName string) ([]OrphanPodDTO, error) {
	orphans, err := s.manager.ScanOrphans(s.ctx, contextName)
	if err != nil {
		return nil, err
	}
	out := make([]OrphanPodDTO, 0, len(orphans))
	for _, o := range orphans {
		out = append(out, orphanToDTO(o))
	}
	return out, nil
}

// CleanupOrphans deletes all pvc-browser pods in the given context that are
// not owned by the current session.
func (s *VolumeBrowserService) CleanupOrphans(contextName string) error {
	return s.manager.CleanupOrphans(s.ctx, contextName)
}

// OnClusterConnected is an internal hook invoked by ClusterService after a
// successful Connect. It runs an orphan scan asynchronously and dispatches
// per the cluster's OrphanCleanupOnStartup setting.
//
//wails:ignore
func (s *VolumeBrowserService) OnClusterConnected(contextName string) {
	if s.manager == nil || s.cfg == nil {
		return
	}
	go func() {
		dtos, _, err := s.runOrphanScan(contextName)
		if err != nil {
			return
		}
		if len(dtos) > 0 && s.emitEvent != nil {
			s.emitEvent(fmt.Sprintf("volumebrowser:orphans:%s", contextName), dtos)
		}
	}()
}

// TriggerOrphanScan runs the scan and applies the OrphanCleanupOnStartup
// dispatch (auto/prompt/ignore) synchronously, returning the orphan list for
// prompt mode. For ignore or after-auto-cleanup it returns an empty slice.
// Used by the frontend on-mount pull path to avoid races against the
// OnClusterConnected event.
func (s *VolumeBrowserService) TriggerOrphanScan(contextName string) ([]OrphanPodDTO, error) {
	if s.manager == nil || s.cfg == nil {
		return []OrphanPodDTO{}, nil
	}
	dtos, _, err := s.runOrphanScan(contextName)
	if err != nil {
		return nil, err
	}
	if dtos == nil {
		return []OrphanPodDTO{}, nil
	}
	return dtos, nil
}

// runOrphanScan scans for orphan pods and applies the configured cleanup mode.
// Returns (dtos, cleanupApplied, err). For "prompt" mode dtos contains the
// orphan list. For "auto" mode dtos is empty and cleanupApplied is true. For
// "ignore" mode the scan is skipped entirely and dtos is empty.
func (s *VolumeBrowserService) runOrphanScan(contextName string) ([]OrphanPodDTO, bool, error) {
	mode := s.cfg.ResolveForCluster(contextName).VolumeBrowser.OrphanCleanupOnStartup
	if mode == "" {
		mode = "prompt"
	}
	if mode == "ignore" {
		return nil, false, nil
	}

	orphans, err := s.manager.ScanOrphans(s.ctx, contextName)
	if err != nil {
		slox.Warn(s.ctx, "volumebrowser: orphan scan failed", "context", contextName, "error", err)
		return nil, false, err
	}
	if len(orphans) == 0 {
		return nil, false, nil
	}

	switch mode {
	case "auto":
		if err := s.manager.CleanupOrphans(s.ctx, contextName); err != nil {
			slox.Warn(s.ctx, "volumebrowser: auto-cleanup failed", "context", contextName, "error", err)
			return nil, false, err
		}
		slox.Info(s.ctx, "volumebrowser: auto-cleaned orphan pods", "context", contextName, "count", len(orphans))
		return nil, true, nil
	case "prompt":
		dtos := make([]OrphanPodDTO, 0, len(orphans))
		for _, o := range orphans {
			dtos = append(dtos, orphanToDTO(o))
		}
		return dtos, false, nil
	}
	return nil, false, nil
}
