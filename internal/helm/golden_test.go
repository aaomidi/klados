package helm

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helm.sh/helm/v4/pkg/release/common"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

// -update regenerates the testdata fixtures and exits the test as passing.
// Without the flag, this file is a no-op.
var updateFixtures = flag.Bool("update", false, "regenerate testdata fixtures")

// TestGenerateFixtures (gated on -update) produces the canonical testdata YAML
// files used elsewhere in this suite. Running without -update is a no-op so
// CI does not regenerate them. The non-fixture-generating tests in this
// package use makeReleaseSecret directly, so this file exists primarily as
// documentation of the fixture wire format.
func TestGenerateFixtures(t *testing.T) {
	if !*updateFixtures {
		t.Skip("pass -update to regenerate fixtures")
	}
	dir := "testdata"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "release-multi-revision"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "release-large-continuation"), 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		path   string
		secret corev1.Secret
	}{
		{"release-deployed.secret.yaml", makeReleaseSecret(t, "deployed-app", "default", 1, common.StatusDeployed, map[string]any{"replicas": 2}, "")},
		{"release-failed.secret.yaml", makeReleaseSecret(t, "failed-app", "default", 1, common.StatusFailed, nil, "")},
		{"release-superseded.secret.yaml", makeReleaseSecret(t, "superseded-app", "default", 1, common.StatusSuperseded, nil, "")},
		{"release-stuck-pending-upgrade.secret.yaml", makeReleaseSecret(t, "stuck-app", "default", 2, common.StatusPendingUpgrade, nil, "")},
		{"release-with-secret-values.secret.yaml", makeReleaseSecret(t, "secret-app", "default", 1, common.StatusDeployed, map[string]any{
			"password": "swordfish",
			"apiToken": "abc123",
			"replicas": 3,
		}, "")},
	}
	for _, c := range cases {
		writeSecretYAML(t, filepath.Join(dir, c.path), &c.secret)
	}

	// Multi-revision fixtures: 2 releases × 5 revisions.
	for _, name := range []string{"alpha", "beta"} {
		for rev := 1; rev <= 5; rev++ {
			status := common.StatusSuperseded
			if rev == 5 {
				status = common.StatusDeployed
			}
			s := makeReleaseSecret(t, name, "default", rev, status, nil, "")
			path := filepath.Join(dir, "release-multi-revision", fmt.Sprintf("%s.v%d.yaml", name, rev))
			writeSecretYAML(t, path, &s)
		}
	}

	// Large-continuation fixture: 3-chunk release.
	full := makeReleaseSecret(t, "chunky", "default", 1, common.StatusDeployed, nil, "")
	data := full.Data["release"]
	a := len(data) / 3
	b := 2 * len(data) / 3
	chunks := [][]byte{data[:a], data[a:b], data[b:]}
	for i, payload := range chunks {
		cs := full.DeepCopy()
		cs.Name = fmt.Sprintf("%s.%d", full.Name, i)
		cs.Data = map[string][]byte{"release": payload}
		writeSecretYAML(t, filepath.Join(dir, "release-large-continuation", fmt.Sprintf("chunk-%d.yaml", i)), cs)
	}
}

func writeSecretYAML(t *testing.T, path string, s *corev1.Secret) {
	t.Helper()
	// Ensure GVK is set so the YAML round-trips cleanly.
	s.APIVersion = "v1"
	s.Kind = "Secret"
	out, err := yaml.Marshal(s)
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	// Sanity check: don't accidentally commit garbage.
	if !strings.Contains(string(out), "helm.sh/release.v1") {
		t.Fatalf("fixture %s missing release type", path)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
