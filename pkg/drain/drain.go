package drain

import (
	"fmt"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	kubelettypes "k8s.io/kubernetes/pkg/kubelet/types"
)

// GetPodsForDeletion finds pods on the given node that are candidates for
// deletion during a drain before a reboot.
// This code mimics pod filtering behavior in
// https://github.com/kubernetes/kubernetes/blob/v1.5.4/pkg/kubectl/cmd/drain.go#L234-L245
// See DrainOptions.getPodsForDeletion and callees.
func GetPodsForDeletion(kc kubernetes.Interface, node string) (pods []v1.Pod, err error) {
	podList, err := kc.CoreV1().Pods(v1.NamespaceAll).List(v1meta.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": node}).String(),
	})
	if err != nil {
		return pods, err
	}

	// Delete pods, even if they are lone pods without a controller. As an
	// exception, skip mirror pods and daemonset pods with an existing
	// daemonset (since the daemonset owner would recreate them anyway).
	for _, pod := range podList.Items {

		// skip mirror pods
		if _, ok := pod.Annotations[kubelettypes.ConfigMirrorAnnotationKey]; ok {
			continue
		}

		// check if pod is a daemonset owner
		if _, err = getOwnerDaemonset(kc, pod); err == nil {
			continue
		}

		pods = append(pods, pod)
	}

	return pods, nil
}

// getOwnerDaemonset returns an existing DaemonSet owner if it exists.
func getOwnerDaemonset(kc kubernetes.Interface, pod v1.Pod) (interface{}, error) {
	if len(pod.OwnerReferences) == 0 {
		return nil, fmt.Errorf("pod %q has no owner objects", pod.Name)
	}
	for _, ownerRef := range pod.OwnerReferences {
		// skip pod if it is owned by an existing daemonset
		if ownerRef.Kind == "DaemonSet" {
			ds, err := getDaemonsetController(kc, pod.Namespace, &ownerRef)
			if err == nil {
				// daemonset owner exists
				return ds, nil
			}
			if !errors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get controller of pod %q: %v", pod.Name, err)
			}
		}
	}
	// pod may have owners, but they don't exist or aren't daemonsets
	return nil, fmt.Errorf("pod %q has no existing damonset owner", pod.Name)
}

// Stripped down version of https://github.com/kubernetes/kubernetes/blob/1bc56825a2dff06f29663a024ee339c25e6e6280/pkg/kubectl/cmd/drain.go#L272
func getDaemonsetController(kc kubernetes.Interface, namespace string, controllerRef *v1meta.OwnerReference) (interface{}, error) {
	switch controllerRef.Kind {
	case "DaemonSet":
		return kc.ExtensionsV1beta1().DaemonSets(namespace).Get(controllerRef.Name, v1meta.GetOptions{})
	}
	return nil, fmt.Errorf("Unknown controller kind %q", controllerRef.Kind)
}
