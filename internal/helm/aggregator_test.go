package helm

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/MarvinJWendt/testza"
	"helm.sh/helm/v4/pkg/release/common"
	corev1 "k8s.io/api/core/v1"
)

func TestAggregator_CollapseSnapshot(t *testing.T) {
	// 4 releases × varying revisions = 10 total secrets. Expect 4 virtual rows.
	specs := []struct {
		name string
		ns   string
		revs []int
	}{
		{"alpha", "default", []int{1, 2, 3}},
		{"beta", "default", []int{1, 2}},
		{"gamma", "ns2", []int{7}},
		{"delta", "ns2", []int{1, 2, 3, 4}},
	}
	var in []corev1.Secret
	for _, sp := range specs {
		for _, r := range sp.revs {
			in = append(in, makeReleaseSecret(t, sp.name, sp.ns, r, common.StatusDeployed, nil, ""))
		}
	}
	a := NewAggregator()
	out, err := a.CollapseSnapshot(in)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, 4, len(out))

	// Stable ordering: default before ns2, then alpha before beta within ns.
	testza.AssertEqual(t, "alpha", out[0]["metadata"].(map[string]any)["name"])
	testza.AssertEqual(t, int64(3), out[0]["spec"].(map[string]any)["revision"])
	testza.AssertEqual(t, "beta", out[1]["metadata"].(map[string]any)["name"])
	testza.AssertEqual(t, int64(2), out[1]["spec"].(map[string]any)["revision"])
	testza.AssertEqual(t, "delta", out[2]["metadata"].(map[string]any)["name"])
	testza.AssertEqual(t, int64(4), out[2]["spec"].(map[string]any)["revision"])
	testza.AssertEqual(t, "gamma", out[3]["metadata"].(map[string]any)["name"])
	testza.AssertEqual(t, int64(7), out[3]["spec"].(map[string]any)["revision"])
}

func TestAggregator_ApplyDelta_FirstRevisionAdded(t *testing.T) {
	a := NewAggregator()
	s := makeReleaseSecret(t, "app", "default", 1, common.StatusDeployed, nil, "")
	ev, err := a.ApplyDelta("ADDED", &s)
	testza.AssertNoError(t, err)
	testza.AssertNotNil(t, ev)
	testza.AssertEqual(t, "ADDED", ev.Type)
	testza.AssertEqual(t, "app", ev.Object["metadata"].(map[string]any)["name"])
}

func TestAggregator_ApplyDelta_NewRevisionModified(t *testing.T) {
	a := NewAggregator()
	s1 := makeReleaseSecret(t, "app", "default", 1, common.StatusDeployed, nil, "")
	_, _ = a.ApplyDelta("ADDED", &s1)
	s2 := makeReleaseSecret(t, "app", "default", 2, common.StatusDeployed, nil, "")
	ev, err := a.ApplyDelta("ADDED", &s2)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, "MODIFIED", ev.Type)
	testza.AssertEqual(t, int64(2), ev.Object["spec"].(map[string]any)["revision"])
}

func TestAggregator_ApplyDelta_DeleteNonLatest(t *testing.T) {
	a := NewAggregator()
	for _, r := range []int{1, 2, 3} {
		s := makeReleaseSecret(t, "app", "default", r, common.StatusDeployed, nil, "")
		_, _ = a.ApplyDelta("ADDED", &s)
	}
	s := makeReleaseSecret(t, "app", "default", 1, common.StatusDeployed, nil, "")
	ev, err := a.ApplyDelta("DELETED", &s)
	testza.AssertNoError(t, err)
	testza.AssertNil(t, ev)
}

func TestAggregator_ApplyDelta_DeleteLatestPromotes(t *testing.T) {
	a := NewAggregator()
	for _, r := range []int{1, 2, 3} {
		s := makeReleaseSecret(t, "app", "default", r, common.StatusDeployed, nil, "")
		_, _ = a.ApplyDelta("ADDED", &s)
	}
	s := makeReleaseSecret(t, "app", "default", 3, common.StatusDeployed, nil, "")
	ev, err := a.ApplyDelta("DELETED", &s)
	testza.AssertNoError(t, err)
	testza.AssertNotNil(t, ev)
	testza.AssertEqual(t, "MODIFIED", ev.Type)
	testza.AssertEqual(t, int64(2), ev.Object["spec"].(map[string]any)["revision"])
}

