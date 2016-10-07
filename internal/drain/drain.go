package drain

import (
	"fmt"

	"k8s.io/client-go/1.4/kubernetes"
	"k8s.io/client-go/1.4/pkg/api"
	"k8s.io/client-go/1.4/pkg/api/errors"
	v1api "k8s.io/client-go/1.4/pkg/api/v1"
	"k8s.io/client-go/1.4/pkg/fields"
	"k8s.io/client-go/1.4/pkg/kubelet/types"
	"k8s.io/client-go/1.4/pkg/runtime"
)

// GetPodsForDeletion finds pods on the given node that are candidates for
// deletion during a drain before a reboot.
// This code mimics pod filtering behavior in
// https://github.com/kubernetes/kubernetes/blob/cbbf22a7d2b06a55066b16885a4baaf4ce92d3a4/pkg/kubectl/cmd/drain.go.
// See DrainOptions.getPodsForDeletion and callees.
func GetPodsForDeletion(kc *kubernetes.Clientset, node string) (pods []v1api.Pod, err error) {
	pi := kc.Pods(api.NamespaceAll)
	podList, err := pi.List(api.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": node}),
	})
	if err != nil {
		return pods, err
	}

	for _, pod := range podList.Items {
		// skip mirror pods
		if _, ok := pod.Annotations[types.ConfigMirrorAnnotationKey]; ok {
			continue
		}

		// unlike kubelet we don't care if you have emptyDir volumes or
		// are not replicated via some controller. sorry.

		// but we do skip daemonset pods, since ds controller will just restart them
		if creatorRef, ok := pod.Annotations[api.CreatedByAnnotation]; ok {
			// decode ref to find kind
			sr := &api.SerializedReference{}
			if err := runtime.DecodeInto(api.Codecs.UniversalDecoder(), []byte(creatorRef), sr); err != nil {
				// really shouldn't happen but at least complain verbosely if it does
				return nil, fmt.Errorf("failed decoding %q annotation on pod %q: %v", api.CreatedByAnnotation, pod.Name, err)
			}

			if sr.Reference.Kind == "DaemonSet" {
				// check if daemonset still exists
				_, err := getController(kc, sr)
				if err == nil {
					// it exists, skip it
					continue
				}
				if !errors.IsNotFound(err) {
					return nil, fmt.Errorf("failed to get controller of pod %q: %v", pod.Name, err)
				}
			}

		}

		pods = append(pods, pod)
	}

	return pods, nil
}

// clone of
// https://github.com/kubernetes/kubernetes/blob/cbbf22a7d2b06a55066b16885a4baaf4ce92d3a4/pkg/kubectl/cmd/drain.go's
// getController().
func getController(kc *kubernetes.Clientset, sr *api.SerializedReference) (interface{}, error) {
	switch sr.Reference.Kind {
	case "ReplicationController":
		return kc.ReplicationControllers(sr.Reference.Namespace).Get(sr.Reference.Name)
	case "DaemonSet":
		return kc.DaemonSets(sr.Reference.Namespace).Get(sr.Reference.Name)
	case "Job":
		return kc.BatchClient.Jobs(sr.Reference.Namespace).Get(sr.Reference.Name)
	case "ReplicaSet":
		return kc.ReplicaSets(sr.Reference.Namespace).Get(sr.Reference.Name)
	}
	return nil, fmt.Errorf("unknown controller kind %q", sr.Reference.Kind)
}
