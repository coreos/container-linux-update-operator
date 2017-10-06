package drain

import (
	"fmt"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
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

	for _, pod := range podList.Items {
		// skip mirror pods
		if _, ok := pod.Annotations[kubelettypes.ConfigMirrorAnnotationKey]; ok {
			continue
		}

		// unlike kubelet we don't care if you have emptyDir volumes or
		// are not replicated via some controller. sorry.

		// but we do skip daemonset pods, since ds controller will just restart them anyways.
		// As an exception, we do delete daemonset pods that have been "orphaned" by their controller.
		if creatorRef, ok := pod.Annotations[v1.CreatedByAnnotation]; ok {
			// decode ref to find kind
			sr := &v1.SerializedReference{}
			if err := runtime.DecodeInto(scheme.Codecs.UniversalDecoder(), []byte(creatorRef), sr); err != nil {
				// really shouldn't happen but at least complain verbosely if it does
				return nil, fmt.Errorf("failed decoding %q annotation on pod %q: %v", v1.CreatedByAnnotation, pod.Name, err)
			}

			if sr.Reference.Kind == "DaemonSet" {
				_, err := getDaemonsetController(kc, sr)
				if err == nil {
					// it exists, skip it
					continue
				}
				if !errors.IsNotFound(err) {
					return nil, fmt.Errorf("failed to get controller of pod %q: %v", pod.Name, err)
				}
				// else the controller is gone, fall through to delete this orphan
			}
		}

		pods = append(pods, pod)
	}

	return pods, nil
}

// Pared down version of
// https://github.com/kubernetes/kubernetes/blob/cbbf22a7d2b06a55066b16885a4baaf4ce92d3a4/pkg/kubectl/cmd/drain.go's
// getDaemonsetController().
func getDaemonsetController(kc kubernetes.Interface, sr *v1.SerializedReference) (interface{}, error) {
	switch sr.Reference.Kind {
	case "DaemonSet":
		return kc.ExtensionsV1beta1().DaemonSets(sr.Reference.Namespace).Get(sr.Reference.Name, v1meta.GetOptions{})
	}
	return nil, fmt.Errorf("unknown controller kind %q", sr.Reference.Kind)
}
