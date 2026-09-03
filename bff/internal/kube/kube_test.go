package kube

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func deployment(ns, name string, replicas, ready int32, annotations map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Annotations: annotations},
			},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: ready},
	}
}

func pod(ns, name, app string, ready bool, restarts int32) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: map[string]string{"app": app}},
		Spec:       corev1.PodSpec{NodeName: "node-1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Ready: ready, RestartCount: restarts},
			},
		},
	}
}

func newTestCache(t *testing.T, objects ...any) *Cache {
	t.Helper()
	runtimeObjs := make([]any, 0, len(objects))
	runtimeObjs = append(runtimeObjs, objects...)

	client := fake.NewSimpleClientset(toRuntime(runtimeObjs)...)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	c, err := NewCacheFromClient(ctx, client, 0)
	if err != nil {
		t.Fatalf("building cache: %v", err)
	}
	return c
}

func TestLookupAggregatesPods(t *testing.T) {
	c := newTestCache(t,
		deployment("demo", "demo-api", 2, 1, nil),
		pod("demo", "demo-api-1", "demo-api", true, 0),
		pod("demo", "demo-api-2", "demo-api", false, 3),
		// A pod of another service must not be counted.
		pod("demo", "demo-frontend-1", "demo-frontend", true, 9),
	)

	w, err := c.Lookup("demo", "demo-api")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if w.Desired != 2 || w.Ready != 1 {
		t.Fatalf("want 1/2 ready, got %d/%d", w.Ready, w.Desired)
	}
	if len(w.Pods) != 2 {
		t.Fatalf("want 2 pods for demo-api, got %d", len(w.Pods))
	}
	if w.Restarts != 3 {
		t.Fatalf("want 3 restarts, got %d (a neighbouring service may have leaked in)", w.Restarts)
	}
}

func TestLookupReadsSLOAnnotation(t *testing.T) {
	c := newTestCache(t,
		deployment("demo", "demo-api", 1, 1, map[string]string{SLOAnnotation: "300ms"}),
	)
	w, err := c.Lookup("demo", "demo-api")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if w.SLOP95 != 300*time.Millisecond {
		t.Fatalf("want 300ms SLO, got %v", w.SLOP95)
	}
}

func TestLookupIgnoresUnparseableSLO(t *testing.T) {
	c := newTestCache(t,
		deployment("demo", "demo-api", 1, 1, map[string]string{SLOAnnotation: "not-a-duration"}),
	)
	w, err := c.Lookup("demo", "demo-api")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if w.SLOP95 != 0 {
		t.Fatalf("a bad annotation should leave the SLO unset, got %v", w.SLOP95)
	}
}

func TestLookupUnknownServiceErrors(t *testing.T) {
	c := newTestCache(t, deployment("demo", "demo-api", 1, 1, nil))
	if _, err := c.Lookup("demo", "nope"); err == nil {
		t.Fatal("want an error for a service with no Deployment")
	}
}
