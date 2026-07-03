package helm

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/release/common"
	release "helm.sh/helm/v4/pkg/release/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeReleaseSecret(t *testing.T, name, ns string, rev int, status common.Status, values map[string]any, manifest string) corev1.Secret {
	t.Helper()
	rel := &release.Release{
		Name:      name,
		Namespace: ns,
		Version:   rev,
		Manifest:  manifest,
		Chart: &chart.Chart{
			Metadata: &chart.Metadata{
				Name:       name + "-chart",
				Version:    "1.2.3",
				AppVersion: "9.9.9",
			},
			Values: map[string]any{},
		},
		Config: values,
		Info: &release.Info{
			Status:        status,
			LastDeployed:  time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(rev) * time.Hour),
			FirstDeployed: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Description:   "test",
		},
	}
	data, err := EncodeRelease(rel)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("sh.helm.release.v1.%s.v%d", name, rev),
			Namespace: ns,
			Labels: map[string]string{
				"name":    name,
				"owner":   "helm",
				"status":  string(status),
				"version": strconv.Itoa(rev),
			},
			ResourceVersion: fmt.Sprintf("rv-%s-%d", name, rev),
		},
		Type: ReleaseSecretType,
		Data: map[string][]byte{"release": data},
	}
}

// fakeSecretLister is an in-memory secretLister.
type fakeSecretLister struct {
	mu  sync.Mutex
	by  map[string][]corev1.Secret // key: contextName + "/" + namespace
	rv  string
	err error
}

func newFakeSecretLister() *fakeSecretLister {
	return &fakeSecretLister{by: map[string][]corev1.Secret{}, rv: "1"}
}

func (f *fakeSecretLister) put(contextName, namespace string, secrets []corev1.Secret) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.by[contextName+"/"+namespace] = secrets
}

func (f *fakeSecretLister) ListSecrets(_ context.Context, contextName, namespace, _, _ string) ([]corev1.Secret, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, "", f.err
	}
	all := f.by[contextName+"/"+namespace]
	if namespace == "" {
		return all, f.rv, nil
	}
	out := make([]corev1.Secret, 0, len(all))
	for _, s := range all {
		if s.Namespace == namespace {
			out = append(out, s)
		}
	}
	return out, f.rv, nil
}

// fakeResourceGetter implements resourceGetter for owned_test.go.
type fakeResourceGetter struct {
	mu        sync.Mutex
	exists    map[string]bool             // key: gvr|ns|name
	byLabel   map[string][]map[string]any // key: gvr|ns
	knownGVRs []string
}

func newFakeResourceGetter() *fakeResourceGetter {
	return &fakeResourceGetter{
		exists:  map[string]bool{},
		byLabel: map[string][]map[string]any{},
		knownGVRs: []string{
			"apps.v1.deployments",
			"core.v1.services",
			"core.v1.configmaps",
		},
	}
}

func (f *fakeResourceGetter) Exists(_ context.Context, _ string, gvr, ns, name string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.exists[gvr+"|"+ns+"|"+name], nil
}

func (f *fakeResourceGetter) ListByLabel(_ context.Context, _, gvr, ns, _ string) ([]map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byLabel[gvr+"|"+ns], nil
}

func (f *fakeResourceGetter) KnownGVRs(_ string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.knownGVRs))
	copy(out, f.knownGVRs)
	return out
}

// syntheticUIDStr mirrors the production helper for use in test assertions.
func syntheticUIDStr(ns, name string) string {
	h := sha1.Sum([]byte(ns + "/" + name))
	return hex.EncodeToString(h[:])
}