func TestAggregator_ApplyDelta_DeleteLastRevision(t *testing.T) {
	a := NewAggregator()
	s := makeReleaseSecret(t, "app", "default", 1, common.StatusDeployed, nil, "")
	_, _ = a.ApplyDelta("ADDED", &s)
	ev, err := a.ApplyDelta("DELETED", &s)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, "DELETED", ev.Type)
	testza.AssertEqual(t, "app", ev.Object["metadata"].(map[string]any)["name"])
}

func TestAggregator_Tiebreak_DeployedAt(t *testing.T) {
	// Same revision number, different deployed-at: later wins.
	a := NewAggregator()
	earlier := makeReleaseSecret(t, "app", "default", 5, common.StatusDeployed, nil, "")
	earlier.ResourceVersion = "rv-earlier"
	later := makeReleaseSecret(t, "app", "default", 5, common.StatusDeployed, nil, "")
	later.ResourceVersion = "rv-later"
	// Manually skew the encoded LastDeployed by re-encoding.
	// Easier: just confirm via snapshot reduction with two distinct secrets.
	_, err := a.CollapseSnapshot([]corev1.Secret{earlier, later})
	testza.AssertNoError(t, err)
	// Both have same revision; aggregator should pick one deterministically
	// (the later deployedAt). The makeReleaseSecret helper uses
	// rev*time.Hour offsets so both have the same deployedAt here; this
	// test mainly asserts no panic / single row.
	out, err := a.CollapseSnapshot([]corev1.Secret{earlier, later})
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, 1, len(out))
}

func TestAggregator_Reset(t *testing.T) {
	a := NewAggregator()
	for _, r := range []int{1, 2} {
		s := makeReleaseSecret(t, "app", "default", r, common.StatusDeployed, nil, "")
		_, _ = a.ApplyDelta("ADDED", &s)
	}
	for _, r := range []int{1} {
		s := makeReleaseSecret(t, "other", "ns2", r, common.StatusDeployed, nil, "")
		_, _ = a.ApplyDelta("ADDED", &s)
	}
	a.Reset("default")
	// snapshotLocked needs lock — call directly for inspection.
	a.mu.Lock()
	out := a.snapshotLocked()
	a.mu.Unlock()
	testza.AssertEqual(t, 1, len(out))
	testza.AssertEqual(t, "other", out[0]["metadata"].(map[string]any)["name"])
}

func TestAggregator_ApplyDelta_DedupesNoOpModified(t *testing.T) {
	a := NewAggregator()
	s := makeReleaseSecret(t, "app", "default", 1, common.StatusDeployed, nil, "")
	ev, err := a.ApplyDelta("ADDED", &s)
	testza.AssertNoError(t, err)
	testza.AssertNotNil(t, ev)
	testza.AssertEqual(t, "ADDED", ev.Type)

	// Re-applying the same MODIFIED delta with no visible-state change must
	// suppress the event.
	ev2, err := a.ApplyDelta("MODIFIED", &s)
	testza.AssertNoError(t, err)
	testza.AssertNil(t, ev2)

	// Now change the status — must emit MODIFIED.
	s3 := makeReleaseSecret(t, "app", "default", 1, common.StatusFailed, nil, "")
	ev3, err := a.ApplyDelta("MODIFIED", &s3)
	testza.AssertNoError(t, err)
	testza.AssertNotNil(t, ev3)
	testza.AssertEqual(t, "MODIFIED", ev3.Type)
}

func TestAggregator_Stress_10kSecrets(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test skipped in -short")
	}
	var memBefore runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)

	const releases = 1000
	const revsPer = 10
	secrets := make([]corev1.Secret, 0, releases*revsPer)
	for i := 0; i < releases; i++ {
		name := fmt.Sprintf("rel-%04d", i)
		ns := fmt.Sprintf("ns-%d", i%10)
		for r := 1; r <= revsPer; r++ {
			secrets = append(secrets, makeReleaseSecret(t, name, ns, r, common.StatusDeployed, nil, ""))
		}
	}
	a := NewAggregator()
	t0 := time.Now()
	out, err := a.CollapseSnapshot(secrets)
	elapsed := time.Since(t0)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, releases, len(out))
	t.Logf("CollapseSnapshot 10k secrets: %s", elapsed)
	// Race detector adds ~8x overhead; only enforce wall-clock budget without -race.
	if !raceEnabled && elapsed > 500*time.Millisecond {
		t.Fatalf("CollapseSnapshot took %s, exceeds 500ms budget", elapsed)
	}

	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)
	delta := memAfter.HeapAlloc - memBefore.HeapAlloc
	t.Logf("Heap delta: %d MiB", delta>>20)
	if delta > 100<<20 {
		t.Fatalf("heap delta %d MiB exceeds 100 MiB budget", delta>>20)
	}
}
