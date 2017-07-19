package k8sutil

import (
	"strings"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
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

// FilterNodesByRequirement filters a list of nodes and returns nodes matching the
// given label requirement.
func FilterNodesByRequirement(nodes []v1api.Node, req *labels.Requirement) []v1api.Node {
	var matches []v1api.Node

	for _, node := range nodes {
		if req.Matches(labels.Set(node.Labels)) {
			matches = append(matches, node)
		}
	}
	return matches
}

// FilterContainerLinuxNodes filters a list of nodes and returns nodes with a
// Container Linux OSImage, as reported by the node's /etc/os-release.
func FilterContainerLinuxNodes(nodes []v1api.Node) []v1api.Node {
	var matches []v1api.Node

	for _, node := range nodes {
		if strings.HasPrefix(node.Status.NodeInfo.OSImage, "Container Linux") {
			matches = append(matches, node)
		}
	}
	return matches
}
