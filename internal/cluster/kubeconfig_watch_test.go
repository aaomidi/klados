package cluster

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MarvinJWendt/testza"
)

func TestWatchKubeconfigs_DetectsCredentialChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	kubeconfig := func(server string) string {
		return `apiVersion: v1
kind: Config
clusters:
- name: c1
  cluster: {server: ` + server + `}
users:
- name: u1
  user: {}
contexts:
- name: ctx1
  context: {cluster: c1, user: u1}
`
	}
	testza.AssertNoError(t, os.WriteFile(path, []byte(kubeconfig("https://old.example:6443")), 0600))

	updated := make(chan struct{}, 4)
	m := NewManager(func(name string, _ any) {
		if name == "kubeconfigs:updated" {
			updated <- struct{}{}
		}
	}, nil, context.Background())
	testza.AssertNoError(t, m.LoadKubeconfigs([]string{path}))

	origDebounce := kubeconfigDebounce
	kubeconfigDebounce = 20 * time.Millisecond
	defer func() { kubeconfigDebounce = origDebounce }()

	changedCh := make(chan []string, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testza.AssertNoError(t, m.WatchKubeconfigs(ctx, t.TempDir(),
		func() []string { return []string{path} },
		func(changed []string) { changedCh <- changed }))

	testza.AssertNoError(t, os.WriteFile(path, []byte(kubeconfig("https://new.example:6443")), 0600))

	select {
	case changed := <-changedCh:
		testza.AssertEqual(t, []string{"ctx1"}, changed)
	case <-time.After(3 * time.Second):
		t.Fatal("kubeconfig file change was not detected")
	}
	select {
	case <-updated:
	case <-time.After(time.Second):
		t.Fatal("kubeconfigs:updated event not emitted")
	}
}
