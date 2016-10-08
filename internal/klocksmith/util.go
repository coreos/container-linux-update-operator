package klocksmith

import (
	"fmt"

	"k8s.io/client-go/1.4/kubernetes"
	v1core "k8s.io/client-go/1.4/kubernetes/typed/core/v1"
	v1api "k8s.io/client-go/1.4/pkg/api/v1"
	"k8s.io/client-go/1.4/pkg/watch"
	"k8s.io/client-go/1.4/rest"

	"github.com/coreos-inc/klocksmith/internal/k8sutil"
)

func k8s() (*kubernetes.Clientset, error) {
	kconf, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes in-cluster config: %v", err)
	}

	k8sClient, err := kubernetes.NewForConfig(kconf)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes client: %v", err)
	}

	return k8sClient, nil
}

func nodeLabelCondition(key, value string) watch.ConditionFunc {
	return func(event watch.Event) (bool, error) {
		switch event.Type {
		case watch.Modified:
			node := event.Object.(*v1api.Node)
			if node.Labels[key] == value {
				return true, nil
			}

			return false, nil
		}

		return false, fmt.Errorf("unhandled watch case for %#v", event)
	}
}

// setNodeLabels sets all keys in m to their respective values in
// node's labels.
func setNodeLabels(nc v1core.NodeInterface, node string, m map[string]string) error {
	n, err := nc.Get(node)
	if err != nil {
		return fmt.Errorf("failed to get node %q: %v", node, err)
	}

	for k, v := range m {
		n.Labels[k] = v
	}

	err = k8sutil.RetryOnConflict(k8sutil.DefaultBackoff, func() (err error) {
		n, err = nc.Update(n)
		return
	})
	if err != nil {
		// may be conflict if max retries were hit
		return fmt.Errorf("unable to set node labels on %q: %v", node, err)
	}

	return nil
}

// unschedulable sets node's 'Unschedulable' property to sched
func unschedulable(nc v1core.NodeInterface, node string, sched bool) error {
	n, err := nc.Get(node)
	if err != nil {
		return fmt.Errorf("failed to get node %q: %v", node, err)
	}

	n.Spec.Unschedulable = sched

	if err := k8sutil.RetryOnConflict(k8sutil.DefaultBackoff, func() (err error) {
		n, err = nc.Update(n)
		return
	}); err != nil {
		return fmt.Errorf("unable to set 'Unschedulable' property of node %q to %t: %v", node, sched, err)
	}

	return nil
}
