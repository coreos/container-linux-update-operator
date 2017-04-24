package k8sutil

import (
	"k8s.io/apimachinery/pkg/fields"
	v1api "k8s.io/client-go/pkg/api/v1"
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
