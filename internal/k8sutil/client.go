package k8sutil

import (
	"fmt"

	"k8s.io/client-go/1.4/kubernetes"
	"k8s.io/client-go/1.4/rest"
)

// InClusterClient gets a kubernetes client with an in-cluster configuration.
func InClusterClient() (*kubernetes.Clientset, error) {
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
