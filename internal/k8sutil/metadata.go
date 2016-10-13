package k8sutil

import (
	"fmt"

	v1core "k8s.io/client-go/1.4/kubernetes/typed/core/v1"
	v1api "k8s.io/client-go/1.4/pkg/api/v1"
	"k8s.io/client-go/1.4/pkg/watch"
)

// NodeLabelCondition returns a condition function that succeeds when a node
// being watched has a label of key equal to value.
func NodeLabelCondition(key, value string) watch.ConditionFunc {
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

// SetNodeLabels sets all keys in m to their respective values in
// node's labels.
func SetNodeLabels(nc v1core.NodeInterface, node string, m map[string]string) error {
	n, err := nc.Get(node)
	if err != nil {
		return fmt.Errorf("failed to get node %q: %v", node, err)
	}

	for k, v := range m {
		n.Labels[k] = v
	}

	err = RetryOnConflict(DefaultBackoff, func() (err error) {
		n, err = nc.Update(n)
		return
	})
	if err != nil {
		// may be conflict if max retries were hit
		return fmt.Errorf("unable to set node labels on %q: %v", node, err)
	}

	return nil
}

// Unschedulable sets node's 'Unschedulable' property to sched
func Unschedulable(nc v1core.NodeInterface, node string, sched bool) error {
	n, err := nc.Get(node)
	if err != nil {
		return fmt.Errorf("failed to get node %q: %v", node, err)
	}

	n.Spec.Unschedulable = sched

	if err := RetryOnConflict(DefaultBackoff, func() (err error) {
		n, err = nc.Update(n)
		return
	}); err != nil {
		return fmt.Errorf("unable to set 'Unschedulable' property of node %q to %t: %v", node, sched, err)
	}

	return nil
}
