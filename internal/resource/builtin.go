package resource

import (
	"fmt"

	"github.com/Vilsol/klados/internal/helm"
	"github.com/Vilsol/klados/internal/resource/enrichers"
)

var builtinDescriptors = []*Descriptor{
	{
		Group: "", Version: "v1", Resource: "pods", Kind: "Pod",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Ready", Expr: "status.readyDisplay", RenderType: RenderText, Width: 80},
			{Name: "Status", Expr: "status.statusDisplay", RenderType: RenderBadge, Width: 100},
			{Name: "Restarts", Expr: "status.restartCount", RenderType: RenderText, Width: 80},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Node", Expr: "spec.nodeName", RenderType: RenderText, Hidden: true},
			{Name: "IP", Expr: "status.podIP", RenderType: RenderText, Hidden: true},
			{Name: "QoS", Expr: "status.qosClass", RenderType: RenderBadge, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Node", Expr: "spec.nodeName", RenderType: RenderText},
			{Label: "Status", Expr: "status.statusDisplay", RenderType: RenderBadge},
			{Label: "Pod IP", Expr: "status.podIP", RenderType: RenderText},
			{Label: "Ready", Expr: "status.readyDisplay", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
			{Label: "Service Account", Expr: "spec.serviceAccountName", RenderType: RenderText},
			{Label: "QoS Class", Expr: "status.qosClass", RenderType: RenderBadge},
			{Label: "Priority", Expr: "spec.priority", RenderType: RenderText},
			{Label: "Restart Policy", Expr: "spec.restartPolicy", RenderType: RenderBadge},
			{Label: "DNS Policy", Expr: "spec.dnsPolicy", RenderType: RenderText},
		},
		DetailPanels: []string{"overview", "containers", "logs", "terminal", "labels", "events", "metrics", "yaml"},
		Actions: []Action{
			{Name: "delete", Label: "Delete"},
			{Name: "force-delete", Label: "Force Delete"},
		},
	},
	{
		Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Ready", Expr: "status.readyDisplay", RenderType: RenderText, Width: 80},
			{Name: "Available", Expr: "status.availableReplicas", RenderType: RenderText, Width: 90},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Up-to-date", Expr: "status.updatedReplicas", RenderType: RenderText, Hidden: true},
			{Name: "Strategy", Expr: "spec.strategy.type", RenderType: RenderBadge, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Ready", Expr: "status.readyDisplay", RenderType: RenderText},
			{Label: "Replicas", Expr: "status.replicas", RenderType: RenderText},
			{Label: "Available", Expr: "status.availableReplicas", RenderType: RenderText},
			{Label: "Strategy", Expr: "spec.strategy.type", RenderType: RenderBadge},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
			{Label: "Service Account", Expr: "spec.template.spec.serviceAccountName", RenderType: RenderText},
			{Label: "Revision", Expr: "metadata.annotations['deployment.kubernetes.io/revision']", RenderType: RenderText},
		},
		DetailPanels: []string{"overview", "aggregate-logs", "labels", "events", "metrics", "yaml"},
		Actions: []Action{
			{Name: "pause", Label: "Pause Rollout", DisabledWhen: "spec.paused == true", DisabledReason: "Rollout is already paused"},
			{Name: "resume", Label: "Resume Rollout", DisabledWhen: "spec.paused != true", DisabledReason: "Rollout is not paused"},
			{Name: "rollback", Label: "Rollback"},
			{Name: "scale", Label: "Scale"},
			{Name: "restart", Label: "Restart"},
			{Name: "delete", Label: "Delete"},
		},
	},
	{
		Group: "apps", Version: "v1", Resource: "statefulsets", Kind: "StatefulSet",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Ready", Expr: "status.readyDisplay", RenderType: RenderText, Width: 80},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Available", Expr: "status.availableReplicas", RenderType: RenderText, Hidden: true},
			{Name: "Current", Expr: "status.currentReplicas", RenderType: RenderText, Hidden: true},
			{Name: "Updated", Expr: "status.updatedReplicas", RenderType: RenderText, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Ready", Expr: "status.readyDisplay", RenderType: RenderText},
			{Label: "Replicas", Expr: "status.replicas", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
			{Label: "Update Strategy", Expr: "spec.updateStrategy.type", RenderType: RenderBadge},
			{Label: "Service Account", Expr: "spec.template.spec.serviceAccountName", RenderType: RenderText},
			{Label: "Service Name", Expr: "spec.serviceName", RenderType: RenderText},
		},
		DetailPanels: []string{"overview", "aggregate-logs", "labels", "events", "metrics", "yaml"},
		Actions: []Action{
			{Name: "rollback", Label: "Rollback"},
			{Name: "scale", Label: "Scale"},
			{Name: "restart", Label: "Restart"},
			{Name: "delete", Label: "Delete"},
		},
	},
	{
		Group: "apps", Version: "v1", Resource: "daemonsets", Kind: "DaemonSet",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Ready", Expr: "status.readyDisplay", RenderType: RenderText, Width: 80},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Desired", Expr: "status.desiredNumberScheduled", RenderType: RenderText, Hidden: true},
			{Name: "Available", Expr: "status.numberAvailable", RenderType: RenderText, Hidden: true},
			{Name: "Node Selector", Expr: "status.nodeSelectorDisplay", RenderType: RenderText, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Ready", Expr: "status.readyDisplay", RenderType: RenderText},
			{Label: "Desired", Expr: "status.desiredNumberScheduled", RenderType: RenderText},
			{Label: "Available", Expr: "status.numberAvailable", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
			{Label: "Update Strategy", Expr: "spec.updateStrategy.type", RenderType: RenderBadge},
			{Label: "Service Account", Expr: "spec.template.spec.serviceAccountName", RenderType: RenderText},
		},
		DetailPanels: []string{"overview", "aggregate-logs", "labels", "events", "metrics", "yaml"},
		Actions: []Action{
			{Name: "rollback", Label: "Rollback"},
			{Name: "restart", Label: "Restart"},
			{Name: "delete", Label: "Delete"},
		},
	},
	{
		Group: "apps", Version: "v1", Resource: "replicasets", Kind: "ReplicaSet",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Ready", Expr: "status.readyReplicas", RenderType: RenderText, Width: 80},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Replicas", Expr: "status.replicas", RenderType: RenderText, Hidden: true},
			{Name: "Owner", Expr: "status.ownerDisplay", RenderType: RenderText, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Ready", Expr: "status.readyReplicas", RenderType: RenderText},
			{Label: "Replicas", Expr: "status.replicas", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "aggregate-logs", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "batch", Version: "v1", Resource: "jobs", Kind: "Job",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Completions", Expr: "status.completionDisplay", RenderType: RenderText, Width: 100},
			{Name: "Duration", Expr: "status.durationDisplay", RenderType: RenderText, Width: 90},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Status", Expr: "status.statusDisplay", RenderType: RenderBadge, Hidden: true},
			{Name: "Backoff Limit", Expr: "spec.backoffLimit", RenderType: RenderText, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Completions", Expr: "status.completionDisplay", RenderType: RenderText},
			{Label: "Duration", Expr: "status.durationDisplay", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "labels", "events", "yaml"},
		Actions: []Action{
			{Name: "delete-cascade", Label: "Delete (Cascade)"},
			{Name: "delete-orphan", Label: "Delete (Orphan Pods)"},
			{Name: "delete", Label: "Delete"},
		},
	},
	{
		Group: "batch", Version: "v1", Resource: "cronjobs", Kind: "CronJob",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Schedule", Expr: "spec.schedule", RenderType: RenderText, Width: 120},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Suspend", Expr: "spec.suspend", RenderType: RenderBadge, Hidden: true},
			{Name: "Last Schedule", Expr: "status.lastScheduleTime", RenderType: RenderAge, Hidden: true},
			{Name: "Active", Expr: "status.activeCount", RenderType: RenderText, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Schedule", Expr: "spec.schedule", RenderType: RenderText},
			{Label: "Suspend", Expr: "spec.suspend", RenderType: RenderBadge},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "labels", "events", "yaml"},
		Actions: []Action{
			{Name: "trigger", Label: "Trigger Now"},
			{Name: "suspend", Label: "Suspend", DisabledWhen: "spec.suspend == true", DisabledReason: "CronJob is already suspended"},
			{Name: "resume", Label: "Resume", DisabledWhen: "spec.suspend != true", DisabledReason: "CronJob is not suspended"},
			{Name: "delete", Label: "Delete"},
		},
	},
	{
		Group: "", Version: "v1", Resource: "services", Kind: "Service",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Type", Expr: "spec.type", RenderType: RenderBadge, Width: 100},
			{Name: "Cluster IP", Expr: "spec.clusterIP", RenderType: RenderText, Width: 130},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "External IP", Expr: "status.externalIPDisplay", RenderType: RenderText, Hidden: true},
			{Name: "Ports", Expr: "status.portsDisplay", RenderType: RenderText, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Type", Expr: "spec.type", RenderType: RenderBadge},
			{Label: "Cluster IP", Expr: "spec.clusterIP", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "service", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "networking.k8s.io", Version: "v1", Resource: "ingresses", Kind: "Ingress",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Class", Expr: "spec.ingressClassName", RenderType: RenderText, Hidden: true},
			{Name: "Hosts", Expr: "status.hostsDisplay", RenderType: RenderText, Hidden: true},
			{Name: "Default Backend", Expr: "status.defaultBackendDisplay", RenderType: RenderText, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "ingress", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "", Version: "v1", Resource: "configmaps", Kind: "ConfigMap",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Keys", Expr: "status.dataKeysCount", RenderType: RenderText, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "configmap", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "", Version: "v1", Resource: "secrets", Kind: "Secret",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Type", Expr: "type", RenderType: RenderBadge, Width: 160},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Keys", Expr: "status.dataKeysCount", RenderType: RenderText, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "secret", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "", Version: "v1", Resource: "persistentvolumes", Kind: "PersistentVolume",
		ClusterScoped: true,
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Status", Expr: "status.phase", RenderType: RenderBadge, Width: 100},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Capacity", Expr: "spec.capacity.storage", RenderType: RenderText, Hidden: true},
			{Name: "Access Modes", Expr: "status.accessModesDisplay", RenderType: RenderText, Hidden: true},
			{Name: "Storage Class", Expr: "spec.storageClassName", RenderType: RenderText, Hidden: true},
			{Name: "Claim", Expr: "status.claimDisplay", RenderType: RenderText, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Status", Expr: "status.phase", RenderType: RenderBadge},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "", Version: "v1", Resource: "namespaces", Kind: "Namespace",
		ClusterScoped: true,
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Status", Expr: "status.phase", RenderType: RenderBadge, Width: 90},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
		},
		OverviewFields: []OverviewField{
			{Label: "Status", Expr: "status.phase", RenderType: RenderBadge},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "", Version: "v1", Resource: "nodes", Kind: "Node",
		ClusterScoped: true,
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Status", Expr: "status.readyStatus", RenderType: RenderBadge, Width: 90},
			{Name: "Drain", Expr: "status.drainPhase", RenderType: RenderBadge, Width: 90},
			{Name: "Roles", Expr: "status.roles", RenderType: RenderText, Width: 130},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Version", Expr: "status.nodeInfo.kubeletVersion", RenderType: RenderText, Hidden: true},
			{Name: "Internal IP", Expr: "status.internalIPDisplay", RenderType: RenderText, Hidden: true},
			{Name: "OS/Arch", Expr: "status.osArchDisplay", RenderType: RenderText, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Status", Expr: "status.readyStatus", RenderType: RenderBadge},
			{Label: "Roles", Expr: "status.roles", RenderType: RenderText},
			{Label: "Conditions", Expr: "status.conditionsSummary", RenderType: RenderText},
			{Label: "Taints", Expr: "status.taintsSummary", RenderType: RenderText},
			{Label: "OS Image", Expr: "status.nodeInfo.osImage", RenderType: RenderText},
			{Label: "Kernel", Expr: "status.nodeInfo.kernelVersion", RenderType: RenderText},
			{Label: "Container Runtime", Expr: "status.nodeInfo.containerRuntimeVersion", RenderType: RenderText},
			{Label: "CPU Allocatable", Expr: "status.allocatable.cpu", RenderType: RenderText},
			{Label: "Memory Allocatable", Expr: "status.allocatable.memory", RenderType: RenderText},
			{Label: "Pods Allocatable", Expr: "status.allocatable.pods", RenderType: RenderText},
			{Label: "Ephemeral Storage", Expr: "status.ephemeralStorage", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "node", "drain", "labels", "events", "metrics", "yaml"},
		Actions: []Action{
			{Name: "cordon", Label: "Cordon", DisabledWhen: "spec.unschedulable == true", DisabledReason: "Node is already cordoned"},
			{Name: "uncordon", Label: "Uncordon", DisabledWhen: "spec.unschedulable != true", DisabledReason: "Node is not cordoned"},
			{Name: "drain", Label: "Drain", DisabledWhen: "status.drainPhase == 'Draining'", DisabledReason: "Node is already draining"},
			{Name: "delete", Label: "Delete"},
		},
	},
	{
		Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions", Kind: "CustomResourceDefinition",
		ClusterScoped: true,
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Group", Expr: "spec.group", RenderType: RenderText},
			{Name: "Scope", Expr: "spec.scope", RenderType: RenderBadge, Width: 110},
			{Name: "Versions", Expr: "status.versionsDisplay", RenderType: RenderText},
			{Name: "Established", Expr: "status.established", RenderType: RenderBadge, Width: 110},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
		},
		OverviewFields: []OverviewField{
			{Label: "Group", Expr: "spec.group", RenderType: RenderText},
			{Label: "Scope", Expr: "spec.scope", RenderType: RenderBadge},
			{Label: "Plural", Expr: "spec.names.plural", RenderType: RenderText},
			{Label: "Singular", Expr: "spec.names.singular", RenderType: RenderText},
			{Label: "Kind", Expr: "spec.names.kind", RenderType: RenderText},
			{Label: "Short Names", Expr: "spec.names.shortNames.join(', ')", RenderType: RenderText},
			{Label: "Storage Version", Expr: "status.storageVersion", RenderType: RenderText},
			{Label: "Established", Expr: "status.established", RenderType: RenderBadge},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "crd", "crd-schema", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "", Version: "v1", Resource: "serviceaccounts", Kind: "ServiceAccount",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Secrets", Expr: "status.secretsCount", RenderType: RenderText, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "serviceaccount", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles", Kind: "Role",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Rules", Expr: "status.rulesCount", RenderType: RenderText, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "rules", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles", Kind: "ClusterRole",
		ClusterScoped: true,
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Rules", Expr: "status.rulesCount", RenderType: RenderText, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "rules", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings", Kind: "RoleBinding",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Role", Expr: "status.roleRefDisplay", RenderType: RenderText, Hidden: true},
			{Name: "Subjects", Expr: "status.subjectsCount", RenderType: RenderText, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "binding", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings", Kind: "ClusterRoleBinding",
		ClusterScoped: true,
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Role", Expr: "status.roleRefDisplay", RenderType: RenderText, Hidden: true},
			{Name: "Subjects", Expr: "status.subjectsCount", RenderType: RenderText, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "binding", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "", Version: "v1", Resource: "persistentvolumeclaims", Kind: "PersistentVolumeClaim",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Status", Expr: "status.phase", RenderType: RenderBadge, Width: 100},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Capacity", Expr: "status.capacity.storage", RenderType: RenderText, Hidden: true},
			{Name: "Access Modes", Expr: "status.accessModesDisplay", RenderType: RenderText, Hidden: true},
			{Name: "Storage Class", Expr: "spec.storageClassName", RenderType: RenderText, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Status", Expr: "status.phase", RenderType: RenderBadge},
			{Label: "Volume", Expr: "spec.volumeName", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "expand", Label: "Expand"}, {Name: "delete", Label: "Delete"}},
	},
	{
		Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses", Kind: "StorageClass",
		ClusterScoped: true,
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Provisioner", Expr: "provisioner", RenderType: RenderText},
			{Name: "Reclaim Policy", Expr: "reclaimPolicy", RenderType: RenderBadge, Width: 120},
			{Name: "Binding Mode", Expr: "volumeBindingMode", RenderType: RenderBadge, Width: 140},
			{Name: "Default", Expr: "status.isDefault", RenderType: RenderBadge, Width: 80},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
			{Name: "Allow Expansion", Expr: "allowVolumeExpansion", RenderType: RenderBadge, Hidden: true},
		},
		OverviewFields: []OverviewField{
			{Label: "Provisioner", Expr: "provisioner", RenderType: RenderText},
			{Label: "Reclaim Policy", Expr: "reclaimPolicy", RenderType: RenderBadge},
			{Label: "Binding Mode", Expr: "volumeBindingMode", RenderType: RenderBadge},
			{Label: "Default", Expr: "status.isDefault", RenderType: RenderBadge},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "sc-parameters", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "storage.k8s.io", Version: "v1", Resource: "csidrivers", Kind: "CSIDriver",
		ClusterScoped: true,
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Attach Required", Expr: "string(spec.attachRequired)", RenderType: RenderBadge, Width: 130},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
		},
		OverviewFields: []OverviewField{
			{Label: "Attach Required", Expr: "string(spec.attachRequired)", RenderType: RenderBadge},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "csi-capabilities", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies", Kind: "NetworkPolicy",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Pod Selector", Expr: "status.podSelectorDisplay", RenderType: RenderText},
			{Name: "Policy Types", Expr: "status.policyTypesDisplay", RenderType: RenderText},
			{Name: "Ingress Rules", Expr: "status.ingressRuleCount", RenderType: RenderText, Width: 100},
			{Name: "Egress Rules", Expr: "status.egressRuleCount", RenderType: RenderText, Width: 100},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Pod Selector", Expr: "status.podSelectorDisplay", RenderType: RenderText},
			{Label: "Policy Types", Expr: "status.policyTypesDisplay", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "netpol", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "networking.k8s.io", Version: "v1", Resource: "ingressclasses", Kind: "IngressClass",
		ClusterScoped: true,
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Controller", Expr: "spec.controller", RenderType: RenderText},
			{Name: "Default", Expr: "status.isDefault", RenderType: RenderBadge, Width: 80},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
		},
		OverviewFields: []OverviewField{
			{Label: "Controller", Expr: "spec.controller", RenderType: RenderText},
			{Label: "Default", Expr: "status.isDefault", RenderType: RenderBadge},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices", Kind: "EndpointSlice",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Address Type", Expr: "addressType", RenderType: RenderBadge, Width: 100},
			{Name: "Service", Expr: "status.serviceDisplay", RenderType: RenderText},
			{Name: "Ports", Expr: "status.portsDisplay", RenderType: RenderText},
			{Name: "Endpoints", Expr: "status.endpointCount", RenderType: RenderText, Width: 90},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Address Type", Expr: "addressType", RenderType: RenderText},
			{Label: "Service", Expr: "status.serviceDisplay", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "endpointslice", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "", Version: "v1", Resource: "resourcequotas", Kind: "ResourceQuota",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Resources", Expr: "status.resourceCount", RenderType: RenderText, Width: 90},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Resources", Expr: "status.resourceCount", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "resourcequota", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "", Version: "v1", Resource: "limitranges", Kind: "LimitRange",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Limits", Expr: "status.limitCount", RenderType: RenderText, Width: 80},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Limits", Expr: "status.limitCount", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "limitrange", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers", Kind: "HorizontalPodAutoscaler",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Reference", Expr: "status.referenceDisplay", RenderType: RenderText},
			{Name: "Targets", Expr: "status.targetsDisplay", RenderType: RenderText},
			{Name: "Min", Expr: "spec.minReplicas", RenderType: RenderText, Width: 60},
			{Name: "Max", Expr: "spec.maxReplicas", RenderType: RenderText, Width: 60},
			{Name: "Current", Expr: "status.currentReplicas", RenderType: RenderText, Width: 80},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Reference", Expr: "status.referenceDisplay", RenderType: RenderText},
			{Label: "Min Replicas", Expr: "spec.minReplicas", RenderType: RenderText},
			{Label: "Max Replicas", Expr: "spec.maxReplicas", RenderType: RenderText},
			{Label: "Current", Expr: "status.currentReplicas", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "hpa", "labels", "events", "metrics", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "policy", Version: "v1", Resource: "poddisruptionbudgets", Kind: "PodDisruptionBudget",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Pod Selector", Expr: "status.podSelectorDisplay", RenderType: RenderText},
			{Name: "Min Available", Expr: "spec.minAvailable", RenderType: RenderText, Width: 110},
			{Name: "Max Unavailable", Expr: "spec.maxUnavailable", RenderType: RenderText, Width: 120},
			{Name: "Allowed Disruptions", Expr: "status.disruptionsAllowed", RenderType: RenderText, Width: 150},
			{Name: "Current Healthy", Expr: "status.currentHealthy", RenderType: RenderText, Width: 120},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Pod Selector", Expr: "status.podSelectorDisplay", RenderType: RenderText},
			{Label: "Min Available", Expr: "spec.minAvailable", RenderType: RenderText},
			{Label: "Max Unavailable", Expr: "spec.maxUnavailable", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "pdb", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "coordination.k8s.io", Version: "v1", Resource: "leases", Kind: "Lease",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Holder", Expr: "spec.holderIdentity", RenderType: RenderText},
			{Name: "Duration", Expr: "status.leaseDurationDisplay", RenderType: RenderText, Width: 90},
			{Name: "Renew", Expr: "spec.renewTime", RenderType: RenderAge, Width: 80},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Holder", Expr: "spec.holderIdentity", RenderType: RenderText},
			{Label: "Duration", Expr: "status.leaseDurationDisplay", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "", Version: "v1", Resource: "events", Kind: "Event",
		Columns: []Column{
			{Name: "Type", Expr: "type", RenderType: RenderBadge, Width: 80},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Reason", Expr: "reason", RenderType: RenderText, Width: 140},
			{Name: "Object", Expr: "involvedObject.kind + '/' + involvedObject.name", RenderType: RenderText, Width: 200},
			{Name: "Message", Expr: "message", RenderType: RenderText},
			{Name: "Count", Expr: "count", RenderType: RenderText, Width: 60},
			{Name: "First seen", Expr: "firstTimestamp", RenderType: RenderAge, Width: 110, Hidden: true},
			{Name: "Last seen", Expr: "lastTimestamp", RenderType: RenderAge, Width: 110},
			{Name: "Source", Expr: "source.component", RenderType: RenderText, Width: 150, Hidden: true},
		},
		DetailPanels: []string{},
	},
	{
		Group: "admissionregistration.k8s.io", Version: "v1", Resource: "mutatingwebhookconfigurations", Kind: "MutatingWebhookConfiguration",
		ClusterScoped: true,
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Webhooks", Expr: "status.webhookCount", RenderType: RenderText, Width: 90},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
		},
		OverviewFields: []OverviewField{
			{Label: "Webhooks", Expr: "status.webhookCount", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "webhooks", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "admissionregistration.k8s.io", Version: "v1", Resource: "validatingwebhookconfigurations", Kind: "ValidatingWebhookConfiguration",
		ClusterScoped: true,
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Webhooks", Expr: "status.webhookCount", RenderType: RenderText, Width: 90},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
		},
		OverviewFields: []OverviewField{
			{Label: "Webhooks", Expr: "status.webhookCount", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "webhooks", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "scheduling.k8s.io", Version: "v1", Resource: "priorityclasses", Kind: "PriorityClass",
		ClusterScoped: true,
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Value", Expr: "value", RenderType: RenderText, Width: 80, Align: AlignRight},
			{Name: "Global Default", Expr: "status.globalDefaultDisplay", RenderType: RenderBadge, Width: 110},
			{Name: "Preemption", Expr: "preemptionPolicy", RenderType: RenderBadge, Width: 110},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
		},
		OverviewFields: []OverviewField{
			{Label: "Value", Expr: "value", RenderType: RenderText},
			{Label: "Global Default", Expr: "status.globalDefaultDisplay", RenderType: RenderBadge},
			{Label: "Preemption", Expr: "preemptionPolicy", RenderType: RenderBadge},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "node.k8s.io", Version: "v1", Resource: "runtimeclasses", Kind: "RuntimeClass",
		ClusterScoped: true,
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Handler", Expr: "handler", RenderType: RenderText},
			{Name: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge, Width: 80},
		},
		OverviewFields: []OverviewField{
			{Label: "Handler", Expr: "handler", RenderType: RenderText},
			{Label: "Age", Expr: "metadata.creationTimestamp", RenderType: RenderAge},
		},
		DetailPanels: []string{"overview", "labels", "events", "yaml"},
		Actions:      []Action{{Name: "delete", Label: "Delete"}},
	},
	{
		Group: "helm", Version: "v1", Resource: "releases", Kind: "HelmRelease",
		IsVirtual:  true,
		GroupLabel: "Helm",
		Columns: []Column{
			{Name: "Name", Expr: "metadata.name", RenderType: RenderText},
			{Name: "Namespace", Expr: "metadata.namespace", RenderType: RenderText, Width: 150},
			{Name: "Status", Expr: "status.statusDisplay", RenderType: RenderBadge, Width: 110},
			{Name: "Revision", Expr: "status.revisionDisplay", RenderType: RenderText, Width: 90},
			{Name: "Chart", Expr: "status.chartDisplay", RenderType: RenderText},
			{Name: "App Version", Expr: "status.appVersion", RenderType: RenderText, Width: 110},
			{Name: "Last Deployed", Expr: "status.lastDeployedDisplay", RenderType: RenderAge, Width: 110},
		},
		OverviewFields: []OverviewField{
			{Label: "Namespace", Expr: "metadata.namespace", RenderType: RenderText},
			{Label: "Status", Expr: "status.statusDisplay", RenderType: RenderBadge},
			{Label: "Revision", Expr: "status.revisionDisplay", RenderType: RenderText},
			{Label: "Chart", Expr: "status.chartDisplay", RenderType: RenderText},
			{Label: "App Version", Expr: "status.appVersion", RenderType: RenderText},
			{Label: "Last Deployed", Expr: "status.lastDeployedDisplay", RenderType: RenderAge},
		},
		DetailPanels: []string{"helm-overview", "helm-values", "helm-manifest", "helm-history", "helm-notes", "helm-resources", "helm-hooks"},
	},
}

func BuiltinDescriptors() []*Descriptor {
	return builtinDescriptors
}

func RegisterBuiltin(reg *Registry, enricherReg *EnricherRegistry, drainSvc enrichers.DrainStateProvider) error {
	for _, d := range builtinDescriptors {
		if err := reg.Register(d); err != nil {
			return fmt.Errorf("registering %s: %w", d.GVR(), err)
		}
	}

	enricherReg.Register("core.v1.pods", &enrichers.PodEnricher{})
	enricherReg.Register("apps.v1.deployments", &enrichers.DeploymentEnricher{})
	enricherReg.Register("apps.v1.statefulsets", &enrichers.StatefulSetEnricher{})
	enricherReg.Register("apps.v1.daemonsets", &enrichers.DaemonSetEnricher{})
	enricherReg.Register("apps.v1.replicasets", &enrichers.ReplicaSetEnricher{})
	enricherReg.Register("batch.v1.jobs", &enrichers.JobEnricher{})
	enricherReg.Register("batch.v1.cronjobs", &enrichers.CronJobEnricher{})
	enricherReg.Register("core.v1.services", &enrichers.ServiceEnricher{})
	enricherReg.Register("networking.k8s.io.v1.ingresses", &enrichers.IngressEnricher{})
	enricherReg.Register("core.v1.configmaps", &enrichers.ConfigMapEnricher{})
	enricherReg.Register("core.v1.secrets", &enrichers.SecretEnricher{})
	enricherReg.Register("core.v1.persistentvolumes", &enrichers.PVEnricher{})
	enricherReg.Register("core.v1.persistentvolumeclaims", &enrichers.PVCEnricher{})
	enricherReg.Register("core.v1.nodes", &enrichers.NodeEnricher{DrainService: drainSvc})
	enricherReg.Register("core.v1.serviceaccounts", &enrichers.ServiceAccountEnricher{})
	enricherReg.Register("rbac.authorization.k8s.io.v1.roles", &enrichers.RoleEnricher{})
	enricherReg.Register("rbac.authorization.k8s.io.v1.clusterroles", &enrichers.RoleEnricher{})
	enricherReg.Register("rbac.authorization.k8s.io.v1.rolebindings", &enrichers.BindingEnricher{})
	enricherReg.Register("rbac.authorization.k8s.io.v1.clusterrolebindings", &enrichers.BindingEnricher{})
	enricherReg.Register("storage.k8s.io.v1.storageclasses", &enrichers.StorageClassEnricher{})
	enricherReg.Register("apiextensions.k8s.io.v1.customresourcedefinitions", &enrichers.CRDEnricher{})
	enricherReg.Register("networking.k8s.io.v1.networkpolicies", &enrichers.NetworkPolicyEnricher{})
	enricherReg.Register("networking.k8s.io.v1.ingressclasses", &enrichers.IngressClassEnricher{})
	enricherReg.Register("discovery.k8s.io.v1.endpointslices", &enrichers.EndpointSliceEnricher{})
	enricherReg.Register("core.v1.resourcequotas", &enrichers.ResourceQuotaEnricher{})
	enricherReg.Register("core.v1.limitranges", &enrichers.LimitRangeEnricher{})
	enricherReg.Register("autoscaling.v2.horizontalpodautoscalers", &enrichers.HPAEnricher{})
	enricherReg.Register("policy.v1.poddisruptionbudgets", &enrichers.PDBEnricher{})
	enricherReg.Register("coordination.k8s.io.v1.leases", &enrichers.LeaseEnricher{})
	webhookEnricher := &enrichers.WebhookConfigEnricher{}
	enricherReg.Register("admissionregistration.k8s.io.v1.mutatingwebhookconfigurations", webhookEnricher)
	enricherReg.Register("admissionregistration.k8s.io.v1.validatingwebhookconfigurations", webhookEnricher)
	enricherReg.Register("scheduling.k8s.io.v1.priorityclasses", &enrichers.PriorityClassEnricher{})
	enricherReg.Register("helm.v1.releases", &helm.Enricher{})

	return nil
}
