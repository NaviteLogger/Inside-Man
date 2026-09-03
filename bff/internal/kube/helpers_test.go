package kube

import "k8s.io/apimachinery/pkg/runtime"

// toRuntime converts test fixtures to runtime.Object for the fake clientset.
func toRuntime(objs []any) []runtime.Object {
	out := make([]runtime.Object, 0, len(objs))
	for _, o := range objs {
		if ro, ok := o.(runtime.Object); ok {
			out = append(out, ro)
		}
	}
	return out
}
