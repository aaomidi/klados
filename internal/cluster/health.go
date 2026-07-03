package cluster

import (
	"context"
	"strings"
	"time"

	"github.com/Vilsol/slox"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// filteredWarnings suppresses known harmless deprecation warnings while forwarding all others.
type filteredWarnings struct{}

func (filteredWarnings) HandleWarningHeader(code int, agent string, text string) {
	if strings.Contains(text, "ComponentStatus is deprecated") {
		return
	}
	rest.WarningLogger{}.HandleWarningHeader(code, agent, text)
}

// FilteredWarningHandler drops the v1 ComponentStatus deprecation warning and
// forwards everything else to the default klog-based handler. Set this on
// rest.Config.WarningHandler when creating a clientset for any connection.
var FilteredWarningHandler rest.WarningHandler = filteredWarnings{}

type HealthStatus int

const (
	HealthOK HealthStatus = iota
	HealthDegraded
	HealthUnknown
)

type APIServerHealth struct {
	Livez   HealthStatus `json:"livez"`
	Readyz  HealthStatus `json:"readyz"`
	Healthz HealthStatus `json:"healthz"`
}

type ComponentHealth struct {
	Name    string       `json:"name"`
	Status  HealthStatus `json:"status"`
	Message string       `json:"message"`
}

type NodeSummary struct {
	Total              int  `json:"total"`
	Ready              int  `json:"ready"`
	NotReady           int  `json:"notReady"`
	SchedulingDisabled int  `json:"schedulingDisabled"`
	PermissionDenied   bool `json:"permissionDenied"`
}

type ClusterHealth struct {
	APIServer  APIServerHealth   `json:"apiServer"`
	Components []ComponentHealth `json:"components"`
	Nodes      NodeSummary       `json:"nodes"`
	CheckedAt  time.Time         `json:"checkedAt"`
}

func CheckHealth(ctx context.Context, conn *Connection) ClusterHealth {
	health := ClusterHealth{
		CheckedAt: time.Now(),
	}

	rc := conn.Clientset.Discovery().RESTClient()

	// Probe /livez, /readyz, /healthz — plain text responses, not JSON
	if rc == nil {
		health.APIServer.Livez = HealthUnknown
		health.APIServer.Readyz = HealthUnknown
		health.APIServer.Healthz = HealthUnknown
	} else {
		for _, probe := range []struct {
			path   string
			target *HealthStatus
		}{
			{"/livez", &health.APIServer.Livez},
			{"/readyz", &health.APIServer.Readyz},
			{"/healthz", &health.APIServer.Healthz},
		} {
			body, err := rc.Get().AbsPath(probe.path).DoRaw(ctx)
			if err != nil {
				slox.Warn(ctx, "health probe failed", "path", probe.path, "error", err)
				*probe.target = HealthDegraded
			} else if strings.TrimSpace(string(body)) == "ok" {
				*probe.target = HealthOK
			} else {
				*probe.target = HealthDegraded
			}
		}
	}

	// Component statuses — deprecated API; 404 or empty = not exposed
	csList, err := conn.Clientset.CoreV1().ComponentStatuses().List(ctx, metav1.ListOptions{})
	if err != nil {
		if errors.IsNotFound(err) || errors.IsMethodNotSupported(err) {
			health.Components = []ComponentHealth{}
		} else {
			slox.Warn(ctx, "componentstatuses list failed", "error", err)
			health.Components = []ComponentHealth{}
		}
	} else if len(csList.Items) == 0 {
		health.Components = []ComponentHealth{}
	} else {
		for _, cs := range csList.Items {
			ch := ComponentHealth{Name: cs.Name, Status: HealthUnknown}
			for _, cond := range cs.Conditions {
				if cond.Type == corev1.ComponentHealthy {
					if cond.Status == corev1.ConditionTrue {
						ch.Status = HealthOK
					} else {
						ch.Status = HealthDegraded
						ch.Message = cond.Message
					}
				}
			}
			health.Components = append(health.Components, ch)
		}
	}

	// Node readiness summary — 403 sets PermissionDenied, not an error state
	nodeList, err := conn.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		if errors.IsForbidden(err) {
			health.Nodes.PermissionDenied = true
		} else {
			slox.Warn(ctx, "node list failed", "error", err)
		}
	} else {
		health.Nodes.Total = len(nodeList.Items)
		for _, node := range nodeList.Items {
			if node.Spec.Unschedulable {
				health.Nodes.SchedulingDisabled++
			}
			ready := false
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
					ready = true
					break
				}
			}
			if ready {
				health.Nodes.Ready++
			} else {
				health.Nodes.NotReady++
			}
		}
	}

	return health
}
