package k8sutil

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// GetClient returns a Kubernetes client (clientset) from the kubeconfig path
// or from the in-cluster service account environment.
func GetClient(path string) (*kubernetes.Clientset, error) {
	conf, err := getClientConfig(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes client config: %v", err)
	}
	return kubernetes.NewForConfig(conf)
}

// getClientConfig returns a Kubernetes client Config.
func getClientConfig(path string) (*rest.Config, error) {
	if path != "" {
		// build Config from a kubeconfig filepath
		return clientcmd.BuildConfigFromFlags("", path)
	}
	// uses pod's service account to get a Config
	return rest.InClusterConfig()
}
