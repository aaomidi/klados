package helm

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/MarvinJWendt/testza"
)

// fakeRunner records calls and lets us inject a delay so concurrency tests
// can deterministically observe lock contention.
type fakeRunner struct {
	mu     sync.Mutex
	calls  []string
	delay  time.Duration
	rbErr  error
	unErr  error
	tstRes TestResult
	tstErr error
}

func (f *fakeRunner) Rollback(_ context.Context, ctxName, ns, rel string, version int, opts RollbackOpts) error {
	f.mu.Lock()
	f.calls = append(f.calls, "rollback")
	f.mu.Unlock()
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	_ = ctxName
	_ = ns
	_ = rel
	_ = version
	_ = opts
	return f.rbErr
}

func (f *fakeRunner) Uninstall(_ context.Context, _, _, _ string, opts UninstallOpts) error {
	f.mu.Lock()
	f.calls = append(f.calls, "uninstall")
	f.mu.Unlock()
	_ = opts
	return f.unErr
}

func (f *fakeRunner) Test(_ context.Context, _, _, _ string, opts TestOpts) (TestResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, "test")
	f.mu.Unlock()
	_ = opts
	return f.tstRes, f.tstErr
}

type fakeDeleter struct {
	mu        sync.Mutex
	deleted   []string
	deleteErr error
}

func (f *fakeDeleter) DeleteSecret(_ context.Context, contextName, ns, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, contextName+"/"+ns+"/"+name)
	return f.deleteErr
}

func TestActions_Rollback_Dispatches(t *testing.T) {
	r := &fakeRunner{}
	a := newActionsWithRunner(r, nil)
	err := a.Rollback(context.Background(), "ctx1", "default", "myrel", 3, RollbackOpts{Wait: true, Timeout: 5 * time.Minute, DisableHooks: true})
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, []string{"rollback"}, r.calls)
}

func TestActions_Uninstall_Dispatches(t *testing.T) {
	r := &fakeRunner{}
	a := newActionsWithRunner(r, nil)
	err := a.Uninstall(context.Background(), "ctx1", "default", "myrel", UninstallOpts{KeepHistory: true})
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, []string{"uninstall"}, r.calls)
}

func TestActions_Test_Dispatches(t *testing.T) {
	r := &fakeRunner{tstRes: TestResult{Phase: "Success", Logs: "ok"}}
	a := newActionsWithRunner(r, nil)
	res, err := a.Test(context.Background(), "ctx1", "default", "myrel", TestOpts{Timeout: time.Minute})
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, "Success", res.Phase)
	testza.AssertEqual(t, "ok", res.Logs)
}

func TestActions_ConcurrentRollback_ReturnsInProgress(t *testing.T) {
	r := &fakeRunner{delay: 100 * time.Millisecond}
	a := newActionsWithRunner(r, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = a.Rollback(context.Background(), "ctx1", "default", "myrel", 1, RollbackOpts{})
	}()
	// Give the first goroutine time to acquire the lock.
	time.Sleep(20 * time.Millisecond)
	err := a.Rollback(context.Background(), "ctx1", "default", "myrel", 1, RollbackOpts{})
	testza.AssertTrue(t, errors.Is(err, ErrOperationInProgress))
	wg.Wait()
}

func TestActions_DifferentReleases_DoNotContend(t *testing.T) {
	r := &fakeRunner{delay: 50 * time.Millisecond}
	a := newActionsWithRunner(r, nil)
	var wg sync.WaitGroup
	wg.Add(2)
	t0 := time.Now()
	go func() {
		defer wg.Done()
		_ = a.Rollback(context.Background(), "ctx1", "default", "rel-a", 1, RollbackOpts{})
	}()
	go func() {
		defer wg.Done()
		_ = a.Rollback(context.Background(), "ctx1", "default", "rel-b", 1, RollbackOpts{})
	}()
	wg.Wait()
	if elapsed := time.Since(t0); elapsed > 90*time.Millisecond {
		t.Fatalf("expected near-parallel execution, took %s", elapsed)
	}
}

func TestActions_ForceDeleteReleaseSecret(t *testing.T) {
	d := &fakeDeleter{}
	a := newActionsWithRunner(&fakeRunner{}, d)
	err := a.ForceDeleteReleaseSecret(context.Background(), "ctx1", "default", "myrel", 4)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, []string{"ctx1/default/sh.helm.release.v1.myrel.v4"}, d.deleted)
}

func TestActions_ForceDeleteReleaseSecret_NoDeleter(t *testing.T) {
	a := newActionsWithRunner(&fakeRunner{}, nil)
	err := a.ForceDeleteReleaseSecret(context.Background(), "ctx1", "default", "myrel", 1)
	testza.AssertNotNil(t, err)
}
