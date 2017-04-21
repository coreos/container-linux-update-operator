package operator

import "k8s.io/client-go/pkg/api/v1"

var managedByOperatorLabel = map[string]string{
	"managed-by": "prometheus-operator",
}

// updateAgent updates the agent on nodes if necessary.
// It returns a value indicating whethre the update-agent is currently up to
// date.
func (k *Kontroller) updateAgent(nodeList *v1.NodeList) (bool, error) {
	agentDaemonset, err := k.kc.DaemonSets(k.namespace).List(&v1.ListOptions{})
}
