package k8sutil

import (
	v1api "k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/fields"
)

// FilterNodesByAnnotation takes a node list and a field selector, and returns
// a node list that matches the field selector.
func FilterNodesByAnnotation(list []v1api.Node, sel fields.Selector) []v1api.Node {
	var ret []v1api.Node

	for _, n := range list {
		if sel.Matches(fields.Set(n.Annotations)) {
			ret = append(ret, n)
		}
	}

	return ret
}
