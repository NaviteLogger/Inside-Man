// Package kube keeps a warm cache of the workloads behind each service.
//
// The services list asks for every service plus its pods on every poll, so
// informers answer from memory and the API server sees one watch, where a
// naive client would make N list calls per request. This is the reason the BFF is in Go, see
// docs/decisions/0001-bff-in-go.md.
package kube

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// SLOAnnotation lets a team override the p95 threshold for one service.
const SLOAnnotation = "insideman.io/slo-p95"

type Cache struct {
	factory     informers.SharedInformerFactory
	deployments appslisters.DeploymentLister
	pods        corelisters.PodLister
}

// Workload is the Kubernetes half of a service, as the UI needs it.
type Workload struct {
	Namespace string        `json:"namespace"`
	Kind      string        `json:"kind"`
	Name      string        `json:"name"`
	Desired   int32         `json:"desired"`
	Ready     int32         `json:"ready"`
	Restarts  int32         `json:"restarts"`
	SLOP95    time.Duration `json:"-"`
	Pods      []Pod         `json:"pods,omitempty"`
}

type Pod struct {
	Name     string `json:"name"`
	Phase    string `json:"phase"`
	Ready    bool   `json:"ready"`
	Restarts int32  `json:"restarts"`
	Node     string `json:"node,omitempty"`
}

// NewCache builds a client from in-cluster config, falling back to kubeconfig
// so the BFF can be run locally against a dev cluster.
func NewCache(ctx context.Context, resync time.Duration) (*Cache, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(), nil).ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("kubernetes config: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kubernetes client: %w", err)
	}
	return NewCacheFromClient(ctx, clientset, resync)
}

// NewCacheFromClient is separated out so tests can pass a fake clientset.
func NewCacheFromClient(ctx context.Context, client kubernetes.Interface, resync time.Duration) (*Cache, error) {
	factory := informers.NewSharedInformerFactory(client, resync)
	c := &Cache{
		factory:     factory,
		deployments: factory.Apps().V1().Deployments().Lister(),
		pods:        factory.Core().V1().Pods().Lister(),
	}

	factory.Start(ctx.Done())
	for typ, ok := range factory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return nil, fmt.Errorf("informer cache for %v did not sync", typ)
		}
	}
	return c, nil
}

// Lookup finds the workload behind a service. Identity is the Deployment name,
// see docs/decisions/0004-service-name-from-workload.md, so the service name is
// the Deployment name and no mapping table is involved.
func (c *Cache) Lookup(namespace, service string) (*Workload, error) {
	dep, err := c.deployments.Deployments(namespace).Get(service)
	if err != nil {
		return nil, err
	}

	sel, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("selector for %s/%s: %w", namespace, service, err)
	}

	pods, err := c.pods.Pods(namespace).List(sel)
	if err != nil {
		return nil, fmt.Errorf("pods for %s/%s: %w", namespace, service, err)
	}
	return workloadOf(dep, pods), nil
}

// Deployments lists every cached Deployment, which the services list uses to
// surface workloads that exist but have produced no spans yet.
func (c *Cache) Deployments(namespace string) ([]*appsv1.Deployment, error) {
	if namespace == "" {
		return c.deployments.List(labels.Everything())
	}
	return c.deployments.Deployments(namespace).List(labels.Everything())
}

func workloadOf(dep *appsv1.Deployment, pods []*corev1.Pod) *Workload {
	w := &Workload{
		Namespace: dep.Namespace,
		Kind:      "Deployment",
		Name:      dep.Name,
		Ready:     dep.Status.ReadyReplicas,
	}
	if dep.Spec.Replicas != nil {
		w.Desired = *dep.Spec.Replicas
	}
	if v, ok := dep.Spec.Template.Annotations[SLOAnnotation]; ok {
		if d, err := time.ParseDuration(v); err == nil {
			w.SLOP95 = d
		}
	}

	for _, p := range pods {
		pod := Pod{Name: p.Name, Phase: string(p.Status.Phase), Node: p.Spec.NodeName, Ready: true}
		if len(p.Status.ContainerStatuses) == 0 {
			pod.Ready = false
		}
		for _, cs := range p.Status.ContainerStatuses {
			pod.Restarts += cs.RestartCount
			if !cs.Ready {
				pod.Ready = false
			}
		}
		w.Restarts += pod.Restarts
		w.Pods = append(w.Pods, pod)
	}
	return w
}
