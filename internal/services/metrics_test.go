package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MarvinJWendt/testza"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/Vilsol/klados/internal/cluster"
	"github.com/Vilsol/klados/internal/config"
	internalmetrics "github.com/Vilsol/klados/internal/metrics"
)

func newTestMetricsService(clientset *fake.Clientset, cap internalmetrics.MetricsCapability) *MetricsService {
	mgr := cluster.NewManager(func(string, any) {}, &config.Config{}, context.Background())
	conn := &cluster.Connection{Clientset: clientset, MetricsCapability: cap}
	mgr.SetConnectionForTest("ctx", conn)
	appSvc := &AppService{clusterMgr: mgr}
	return &MetricsService{appService: appSvc, ctx: context.Background()}
}

func TestCollectAnnotations_OOMKill(t *testing.T) {
	oomTime := time.Now().Add(-5 * time.Minute)

	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("get", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "mypod", Namespace: "default"},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "main",
						LastTerminationState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								Reason:     "OOMKilled",
								FinishedAt: metav1.NewTime(oomTime),
							},
						},
					},
				},
			},
		}
		return true, pod, nil
	})

	svc := newTestMetricsService(clientset, internalmetrics.MetricsCapability{})
	start := oomTime.Add(-10 * time.Minute)
	end := oomTime.Add(10 * time.Minute)

	annotations, err := svc.collectAnnotations(context.Background(), "ctx", internalmetrics.MetricsCapability{}, "default", "mypod", start, end)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, 1, len(annotations))
	testza.AssertEqual(t, "OOMKilled", annotations[0].Label)
	testza.AssertEqual(t, "error", annotations[0].Severity)
	testza.AssertEqual(t, oomTime.Unix(), annotations[0].Timestamp)
}

func TestCollectAnnotations_Events(t *testing.T) {
	evTime := time.Now().Add(-3 * time.Minute)

	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("get", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "mypod", Namespace: "default"}}, nil
	})
	clientset.PrependReactor("list", "events", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		list := &corev1.EventList{
			Items: []corev1.Event{
				{
					ObjectMeta:    metav1.ObjectMeta{Name: "ev1", Namespace: "default"},
					Reason:        "BackOff",
					Type:          "Warning",
					LastTimestamp: metav1.NewTime(evTime),
				},
			},
		}
		return true, list, nil
	})

	svc := newTestMetricsService(clientset, internalmetrics.MetricsCapability{})
	start := evTime.Add(-10 * time.Minute)
	end := evTime.Add(10 * time.Minute)

	annotations, err := svc.collectAnnotations(context.Background(), "ctx", internalmetrics.MetricsCapability{}, "default", "mypod", start, end)
	testza.AssertNoError(t, err)

	backoffs := []internalmetrics.Annotation{}
	for _, a := range annotations {
		if a.Label == "BackOff" {
			backoffs = append(backoffs, a)
		}
	}
	testza.AssertEqual(t, 1, len(backoffs))
	testza.AssertEqual(t, "warning", backoffs[0].Severity)
}

func TestCollectAnnotations_ThrottlingSkippedWithoutPrometheus(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("get", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "mypod", Namespace: "default"}}, nil
	})
	clientset.PrependReactor("list", "events", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, &corev1.EventList{}, nil
	})

	cap := internalmetrics.MetricsCapability{HasPrometheus: false}
	svc := newTestMetricsService(clientset, cap)
	end := time.Now()
	start := end.Add(-15 * time.Minute)

	annotations, err := svc.collectAnnotations(context.Background(), "ctx", cap, "default", "mypod", start, end)
	testza.AssertNoError(t, err)
	for _, a := range annotations {
		testza.AssertNotEqual(t, "CPU Throttled", a.Label)
	}
}

func TestCollectThresholds_PodSpecFallback(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("get", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "mypod", Namespace: "default"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "main",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
					},
				},
			},
		}
		return true, pod, nil
	})

	cap := internalmetrics.MetricsCapability{HasKSM: false}
	svc := newTestMetricsService(clientset, cap)
	end := time.Now()
	start := end.Add(-15 * time.Minute)

	lines, err := svc.collectThresholds(context.Background(), "ctx", cap, "core.v1.pods", "default", "mypod", start, end)
	testza.AssertNoError(t, err)
	testza.AssertTrue(t, len(lines) > 0)

	labels := map[string]bool{}
	for _, l := range lines {
		labels[l.Label] = true
		testza.AssertEqual(t, 2, len(l.Series))
		testza.AssertEqual(t, start.Unix(), l.Series[0].Timestamp)
		testza.AssertEqual(t, end.Unix(), l.Series[1].Timestamp)
	}
	testza.AssertTrue(t, labels["CPU Request:main"])
	testza.AssertTrue(t, labels["CPU Limit:main"])
	testza.AssertTrue(t, labels["Memory Request:main"])
	testza.AssertTrue(t, labels["Memory Limit:main"])
}

func TestCollectThresholds_KSMPath(t *testing.T) {
	now := time.Now()
	// Fake Prometheus server returning KSM threshold data as matrix
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "matrix",
				"result": []map[string]interface{}{
					{
						"metric": map[string]string{"container": "main"},
						"values": [][]interface{}{
							{float64(now.Unix()), "0.5"},
						},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	clientset := fake.NewSimpleClientset()
	cap := internalmetrics.MetricsCapability{HasPrometheus: true, PrometheusURL: srv.URL, HasKSM: true}

	mgr := cluster.NewManager(func(string, any) {}, &config.Config{}, context.Background())
	conn := &cluster.Connection{Clientset: clientset, MetricsCapability: cap, Config: nil}
	mgr.SetConnectionForTest("ctx", conn)
	appSvc := &AppService{clusterMgr: mgr}
	svc := &MetricsService{appService: appSvc, ctx: context.Background()}

	end := now
	start := end.Add(-15 * time.Minute)

	lines, err := svc.collectThresholds(context.Background(), "ctx", cap, "core.v1.pods", "default", "mypod", start, end)
	testza.AssertNoError(t, err)
	testza.AssertTrue(t, len(lines) > 0)
	testza.AssertEqual(t, "CPU Request:main", lines[0].Label)
	testza.AssertEqual(t, float64(0.5), lines[0].Series[0].Value)
}
