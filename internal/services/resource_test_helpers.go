//go:build !release

package services

import (
	"context"

	"github.com/Vilsol/klados/internal/cluster"
	"github.com/Vilsol/klados/internal/config"
	"github.com/Vilsol/klados/internal/resource"
	discoveryiface "k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// NewResourceServiceForTest creates a ResourceService for unit testing
// by injecting a fake Kubernetes clientset.
func NewResourceServiceForTest(
	clientset kubernetes.Interface,
	engine *resource.ResourceEngine,
	reg *resource.Registry,
	enricherReg *resource.EnricherRegistry,
) *ResourceService {
	mgr := cluster.NewManager(func(string, any) {}, &config.Config{}, context.Background())
	conn := &cluster.Connection{Clientset: clientset}
	mgr.SetConnectionForTest("ctx", conn)

	appSvc := &AppService{clusterMgr: mgr}
	return &ResourceService{
		appService:  appSvc,
		engine:      engine,
		registry:    reg,
		enricherReg: enricherReg,
		ctx:         context.Background(),
	}
}

// NewResourceServiceForApplyTest creates a ResourceService for testing ApplyManifest,
// with an injectable Discovery client (needed for GVR resolution).
func NewResourceServiceForApplyTest(
	clientset kubernetes.Interface,
	dynClient dynamic.Interface,
	disc discoveryiface.DiscoveryInterface,
	engine *resource.ResourceEngine,
	reg *resource.Registry,
	enricherReg *resource.EnricherRegistry,
) *ResourceService {
	mgr := cluster.NewManager(func(string, any) {}, &config.Config{}, context.Background())
	conn := &cluster.Connection{Clientset: clientset, Dynamic: dynClient, Discovery: disc}
	mgr.SetConnectionForTest("ctx", conn)

	appSvc := &AppService{clusterMgr: mgr}
	return &ResourceService{
		appService:  appSvc,
		engine:      engine,
		registry:    reg,
		enricherReg: enricherReg,
		ctx:         context.Background(),
	}
}
